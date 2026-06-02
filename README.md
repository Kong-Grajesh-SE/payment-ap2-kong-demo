# Autonomous Commerce with Kong Enterprise + AP2 Protocol

A demonstration of **autonomous agent-to-agent payments** governed by Kong Gateway. Four independent AI agents negotiate, authorize, and settle payments using the **AP2 (Autonomous Payment Protocol)** — with every hop routed through Kong for observability, governance, and A2A protocol awareness.

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
| **OpenTelemetry** (bundled) | Every agent call gets distributed tracing |
| **Konnect Debugger** | Live request inspection with `KONG_CLUSTER_RPC=on` |
| **Konnect Analytics** | Full visibility into agent-to-agent traffic patterns |
| **decK `--select-tag`** | Additive config — doesn't touch existing gateway setup |
| **Route-based agent mesh** | Kong as the A2A traffic mesh — all inter-agent calls visible |

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
# Baseline (LLM route + OTel) — scoped by tag, won't touch agent mesh
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-baseline \
  config/baseline.yml

# Agent mesh (additive — won't touch baseline config)
deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.clean.yml
```

> **Or use the automated setup:** `./setup.sh` handles all of the above.

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
  baseline.yml           # LLM route + OpenTelemetry (Phase 0, tagged: ap2-baseline)
  kong.deck.clean.yml    # Agent routes + ai-a2a-proxy (Phase 1, tagged: ap2-agents)
  otel-collector.yml     # OTel Collector → Jaeger config

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
setup.sh                 # Automated setup script
cleanup.sh               # Automated cleanup script
```

## Key Design Decisions

### Why `--select-tag ap2-agents`?
decK's `--select-tag` makes sync **additive**. It only manages entities with that tag — your existing services, routes, and plugins remain untouched. This is how you safely add AP2 to a production gateway.

### Why `ai-a2a-proxy`?
This bundled plugin understands A2A/JSON-RPC 2.0 semantics. It logs agent interaction statistics and payloads, giving Kong protocol-level awareness of the agent mesh.

### Why agents self-manage DID/audit?
In this branch, each agent registers its own DID and writes its own audit records. This keeps the gateway layer simple (no custom plugins, no DP rebuild). For the **zero-trust gateway enforcement** approach where Kong manages DID/audit, see the [`phase-2-custom-plugins`](../../tree/phase-2-custom-plugins) branch.

## Cleanup

```bash
# Automated cleanup
./cleanup.sh              # Remove agent mesh + stop Docker
./cleanup.sh --all        # Full teardown (agents + baseline + volumes + node_modules)
```

Or manually:
```bash
# Remove agent mesh entities
deck gateway reset \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents --force

# Remove baseline entities
deck gateway reset \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-baseline --force

# Stop services
docker compose down
```

## Branch Strategy

| Branch | What | DP Change Required? |
|--------|------|---------------------|
| `main` | Phase 1 — Agent routes + ai-a2a-proxy (bundled). Agents self-manage DID and audit. | **No** |
| `phase-2-custom-plugins` | Phase 2 — Custom Go plugins (kong-did-interceptor, kong-did-verifier, kong-worm-logger). Kong enforces zero-trust. | **Yes** (volume mount Go binaries) |

## Related Documentation

- [SETUP.md](./SETUP.md) — Detailed step-by-step setup guide with sample responses
- [ai-a2a-proxy plugin](https://docs.konghq.com/hub/kong-inc/ai-a2a-proxy/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [decK CLI](https://docs.konghq.com/deck/latest/)
