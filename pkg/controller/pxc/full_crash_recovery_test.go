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
			cr:     withRecovery(invalidUUID, 10),
			uuid:   clusterUUID,
			seqno:  11,
			wantOK: true,
		},
		"both uuids uninitialized (fresh clusters), seqno advanced": {
			cr:     withRecovery(uninitializedUUID, -1),
			uuid:   uninitializedUUID,
			seqno:  -1,
			wantOK: true,
		},
		"uninitialized current with known status uuid": {
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

func TestEvaluateRecoveryScan(t *testing.T) {
	tests := []struct {
		name                    string
		results                 []podScanResult
		expectedAction          recoveryAction
		expectedMaxSeqPod       string
		expectedUnavailablePods []string
	}{
		{
			name: "all pods scannable and waiting - proceed with highest seq",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 100}, err: nil},
				{name: "cluster-pxc-1", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 200}, err: nil},
				{name: "cluster-pxc-2", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 150}, err: nil},
			},
			expectedAction:    recoveryProceed,
			expectedMaxSeqPod: "cluster-pxc-1",
		},
		{
			name: "single pod scannable with highest seq still wins",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 300}, err: nil},
				{name: "cluster-pxc-1", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 100}, err: nil},
				{name: "cluster-pxc-2", waiting: true, info: podRecoveryInfo{uuid: "uuid-a", seq: 200}, err: nil},
			},
			expectedAction:    recoveryProceed,
			expectedMaxSeqPod: "cluster-pxc-0",
		},
		{
			name: "one pod not waiting - not a full crash",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{seq: 100}, err: nil},
				{name: "cluster-pxc-1", waiting: false, info: podRecoveryInfo{}, err: nil},
				{name: "cluster-pxc-2", waiting: true, info: podRecoveryInfo{seq: 200}, err: nil},
			},
			expectedAction: recoveryNotFullCrash,
		},
		{
			name: "one pod log failure - blocked because unavailable pod might be newer",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{seq: 100}, err: nil},
				{name: "cluster-pxc-1", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
				{name: "cluster-pxc-2", waiting: true, info: podRecoveryInfo{seq: 150}, err: nil},
			},
			expectedAction:          recoveryBlocked,
			expectedMaxSeqPod:       "cluster-pxc-2",
			expectedUnavailablePods: []string{"cluster-pxc-1"},
		},
		{
			name: "all pods failed to scan",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
				{name: "cluster-pxc-1", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
				{name: "cluster-pxc-2", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
			},
			expectedAction:          recoveryAllFailed,
			expectedUnavailablePods: []string{"cluster-pxc-0", "cluster-pxc-1", "cluster-pxc-2"},
		},
		{
			name: "two pods failed - blocked",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{seq: 50}, err: nil},
				{name: "cluster-pxc-1", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
				{name: "cluster-pxc-2", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
			},
			expectedAction:          recoveryBlocked,
			expectedMaxSeqPod:       "cluster-pxc-0",
			expectedUnavailablePods: []string{"cluster-pxc-1", "cluster-pxc-2"},
		},
		{
			name: "single pod cluster - all scannable",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: true, info: podRecoveryInfo{seq: 42}, err: nil},
			},
			expectedAction:    recoveryProceed,
			expectedMaxSeqPod: "cluster-pxc-0",
		},
		{
			name: "single pod cluster - scan fails",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: false, info: podRecoveryInfo{}, err: assert.AnError},
			},
			expectedAction:          recoveryAllFailed,
			expectedUnavailablePods: []string{"cluster-pxc-0"},
		},
		{
			name: "first pod not waiting returns immediately",
			results: []podScanResult{
				{name: "cluster-pxc-0", waiting: false, info: podRecoveryInfo{}, err: nil},
				{name: "cluster-pxc-1", waiting: true, info: podRecoveryInfo{seq: 200}, err: nil},
			},
			expectedAction: recoveryNotFullCrash,
		},
		{
			name:           "empty results treated as all failed",
			results:        []podScanResult{},
			expectedAction: recoveryAllFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := evaluateRecoveryScan(tt.results)

			assert.Equal(t, tt.expectedAction, decision.action, "recovery action mismatch")

			if tt.expectedMaxSeqPod != "" {
				assert.Equal(t, tt.expectedMaxSeqPod, decision.maxSeqPod, "max seq pod mismatch")
			}

			if tt.expectedUnavailablePods != nil {
				assert.Equal(t, tt.expectedUnavailablePods, decision.unavailablePods, "unavailable pods mismatch")
			}
		})
	}
}
