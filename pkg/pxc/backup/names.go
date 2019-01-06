package backup

import (
	"math/rand"
	"strings"
	"time"

	api "github.com/Percona-Lab/percona-xtradb-cluster-operator/pkg/apis/pxc/v1alpha1"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const genSymbols = "abcdefghijklmnopqrstuvwxyz1234567890"

func genRandString(ln int) string {
	b := make([]byte, ln)
	for i := range b {
		b[i] = genSymbols[rand.Intn(len(genSymbols))]
	}

	return string(b)
}

func genScheduleLabel(sched string) string {
	r := strings.NewReplacer("*", "N", "/", "E", " ", "_")
	return r.Replace(sched)
}

// genName63 generates legit name for backup resources.
// k8s sets the `job-name` label for the created by job pod.
// So we have to be sure that job name won't be longer than 63 symbols.
// Yet the job name has to have some meaningful name which won't be conflicting with other jobs' names.
func genName63(cr *api.PerconaXtraDBBackup) string {
	postfix := cr.Name
	maxNameLen := 16
	typ, ok := cr.GetLabels()["type"]

	// in case it's not a cron-job we're not sure if the name fits rules
	// but there is more room for names
	if !ok || typ != "cron" {
		maxNameLen = 29
		if len(postfix) > maxNameLen {
			postfix = postfix[:maxNameLen]
		}
	}

	name := cr.Spec.PXCCluster
	if len(cr.Spec.PXCCluster) > maxNameLen {
		name = name[:maxNameLen]
	}
	name += "-xb-" + postfix

	return name
}
