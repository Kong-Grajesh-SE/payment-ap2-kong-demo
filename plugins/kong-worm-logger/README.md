# kong-worm-logger

A Kong Gateway Go plugin that writes immutable audit records to WORM (Write-Once-Read-Many) storage for every A2A agent interaction.

## Purpose

In the AP2 autonomous payment protocol, every agent interaction must be audited with a tamper-proof record. This plugin ensures that **Kong writes the audit trail** - agents cannot bypass, alter, or omit records.

Without this plugin, each agent writes its own audit records. A compromised agent could skip audit writes or forge records. With this plugin, Kong unconditionally writes an immutable record for every interaction, correlated with OpenTelemetry trace context and DID verification status.

## How It Works

```
Request → [Access: capture body] → Agent → Response → [Log: write WORM record]
```

This plugin operates in two phases:

**Access phase** (before upstream):
1. Reads the raw request body (not available in Log phase)
2. Extracts JSON-RPC method name
3. Extracts mandate type and payload if present
4. Stashes data in `kong.ctx.shared` for cross-phase access

**Log phase** (after response is sent to client):
1. Reads W3C `traceparent` header for trace correlation
2. Reads DID headers (`X-Agent-DID`, `X-Receiver-DID`)
3. Reads verification results (`X-Kong-DID-Verified`, `X-Kong-Trust-Score`)
4. Assembles a complete audit record
5. POSTs the record to WORM Storage (immutable write)

## Audit Record Schema

```json
{
  "trace_id": "abc123...",
  "span_id": "def456...",
  "sender_did": "did:peer:2.Ez...",
  "receiver_did": "did:peer:2.Ez...",
  "jsonrpc_method": "search/execute",
  "mandate_type": "IntentMandate",
  "mandate_payload": { ... },
  "kong_verified": true,
  "trust_score": 100
}
```

## Configuration

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `worm_storage_url` | string | yes | `http://worm-storage:8090` | URL of the WORM Storage service |

## Plugin Info

| Property | Value |
|----------|-------|
| Phases | Access + Log |
| Priority | 100 |
| Language | Go (go-pdk) |
| Version | 0.1.0 |
| Protocols | http, https, grpc, grpcs |

## Installation

### Konnect Hybrid Mode

1. Upload schema to Control Plane:
   ```bash
   curl -i -X POST \
     https://us.api.konghq.com/v2/control-planes/{CP_ID}/core-entities/plugin-schemas \
     -H "Authorization: Bearer $KONNECT_API_TOKEN" \
     -H "Content-Type: application/json" \
     --data "{\"lua_schema\": $(jq -Rs . ./schema.konnect.lua)}"
   ```

2. Build and install on Data Plane:
   ```bash
   GOOS=linux GOARCH=amd64 go build -o kong-worm-logger .
   ```

3. Mount binary into Kong DP container:
   ```bash
   docker run -d \
     -e "KONG_PLUGINS=bundled,kong-worm-logger" \
     -e "KONG_PLUGINSERVER_NAMES=kong-worm-logger" \
     -e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_START_CMD=/opt/kong/plugins/kong-worm-logger" \
     -e "KONG_PLUGINSERVER_KONG_WORM_LOGGER_QUERY_CMD=/opt/kong/plugins/kong-worm-logger -dump" \
     -v "$(pwd)/kong-worm-logger:/opt/kong/plugins/kong-worm-logger" \
     kong/kong-gateway:3.14
   ```

## File Structure

```
kong-worm-logger/
├── README.md              # This file
├── handler.go             # Access (capture) + Log (write) phase logic
├── main.go                # Go PDK server entry point
├── go.mod                 # Go module definition
├── schema.lua             # Full schema with typedefs (installed on DP)
└── schema.konnect.lua     # Self-contained schema (uploaded to Konnect CP)
```

## References

- [Writing plugins in Go](https://developer.konghq.com/custom-plugins/go/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [Installation and distribution](https://developer.konghq.com/custom-plugins/installation-and-distribution/)
