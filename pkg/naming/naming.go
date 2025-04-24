package naming

const (
	annotationPrefix         = "percona.com/"
	internalAnnotationPrefix = "internal." + annotationPrefix
)

const (
	FinalizerDeleteSSL            = annotationPrefix + "delete-ssl"
	FinalizerDeletePxcPodsInOrder = annotationPrefix + "delete-pxc-pods-in-order"
	FinalizerDeleteProxysqlPvc    = annotationPrefix + "delete-proxysql-pvc"
	FinalizerDeletePxcPvc         = annotationPrefix + "delete-pxc-pvc"
	FinalizerDeleteBackup         = annotationPrefix + "delete-backup"
	FinalizerReleaseLock          = internalAnnotationPrefix + "release-lock"
)

const (
	OperatorController = "pxc-controller"
)

const (
	EventStorageClassNotSupportResize = "StorageClassNotSupportResize"
	EventExceededQuota                = "ExceededQuota"
)
