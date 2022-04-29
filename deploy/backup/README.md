## Documentation
Learn more about backups and restores in our [documentation](https://www.percona.com/doc/kubernetes-operator-for-pxc/backups.html).

## Make on-demand backup
1. correct the backup name, cluster name, and PVC settings in the `deploy/backup/backup.yaml` file
2. run backup
   ```
   kubectl apply -f deploy/backup/backup.yaml
   ```
## Restore from backup
1. Make sure that the cluster is running
2. List avaible backups
   ```
   kubectl get pxc-backup
   ```
3. List available clusters
   ```
   kubectl get pxc
   ```
4. start the resoration process
   ```
   kubectl apply -f deploy/backup/restore.yaml
   ```
## Copy backup to local machine
1. List available backups
   ```
   kubectl get pxc-backup
   ```
2. Download backup
   ```
   ./deploy/backup/copy-backup.sh <backup-name> path/to/dir
   ```
3. Restore backup locally if needed
   ```
   service mysqld stop
   rm -rf /var/lib/mysql/*
   cat xtrabackup.stream | xbstream --decompress -x -C /var/lib/mysql
   xtrabackup --prepare --target-dir=/var/lib/mysql
   chown -R mysql:mysql /var/lib/mysql
   service mysqld start
   ```
## Delete backup
1. List available backups
   ```
   kubectl get pxc-backup
   ```
2. Delete backup
   ```
   kubectl delete pxc-backup/<backup-name>
   ```
