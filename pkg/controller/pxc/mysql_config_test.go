package pxc

import (
	"context"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/apis"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
)

type fakeGlobalVariables struct {
	setErrs  map[string]error
	current  map[string]string
	getErr   error
	closeErr error
	set      map[string]string
	closed   bool
}

func (f *fakeGlobalVariables) SetGlobalVariable(name, value string) error {
	if err, ok := f.setErrs[name]; ok {
		return err
	}
	if f.set == nil {
		f.set = make(map[string]string)
	}
	f.set[name] = value
	return nil
}

func (f *fakeGlobalVariables) GlobalVariable(name string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.current[name], nil
}

func (f *fakeGlobalVariables) Close() error {
	f.closed = true
	return f.closeErr
}

func connectTo(db globalVariables, err error) globalVariablesConnector {
	return func(context.Context, corev1.Pod) (globalVariables, error) {
		return db, err
	}
}

func mysqlErr(number uint16) error {
	return &mysql.MySQLError{Number: number, Message: "test"}
}

func TestSetPodGlobalVariables(t *testing.T) {
	errClose := errors.New("close failed")
	errConnect := errors.New("connection refused")
	errDriver := errors.New("driver: bad connection")

	conf := config.MySQLdConfig{
		"max_connections":               "200",
		"innodb_buffer_pool_chunk_size": "128M",
		"loose_audit_log_policy":        "ALL",
		"plugin_load_add":               "audit_log.so",
		"wsrep_provider_options":        "gcs.fc_limit=73;gcache.size=128M",
	}

	tests := map[string]struct {
		options     []string
		db          *fakeGlobalVariables
		connectErr  error
		wantRestart bool
		wantSet     map[string]string
		wantErr     error
		wantErrMsg  string
	}{
		"dynamic options": {
			options: []string{"max_connections", "wsrep_provider_options"},
			db:      &fakeGlobalVariables{},
			wantSet: map[string]string{
				"max_connections":        "200",
				"wsrep_provider_options": "'gcs.fc_limit=73;gcache.size=128M'",
			},
		},
		"read-only option already holding the value": {
			options: []string{"innodb_buffer_pool_chunk_size"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"innodb_buffer_pool_chunk_size": mysqlErr(errReadOnlyVariable)},
				current: map[string]string{"innodb_buffer_pool_chunk_size": "134217728"},
			},
		},
		"read-only option with another value": {
			options: []string{"innodb_buffer_pool_chunk_size"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"innodb_buffer_pool_chunk_size": mysqlErr(errReadOnlyVariable)},
				current: map[string]string{"innodb_buffer_pool_chunk_size": "1048576"},
			},
			wantRestart: true,
		},
		"read-only option that cannot be read back": {
			options: []string{"innodb_buffer_pool_chunk_size"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"innodb_buffer_pool_chunk_size": mysqlErr(errReadOnlyVariable)},
				getErr:  errDriver,
			},
			wantRestart: true,
		},
		"provider options rejected but held": {
			options: []string{"wsrep_provider_options"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"wsrep_provider_options": mysqlErr(errIncorrectArguments)},
				current: map[string]string{"wsrep_provider_options": "gcache.size = 134217728; gcs.fc_limit = 73; pc.weight = 10"},
			},
		},
		"provider options rejected and not held": {
			options: []string{"wsrep_provider_options"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"wsrep_provider_options": mysqlErr(errIncorrectArguments)},
				current: map[string]string{"wsrep_provider_options": "gcache.size = 128M; gcs.fc_limit = 16"},
			},
			wantRestart: true,
		},
		"unknown loose option": {
			options: []string{"loose_audit_log_policy"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"audit_log_policy": mysqlErr(errUnknownVariable)},
			},
		},
		"startup option without a variable": {
			options: []string{"plugin_load_add"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"plugin_load_add": mysqlErr(errUnknownVariable)},
			},
			wantRestart: true,
		},
		"incorrect arguments for another option": {
			options: []string{"max_connections"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"max_connections": mysqlErr(errIncorrectArguments)},
			},
			wantErrMsg: "set max_connections",
		},
		"non mysql error": {
			options: []string{"max_connections"},
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"max_connections": errDriver},
			},
			wantErr: errDriver,
		},
		"connection fails": {
			options:    []string{"max_connections"},
			connectErr: errConnect,
			wantErr:    errConnect,
		},
		"close fails": {
			options: []string{"max_connections"},
			db:      &fakeGlobalVariables{closeErr: errClose},
			wantErr: errClose,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var db globalVariables
			if tc.db != nil {
				db = tc.db
			}
			pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc-0"}}

			restart, err := setPodGlobalVariables(t.Context(), pod, connectTo(db, tc.connectErr), conf, tc.options)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assert.False(t, restart)
				return
			case tc.wantErrMsg != "":
				require.ErrorContains(t, err, tc.wantErrMsg)
				assert.False(t, restart)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRestart, restart)
			assert.Equal(t, tc.wantSet, tc.db.set)
			assert.True(t, tc.db.closed)
		})
	}
}

