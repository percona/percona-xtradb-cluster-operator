#!/bin/bash

ctrl="oc"

function usage {
  cat << EOF
 usage: $0 [-h] -r "new restored pvc" -v "backup pvc" -d "backup directory (under /backup, do not include "/backup")"
 
 OPTIONS:
    -h        Show this message
    -r string new restored cluster persistent volume claim
    -d string backup path (run list_backups.sh to see the current directory)
    -v string persistent volume claim with backup
EOF

}

while getopts :d:r:v: flag; do
  case $flag in
    d)
      backupDir="${OPTARG}";
      ;;
    r)
      restorePVC="${OPTARG}";
      ;;
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

if [ "$backupDir"  == "" ]; then echo "backupDir is not defined, use -d <backup dir>"; usage; exit 1; fi
if [ "$backupPVC"  == "" ]; then echo "backupPVC is not defined, use -v <backup PVC>"; usage; exit 1; fi
if [ "$restorePVC"  == "" ]; then echo "restorePVC is not defined, use -d <restore PVC>"; usage; exit 1; fi

cat <<EOF | $ctrl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: xtrabackup-restore-job
spec:
  template:
    spec:
      containers:
      - name: xtrabackup
        image: perconalab/backupjob-openshift
        command:
        - bash
        - "-c"
        - |
          set -ex
          cd /datadir
          rm -fr *
          cat /backup/\$BACKUPSRC/xtrabackup.stream | xbstream -x 
          xtrabackup --prepare --target-dir=/datadir
        env:
          - name: BACKUPSRC
            value: $backupDir
        volumeMounts:
        - name: backup
          mountPath: /backup
        - name: datadir
          mountPath: /datadir
      restartPolicy: Never
      volumes:
      - name: backup
        persistentVolumeClaim:
          claimName: $backupPVC
      - name: datadir
        persistentVolumeClaim:
          claimName: $restorePVC
  backoffLimit: 4
EOF