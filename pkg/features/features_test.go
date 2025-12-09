package features

import (
	"context"
	"testing"

	"gotest.tools/assert"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	gate := NewGate()

	assert.Assert(t, false == gate.Enabled(XtrabackupSidecar))
}

func TestStringFormat(t *testing.T) {
	t.Parallel()
	gate := NewGate()

	assert.NilError(t, gate.Set(""))
	assert.NilError(t, gate.Set("XtrabackupSidecar=true"))
	assert.Assert(t, true == gate.Enabled(XtrabackupSidecar))

}

func TestContext(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	ctx := NewContextWithGate(context.Background(), gate)

	assert.Equal(t, ShowAssigned(ctx), "")

	assert.NilError(t, gate.Set("XtrabackupSidecar=true"))
	assert.Assert(t, Enabled(ctx, XtrabackupSidecar))
	assert.Equal(t, ShowAssigned(ctx), "XtrabackupSidecar=true")
}