func TestSetGlobalVariablesAcrossPods(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc-1"}},
	}
	conf := config.MySQLdConfig{"innodb_purge_threads": "4"}

	dbs := map[string]*fakeGlobalVariables{
		"cluster1-pxc-0": {current: map[string]string{"innodb_purge_threads": "4"}},
		"cluster1-pxc-1": {current: map[string]string{"innodb_purge_threads": "1"}},
	}
	for _, db := range dbs {
		db.setErrs = map[string]error{"innodb_purge_threads": mysqlErr(errReadOnlyVariable)}
	}
	connect := func(_ context.Context, pod corev1.Pod) (globalVariables, error) {
		return dbs[pod.Name], nil
	}

	restart, err := setGlobalVariables(t.Context(), pods, connect, conf, []string{"innodb_purge_threads"})
	require.NoError(t, err)
	assert.True(t, restart)

	failing := func(_ context.Context, pod corev1.Pod) (globalVariables, error) {
		return nil, errors.New("refused")
	}
	_, err = setGlobalVariables(t.Context(), pods, failing, conf, []string{"innodb_purge_threads"})
	require.ErrorContains(t, err, "pod cluster1-pxc-0")
}

func TestRolloutInProgress(t *testing.T) {
	tests := map[string]struct {
		sts  appsv1.StatefulSet
		want bool
	}{
		"rolled out": {
			sts: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.StatefulSetSpec{Replicas: new(int32(3))},
				Status:     appsv1.StatefulSetStatus{ObservedGeneration: 2, UpdatedReplicas: 3},
			},
		},
		"spec not observed yet": {
			sts: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.StatefulSetSpec{Replicas: new(int32(3))},
				Status:     appsv1.StatefulSetStatus{ObservedGeneration: 2, UpdatedReplicas: 3},
			},
			want: true,
		},
		"pods left to replace": {
			sts: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.StatefulSetSpec{Replicas: new(int32(3))},
				Status:     appsv1.StatefulSetStatus{ObservedGeneration: 2, UpdatedReplicas: 1},
			},
			want: true,
		},
		"replicas default to one": {
			sts:  appsv1.StatefulSet{Status: appsv1.StatefulSetStatus{UpdatedReplicas: 1}},
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, rolloutInProgress(&tc.sts))
		})
	}
}

func newMySQLConfigCR() *api.PerconaXtraDBCluster {
	cr := newCR("cluster1", "pxc")
	cr.Spec.CRVersion = liveConfigVersion
	cr.Spec.PXC.Configuration = "[mysqld]\nmax_connections=200"
	cr.Status.PXC.Status = api.AppStateReady
	return cr
}

func newMySQLConfigSts(cr *api.PerconaXtraDBCluster, applied config.MySQLdConfig) *appsv1.StatefulSet {
	sts := statefulset.NewNode(cr).StatefulSet()
	sts.Spec.Replicas = new(cr.Spec.PXC.Size)
	sts.Status.UpdatedReplicas = cr.Spec.PXC.Size
	if applied != nil {
		data, err := applied.JSON()
		if err != nil {
			panic(err)
		}
		sts.Annotations = map[string]string{lastAppliedMySQLConfigAnnotation: data}
	}
	return sts
}

