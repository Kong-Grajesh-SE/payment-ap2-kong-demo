#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════
# AP2 Payment Demo — Cleanup (Phase 1)
# ═══════════════════════════════════════════════════════════════════
#
# Usage:
#   ./cleanup.sh              # Stop services + remove Kong agent routes
#   ./cleanup.sh --all        # Full teardown (volumes, node_modules, .env)
#   ./cleanup.sh --kong-only  # Remove Kong config only (keep Docker services)
#   ./cleanup.sh --docker-only  # Stop Docker only (keep Kong config)
#
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
CLEAN_ALL=false
KONG_ONLY=false
DOCKER_ONLY=false

for arg in "$@"; do
  case "$arg" in
    --all)          CLEAN_ALL=true ;;
    --kong-only)    KONG_ONLY=true ;;
    --docker-only)  DOCKER_ONLY=true ;;
    --help|-h)
      echo "Usage: ./cleanup.sh [--all] [--kong-only] [--docker-only]"
      echo "  --all          Full teardown: Docker + volumes + Kong + node_modules + .env"
      echo "  --kong-only    Remove Kong config only (keep Docker services running)"
      echo "  --docker-only  Stop Docker services only (keep Kong config)"
      exit 0
      ;;
  esac
done

info()    { echo -e "${BLUE}ℹ${NC}  $1"; }
success() { echo -e "${GREEN}✓${NC}  $1"; }
warn()    { echo -e "${YELLOW}⚠${NC}  $1"; }
fail()    { echo -e "${RED}✗${NC}  $1"; }
header()  { echo -e "\n${BLUE}═══ $1 ═══${NC}\n"; }

# Source .env for Konnect credentials
if [ -f .env ]; then
  set -a
  source .env 2>/dev/null || true
  set +a
fi

# ═══════════════════════════════════════════════════════════════════
# Kong Config Cleanup
# ═══════════════════════════════════════════════════════════════════
cleanup_kong() {
  header "Removing Kong Configuration"

  TOKEN="${KONNECT_API_TOKEN:-}"
  CP_NAME="${KONNECT_CONTROL_PLANE_NAME:-}"

  if [ -z "$TOKEN" ] || [ -z "$CP_NAME" ]; then
    warn "KONNECT_API_TOKEN or KONNECT_CONTROL_PLANE_NAME not set — skipping Kong cleanup"
    echo "  To clean Kong manually:"
    echo "    deck gateway reset --konnect-token <token> --konnect-control-plane-name <name> --select-tag ap2-agents --force"
    return
  fi

  # Remove agent mesh routes (tagged entities only)
  info "Removing agent mesh entities (--select-tag ap2-agents)..."
  if deck gateway reset \
    --konnect-token "$TOKEN" \
    --konnect-control-plane-name "$CP_NAME" \
    --select-tag ap2-agents \
    --force 2>/dev/null; then
    success "Agent mesh entities removed"
  else
    warn "Failed to remove agent mesh entities (may already be clean)"
  fi

  if $CLEAN_ALL; then
    # Remove baseline config too
    info "Removing baseline config (LLM route + OTel)..."
    read -rp "  This will remove ALL Kong config on this CP. Continue? (y/N): " confirm
    if [[ "$confirm" =~ ^[Yy] ]]; then
      if deck gateway reset \
        --konnect-token "$TOKEN" \
        --konnect-control-plane-name "$CP_NAME" \
        --force 2>/dev/null; then
        success "All Kong config removed"
      else
        warn "Failed to reset full Kong config"
      fi
    else
      info "Skipped full Kong reset"
    fi
  fi
}

# ═══════════════════════════════════════════════════════════════════
# Docker Cleanup
# ═══════════════════════════════════════════════════════════════════
cleanup_docker() {
  header "Stopping Docker Services"

  if ! docker info &>/dev/null; then
    warn "Docker daemon not running — skipping"
    return
  fi

  info "Stopping containers..."
  docker compose down

  if $CLEAN_ALL; then
    info "Removing volumes (database data)..."
    docker compose down -v
    success "Containers and volumes removed"
  else
    success "Containers stopped (volumes preserved)"
  fi
}

# ═══════════════════════════════════════════════════════════════════
# Artifacts Cleanup (--all only)
# ═══════════════════════════════════════════════════════════════════
cleanup_artifacts() {
  header "Removing Build Artifacts"

  # Node modules
  for dir in demo/server demo/client; do
    if [ -d "$dir/node_modules" ]; then
      rm -rf "$dir/node_modules"
      success "Removed $dir/node_modules"
    fi
    if [ -d "$dir/dist" ]; then
      rm -rf "$dir/dist"
      success "Removed $dir/dist"
    fi
  done

  # .env
  if [ -f .env ]; then
    read -rp "  Remove .env file? (y/N): " remove_env
    if [[ "$remove_env" =~ ^[Yy] ]]; then
      rm -f .env
      success "Removed .env"
    else
      info "Kept .env"
    fi
  fi
}

# ═══════════════════════════════════════════════════════════════════
# Execute
# ═══════════════════════════════════════════════════════════════════
if $KONG_ONLY; then
  cleanup_kong
elif $DOCKER_ONLY; then
  cleanup_docker
else
  cleanup_kong
  cleanup_docker
  if $CLEAN_ALL; then
    cleanup_artifacts
  fi
fi

header "Cleanup Complete"
if $CLEAN_ALL; then
  echo -e "  Full teardown finished. Run ${GREEN}./setup.sh${NC} to start fresh."
else
  echo -e "  Services stopped. Run ${GREEN}./setup.sh --skip-env${NC} to restart."
fi
echo ""
