import { useState, useCallback, useEffect, useRef } from "react";
import Header from "./components/Header";
import ChatPanel, { type ChatMessage } from "./components/ChatPanel";
import AgentTimeline, { type AgentStep } from "./components/AgentTimeline";

function generateId() {
  return Math.random().toString(36).slice(2, 10);
}

export default function App() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [agentSteps, setAgentSteps] = useState<AgentStep[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [statusMessage, setStatusMessage] = useState("");
  const [traceId, setTraceId] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const sessionIdRef = useRef<string | null>(null);

  useEffect(() => {
    fetch("/api/health")
      .then((r) => r.json())
      .then(() => setConnected(true))
      .catch(() => setConnected(false));
  }, []);

  const handleSend = useCallback(
    async (message: string) => {
      if (isLoading) return;

      const userMsg: ChatMessage = {
        id: generateId(),
        role: "user",
        content: message,
        timestamp: new Date(),
      };
      setMessages((prev) => [...prev, userMsg]);
      setIsLoading(true);

      try {
        const res = await fetch("/api/chat", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            message,
            sessionId: sessionIdRef.current,
          }),
        });

        if (!res.ok || !res.body) throw new Error(`Server error: ${res.status}`);

        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";

          for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            if (line.startsWith("event: ")) {
              const event = line.slice(7);
              const dataLine = lines[i + 1];
              if (dataLine?.startsWith("data: ")) {
                try {
                  const data = JSON.parse(dataLine.slice(6));
                  handleSSEEvent(event, data);
                } catch { /* skip malformed */ }
                i++; // skip the data line
              }
            }
          }
        }
      } catch (err) {
        setMessages((prev) => [
          ...prev,
          {
            id: generateId(),
            role: "assistant",
            content: `Sorry, something went wrong: ${err instanceof Error ? err.message : "Unknown error"}. Make sure the demo server is running.`,
            timestamp: new Date(),
          },
        ]);
      } finally {
        setIsLoading(false);
        setStatusMessage("");
      }
    },
    [isLoading]
  );

  function handleSSEEvent(event: string, data: Record<string, unknown>) {
    switch (event) {
      case "session":
        sessionIdRef.current = data.id as string;
        break;

      case "status":
        setStatusMessage(data.message as string);
        break;

      case "assistant_message": {
        setMessages((prev) => [
          ...prev,
          { id: generateId(), role: "assistant", content: data.content as string, timestamp: new Date() },
        ]);
        break;
      }

      case "product_options": {
        const products = data.products as Array<{ id: string; name: string; price: number; currency: string; merchant: string; description: string }>;
        const content = `${data.prompt}\n\n` +
          products.map((p, i) => `**${i + 1}.** ${p.name} — $${p.price}\n   _${p.description}_ (${p.merchant})`).join("\n\n");
        setMessages((prev) => [
          ...prev,
          { id: generateId(), role: "assistant", content, timestamp: new Date(), choices: products.map((p, i) => ({ label: `${i + 1}. ${p.name} — $${p.price}`, value: String(i + 1) })) },
        ]);
        break;
      }

      case "payment_options": {
        const methods = data.methods as Array<{ id: string; label: string }>;
        const content = methods.map((m, i) => `**${i + 1}.** ${m.label}`).join("\n");
        setMessages((prev) => [
          ...prev,
          { id: generateId(), role: "assistant", content: `Select a payment method:\n\n${content}`, timestamp: new Date(), choices: methods.map((m, i) => ({ label: `${i + 1}. ${m.label}`, value: String(i + 1) })) },
        ]);
        break;
      }

      case "checkout_summary": {
        const p = data.product as { name: string; price: number; currency: string; merchant: string };
        const pm = data.payment as { label: string };
        setMessages((prev) => {
          // The assistant_message already added the text; add a structured receipt card
          const last = prev[prev.length - 1];
          if (last?.role === "assistant") {
            return [
              ...prev.slice(0, -1),
              { ...last, choices: [{ label: "Yes, confirm purchase", value: "yes" }, { label: "Cancel", value: "no" }] },
            ];
          }
          return prev;
        });
        break;
      }

      case "otp_challenge": {
        // The assistant_message with OTP prompt is already added
        break;
      }

      case "mandate": {
        const type = data.type as string;
        const signedBy = data.signedBy as string;
        setAgentSteps((prev) => [
          ...prev,
          { id: generateId(), agent: `${type}`, description: `Signed by ${signedBy}`, status: "completed" as const, timestamp: new Date() },
        ]);
        break;
      }

      case "agent_step": {
        const agentName = data.agent as string;
        const status = data.status as "active" | "completed";
        const description = data.description as string;

        setAgentSteps((prev) => {
          const existing = prev.find((s) => s.agent === agentName && s.status === "active");
          if (existing) {
            return prev.map((s) =>
              s.id === existing.id ? { ...s, status, description, timestamp: status === "completed" ? new Date() : s.timestamp } : s
            );
          }
          return [...prev, { id: generateId(), agent: agentName, description, status, timestamp: status === "completed" ? new Date() : undefined }];
        });
        break;
      }

      case "did_provisioned":
        // Could show DID in timeline
        break;

      case "mesh_started":
        setTraceId(data.traceId as string);
        break;

      case "payment_receipt":
        // Receipt data available; summary comes in "complete" event
        break;

      case "complete": {
        setMessages((prev) => [
          ...prev,
          { id: generateId(), role: "assistant", content: data.summary as string, timestamp: new Date() },
        ]);
        // Reset for next transaction
        sessionIdRef.current = null;
        setAgentSteps([]);
        break;
      }

      case "error": {
        setMessages((prev) => [
          ...prev,
          { id: generateId(), role: "assistant", content: `Error: ${data.message}`, timestamp: new Date() },
        ]);
        break;
      }
    }
  }

  return (
    <div className="h-screen flex flex-col" style={{ background: "var(--bg-0)", color: "var(--fg-0)" }}>
      <Header connected={connected} />

      <main className="flex-1 flex overflow-hidden relative blueprint">
        <div className="flex-1 flex flex-col min-w-0 relative z-10">
          <ChatPanel
            messages={messages}
            onSend={handleSend}
            isLoading={isLoading}
            statusMessage={statusMessage}
          />
        </div>

        <div className="w-80 border-l hidden lg:flex flex-col relative z-10"
             style={{ borderColor: "var(--border-subtle)", background: "var(--bg-100)" }}>
          <AgentTimeline steps={agentSteps} traceId={traceId} />
        </div>
      </main>

      <footer className="border-t px-6 py-2 flex items-center justify-between"
              style={{ borderColor: "var(--border-subtle)", background: "var(--bg-100)", fontSize: "11px", color: "var(--fg-3)" }}>
        <span>
          Powered by Kong Enterprise 3.14 / Volcano Agent SDK / Mistral AI
        </span>
        <span className="flex items-center gap-3 font-mono uppercase" style={{ fontSize: "10px", letterSpacing: "0.06em" }}>
          <span>AP2 Protocol</span>
          <span style={{ color: "var(--accent)" }}>/</span>
          <span>DID:peer</span>
          <span style={{ color: "var(--accent)" }}>/</span>
          <span>WORM Audit</span>
          <span style={{ color: "var(--accent)" }}>/</span>
          <span>OpenTelemetry</span>
        </span>
      </footer>
    </div>
  );
}
