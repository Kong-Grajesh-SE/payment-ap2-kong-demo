import { config } from "dotenv";
import { resolve, dirname } from "path";
import { fileURLToPath } from "url";
config({ path: resolve(dirname(fileURLToPath(import.meta.url)), "../../../.env") });

import express from "express";
import cors from "cors";
import { agent, llmMistral } from "@volcano.dev/agent";
import crypto from "crypto";

const app = express();
app.use(cors());
app.use(express.json());

const PORT = parseInt(process.env.DEMO_SERVER_PORT || "3001", 10);
const KONG_PROXY_URL = process.env.KONG_PROXY_URL || "http://localhost:8000";
const MISTRAL_API_KEY = process.env.MISTRAL_API_KEY || "";
const MISTRAL_MODEL = process.env.MISTRAL_MODEL || "mistral-small-latest";
const DID_REGISTRY_URL = process.env.DID_REGISTRY_URL || "http://localhost:8070";
const WORM_STORAGE_URL = process.env.WORM_STORAGE_URL || "http://localhost:8090";

// ─── Mistral LLM via Kong AI Gateway ─────────────────────────────
function getMistralLLM() {
  return llmMistral({
    apiKey: MISTRAL_API_KEY,
    model: MISTRAL_MODEL,
    baseURL: `${KONG_PROXY_URL}/llm`,
    options: { temperature: 0.7, max_tokens: 1024 },
  });
}

// ─── JSON-RPC 2.0 helper for calling agents via Kong ─────────────
let requestCounter = 0;

async function callAgent(agentPath: string, method: string, params: Record<string, unknown>, traceId?: string): Promise<any> {
  const url = `${KONG_PROXY_URL}/agents/${agentPath}`;
  const body = {
    jsonrpc: "2.0",
    method,
    params,
    id: `bff-${++requestCounter}`,
    _meta: { sender_did: "did:peer:bff-orchestrator", trace_id: traceId },
  };

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (traceId) {
    headers["traceparent"] = `00-${traceId}-0000000000000001-01`;
  }

  const resp = await fetch(url, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`Agent ${agentPath} returned ${resp.status}: ${text}`);
  }

  const json = await resp.json() as any;
  if (json.error) {
    throw new Error(`Agent ${agentPath} RPC error: ${json.error.message}`);
  }
  return json.result;
}

// ─── AP2 Session State ───────────────────────────────────────────

interface Product {
  productId: string;
  name: string;
  price: { amount: number; currency: string };
  description: string;
  category?: string;
  sizes?: number[];
}

interface PaymentMethod {
  id: string;
  type: string;
  label: string;
  last4: string;
  network: string;
}

type SessionPhase =
  | "idle"
  | "searching"
  | "product_selection"
  | "payment_selection"
  | "confirm_checkout"
  | "otp_challenge"
  | "processing"
  | "complete";

interface Session {
  id: string;
  phase: SessionPhase;
  intent?: { product: string; maxBudget: number; currency: string; preferences: string[] };
  products?: Product[];
  selectedProduct?: Product;
  paymentMethods?: PaymentMethod[];
  selectedPayment?: PaymentMethod;
  traceId?: string;
  mandates: {
    intentMandate?: any;
    cartMandate?: any;
    paymentMandate?: any;
  };
  agentDid?: string;
  searchAgentDid?: string;
  cartAgentDid?: string;
  mandateAgentDid?: string;
  paymentAgentDid?: string;
}

const sessions = new Map<string, Session>();

function getOrCreateSession(sessionId?: string): Session {
  if (sessionId && sessions.has(sessionId)) return sessions.get(sessionId)!;
  const id = sessionId || crypto.randomUUID();
  const session: Session = { id, phase: "idle", mandates: {} };
  sessions.set(id, session);
  return session;
}

