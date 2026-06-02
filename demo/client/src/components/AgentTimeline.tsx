import {
  Search,
  ShoppingCart,
  FileCheck,
  CreditCard,
  CheckCircle2,
  Clock,
  ExternalLink,
  Shield,
  Database,
  Fingerprint,
  Wallet,
  ScrollText,
  Cpu,
} from "lucide-react";

export interface AgentStep {
  id: string;
  agent: string;
  description: string;
  status: "pending" | "active" | "completed";
  timestamp?: Date;
}

interface AgentTimelineProps {
  steps: AgentStep[];
  traceId: string | null;
}

const AGENT_ICONS: Record<string, typeof Search> = {
  "Shopping Agent": ShoppingCart,
  "Merchant Agent": Search,
  "Credentials Provider": Wallet,
  "Payment Processor": CreditCard,
  "Kong DID Verifier": Shield,
  "Kong WORM Logger": Database,
  "IntentMandate": ScrollText,
  "CartMandate": FileCheck,
  "PaymentMandate": ScrollText,
};

export default function AgentTimeline({ steps, traceId }: AgentTimelineProps) {
  const isMandate = (name: string) => name.endsWith("Mandate");

  return (
    <div className="h-full flex flex-col">
      {/* Panel Header */}
      <div className="px-4 py-3 border-b" style={{ borderColor: "var(--border-subtle)", background: "var(--bg-100)" }}>
        <h3 className="text-sm font-bold text-fg-0 flex items-center gap-2">
          <Cpu className="w-4 h-4 text-lime" />
          AP2 Protocol Flow
        </h3>
        <p className="text-xs text-fg-3 mt-0.5 tracking-wide uppercase" style={{ fontSize: "10px", letterSpacing: "0.08em" }}>
          Agents + Mandates + Kong Plugins
        </p>
      </div>

      {/* Timeline */}
      <div className="flex-1 overflow-y-auto custom-scrollbar px-4 py-4" style={{ background: "var(--bg-0)" }}>
        {steps.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <Clock className="w-8 h-8 text-fg-3 mb-2" />
            <p className="text-xs text-fg-3">
              Start a purchase to see the AP2 flow
            </p>
          </div>
        ) : (
          <div className="space-y-1">
            {steps.map((step, i) => {
              const Icon = AGENT_ICONS[step.agent] || CheckCircle2;
              const isLast = i === steps.length - 1;
              const mandate = isMandate(step.agent);

              return (
                <div key={step.id} className="relative animate-slide-up">
                  {!isLast && (
                    <div
                      className="absolute left-4 top-10 w-0.5 h-6"
                      style={{
                        background: step.status === "completed"
                          ? "rgba(204,255,0,0.3)"
                          : "var(--border-subtle)",
                      }}
                    />
                  )}

                  <div
                    className="flex items-start gap-3 p-2.5 rounded-xs transition-all"
                    style={{
                      background: mandate
                        ? "rgba(204,255,0,0.04)"
                        : step.status === "active"
                          ? "rgba(204,255,0,0.06)"
                          : step.status === "completed"
                            ? "var(--bg-200)"
                            : "transparent",
                      border: mandate
                        ? "1px dashed rgba(204,255,0,0.25)"
                        : step.status === "active"
                          ? "1px solid rgba(204,255,0,0.2)"
                          : "1px solid transparent",
                      opacity: step.status === "pending" ? 0.4 : 1,
                    }}
                  >
                    <div
                      className="w-8 h-8 rounded-xs flex items-center justify-center flex-shrink-0"
                      style={{
                        background: mandate
                          ? "rgba(204,255,0,0.15)"
                          : step.status === "completed"
                            ? "rgba(204,255,0,0.1)"
                            : "var(--bg-300)",
                        border: step.status === "completed"
                          ? "1px solid rgba(204,255,0,0.25)"
                          : "1px solid var(--border-subtle)",
                      }}
                    >
                      {step.status === "active" ? (
                        <div className="w-3 h-3 border-2 border-lime border-t-transparent rounded-full animate-spin" />
                      ) : (
                        <Icon className={`w-4 h-4 ${step.status === "completed" ? "text-lime" : "text-fg-3"}`} />
                      )}
                    </div>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        {mandate && (
                          <span className="text-lime font-mono uppercase" style={{ fontSize: "9px", letterSpacing: "0.06em" }}>MANDATE</span>
                        )}
                        <span className="text-xs font-bold text-fg-0 truncate">
                          {step.agent}
                        </span>
                        {step.status === "completed" && (
                          <CheckCircle2 className="w-3 h-3 text-lime flex-shrink-0" />
                        )}
                      </div>
                      <p className="text-[11px] text-fg-2 mt-0.5">
                        {step.description}
                      </p>
                      {step.timestamp && (
                        <span className="text-[10px] text-fg-3 font-mono">
                          {step.timestamp.toLocaleTimeString()}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Kong Features Footer */}
      <div className="border-t p-4 space-y-2" style={{ borderColor: "var(--border-subtle)", background: "var(--bg-100)" }}>
        <div className="grid grid-cols-3 gap-2">
          {[
            { icon: Fingerprint, label: "DID Signed" },
            { icon: Database, label: "WORM Audit" },
            { icon: Shield, label: "Kong Verified" },
          ].map(({ icon: FeatureIcon, label }) => (
            <div key={label} className="kong-card flex flex-col items-center gap-1 p-2">
              <FeatureIcon className="w-3.5 h-3.5 text-lime" />
              <span className="text-fg-3 font-bold uppercase"
                    style={{ fontSize: "9px", letterSpacing: "0.06em" }}>
                {label}
              </span>
            </div>
          ))}
        </div>

        {traceId && (
          <a
            href={`http://localhost:16686/trace/${traceId}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center justify-center gap-1.5 text-xs text-lime
                       hover:text-lime-hover transition-colors font-bold py-1.5"
            style={{ transitionDuration: "var(--dur-xs)" }}
          >
            View trace in Jaeger
            <ExternalLink className="w-3 h-3" />
          </a>
        )}
      </div>
    </div>
  );
}
