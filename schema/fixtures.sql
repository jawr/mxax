INSERT INTO accounts (username, password) VALUES ('jess@lawrence.pm', ''::BYTEA);
INSERT INTO domains (account_id, name) VALUES (1, 'lawrence.pm');
INSERT INTO aliases (domain_id, rule) VALUES (1, 'jess');
