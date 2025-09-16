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
    callback_id TEXT,
    self TEXT,
    role TEXT,
    auto_accept BOOLEAN, -- Note: sqlite doesn't have boolean, this actually becomes a NUMERIC field.
    requester_info TEXT, -- For now, this should be just the JSON representation of the requester info.
    trace_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    locked BOOLEAN,
    FOREIGN KEY(agreement_id) REFERENCES agreements(id)
);

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
    callback_id TEXT,
    self TEXT,
    role TEXT,
    transfer_direction TEXT,
    publish_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    requester_info TEXT, -- For now, this should be just the JSON representation of the requester_info.
    trace_info TEXT, -- For now, this should be just the JSON representation of the trace info.
    auto_accept BOOLEAN, -- Note: sqlite doesn't have boolean, this actually becomes a NUMERIC field.
    locked BOOLEAN,
    FOREIGN KEY(agreement_id) REFERENCES agreements(id)
);


-- Verification token table.
CREATE TABLE IF NOT EXISTS tokens (
    key TEXT NOT NULL PRIMARY KEY,
    token TEXT NOT NULL
);
