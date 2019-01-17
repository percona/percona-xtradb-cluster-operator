Providing Backups
==============================================================

Percona XtraDB Cluster Operator allows doing cluster backup in two ways.
*Scheduled backups* are configured in the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file to be executed automatically in proper time.
*On-demand backups* can be done manually at any moment.

## Making scheduled backups

Backups schedule is defined in the  ``backup`` section of the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file. The schedule is specified in crontab format as explained in the [Operator Options](https://percona-lab.github.io/percona-xtradb-cluster-operator/configure/operator).

## Making on-demand backup

To make on-demand backup, user should use YAML file with correct names for the backup and the PXC Cluster, and correct PVC settings. The example of such file is [deploy/backup/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/backup/cr.yaml).

When the backup config file is ready, actual backup command is executed:

   ```
   kubectl apply -f deploy/backup/cr.yaml
   ```

**Note:** *Storing backup settings in a separate file can be replaced by passing its content to the `kubectl apply` command as follows:*

   ```
   cat <<EOF | kubectl apply -f-
   apiVersion: "pxc.percona.com/v1alpha1"
   kind: "PerconaXtraDBBackup"
   metadata:
     name: "backup1"
   spec:
     pxcCluster: "cluster1"
     volume:
       # storageClass: standard
       size: 6Gi
   EOF
   ```

## Restore the cluster from a previously saved backup

Following steps are needed to restore a previously saved backup:

1. First of all make sure that the cluster is running.
2. Now find out correct names for the backup and the cluster. Available backups can be listed with the following command:
   ```
   kubectl get pxc-backup
   ```
   And the following command will list available clusters:
   ```
   kubectl get pxc
   ```
4. When both correct names are known, the actual restoration process can be started as follows:
   ```
   ./deploy/backup/restore-backup.sh <backup-name> <cluster-name>
   ```

## Delete the unneeded backup

Deleting a previously saved backup requires not more than the backup name. This name can be taken from the list of available backups returned by the following command:

   ```
   kubectl get pxc-backup
   ```

When the name is known, backup can be deleted as follows:

   ```
   kubectl delete pxc-backup/<backup-name>
   ```

## Copy backup to a local machine

Make a local copy of a previously saved backup requires not more than the backup name. This name can be taken from the list of available backups returned by the following command:

   ```
   kubectl get pxc-backup
   ```

When the name is known, backup can be downloaded to the local machine as follows:

   ```
   ./deploy/backup/copy-backup.sh <backup-name> path/to/dir
   ```

For example, this downloaded backup can be restored to the local installation of Percona Server:

   ```
   service mysqld stop
   rm -rf /var/lib/mysql/*
   cat xtrabackup.stream | xbstream -x -C /var/lib/mysql
   xtrabackup --prepare --target-dir=/var/lib/mysql
   chown -R mysql:mysql /var/lib/mysql
   service mysqld start
   ```
