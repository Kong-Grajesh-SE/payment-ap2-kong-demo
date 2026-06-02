# Setup Guide - AP2 Payment Demo

Step-by-step instructions to deploy the AP2 autonomous payment demo on top of an existing Kong Gateway.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Docker Desktop | 4.x+ | Run Kong DP + agent services |
| decK CLI | 1.38+ | Declarative config management |
| Konnect account | Enterprise | Control plane + Debugger |
| Mistral API key | - | LLM for intent extraction |
| Node.js | 20+ | Demo app (BFF + client) |

```bash
# Install decK (macOS)
brew install kong/deck/deck
deck version
```

---

## Phase 0: Baseline - Kong Gateway with LLM + OTel

> Skip this if you already have Kong running with an LLM route and OpenTelemetry.

### 0.1 Create a Control Plane in Konnect

1. Go to [cloud.konghq.com](https://cloud.konghq.com)
2. Create a Control Plane (e.g., `PE-Bootcamp`)
3. Note the **Control Plane ID** and generate a **Personal Access Token** (`kpat_...`)

### 0.2 Start a Data Plane

From Konnect UI: **Gateway Manager → Data Plane Nodes → New Data Plane Node → Docker**

Konnect provides a `docker run` command. Add these env vars:

```bash
docker run -d --name kong-dp \
  # ... (all Konnect-provided flags) ...
  -e "KONG_CLUSTER_RPC=on" \
  -e "KONG_TRACING_INSTRUMENTATIONS=all" \
  -e "KONG_TRACING_SAMPLING_RATE=1.0" \
  -p 8000:8000 \
  -p 8443:8443 \
  kong/kong-gateway:3.14
```

| Env Var | Purpose |
|---------|---------|
| `KONG_CLUSTER_RPC=on` | Enables Konnect Debugger (bidirectional RPC) |
| `KONG_TRACING_INSTRUMENTATIONS=all` | Full OTel tracing across all phases |
| `KONG_TRACING_SAMPLING_RATE=1.0` | 100% sampling (demo only) |

### 0.3 Sync baseline config (LLM route + OTel)

```bash
export KONNECT_API_TOKEN="kpat_..."
export KONNECT_CONTROL_PLANE_NAME="PE-Bootcamp"

deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  config/baseline.yml
```

**Verify LLM route:**
```bash
curl -s http://localhost:8000/llm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -d '{"model":"mistral-small-latest","messages":[{"role":"user","content":"hello"}]}'
```

---

## Phase 1: Add A2A Agent Mesh (Zero DP Changes)

This phase adds 4 agent microservices behind Kong with the `ai-a2a-proxy` plugin.
**No DP rebuild. No restart. Config push only.**

### 1.1 Start agent services

```bash
docker compose up -d
```

This starts:

| Service | Port | Role |
|---------|------|------|
| search-agent | 9001 | Product search, creates IntentMandate |
| cart-intent-agent | 9002 | Builds cart, creates CartMandate |
| cart-mandate-agent | 9003 | Authorizes payment, creates PaymentMandate |
| payment-agent | 9004 | Settles payment, generates receipt |
| did-registry | 8070 | W3C DID creation/resolution |
| worm-storage | 8090 | Immutable audit log (PostgreSQL) |
| otel-collector | 4318 | OpenTelemetry Collector |
| jaeger | 16686 | Distributed tracing UI |

**Verify agents are healthy:**
```bash
curl -s http://localhost:9001/health  # {"status":"healthy","agent":"search"}
curl -s http://localhost:9002/health  # {"status":"healthy","agent":"cart-intent"}
curl -s http://localhost:9003/health  # {"status":"healthy","agent":"cart-mandate"}
curl -s http://localhost:9004/health  # {"status":"healthy","agent":"payment"}
```

### 1.2 Sync agent routes + ai-a2a-proxy via decK

```bash
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.clean.yml
```

**Expected output:**
```
creating service search-svc
creating service cart-intent-svc
creating service cart-mandate-svc
creating service payment-svc
creating route search-route
creating route cart-intent-route
creating route cart-mandate-route
creating route payment-route
creating plugin ai-a2a-proxy for service search-svc
creating plugin ai-a2a-proxy for service cart-intent-svc
creating plugin ai-a2a-proxy for service cart-mandate-svc
creating plugin ai-a2a-proxy for service payment-svc
Summary:
  Created: 12
  Updated: 0
  Deleted: 0
```

> `--select-tag ap2-agents` means decK only manages entities with that tag.
> Your existing LLM route, OTel plugin, and any other config remains untouched.

### 1.3 Verify - End-to-End AP2 Flow Through Kong

#### Step 1: Search Agent - Product Discovery

```bash
curl -s -X POST http://localhost:8000/agents/search \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "search/execute",
    "params": {"intent": "running shoes"},
    "id": "test-1"
  }' | python3 -m json.tool
```

**Expected response:**
```json
{
    "jsonrpc": "2.0",
    "result": {
        "agentDID": "did:peer:2.Ez...",
        "intentMandate": {
            "type": "IntentMandate",
            "consumer": "did:peer:anonymous",
            "intent": "running shoes",
            "constraints": {
                "maxAmount": {"amount": 200, "currency": "USD"},
                "currency": "USD",
                "categories": ["footwear"]
            },
            "issuedAt": "2026-06-02T...",
            "signature": "..."
        },
        "products": [
            {"productId": "nike-am90-001", "name": "Nike Air Max 90", "price": {"amount": 130, "currency": "USD"}},
            {"productId": "nike-am90-002", "name": "Nike Air Max 90 Premium", "price": {"amount": 160, "currency": "USD"}},
            {"productId": "nike-af1-001", "name": "Nike Air Force 1 Low", "price": {"amount": 110, "currency": "USD"}}
        ]
    },
    "id": "test-1",
    "_meta": {"sender_did": "did:peer:2.Ez..."}
}
```

#### Step 2: Cart Intent Agent - Add to Cart

```bash
curl -s -X POST http://localhost:8000/agents/cart-intent \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "cart/addIntent",
    "params": {
      "intentMandate": {"type":"IntentMandate","consumer":"did:peer:anonymous","intent":"running shoes","constraints":{"maxAmount":{"amount":200,"currency":"USD"}},"issuedAt":"2026-06-02T09:40:06Z","signature":"<from-step-1>"},
      "selectedProduct": {"productId":"nike-am90-001","name":"Nike Air Max 90","price":{"amount":130,"currency":"USD"},"size":10}
    },
    "id": "test-2"
  }' | python3 -m json.tool
```

**Expected:** Returns `CartMandate` with items, totalAmount, paymentMethods, and Ed25519 signature.

#### Step 3: Cart Mandate Agent - Authorize Payment

```bash
curl -s -X POST http://localhost:8000/agents/cart-mandate \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "cart/confirmMandate",
    "params": {
      "cartMandate": {"type":"CartMandate","intentRef":"...","merchant":"Demo Commerce Store","items":[{"productId":"default-001","name":"Demo Product","quantity":1,"unitPrice":{"amount":100,"currency":"USD"}}],"totalAmount":{"amount":100,"currency":"USD"},"paymentMethods":["card","bank_transfer"],"issuedAt":"2026-06-02T09:40:23Z","signature":"<from-step-2>"},
      "paymentMethod": "card",
      "consumerDID": "did:peer:anonymous"
    },
    "id": "test-3"
  }' | python3 -m json.tool
```

**Expected:** Returns `PaymentMandate` with DPAN token, authCode, status "authorized", and signature.

#### Step 4: Payment Agent - Settle Payment

```bash
curl -s -X POST http://localhost:8000/agents/payment \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "payment/execute",
    "params": {
      "paymentMandate": {"type":"PaymentMandate","cartRef":"...","amount":{"amount":100,"currency":"USD"},"method":"card","dpan":"4000-xxxx","authCode":"a77d68","status":"authorized","issuedAt":"2026-06-02T09:40:38Z","signature":"<from-step-3>"},
      "otp": "123456"
    },
    "id": "test-4"
  }' | python3 -m json.tool
```

**Expected:** Returns settlement with `receiptId`, `status: "settled"`, `processorId`, and signed mandate.

### 1.4 What you get at this point

- ✅ All agent traffic flows through Kong (4 services, 4 routes)
- ✅ Every request gets an OTel trace (visible in Konnect Analytics)
- ✅ `ai-a2a-proxy` logs A2A payloads and statistics
- ✅ Konnect Debugger can inspect any request in real-time
- ✅ Each agent has its own DID (Ed25519 key pair)
- ✅ Every mandate is cryptographically signed
- ✅ Zero changes to the Data Plane

---

## Phase 2: Run the Demo Application

### 2.1 Configure environment

```bash
cp .env.example .env
# Edit .env with your values
```

### 2.2 Start demo server + client

```bash
# Terminal 1: BFF server
cd demo/server && npm install && npm run dev

# Terminal 2: React client
cd demo/client && npm install && npm run dev
```

### 2.3 Use the demo

Open **http://localhost:5173** and type a shopping intent (e.g., "I want to buy running shoes").

| Step | User Action | What Happens (via Kong) |
|------|-------------|------------------------|
| 1 | Types "buy running shoes" | Mistral extracts intent → `POST /llm` |
| 2 | - | Search Agent called → `POST /agents/search` |
| 3 | Picks a product | Cart Intent Agent called → `POST /agents/cart-intent` |
| 4 | Selects payment method | Wallet loaded (simulated locally) |
| 5 | Confirms purchase | Cart Mandate Agent called → `POST /agents/cart-mandate` |
| 6 | Enters OTP (`123`) | Payment Agent called → `POST /agents/payment` |
| 7 | - | DID verified, WORM audit written, receipt generated |

Every hop goes through Kong → ai-a2a-proxy → OTel → visible in Konnect.

---

## Observability

### Konnect Analytics
All agent traffic appears in Konnect Analytics under the `ap2-agents` tagged services.

### Konnect Debugger
With `KONG_CLUSTER_RPC=on`, inspect any live request:
- Request/response bodies (A2A JSON-RPC payloads)
- Plugin execution order and timing
- Trace propagation across agents

### Jaeger (local)
```bash
open http://localhost:16686
```

---

## Cleanup

### Remove only AP2 entities (leave LLM + OTel intact):
```bash
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  /dev/stdin <<< '_format_version: "3.0"'
```

### Stop all services:
```bash
docker compose down
```

---

---

## Phase 3: Custom Plugins - Zero-Trust Gateway Enforcement

> This section is specific to the `phase-2-custom-plugins` branch.
>
> Reference: [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/) | [Installation & distribution](https://developer.konghq.com/custom-plugins/installation-and-distribution/)

### 3.1 Upload plugin schemas to Konnect CP

In Konnect hybrid mode, you upload the plugin's `schema.lua` (without `require()` statements) to the Control Plane via API. Konnect uses it for config validation and the plugin catalog.

```bash
export KONNECT_CONTROL_PLANE_ID="your-cp-id"  # From Konnect UI
chmod +x scripts/upload-schemas.sh
./scripts/upload-schemas.sh
```

**Expected output:**
```
=== Uploading Custom Plugin Schemas to Konnect CP ===
Region: us
Control Plane: 2e94e75e-66dc-4083-99a2-24ca016c420a

📤 Uploading schema for kong-did-interceptor...
✅ kong-did-interceptor schema uploaded successfully
📤 Uploading schema for kong-did-verifier...
✅ kong-did-verifier schema uploaded successfully
📤 Uploading schema for kong-worm-logger...
✅ kong-worm-logger schema uploaded successfully

=== Verifying uploads ===
  ✓ kong-did-interceptor - verified in CP
  ✓ kong-did-verifier - verified in CP
  ✓ kong-worm-logger - verified in CP

=== Done ===
```

**Verify manually** (optional):
```bash
curl -s https://us.api.konghq.com/v2/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas/kong-did-interceptor \
  -H "Authorization: Bearer $KONNECT_API_TOKEN" | jq .name
# "kong-did-interceptor"
```

### 3.2 Build Go plugin binaries

Compile the Go plugin servers locally. These binaries will be volume-mounted into the Kong DP container.

```bash
# Build for Linux (since Kong DP runs in a Linux container)
cd plugins/kong-did-interceptor && GOOS=linux GOARCH=amd64 go build -o kong-did-interceptor . && cd -
cd plugins/kong-did-verifier && GOOS=linux GOARCH=amd64 go build -o kong-did-verifier . && cd -
cd plugins/kong-worm-logger && GOOS=linux GOARCH=amd64 go build -o kong-worm-logger . && cd -
```

### 3.3 Add plugins to your existing Data Plane

You likely already have a Kong DP running. You only need to **add** the following to your existing DP configuration:

#### What to add to your existing `docker run` (or docker-compose / K8s spec):

**Additional environment variables:**
```bash
# Tell Kong about the custom plugins
-e "KONG_PLUGINS=bundled,kong-did-interceptor,kong-did-verifier,kong-worm-logger"

# Go plugin server configuration
-e "KONG_PLUGINSERVER_NAMES=kong-did-interceptor,kong-did-verifier,kong-worm-logger"
-e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_START_CMD=/opt/kong/plugins/kong-did-interceptor"
-e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_QUERY_CMD=/opt/kong/plugins/kong-did-interceptor -dump"
-e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_START_CMD=/opt/kong/plugins/kong-did-verifier"
-e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_QUERY_CMD=/opt/kong/plugins/kong-did-verifier -dump"
-e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_START_CMD=/opt/kong/plugins/kong-worm-logger"
-e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_QUERY_CMD=/opt/kong/plugins/kong-worm-logger -dump"
```

**Additional volume mounts** (map compiled binaries into the container):
```bash
-v "/path/to/kong-did-interceptor:/opt/kong/plugins/kong-did-interceptor"
-v "/path/to/kong-did-verifier:/opt/kong/plugins/kong-did-verifier"
-v "/path/to/kong-worm-logger:/opt/kong/plugins/kong-worm-logger"
```

#### Applying the change

**Docker** - Container env vars and volumes can't be changed at runtime, so recreate:
```bash
# Note your existing container's full config first
docker inspect kong-dp --format '{{json .Config}}' > /tmp/kong-dp-config.json

# Recreate with additional flags
docker stop kong-dp && docker rm kong-dp
docker run -d --name kong-dp \
  ... (your existing flags) ...
  ... (add the env vars and volumes above) ...
  kong/kong-gateway:3.14
```

**Docker Compose** - Add to your existing service definition, then:
```bash
docker compose up -d    # only recreates the changed service
```

**Kubernetes** - Add env vars and volume mounts to your existing DP Deployment spec:
```yaml
# In your Kong DP Deployment spec, add to containers[0]:
env:
  - name: KONG_PLUGINS
    value: "bundled,kong-did-interceptor,kong-did-verifier,kong-worm-logger"
  - name: KONG_PLUGINSERVER_NAMES
    value: "kong-did-interceptor,kong-did-verifier,kong-worm-logger"
  # ... (START_CMD and QUERY_CMD for each plugin)
volumeMounts:
  - name: custom-plugins
    mountPath: /opt/kong/plugins
volumes:
  - name: custom-plugins
    configMap:
      name: kong-custom-plugins   # or hostPath / PVC with the binaries
```
Then `kubectl rollout restart deployment/kong-dp`.

> **Key point:** You're adding to your existing DP - not replacing it. Your existing
> Konnect connection, certificates, and configuration stay exactly the same.
> The only additions are the plugin env vars and binary mounts.

### 3.4 Sync full config (with custom plugins)

```bash
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.yml
```

**Expected output:**
```
updating plugin ai-a2a-proxy for service search-svc
creating plugin kong-did-interceptor for service search-svc
creating plugin kong-did-verifier for service search-svc
creating plugin kong-worm-logger for service search-svc
...
Summary:
  Created: 12
  Updated: 4
  Deleted: 0
```

### 3.5 Verify custom plugins are active

```bash
curl -s -X POST http://localhost:8000/agents/search \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"search/execute","params":{"intent":"shoes"},"id":"1"}' \
  | python3 -m json.tool
```

In the response, look for:
- `_meta.sender_did` - DID provisioned by `kong-did-interceptor`
- Response passes through `kong-did-verifier` without 403
- WORM storage has a new record: `curl -s http://localhost:8090/records | python3 -m json.tool`

---

## Next Steps

This branch demonstrates the full zero-trust approach. Every agent call now goes through:

1. **kong-did-interceptor** (Access phase) - provisions DID, signs request body, injects headers
2. **kong-did-verifier** (Access phase) - resolves DID, verifies Ed25519 signature against request body
3. **Agent processes request** - receives verified identity context
4. **kong-worm-logger** (Log phase) - writes immutable audit record after response

Agents cannot bypass identity or audit - Kong is the single source of truth.

For the simpler approach where agents self-manage DID/audit (no DP changes needed), see the [`main`](../../tree/main) branch.
