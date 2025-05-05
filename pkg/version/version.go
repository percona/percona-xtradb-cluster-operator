package version

import (
	_ "embed"
	"strings"
)

//go:generate sh -c "yq -i '.metadata.labels.\"app.kubernetes.io/version\" = \"v\" + load(\"version.txt\")' ../../config/crd/patches/versionlabel_in_pxc.yaml"
//go:generate sh -c "yq -i '.metadata.labels.\"app.kubernetes.io/version\" = \"v\" + load(\"version.txt\")' ../../config/crd/patches/versionlabel_in_pxcbackup.yaml"
//go:generate sh -c "yq -i '.metadata.labels.\"app.kubernetes.io/version\" = \"v\" + load(\"version.txt\")' ../../config/crd/patches/versionlabel_in_pxcrestore.yaml"

//go:embed version.txt
var version string

func Version() string {
	return strings.TrimSpace(version)
}
