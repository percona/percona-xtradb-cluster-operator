package naming

// BackupStorageCAFileDirectory is the path where the CA bundle file will be mounted in a container.
const (
	BackupStorageCAFileDirectory = "/tmp/s3/certs"
	BackupStorageCAFileName      = "ca.crt"
)
