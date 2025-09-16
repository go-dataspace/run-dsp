-- Table containing the ODRL agreements.
-- As ODRL is a very flexible format and we don't query on it right now,
-- we just store an ID linked with the JSON representation of it.
CREATE TABLE IF NOT EXISTS agreements (
    id TEXT NOT NULL PRIMARY KEY,
    odrl TEXT NOT NULL
);

-- Table containing the contract negotiations
-- For now, it seems not useful to have a separate
-- primary key.
CREATE TABLE IF NOT EXISTS contract_negotiations (
    provider_pid TEXT NULL, -- Note: sqlite has no UUID field, or varchar, so this becomes TEXT.
    consumer_pid TEXT NULL,
    agreement_id TEXT NULL,
    state TEXT,
    callback_url TEXT,
    self_url TEXT,
    role TEXT,
    auto_accept BOOLEAN, -- Note: sqlite doesn't have boolean, this actually becomes a NUMERIC field.
    requester_info TEXT, -- For now, this should be just the JSON representation of the requester info.
    trace_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    locked BOOLEAN,
    FOREIGN KEY(agreement_id) REFERENCES agreements(id)
);

CREATE UNIQUE INDEX cn_provider_pid_idx ON contract_negotiations(provider_pid);
CREATE UNIQUE INDEX cn_consumer_pid_idx ON contract_negotiations(consumer_pid);
CREATE UNIQUE INDEX cn_agreement_id_idx ON contract_negotiations(agreement_id);
CREATE UNIQUE INDEX cn_callback_idx ON contract_negotiations(callback_url);
CREATE UNIQUE INDEX cn_role_idx ON contract_negotiations(role);
CREATE UNIQUE INDEX cn_state_idx ON contract_negotiations(state);

-- Table containing the contract negotiations
-- For now, it seems not useful to have a separate
-- primary key.
CREATE TABLE IF NOT EXISTS transfer_requests (
    provider_pid TEXT NULL, -- Note: sqlite has no UUID field, or varchar, so this becomes TEXT.
    consumer_pid TEXT NULL,
    agreement_id TEXT, -- TODO: Change this to an actual foreign key.
    target TEXT,
    format TEXT,
    state TEXT,
    callback_url TEXT,
    self_url TEXT,
    role TEXT,
    transfer_direction TEXT,
    publish_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    requester_info TEXT, -- For now, this should be just the JSON representation of the requester_info.
    trace_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    auto_accept BOOLEAN, -- Note: sqlite doesn't have boolean, this actually becomes a NUMERIC field.
    locked BOOLEAN,
    FOREIGN KEY(agreement_id) REFERENCES agreements(id)
);

CREATE UNIQUE INDEX tr_provider_pid_idx ON transfer_requests(provider_pid);
CREATE UNIQUE INDEX tr_consumer_pid_idx ON transfer_requests(consumer_pid);
CREATE UNIQUE INDEX tr_agreement_id_idx ON transfer_requests(agreement_id);
CREATE UNIQUE INDEX tr_callback_idx ON transfer_requests(callback_url);
CREATE UNIQUE INDEX tr_role_idx ON transfer_requests(role);
CREATE UNIQUE INDEX tr_state_idx ON transfer_requests(state);

-- Verification token table.
CREATE TABLE IF NOT EXISTS tokens (
    key TEXT NOT NULL PRIMARY KEY,
    token TEXT NOT NULL
);
