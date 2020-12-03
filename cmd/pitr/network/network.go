package network

import (
	"os/exec"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func GetPXCLastHost(pxcServiceName string) (string, error) {
	cmd := exec.Command("peer-list", "-on-start=/usr/bin/get-pxc-state", "-service="+pxcServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrap(err, "get output")
	}
	nodes := strings.Split(string(out), "node:")
	sort.Strings(nodes)
	lastHost := ""
	for _, node := range nodes {
		if strings.Contains(node, "wsrep_ready:ON:wsrep_connected:ON:wsrep_local_state_comment:Synced:wsrep_cluster_status:Primary") {
			nodeArr := strings.Split(node, ":")
			lastHost = nodeArr[0]
		}
	}
	if len(lastHost) == 0 {
		return "", errors.New("cant find host")
	}

	return lastHost, nil
}
