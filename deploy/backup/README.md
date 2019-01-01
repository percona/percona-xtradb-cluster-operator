## Make on-demand backup
1. correct the backup name, cluster name, and PVC settings in the `deploy/backup/cr.yaml` file
2. run backup
   ```
   kubectl apply -f deploy/backup/cr.yaml
   ```
## Restore from backup
1. Make sure that the cluster is running
2. List avaible backups
   ```
   kubectl get pxc-backup
   ```
3. start the resoration process
   ```
   ./deploy/backup/restore-backup.sh backup1 cluster1
   ```
