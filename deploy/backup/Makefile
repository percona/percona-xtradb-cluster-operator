.PHONY: status

status:
	@kubectl get pxc-backup/${name} -o go-template --template='Cluster: {{.spec.pxcCluster | println}}Started: {{ .metadata.creationTimestamp | println}}Status: {{.status.state | println}}{{if .status.completed}}Completed: {{.status.completed | println}}{{end}}'