func newReadyPXCPods(cr *api.PerconaXtraDBCluster, ready bool) []client.Object {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	var pods []client.Object
	for _, name := range []string{"cluster1-pxc-0", "cluster1-pxc-1", "cluster1-pxc-2"} {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cr.Namespace, Labels: naming.LabelsPXC(cr)},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
			},
		})
	}
	return pods
}

func newConfigReconciler(t *testing.T, objs ...client.Object) *ReconcilePerconaXtraDBCluster {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apis.AddToScheme(s))

	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return &ReconcilePerconaXtraDBCluster{client: cl, apiReader: cl, scheme: s, recorder: new(record.FakeRecorder)}
}

func TestSyncMySQLConfig(t *testing.T) {
	desired := config.MySQLdConfig{"max_connections": "200"}

	tests := map[string]struct {
		cr          func() *api.PerconaXtraDBCluster
		sts         func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet
		podsReady   bool
		db          *fakeGlobalVariables
		wantApplied config.MySQLdConfig
		wantRestart bool
		wantSet     map[string]string
		wantErrMsg  string
	}{
		"older cr version keeps restarting through the config hash": {
			cr: func() *api.PerconaXtraDBCluster {
				cr := newMySQLConfigCR()
				cr.Spec.CRVersion = "1.20.0"
				return cr
			},
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet { return newMySQLConfigSts(cr, nil) },
		},
		"paused": {
			cr: func() *api.PerconaXtraDBCluster {
				cr := newMySQLConfigCR()
				cr.Spec.Pause = true
				return cr
			},
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet { return newMySQLConfigSts(cr, nil) },
		},
		"no statefulset yet": {
			cr: newMySQLConfigCR,
		},
		"first pass records the configuration pods start with": {
			cr:          newMySQLConfigCR,
			sts:         func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet { return newMySQLConfigSts(cr, nil) },
			wantApplied: desired,
		},
		"nothing changed": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, desired)
			},
			podsReady:   true,
			db:          &fakeGlobalVariables{},
			wantApplied: desired,
		},
		"removed option restarts": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "200", "sync_binlog": "1"})
			},
			wantApplied: desired,
			wantRestart: true,
		},
		"changed option is applied live": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
			},
			podsReady:   true,
			db:          &fakeGlobalVariables{},
			wantApplied: desired,
			wantSet:     map[string]string{"max_connections": "200"},
		},
		"changed option that needs a restart": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
			},
			podsReady: true,
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"max_connections": mysqlErr(errReadOnlyVariable)},
				current: map[string]string{"max_connections": "100"},
			},
			wantApplied: desired,
			wantRestart: true,
		},
		"changed option waits for the rollout": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				sts := newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
				sts.Status.UpdatedReplicas = 1
				return sts
			},
			podsReady:   true,
			db:          &fakeGlobalVariables{},
			wantApplied: config.MySQLdConfig{"max_connections": "100"},
		},
		"changed option waits for ready pods": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
			},
			db:          &fakeGlobalVariables{},
			wantApplied: config.MySQLdConfig{"max_connections": "100"},
		},
		"changed option waits for a ready cluster": {
			cr: func() *api.PerconaXtraDBCluster {
				cr := newMySQLConfigCR()
				cr.Status.PXC.Status = api.AppStateInit
				return cr
			},
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
			},
			podsReady:   true,
			db:          &fakeGlobalVariables{},
			wantApplied: config.MySQLdConfig{"max_connections": "100"},
		},
		"failed apply keeps the record": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				return newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
			},
			podsReady: true,
			db: &fakeGlobalVariables{
				setErrs: map[string]error{"max_connections": mysqlErr(1232)},
			},
			wantApplied: config.MySQLdConfig{"max_connections": "100"},
			wantErrMsg:  "set global variables",
		},
		"malformed record": {
			cr: newMySQLConfigCR,
			sts: func(cr *api.PerconaXtraDBCluster) *appsv1.StatefulSet {
				sts := newMySQLConfigSts(cr, nil)
				sts.Annotations = map[string]string{lastAppliedMySQLConfigAnnotation: "{"}
				return sts
			},
			wantErrMsg: "parse last applied mysql config",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := tc.cr()
			objs := []client.Object{cr}
			if tc.sts != nil {
				objs = append(objs, tc.sts(cr))
			}
			objs = append(objs, newReadyPXCPods(cr, tc.podsReady)...)
			r := newConfigReconciler(t, objs...)

			var db globalVariables = &fakeGlobalVariables{}
			if tc.db != nil {
				db = tc.db
			}

			err := r.syncMySQLConfig(t.Context(), cr, connectTo(db, nil))
			if tc.wantErrMsg != "" {
				require.ErrorContains(t, err, tc.wantErrMsg)
			} else {
				require.NoError(t, err)
			}

			if tc.sts == nil {
				return
			}

			sts := new(appsv1.StatefulSet)
			require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(tc.sts(cr)), sts))

			recorded := sts.Annotations[lastAppliedMySQLConfigAnnotation]
			if tc.wantApplied == nil {
				assert.Equal(t, tc.sts(cr).Annotations[lastAppliedMySQLConfigAnnotation], recorded)
			} else {
				applied, err := config.ParseMySQLdJSON(recorded)
				require.NoError(t, err)
				assert.Equal(t, tc.wantApplied, applied)
			}

			hash, restarted := sts.Spec.Template.Annotations[mysqlConfigHashAnnotation]
			assert.Equal(t, tc.wantRestart, restarted)
			if tc.wantRestart {
				want, err := desired.Hash()
				require.NoError(t, err)
				assert.Equal(t, want, hash)
			}

			if tc.db != nil {
				assert.Equal(t, tc.wantSet, tc.db.set)
			}
		})
	}
}

