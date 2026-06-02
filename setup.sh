#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════
# AP2 Payment Demo — Automated Setup (Phase 1)
# ═══════════════════════════════════════════════════════════════════
#
# Usage:
#   ./setup.sh              # Interactive — prompts for missing values
#   ./setup.sh --check      # Check prerequisites only
#   ./setup.sh --skip-env   # Skip .env setup (already configured)
#
# What this script does:
#   1. Checks prerequisites (Docker, decK, Node.js)
#   2. Creates .env from .env.example (prompts for secrets)
#   3. Starts agent services (docker compose up)
#   4. Waits for services to be healthy
#   5. Syncs Kong config via decK (baseline + agent mesh)
#   6. Verifies agents are reachable through Kong
#   7. Installs demo app dependencies
# ═══════════════════════════════════════════════════════════════════

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# ─── Flags ────────────────────────────────────────────────────────
SKIP_ENV=false
CHECK_ONLY=false

for arg in "$@"; do
  case "$arg" in
    --skip-env)  SKIP_ENV=true ;;
    --check)     CHECK_ONLY=true ;;
    --help|-h)
      echo "Usage: ./setup.sh [--check] [--skip-env]"
      echo "  --check     Check prerequisites only"
      echo "  --skip-env  Skip .env setup (already configured)"
      exit 0
      ;;
  esac
done

# ─── Helpers ──────────────────────────────────────────────────────
info()    { echo -e "${BLUE}ℹ${NC}  $1"; }
success() { echo -e "${GREEN}✓${NC}  $1"; }
warn()    { echo -e "${YELLOW}⚠${NC}  $1"; }
fail()    { echo -e "${RED}✗${NC}  $1"; }
header()  { echo -e "\n${BLUE}═══ $1 ═══${NC}\n"; }

check_cmd() {
  if command -v "$1" &>/dev/null; then
    success "$1 found: $(command -v "$1")"
    return 0
  else
    fail "$1 not found"
    return 1
  fi
}

wait_for_url() {
  local url="$1" name="$2" max_attempts="${3:-30}"
  local attempt=0
  while [ $attempt -lt $max_attempts ]; do
    if curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null | grep -q "200\|404"; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 2
  done
  return 1
}

# ═══════════════════════════════════════════════════════════════════
# STEP 1: Check Prerequisites
# ═══════════════════════════════════════════════════════════════════
header "Step 1: Checking Prerequisites"

MISSING=0

check_cmd docker   || MISSING=$((MISSING + 1))
check_cmd deck     || MISSING=$((MISSING + 1))
check_cmd node     || MISSING=$((MISSING + 1))
check_cmd npm      || MISSING=$((MISSING + 1))
check_cmd curl     || MISSING=$((MISSING + 1))
check_cmd jq       || MISSING=$((MISSING + 1))

# Check Docker is running
if docker info &>/dev/null; then
  success "Docker daemon is running"
else
  fail "Docker daemon is not running — start Docker Desktop"
  MISSING=$((MISSING + 1))
fi

# Check versions
if command -v node &>/dev/null; then
  NODE_VERSION=$(node -v | sed 's/v//' | cut -d. -f1)
  if [ "$NODE_VERSION" -ge 20 ]; then
    success "Node.js version $(node -v) (≥ 20 required)"
  else
    warn "Node.js $(node -v) — version 20+ recommended"
  fi
fi

if command -v deck &>/dev/null; then
  success "decK version $(deck version 2>&1 | head -1 | awk '{print $NF}')"
fi

if [ $MISSING -gt 0 ]; then
  echo ""
  fail "$MISSING prerequisite(s) missing. Install them and re-run."
  echo ""
  echo "  brew install kong/deck/deck   # decK CLI"
  echo "  brew install node             # Node.js"
  echo "  brew install jq               # JSON processor"
  exit 1
fi

success "All prerequisites satisfied"

if $CHECK_ONLY; then
  echo ""
  success "Prerequisite check complete."
  exit 0
fi

# ═══════════════════════════════════════════════════════════════════
# STEP 2: Environment Configuration
# ═══════════════════════════════════════════════════════════════════
header "Step 2: Environment Configuration"

if $SKIP_ENV; then
  if [ -f .env ]; then
    success ".env exists — skipping (--skip-env)"
  else
    fail ".env not found. Run without --skip-env to create it."
    exit 1
  fi
elif [ -f .env ]; then
  warn ".env already exists"
  read -rp "  Overwrite? (y/N): " overwrite
  if [[ "$overwrite" =~ ^[Yy] ]]; then
    cp .env.example .env
    info "Copied .env.example → .env"
  else
    info "Keeping existing .env"
  fi
else
  cp .env.example .env
  info "Created .env from .env.example"
fi

# Source current .env
set -a
source .env 2>/dev/null || true
set +a

# Prompt for missing required values
prompt_if_empty() {
  local var_name="$1" prompt_msg="$2" current_val="${!1:-}"
  if [ -z "$current_val" ] || [[ "$current_val" == "<"* ]]; then
    read -rp "  $prompt_msg: " new_val
    if [ -n "$new_val" ]; then
      if grep -q "^${var_name}=" .env 2>/dev/null; then
        sed -i '' "s|^${var_name}=.*|${var_name}=${new_val}|" .env
      else
        echo "${var_name}=${new_val}" >> .env
      fi
      export "$var_name=$new_val"
    fi
  else
    success "$var_name is set"
  fi
}

echo ""
info "Checking required environment variables..."
echo ""

prompt_if_empty "KONNECT_API_TOKEN" "Konnect API Token (kpat_...)"
prompt_if_empty "KONNECT_CONTROL_PLANE_NAME" "Konnect Control Plane name"
prompt_if_empty "MISTRAL_API_KEY" "Mistral API Key"

