#!/bin/bash
set -eo pipefail
shopt -s nullglob
set -o xtrace

# if command starts with an option, prepend mysqld
if [ "${1:0:1}" = '-' ]; then
	set -- mysqld "$@"
fi

# skip setup if they want an option that stops mysqld
wantHelp=
for arg; do
	case "$arg" in
		-'?' | --help | --print-defaults | -V | --version)
			wantHelp=1
			break
			;;
	esac
done

# usage: file_env VAR [DEFAULT]
#    ie: file_env 'XYZ_DB_PASSWORD' 'example'
# (will allow for "$XYZ_DB_PASSWORD_FILE" to fill in the value of
#  "$XYZ_DB_PASSWORD" from a file, especially for Docker's secrets feature)
file_env() {
	set +o xtrace
	local var="$1"
	local fileVar="${var}_FILE"
	local def="${2:-}"
	if [ "${!var:-}" ] && [ "${!fileVar:-}" ]; then
		echo >&2 "error: both $var and $fileVar are set (but are exclusive)"
		exit 1
	fi
	local val="$def"
	if [ "${!var:-}" ]; then
		val="${!var}"
	elif [ "${!fileVar:-}" ]; then
		val="$(< "${!fileVar}")"
	elif [ "${3:-}" ] && [ -f "/etc/mysql/mysql-users-secret/$3" ]; then
		val="$(</etc/mysql/mysql-users-secret/$3)"
	fi
	export "$var"="$val"
	unset "$fileVar"
	set -o xtrace
}

# usage: process_init_file FILENAME MYSQLCOMMAND...
#    ie: process_init_file foo.sh mysql -uroot
# (process a single initializer file, based on its extension. we define this
# function here, so that initializer scripts (*.sh) can use the same logic,
# potentially recursively, or override the logic used in subsequent calls)
process_init_file() {
	local f="$1"
	shift
	local mysql=("$@")

	case "$f" in
		*.sh)
			echo "$0: running $f"
			. "$f"
			;;
		*.sql)
			echo "$0: running $f"
			"${mysql[@]}" <"$f"
			echo
			;;
		*.sql.gz)
			echo "$0: running $f"
			gunzip -c "$f" | "${mysql[@]}"
			echo
			;;
		*) echo "$0: ignoring $f" ;;
	esac
	echo
}

_check_config() {
	toRun=( "$@" --verbose --help )
	if ! errors="$("${toRun[@]}" 2>&1 >/dev/null)"; then
		cat >&2 <<-EOM

			ERROR: mysqld failed while attempting to check config
			command was: "${toRun[*]}"

			$errors
		EOM
		exit 1
	fi
}

# Fetch value from server config
# We use mysqld --verbose --help instead of my_print_defaults because the
# latter only show values present in config files, and not server defaults
_get_config() {
	local conf="$1"
	shift
	"$@" --verbose --help --log-bin-index="$(mktemp -u)" 2>/dev/null \
		| awk '$1 == "'"$conf"'" && /^[^ \t]/ { sub(/^[^ \t]+[ \t]+/, ""); print; exit }'
	# match "datadir      /some/path with/spaces in/it here" but not "--xyz=abc\n     datadir (xyz)"
}

