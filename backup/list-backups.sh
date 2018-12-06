ctrl="oc"

container_phase=$($ctrl get po/list-backups -o=go-template='{{ .status.phase }}' 2>/dev/null)
#echo "Container list-backups phase is: $container_phase"
if [ "$container_phase" != "Running" ]
then
	# start container
	echo "Starting container..."
	$ctrl create -f list-backups.yaml 2>/dev/null
        sleep 5
fi

$ctrl rsh po/list-backups du -h /backup/ | sort -k2
last_backup=$($ctrl rsh po/list-backups du -h /backup/ | sort -k2 | grep -v '4.0K' | tail -n 1|cut -d$'\t' -f 2| cut -d '/' -f 3)

echo "To restore a backup run: restore.sh -d <backup dir>, i.e. ./restore.sh -d $last_backup"
#$ctrl delete -f list-backups.yaml
