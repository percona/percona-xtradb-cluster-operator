package pxctls

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Issue returns CA certificate, TLS certificate and TLS private key
func Issue(hosts []string) (caCert []byte, tlsCert []byte, tlsKey []byte, err error) {
	rsaBits := 2048
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate rsa key: %v", err)
	}
	subject := pkix.Name{
		Organization: []string{"Root CA"},
	}
	issuer := pkix.Name{
		Organization: []string{"Root CA"},
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      subject,
		Issuer:       issuer,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180),
		DNSNames:     hosts,
		IsCA:         true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate CA certificate: %v", err)
	}
	certOut := &bytes.Buffer{}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode CA certificate: %v", err)
	}
	cert := certOut.Bytes()

	keyOut := &bytes.Buffer{}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	err = pem.Encode(keyOut, block)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode RSA private key: %v", err)
	}
	privKey := keyOut.Bytes()

	template = x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      subject,
		Issuer:       issuer,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180),
		DNSNames:     hosts,
		IsCA:         true,
	}
	tlsDerBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, nil, err
	}
	tlsCertOut := &bytes.Buffer{}
	err = pem.Encode(tlsCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: tlsDerBytes})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode TLS  certificate: %v", err)
	}
	tlsCert = tlsCertOut.Bytes()

	return cert, tlsCert, privKey, nil
}