func TestSyncMySQLConfigRestoresStash(t *testing.T) {
	applied := config.MySQLdConfig{"max_connections": "100"}
	data, err := applied.JSON()
	require.NoError(t, err)

	cr := newMySQLConfigCR()
	cr.Annotations = map[string]string{lastAppliedMySQLConfigAnnotation: data}
	sts := newMySQLConfigSts(cr, nil)
	r := newConfigReconciler(t, append([]client.Object{cr, sts}, newReadyPXCPods(cr, true)...)...)

	db := &fakeGlobalVariables{}
	require.NoError(t, r.syncMySQLConfig(t.Context(), cr, connectTo(db, nil)))

	assert.Equal(t, map[string]string{"max_connections": "200"}, db.set)

	gotCR := new(api.PerconaXtraDBCluster)
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), gotCR))
	assert.NotContains(t, gotCR.Annotations, lastAppliedMySQLConfigAnnotation)
}

func TestDeleteStatefulSetForVolumeTemplateStashesRecord(t *testing.T) {
	cr := newMySQLConfigCR()
	sts := newMySQLConfigSts(cr, config.MySQLdConfig{"max_connections": "100"})
	r := newConfigReconciler(t, cr, sts)

	require.NoError(t, r.deleteStatefulSetForVolumeTemplate(t.Context(), cr, sts))

	err := r.client.Get(t.Context(), client.ObjectKeyFromObject(sts), new(appsv1.StatefulSet))
	require.True(t, client.IgnoreNotFound(err) == nil && err != nil)

	gotCR := new(api.PerconaXtraDBCluster)
	require.NoError(t, r.client.Get(t.Context(), client.ObjectKeyFromObject(cr), gotCR))
	assert.Equal(t, sts.Annotations[lastAppliedMySQLConfigAnnotation], gotCR.Annotations[lastAppliedMySQLConfigAnnotation])

	require.NoError(t, r.deleteStatefulSetForVolumeTemplate(t.Context(), cr, sts), "already deleted")
}

