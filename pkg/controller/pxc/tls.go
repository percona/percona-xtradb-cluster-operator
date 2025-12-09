package pxc

import (
	"bytes"
	"context"
	"fmt"
	"time"

	cm "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/statefulset"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxctls"
)

func (r *ReconcilePerconaXtraDBCluster) reconcileSSL(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	if err := r.reconcileTLSToggle(ctx, cr); err != nil {
		return errors.Wrap(err, "reconcile tls toggle")
	}

	if !cr.TLSEnabled() {
		return nil
	}

	secretObj := corev1.Secret{}
	secretInternalObj := corev1.Secret{}
	errSecret := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLSecretName,
		},
		&secretObj,
	)
	errInternalSecret := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: cr.Namespace,
			Name:      cr.Spec.PXC.SSLInternalSecretName,
		},
		&secretInternalObj,
	)
	if errSecret == nil && errInternalSecret == nil {
		return nil
	} else if errSecret != nil && !k8serr.IsNotFound(errSecret) {
		return fmt.Errorf("get secret: %v", errSecret)
	} else if errInternalSecret != nil && !k8serr.IsNotFound(errInternalSecret) {
		return fmt.Errorf("get internal secret: %v", errInternalSecret)
	}
	// don't create secret ssl-internal if secret ssl is not created by operator
	if errSecret == nil && !metav1.IsControlledBy(&secretObj, cr) {
		return nil
	}
	err := r.createSSLByCertManager(ctx, cr)
	if err != nil {
		if cr.Spec.TLS != nil && cr.Spec.TLS.IssuerConf != nil {
			return fmt.Errorf("create ssl with cert manager %w", err)
		}
		err = r.createSSLManualy(ctx, cr)
		if err != nil {
			return fmt.Errorf("create ssl internally: %v", err)
		}
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createSSLByCertManager(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	issuerName := cr.Name + "-pxc-issuer"
	caIssuerName := cr.Name + "-pxc-ca-issuer"
	issuerKind := "Issuer"
	issuerGroup := ""
	caDuration := &metav1.Duration{Duration: pxctls.DefaultCAValidity}
	if cr.Spec.TLS != nil && cr.Spec.TLS.CADuration != nil {
		caDuration = cr.Spec.TLS.CADuration
	}

	if cr.Spec.TLS != nil && cr.Spec.TLS.IssuerConf != nil {
		issuerKind = cr.Spec.TLS.IssuerConf.Kind
		issuerName = cr.Spec.TLS.IssuerConf.Name
		issuerGroup = cr.Spec.TLS.IssuerConf.Group
	} else {
		if err := r.createIssuer(ctx, cr, caIssuerName, ""); err != nil {
			return err
		}

		caCert := &cm.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-ca-cert",
				Namespace: cr.Namespace,
			},
			Spec: cm.CertificateSpec{
				SecretName: cr.Name + "-ca-cert",
				CommonName: cr.Name + "-ca",
				IsCA:       true,
				IssuerRef: cmmeta.ObjectReference{
					Name:  caIssuerName,
					Kind:  issuerKind,
					Group: issuerGroup,
				},
				Duration:    caDuration,
				RenewBefore: &metav1.Duration{Duration: pxctls.DefaultRenewBefore},
			},
		}
		if cr.CompareVersionWith("1.16.0") >= 0 {
			caCert.Labels = naming.LabelsCluster(cr)
		}

		err := r.client.Create(ctx, caCert)
		if err != nil && !k8serr.IsAlreadyExists(err) {
			return fmt.Errorf("create CA certificate: %v", err)
		}

		if err := r.waitForCerts(ctx, cr.Namespace, caCert.Spec.SecretName); err != nil {
			return err
		}

		if err := r.createIssuer(ctx, cr, issuerName, caCert.Spec.SecretName); err != nil {
			return err
		}
	}

	duration := &metav1.Duration{Duration: pxctls.DefaultCertValidity}
	if cr.Spec.TLS != nil && cr.Spec.TLS.Duration != nil {
		duration = cr.Spec.TLS.Duration
	}

	kubeCert := &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl",
			Namespace: cr.Namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLSecretName,
			CommonName: cr.Name + "-proxysql",
			DNSNames: []string{
				cr.Name + "-pxc",
				cr.Name + "-proxysql",
				"*." + cr.Name + "-pxc",
				"*." + cr.Name + "-proxysql",
			},
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuerName,
				Kind:  issuerKind,
				Group: issuerGroup,
			},
		},
	}
	if cr.CompareVersionWith("1.16.0") >= 0 {
		kubeCert.Labels = naming.LabelsCluster(cr)
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		kubeCert.Spec.DNSNames = append(kubeCert.Spec.DNSNames, cr.Spec.TLS.SANs...)
	}
	if cr.CompareVersionWith("1.19.0") >= 0 {
		kubeCert.Spec.Duration = duration
	}

	err := r.client.Create(ctx, kubeCert)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create certificate: %v", err)
	}

	if cr.Spec.PXC.SSLSecretName == cr.Spec.PXC.SSLInternalSecretName {
		return r.waitForCerts(ctx, cr.Namespace, cr.Spec.PXC.SSLSecretName)
	}

	kubeCert = &cm.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-ssl-internal",
			Namespace: cr.Namespace,
		},
		Spec: cm.CertificateSpec{
			SecretName: cr.Spec.PXC.SSLInternalSecretName,
			CommonName: cr.Name + "-pxc",
			DNSNames: []string{
				cr.Name + "-pxc",
				"*." + cr.Name + "-pxc",
				cr.Name + "-haproxy-replicas." + cr.Namespace + ".svc.cluster.local",
				cr.Name + "-haproxy-replicas." + cr.Namespace,
				cr.Name + "-haproxy-replicas",
				cr.Name + "-haproxy." + cr.Namespace + ".svc.cluster.local",
				cr.Name + "-haproxy." + cr.Namespace,
				cr.Name + "-haproxy",
			},
			IssuerRef: cmmeta.ObjectReference{
				Name:  issuerName,
				Kind:  issuerKind,
				Group: issuerGroup,
			},
		},
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		kubeCert.Spec.DNSNames = append(kubeCert.Spec.DNSNames, cr.Spec.TLS.SANs...)
	}
	if cr.CompareVersionWith("1.16.0") >= 0 {
		kubeCert.Labels = naming.LabelsCluster(cr)
	}
	if cr.CompareVersionWith("1.19.0") >= 0 {
		kubeCert.Spec.Duration = duration
	}
	err = r.client.Create(ctx, kubeCert)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create internal certificate: %v", err)
	}

	return r.waitForCerts(ctx, cr.Namespace, cr.Spec.PXC.SSLSecretName, cr.Spec.PXC.SSLInternalSecretName)
}

