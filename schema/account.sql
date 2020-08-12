CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- create triggers for update_at!
CREATE TABLE accounts (
	id SERIAL PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	password BYTEA NOT NULL,
	smtp_password BYTEA,
	account_type INT NOT NULL DEFAULT 0,
	log_level INT NOT NULL DEFAULT 0,
	verify_code UUID NOT NULL DEFAULT gen_random_uuid(),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	verified_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	last_login_at TIMESTAMP WITH TIME ZONE
);
ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
DROP POLICY accounts_isolation_policy ON accounts;
CREATE POLICY accounts_isolation_policy ON accounts 
	USING (id = current_setting('mxax.current_account_id')::INT);

-- domains
CREATE TABLE domains (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	name TEXT UNIQUE NOT NULL,
	verify_code TEXT UNIQUE NOT NULL,
	verified_at TIMESTAMP WITH TIME ZONE,
	expires_at DATE NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE
);

ALTER TABLE domains ENABLE ROW LEVEL SECURITY;
DROP POLICY domains_isolation_policy ON domains;
CREATE POLICY domains_isolation_policy ON domains
	USING (account_id = current_setting('mxax.current_account_id')::INT);


-- records
CREATE TABLE records (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	domain_id INT NOT NULL REFERENCES domains(id),
	host TEXT NOT NULL,
	rtype TEXT NOT NULL,
	value TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	last_verified_at TIMESTAMP WITH TIME ZONE
);

ALTER TABLE records ENABLE ROW LEVEL SECURITY;
DROP POLICY records_isolation_policy ON records;
CREATE POLICY records_isolation_policy ON records 
	USING (account_id = current_setting('mxax.current_account_id')::INT);



CREATE TABLE aliases (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	domain_id INT NOT NULL REFERENCES domains(id),
	rule TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	UNIQUE(domain_id, rule)
);
ALTER TABLE aliases ENABLE ROW LEVEL SECURITY;
DROP POLICY aliases_isolation_policy ON aliases;
CREATE POLICY aliases_isolation_policy ON aliases 
	USING (account_id = current_setting('mxax.current_account_id')::INT);



-- destinations
CREATE TABLE destinations (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	address TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	UNIQUE (account_id, address)
);

ALTER TABLE destinations ENABLE ROW LEVEL SECURITY;
DROP POLICY destinations_isolation_policy ON destinations;
CREATE POLICY destinations_isolation_policy ON destinations 
	USING (account_id = current_setting('mxax.current_account_id')::INT);


-- alias destinations
CREATE TABLE alias_destinations (
	alias_id INT NOT NULL REFERENCES aliases(id),
	destination_id INT NOT NULL REFERENCES destinations(id),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMP WITH TIME ZONE,
	UNIQUE (alias_id, destination_id)
);

ALTER TABLE alias_destinations ENABLE ROW LEVEL SECURITY;
DROP POLICY alias_destinations_isolation_policy ON alias_destinations;
CREATE POLICY alias_destinations_isolation_policy ON alias_destinations 
	USING (account_id = current_setting('mxax.current_account_id')::INT);


-- dkim 
CREATE TABLE dkim_keys (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	domain_id INT NOT NULL REFERENCES domains(id),
	private_key BYTEA NOT NULL,
	public_key BYTEA NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX dkim_keys_domain_id_deleted_at_idx ON dkim_keys (domain_id) 
	WHERE deleted_at IS NULL AND account_id = current_setting('mxax.current_account_id')::INT;

ALTER TABLE dkim_keys ENABLE ROW LEVEL SECURITY;
DROP POLICY dkim_keys_isolation_policy ON dkim_keys;
CREATE POLICY dkim_keys_isolation_policy ON dkim_keys 
	USING (account_id = current_setting('mxax.current_account_id')::INT);


CREATE TABLE return_paths (
	id UUID PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	alias_id INT NOT NULL REFERENCES aliases(id),
	return_to TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	returned_at TIMESTAMP WITH TIME ZONE
);
ALTER TABLE return_paths ENABLE ROW LEVEL SECURITY;
CREATE POLICY return_paths_isolation_policy ON return_paths 
	USING (
		account_id = current_setting('mxax.current_account_id')::INT
	);

-- timescale table for loggin
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

DROP TABLE logs;
CREATE TABLE logs (
	time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	from_email TEXT NOT NULL,
	via_email TEXT NOT NULL,
	to_email TEXT NOT NULL,
	id UUID NOT NULL,
	account_id INT,
	domain_id INT,
	etype INT NOT NULL,
	status TEXT NOT NULL,
	message BYTEA
);
ALTER TABLE logs ENABLE ROW LEVEL SECURITY;
CREATE POLICY logs_isolation_policy ON logs 
	USING (
		account_id = current_setting('mxax.current_account_id')::INT
	);

SELECT create_hypertable('logs', 'time');
