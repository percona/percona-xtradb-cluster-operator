package pxctls

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidityNotAfter(t *testing.T) {
	previousStartTime := operatorStartTime
	operatorStartTime = time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	t.Cleanup(func() {
		operatorStartTime = previousStartTime
	})

	notBefore := time.Date(2021, time.February, 3, 4, 5, 6, 0, time.UTC)

	got, want := caValidityNotAfter(false, notBefore), operatorStartTime.Add(DefaultCAValidity)
	assert.Equal(t, want, got, "legacy ca notAfter mismatch")

	got, want = caValidityNotAfter(true, notBefore), notBefore.Add(DefaultCAValidity)
	assert.Equal(t, want, got, "ca notAfter mismatch")

	got, want = certValidityNotAfter(false, notBefore), operatorStartTime.Add(DefaultCertValidity)
	assert.Equal(t, want, got, "legacy cert notAfter mismatch")

	got, want = certValidityNotAfter(true, notBefore), notBefore.Add(DefaultCertValidity)
	assert.Equal(t, want, got, "cert notAfter mismatch")
}

func TestIssue(t *testing.T) {
	t.Run("notAfter", func(t *testing.T) {
		previousStartTime := operatorStartTime
		operatorStartTime = time.Now().Add(-48 * time.Hour)
		t.Cleanup(func() {
			operatorStartTime = previousStartTime
		})

		caPEM, tlsPEM, _, err := Issue([]string{"percona.com"}, true, true)
		require.NoError(t, err, "issue certs")

		parseCert := func(t *testing.T, certPEM []byte) *x509.Certificate {
			t.Helper()

			block, _ := pem.Decode(certPEM)
			require.NotNil(t, block, "decode cert pem")

			cert, err := x509.ParseCertificate(block.Bytes)
			require.NoError(t, err, "parse cert")

			return cert
		}

		caCert := parseCert(t, caPEM)
		tlsCert := parseCert(t, tlsPEM)

		assertDuration := func(t *testing.T, got, want time.Duration, name string) {
			t.Helper()

			const drift = 2 * time.Second
			assert.InDelta(t, want, got, float64(drift), "%s duration mismatch", name)
		}

		assertDuration(t, caCert.NotAfter.Sub(caCert.NotBefore), DefaultCAValidity, "ca")
		assertDuration(t, tlsCert.NotAfter.Sub(tlsCert.NotBefore), DefaultCertValidity, "tls")
	})
}
