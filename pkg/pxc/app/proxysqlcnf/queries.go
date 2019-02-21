package proxysqlcnf

var proxysqlServersTable = `
	CREATE TABLE IF NOT EXISTS "proxysql_servers" (
    hostname VARCHAR NOT NULL,
    port INT NOT NULL DEFAULT 6032,
    weight INT CHECK (weight >= 0) NOT NULL DEFAULT 0,
    comment VARCHAR NOT NULL DEFAULT '',
    PRIMARY KEY (hostname, port));`

var runtimeProxysqlServersTable = `
	CREATE TABLE IF NOT EXISTS "runtime_proxysql_servers" (
    hostname VARCHAR NOT NULL,
    port INT NOT NULL DEFAULT 6032,
    weight INT CHECK (weight >= 0) NOT NULL DEFAULT 0,
    comment VARCHAR NOT NULL DEFAULT '',
    PRIMARY KEY (hostname, port));
	`

var runtimeChecksumsValuesTable = `
	CREATE TABLE IF NOT EXISTS "runtime_checksums_values" (
    name VARCHAR NOT NULL,
    version INT NOT NULL,
    epoch INT NOT NULL,
    checksum VARCHAR NOT NULL,
    PRIMARY KEY (name));
	`
