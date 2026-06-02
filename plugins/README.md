# Custom Kong Gateway Plugins - AP2 Zero-Trust Enforcement

These three Go plugins move trust enforcement from the agent layer to the **gateway layer**. Instead of trusting agents to self-manage identity and audit, Kong provisions DIDs, verifies signatures, and writes immutable records - independently and transparently.

## Why Custom Plugins?

In the `main` branch, each agent:
- Registers its own DID with the DID Registry
- Signs its own responses
- Writes its own audit records to WORM storage

This works for demos, but in production it means **you must trust every agent**. A compromised or malicious agent could skip signing, forge a DID, or omit audit records.

These plugins solve that by making Kong the **single enforcement point**:

```
Request → [kong-did-interceptor] → Agent → [kong-did-verifier] → [kong-worm-logger] → Response
              (Access phase)                  (Access phase)         (Log phase)
```

| Guarantee | Without Plugins | With Plugins |
|-----------|----------------|--------------|
| Every agent has a DID | Agent must self-register | Kong provisions automatically |
| Responses are signed | Agent must sign correctly | Kong verifies independently |
| Audit trail exists | Agent must write records | Kong writes unconditionally |
| Agents can bypass controls | Yes | No - gateway layer enforces |

---

## Plugin 1: `kong-did-interceptor`

**Phase:** Access (runs before request reaches the upstream agent)

**What it does:**
1. Checks if the request already carries an `X-Agent-DID` header
2. If not, provisions a new W3C DID:peer from the DID Registry (Ed25519 key pair)
3. Signs the raw request body with the new DID's private key
4. Injects headers: `X-Agent-DID`, `X-DID-Provisioned-At`, `X-DID-Signature`
5. Optionally injects a `kong_did_context` DataPart into A2A message parts

**Why it's needed:** Ensures every agent interaction has a cryptographic identity, even if the calling agent doesn't provide one. The agent doesn't need to know about DID management.

**Configuration:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `did_registry_url` | string | `http://did-registry:8070` | DID Registry endpoint |
| `did_method` | string | `peer` | DID method (`peer` or `web`) |
| `did_web_domain` | string | `localhost` | Domain for `did:web` method |

---

## Plugin 2: `kong-did-verifier`

**Phase:** Access (runs after interceptor, before upstream)

**What it does:**
1. Reads `X-Agent-DID` and `X-DID-Signature` headers
2. Resolves the DID Document from the DID Registry to get the public key
3. Verifies the Ed25519 signature against the raw request body
4. If verification fails → returns JSON-RPC error `-32003` (blocks the request)
5. If verification passes → sets `X-Kong-DID-Verified: true` and `X-Kong-Trust-Score` headers

**Why it's needed:** Prevents forged or tampered requests from reaching agents. Even if an attacker injects a fake `X-Agent-DID` header, they can't produce a valid signature without the private key.

**Configuration:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `did_registry_url` | string | `http://did-registry:8070` | DID Registry for resolution |

---

## Plugin 3: `kong-worm-logger`

**Phase:** Access (captures body) + Log (writes record after response is sent)

**What it does:**
1. **Access phase:** Reads the request body and stashes JSON-RPC method + mandate data in `kong.ctx.shared` (body isn't available in Log phase)
2. **Log phase:** Assembles an audit record with:
   - W3C Trace ID + Span ID (from `traceparent` header)
   - Sender/Receiver DIDs
   - JSON-RPC method name
   - Mandate type and payload
   - Kong verification status and trust score
3. POSTs the record to WORM Storage (write-once, immutable)

**Why it's needed:** Creates a tamper-proof audit trail that agents cannot bypass or alter. Every A2A interaction is recorded with its cryptographic identity and verification status - independent of what the agents report.

**Configuration:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `worm_storage_url` | string | `http://worm-storage:8090` | WORM Storage endpoint |

---

## Plugin Execution Order

All three plugins run on each agent service route. Kong executes them by priority:

```
Request arrives at Kong
  │
  ├─ 1. kong-did-interceptor (priority: 1000)
  │     → Provisions DID, signs body, injects headers
  │
  ├─ 2. kong-did-verifier (priority: 900)
  │     → Resolves DID, verifies signature
  │     → Blocks request if signature invalid
  │
  ├─ 3. ai-a2a-proxy (bundled)
  │     → A2A protocol awareness, logging
  │
  ▼ [Request forwarded to agent]
  
  ... agent processes and responds ...

  ├─ 4. kong-worm-logger (log phase, priority: 100)
  │     → Writes immutable audit record
  │
  ▼ Response returned to caller
```

---

## File Structure

```
plugins/
├── README.md                    # This file
├── kong-did-interceptor/
│   ├── handler.go               # Access-phase: DID provisioning + signing
│   ├── main.go                  # Go PDK plugin server entry
│   ├── go.mod                   # Go module
│   ├── schema.lua               # Full schema with typedefs (DP runtime)
│   └── schema.konnect.lua       # No-require schema (Konnect CP upload)
├── kong-did-verifier/
│   ├── handler.go               # Access-phase: DID resolution + Ed25519 verify
│   ├── main.go                  # Go PDK plugin server entry
│   ├── go.mod
│   ├── schema.lua
│   └── schema.konnect.lua
└── kong-worm-logger/
    ├── handler.go               # Access (capture) + Log (write) phases
    ├── main.go                  # Go PDK plugin server entry
    ├── go.mod
    ├── schema.lua
    └── schema.konnect.lua
```

---

## Distribution (Konnect Hybrid Mode)

Per [Kong docs](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/):

1. **Upload `schema.konnect.lua`** to Konnect CP via API - Konnect uses it for config validation
2. **Install Go plugin binaries** on each DP node via volume mount

```bash
# 1. Upload schemas
./scripts/upload-schemas.sh

# 2. Build binaries (for Linux container)
cd plugins/kong-did-interceptor && GOOS=linux GOARCH=amd64 go build -o kong-did-interceptor .
cd plugins/kong-did-verifier && GOOS=linux GOARCH=amd64 go build -o kong-did-verifier .
cd plugins/kong-worm-logger && GOOS=linux GOARCH=amd64 go build -o kong-worm-logger .

# 3. Volume-mount into stock Kong DP container
docker run -d --name kong-dp \
  -e "KONG_PLUGINS=bundled,kong-did-interceptor,kong-did-verifier,kong-worm-logger" \
  -e "KONG_PLUGINSERVER_NAMES=kong-did-interceptor,kong-did-verifier,kong-worm-logger" \
  -v "$(pwd)/plugins/kong-did-interceptor/kong-did-interceptor:/opt/kong/plugins/kong-did-interceptor" \
  -v "$(pwd)/plugins/kong-did-verifier/kong-did-verifier:/opt/kong/plugins/kong-did-verifier" \
  -v "$(pwd)/plugins/kong-worm-logger/kong-worm-logger:/opt/kong/plugins/kong-worm-logger" \
  ... # other Konnect flags
  kong/kong-gateway:3.14
```

No custom DP image needed. See [SETUP.md](../SETUP.md) for full instructions.

---

## Schema Files Explained

Each plugin has two schema files:

| File | Used Where | `require()` allowed? | Purpose |
|------|-----------|---------------------|---------|
| `schema.lua` | Data Plane | Yes | Full schema with `kong.db.schema.typedefs` for runtime validation |
| `schema.konnect.lua` | Control Plane | **No** | Self-contained schema uploaded via API; Konnect renders plugin config UI |

The Konnect requirement of no `require()` statements is documented at [developer.konghq.com](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/#requirements).
