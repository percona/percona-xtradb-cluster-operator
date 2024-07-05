package naming

const (
	annotationPrefix = "percona.com/"
)

const (
	FinalizerDeleteSSL            = annotationPrefix + "delete-ssl"
	FinalizerDeletePxcPodsInOrder = annotationPrefix + "delete-pxc-pods-in-order"
	FinalizerDeleteProxysqlPvc    = annotationPrefix + "delete-proxysql-pvc"
	FinalizerDeletePxcPvc         = annotationPrefix + "delete-pxc-pvc"

	FinalizerDeleteS3Backup = annotationPrefix + "delete-s3-backup" // TODO: rename to a more appropriate name like `delete-backup`
)
