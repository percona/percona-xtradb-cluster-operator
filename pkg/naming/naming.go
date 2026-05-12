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
	FinalizerKeepJob              = internalAnnotationPrefix + "keep-job"
)

const (
	OperatorController           = "pxc-controller"
	OperatorWebhookTLSSecretName = "pxc-webhook-ssl"
)

const (
	EventStorageClassNotSupportResize = "StorageClassNotSupportResize"
	EventExceededQuota                = "ExceededQuota"
)

const (
	ContainerNamePXC = "pxc"
)

const (
	DataVolumeName = "datadir"
	BinVolumeName  = "bin"
)
