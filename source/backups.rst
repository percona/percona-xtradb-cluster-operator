Providing Backups
=================

The Operator usually stores Percona XtraDB Cluster backups on `Amazon S3 or S3-compatible
storage <https://en.wikipedia.org/wiki/Amazon_S3#S3_API_and_competing_services>`_ outside the Kubernetes cluster:


.. image:: assets/images/backup-s3.png
   :align: center
   :alt: Backup on S3-compatible storage

But storing backups on `Persistent Volumes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/>`_ inside the Kubernetes cluster is also possible:


.. image:: assets/images/backup-pv.png
   :align: center
   :alt: Backup on Persistent Volume

The Operator allows doing backup in two ways. 
*Scheduled backups* are configured in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`_
file to be executed automatically in proper time. *On-demand backups*
can be done manually at any moment.

.. contents:: :local:

.. _backups.scheduled:

Making scheduled backups
------------------------

Since backups are stored separately on the Amazon S3, a secret with
``AWS_ACCESS_KEY_ID`` and ``AWS_SECRET_ACCESS_KEY`` should be present on
the Kubernetes cluster. The secrets file with these base64-encoded keys should
be created: for example ``deploy/backup-s3.yaml`` file with the following
contents:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: my-cluster-name-backup-s3
   type: Opaque
   data:
     AWS_ACCESS_KEY_ID: UkVQTEFDRS1XSVRILUFXUy1BQ0NFU1MtS0VZ
     AWS_SECRET_ACCESS_KEY: UkVQTEFDRS1XSVRILUFXUy1TRUNSRVQtS0VZ

.. note:: The following command can be used to get a base64-encoded string from
   a plain text one: ``$ echo -n 'plain-text-string' | base64``

The ``name`` value is the `Kubernetes
secret <https://kubernetes.io/docs/concepts/configuration/secret/>`__
name which will be used further, and ``AWS_ACCESS_KEY_ID`` and
``AWS_SECRET_ACCESS_KEY`` are the keys to access S3 storage (and
obviously they should contain proper values to make this access
possible). To have effect secrets file should be applied with the
appropriate command to create the secret object, e.g. 
``kubectl apply -f deploy/backup-s3.yaml`` (for Kubernetes).

Backups schedule is defined in the ``backup`` section of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file. This section contains following subsections:

* ``storages`` subsection contains data needed to access the S3-compatible cloud
  to store backups.
* ``schedule`` subsection allows to actually schedule backups (the schedule is
  specified in crontab format).

Here is an example of `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`_ which uses Amazon S3 storage for backups:

.. code:: yaml

   ...
   backup:
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

if you use some S3-compatible storage instead of the original
Amazon S3, the `endpointURL <https://docs.min.io/docs/aws-cli-with-minio.html>`_ is needed in the `s3` subsection which points to the actual cloud used for backups and
is specific to the cloud provider. For example, using `Google Cloud <https://cloud.google.com>`_ involves the `following <https://storage.googleapis.com>`_ endpointUrl:

.. code:: yaml

   endpointUrl: https://storage.googleapis.com

The options within these three subsections are further explained in the
:ref:`operator.custom-resource-options`.

One option which should be mentioned separately is
``credentialsSecret`` which is a `Kubernetes
secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_
for backups. Value of this key should be the same as the name used to
create the secret object (``my-cluster-name-backup-s3`` in the last
example).

The schedule is specified in crontab format as explained in
:ref:`operator.custom-resource-options`.

.. _backups-manual:

Making on-demand backup
-----------------------

To make an on-demand backup, the user should first configure the backup storage
in the ``backup.storages`` subsection of the ``deploy/cr.yaml`` configuration
file in a same way it was done for scheduled backups. When the
``deploy/cr.yaml`` file contains correctly configured storage and is applied
with ``kubectl`` command, use *a special backup configuration YAML file* with
the following contents:

* **backup name** in the ``metadata.name`` key,
* **PXC Cluster name** in the ``spec.pxcCluster`` key,
* **storage name** from ``deploy/cr.yaml`` in the ``spec.storageName`` key.

The example of the backup configuration file is `deploy/backup/backup.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/backup/backup.yaml>`_.

When the backup destination is configured and applied with `kubectl apply -f deploy/cr.yaml` command, the actual backup command is executed:

.. code:: bash

   kubectl apply -f deploy/backup/backup.yaml

.. note:: Storing backup settings in a separate file can be replaced by
   passing its content to the ``kubectl apply`` command as follows:

   .. code:: bash

      cat <<EOF | kubectl apply -f-
      apiVersion: pxc.percona.com/v1
      kind: PerconaXtraDBClusterBackup
      metadata:
        name: backup1
      spec:
        pxcCluster: cluster1
        storageName: s3-us-west
      EOF

.. _backups-private-volume:

Storing backup on ‎Persistent Volume
-----------------------------------

Here is an example of the ``deploy/cr.yaml`` backup section fragment, which
configures a private volume for filesystem-type storage:

.. code:: yaml

  ...
  backup:
    ...
    storages:
      fs-pvc:
        type: filesystem
        volume:
          persistentVolumeClaim:
            accessModes: [ "ReadWriteOnce" ]
            resources:
              requests:
                storage: 6Gi
    ...

.. note:: Please take into account that 6Gi storage size specified in this
   example may be insufficient for the real-life setups; consider using tens or
   hundreds of gigabytes. Also, you can edit this option later, and changes will
   take effect after applying the updated ``deploy/cr.yaml`` file with
   ``kubectl``.

.. _backups-compression:

Enabling compression for backups
--------------------------------

There is a possibility to enable 
`LZ4 compression <https://en.wikipedia.org/wiki/LZ4_(compression_algorithm)>`_
for backups.

