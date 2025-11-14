package pxc

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

type repeatingReader struct {
	pattern []byte
	pos     int
	reads   int
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if len(r.pattern) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	for i := range p {
		p[i] = r.pattern[r.pos]
		r.pos = (r.pos + 1) % len(r.pattern)
	}
	r.reads++
	if r.reads > 10000 {
		panic("too many reads: likely stuck in crypto/rand.Int retry loop. Try using a different pattern that produces values < max")
	}
	return len(p), nil
}

func TestGeneratePassProxyadmin(t *testing.T) {
	idx := strings.Index(passSymbols, "*")
	require.NotEqual(t, -1, idx, "we can delete this test if passSymbols doesn't contain '*'")
	randReader = &repeatingReader{
		pattern: []byte{
			byte(idx),
			1,
			2,
			3,
			4,
			5,
			6,
			7,
			8,
		},
	}

	p, err := generatePass("")
	require.NoError(t, err)
	assert.Equal(t, true, strings.HasPrefix(string(p), "*"), "expected '*' prefix when no rules are applied to the password")

	p, err = generatePass(users.ProxyAdmin)
	require.NoError(t, err)
	assert.Equal(t, false, strings.HasPrefix(string(p), "*"), "unexpected '*' prefix: proxyadmin passwords should not include it")
}
