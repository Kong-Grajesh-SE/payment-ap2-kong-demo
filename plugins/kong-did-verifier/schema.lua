local typedefs = require "kong.db.schema.typedefs"

return {
  name = "kong-did-verifier",
  fields = {
    { protocols = typedefs.protocols_http },
    { consumer = typedefs.no_consumer },
    { config = {
        type = "record",
        fields = {
          { did_registry_url = { type = "string", required = true, default = "http://did-registry:8070" } },
        },
      },
    },
  },
}