func (r *ReconcilePerconaXtraDBCluster) waitForCerts(ctx context.Context, namespace string, secretsList ...string) error {
	ticker := time.NewTicker(3 * time.Second)
	timeoutTimer := time.NewTimer(30 * time.Second)
	defer timeoutTimer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-timeoutTimer.C:
			return errors.Errorf("timeout: can't get tls certificates from certmanager, %s", secretsList)
		case <-ticker.C:
			sucessCount := 0
			for _, secretName := range secretsList {
				secret := &corev1.Secret{}
				err := r.client.Get(ctx, types.NamespacedName{
					Name:      secretName,
					Namespace: namespace,
				}, secret)
				if err != nil && !k8serr.IsNotFound(err) {
					return err
				} else if err == nil {
					sucessCount++
				}
			}
			if sucessCount == len(secretsList) {
				return nil
			}
		}
	}
}

func (r *ReconcilePerconaXtraDBCluster) createIssuer(ctx context.Context, cr *api.PerconaXtraDBCluster, issuer string, caCertSecret string) error {
	spec := cm.IssuerSpec{}

	if caCertSecret == "" {
		spec = cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				SelfSigned: &cm.SelfSignedIssuer{},
			},
		}
	} else {
		spec = cm.IssuerSpec{
			IssuerConfig: cm.IssuerConfig{
				CA: &cm.CAIssuer{SecretName: caCertSecret},
			},
		}
	}

	var ls map[string]string
	if cr.CompareVersionWith("1.16.0") >= 0 {
		ls = naming.LabelsCluster(cr)
	}
	err := r.client.Create(ctx, &cm.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      issuer,
			Namespace: cr.Namespace,
			Labels:    ls,
		},
		Spec: spec,
	})
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create issuer: %v", err)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) createSSLManualy(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	data := make(map[string][]byte)
	proxyHosts := []string{
		cr.Name + "-pxc",
		cr.Name + "-proxysql",
		"*." + cr.Name + "-pxc",
		"*." + cr.Name + "-proxysql",
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		proxyHosts = append(proxyHosts, cr.Spec.TLS.SANs...)
	}
	caCert, tlsCert, key, err := pxctls.Issue(proxyHosts, cr.CompareVersionWith("1.19.0") >= 0)
	if err != nil {
		return fmt.Errorf("create proxy certificate: %v", err)
	}
	data["ca.crt"] = caCert
	data["tls.crt"] = tlsCert
	data["tls.key"] = key
	if err != nil {
		return err
	}
	secretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.PXC.SSLSecretName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	if cr.CompareVersionWith("1.16.0") >= 0 {
		secretObj.Labels = naming.LabelsCluster(cr)
	}
	err = r.client.Create(ctx, &secretObj)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create TLS secret: %v", err)
	}
	pxcHosts := []string{
		cr.Name + "-pxc",
		"*." + cr.Name + "-pxc",
		cr.Name + "-haproxy-replicas." + cr.Namespace + ".svc.cluster.local",
		cr.Name + "-haproxy-replicas." + cr.Namespace,
		cr.Name + "-haproxy-replicas",
		cr.Name + "-haproxy." + cr.Namespace + ".svc.cluster.local",
		cr.Name + "-haproxy." + cr.Namespace,
		cr.Name + "-haproxy",
	}
	if cr.Spec.TLS != nil && len(cr.Spec.TLS.SANs) > 0 {
		pxcHosts = append(pxcHosts, cr.Spec.TLS.SANs...)
	}
	caCert, tlsCert, key, err = pxctls.Issue(pxcHosts, cr.CompareVersionWith("1.19.0") >= 0)
	if err != nil {
		return fmt.Errorf("create pxc certificate: %v", err)
	}
	data["ca.crt"] = caCert
	data["tls.crt"] = tlsCert
	data["tls.key"] = key
	secretObjInternal := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.PXC.SSLInternalSecretName,
			Namespace: cr.Namespace,
		},
		Data: data,
		Type: corev1.SecretTypeTLS,
	}
	if cr.CompareVersionWith("1.16.0") >= 0 {
		secretObjInternal.Labels = naming.LabelsCluster(cr)
	}
	err = r.client.Create(ctx, &secretObjInternal)
	if err != nil && !k8serr.IsAlreadyExists(err) {
		return fmt.Errorf("create TLS internal secret: %v", err)
	}
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileTLSToggle(ctx context.Context, cr *api.PerconaXtraDBCluster) error {
	if cr.CompareVersionWith("1.16.0") < 0 {
		return nil
	}

	condition := cr.Status.FindCondition(naming.ConditionTLS)
	if condition == nil {
		cr.Status.AddCondition(api.ClusterCondition{
			Type:               naming.ConditionTLS,
			Status:             api.ConditionStatus(naming.GetConditionTLSState(cr)),
			LastTransitionTime: metav1.NewTime(time.Now().Truncate(time.Second)),
		})
		return nil
	}

	if condition.Status == api.ConditionStatus(naming.GetConditionTLSState(cr)) {
		return nil
	}

	clusterPaused, err := k8s.PauseCluster(ctx, r.client, cr)
	if err != nil {
		return errors.Wrap(err, "failed to pause cluster")
	}
	if !clusterPaused {
		return nil
	}

	switch naming.ConditionTLSState(condition.Status) {
	case naming.ConditionTLSStateEnabled:
		if err := r.deleteCerts(ctx, cr); err != nil {
			return errors.Wrap(err, "failed to delete tls secrets")
		}
	case naming.ConditionTLSStateDisabled:
	default:
		return errors.Errorf("unknown value for %s condition status: %s", naming.ConditionTLS, condition.Status)
	}

	patch := client.MergeFrom(cr.DeepCopy())
	cr.Spec.Unsafe.TLS = !*cr.Spec.TLS.Enabled
	if err := r.client.Patch(ctx, cr.DeepCopy(), patch); err != nil {
		return errors.Wrap(err, "failed to patch cr")
	}

	_, err = k8s.UnpauseCluster(ctx, r.client, cr)
	if err != nil {
		return errors.Wrap(err, "failed to start cluster")
	}

	condition.Status = api.ConditionStatus(naming.GetConditionTLSState(cr))
	condition.LastTransitionTime = metav1.NewTime(time.Now().Truncate(time.Second))
	return nil
}