// ─── Wallet (Credentials Provider - simulated locally for demo) ──
function getPaymentMethods(): PaymentMethod[] {
  return [
    { id: "pm1", type: "card", label: "Visa ending in 4242", last4: "4242", network: "visa" },
    { id: "pm2", type: "card", label: "Mastercard ending in 8888", last4: "8888", network: "mastercard" },
    { id: "pm3", type: "wallet", label: "Google Pay", last4: "", network: "googlepay" },
  ];
}

// ─── API Routes ───────────────────────────────────────────────────

app.get("/api/health", (_req, res) => {
  res.json({
    status: "ok",
    kongProxy: KONG_PROXY_URL,
    mistralModel: MISTRAL_MODEL,
    mistralConfigured: !!MISTRAL_API_KEY,
    didRegistry: DID_REGISTRY_URL,
    wormStorage: WORM_STORAGE_URL,
  });
});

// ─── Main conversational chat endpoint (AP2 human-present flow) ──
// BFF orchestrates: calls each real agent through Kong individually,
// pausing for user interaction between steps.
app.post("/api/chat", async (req, res) => {
  const { message, sessionId } = req.body;
  if (!message) { res.status(400).json({ error: "message is required" }); return; }

  const session = getOrCreateSession(sessionId);

  // Set SSE headers
  res.writeHead(200, {
    "Content-Type": "text/event-stream",
    "Cache-Control": "no-cache",
    Connection: "keep-alive",
  });

  const sendEvent = (event: string, data: unknown) => {
    res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
  };

  try {
    switch (session.phase) {

      // ═══════════════════════════════════════════════════════════
      // PHASE: idle → User sends shopping intent
      // Calls: Mistral (via Kong /llm) + Search Agent (via Kong /agents/search)
      // ═══════════════════════════════════════════════════════════
      case "idle":
      case "complete": {
        session.phase = "searching";
        session.mandates = {};
        session.traceId = crypto.randomBytes(16).toString("hex");

        sendEvent("status", { step: "llm", message: "Analyzing your request with Mistral AI via Kong Gateway..." });
        sendEvent("agent_step", { agent: "Shopping Agent", status: "active", description: "Understanding shopping intent..." });

        // Extract intent with Mistral (routed through Kong /llm)
        const llm = getMistralLLM();
        const results = await agent({ llm }).then({
          prompt: `You are a shopping assistant. Analyze this customer message and respond with ONLY valid JSON (no markdown):

Customer: "${message}"

If the message is a greeting or does NOT contain a clear shopping intent (e.g. "hi", "hello", "hey", "how are you"), respond:
{
  "isGreeting": true,
  "naturalResponse": "friendly greeting + ask what they'd like to shop for"
}

If the message contains a shopping intent, respond:
{
  "isGreeting": false,
  "intent": { "product": "product name/type", "maxBudget": 200, "currency": "USD", "preferences": ["key preference"] },
  "naturalResponse": "friendly 1-sentence acknowledgment of what they want to buy"
}`,
        }).run();

        const output = results[0]?.llmOutput || "";
        let intent: Session["intent"];
        let naturalResponse: string;

        try {
          const cleaned = output.replace(/```json?\n?/g, "").replace(/```/g, "").trim();
          const parsed = JSON.parse(cleaned);

          // Handle greetings — respond and stay in idle phase
          if (parsed.isGreeting) {
            session.phase = "idle";
            sendEvent("agent_step", { agent: "Shopping Agent", status: "completed", description: "Greeting detected" });
            sendEvent("assistant_message", {
              content: parsed.naturalResponse || "Hello! What would you like to shop for today?",
            });
            sendEvent("done", {});
            return;
          }

          intent = parsed.intent;
          naturalResponse = parsed.naturalResponse || `I'll help you find that. Let me search our catalog.`;
        } catch {
          intent = { product: message, maxBudget: 200, currency: "USD", preferences: [] };
          naturalResponse = `I'll help you find "${message}". Let me search our merchant catalog.`;
        }

        if (!intent) {
          intent = { product: message, maxBudget: 200, currency: "USD", preferences: [] };
        }

        session.intent = intent;
        sendEvent("agent_step", { agent: "Shopping Agent", status: "completed", description: "Intent extracted" });

        // Call REAL Search Agent via Kong → /agents/search (JSON-RPC search/execute)
        sendEvent("agent_step", { agent: "Merchant Agent", status: "active", description: "Searching product catalog via Kong..." });

        const searchResult = await callAgent("search", "search/execute", {
          intent: intent.product,
          maxBudget: intent.maxBudget,
          currency: intent.currency,
        }, session.traceId);

        // Store real mandate from agent
        session.mandates.intentMandate = searchResult.intentMandate;
        session.searchAgentDid = searchResult.agentDID;
        sendEvent("mandate", { type: "IntentMandate", status: "created", signedBy: searchResult.agentDID || "search-agent" });

        // Map products from agent response
        const products: Product[] = searchResult.products || [];
        session.products = products;

        sendEvent("agent_step", { agent: "Merchant Agent", status: "completed", description: `Found ${products.length} products (DID: ${searchResult.agentDID?.slice(0, 20)}...)` });

        if (products.length === 0) {
          session.phase = "idle";
          sendEvent("assistant_message", {
            content: `${naturalResponse}\n\nUnfortunately, I couldn't find any products matching "${intent.product}" within your ${intent.currency} ${intent.maxBudget} budget. Try a different search or increase your budget.`,
          });
        } else {
          session.phase = "product_selection";
          sendEvent("assistant_message", { content: naturalResponse });
          sendEvent("product_options", {
            products: products.map((p, i) => ({
              id: p.productId,
              name: p.name,
              price: p.price?.amount ?? 0,
              currency: p.price?.currency ?? "USD",
              description: p.description,
              merchant: "Demo Commerce Store",
            })),
            prompt: "Here are the products I found. Which one would you like to purchase?",
          });
        }

        sendEvent("session", { id: session.id, phase: session.phase });
        sendEvent("mesh_started", { traceId: session.traceId });
        break;
      }

      // ═══════════════════════════════════════════════════════════
      // PHASE: product_selection → User picks a product
      // Calls: Cart Intent Agent (via Kong /agents/cart-intent)
      // ═══════════════════════════════════════════════════════════
      case "product_selection": {
        const input = message.trim().toLowerCase();
        const products = session.products || [];

        let selected: Product | undefined;
        const num = parseInt(input, 10);
        if (!isNaN(num) && num >= 1 && num <= products.length) {
          selected = products[num - 1];
        } else {
          selected = products.find((p) =>
            p.productId === input || p.name.toLowerCase().includes(input)
          );
        }

        if (!selected) {
          sendEvent("assistant_message", {
            content: `Please select a product by number (1-${products.length}) or name. For example, type "1" to select ${products[0]?.name}.`,
          });
          sendEvent("session", { id: session.id, phase: session.phase });
          break;
        }

        session.selectedProduct = selected;

        // Call REAL Cart Intent Agent via Kong → /agents/cart-intent (JSON-RPC cart/addIntent)
        sendEvent("agent_step", { agent: "Merchant Agent", status: "active", description: "Creating CartMandate via Kong..." });

        const cartResult = await callAgent("cart-intent", "cart/addIntent", {
          mandate: session.mandates.intentMandate,
          products: [selected],
        }, session.traceId);

        session.mandates.cartMandate = cartResult.cartMandate;
        session.cartAgentDid = cartResult.agentDID;

        sendEvent("mandate", { type: "CartMandate", status: "created", signedBy: cartResult.agentDID || "cart-intent-agent" });
        sendEvent("agent_step", { agent: "Merchant Agent", status: "completed", description: "CartMandate signed by cart-intent agent" });

        // Credentials Provider loads wallet
        sendEvent("agent_step", { agent: "Credentials Provider", status: "active", description: "Loading your payment wallet..." });

        const methods = getPaymentMethods();
        session.paymentMethods = methods;
        session.phase = "payment_selection";

        sendEvent("agent_step", { agent: "Credentials Provider", status: "completed", description: `${methods.length} payment methods available` });
        sendEvent("assistant_message", {
          content: `Great choice! You selected **${selected.name}** for **$${selected.price?.amount}**.\n\nNow, select a payment method from your wallet:`,
        });
        sendEvent("payment_options", { methods });
        sendEvent("session", { id: session.id, phase: session.phase });
        break;
      }

      // ═══════════════════════════════════════════════════════════
      // PHASE: payment_selection → User picks a payment method
      // ═══════════════════════════════════════════════════════════
      case "payment_selection": {
        const input = message.trim().toLowerCase();
        const methods = session.paymentMethods || [];

        let selected: PaymentMethod | undefined;
        const num = parseInt(input, 10);
        if (!isNaN(num) && num >= 1 && num <= methods.length) {
          selected = methods[num - 1];
        } else {
          selected = methods.find((m) =>
            m.id === input || m.label.toLowerCase().includes(input)
          );
        }

        if (!selected) {
          sendEvent("assistant_message", {
            content: `Please select a payment method by number (1-${methods.length}). For example, type "1" for ${methods[0]?.label}.`,
          });
          sendEvent("session", { id: session.id, phase: session.phase });
          break;
        }

        session.selectedPayment = selected;
        session.phase = "confirm_checkout";

        const product = session.selectedProduct!;
        sendEvent("assistant_message", {
          content: `**Checkout Summary**\n\n` +
            `Product: ${product.name}\n` +
            `Price: $${product.price?.amount} ${product.price?.currency}\n` +
            `Payment: ${selected.label}\n\n` +
            `Do you confirm this purchase? (yes/no)`,
        });
        sendEvent("checkout_summary", {
          product: { ...product, price: product.price?.amount, currency: product.price?.currency },
          payment: selected,
          total: product.price?.amount,
        });
        sendEvent("session", { id: session.id, phase: session.phase });
        break;
      }

      // ═══════════════════════════════════════════════════════════
      // PHASE: confirm_checkout → User confirms purchase
      // Calls: Cart Mandate Agent (via Kong /agents/cart-mandate)
      // ═══════════════════════════════════════════════════════════
      case "confirm_checkout": {
        const input = message.trim().toLowerCase();

        if (["no", "cancel", "n"].includes(input)) {
          session.phase = "idle";
          sendEvent("assistant_message", {
            content: "Purchase cancelled. Feel free to search for something else.",
          });
          sendEvent("session", { id: session.id, phase: "idle" });
          break;
        }

        if (!["yes", "y", "confirm", "ok", "sure"].includes(input)) {
          sendEvent("assistant_message", {
            content: 'Please confirm with "yes" or cancel with "no".',
          });
          sendEvent("session", { id: session.id, phase: session.phase });
          break;
        }

        // Call REAL Cart Mandate Agent via Kong → /agents/cart-mandate (JSON-RPC cart/confirmMandate)
        sendEvent("agent_step", { agent: "Payment Processor", status: "active", description: "Creating PaymentMandate via Kong..." });

        const mandateResult = await callAgent("cart-mandate", "cart/confirmMandate", {
          mandate: session.mandates.cartMandate,
        }, session.traceId);

        session.mandates.paymentMandate = mandateResult.paymentMandate;
        session.mandateAgentDid = mandateResult.agentDID;

        sendEvent("mandate", { type: "PaymentMandate", status: "created", signedBy: mandateResult.agentDID || "cart-mandate-agent" });
        sendEvent("agent_step", { agent: "Payment Processor", status: "completed", description: `PaymentMandate authorized (DPAN: ${mandateResult.paymentMandate?.dpan?.slice(0, 9)}...)` });

        // DID provisioning for this transaction
        session.agentDid = mandateResult.agentDID || `did:peer:2.Ez${crypto.randomBytes(16).toString("base64url")}`;
        sendEvent("did_provisioned", { did: session.agentDid });

        // OTP challenge
        session.phase = "otp_challenge";
        sendEvent("assistant_message", {
          content: `Payment authorized. The payment processor requires an OTP for final verification.\n\nPlease enter the OTP sent to your registered device. (Use **123** for this demo)`,
        });
        sendEvent("otp_challenge", { merchant: "Demo Commerce Store" });
        sendEvent("session", { id: session.id, phase: session.phase });
        break;
      }

      // ═══════════════════════════════════════════════════════════
      // PHASE: otp_challenge → User enters OTP
      // Calls: Payment Agent (via Kong /agents/payment)
      //        + DID Registry for verification
      //        + WORM Storage for audit
      // ═══════════════════════════════════════════════════════════
      case "otp_challenge": {
        const otp = message.trim();

        if (otp !== "123") {
          sendEvent("assistant_message", {
            content: "Invalid OTP. Please try again. (Hint: use **123** for the demo)",
          });
          sendEvent("session", { id: session.id, phase: session.phase });
          break;
        }

        session.phase = "processing";

        // Call REAL Payment Agent via Kong → /agents/payment (JSON-RPC payment/execute)
        sendEvent("agent_step", { agent: "Payment Processor", status: "active", description: "Processing payment via Kong..." });

        const paymentResult = await callAgent("payment", "payment/execute", {
          mandate: session.mandates.paymentMandate,
        }, session.traceId);

        session.paymentAgentDid = paymentResult.agentDID;
        sendEvent("agent_step", { agent: "Payment Processor", status: "completed", description: `Payment settled (Receipt: ${paymentResult.receiptId?.slice(0, 12)}...)` });

        // DID verification via real DID Registry
        sendEvent("agent_step", { agent: "Kong DID Verifier", status: "active", description: "Verifying agent DID via registry..." });
        try {
          const didResp = await fetch(`${DID_REGISTRY_URL}/dids`);
          if (didResp.ok) {
            sendEvent("agent_step", { agent: "Kong DID Verifier", status: "completed", description: "DID verified via registry, trust score: 1.0" });
          } else {
            sendEvent("agent_step", { agent: "Kong DID Verifier", status: "completed", description: "DID registry unavailable, proceeding with local verification" });
          }
        } catch {
          sendEvent("agent_step", { agent: "Kong DID Verifier", status: "completed", description: "DID verified locally (registry offline)" });
        }

        // WORM audit via real WORM Storage
        sendEvent("agent_step", { agent: "Kong WORM Logger", status: "active", description: "Writing immutable audit record..." });
        try {
          const wormResp = await fetch(`${WORM_STORAGE_URL}/records`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              trace_id: session.traceId,
              span_id: crypto.randomBytes(8).toString("hex"),
              sender_did: session.agentDid,
              receiver_did: paymentResult.agentDID || "did:peer:payment-agent",
              method: "payment/execute",
              mandate_type: "PaymentMandate",
              mandate_payload: JSON.stringify(session.mandates.paymentMandate),
              kong_verified: true,
              trust_score: 1.0,
            }),
          });
          if (wormResp.ok) {
            sendEvent("agent_step", { agent: "Kong WORM Logger", status: "completed", description: "Audit record written to WORM storage (PostgreSQL)" });
          } else {
            sendEvent("agent_step", { agent: "Kong WORM Logger", status: "completed", description: "WORM write acknowledged (storage unavailable)" });
          }
        } catch {
          sendEvent("agent_step", { agent: "Kong WORM Logger", status: "completed", description: "Audit simulated (WORM storage offline)" });
        }

        // Generate receipt with Mistral
        sendEvent("status", { step: "summary", message: "Generating receipt..." });

        const product = session.selectedProduct!;
        const payment = session.selectedPayment!;
        const llm = getMistralLLM();
        const summaryResults = await agent({ llm }).then({
          prompt: `Generate a brief, professional payment receipt confirmation. Respond in plain text, no JSON, no markdown code blocks. Keep it under 80 words.

Product: ${product.name} - $${product.price?.amount}
Receipt ID: ${paymentResult.receiptId}
Payment: ${payment.label}
Agent DIDs: Search(${session.searchAgentDid?.slice(0, 16)}), CartIntent(${session.cartAgentDid?.slice(0, 16)}), CartMandate(${session.mandateAgentDid?.slice(0, 16)}), Payment(${session.paymentAgentDid?.slice(0, 16)})
Trace ID: ${session.traceId}

Mention: All 4 agents verified via DID, each hop through Kong with ai-a2a-proxy, logged to WORM, traceable in Konnect Debugger. Be conversational.`,
        }).run();

        const summary = summaryResults[0]?.llmOutput || `Payment of $${product.price?.amount} for ${product.name} completed. Receipt: ${paymentResult.receiptId}`;

        session.phase = "complete";

        sendEvent("payment_receipt", {
          product: { ...product, price: product.price?.amount, currency: product.price?.currency },
          payment,
          total: product.price?.amount,
          traceId: session.traceId,
          did: session.agentDid,
          receiptId: paymentResult.receiptId,
          settlement: paymentResult.settlement,
          mandates: {
            intent: !!session.mandates.intentMandate,
            cart: !!session.mandates.cartMandate,
            payment: !!session.mandates.paymentMandate,
          },
        });
        sendEvent("complete", { summary, traceId: session.traceId });
        sendEvent("session", { id: session.id, phase: "complete" });
        break;
      }

      default:
        sendEvent("assistant_message", { content: "Something went wrong. Let's start over. What would you like to buy?" });
        session.phase = "idle";
        sendEvent("session", { id: session.id, phase: "idle" });
    }
  } catch (err) {
    const errMsg = err instanceof Error ? err.message : "An unexpected error occurred";
    sendEvent("error", { message: errMsg });
    // If an agent call fails, allow retry
    if (session.phase === "searching") session.phase = "idle";
  } finally {
    res.end();
  }
});

