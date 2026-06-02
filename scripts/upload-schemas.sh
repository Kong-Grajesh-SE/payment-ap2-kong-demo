#!/usr/bin/env bash
# Upload custom plugin schemas to Konnect Control Plane (Hybrid Mode)
# Reference: https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/
#
# In Konnect hybrid mode, you upload the schema.lua (no require() statements)
# to the Control Plane via API. Then install the full plugin files on each DP node.
#
# Required: KONNECT_API_TOKEN and KONNECT_CONTROL_PLANE_ID env vars

set -euo pipefail

: "${KONNECT_API_TOKEN:?Set KONNECT_API_TOKEN}"
: "${KONNECT_CONTROL_PLANE_ID:?Set KONNECT_CONTROL_PLANE_ID}"
: "${KONNECT_REGION:=us}"

KONNECT_API="https://${KONNECT_REGION}.api.konghq.com/v2"
PLUGINS_DIR="$(cd "$(dirname "$0")/../plugins" && pwd)"

upload_schema() {
  local plugin_name="$1"
  # Use schema.konnect.lua (no require statements) for Konnect CP upload
  local schema_file="$PLUGINS_DIR/$plugin_name/schema.konnect.lua"

  if [[ ! -f "$schema_file" ]]; then
    echo "❌ Schema not found: $schema_file"
    return 1
  fi

  echo "📤 Uploading schema for $plugin_name..."

  # Use jq -Rs to read and escape the schema file content
  # Reference: https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/#add-a-custom-plugin-to-a-control-plane
  local response
  response=$(curl -s -w "\n%{http_code}" -X POST \
    "$KONNECT_API/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas" \
    -H "Authorization: Bearer $KONNECT_API_TOKEN" \
    -H "Content-Type: application/json" \
    --data "{\"lua_schema\": $(jq -Rs . "$schema_file")}")

  local http_code
  http_code=$(echo "$response" | tail -1)
  local body
  body=$(echo "$response" | sed '$d')

  if [[ "$http_code" == "201" || "$http_code" == "200" ]]; then
    echo "✅ $plugin_name schema uploaded successfully"
  elif [[ "$http_code" == "409" ]]; then
    echo "⚠️  $plugin_name schema already exists - updating..."
    curl -s -X PUT \
      "$KONNECT_API/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas/$plugin_name" \
      -H "Authorization: Bearer $KONNECT_API_TOKEN" \
      -H "Content-Type: application/json" \
      --data "{\"lua_schema\": $(jq -Rs . "$schema_file")}" > /dev/null
    echo "✅ $plugin_name schema updated"
  else
    echo "❌ Failed to upload $plugin_name (HTTP $http_code)"
    echo "$body"
    return 1
  fi
}

verify_schema() {
  local plugin_name="$1"
  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X GET \
    "$KONNECT_API/control-planes/$KONNECT_CONTROL_PLANE_ID/core-entities/plugin-schemas/$plugin_name" \
    -H "Authorization: Bearer $KONNECT_API_TOKEN")

  if [[ "$http_code" == "200" ]]; then
    echo "  ✓ $plugin_name - verified in CP"
  else
    echo "  ✗ $plugin_name - NOT found (HTTP $http_code)"
  fi
}

echo "=== Uploading Custom Plugin Schemas to Konnect CP ==="
echo "Region: $KONNECT_REGION"
echo "Control Plane: $KONNECT_CONTROL_PLANE_ID"
echo ""

upload_schema "kong-did-interceptor"
upload_schema "kong-did-verifier"
upload_schema "kong-worm-logger"

echo ""
echo "=== Verifying uploads ==="
verify_schema "kong-did-interceptor"
verify_schema "kong-did-verifier"
verify_schema "kong-worm-logger"

echo ""
echo "=== Done ==="
echo ""
echo "Next: Install plugin files on your Data Plane node."
echo "See SETUP.md for volume-mount instructions."
