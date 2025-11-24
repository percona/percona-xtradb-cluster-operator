#!/bin/bash

set -o errexit
set -o xtrace

function sed_in_place() {
	local cmd=$1
	local file=$2
	local tmp=$(mktemp)

	sed "${cmd}" "${file}" >"${tmp}"
	cat "${tmp}" >"${file}"
	rm "${tmp}"
}

cp /opt/percona/proxysql.cnf /etc/proxysql
cp /opt/percona/proxysql-admin.cnf /etc

MYSQL_INTERFACES='0.0.0.0:3306;0.0.0.0:33062'
CLUSTER_PORT='33062'

PROXY_CFG=/etc/proxysql/proxysql.cnf
PROXY_ADMIN_CFG=/etc/proxysql-admin.cnf

# Percona scheduler
PERCONA_SCHEDULER_CFG_TMPL=/opt/percona/proxysql_scheduler_config.tmpl
PERCONA_SCHEDULER_CFG=/opt/percona/scheduler-config.toml
if [[ -f ${PERCONA_SCHEDULER_CFG_TMPL} ]]; then
	cp ${PERCONA_SCHEDULER_CFG_TMPL} ${PERCONA_SCHEDULER_CFG}
fi

# internal scheduler
sed_in_place "s/#export WRITERS_ARE_READERS=.*$/export WRITERS_ARE_READERS='yes'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/interfaces=\"0.0.0.0:3306\"/interfaces=\"${MYSQL_INTERFACES:-0.0.0.0:3306}\"/g" ${PROXY_CFG}
sed_in_place "s/stacksize=1048576/stacksize=${MYSQL_STACKSIZE:-1048576}/g" ${PROXY_CFG}
sed_in_place "s/threads=2/threads=${MYSQL_THREADS:-2}/g" ${PROXY_CFG}

set +o xtrace # hide sensitive information
OPERATOR_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${OPERATOR_PASSWORD}")
MONITOR_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${MONITOR_PASSWORD}")
PROXY_ADMIN_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${PROXY_ADMIN_PASSWORD}")

sed_in_place "s/\"admin:admin\"/\"${PROXY_ADMIN_USER:-admin}:${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}\"/g" ${PROXY_CFG}
sed_in_place "s/cluster_username=\"admin\"/cluster_username=\"${PROXY_ADMIN_USER:-admin}\"/g" ${PROXY_CFG}
sed_in_place "s/cluster_password=\"admin\"/cluster_password=\"${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}\"/g" ${PROXY_CFG}
sed_in_place "s/monitor_password=\"monitor\"/monitor_password=\"${MONITOR_PASSWORD_ESCAPED:-monitor}\"/g" ${PROXY_CFG}
sed_in_place "s/PROXYSQL_USERNAME='admin'/PROXYSQL_USERNAME='${PROXY_ADMIN_USER:-admin}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/PROXYSQL_PASSWORD='admin'/PROXYSQL_PASSWORD='${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/CLUSTER_USERNAME='admin'/CLUSTER_USERNAME='${OPERATOR_USERNAME:-operator}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/CLUSTER_PASSWORD='admin'/CLUSTER_PASSWORD='${OPERATOR_PASSWORD_ESCAPED:-operator}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/CLUSTER_PORT='3306'/CLUSTER_PORT='${CLUSTER_PORT:-3306}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/MONITOR_USERNAME='monitor'/MONITOR_USERNAME='${MONITOR_USERNAME:-monitor}'/g" ${PROXY_ADMIN_CFG}
sed_in_place "s/MONITOR_PASSWORD='monitor'/MONITOR_PASSWORD='${MONITOR_PASSWORD_ESCAPED:-monitor}'/g" ${PROXY_ADMIN_CFG}
set -o xtrace # hide sensitive information

