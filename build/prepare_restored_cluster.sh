#!/bin/bash

set -o errexit

function escape_special() {
	{ set +x; } 2>/dev/null
	echo "$1" \
		| sed 's/\\/\\\\/g' \
		| sed 's/'\''/'\\\\\''/g' \
		| sed 's/"/\\\"/g'
}

function get_password() {
	local user=$1

	escape_special $(</etc/mysql/mysql-users-secret/${user})
}

MYSQL_VERSION=$(mysqld -V | awk '{print $3}' | awk -F'.' '{print $1"."$2}')
CFG=/etc/mysql/node.cnf

# Operator-managed users must not be subject to server-side password policies that aren't
# appropriate for accounts whose passwords are (re-)applied by the restore process itself:
#   - password_history / password_reuse_interval: restoring a backup re-applies the same
#     password that was already used (and recorded in the password history), which would
#     otherwise be rejected with "Error 3638 ... contradict the password history policy".
#   - password_require_current: would otherwise require the account's current password to be
#     supplied via a REPLACE clause for self password changes (e.g. root), unless the
#     connecting session holds SUPER/CONNECTION_ADMIN.
# Both clauses are only supported starting MySQL 8.0, so use the major version (not an
# exact-match list) to stay correct for future 8.x/9.x releases too.
PASSWORD_POLICY_EXEMPT_CLAUSE=""
if [[ $(echo "$MYSQL_VERSION" | cut -d. -f1) -ge 8 ]]; then
	PASSWORD_POLICY_EXEMPT_CLAUSE=" PASSWORD HISTORY 0 PASSWORD REUSE INTERVAL 0 DAY PASSWORD REQUIRE CURRENT OPTIONAL"
fi

vault_secret="/etc/mysql/vault-keyring-secret/keyring_vault.conf"
if [ -f "${vault_secret}" ]; then
	if [[ $MYSQL_VERSION =~ ^(5\.7|8\.0)$ ]]; then
		sed -i "/\[mysqld\]/a early-plugin-load=keyring_vault.so" $CFG
		sed -i "/\[mysqld\]/a keyring_vault_config=${vault_secret}" $CFG
	fi

	if [[ $MYSQL_VERSION == '8.4' ]]; then
		echo -n '{ "components": "file://component_keyring_vault" }' > /var/lib/mysql/mysqld.my
		cp ${vault_secret} /var/lib/mysql/component_keyring_vault.cnf
	fi
fi

mysqld --skip-grant-tables --skip-networking &

# TODO: Is there a better way?
sleep 60

mysql <<EOF
SET @@SESSION.SQL_LOG_BIN=0;

FLUSH PRIVILEGES;

CREATE USER IF NOT EXISTS 'root'@'%' IDENTIFIED BY '$(get_password root)' PASSWORD EXPIRE NEVER;
ALTER USER IF EXISTS 'root'@'%' IDENTIFIED BY '$(get_password root)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
GRANT ALL ON *.* TO 'root'@'%' WITH GRANT OPTION;

CREATE USER IF NOT EXISTS 'root'@'localhost' IDENTIFIED BY '$(get_password root)' PASSWORD EXPIRE NEVER;
ALTER USER IF EXISTS 'root'@'localhost' IDENTIFIED BY '$(get_password root)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
GRANT ALL ON *.* TO 'root'@'localhost' WITH GRANT OPTION;

ALTER USER IF EXISTS 'operator'@'%' IDENTIFIED BY '$(get_password operator)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
CREATE USER IF NOT EXISTS 'operator'@'%' IDENTIFIED BY '$(get_password operator)' PASSWORD EXPIRE NEVER;
GRANT ALL ON *.* TO 'operator'@'%' WITH GRANT OPTION;

ALTER USER IF EXISTS 'replication'@'%' IDENTIFIED BY '$(get_password replication)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
CREATE USER IF NOT EXISTS 'replication'@'%' IDENTIFIED BY '$(get_password replication)' PASSWORD EXPIRE NEVER;
GRANT REPLICATION SLAVE ON *.* to 'replication'@'%';

ALTER USER IF EXISTS 'xtrabackup'@'%' IDENTIFIED BY '$(get_password xtrabackup)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
CREATE USER IF NOT EXISTS 'xtrabackup'@'%' IDENTIFIED BY '$(get_password xtrabackup)' PASSWORD EXPIRE NEVER;
GRANT ALL ON *.* TO 'xtrabackup'@'%' WITH GRANT OPTION;

ALTER USER IF EXISTS 'monitor'@'%' IDENTIFIED BY '$(get_password monitor)'${PASSWORD_POLICY_EXEMPT_CLAUSE};
CREATE USER IF NOT EXISTS 'monitor'@'%' IDENTIFIED BY '$(get_password monitor)' WITH MAX_USER_CONNECTIONS 100 PASSWORD EXPIRE NEVER;
GRANT SELECT, PROCESS, SUPER, REPLICATION CLIENT, RELOAD ON *.* TO 'monitor'@'%';
GRANT SELECT ON performance_schema.* TO 'monitor'@'%';
/*!80400 GRANT AUDIT_ADMIN ON *.* TO 'monitor'@'%' */;
/*!80016 GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'%' */;
/*!80016 GRANT SYSTEM_USER ON *.* TO 'monitor'@'%' */;

FLUSH PRIVILEGES;
EOF

rm /var/lib/mysql/grastate.dat /var/lib/mysql/gvwstate.dat