func (r *ReconcilePerconaXtraDBCluster) rotateSSLCertificates(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
) error {
	if !cr.TLSEnabled() {
		return nil
	}

	if inProgress, err := r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLSecretName); err != nil {
		return errors.Wrap(err, "failed to rotate ssl certificate")
	} else if inProgress {
		return nil
	}

	if inProgress, err := r.rotateSSLCertificate(ctx, cr, cr.Spec.PXC.SSLInternalSecretName); err != nil {
		return errors.Wrap(err, "failed to rotate internal ssl certificate")
	} else if inProgress {
		return nil
	}

	return nil
}

// rotateSSLCertificate rotates the TLS certificates for the given secret name
// returns true if the certificate rotation is in progress, false otherwise
// returns an error if the certificate rotation fails
func (r *ReconcilePerconaXtraDBCluster) rotateSSLCertificate(
	ctx context.Context,
	cr *api.PerconaXtraDBCluster,
	secretName string,
) (bool, error) {
	if !cr.TLSEnabled() {
		return false, nil
	}

	// newSecret contains the new TLS certificates
	newSecretName := secretName + "-new"
	newSecretObj := corev1.Secret{}
	if err := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: cr.GetNamespace(),
			Name:      newSecretName,
		},
		&newSecretObj,
	); k8serr.IsNotFound(err) {
		return false, nil // nothing to do
	} else if err != nil {
		return false, fmt.Errorf("failed to get new secret %s: %w", newSecretName, err)
	}

	// secretObj contains the current TLS certificates
	secretObj := corev1.Secret{}
	if err := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: cr.GetNamespace(),
			Name:      secretName,
		},
		&secretObj,
	); err != nil {
		// todo: if not found, should we create the secret using newSecretObj?
		return false, fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	log := logf.FromContext(ctx).WithValues("secretName", secretName)

	if added, err := r.addTLSCertRotationAnnotation(ctx, cr, secretName); err != nil {
		return false, fmt.Errorf("failed to add SSL certificate rotation annotation: %w", err)
	} else if added {
		log.Info("Starting SSL certificate rotation")
	}

	// Wait for current set of TLS certificates to be applied onto the PXC nodes.
	if ok, err := r.isSSLReconciled(ctx, cr, secretName); err != nil {
		return false, fmt.Errorf("failed to check if ssl is reconciled: %w", err)
	} else if !ok {
		log.Info("Waiting for SSL to be reconciled")
		return true, nil // check again later
	}

	// step 1: combine both CA certificates
	currentCA := secretObj.Data["ca.crt"]
	newCA := newSecretObj.Data["ca.crt"]
	if !bytes.Contains(currentCA, newCA) {
		log.Info("Combining new CA certificate and applying")
		updatedSecret := secretObj.DeepCopy()
		updatedSecret.Data["ca.crt"] = append(currentCA, newCA...)
		return true, r.client.Update(ctx, updatedSecret)
	}

	// step 2: use new TLS certificates
	currentTLSCert := secretObj.Data["tls.crt"]
	newTLSCert := newSecretObj.Data["tls.crt"]
	currentTLSKey := secretObj.Data["tls.key"]
	newTLSKey := newSecretObj.Data["tls.key"]
	if !bytes.Equal(currentTLSKey, newTLSKey) || !bytes.Equal(currentTLSCert, newTLSCert) {
		log.Info("Applying new TLS certificates")
		updatedSecret := secretObj.DeepCopy()
		updatedSecret.Data["tls.crt"] = newTLSCert
		updatedSecret.Data["tls.key"] = newTLSKey
		return true, r.client.Update(ctx, updatedSecret)
	}

	// step 3: use new CA certificate
	if !bytes.Equal(currentCA, newCA) {
		log.Info("Applying new CA certificate")
		updatedSecret := secretObj.DeepCopy()
		updatedSecret.Data["ca.crt"] = newCA
		return true, r.client.Update(ctx, updatedSecret)
	}

	if err := r.removeTLSCertRotationAnnotation(ctx, cr, secretName); err != nil {
		return false, fmt.Errorf("failed to remove SSL certificate rotation annotation: %w", err)
	}

	if !newSecretObj.GetDeletionTimestamp().IsZero() {
		return false, nil
	}

	if err := r.client.Delete(ctx, &newSecretObj); err != nil {
		return false, fmt.Errorf("failed to delete new secret: %w", err)
	}
	log.Info("SSL certificate rotation completed")

	return false, nil
}
func (r *ReconcilePerconaXtraDBCluster) removeTLSCertRotationAnnotation(ctx context.Context, cr *api.PerconaXtraDBCluster, secretName string) error {
	annots := cr.GetAnnotations()
	var annot string
	switch secretName {
	case cr.Spec.PXC.SSLSecretName:
		annot = api.AnnotationSSLCertRotationInProgress
	case cr.Spec.PXC.SSLInternalSecretName:
		annot = api.AnnotationInternalSSLCertRotationInProgress
	default:
		return fmt.Errorf("unknown secret name for SSL certificate rotation: %s", secretName)
	}

	_, exists := annots[annot]
	if !exists {
		return nil
	}

	delete(annots, annot)
	cr.SetAnnotations(annots)
	return r.client.Update(ctx, cr)
}