func TestUserMySQLConfig(t *testing.T) {
	cr := newMySQLConfigCR()
	ownerRef := metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(), Kind: "PerconaXtraDBCluster",
		Name: cr.Name, UID: cr.UID, Controller: new(true),
	}

	tests := map[string]struct {
		configuration string
		objs          []client.Object
		want          []string
	}{
		"secret wins over everything": {
			configuration: "[mysqld]\nmax_connections=200",
			objs: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc", Namespace: cr.Namespace},
					Data:       map[string][]byte{"b.cnf": []byte("b"), "a.cnf": []byte("a"), "README": []byte("x")},
				},
			},
			want: []string{"a", "b"},
		},
		"spec configuration": {
			configuration: "[mysqld]\nmax_connections=200",
			want:          []string{"[mysqld]\nmax_connections=200"},
		},
		"configmap the user created": {
			objs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc", Namespace: cr.Namespace},
					Data:       map[string]string{"init.cnf": "[mysqld]\nsync_binlog=1"},
				},
			},
			want: []string{"[mysqld]\nsync_binlog=1"},
		},
		"configmap left from a cleared configuration": {
			objs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc", Namespace: cr.Namespace, OwnerReferences: []metav1.OwnerReference{ownerRef}},
					Data:       map[string]string{"init.cnf": "[mysqld]\nsync_binlog=1"},
				},
			},
		},
		"none": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := cr.DeepCopy()
			cr.Spec.PXC.Configuration = tc.configuration
			r := newConfigReconciler(t, append([]client.Object{cr}, tc.objs...)...)

			got, err := r.userMySQLConfig(t.Context(), cr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAutoConfigParams(t *testing.T) {
	withResources := func(cpu, memory string, limits bool) func(cr *api.PerconaXtraDBCluster) {
		return func(cr *api.PerconaXtraDBCluster) {
			list := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(memory)}
			if limits {
				cr.Spec.PXC.Resources.Limits = list
			} else {
				cr.Spec.PXC.Resources.Requests = list
			}
		}
	}
	enable := func(cr *api.PerconaXtraDBCluster) {
		cr.Spec.PXC.AutoConfig = api.AutoConfigSpec{Enabled: new(true), LoadType: api.AutoConfigLoadTypeSomeWrites}
		cr.Status.PXC.Version = "8.4.7-7.1"
	}

	tests := map[string]struct {
		modify     []func(cr *api.PerconaXtraDBCluster)
		objs       []client.Object
		check      func(t *testing.T, got *autoConfigParams)
		wantErr    error
		wantErrMsg string
	}{
		"no memory limit": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("1", "4Gi", false)},
			check: func(t *testing.T, got *autoConfigParams) {
				assert.Nil(t, got)
			},
		},
		"autotune": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("1", "4Gi", true)},
			check: func(t *testing.T, got *autoConfigParams) {
				require.NotNil(t, got)
				assert.Contains(t, got.content, "innodb_buffer_pool_size")
				assert.NotContains(t, got.content, "wsrep_provider_options")
				assert.Empty(t, got.fallbackReason)
			},
		},
		"autotune with too little memory": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("1", "1Mi", true), func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.PXC.Configuration = ""
			}},
			wantErrMsg: "Not enough memory",
		},
		"autoconfig from requests": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", false), enable},
			check: func(t *testing.T, got *autoConfigParams) {
				require.NotNil(t, got)
				assert.Contains(t, got.content, "wsrep_provider_options=")
				assert.NotContains(t, got.content, "max_connections=", "set in pxc.configuration")
				assert.Empty(t, got.fallbackReason)
			},
		},
		"autoconfig leaves options of the user secret": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", true), enable},
			objs: []client.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc", Namespace: "pxc"},
				Data:       map[string][]byte{"my.cnf": []byte("[mysqld]\nwsrep-provider-options=pc.weight=10")},
			}},
			check: func(t *testing.T, got *autoConfigParams) {
				require.NotNil(t, got)
				assert.NotContains(t, got.content, "wsrep_provider_options=")
				assert.Contains(t, got.content, "max_connections=", "pxc.configuration is not mounted")
			},
		},
		"autoconfig falls back to autotune": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", true), enable, func(cr *api.PerconaXtraDBCluster) {
				cr.Status.PXC.Version = "8.0.32-24.2"
			}},
			check: func(t *testing.T, got *autoConfigParams) {
				require.NotNil(t, got)
				assert.Contains(t, got.content, "innodb_buffer_pool_size")
				assert.NotContains(t, got.content, "wsrep_provider_options")
				assert.Contains(t, got.fallbackReason, "out of the autoconfig supported range")
			},
		},
		"autoconfig with a volume too small": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", true), enable, func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.PXC.VolumeSpec = &api.VolumeSpec{PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}},
				}}
			}},
			wantErr: config.ErrInsufficientStorage,
		},
		"autoconfig needs a live config cr version": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", true), enable, func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.CRVersion = "1.20.0"
			}},
			check: func(t *testing.T, got *autoConfigParams) {
				require.NotNil(t, got)
				assert.NotContains(t, got.content, "wsrep_provider_options")
				assert.Empty(t, got.fallbackReason)
			},
		},
		"autoconfig with malformed user configuration": {
			modify: []func(cr *api.PerconaXtraDBCluster){withResources("2", "4Gi", true), enable, func(cr *api.PerconaXtraDBCluster) {
				cr.Spec.PXC.Configuration = "[mysqld"
			}},
			wantErrMsg: "parse user configuration",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := newMySQLConfigCR()
			for _, m := range tc.modify {
				m(cr)
			}
			r := newConfigReconciler(t, append([]client.Object{cr}, tc.objs...)...)

			got, err := r.autoConfigParams(t.Context(), cr)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, got)
				return
			case tc.wantErrMsg != "":
				require.ErrorContains(t, err, tc.wantErrMsg)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestDataVolumeCapacity(t *testing.T) {
	pvc := func(name, capacity string) client.Object {
		cr := newMySQLConfigCR()
		claim := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cr.Namespace, Labels: naming.LabelsPXC(cr)},
		}
		if capacity != "" {
			claim.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(capacity)}
		}
		return claim
	}
	claimSpec := &api.VolumeSpec{PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
		Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("6G")}},
	}}

	tests := map[string]struct {
		volumeSpec *api.VolumeSpec
		objs       []client.Object
		want       int64
	}{
		"no volume spec": {},
		"host path": {
			volumeSpec: &api.VolumeSpec{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}},
		},
		"empty dir without a limit": {
			volumeSpec: &api.VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		"empty dir with a limit": {
			volumeSpec: &api.VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: new(resource.MustParse("1G"))}},
			want:       1_000_000_000,
		},
		"claims not bound yet": {
			volumeSpec: claimSpec,
			objs:       []client.Object{pvc("datadir-cluster1-pxc-0", "")},
			want:       6_000_000_000,
		},
		"smallest datadir claim": {
			volumeSpec: claimSpec,
			objs: []client.Object{
				pvc("datadir-cluster1-pxc-0", "10G"),
				pvc("datadir-cluster1-pxc-1", "8G"),
				pvc("extra-cluster1-pxc-0", "1G"),
			},
			want: 8_000_000_000,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := newMySQLConfigCR()
			cr.Spec.PXC.VolumeSpec = tc.volumeSpec
			r := newConfigReconciler(t, append([]client.Object{cr}, tc.objs...)...)

			got, err := r.dataVolumeCapacity(t.Context(), cr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGetConfigHashExcludesLiveConfig(t *testing.T) {
	userConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1-pxc", Namespace: "pxc"},
		Data:       map[string]string{"init.cnf": "[mysqld]\nmax_connections=200"},
	}

	tests := map[string]struct {
		crVersion string
		app       func(cr *api.PerconaXtraDBCluster) api.StatefulApp
		wantEmpty bool
	}{
		"pxc on a live config version": {
			crVersion: liveConfigVersion,
			app:       func(cr *api.PerconaXtraDBCluster) api.StatefulApp { return statefulset.NewNode(cr) },
			wantEmpty: true,
		},
		"pxc on an older version": {
			crVersion: "1.20.0",
			app:       func(cr *api.PerconaXtraDBCluster) api.StatefulApp { return statefulset.NewNode(cr) },
		},
		"haproxy keeps its config hash": {
			crVersion: liveConfigVersion,
			app:       func(cr *api.PerconaXtraDBCluster) api.StatefulApp { return statefulset.NewHAProxy(cr) },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := newMySQLConfigCR()
			cr.Spec.CRVersion = tc.crVersion
			r := newConfigReconciler(t, cr, userConfig)

			got, err := r.getConfigHash(t.Context(), cr, tc.app(cr))
			require.NoError(t, err)
			assert.Equal(t, tc.wantEmpty, got == "")
		})
	}
}