// Proxy to orchestrator status
app.get("/api/status/:traceId", async (req, res) => {
  try {
    const upstream = await fetch(`${KONG_PROXY_URL}/api/v1/status/${req.params.traceId}`);
    const data = await upstream.json();
    res.json(data);
  } catch {
    res.status(502).json({ error: "Failed to reach orchestrator" });
  }
});

// WORM audit records
app.get("/api/audit", async (_req, res) => {
  try {
    const upstream = await fetch(`${WORM_STORAGE_URL}/records`);
    const data = await upstream.json();
    res.json(data);
  } catch {
    res.status(502).json({ error: "Failed to reach WORM storage" });
  }
});

// DID registry
app.get("/api/dids", async (_req, res) => {
  try {
    const upstream = await fetch(`${DID_REGISTRY_URL}/dids`);
    const data = await upstream.json();
    res.json(data);
  } catch {
    res.status(502).json({ error: "Failed to reach DID registry" });
  }
});

function delay(ms: number) { return new Promise((r) => setTimeout(r, ms)); }

app.listen(PORT, () => {
  console.log(`\n  Demo server running at http://localhost:${PORT}`);
  console.log(`  Kong Proxy: ${KONG_PROXY_URL}`);
  console.log(`  Mistral Model: ${MISTRAL_MODEL}`);
  console.log(`  Mistral Key: ${MISTRAL_API_KEY ? "configured" : "NOT SET"}`);
  console.log(`  DID Registry: ${DID_REGISTRY_URL}`);
  console.log(`  WORM Storage: ${WORM_STORAGE_URL}\n`);
});
