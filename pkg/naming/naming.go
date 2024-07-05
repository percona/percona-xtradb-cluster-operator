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

	// TODO depricated in 1.15.0 should be deleted in 1.18.0
	FinalizerDeleteS3Backup = annotationPrefix + "delete-s3-backup"
)