# Fetch value from customized configs, needed for non-mysqld options like sst
_get_cnf_config() {
	local group=$1
	local var=${2//_/-}
	local reval=""

	reval=$(
		my_print_defaults "${group}" \
			| awk -F= '{st=index($0,"="); cur=$0; if ($1 ~ /_/) { gsub(/_/,"-",$1);} if (st != 0) { print $1"="substr(cur,st+1) } else { print cur }}' \
			| grep -- "--$var=" \
			| cut -d= -f2- \
			| tail -1
	)

	if [[ -z $reval ]]; then
		reval=$3
	fi
	echo "$reval"
}

_get_tmpdir() {
	local defaul_value="$1"
	local tmpdir_path=""

	tmpdir_path=$(_get_cnf_config mysqld tmpdir "")
	if [[ -z ${tmpdir_path} ]]; then
		tmpdir_path=$(_get_cnf_config xtrabackup tmpdir "")
	fi
	if [[ -z ${tmpdir_path} ]]; then
		tmpdir_path="$defaul_value"
	fi
	echo "$tmpdir_path"
}

MYSQL_VERSION=$(mysqld -V | awk '{print $3}' | awk -F'.' '{print $1"."$2}')
MYSQL_PATCH_VERSION=$(mysqld -V | awk '{print $3}' | awk -F'.' '{print $3}' | awk -F'-' '{print $1}')

file_env 'XTRABACKUP_PASSWORD' 'xtrabackup' 'xtrabackup'
file_env 'CLUSTERCHECK_PASSWORD' '' 'clustercheck'

NODE_NAME=$(hostname -f)
NODE_PORT=3306

if [ "$1" = 'mysqld' -a -z "$wantHelp" ]; then
	# still need to check config, container may have started with --user
	_check_config "$@"

	if [ -n "$INIT_TOKUDB" ]; then
		export LD_PRELOAD=/usr/lib64/libjemalloc.so.1
	fi
	# Get config
	DATADIR="$(_get_config 'datadir' "$@")"
	TMPDIR=$(_get_tmpdir "$DATADIR/mysql-tmpdir")

	rm -rfv "$TMPDIR"

#	it is temporary solution
	echo '[mysqld]' > /etc/my.cnf.d/node.cnf 
	sed -i "/\[mysqld\]/a report_host=${MY_FQDN}" /etc/my.cnf.d/node.cnf
	if [ ! -d "$DATADIR/mysql" ]; then
		file_env 'MYSQL_ROOT_PASSWORD' '' 'root'
		{ set +x; } 2>/dev/null
		if [ -z "$MYSQL_ROOT_PASSWORD" -a -z "$MYSQL_ALLOW_EMPTY_PASSWORD" -a -z "$MYSQL_RANDOM_ROOT_PASSWORD" ]; then
			echo >&2 'error: database is uninitialized and password option is not specified '
			echo >&2 '  You need to specify one of MYSQL_ROOT_PASSWORD, MYSQL_ALLOW_EMPTY_PASSWORD and MYSQL_RANDOM_ROOT_PASSWORD'
			exit 1
		fi
		set -x

		mkdir -p "$DATADIR"
#		find "$DATADIR" -mindepth 1 -prune -o -exec rm -rfv {} \+ 1>/dev/null

		echo 'Initializing database'
		# we initialize database into $TMPDIR because "--initialize-insecure" option does not work if directory is not empty
		# in some cases storage driver creates unremovable artifacts (see K8SPXC-286), so $DATADIR cleanup is not possible
		"$@" --initialize-insecure --skip-ssl --datadir="$TMPDIR"
		mv "$TMPDIR"/* "$DATADIR/"
		rm -rfv "$TMPDIR"
		echo 'Database initialized'

		SOCKET="$(_get_config 'socket' "$@")"
		"$@" --skip-networking --socket="${SOCKET}" &
		pid="$!"

		mysql=(mysql --protocol=socket -uroot -hlocalhost --socket="${SOCKET}" --password="")

		for i in {120..0}; do
			if echo 'SELECT 1' | "${mysql[@]}" &> /dev/null; then
				break
			fi
			echo 'MySQL init process in progress...'
			sleep 1
		done
		if [ "$i" = 0 ]; then
			echo >&2 'MySQL init process failed.'
			exit 1
		fi

		if [ -z "$MYSQL_INITDB_SKIP_TZINFO" ]; then
			(
				echo "SET @@SESSION.SQL_LOG_BIN = off;"
				# sed is for https://bugs.mysql.com/bug.php?id=20545
				mysql_tzinfo_to_sql /usr/share/zoneinfo | sed 's/Local time zone must be set--see zic manual page/FCTY/'
			) | "${mysql[@]}" mysql
		fi

		# install TokuDB engine
		if [ -n "$INIT_TOKUDB" ]; then
			ps-admin --docker --enable-tokudb -u root -p $MYSQL_ROOT_PASSWORD
		fi
		if [ -n "$INIT_ROCKSDB" ]; then
			ps-admin --docker --enable-rocksdb -u root -p $MYSQL_ROOT_PASSWORD
		fi

		{ set +x; } 2>/dev/null
		if [ ! -z "$MYSQL_RANDOM_ROOT_PASSWORD" ]; then
			MYSQL_ROOT_PASSWORD="$(pwmake 128)"
			echo "GENERATED ROOT PASSWORD: $MYSQL_ROOT_PASSWORD"
		fi
		set -x

		rootCreate=
		# default root to listen for connections from anywhere
		file_env 'MYSQL_ROOT_HOST' '%'
		if [ ! -z "$MYSQL_ROOT_HOST" -a "$MYSQL_ROOT_HOST" != 'localhost' ]; then
			# no, we don't care if read finds a terminating character in this heredoc
			# https://unix.stackexchange.com/questions/265149/why-is-set-o-errexit-breaking-this-read-heredoc-expression/265151#265151
			read -r -d '' rootCreate <<-EOSQL || true
				CREATE USER 'root'@'${MYSQL_ROOT_HOST}' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}' ;
				GRANT ALL ON *.* TO 'root'@'${MYSQL_ROOT_HOST}' WITH GRANT OPTION ;
			EOSQL
		fi

		file_env 'MONITOR_HOST' 'localhost'
		file_env 'MONITOR_PASSWORD' 'monitor' 'monitor'
		file_env 'REPLICATION_PASSWORD' '' 'replication'
		if [ "$MYSQL_VERSION" == '8.0' ]; then
			read -r -d '' monitorConnectGrant <<-EOSQL || true
				GRANT SERVICE_CONNECTION_ADMIN ON *.* TO 'monitor'@'${MONITOR_HOST}';
			EOSQL
		fi
		"${mysql[@]}" <<-EOSQL
			-- What's done in this file shouldn't be replicated
			--  or products like mysql-fabric won't work
			SET @@SESSION.SQL_LOG_BIN=0;

			DELETE FROM mysql.user WHERE user NOT IN ('mysql.sys', 'mysqlxsys', 'root', 'mysql.infoschema', 'mysql.session') OR host NOT IN ('localhost') ;
			ALTER USER 'root'@'localhost' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}' ;
			GRANT ALL ON *.* TO 'root'@'localhost' WITH GRANT OPTION ;
			${rootCreate}
			/*!80016 REVOKE SYSTEM_USER ON *.* FROM root */;

			CREATE USER 'operator'@'${MYSQL_ROOT_HOST}' IDENTIFIED BY '${OPERATOR_ADMIN_PASSWORD}' ;
			GRANT ALL ON *.* TO 'operator'@'${MYSQL_ROOT_HOST}' WITH GRANT OPTION ;

			CREATE USER 'xtrabackup'@'%' IDENTIFIED BY '${XTRABACKUP_PASSWORD}';
			GRANT ALL ON *.* TO 'xtrabackup'@'%';

			CREATE USER 'monitor'@'${MONITOR_HOST}' IDENTIFIED BY '${MONITOR_PASSWORD}' WITH MAX_USER_CONNECTIONS 100;
			GRANT SELECT, PROCESS, SUPER, REPLICATION CLIENT, RELOAD ON *.* TO 'monitor'@'${MONITOR_HOST}';
			GRANT SELECT ON performance_schema.* TO 'monitor'@'${MONITOR_HOST}';
			${monitorConnectGrant}

			CREATE USER 'clustercheck'@'localhost' IDENTIFIED BY '${CLUSTERCHECK_PASSWORD}';
			GRANT PROCESS ON *.* TO 'clustercheck'@'localhost';

			CREATE USER 'replication'@'%' IDENTIFIED BY '${REPLICATION_PASSWORD}';
			GRANT REPLICATION SLAVE ON *.* to 'replication'@'%';

			CREATE USER 'orchestrator'@'%' IDENTIFIED BY '${ORC_TOPOLOGY_PASSWORD}';
			GRANT SUPER, PROCESS, REPLICATION SLAVE, RELOAD ON *.* TO 'orchestrator'@'%';
			GRANT SELECT ON mysql.slave_master_info TO 'orchestrator'@'%';

			DROP DATABASE IF EXISTS test;
			FLUSH PRIVILEGES ;
		EOSQL

		{ set +x; } 2>/dev/null
		if [ ! -z "$MYSQL_ROOT_PASSWORD" ]; then
			mysql+=(-p"${MYSQL_ROOT_PASSWORD}")
		fi
		set -x

		file_env 'MYSQL_DATABASE'
		if [ "$MYSQL_DATABASE" ]; then
			echo "CREATE DATABASE IF NOT EXISTS \`$MYSQL_DATABASE\` ;" | "${mysql[@]}"
			mysql+=("$MYSQL_DATABASE")
		fi

		file_env 'MYSQL_USER'
		file_env 'MYSQL_PASSWORD'
		{ set +x; } 2>/dev/null
		if [ "$MYSQL_USER" -a "$MYSQL_PASSWORD" ]; then
			echo "CREATE USER '$MYSQL_USER'@'%' IDENTIFIED BY '$MYSQL_PASSWORD' ;" | "${mysql[@]}"

			if [ "$MYSQL_DATABASE" ]; then
				echo "GRANT ALL ON \`$MYSQL_DATABASE\`.* TO '$MYSQL_USER'@'%' ;" | "${mysql[@]}"
			fi

			echo 'FLUSH PRIVILEGES ;' | "${mysql[@]}"
		fi
		set -x

		echo
		ls /docker-entrypoint-initdb.d/ >/dev/null
		for f in /docker-entrypoint-initdb.d/*; do
			process_init_file "$f" "${mysql[@]}"
		done

		{ set +x; } 2>/dev/null
		if [ ! -z "$MYSQL_ONETIME_PASSWORD" ]; then
			"${mysql[@]}" <<-EOSQL
				ALTER USER 'root'@'%' PASSWORD EXPIRE;
			EOSQL
		fi
		set -x
		if ! kill -s TERM "$pid" || ! wait "$pid"; then
			echo >&2 'MySQL init process failed.'
			exit 1
		fi

		echo
		echo 'MySQL init process done. Ready for start up.'
		echo
	fi

	# exit when MYSQL_INIT_ONLY environment variable is set to avoid starting mysqld
	if [ ! -z "$MYSQL_INIT_ONLY" ]; then
		echo 'Initialization complete, now exiting!'
		exit 0
	fi
fi

exec "$@"
