# kong-did-verifier

A Kong Gateway Go plugin that verifies Ed25519 DID signatures on incoming requests during the Access phase.

## Purpose

After `kong-did-interceptor` provisions a DID and signs the request, this plugin independently verifies that signature by resolving the DID Document and checking the Ed25519 signature against the request body.

Without this plugin, a forged `X-Agent-DID` header with an invalid signature could reach upstream agents unchallenged. With this plugin, Kong blocks any request where the signature doesn't match - preventing identity spoofing and payload tampering.

## How It Works

```
Request → [kong-did-interceptor] → [kong-did-verifier] → Upstream Agent
                                          │
                                          ├─ Resolves DID Document from Registry
                                          ├─ Extracts Ed25519 public key
                                          ├─ Verifies signature against body
                                          │
                                          ├─ ✅ Pass: sets X-Kong-DID-Verified: true
                                          └─ ❌ Fail: returns JSON-RPC error -32003
```

1. Reads `X-Agent-DID` and `X-DID-Signature` headers
2. Resolves the DID Document from the DID Registry to extract the public key
3. Decodes the base64url signature
4. Verifies Ed25519 signature against the raw request body
5. On success: sets `X-Kong-DID-Verified: true` and `X-Kong-Trust-Score` headers
6. On failure: returns a JSON-RPC 2.0 error response (code `-32003`) and blocks the request

## Configuration

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `did_registry_url` | string | yes | `http://did-registry:8070` | URL of the DID Registry for resolution |

## Plugin Info

| Property | Value |
|----------|-------|
| Phase | Access |
| Priority | 900 |
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
   GOOS=linux GOARCH=amd64 go build -o kong-did-verifier .
   ```

3. Mount binary into Kong DP container:
   ```bash
   docker run -d \
     -e "KONG_PLUGINS=bundled,kong-did-verifier" \
     -e "KONG_PLUGINSERVER_NAMES=kong-did-verifier" \
     -e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_START_CMD=/opt/kong/plugins/kong-did-verifier" \
     -e "KONG_PLUGINSERVER_KONG_DID_VERIFIER_QUERY_CMD=/opt/kong/plugins/kong-did-verifier -dump" \
     -v "$(pwd)/kong-did-verifier:/opt/kong/plugins/kong-did-verifier" \
     kong/kong-gateway:3.14
   ```

## File Structure

```
kong-did-verifier/
├── README.md              # This file
├── handler.go             # Access-phase logic (DID resolution + Ed25519 verify)
├── main.go                # Go PDK server entry point
├── go.mod                 # Go module definition
├── schema.lua             # Full schema with typedefs (installed on DP)
└── schema.konnect.lua     # Self-contained schema (uploaded to Konnect CP)
```

## References

- [Writing plugins in Go](https://developer.konghq.com/custom-plugins/go/)
- [Custom plugins in Konnect hybrid mode](https://developer.konghq.com/custom-plugins/konnect-hybrid-mode/)
- [Installation and distribution](https://developer.konghq.com/custom-plugins/installation-and-distribution/)
