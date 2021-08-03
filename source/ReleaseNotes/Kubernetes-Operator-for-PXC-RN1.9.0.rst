.. rn:: 1.9.0

================================================================================
*Percona Distribution for MySQL Operator* 1.9.0
================================================================================

:Date: August 5, 2021
:Installation: For installation please refer to `the documentation page <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

Release Highlights
================================================================================

* Starting from this release, the Operator changes its official name to
  **Percona Distribution for MySQL Operator**. This new name emphasizes
  graduate changes which incorporated a collection of Percona’s solutions to run
  and operate Percona Server for MySQL and Percona XtraDB Cluster, available
  separately as `Percona Distribution for MySQL <https://www.percona.com/doc/percona-distribution-mysql/8.0/index.html>`_.
* The :ref:`cross-site replication<operator-replication>` feature allows an
  asynchronous replication between two Percona XtraDB Clusters, including
  scenarios when one of the clusters is outside of the Kubernetes environment.

New Features
================================================================================

* :jirabug:`K8SPXC-657`: Store custom configuration in Secrets
* :jirabug:`K8SPXC-308`: Add asynchronous replication setup for PXC cluster running in K8S
* :jirabug:`K8SPXC-688`: Add possibility of defining env variables via CR

Improvements
================================================================================

* :jirabug:`K8SPXC-791`: allow "sleep infinity" on non-debug images
* :jirabug:`K8SPXC-764`: Allow backups even if just a single node is available
* :jirabug:`K8SPXC-765`: Add ConfigMaps deletion for custom configurations (Thanks to Oleksandr Levchenkov for reporting this issue)
* :jirabug:`K8SPXC-734`: Include PXC namespace in the manual recovery command (Thanks to Michael Lin for reporting this issue)
* :jirabug:`K8SPXC-656`: Set imagePullPolicy for init container (Thanks to Herberto Graça for reporting this issue)
* :jirabug:`K8SPXC-511`: Delete Secret object in Kubernetes if pvc finalizer is enabled (Thanks to Matthias Baur for reporting this issue)
* :jirabug:`K8SPXC-784`: Parameterize operator deployment name
* :jirabug:`K8SPXC-772`: Add common labels to service
* :jirabug:`K8SPXC-749`: Add tunable parameters for any timeout existing in the checks
* :jirabug:`K8SPXC-731`: Capture cluster provisioning progress in the Custom Resource
* :jirabug:`K8SPXC-730`: Rework statuses for a Custom Resource
* :jirabug:`K8SPXC-720`: Create additional PITR test
* :jirabug:`K8SPXC-697`: Add namespace support in copy-backup script
* :jirabug:`K8SPXC-673`: Add PMM client sidecar for HAProxy pods
* :jirabug:`K8SPXC-568`: Restrict running more than 5 pods of PXC if unsafe flag is not set
* :jirabug:`K8SPXC-556`: Restrict running less than 2 pods of Haproxy if unsafe flag is not set
* :jirabug:`K8SPXC-554`: Reduce number of various object updates from the operator
* :jirabug:`K8SPXC-421`: PXC pods have X Plugin enabled, but it's not available nor balanced
* :jirabug:`K8SPXC-336`: Fix the tangle in cluster statuses
* :jirabug:`K8SPXC-321`: Restrict running less than 2 pods of proxySQL if unsafe flag is not set


Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-757`: Manual Crash Recovery interferes with auto recovery even with auto_recovery: false
* :jirabug:`K8SPXC-742`: socat in percona/percona-xtradb-cluster-operator:1.7.0-pxc5.7-backup generates "E SSL_read(): Connection reset by peer"
* :jirabug:`K8SPXC-706`: Certificate renewal - PXC fails to restart (Thanks to Jeff Andrews for reporting this issue)
* :jirabug:`K8SPXC-785`: Backup to S3 produces error messages even during successful backup
* :jirabug:`K8SPXC-642`: PodDisruptionBudget Problem due to wrong haproxy Statefulset Labels (Thanks to Davi S Evangelista for reporting this issue)
* :jirabug:`K8SPXC-585`: Can't delete cluster (operator stuck in reconcileUsers) (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-756`: While cluster is paused - operator schedule backups. (Thanks to Dmytro for reporting this issue)
* :jirabug:`K8SPXC-821`: custom config from secret is not mounted to proxysql
* :jirabug:`K8SPXC-815`: ready count in cr status can be higher than size value
* :jirabug:`K8SPXC-813`: restore doesn't error on wrong AWS credentials
* :jirabug:`K8SPXC-811`: HAProxy ready nodes missing in cr status
* :jirabug:`K8SPXC-805`: Deletion of pxc-backups object hangs if operator can't list objects from S3 bucket
* :jirabug:`K8SPXC-787`: The cluster doesn't become ready after password for xtrabackup user is changed
* :jirabug:`K8SPXC-775`: The custom mysqld config isn't checked in case of cluster update
* :jirabug:`K8SPXC-767`: On demand backup hangs if it was created when the cluster was in 'initializing' state
* :jirabug:`K8SPXC-743`: Remove confusing error messages from the log of backup
* :jirabug:`K8SPXC-726`: cannot delete a pvc backup which had delete-s3-backup finalizer specified
* :jirabug:`K8SPXC-682`: Auto tuning sets wrong innodb_buffer_pool_size
