CREATE TABLE IF NOT EXISTS audit_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id        TEXT NOT NULL,
    span_id         TEXT NOT NULL,
    sender_did      TEXT NOT NULL,
    receiver_did    TEXT,
    jsonrpc_method  TEXT NOT NULL,
    mandate_type    TEXT,
    mandate_payload JSONB,
    kong_verified   BOOLEAN DEFAULT false,
    trust_score     INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_trace_id ON audit_records(trace_id);
CREATE INDEX IF NOT EXISTS idx_audit_sender_did ON audit_records(sender_did);

-- WORM enforcement: revoke UPDATE and DELETE from the application user
REVOKE UPDATE, DELETE ON audit_records FROM worm_user;
