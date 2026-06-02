# kong-did-interceptor

A Kong Gateway Go plugin that provisions W3C Decentralized Identifiers (DIDs) for agent requests during the Access phase.

## Purpose

In the AP2 autonomous payment protocol, every agent interaction must carry a cryptographic identity. This plugin ensures that **Kong provisions the DID** — agents don't need to self-register or manage their own identity lifecycle.

Without this plugin, agents must self-register DIDs. A compromised agent could skip registration or forge an identity. With this plugin, Kong guarantees every request has a verified DID before it reaches the upstream agent.

## How It Works

```
Request → [kong-did-interceptor] → Upstream Agent
```

1. Checks if `X-Agent-DID` header already exists (agent-provided identity)
2. If not, calls DID Registry to provision a new `did:peer` (Ed25519 key pair)
3. Signs the raw request body with the DID's private key
4. Injects headers:
   - `X-Agent-DID` — the provisioned DID identifier
   - `X-DID-Provisioned-At` — ISO 8601 timestamp
   - `X-DID-Signature` — base64url-encoded Ed25519 signature
5. Optionally injects a `kong_did_context` DataPart into A2A message parts

## Configuration

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `did_registry_url` | string | yes | `http://did-registry:8070` | URL of the DID Registry service |
| `did_method` | string | yes | `peer` | DID method to use (`peer` or `web`) |
| `did_web_domain` | string | no | `localhost` | Domain for `did:web` method |

## Plugin Info

| Property | Value |
|----------|-------|
| Phase | Access |
| Priority | 1000 |
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
   GOOS=linux GOARCH=amd64 go build -o kong-did-interceptor .
   ```

3. Mount binary into Kong DP container:
   ```bash
   docker run -d \
     -e "KONG_PLUGINS=bundled,kong-did-interceptor" \
     -e "KONG_PLUGINSERVER_NAMES=kong-did-interceptor" \
     -e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_START_CMD=/opt/kong/plugins/kong-did-interceptor" \
     -e "KONG_PLUGINSERVER_KONG_DID_INTERCEPTOR_QUERY_CMD=/opt/kong/plugins/kong-did-interceptor -dump" \
     -v "$(pwd)/kong-did-interceptor:/opt/kong/plugins/kong-did-interceptor" \
     kong/kong-gateway:3.14
   ```

## File Structure

```
kong-did-interceptor/
├── README.md              # This file
├── handler.go             # Access-phase logic (DID provisioning + signing)
├── main.go                # Go PDK server entry point
├── go.mod                 # Go module definition
├── schema.lua             # Full schema with typedefs (installed on DP)
└── schema.konnect.lua     # Self-contained schema (uploaded to Konnect CP)
```

## References

- [Writing plugins in Go](https://developer.konghq.com/custom-plugins/go/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [Installation and distribution](https://developer.konghq.com/custom-plugins/installation-and-distribution/)
