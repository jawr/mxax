CREATE TABLE accounts (
	id SERIAL PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	password BYTEA NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	last_login_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE domains (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	name TEXT UNIQUE  NOT NULL,
	verified_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE aliases (
	id SERIAL PRIMARY KEY,
	domain_id INT NOT NULL REFERENCES domains(id),
	rule TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE destinations (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL REFERENCES accounts(id),
	address TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE,
	UNIQUE (account_id, address)
);

CREATE TABLE alias_destinations (
	alias_id INT NOT NULL REFERENCES aliases(id),
	destination_id INT NOT NULL REFERENCES destinations(id),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMP WITH TIME ZONE,
	UNIQUE (alias_id, destination_id)
);

CREATE TABLE dkim_keys (
	id SERIAL PRIMARY KEY,
	domain_id INT NOT NULL REFERENCES domains(id),
	private_key BYTEA NOT NULL,
	public_key BYTEA NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMP WITH TIME ZONE,
	deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX dkim_keys_domain_id_deleted_at_idx ON dkim_keys (domain_id) WHERE deleted_at IS NULL;

CREATE TABLE return_paths (
	id UUID PRIMARY KEY,
	alias_id INT NOT NULL REFERENCES aliases(id),
	return_to TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
