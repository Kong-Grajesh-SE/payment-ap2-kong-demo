# Autonomous Commerce with Kong Enterprise + AP2 Protocol

> **Branch: `phase-2-custom-plugins`** - Kong enforces zero-trust (DID provisioning, signature verification, WORM audit) via custom Go plugins.
>
> For the simpler approach (agents self-manage, no DP changes), see [`main`](../../tree/main).

A demonstration of **autonomous agent-to-agent payments** governed by Kong Gateway. Four independent AI agents negotiate, authorize, and settle payments using the **AP2 (Autonomous Payment Protocol)** - with every hop routed through Kong. In this branch, **Kong is the trust authority**: it provisions DIDs, verifies signatures, and writes immutable audit records independently of the agents.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ Demo UI (React)                                                       │
│ http://localhost:5173                                                  │
└───────────────┬──────────────────────────────────────────────────────┘
                │ SSE (chat flow)
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│ BFF Server (Node.js/Express + Volcano SDK)                            │
│ http://localhost:3001                                                  │
│ • Extracts intent via Mistral LLM (through Kong /llm route)          │
│ • Orchestrates 4-step AP2 flow (through Kong /agents/* routes)       │
│ • Uses @volcano.dev/agent SDK with llmMistral() → Kong baseURL      │
└───────────────┬──────────────────────────────────────────────────────┘
                │ All requests via Kong (LLM + A2A agents)
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Kong Gateway (Konnect DP)  http://localhost:8000                      │
│                                                                       │
│ ┌───────────────────────────────────────────────────────────────────┐│
│ │ Global: opentelemetry                                             ││
│ └───────────────────────────────────────────────────────────────────┘│
│                                                                       │
│ ┌─────────────────────┐  ┌──────────────────────────────────────┐   │
│ │ Route: /llm         │  │ Routes: /agents/{search,cart-*,pay*} │   │
│ │ → api.mistral.ai    │  │ Plugins per route:                   │   │
│ │                     │  │  • kong-did-interceptor (custom)     │   │
│ │                     │  │  • kong-did-verifier   (custom)     │   │
│ │                     │  │  • ai-a2a-proxy        (bundled)    │   │
│ │                     │  │  • kong-worm-logger    (custom)     │   │
│ └────────┬────────────┘  └────────────────┬─────────────────────┘   │
└──────────┼────────────────────────────────┼─────────────────────────┘
           │                    ┌───────────┼───────────┬──────────┐
           ▼                    ▼           ▼           ▼          ▼
    ┌─────────────┐      ┌────────┐  ┌────────┐ ┌──────────┐ ┌────────┐
    │ Mistral AI  │      │ Search │  │ Cart   │ │ Cart     │ │Payment │
    │ (LLM)      │      │ Agent  │  │ Intent │ │ Mandate  │ │ Agent  │
    │ mistral.ai  │      │ :9001  │  │ :9002  │ │ :9003    │ │ :9004  │
    └─────────────┘      └───┬────┘  └───┬────┘ └────┬─────┘ └───┬────┘
                             │           │            │           │
                             └───────────┴────────────┴───────────┘
                                              │
                               ┌──────────────┼──────────────┐
                               ▼                             ▼
                         ┌───────────┐                ┌───────────┐
                         │ DID       │                │ WORM      │
                         │ Registry  │                │ Storage   │
                         │ :8070     │                │ :8090     │
                         └───────────┘                └───────────┘
```

## What This Demonstrates

| Kong Capability | How It's Used |
|----------------|---------------|
| **ai-a2a-proxy** (bundled) | Parses JSON-RPC 2.0 A2A payloads, logs agent interactions |
| **kong-did-interceptor** (custom) | Kong provisions DIDs before requests reach agents |
| **kong-did-verifier** (custom) | Kong verifies Ed25519 DID signatures on responses |
| **kong-worm-logger** (custom) | Kong writes immutable audit records independently |
| **OpenTelemetry** (bundled) | Every agent call gets distributed tracing |
| **Konnect Debugger** | Live request inspection with `KONG_CLUSTER_RPC=on` + `KONG_CLUSTER_RPC_SYNC=on` |
| **Konnect Analytics** | Full visibility into agent-to-agent traffic patterns |
| **decK `--select-tag`** | Additive config - doesn't touch existing gateway setup |
| **Custom plugin schemas** | Uploaded to Konnect CP; plugin files installed on DP via volume mount |

## Volcano SDK

The BFF server uses [`@volcano.dev/agent`](https://www.npmjs.com/package/@volcano.dev/agent) (v1.1.0+) to interact with Mistral LLM through Kong. Two main exports are used:

| Export | Usage |
|--------|-------|
| `llmMistral()` | Creates a Mistral LLM client with `baseURL` pointed at Kong's `/llm` route — all LLM calls are proxied through Kong Gateway instead of hitting Mistral directly |
| `agent()` | Wraps LLM calls with structured prompt → response chaining (`.then({ prompt }).run()`) for intent extraction and receipt generation |

```ts
import { agent, llmMistral } from "@volcano.dev/agent";

// LLM calls routed through Kong: localhost:8000/llm → api.mistral.ai
const llm = llmMistral({
  apiKey: MISTRAL_API_KEY,
  model: "mistral-small-latest",
  baseURL: `${KONG_PROXY_URL}/llm`,
});

// Structured agent call for intent extraction
const result = await agent({ llm })
  .then({ prompt: "Analyze: 'I want running shoes' ..." })
  .run();
```

This means **every LLM call** (intent extraction, greeting detection, receipt generation) flows through Kong — giving you full observability in Konnect Analytics and Debugger alongside the A2A agent traffic.

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
-e "KONG_CLUSTER_RPC_SYNC=on" \
-e "KONG_TRACING_INSTRUMENTATIONS=all" \
-e "KONG_TRACING_SAMPLING_RATE=1.0" \
-e "KONG_TLS_CERTIFICATE_VERIFY=off" \
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
  kong.deck.yml          # Agent routes + custom plugins (Phase 2, tagged: ap2-agents)
  otel-collector.yml     # OTel Collector → Jaeger config

plugins/
  kong-did-interceptor/  # Provisions DID before request reaches agent
    handler.go           # Go plugin server entry + handler logic
    main.go              # Plugin server main (PDK)
    schema.lua           # Full schema with typedefs (installed on DP)
    schema.konnect.lua   # Simplified schema, no require() (uploaded to Konnect CP)
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
  server/                # BFF (Node.js/Express) - orchestrates AP2 via Kong
  client/                # React UI (Tailwind CSS)

docker-compose.yml       # All services + OTel + Jaeger
setup.sh                 # Automated setup (--phase2 for custom plugins)
cleanup.sh               # Automated cleanup (--all for full teardown)
```

## Key Design Decisions

### Why `--select-tag ap2-agents`?
decK's `--select-tag` makes sync **additive**. It only manages entities with that tag - your existing services, routes, and plugins remain untouched. This is how you safely add AP2 to a production gateway.

### Why `ai-a2a-proxy`?
This bundled plugin understands A2A/JSON-RPC 2.0 semantics. It logs agent interaction statistics and payloads, giving Kong protocol-level awareness of the agent mesh.

### Why custom Go plugins?
In the `main` branch, agents self-manage DID/audit. This works but requires trusting the agents. In **this branch**, Kong enforces zero-trust:

| Plugin | Phase | What It Does |
|--------|-------|-------------|
| `kong-did-interceptor` | Access | Provisions a DID for each agent if it doesn't have one. Injects `X-Agent-DID` header. |
| `kong-did-verifier` | Access | Verifies the Ed25519 DID signature against the request body. Blocks if invalid. |
| `kong-worm-logger` | Log | Writes an immutable record of every A2A interaction to WORM storage. |

This means agents **cannot** bypass identity or audit - the gateway layer is the single source of truth.

### How are custom plugins distributed? (Konnect Hybrid Mode)
Per [Kong docs](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/), in Konnect hybrid mode:
1. **Upload `schema.konnect.lua`** to the Control Plane via API - Konnect uses it for config validation and the plugin catalog
2. **Install plugin files on each DP node** - via volume mount or copy into the container

No custom DP image needed. The Go plugin binaries are compiled locally and mounted into the stock Kong container.

### Why separate `schema.lua` and `schema.konnect.lua`?
- `schema.lua` - Full schema with `require "kong.db.schema.typedefs"` (installed on DP, used at runtime)
- `schema.konnect.lua` - Same schema but self-contained, no `require()` statements (Konnect CP requirement per [docs](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/#requirements))

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

## Phase 2 Setup (Custom Plugins)

After completing the Phase 1 setup (see [SETUP.md](./SETUP.md)), add the custom plugins:

### 1. Upload schemas to Konnect CP

```bash
export KONNECT_CONTROL_PLANE_ID="your-cp-id"
chmod +x scripts/upload-schemas.sh
./scripts/upload-schemas.sh
```

### 2. Build Go plugin binaries

```bash
cd plugins/kong-did-interceptor && go build -o kong-did-interceptor . && cd -
cd plugins/kong-did-verifier && go build -o kong-did-verifier . && cd -
cd plugins/kong-worm-logger && go build -o kong-worm-logger . && cd -
```

### 3. Add plugins to your existing DP

Add these to your existing Kong DP configuration (env vars + volume mounts):

```bash
# Additional env vars for your existing docker run / compose / K8s spec:
-e "KONG_PLUGINS=bundled,kong-did-interceptor,kong-did-verifier,kong-worm-logger"
-e "KONG_PLUGINSERVER_NAMES=kong-did-interceptor,kong-did-verifier,kong-worm-logger"
-e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_START_CMD=/opt/kong/plugins/kong-did-interceptor"
-e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_QUERY_CMD=/opt/kong/plugins/kong-did-interceptor -dump"
-e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_START_CMD=/opt/kong/plugins/kong-did-verifier"
-e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_QUERY_CMD=/opt/kong/plugins/kong-did-verifier -dump"
-e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_START_CMD=/opt/kong/plugins/kong-worm-logger"
-e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_QUERY_CMD=/opt/kong/plugins/kong-worm-logger -dump"

# Mount compiled binaries into the container:
-v "/path/to/kong-did-interceptor:/opt/kong/plugins/kong-did-interceptor"
-v "/path/to/kong-did-verifier:/opt/kong/plugins/kong-did-verifier"
-v "/path/to/kong-worm-logger:/opt/kong/plugins/kong-worm-logger"
```

Then recreate/restart the DP container. Your existing Konnect connection, certs, and config stay the same - you're only adding plugin support. See [SETUP.md](./SETUP.md) for Docker / Docker Compose / Kubernetes specifics.

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
| [`main`](../../tree/main) | Phase 1 - Agent routes + ai-a2a-proxy (bundled). Agents self-manage DID and audit. | **No** |
| `phase-2-custom-plugins` (this) | Phase 2 - Custom Go plugins (kong-did-interceptor, kong-did-verifier, kong-worm-logger). Kong enforces zero-trust. | **Yes** (volume mount Go binaries) |

## Related Documentation

- [SETUP.md](./SETUP.md) - Detailed step-by-step setup guide with sample responses
- [ai-a2a-proxy plugin](https://docs.konghq.com/hub/kong-inc/ai-a2a-proxy/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [Custom plugin installation & distribution](https://developer.konghq.com/custom-plugins/installation-and-distribution/)
- [decK CLI](https://docs.konghq.com/deck/latest/)
