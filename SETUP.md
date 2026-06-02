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
  -e "KONG_CLUSTER_RPC_SYNC=on" \
  -e "KONG_TRACING_INSTRUMENTATIONS=all" \
  -e "KONG_TRACING_SAMPLING_RATE=1.0" \
  -e "KONG_TLS_CERTIFICATE_VERIFY=off" \
  -p 8000:8000 \
  -p 8443:8443 \
  kong/kong-gateway:3.14
```

| Env Var | Purpose |
|---------|---------|
| `KONG_CLUSTER_RPC=on` | Enables Konnect Debugger (bidirectional RPC) |
| `KONG_CLUSTER_RPC_SYNC=on` | Enables config sync over RPC channel |
| `KONG_TRACING_INSTRUMENTATIONS=all` | Full OTel tracing across all phases |
| `KONG_TRACING_SAMPLING_RATE=1.0` | 100% sampling (demo only) |
| `KONG_TLS_CERTIFICATE_VERIFY=off` | Skip TLS cert verification (demo only) |

### 0.3 Sync baseline config (LLM route + OTel)

```bash
export KONNECT_API_TOKEN="kpat_..."
export KONNECT_CONTROL_PLANE_NAME="PE-Bootcamp"

deck gateway sync \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-baseline \
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
> Similarly, baseline uses `--select-tag ap2-baseline` so both syncs are independent.

> **Automated alternative:** `./setup.sh` handles Phases 0–2 automatically.

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

## Run the Demo Application

### Configure environment

```bash
cp .env.example .env
# Edit .env with your values
```

### Start demo server + client

```bash
# Terminal 1: BFF server
cd demo/server && npm install && npm run dev

# Terminal 2: React client
cd demo/client && npm install && npm run dev
```

### Use the demo

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
deck gateway reset \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-agents --force
```

### Remove baseline entities:
```bash
deck gateway reset \
  --konnect-token "$KONNECT_API_TOKEN" \
  --konnect-control-plane-name "$KONNECT_CONTROL_PLANE_NAME" \
  --select-tag ap2-baseline --force
```

### Stop all services:
```bash
docker compose down
```

> **Automated alternative:** `./cleanup.sh` or `./cleanup.sh --all` for full teardown.

---

## Next Steps: Custom Plugins (Phase 2)

For the **zero-trust gateway enforcement** approach - where Kong (not agents) manages DID provisioning, signature verification, and WORM audit - check out the [`phase-2-custom-plugins`](../../tree/phase-2-custom-plugins) branch.

That branch adds:
- `kong-did-interceptor` - Kong provisions DIDs before requests reach agents
- `kong-did-verifier` - Kong verifies DID signatures on responses
- `kong-worm-logger` - Kong writes immutable audit records independently

This requires a custom DP image (Go plugin binaries baked in) and uploading Lua schemas to Konnect.
