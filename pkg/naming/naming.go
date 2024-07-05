package naming

const (
	annotationPrefix      = "percona.com/"
	annotationPrefixMysql = "pxc.percona.com/"
)

//const (
//	LabelName      = "app.kubernetes.io/name"
//	LabelInstance  = "app.kubernetes.io/instance"
//	LabelManagedBy = "app.kubernetes.io/managed-by"
//	LabelPartOf    = "app.kubernetes.io/part-of"
//	LabelComponent = "app.kubernetes.io/component"
//)
//
//const (
//	LabelMySQLPrimary = annotationPrefixMysql + "primary"
//	LabelExposed      = annotationPrefix + "exposed"
//)

const (
	FinalizerDeleteSSL            = annotationPrefix + "delete-ssl"
	FinalizerDeletePxcPodsInOrder = annotationPrefix + "delete-pxc-pods-in-order"
	FinalizerDeleteProxysqlPvc    = annotationPrefix + "delete-proxysql-pvc"
	FinalizerDeletePxcPvc         = annotationPrefix + "delete-pxc-pvc"

	FinalizerDeleteS3Backup = annotationPrefix + "delete-s3-backup" // TODO: rename to a more appropriate name like `delete-backup`
)

//type AnnotationKey string
//
//func (s AnnotationKey) String() string {
//	return string(s)
//}
//
//const (
//	AnnotationSecretHash       AnnotationKey = annotationPrefix + "last-applied-secret"
//	AnnotationConfigHash       AnnotationKey = annotationPrefix + "configuration-hash"
//	AnnotationTLSHash          AnnotationKey = annotationPrefix + "last-applied-tls"
//	AnnotationPasswordsUpdated AnnotationKey = annotationPrefix + "passwords-updated"
//	AnnotationLastConfigHash   AnnotationKey = annotationPrefix + "last-config-hash"
//)
