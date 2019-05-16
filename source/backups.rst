Providing Backups
=================

Percona XtraDB Cluster Operator allows doing cluster backup in two ways.
*Scheduled backups* are configured in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file to be executed automatically in proper time. *On-demand backups*
can be done manually at any moment.

Backup images are usually stored on `Amazon S3 or S3-compatible
storage <https://en.wikipedia.org/wiki/Amazon_S3#S3_API_and_competing_services>`__
(storing backups on private storage is also possible, but they are
described separately).

Making scheduled backups
------------------------

Since backups are stored separately on the Amazon S3, a secret with
``AWS_ACCESS_KEY_ID`` and ``AWS_SECRET_ACCESS_KEY`` should be present on
the Kubernetes cluster. The secrets file with these keys should be
created: for example ``deploy/backup-s3.yaml`` file with the following
contents:

::

   apiVersion: v1
   kind: Secret
   metadata:
     name: my-cluster-name-backup-s3
   type: Opaque
   data:
     AWS_ACCESS_KEY_ID: UkVQTEFDRS1XSVRILUFXUy1BQ0NFU1MtS0VZ
     AWS_SECRET_ACCESS_KEY: UkVQTEFDRS1XSVRILUFXUy1TRUNSRVQtS0VZ

The ``name`` value is the `Kubernetes
secret <https://kubernetes.io/docs/concepts/configuration/secret/>`__
name which will be used further, and ``AWS_ACCESS_KEY_ID`` and
``AWS_SECRET_ACCESS_KEY`` are the keys to access S3 storage (and
obviously they should contain proper values to make this access
possible). To have effect secrets file should be applied with the
appropriate command to create the secret object,
e.g. \ ``kubectl apply -f deploy/backup-s3.yaml`` (for Kubernetes).

Backups schedule is defined in the ``backup`` section of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file. This section contains following subsections: \* ``storages``
subsection contains data needed to access the S3-compatible cloud to
store backups. \* ``schedule`` subsection allows to actually schedule
backups (the schedule is specified in crontab format).

Here is an example which uses Amazon S3 storage for backups:

::

   ...
   backup:
     enabled: true
     version: 0.3.0
     ...
     storages:
       s3-us-west:
         type: s3
         s3:
           bucket: S3-BACKUP-BUCKET-NAME-HERE
           region: us-west-2
           credentialsSecret: my-cluster-name-backup-s3
     ...
     schedule:
      - name: "sat-night-backup"
        schedule: "0 0 * * 6"
        keep: 3
        storageName: s3-us-west
     ...

**Note:** *if you use some S3-compatible storage instead of the original
Amazon S3, one more key is needed in the ``s3`` subsection: the
``endpointUrl``, which points to the actual cloud used for backups and
is specific to the cloud provider. For example, using*\ `Google
Cloud <https://cloud.google.com>`__\ *involves the following one:
``endpointUrl: https://storage.googleapis.com``.*

The options within these three subsections are further explained in the
`Operator
Options <https://percona.github.io/percona-xtradb-cluster-operator/configure/operator>`__.

The only option which should be mentioned separately is
``credentialsSecret`` which is a `Kubernetes
secret <https://kubernetes.io/docs/concepts/configuration/secret/>`__
for backups. Value of this key should be the same as the name used to
create the secret object (``my-cluster-name-backup-s3`` in the last
example).

The schedule is specified in crontab format as explained in the
`Operator
Options <https://percona.github.io/percona-xtradb-cluster-operator/configure/operator>`__.

Making on-demand backup
-----------------------

To make on-demand backup, user should use YAML file with correct names
for the backup and the PXC Cluster, and correct PVC settings. The
example of such file is
`deploy/backup/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/backup/cr.yaml>`__.

When the backup config file is ready, actual backup command is executed:

::

   kubectl apply -f deploy/backup/cr.yaml

**Note:** *Storing backup settings in a separate file can be replaced by
passing its content to the ``kubectl apply`` command as follows:*

::

   cat <<EOF | kubectl apply -f-
   apiVersion: "pxc.percona.com/v1alpha1"
   kind: "PerconaXtraDBBackup"
   metadata:
     name: "backup1"
   spec:
     pxcCluster: "cluster1"
     storageName: "s3-us-west"
   EOF

Restore the cluster from a previously saved backup
--------------------------------------------------

Following steps are needed to restore a previously saved backup:

1. First of all make sure that the cluster is running.

2. Now find out correct names for the backup and the cluster. Available
   backups can be listed with the following command:

   ::

      kubectl get pxc-backup

   And the following command will list available clusters:

   ::

      kubectl get pxc

3. When both correct names are known, the actual restoration process can
   be started as follows:

   ::

      kubectl apply -f deploy/backup/restore.yaml

**Note:** *Storing backup settings in a separate file can be replaced by
passing its content to the ``kubectl apply`` command as follows:*

      ::

         cat <<EOF | kubectl apply -f-
         apiVersion: "pxc.percona.com/v1alpha1"
         kind: "PerconaXtraDBBackup"
         metadata:
           name: "backup1"
         spec:
           pxcCluster: "cluster1"
           storageName: "s3-us-west"
         EOF

Delete the unneeded backup
--------------------------

Deleting a previously saved backup requires not more than the backup
name. This name can be taken from the list of available backups returned
by the following command:

::

   kubectl get pxc-backup

When the name is known, backup can be deleted as follows:

::

   kubectl delete pxc-backup/<backup-name>

Copy backup to a local machine
------------------------------

Make a local copy of a previously saved backup requires not more than
the backup name. This name can be taken from the list of available
backups returned by the following command:

::

   kubectl get pxc-backup

When the name is known, backup can be downloaded to the local machine as
follows:

::

   ./deploy/backup/copy-backup.sh <backup-name> path/to/dir

For example, this downloaded backup can be restored to the local
installation of Percona Server:

::

   service mysqld stop
   rm -rf /var/lib/mysql/*
   cat xtrabackup.stream | xbstream -x -C /var/lib/mysql
   xtrabackup --prepare --target-dir=/var/lib/mysql
   chown -R mysql:mysql /var/lib/mysql
   service mysqld start
