package pxc

import (
	"context"
	"fmt"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/encryption"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcilePerconaXtraDBCluster) reconsileKeyring(cr *api.PerconaXtraDBCluster) error {
	secretObj := corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      "keyring",
		},
		&secretObj,
	)

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("get secret: %v", err)
	}

	// check if brand new secret should be generated
	if errors.IsNotFound(err) {
		log.Info("keyring secret not found create a new one")

		owner, err := OwnerRef(cr, r.scheme)
		if err != nil {
			return err
		}
		ownerReferences := []metav1.OwnerReference{owner}
		secretObj = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "keyring",
				Namespace:       cr.Namespace,
				OwnerReferences: ownerReferences,
			},
			Data: make(map[string][]byte),
		}

		err = r.client.Create(context.TODO(), &secretObj)
		if err != nil {
			return fmt.Errorf("failed to create keyring secret: %v", err)
		}
	}

	dataLen := len(secretObj.Data)
	diff := int(cr.Spec.PXC.Size) - len(secretObj.Data)

	if diff <= 0 {
		return nil
	}

	for i := len(secretObj.Data); i < dataLen+diff; i++ {
		id := fmt.Sprintf("%s-%s-%d", cr.Name, app.Name, i)
		log.Info(fmt.Sprintf("generate new keyring %s", id))

		keyring, err := encryption.NewKeyring()
		if err != nil {
			return fmt.Errorf("failed to generate keyring %v", err)
		}

		secretObj.Data[id] = keyring
	}

	err = r.client.Update(context.TODO(), &secretObj)
	if err != nil {
		return fmt.Errorf("failed to update keyring secret: %v", err)
	}

	return nil
}
