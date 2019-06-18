package pxc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	mrand "math/rand"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileUsersSecret(cr *api.PerconaXtraDBCluster) error {
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.SecretsName,
		},
		&secretObj,
	)
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get secret: %v", err)
	}

	data := make(map[string][]byte)
	data["root"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create root users password: %v", err)
	}
	data["xtrabackup"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create xtrabackup users password: %v", err)
	}
	data["monitor"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create monitor users password: %v", err)
	}
	data["clustercheck"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create clustercheck users password: %v", err)
	}
	data["proxyadmin"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create proxyadmin users password: %v", err)
	}
	data["pmmserver"], err = generatePass()
	if err != nil {
		return fmt.Errorf("create pmmserver users password: %v", err)
	}
	secretObj = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.SecretsName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}
	err = r.client.Create(context.TODO(), &secretObj)
	if err != nil {
		return fmt.Errorf("create Users secret: %v", err)
	}
	return nil
}

func generatePass() ([]byte, error) {
	mrand.Seed(time.Now().UnixNano())
	max := 20
	min := 16
	ln := mrand.Intn(max-min) + min
	b := make([]byte, ln)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
	base64.StdEncoding.Encode(buf, b)

	return buf, nil
}
