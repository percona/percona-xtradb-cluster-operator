#!/bin/bash

set -o xtrace

cp /opt/percona/proxysql.cnf /etc/proxysql
cp /opt/percona/proxysql-admin.cnf /etc

PROXY_CFG=/etc/proxysql/proxysql.cnf
PROXY_ADMIN_CFG=/etc/proxysql-admin.cnf

MYSQL_INTERFACES='0.0.0.0:3306;0.0.0.0:33062'
CLUSTER_PORT='33062'
sed "s/#export WRITERS_ARE_READERS=.*$/export WRITERS_ARE_READERS='yes'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}

sed "s/interfaces=\"0.0.0.0:3306\"/interfaces=\"${MYSQL_INTERFACES:-0.0.0.0:3306}\"/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/stacksize=1048576/stacksize=${MYSQL_STACKSIZE:-1048576}/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/threads=2/threads=${MYSQL_THREADS:-2}/g" ${PROXY_CFG} 1<>${PROXY_CFG}

set +o xtrace # hide sensitive information
OPERATOR_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${OPERATOR_PASSWORD}")
MONITOR_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${MONITOR_PASSWORD}")
PROXY_ADMIN_PASSWORD_ESCAPED=$(sed 's/[][\-\!\#\$\%\&\(\)\*\+\,\.\:\;\<\=\>\?\@\^\_\~\{\}]/\\&/g' <<<"${PROXY_ADMIN_PASSWORD}")

sed "s/\"admin:admin\"/\"${PROXY_ADMIN_USER:-admin}:${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}\"/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/cluster_username=\"admin\"/cluster_username=\"${PROXY_ADMIN_USER:-admin}\"/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/cluster_password=\"admin\"/cluster_password=\"${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}\"/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/monitor_password=\"monitor\"/monitor_password=\"${MONITOR_PASSWORD_ESCAPED:-monitor}\"/g" ${PROXY_CFG} 1<>${PROXY_CFG}
sed "s/PROXYSQL_USERNAME='admin'/PROXYSQL_USERNAME='${PROXY_ADMIN_USER:-admin}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/PROXYSQL_PASSWORD='admin'/PROXYSQL_PASSWORD='${PROXY_ADMIN_PASSWORD_ESCAPED:-admin}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/CLUSTER_USERNAME='admin'/CLUSTER_USERNAME='${OPERATOR_USERNAME:-operator}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/CLUSTER_PASSWORD='admin'/CLUSTER_PASSWORD='${OPERATOR_PASSWORD_ESCAPED:-operator}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/CLUSTER_PORT='3306'/CLUSTER_PORT='${CLUSTER_PORT:-3306}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/MONITOR_USERNAME='monitor'/MONITOR_USERNAME='${MONITOR_USERNAME:-monitor}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
sed "s/MONITOR_PASSWORD='monitor'/MONITOR_PASSWORD='${MONITOR_PASSWORD_ESCAPED:-monitor}'/g" ${PROXY_ADMIN_CFG} 1<>${PROXY_ADMIN_CFG}
set -o xtrace

## SSL/TLS support
CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
if [ -f "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt" ]; then
	CA=/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
fi
SSL_DIR=${SSL_DIR:-/etc/proxysql/ssl}
if [ -f "${SSL_DIR}/ca.crt" ]; then
	CA=${SSL_DIR}/ca.crt
fi
SSL_INTERNAL_DIR=${SSL_INTERNAL_DIR:-/etc/proxysql/ssl-internal}
if [ -f "${SSL_INTERNAL_DIR}/ca.crt" ]; then
	CA=${SSL_INTERNAL_DIR}/ca.crt
fi

KEY=${SSL_DIR}/tls.key
CERT=${SSL_DIR}/tls.crt
if [ -f "${SSL_INTERNAL_DIR}/tls.key" ] && [ -f "${SSL_INTERNAL_DIR}/tls.crt" ]; then
	KEY=${SSL_INTERNAL_DIR}/tls.key
	CERT=${SSL_INTERNAL_DIR}/tls.crt
fi

if [ -f "$CA" ] && [ -f "$KEY" ] && [ -f "$CERT" ] && [ -n "$PXC_SERVICE" ]; then
	sed "s^have_ssl=false^have_ssl=true^" ${PROXY_CFG} 1<>${PROXY_CFG}
	sed "s^ssl_p2s_ca=\"\"^ssl_p2s_ca=\"$CA\"^" ${PROXY_CFG} 1<>${PROXY_CFG}
	sed "s^ssl_p2s_ca=\"\"^ssl_p2s_ca=\"$CA\"^" ${PROXY_CFG} 1<>${PROXY_CFG}
	sed "s^ssl_p2s_key=\"\"^ssl_p2s_key=\"$KEY\"^" ${PROXY_CFG} 1<>${PROXY_CFG}
	sed "s^ssl_p2s_cert=\"\"^ssl_p2s_cert=\"$CERT\"^" ${PROXY_CFG} 1<>${PROXY_CFG}
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