func TestReconcileAutotuneConfigMap(t *testing.T) {
	resources := corev1.ResourceRequirements{Limits: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi"),
	}}
	enabled := func(version string) func(cr *api.PerconaXtraDBCluster) {
		return func(cr *api.PerconaXtraDBCluster) {
			cr.Spec.PXC.Resources = resources
			cr.Spec.PXC.AutoConfig = api.AutoConfigSpec{Enabled: new(true), LoadType: api.AutoConfigLoadTypeSomeWrites}
			cr.Status.PXC.Version = version
		}
	}

	tests := map[string]struct {
		modify      func(cr *api.PerconaXtraDBCluster)
		existing    bool
		wantContent string
		wantDeleted bool
		wantEvent   string
		wantErr     error
	}{
		"calculated": {
			modify:      enabled("8.4.7-7.1"),
			wantContent: "wsrep_provider_options=",
		},
		"fallback is reported": {
			modify:      enabled("8.0.32-24.2"),
			wantContent: "innodb_buffer_pool_size",
			wantEvent:   "AutoConfigFallback",
		},
		"insufficient storage is reported": {
			modify: func(cr *api.PerconaXtraDBCluster) {
				enabled("8.4.7-7.1")(cr)
				cr.Spec.PXC.VolumeSpec = &api.VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: new(resource.MustParse("1Gi"))}}
			},
			wantEvent: "AutoConfigInsufficientStorage",
			wantErr:   config.ErrInsufficientStorage,
		},
		"nothing to tune deletes the owned configmap": {
			modify:      func(cr *api.PerconaXtraDBCluster) {},
			existing:    true,
			wantDeleted: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cr := newMySQLConfigCR()
			cr.UID = "cr-uid"
			tc.modify(cr)

			name := config.AutoTuneConfigMapName(cr.Name, naming.ComponentPXC)
			objs := []client.Object{cr}
			if tc.existing {
				cm := config.NewAutoConfigMap(cr, name, "\nmax_connections=1")
				cm.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: api.SchemeGroupVersion.String(), Kind: "PerconaXtraDBCluster",
					Name: cr.Name, UID: cr.UID, Controller: new(true),
				}}
				objs = append(objs, cm)
			}
			r := newConfigReconciler(t, objs...)
			recorder := record.NewFakeRecorder(10)
			r.recorder = recorder

			_, err := r.reconcileAutotuneConfigMap(t.Context(), cr)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tc.wantEvent != "" {
				require.Len(t, recorder.Events, 1)
				assert.Contains(t, <-recorder.Events, tc.wantEvent)
			} else {
				assert.Empty(t, recorder.Events)
			}

			cm := new(corev1.ConfigMap)
			err = r.client.Get(t.Context(), client.ObjectKey{Namespace: cr.Namespace, Name: name}, cm)
			if tc.wantContent == "" {
				assert.True(t, client.IgnoreNotFound(err) == nil && err != nil, "configmap should not exist")
				return
			}
			require.NoError(t, err)
			assert.Contains(t, cm.Data[config.AutoConfigKey], tc.wantContent)
			assert.True(t, metav1.IsControlledBy(cm, cr))
		})
	}
}
