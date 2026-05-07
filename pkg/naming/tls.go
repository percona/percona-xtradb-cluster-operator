package naming

import (
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

// Cert-manager Certificate and Issuer name suffixes managed by the operator.
const (
	suffixCAIssuer          = "-pxc-ca-issuer"
	suffixIssuer            = "-pxc-issuer"
	suffixCACertificate     = "-ca-cert"
	suffixSSLCertificate    = "-ssl"
	suffixSSLIntCertificate = "-ssl-internal"
)

// CAIssuerName returns the name of the self-signed cert-manager Issuer used to
// sign the cluster's CA certificate.
func CAIssuerName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + suffixCAIssuer
}

// IssuerName returns the name of the cert-manager Issuer used to sign the
// cluster's leaf TLS certificates.
func IssuerName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + suffixIssuer
}

// CACertificateName returns the name of the cert-manager Certificate (and the
// Secret it produces) holding the cluster's CA.
func CACertificateName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + suffixCACertificate
}

// SSLCertificateName returns the name of the cert-manager Certificate that
// produces the cluster's external/leaf TLS Secret.
func SSLCertificateName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + suffixSSLCertificate
}

// SSLInternalCertificateName returns the name of the cert-manager Certificate
// that produces the cluster's internal TLS Secret.
func SSLInternalCertificateName(cr *api.PerconaXtraDBCluster) string {
	return cr.Name + suffixSSLIntCertificate
}
