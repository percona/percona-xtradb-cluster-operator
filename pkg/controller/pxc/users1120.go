package pxc

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"reflect"

	"github.com/hashicorp/go-version"
	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileUsers1120(cr *api.PerconaXtraDBCluster) (*ReconcileUsersResult, error) {
	sysUsersSecretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&sysUsersSecretObj,
	)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "get sys users secret '%s'", cr.Spec.SecretsName)
	}

	secretName := internalPrefix + cr.Name

	internalSysSecretObj := corev1.Secret{}

	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      secretName,
		},
		&internalSysSecretObj,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Wrap(err, "get internal sys users secret")
	}

	if k8serrors.IsNotFound(err) {
		internalSysUsersSecret := sysUsersSecretObj.DeepCopy()
		internalSysUsersSecret.ObjectMeta = metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cr.Namespace,
		}
		err = r.client.Create(context.TODO(), internalSysUsersSecret)
		if err != nil {
			return nil, errors.Wrap(err, "create internal sys users secret")
		}
		return nil, nil
	}

	if reflect.DeepEqual(sysUsersSecretObj.Data, internalSysSecretObj.Data) {
		log.Printf("user secrets not changed.\n")
		return nil, nil
	}

	if cr.Status.Status != api.AppStateReady {
		return nil, nil
	}

	restarts, err := r.updateUsers(cr, &sysUsersSecretObj, &internalSysSecretObj)
	if err != nil {
		return nil, errors.Wrap(err, "manage sys users")
	}

	updatedSecrets := corev1.Secret{}
	err = r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&updatedSecrets,
	)

	if err != nil {
		return nil, errors.Wrapf(err, "get updated users secret '%s'", cr.Spec.SecretsName)
	}

	newSysData, err := json.Marshal(sysUsersSecretObj.Data)
	if err != nil {
		return nil, errors.Wrap(err, "marshal sys secret data")
	}
	newSecretDataHash := sha256Hash(newSysData)

	result := &ReconcileUsersResult{
		updateReplicationPassword: restarts.updateReplicationPass,
	}

	if restarts.restartProxy {
		result.proxysqlAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}
	if restarts.restartPXC {
		result.pxcAnnotations = map[string]string{"last-applied-secret": newSecretDataHash}
	}

	return result, nil
}

func (r *ReconcilePerconaXtraDBCluster) updateUsers(cr *api.PerconaXtraDBCluster, sysUsersSecretObj, internalSysSecretObj *corev1.Secret) (*userUpdateRestart, error) {
	restarts := &userUpdateRestart{}

	type user struct {
		name      string
		hosts     []string
		proxyUser bool
	}
	requiredUsers := []user{
		{
			name:   "root",
			hosts:  []string{"localhost", "%"},
		},
		{
			name:      "monitor",
			hosts:     []string{"%"},
		},
		{
			name:  "clustercheck",
			hosts: []string{"localhost"},
		},
		{
			name:   "operator",
			hosts:  []string{"%"},
		},
		{
			name:   "xtrabackup",
			hosts:  []string{"%"},
		},
	}

	if cr.CompareVersionWith("1.9.0") >= 0 {
		requiredUsers = append(requiredUsers, user{
			name:   "replication",
			hosts:  []string{"%"},
		})
	}

	if cr.Spec.PMM != nil && cr.Spec.PMM.Enabled {
		name := "pmmserverkey"
		if !cr.Spec.PMM.UseAPI(sysUsersSecretObj) {
			name = "pmmserver"
		}
		requiredUsers = append(requiredUsers, user{
			name:   name,
		})
	}
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.Enabled {
		requiredUsers = append(requiredUsers, user{
			name:      "proxyadmin",
			proxyUser: true,
		})
	}

	uu := []*users.SysUser{}

	for u := range []string{"root", "operator", "monitor", "xtrabackup", "replication", "pmmserver", "pmmserverkey", "clustercheck"} {
		if bytes.Equal(sysUsersSecretObj.Data[u], internalSysSecretObj.Data[u]) {
			continue
		}

		uu = append(uu, &users.SysUser{
			Name: u,
		})
	}

	// for u := range uu {

	// }

	return restarts, nil
}
