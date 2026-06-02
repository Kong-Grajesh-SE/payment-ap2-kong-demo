import { useState, useRef, useEffect } from "react";
import { Send, Bot, User, Loader2, Sparkles } from "lucide-react";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: Date;
  intent?: Record<string, unknown>;
  choices?: Array<{ label: string; value: string }>;
}

interface ChatPanelProps {
  messages: ChatMessage[];
  onSend: (message: string) => void;
  isLoading: boolean;
  statusMessage: string;
}

const SUGGESTIONS = [
  "I want to buy Nike running shoes under $200",
  "Find me premium headphones within $150",
  "Search for a leather wallet under $80",
];

export default function ChatPanel({
  messages,
  onSend,
  isLoading,
  statusMessage,
}: ChatPanelProps) {
  const [input, setInput] = useState("");
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, statusMessage]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isLoading) return;
    onSend(input.trim());
    setInput("");
  };

  const handleChoice = (value: string) => {
    if (isLoading) return;
    onSend(value);
  };

  // Render markdown-like bold text
  function renderContent(text: string) {
    const parts = text.split(/(\*\*[^*]+\*\*)/g);
    return parts.map((part, i) => {
      if (part.startsWith("**") && part.endsWith("**")) {
        return <strong key={i} className="font-bold text-fg-0">{part.slice(2, -2)}</strong>;
      }
      if (part.startsWith("_") && part.endsWith("_")) {
        return <em key={i} className="text-fg-3 text-xs">{part.slice(1, -1)}</em>;
      }
      return <span key={i}>{part}</span>;
    });
  }

  return (
    <div className="flex flex-col h-full">
      {/* Messages Area */}
      <div className="flex-1 overflow-y-auto custom-scrollbar px-4 py-6 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center px-4">
            <div className="w-16 h-16 rounded-xs flex items-center justify-center mb-4 relative"
                 style={{ background: "rgba(204,255,0,0.08)", border: "1px solid rgba(204,255,0,0.2)" }}>
              <Sparkles className="w-8 h-8 text-lime" />
              <div className="absolute inset-0 rounded-xs" style={{ boxShadow: "0 0 40px rgba(204,255,0,0.12)" }} />
            </div>
            <h2 className="text-lg font-bold text-fg-0 mb-1">
              Autonomous Commerce Demo
            </h2>
            <p className="text-sm text-fg-3 mb-2 max-w-md">
              Tell me what you'd like to buy. I'll search merchant catalogs,
              let you pick a product and payment method, then complete a
              DID-verified payment - following the AP2 human-present flow.
            </p>
            <p className="text-xs text-fg-3 mb-6 max-w-sm font-mono" style={{ letterSpacing: "0.02em" }}>
              IntentMandate / CartMandate / PaymentMandate / OTP
            </p>
            <div className="flex flex-wrap gap-2 justify-center">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => onSend(s)}
                  className="btn-secondary text-xs px-3 py-1.5"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((msg) => (
          <div key={msg.id} className="animate-slide-up">
            <div className={`flex gap-3 ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
              {msg.role !== "user" && (
                <div className="w-8 h-8 rounded-xs flex items-center justify-center flex-shrink-0 mt-1"
                     style={{ background: "rgba(204,255,0,0.08)", border: "1px solid rgba(204,255,0,0.15)" }}>
                  <Bot className="w-4 h-4 text-lime" />
                </div>
              )}
              <div className={`max-w-[80%] ${msg.role === "user" ? "chat-bubble-user" : "chat-bubble-ai"}`}>
                <p className="text-sm leading-relaxed whitespace-pre-wrap">{renderContent(msg.content)}</p>
                <span className={`text-[10px] mt-1 block ${msg.role === "user" ? "text-black/50" : "text-fg-3"}`}>
                  {msg.timestamp.toLocaleTimeString()}
                </span>
              </div>
              {msg.role === "user" && (
                <div className="w-8 h-8 rounded-xs flex items-center justify-center flex-shrink-0 mt-1"
                     style={{ background: "var(--bg-300)" }}>
                  <User className="w-4 h-4 text-fg-2" />
                </div>
              )}
            </div>

            {/* Choice buttons */}
            {msg.choices && !isLoading && (
              <div className="flex flex-wrap gap-2 mt-2 ml-11">
                {msg.choices.map((c) => (
                  <button
                    key={c.value}
                    onClick={() => handleChoice(c.value)}
                    className="btn-secondary text-xs px-3 py-1.5 hover:border-lime hover:text-lime"
                    style={{ transitionDuration: "var(--dur-xs)" }}
                  >
                    {c.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}

        {isLoading && statusMessage && (
          <div className="flex gap-3 animate-fade-in">
            <div className="w-8 h-8 rounded-xs flex items-center justify-center flex-shrink-0"
                 style={{ background: "rgba(204,255,0,0.08)" }}>
              <Loader2 className="w-4 h-4 text-lime animate-spin" />
            </div>
            <div className="chat-bubble-ai">
              <p className="text-sm text-fg-2 flex items-center gap-2">
                <span className="inline-block w-1.5 h-1.5 bg-lime rounded-full animate-pulse" />
                {statusMessage}
              </p>
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Area */}
      <div className="border-t p-4" style={{ borderColor: "var(--border-subtle)", background: "var(--bg-100)" }}>
        <form onSubmit={handleSubmit} className="flex gap-2">
          <input
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="What would you like to buy?"
            disabled={isLoading}
            className="flex-1 rounded-pill px-4 py-2.5 text-sm
                       placeholder:text-fg-3 focus:outline-none
                       transition-all disabled:opacity-50"
            style={{
              background: "var(--bg-200)",
              color: "var(--fg-0)",
              border: "1px solid var(--border-default)",
            }}
            onFocus={(e) => { e.currentTarget.style.borderColor = "rgba(204,255,0,0.5)"; e.currentTarget.style.boxShadow = "0 0 0 3px rgba(204,255,0,0.1)"; }}
            onBlur={(e) => { e.currentTarget.style.borderColor = "var(--border-default)"; e.currentTarget.style.boxShadow = "none"; }}
          />
          <button
            type="submit"
            disabled={isLoading || !input.trim()}
            className="btn-primary px-4 py-2.5 flex items-center gap-2 text-sm font-bold
                       disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Send className="w-4 h-4" />
          </button>
        </form>
      </div>
    </div>
  );
}
