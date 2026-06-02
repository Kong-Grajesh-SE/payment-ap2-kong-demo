-- Konnect-compatible schema (no require statements)
-- This file is used for uploading to Konnect Control Plane.
-- The full schema.lua with typedefs is used on the Data Plane.
return {
  name = "kong-did-verifier",
  fields = {
    { protocols = {
        type = "set",
        elements = { type = "string", one_of = { "http", "https", "grpc", "grpcs" } },
        default = { "http", "https" },
        required = true,
      },
    },

    { config = {
        type = "record",
        fields = {
          { did_registry_url = { type = "string", required = true, default = "http://did-registry:8070" } },
        },
      },
    },
  },
}
