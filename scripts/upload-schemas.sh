#!/usr/bin/env bash
# Upload custom plugin schemas to Konnect Control Plane
# Required: KONNECT_API_TOKEN and KONNECT_CONTROL_PLANE_ID env vars

set -euo pipefail

: "${KONNECT_API_TOKEN:?Set KONNECT_API_TOKEN}"
: "${KONNECT_CONTROL_PLANE_ID:?Set KONNECT_CONTROL_PLANE_ID}"

KONNECT_API="https://us.api.konghq.com/v2"
PLUGINS_DIR="$(cd "$(dirname "$0")/../plugins" && pwd)"

upload_schema() {
  local plugin_name="$1"
  local schema_file="$PLUGINS_DIR/$plugin_name/schema.konnect.lua"

  if [[ ! -f "$schema_file" ]]; then
    echo "❌ Schema not found: $schema_file"
    return 1
  fi

  echo "📤 Uploading schema for $plugin_name..."

  local lua_schema
  lua_schema=$(cat "$schema_file")

  local response
  response=$(curl -s -w "\n%{http_code}" -X POST \
    "$KONNECT_API/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas" \
    -H "Authorization: Bearer $KONNECT_API_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg schema "$lua_schema" '{lua_schema: $schema}')")

  local http_code
  http_code=$(echo "$response" | tail -1)
  local body
  body=$(echo "$response" | sed '$d')

  if [[ "$http_code" == "201" || "$http_code" == "200" ]]; then
    echo "✅ $plugin_name schema uploaded successfully"
  elif [[ "$http_code" == "409" ]]; then
    echo "⚠️  $plugin_name schema already exists — updating..."
    # PUT to update
    curl -s -X PUT \
      "$KONNECT_API/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas/$plugin_name" \
      -H "Authorization: Bearer $KONNECT_API_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$(jq -n --arg schema "$lua_schema" '{lua_schema: $schema}')" > /dev/null
    echo "✅ $plugin_name schema updated"
  else
    echo "❌ Failed to upload $plugin_name (HTTP $http_code)"
    echo "$body"
    return 1
  fi
}

echo "=== Uploading Custom Plugin Schemas to Konnect ==="
echo "Control Plane: $KONNECT_CONTROL_PLANE_ID"
echo ""

upload_schema "kong-did-interceptor"
upload_schema "kong-did-verifier"
upload_schema "kong-worm-logger"

echo ""
echo "=== Done ==="
echo "Now rebuild DP with custom plugins: docker build -f plugins/Dockerfile.kong-dp -t kong-dp-custom ."
