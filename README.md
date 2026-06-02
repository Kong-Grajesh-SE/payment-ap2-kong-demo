# Autonomous Commerce with Kong Enterprise + AP2 Protocol

> **Branch: `phase-2-custom-plugins`** — Kong enforces zero-trust (DID provisioning, signature verification, WORM audit) via custom Go plugins. Requires a custom DP image.
>
> For the simpler approach (agents self-manage, no DP changes), see [`main`](../../tree/main).

A demonstration of **autonomous agent-to-agent payments** governed by Kong Gateway. Four independent AI agents negotiate, authorize, and settle payments using the **AP2 (Autonomous Payment Protocol)** — with every hop routed through Kong. In this branch, **Kong is the trust authority**: it provisions DIDs, verifies signatures, and writes immutable audit records independently of the agents.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ Demo UI (React)                                                       │
│ http://localhost:5173                                                  │
└───────────────┬──────────────────────────────────────────────────────┘
                │ SSE (chat flow)
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│ BFF Server (Node.js/Express)                                          │
│ http://localhost:3001                                                  │
│ • Extracts intent via Mistral LLM (through Kong)                      │
│ • Orchestrates 4-step AP2 flow (through Kong)                         │
│ • Verifies DIDs + writes WORM audit                                   │
└───────────────┬──────────────────────────────────────────────────────┘
                │ JSON-RPC 2.0 / A2A Protocol
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Kong Gateway (Konnect DP)                                             │
│ http://localhost:8000                                                  │
│ ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐   │
│ │ ai-a2a-proxy     │  │ opentelemetry    │  │ Route: /llm       │   │
│ │ (per agent route)│  │ (global)         │  │ Route: /agents/*  │   │
│ └──────────────────┘  └──────────────────┘  └───────────────────┘   │
└───────────────┬──────────────────────────────────────────────────────┘
                │
    ┌───────────┼───────────┬───────────────┐
    ▼           ▼           ▼               ▼
┌────────┐ ┌────────┐ ┌────────────┐ ┌─────────┐
│ Search │ │ Cart   │ │ Cart       │ │ Payment │
│ Agent  │ │ Intent │ │ Mandate    │ │ Agent   │
│ :9001  │ │ :9002  │ │ :9003      │ │ :9004   │
└───┬────┘ └───┬────┘ └─────┬──────┘ └────┬────┘
    │          │             │              │
    └──────────┴─────────────┴──────────────┘
                        │
          ┌─────────────┼─────────────┐
          ▼                           ▼
    ┌───────────┐              ┌───────────┐
    │ DID       │              │ WORM      │
    │ Registry  │              │ Storage   │
    │ :8070     │              │ :8090     │
    └───────────┘              └───────────┘
```

## What This Demonstrates

| Kong Capability | How It's Used |
|----------------|---------------|
| **ai-a2a-proxy** (bundled) | Parses JSON-RPC 2.0 A2A payloads, logs agent interactions |
| **kong-did-interceptor** (custom) | Kong provisions DIDs before requests reach agents |
| **kong-did-verifier** (custom) | Kong verifies Ed25519 DID signatures on responses |
| **kong-worm-logger** (custom) | Kong writes immutable audit records independently |
| **OpenTelemetry** (bundled) | Every agent call gets distributed tracing |
| **Konnect Debugger** | Live request inspection with `KONG_CLUSTER_RPC=on` |
| **Konnect Analytics** | Full visibility into agent-to-agent traffic patterns |
| **decK `--select-tag`** | Additive config — doesn't touch existing gateway setup |
| **Custom plugin schemas** | Uploaded to Konnect CP; Go binaries baked into DP image |

## AP2 Protocol Flow

```
Step 1: search/execute      → IntentMandate (Ed25519 signed)
Step 2: cart/addIntent      → CartMandate (Ed25519 signed)
Step 3: cart/confirmMandate → PaymentMandate (DPAN + authCode)
Step 4: payment/execute     → Settlement (receipt + audit)
```

Each mandate is cryptographically signed by the issuing agent's DID. The chain of mandates forms a verifiable audit trail.

## Quick Start

### Prerequisites

- Docker Desktop 4.x+
- [decK CLI](https://docs.konghq.com/deck/latest/installation/) 1.38+
- Kong Konnect account (Enterprise) with a Control Plane
- Mistral API key ([console.mistral.ai](https://console.mistral.ai))
- Node.js 20+

### 1. Clone and configure

```bash
git clone https://github.com/Kong-Grajesh-SE/payment-ap2-kong-demo.git
cd payment-ap2-kong-demo
cp .env.example .env
# Edit .env with your values
```

### 2. Provision Kong Data Plane

From Konnect UI: **Gateway Manager → Data Plane Nodes → New Data Plane Node → Docker**

Add these env vars to the `docker run` command Konnect provides:

```bash
-e "KONG_CLUSTER_RPC=on" \
-e "KONG_TRACING_INSTRUMENTATIONS=all" \
-e "KONG_TRACING_SAMPLING_RATE=1.0" \
-p 8000:8000 -p 8443:8443
```

### 3. Start agent services

```bash
docker compose up -d
```

### 4. Sync Kong configuration

```bash
# Baseline (LLM route + OTel)
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  config/baseline.yml

# Agent mesh (additive — won't touch existing config)
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.clean.yml
```

### 5. Run the demo app

```bash
# Terminal 1: BFF server
cd demo/server && npm install && npm run dev

# Terminal 2: React client
cd demo/client && npm install && npm run dev
```

Open **http://localhost:5173** and type "I want to buy running shoes".

## Verify Agent Routes

```bash
# Health checks
curl -s http://localhost:9001/health  # search
curl -s http://localhost:9002/health  # cart-intent
curl -s http://localhost:9003/health  # cart-mandate
curl -s http://localhost:9004/health  # payment

# Test through Kong
curl -s -X POST http://localhost:8000/agents/search \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"search/execute","params":{"intent":"shoes"},"id":"1"}'
```

## File Structure

```
config/
  baseline.yml           # LLM route + OpenTelemetry (Phase 0)
  kong.deck.clean.yml    # Agent routes + ai-a2a-proxy (Phase 1, additive)
  kong.deck.yml          # Full config with custom plugins (Phase 2)
  otel-collector.yml     # OTel Collector → Jaeger config

plugins/
  Dockerfile.kong-dp     # Multi-stage build: Go plugins → custom DP image
  kong-did-interceptor/  # Provisions DID before request reaches agent
    handler.go           # Access-phase logic
    schema.lua           # Full schema (used on DP)
    schema.konnect.lua   # Simplified schema (uploaded to Konnect CP)
  kong-did-verifier/     # Verifies DID signature on agent responses
  kong-worm-logger/      # Writes WORM audit record independently

scripts/
  upload-schemas.sh      # Upload plugin schemas to Konnect CP

services/
  agents/                # 4 Go microservices (A2A JSON-RPC)
    ap2/                 # Shared AP2 package (mandates, signing, JSON-RPC)
    search/              # Product search + IntentMandate
    cart-intent/         # Cart creation + CartMandate
    cart-mandate/        # Payment authorization + PaymentMandate
    payment/             # Settlement + receipt
  did-registry/          # W3C DID:peer registry (Ed25519)
  worm-storage/          # Write-Once-Read-Many audit (PostgreSQL)

demo/
  server/                # BFF (Node.js/Express) — orchestrates AP2 via Kong
  client/                # React UI (Tailwind CSS)

docker-compose.yml       # All services + OTel + Jaeger
```

## Key Design Decisions

### Why `--select-tag ap2-agents`?
decK's `--select-tag` makes sync **additive**. It only manages entities with that tag — your existing services, routes, and plugins remain untouched. This is how you safely add AP2 to a production gateway.

### Why `ai-a2a-proxy`?
This bundled plugin understands A2A/JSON-RPC 2.0 semantics. It logs agent interaction statistics and payloads, giving Kong protocol-level awareness of the agent mesh.

### Why custom Go plugins?
In the `main` branch, agents self-manage DID/audit. This works but requires trusting the agents. In **this branch**, Kong enforces zero-trust:

| Plugin | Phase | What It Does |
|--------|-------|-------------|
| `kong-did-interceptor` | Access | Provisions a DID for each agent if it doesn't have one. Injects `X-Agent-DID` header. |
| `kong-did-verifier` | Response | Verifies the Ed25519 signature in `_meta.sender_did` matches the response body. |
| `kong-worm-logger` | Log | Writes an immutable record of every A2A interaction to WORM storage. |

This means agents **cannot** bypass identity or audit — the gateway layer is the single source of truth.

### Why a custom DP image?
Kong's Go plugin support requires compiling plugins into the data plane binary. The `plugins/Dockerfile.kong-dp` multi-stage build:
1. Compiles each Go plugin into a shared object
2. Copies the `.so` files into the Kong DP image
3. Configures `KONG_PLUGINSERVER_NAMES` and plugin paths

### Why separate `schema.lua` and `schema.konnect.lua`?
- `schema.lua` — Full schema with `typedefs` (used on the DP at runtime)
- `schema.konnect.lua` — Simplified schema without `require` statements (uploaded to Konnect CP via API so it can render the plugin config form)

## Cleanup

```bash
# Remove only AP2 entities (leave LLM + OTel)
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  /dev/stdin <<< '_format_version: "3.0"'

# Stop services
docker compose down
```

## Phase 2 Setup (Custom Plugins)

After completing the Phase 1 setup (see [SETUP.md](./SETUP.md)), add the custom plugins:

### 1. Upload schemas to Konnect

```bash
export KONNECT_CONTROL_PLANE_ID="your-cp-id"
chmod +x scripts/upload-schemas.sh
./scripts/upload-schemas.sh
```

### 2. Build custom DP image

```bash
docker build -f plugins/Dockerfile.kong-dp -t kong-dp-custom .
```

### 3. Replace the vanilla DP

```bash
docker stop kong-dp
docker run -d --name kong-dp-custom \
  # ... (same Konnect flags as Phase 1) ...
  -e "KONG_CLUSTER_RPC=on" \
  -e "KONG_TRACING_INSTRUMENTATIONS=all" \
  -e "KONG_TRACING_SAMPLING_RATE=1.0" \
  -p 8000:8000 -p 8443:8443 \
  kong-dp-custom
```

### 4. Sync full config (with custom plugins)

```bash
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.yml
```

Now every agent call goes through DID provisioning → agent → DID verification → WORM audit, all enforced by Kong.

## Branch Strategy

| Branch | What | DP Change Required? |
|--------|------|---------------------|
| [`main`](../../tree/main) | Phase 1 — Agent routes + ai-a2a-proxy (bundled). Agents self-manage DID and audit. | **No** |
| `phase-2-custom-plugins` (this) | Phase 2 — Custom Go plugins (kong-did-interceptor, kong-did-verifier, kong-worm-logger). Kong enforces zero-trust. | **Yes** (custom DP image) |

## Related Documentation

- [SETUP.md](./SETUP.md) — Detailed step-by-step setup guide with sample responses
- [ai-a2a-proxy plugin](https://docs.konghq.com/hub/kong-inc/ai-a2a-proxy/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [decK CLI](https://docs.konghq.com/deck/latest/)
