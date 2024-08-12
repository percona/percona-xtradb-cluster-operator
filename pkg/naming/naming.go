package naming

const (
	annotationPrefix = "percona.com/"
)

const (
	FinalizerDeleteSSL            = annotationPrefix + "delete-ssl"
	FinalizerDeletePxcPodsInOrder = annotationPrefix + "delete-pxc-pods-in-order"
	FinalizerDeleteProxysqlPvc    = annotationPrefix + "delete-proxysql-pvc"
	FinalizerDeletePxcPvc         = annotationPrefix + "delete-pxc-pvc"
	FinalizerDeleteBackup         = annotationPrefix + "delete-backup"
	FinalizerS3DeleteBackup       = "delete-s3-backup"
)