# Percona scheduler
if [[ -f ${PERCONA_SCHEDULER_CFG} ]]; then
	set +o xtrace # hide sensitive information
	sed_in_place "s/SCHEDULER_PROXYSQLHOST/'$(hostname -f)'/" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_PROXYSQLPASSWORD/'${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}'/" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_CLUSTERPASSWORD/'${OPERATOR_PASSWORD_ESCAPED:-operator}'/" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_CLUSTERPORT/'${CLUSTER_PORT:-3306}'/" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_MONITORPASSWORD/'${MONITOR_PASSWORD_ESCAPED:-monitor}'/" ${PERCONA_SCHEDULER_CFG}
	set -o xtrace # hide sensitive information

	sed_in_place "s/SCHEDULER_MAXCONNECTIONS/${SCHEDULER_MAXCONNECTIONS}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_NODECHECKINTERVAL/${SCHEDULER_NODECHECKINTERVAL}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_CHECKTIMEOUT/${SCHEDULER_CHECKTIMEOUT}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_PINGTIMEOUT/${SCHEDULER_PINGTIMEOUT}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_RETRYDOWN/${SCHEDULER_RETRYDOWN}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_RETRYUP/${SCHEDULER_RETRYUP}/g" ${PERCONA_SCHEDULER_CFG}
	sed_in_place "s/SCHEDULER_WRITERALSOREADER/${SCHEDULER_WRITERALSOREADER}/g" ${PERCONA_SCHEDULER_CFG}
fi

## SSL/TLS support
CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
if [ -f "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt" ]; then
	CA=/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
fi
SSL_DIR=${SSL_DIR:-/etc/proxysql/ssl}
if [ -f "${SSL_DIR}/ca.crt" ]; then
	CA=${SSL_DIR}/ca.crt
	if [[ -f ${PERCONA_SCHEDULER_CFG} ]]; then
		sed_in_place "s:^sslCertificatePath.*= .*\"$:sslCertificatePath = \"${SSL_DIR}\":" ${PERCONA_SCHEDULER_CFG}
	fi
fi
SSL_INTERNAL_DIR=${SSL_INTERNAL_DIR:-/etc/proxysql/ssl-internal}
if [ -f "${SSL_INTERNAL_DIR}/ca.crt" ]; then
	CA=${SSL_INTERNAL_DIR}/ca.crt
	if [[ -f ${PERCONA_SCHEDULER_CFG} ]]; then
		sed_in_place "s:^sslCertificatePath.*= .*\"$:sslCertificatePath = \"${SSL_INTERNAL_DIR}\":" ${PERCONA_SCHEDULER_CFG}
	fi
fi

KEY=${SSL_DIR}/tls.key
CERT=${SSL_DIR}/tls.crt
if [ -f "${SSL_INTERNAL_DIR}/tls.key" ] && [ -f "${SSL_INTERNAL_DIR}/tls.crt" ]; then
	KEY=${SSL_INTERNAL_DIR}/tls.key
	CERT=${SSL_INTERNAL_DIR}/tls.crt
fi

if [ -f "$CA" ] && [ -f "$KEY" ] && [ -f "$CERT" ] && [ -n "$PXC_SERVICE" ]; then
	sed_in_place "s^have_ssl=false^have_ssl=true^" ${PROXY_CFG}
	sed_in_place "s^ssl_p2s_ca=\"\"^ssl_p2s_ca=\"$CA\"^" ${PROXY_CFG}
	sed_in_place "s^ssl_p2s_ca=\"\"^ssl_p2s_ca=\"$CA\"^" ${PROXY_CFG}
	sed_in_place "s^ssl_p2s_key=\"\"^ssl_p2s_key=\"$KEY\"^" ${PROXY_CFG}
	sed_in_place "s^ssl_p2s_cert=\"\"^ssl_p2s_cert=\"$CERT\"^" ${PROXY_CFG}

	# Percona scheduler
	if [[ -f ${PERCONA_SCHEDULER_CFG} ]]; then
		sed_in_place "s:^sslCa.*=.*\"$:sslCa = \"${CA##*/}\":" ${PERCONA_SCHEDULER_CFG}
		sed_in_place "s:^sslKey.*=.*\"$:sslKey = \"${KEY##*/}\":" ${PERCONA_SCHEDULER_CFG}
		sed_in_place "s:^sslClient.*=.*\"$:sslClient = \"${CERT##*/}\":" ${PERCONA_SCHEDULER_CFG}
	fi
fi

if [ -f "${SSL_DIR}/tls.key" ] && [ -f "${SSL_DIR}/tls.crt" ]; then
	cp "${SSL_DIR}/tls.key" /var/lib/proxysql/proxysql-key.pem
	cp "${SSL_DIR}/tls.crt" /var/lib/proxysql/proxysql-cert.pem
fi
if [ -f "${SSL_DIR}/ca.crt" ]; then
	cp "${SSL_DIR}/ca.crt" /var/lib/proxysql/proxysql-ca.pem
fi

test -e /opt/percona/hookscript/hook.sh && source /opt/percona/hookscript/hook.sh

exec "$@"
