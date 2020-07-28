CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

DROP TABLE metrics__inbound_rejects;
CREATE TABLE metrics__inbound_rejects (
	time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	from_email TEXT NOT NULL,
	to_email TEXT NOT NULL,
	domain_id INTEGER
);
SELECT create_hypertable('metrics__inbound_rejects', 'time');

DROP TABLE metrics__inbound_forwards;
CREATE TABLE metrics__inbound_forwards (
	time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	from_email TEXT NOT NULL,
	domain_id INTEGER REFERENCES domains (id),
	alias_id INTEGER REFERENCES aliases (id),
	destination_id INTEGER REFERENCES destinations (id)
);
SELECT create_hypertable('metrics__inbound_forwards', 'time');

DROP TABLE metrics__inbound_bounces;
CREATE TABLE metrics__inbound_bounces (
	time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	from_email TEXT NOT NULL,
	domain_id INTEGER REFERENCES domains (id),
	alias_id INTEGER,
	destination_id INTEGER,
	message JSONB NOT NULL,
	reason TEXT NOT NULL
);
SELECT create_hypertable('metrics__inbound_bounces', 'time');
