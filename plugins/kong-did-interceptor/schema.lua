local typedefs = require "kong.db.schema.typedefs"

return {
  name = "kong-did-interceptor",
  fields = {
    { protocols = typedefs.protocols_http },
    { consumer = typedefs.no_consumer },
    { config = {
        type = "record",
        fields = {
          { did_registry_url = { type = "string", required = true, default = "http://did-registry:8070" } },
          { did_method = { type = "string", required = true, default = "peer", one_of = {"peer", "web"} } },
          { did_web_domain = { type = "string", default = "localhost" } },
        },
      },
    },
  },
}