func (r *ReconcilePerconaXtraDBCluster) addTLSCertRotationAnnotation(ctx context.Context, cr *api.PerconaXtraDBCluster, secretName string) (bool, error) {
	annots := cr.GetAnnotations()
	if annots == nil {
		annots = make(map[string]string)
	}

	var annot string
	switch secretName {
	case cr.Spec.PXC.SSLSecretName:
		annot = api.AnnotationSSLCertRotationInProgress
	case cr.Spec.PXC.SSLInternalSecretName:
		annot = api.AnnotationInternalSSLCertRotationInProgress
	default:
		return false, fmt.Errorf("unknown secret name for SSL certificate rotation: %s", secretName)
	}

	_, exists := annots[annot]
	if exists {
		return false, nil
	}

	now := metav1.Now().Format(time.RFC3339)
	annots[annot] = now
	cr.SetAnnotations(annots)
	return true, r.client.Update(ctx, cr)
}

func (r *ReconcilePerconaXtraDBCluster) isSSLReconciled(ctx context.Context, cr *api.PerconaXtraDBCluster, secretName string) (bool, error) {
	calculatedHash, err := r.getSecretHash(cr, secretName, false)
	if err != nil {
		return false, fmt.Errorf("failed to get secret hash: %w", err)
	}

	sfs := statefulset.NewNode(cr).StatefulSet()
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(sfs), sfs); err != nil {
		return false, fmt.Errorf("failed to get statefulset: %w", err)
	}

	var annotation string
	switch secretName {
	case cr.Spec.PXC.SSLSecretName:
		annotation = sslHashAnnotation
	case cr.Spec.PXC.SSLInternalSecretName:
		annotation = sslInternalHashAnnotation
	default:
		return false, fmt.Errorf("unknown secret name for SSL certificate rotation: %s", secretName)
	}
	currentAnnotations := sfs.Spec.Template.Annotations
	observedHash := currentAnnotations[annotation]

	return calculatedHash == observedHash && cr.Status.Status == api.AppStateReady, nil
}
