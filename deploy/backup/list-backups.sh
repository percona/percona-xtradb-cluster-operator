#!/bin/bash

ctrl="oc"

ctrlargs="rsh po/list-backups"
if [ "$ctrl" == "kubectl" ]
then
	ctrlargs="exec list-backups --"
fi


function usage {
  cat << EOF
 usage: $0 [-h] -v persistent volume claim with backup"
 
 OPTIONS:
    -h        Show this message
    -v string persistent volume claim with backup
EOF

}

while getopts :v: flag; do
  case $flag in
    v)
      backupPVC="${OPTARG}";
      ;;
    h)
      usage;
      exit 0;
      ;;
    *)
      usage;
      exit 1;
      ;;
  esac
done
shift $((OPTIND -1))

if [ "$backupPVC"  == "" ]; then echo "backupPVC is not defined, use -v <PVC>"; usage; exit 1; fi


container_phase=$($ctrl get po/list-backups -o=go-template='{{ .status.phase }}' 2>/dev/null)
if [ "$container_phase" != "Running" ]
then
	# start container
	echo "Starting container..."
	cat <<EOF | $ctrl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: list-backups
spec:
  containers:
  - image: busybox
    imagePullPolicy: IfNotPresent
    name: list-backups
    volumeMounts:
    - name: backup
      mountPath: /backup
    stdin: true
    stdinOnce: true
  volumes:
  - name: backup
    persistentVolumeClaim:
      claimName: $backupPVC
EOF

echo "Waiting for container to be ready..."
echo ""
fi

for i in {1..20}
do
	container_phase=$($ctrl get po/list-backups -o=go-template='{{ .status.phase }}' 2>/dev/null)
	if [ "$container_phase" == "Running" ]
	then
		break
	fi
	sleep 1
done

$ctrl $ctrlargs du -h /backup/ | sort -k2
last_backup=$($ctrl $ctrlargs du -h /backup/ | sort -k2 | egrep -v '4.0K|lost\+found' | tail -n 1|cut -d$'\t' -f 2| cut -d '/' -f 3)

echo "To restore a backup run: ./restore-backup.sh -d <backup dir> -v <backup PVC> -r <restore PVC>, i.e. ./restore-backup.sh -d $last_backup -v $backupPVC -r <datadir-cluster1-pxc-node-0>"
