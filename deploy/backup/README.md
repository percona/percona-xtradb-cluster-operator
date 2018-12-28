## Restore from backup
To start the cluster from the backup
1. Make sure the cluster is not running
2. Locate directory you want to restore from on the backup volume, e.g. `cluster1-pxc-nodes-2018-12-28-18-29`. Use `list-backups.sh -v <backup PVC>` to get list of backups
3. Run `restore-backup.sh -d <backup dir> -v <backup PVC> -r <restore PVC>`