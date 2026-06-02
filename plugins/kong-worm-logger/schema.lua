local typedefs = require "kong.db.schema.typedefs"

return {
  name = "kong-worm-logger",
  fields = {
    { protocols = typedefs.protocols_http },
    { consumer = typedefs.no_consumer },
    { config = {
        type = "record",
        fields = {
          { worm_storage_url = { type = "string", required = true, default = "http://worm-storage:8090" } },
        },
      },
    },
  },
}
