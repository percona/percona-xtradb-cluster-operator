package pxc

import (
	"io"
	"strings"
	"testing"

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
	if idx == -1 {
		t.Fatal("we can delete this test if passSymbols doesn't contain '*'")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(p), "*") {
		t.Fatal("expected '*' prefix when no rules are applied to the password")
	}

	p, err = generatePass(users.ProxyAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(p), "*") {
		t.Fatal("unexpected '*' prefix: proxyadmin passwords should not include it")
	}
}
