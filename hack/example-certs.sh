#!/usr/bin/env bash
#
# Regenerates the example TLS secrets in deploy/ssl-secrets.yaml.
#
# Usage:
#   hack/example-certs.sh              always regenerate
#   hack/example-certs.sh --if-needed  regenerate only if the existing secrets
#                                      are invalid or expire soon
#
# Each secret gets its own CA, as in the certificates the operator generates.

set -o errexit
set -o nounset
set -o pipefail

OUTPUT_FILE="${OUTPUT_FILE:-deploy/ssl-secrets.yaml}"
CLUSTER_NAME="${CLUSTER_NAME:-cluster1}"
VALIDITY_DAYS="${VALIDITY_DAYS:-5110}"          # ~14 years
MIN_REMAINING_DAYS="${MIN_REMAINING_DAYS:-365}" # regenerate when less is left

SECRETS=("${CLUSTER_NAME}-ssl" "${CLUSTER_NAME}-ssl-internal")

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

b64() {
	base64 <"$1" | tr -d '\n'
}

# Prints the base64 value of <key> from the secret named <name> in OUTPUT_FILE.
secret_field() {
	local name="$1" key="$2"
	awk -v name="${name}" -v key="${key}" '
		function flush() { if (nm == name && val != "") print val; nm = ""; val = "" }
		/^---$/ { flush(); next }
		$1 == "name:" { nm = $2 }
		$1 == key ":" { val = $2 }
		END { flush() }
	' "${OUTPUT_FILE}"
}

# Returns 0 when the secret holds a valid, non-expiring cert chain whose key matches.
secret_is_valid() {
	local name="$1" dir="${workdir}/check-${1}" field
	mkdir -p "${dir}"
	for field in ca.crt tls.crt tls.key; do
		secret_field "${name}" "${field}" | base64 -d >"${dir}/${field}" 2>/dev/null || return 1
		[[ -s "${dir}/${field}" ]] || return 1
	done

	openssl verify -CAfile "${dir}/ca.crt" "${dir}/tls.crt" >/dev/null 2>&1 || return 1
	openssl x509 -in "${dir}/tls.crt" -noout -checkend $((MIN_REMAINING_DAYS * 86400)) >/dev/null 2>&1 || return 1

	local cert_pub key_pub
	cert_pub="$(openssl x509 -in "${dir}/tls.crt" -noout -pubkey 2>/dev/null)" || return 1
	key_pub="$(openssl pkey -in "${dir}/tls.key" -pubout 2>/dev/null)" || return 1
	[[ ${cert_pub} == "${key_pub}" ]] || return 1
}

all_secrets_valid() {
	[[ -f ${OUTPUT_FILE} ]] || return 1
	local name
	for name in "${SECRETS[@]}"; do
		secret_is_valid "${name}" || {
			echo "example-certs: ${name} is invalid or expires within ${MIN_REMAINING_DAYS} days" >&2
			return 1
		}
	done
}

# Generates <dir>/{ca.crt,tls.crt,tls.key}
generate_pair() {
	local dir="$1"
	mkdir -p "${dir}"

	cat >"${dir}/ca.cnf" <<-EOF
		[req]
		distinguished_name = dn
		prompt = no
		x509_extensions = ext
		[dn]
		CN = Root CA
		[ext]
		basicConstraints = critical,CA:TRUE
		keyUsage = critical,keyCertSign,cRLSign
		subjectKeyIdentifier = hash
	EOF

	cat >"${dir}/leaf.cnf" <<-EOF
		[ext]
		basicConstraints = critical,CA:FALSE
		keyUsage = critical,digitalSignature,keyEncipherment
		extendedKeyUsage = serverAuth,clientAuth
		subjectKeyIdentifier = hash
		authorityKeyIdentifier = keyid
		subjectAltName = DNS:${CLUSTER_NAME}-proxysql,DNS:*.${CLUSTER_NAME}-proxysql-unready,DNS:*.${CLUSTER_NAME}-pxc
	EOF

	openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days "${VALIDITY_DAYS}" \
		-config "${dir}/ca.cnf" -keyout "${dir}/ca.key" -out "${dir}/ca.crt" 2>/dev/null

	openssl req -newkey rsa:2048 -nodes -sha256 \
		-subj "/CN=${CLUSTER_NAME}-pxc" -keyout "${dir}/tls.key" -out "${dir}/tls.csr" 2>/dev/null

	openssl x509 -req -sha256 -days "${VALIDITY_DAYS}" -in "${dir}/tls.csr" \
		-CA "${dir}/ca.crt" -CAkey "${dir}/ca.key" -CAcreateserial \
		-extfile "${dir}/leaf.cnf" -extensions ext -out "${dir}/tls.crt" 2>/dev/null

	# Always PKCS#1 ("BEGIN RSA PRIVATE KEY"), whatever the openssl version defaults to.
	openssl rsa -in "${dir}/tls.key" -traditional -out "${dir}/tls.key.pkcs1" 2>/dev/null
	mv "${dir}/tls.key.pkcs1" "${dir}/tls.key"

	openssl verify -CAfile "${dir}/ca.crt" "${dir}/tls.crt" >/dev/null
}

write_secret() {
	local name="$1" dir="$2"
	cat <<-EOF
		apiVersion: v1
		data:
		  ca.crt: $(b64 "${dir}/ca.crt")
		  tls.crt: $(b64 "${dir}/tls.crt")
		  tls.key: $(b64 "${dir}/tls.key")
		kind: Secret
		metadata:
		  name: ${name}
		type: kubernetes.io/tls
	EOF
}

regenerate() {
	local out="${workdir}/ssl-secrets.yaml" name first=1
	: >"${out}"
	for name in "${SECRETS[@]}"; do
		generate_pair "${workdir}/${name}"
		[[ ${first} -eq 1 ]] || echo "---" >>"${out}"
		first=0
		write_secret "${name}" "${workdir}/${name}" >>"${out}"
	done
	mv "${out}" "${OUTPUT_FILE}"
	echo "example-certs: regenerated ${OUTPUT_FILE}"
}

main() {
	if [[ ${1:-} == "--if-needed" ]] && all_secrets_valid; then
		echo "example-certs: ${OUTPUT_FILE} is up to date, skipping"
		return 0
	fi
	regenerate
}

main "$@"
