package features

import (
	"context"
	"testing"

	"gotest.tools/assert"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	gate := NewGate()

	assert.Assert(t, false == gate.Enabled(BackupSidecar))
}

func TestStringFormat(t *testing.T) {
	t.Parallel()
	gate := NewGate()

	assert.NilError(t, gate.Set(""))
	assert.NilError(t, gate.Set("BackupSidecar=true"))
	assert.Assert(t, true == gate.Enabled(BackupSidecar))

}

func TestContext(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	ctx := NewContextWithGate(context.Background(), gate)

	assert.Equal(t, ShowAssigned(ctx), "")

	assert.NilError(t, gate.Set("BackupSidecar=true"))
	assert.Assert(t, Enabled(ctx, BackupSidecar))
	assert.Equal(t, ShowAssigned(ctx), "BackupSidecar=true")
}
