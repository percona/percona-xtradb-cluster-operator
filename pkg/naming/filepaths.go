package naming

// BackupStorageCAFileDirectory is the path where the CA bundle file will be mounted in a container.
const (
	BackupStorageCAFileDirectory = "/etc/s3/certs"
	BackupStorageCAFileName      = "ca.crt"
)

const (
	PITRNotReady     = "pitr-not-ready"
	GapDetected      = "/tmp/gap-detected"
	TimelinePath     = "/tmp/pitr-timeline" // path to file with timeline
	LatestBackupPath = "/tmp/latest-backup"
)
