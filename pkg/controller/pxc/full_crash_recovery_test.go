package pxc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

func TestParseRecoveredPosition(t *testing.T) {
	const marker = `#####################################################LAST_LINE`
	const tail = `#####################################################`

	tests := map[string]struct {
		log       string
		wantUUID  string
		wantSeq   int64
		errSubstr string
	}{
		"new format with uuid and seqno": {
			log:      marker + ":cluster1-pxc-0:3f1b9c4e-1111-2222-3333-444455556666:42:" + tail,
			wantUUID: "3f1b9c4e-1111-2222-3333-444455556666",
			wantSeq:  42,
		},
		"new format with uninitialized uuid": {
			log:      marker + ":cluster1-pxc-1:00000000-0000-0000-0000-000000000000:-1:" + tail,
			wantUUID: uninitializedUUID,
			wantSeq:  -1,
		},
		"legacy format without uuid": {
			log:      marker + ":cluster1-pxc-0:42:" + tail,
			wantUUID: invalidUUID,
			wantSeq:  42,
		},
		"legacy format with -1 seqno": {
			log:      marker + ":cluster1-pxc-2:-1:" + tail,
			wantUUID: invalidUUID,
			wantSeq:  -1,
		},
		"too few fields": {
			log:       marker + ":cluster1-pxc-0:" + tail,
			wantUUID:  invalidUUID,
			wantSeq:   invalidSeqno,
			errSubstr: "invalid log format",
		},
		"too many fields": {
			log:       marker + ":cluster1-pxc-0:uuid:42:extra:" + tail,
			wantUUID:  invalidUUID,
			wantSeq:   invalidSeqno,
			errSubstr: "invalid log format",
		},
		"non-numeric seqno in new format": {
			log:       marker + ":cluster1-pxc-0:3f1b9c4e-1111-2222-3333-444455556666:notanumber:" + tail,
			wantUUID:  "3f1b9c4e-1111-2222-3333-444455556666",
			wantSeq:   invalidSeqno,
			errSubstr: "parse sequence",
		},
		"non-numeric seqno in legacy format": {
			log:       marker + ":cluster1-pxc-0:notanumber:" + tail,
			wantUUID:  invalidUUID,
			wantSeq:   invalidSeqno,
			errSubstr: "parse sequence",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			uuid, seq, err := parseRecoveredPosition(tt.log)
			if tt.errSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantUUID, uuid)
			assert.Equal(t, tt.wantSeq, seq)
		})
	}
}

func TestIsAutomaticRecoverySafe(t *testing.T) {
	const clusterUUID = "3f1b9c4e-1111-2222-3333-444455556666"
	const otherUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	withRecovery := func(uuid string, seqno int64) *pxcv1.PerconaXtraDBCluster {
		return &pxcv1.PerconaXtraDBCluster{
			Status: pxcv1.PerconaXtraDBClusterStatus{
				Recovery: &pxcv1.RecoveryStatus{
					ClusterUUID:       uuid,
					LastRecoverySeqNo: seqno,
				},
			},
		}
	}

	tests := map[string]struct {
		cr     *pxcv1.PerconaXtraDBCluster
		uuid   string
		seqno  int64
		wantOK bool
	}{
		"first recovery, status not set": {
			cr:     &pxcv1.PerconaXtraDBCluster{},
			uuid:   clusterUUID,
			seqno:  10,
			wantOK: true,
		},
		"first recovery with invalid uuid and seqno": {
			cr:     &pxcv1.PerconaXtraDBCluster{},
			uuid:   invalidUUID,
			seqno:  invalidSeqno,
			wantOK: true,
		},
		"same uuid, seqno advanced": {
			cr:     withRecovery(clusterUUID, 10),
			uuid:   clusterUUID,
			seqno:  11,
			wantOK: true,
		},
		"same uuid, seqno equal (no progress)": {
			cr:     withRecovery(clusterUUID, 10),
			uuid:   clusterUUID,
			seqno:  10,
			wantOK: true,
		},
		"same uuid, seqno regressed": {
			cr:     withRecovery(clusterUUID, 10),
			uuid:   clusterUUID,
			seqno:  5,
			wantOK: false,
		},
		"uuid mismatch with advanced seqno": {
			cr:     withRecovery(clusterUUID, 10),
			uuid:   otherUUID,
			seqno:  100,
			wantOK: false,
		},
		"unknown current uuid with known status uuid, seqno advanced": {
			// Rolling-upgrade case: previous recovery saw a real UUID,
			// current scan reads a legacy-format log and can't determine UUID.
			// Fall back to seqno-only check.
			cr:     withRecovery(clusterUUID, 10),
			uuid:   invalidUUID,
			seqno:  11,
			wantOK: true,
		},
		"unknown current uuid with known status uuid, seqno regressed": {
			cr:     withRecovery(clusterUUID, 10),
			uuid:   invalidUUID,
			seqno:  5,
			wantOK: false,
		},
		"known current uuid with unknown status uuid, seqno advanced": {
			// Status was recorded during rolling upgrade with no UUID, now
			// pods report real UUIDs.
			cr:     withRecovery(invalidUUID, 10),
			uuid:   clusterUUID,
			seqno:  11,
			wantOK: true,
		},
		"both uuids uninitialized (fresh clusters), seqno advanced": {
			// Two fresh-grastate recoveries should not be falsely identified
			// as the same cluster, but with seqno -1 on both sides the seqno
			// check still permits the harmless bootstrap.
			cr:     withRecovery(uninitializedUUID, -1),
			uuid:   uninitializedUUID,
			seqno:  -1,
			wantOK: true,
		},
		"uninitialized current with known status uuid": {
			// PVCs wiped, fresh cluster comes up — entrypoint emits all-zeros.
			// Status has the real previous UUID. Treat current as unknown,
			// fall back to seqno: if seqno is -1 vs 100, regression → unsafe.
			cr:     withRecovery(clusterUUID, 100),
			uuid:   uninitializedUUID,
			seqno:  -1,
			wantOK: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := isAutomaticRecoverySafe(tt.cr, tt.uuid, tt.seqno)
			assert.Equal(t, tt.wantOK, got)
		})
	}
}