# Re-source after updates
set -a
source .env 2>/dev/null || true
set +a

success "Environment configured"

# ═══════════════════════════════════════════════════════════════════
# STEP 3: Start Agent Services
# ═══════════════════════════════════════════════════════════════════
header "Step 3: Starting Agent Services (Docker Compose)"

info "Building and starting services..."
docker compose up -d --build

echo ""
info "Waiting for services to be healthy..."

SERVICES=("did-registry:8070" "worm-storage:8090" "search-agent:9001" "cart-intent-agent:9002" "cart-mandate-agent:9003" "payment-agent:9004")

ALL_HEALTHY=true
for svc_port in "${SERVICES[@]}"; do
  svc="${svc_port%%:*}"
  port="${svc_port##*:}"
  if wait_for_url "http://localhost:${port}/health" "$svc" 30; then
    success "$svc is healthy (port $port)"
  else
    fail "$svc did not become healthy on port $port"
    ALL_HEALTHY=false
  fi
done

if ! $ALL_HEALTHY; then
  warn "Some services failed to start. Check: docker compose logs"
fi

# ═══════════════════════════════════════════════════════════════════
# STEP 4: Check Kong DP
# ═══════════════════════════════════════════════════════════════════
header "Step 4: Checking Kong Data Plane"

KONG_URL="${KONG_PROXY_URL:-http://localhost:8000}"

if wait_for_url "$KONG_URL" "Kong DP" 5; then
  success "Kong DP is reachable at $KONG_URL"
else
  warn "Kong DP not reachable at $KONG_URL"
  echo ""
  echo "  If you haven't started your Kong DP yet:"
  echo "  1. Go to Konnect → Gateway Manager → Data Plane Nodes → New Data Plane Node"
  echo "  2. Use the Docker command Konnect provides, adding:"
  echo "     -e KONG_CLUSTER_RPC=on"
  echo "     -e KONG_TRACING_INSTRUMENTATIONS=all"
  echo "     -e KONG_TRACING_SAMPLING_RATE=1.0"
  echo "     -p 8000:8000 -p 8443:8443"
  echo ""
  read -rp "  Press Enter once your Kong DP is running (or Ctrl+C to exit)..."

  if ! wait_for_url "$KONG_URL" "Kong DP" 10; then
    fail "Kong DP still not reachable. Fix and re-run."
    exit 1
  fi
  success "Kong DP is now reachable"
fi

# ═══════════════════════════════════════════════════════════════════
# STEP 5: Sync Kong Configuration via decK
# ═══════════════════════════════════════════════════════════════════
header "Step 5: Syncing Kong Configuration"

CP_NAME="${KONNECT_CONTROL_PLANE_NAME:-}"
TOKEN="${KONNECT_API_TOKEN:-}"

if [ -z "$CP_NAME" ] || [ -z "$TOKEN" ]; then
  fail "KONNECT_CONTROL_PLANE_NAME and KONNECT_API_TOKEN must be set in .env"
  exit 1
fi

# Phase 0: Baseline (LLM route + OTel)
info "Syncing baseline config (LLM route + OpenTelemetry)..."
deck gateway sync \
  --konnect-token "$TOKEN" \
  --konnect-control-plane-name "$CP_NAME" \
  config/baseline.yml

success "Baseline synced"

# Phase 1: Agent mesh (additive with --select-tag)
info "Syncing agent mesh config (ai-a2a-proxy + 4 agent routes)..."
deck gateway sync \
  --konnect-token "$TOKEN" \
  --konnect-control-plane-name "$CP_NAME" \
  --select-tag ap2-agents \
  config/kong.deck.clean.yml

success "Agent mesh synced (--select-tag ap2-agents)"

# ═══════════════════════════════════════════════════════════════════
# STEP 6: Verify Agent Routes Through Kong
# ═══════════════════════════════════════════════════════════════════
header "Step 6: Verifying Agent Routes Through Kong"

AGENTS=("search" "cart-intent" "cart-mandate" "payment")
ALL_OK=true

for agent_name in "${AGENTS[@]}"; do
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$KONG_URL/agents/$agent_name" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"health/check","params":{},"id":"setup-verify"}' 2>/dev/null || echo "000")

  if [ "$HTTP_CODE" = "200" ]; then
    success "$agent_name agent reachable via Kong ($KONG_URL/agents/$agent_name)"
  else
    fail "$agent_name agent returned HTTP $HTTP_CODE via Kong"
    ALL_OK=false
  fi
done

if $ALL_OK; then
  success "All agents are reachable through Kong"
else
  warn "Some agents are not reachable through Kong. Check 'docker compose logs' and Kong DP logs."
fi

# ═══════════════════════════════════════════════════════════════════
# STEP 7: Install Demo App Dependencies
# ═══════════════════════════════════════════════════════════════════
header "Step 7: Installing Demo App Dependencies"

info "Installing BFF server dependencies..."
(cd demo/server && npm install --silent)
success "Server dependencies installed"

info "Installing React client dependencies..."
(cd demo/client && npm install --silent)
success "Client dependencies installed"

# ═══════════════════════════════════════════════════════════════════
# Done
# ═══════════════════════════════════════════════════════════════════
header "Setup Complete"

echo -e "  ${GREEN}Everything is ready.${NC} Start the demo:\n"
echo "    # Terminal 1: BFF server"
echo "    cd demo/server && npm run dev"
echo ""
echo "    # Terminal 2: React client"
echo "    cd demo/client && npm run dev"
echo ""
echo "    # Then open http://localhost:5173"
echo ""
echo "  Observability:"
echo "    Jaeger UI:        http://localhost:16686"
echo "    Konnect Debugger: https://cloud.konghq.com (Gateway Manager → Debugger)"
echo ""