.. note:: This feature is available only with PXC 8.0 and not PXC 5.7.

To enable compression, use :ref:`pxc-configuration` key in the
``deploy/cr.yaml`` configuration file to supply Percona XtraDB Cluster nodes
with two additional ``my.cnf`` options under its ``[sst]`` and ``[xtrabackup]``
sections as follows:

.. code:: yaml

   pxc:
     image: percona/percona-xtradb-cluster:8.0.19-10.1
     configuration: |
       ...
       [sst]
       xbstream-opts=--decompress
       [xtrabackup]
       compress=lz4
       ...

When enabled, compression will be used for both backups and `SST <https://www.percona.com/doc/percona-xtradb-cluster/8.0/manual/state_snapshot_transfer.html>`_.

.. _backups-restore:

Restore the cluster from a previously saved backup
--------------------------------------------------

Backup can be restored not only on the Kubernetes cluster where it was made, but
also on any Kubernetes-based environment with the installed Operator.

.. note:: When restoring to a new Kubernetes-based environment, make sure it
   has a Secrets object with the same user passwords as in the original cluster.
   More details about secrets can be found in :ref:`users.system-users`.

Following steps are needed to restore a previously saved backup:

1. First of all make sure that the cluster is running.

2. Now find out correct names for the **backup** and the **cluster**. Available
   backups can be listed with the following command:

   .. code:: bash

      kubectl get pxc-backup

   .. note:: Obviously, you can make this check only on the same cluster on
      which you have previously made the backup.

   And the following command will list existing Percona XtraDB Cluster names in
   the current Kubernetes-based environment:

   .. code:: bash

      kubectl get pxc

3. When both correct names are known, it is needed to set appropriate keys
   in the ``deploy/backup/restore.yaml`` file.

   * set ``spec.pxcCluster`` key to the name of the target cluster to restore
     the backup on,
   * if you are restoring backup on the *same* Kubernetes-based cluster you have
      used to save this backup, set ``spec.backupName`` key to the name of your
      backup,
   * if you are restoring backup on the Kubernetes-based cluster *different*
     from one you have used to save this backup, set ``spec.backupSource``
     subsection instead of ``spec.backupName`` field to point on the appropriate
     PVC or S3-compatible storage:

     A. If backup was stored on the PVC volume, ``backupSource`` should contain
        the storage name (which should be configured in the main CR) and PVC Name:

        .. code-block:: yaml

           ...
           backupSource:
             destination: pvc/PVC_VOLUME_NAME
             storageName: pvc
             ...

     B. If backup was stored on the S3-compatible storage, ``backupSource``
        should contain ``destination`` key equal to the s3 bucket with a special
        ``s3://`` prefix, followed by the necessary S3 configuration keys, same
        as in ``deploy/cr.yaml`` file:

        .. code-block:: yaml

           ...
           backupSource:
             destination: s3://S3-BUCKET-NAME/BACKUP-NAME
             s3:
               credentialsSecret: my-cluster-name-backup-s3
               region: us-west-2
               endpointURL: https://URL-OF-THE-S3-COMPATIBLE-STORAGE
           ...

   After that, the actual restoration process can be started as follows:

   .. code:: bash

      kubectl apply -f deploy/backup/restore.yaml

.. note:: Storing backup settings in a separate file can be replaced by passing
   its content to the ``kubectl apply`` command as follows:

   .. code:: bash

      cat <<EOF | kubectl apply -f-
      apiVersion: "pxc.percona.com/v1"
      kind: "PerconaXtraDBClusterRestore"
      metadata:
        name: "restore1"
      spec:
        pxcCluster: "cluster1"
        backupName: "backup1"
      EOF

.. _backups-delete:

Delete the unneeded backup
--------------------------

Deleting a previously saved backup requires not more than the backup
name. This name can be taken from the list of available backups returned
by the following command:

.. code:: bash

   kubectl get pxc-backup

When the name is known, backup can be deleted as follows:

.. code:: bash

   kubectl delete pxc-backup/<backup-name>

.. _backups-copy:

Copy backup to a local machine
------------------------------

Make a local copy of a previously saved backup requires not more than
the backup name. This name can be taken from the list of available
backups returned by the following command:

.. code:: bash

   kubectl get pxc-backup

When the name is known, backup can be downloaded to the local machine as
follows:

.. code:: bash

   ./deploy/backup/copy-backup.sh <backup-name> path/to/dir

For example, this downloaded backup can be restored to the local
installation of Percona Server:

.. code:: bash

   service mysqld stop
   rm -rf /var/lib/mysql/*
   cat xtrabackup.stream | xbstream -x -C /var/lib/mysql
   xtrabackup --prepare --target-dir=/var/lib/mysql
   chown -R mysql:mysql /var/lib/mysql
   service mysqld start
