import { Shield, Activity, ExternalLink } from "lucide-react";

interface HeaderProps {
  connected: boolean;
}

export default function Header({ connected }: HeaderProps) {
  return (
    <header className="relative border-b border-border-subtle" style={{ background: "var(--bg-100)" }}>
      <div className="max-w-7xl mx-auto px-6 py-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-3">
              {/* Logomark with lime glow */}
              <div className="w-10 h-10 rounded-xs flex items-center justify-center relative"
                   style={{ background: "rgba(204,255,0,0.1)", border: "1px solid rgba(204,255,0,0.25)" }}>
                <Shield className="w-5 h-5 text-lime" />
                <div className="absolute inset-0 rounded-xs" style={{ boxShadow: "0 0 24px rgba(204,255,0,0.15)" }} />
              </div>
              <div>
                <h1 className="text-lg font-bold tracking-tight text-fg-0">
                  Autonomous Commerce
                </h1>
                <p className="text-xs font-medium text-fg-3">
                  Kong Enterprise + DID + A2A Protocol
                </p>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <div className="hidden md:flex items-center gap-6 text-sm">
              <a
                href="http://localhost:16686"
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 text-fg-2 hover:text-lime transition-colors"
                style={{ transitionDuration: "var(--dur-xs)", transitionTimingFunction: "var(--ease-out-swift)" }}
              >
                <Activity className="w-3.5 h-3.5" />
                Jaeger Traces
                <ExternalLink className="w-3 h-3" />
              </a>
            </div>

            <div className="flex items-center gap-2 px-3 py-1.5 rounded-pill"
                 style={{ border: "1px solid var(--border-default)", background: "var(--bg-200)" }}>
              <div
                className={`w-2 h-2 rounded-full ${connected ? "bg-lime animate-pulse-slow" : "bg-red-500"}`}
              />
              <span className="text-xs font-medium text-fg-1">
                {connected ? "Kong Gateway" : "Disconnected"}
              </span>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}
