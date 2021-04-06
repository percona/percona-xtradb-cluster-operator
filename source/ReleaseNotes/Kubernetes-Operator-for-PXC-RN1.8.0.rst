.. rn:: 1.8.0

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.8.0
================================================================================

:Date: April 13, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

New Features
================================================================================

* :jirabug:`K8SPXC-528`: Support for custom sidecar container to extend the Operator capabilities
* :jirabug:`K8SPXC-647`: Allow cluster sacale in and scale out with the ``kubectl scale`` command or HorizontalPodAutoscaler
* :jirabug:`K8SPXC-643`: Automatic recovery after network partition

Improvements
================================================================================

* :jirabug:`K8SPXC-654`: Use admin port for liveness/readiness probs
* :jirabug:`K8SPXC-442`: Add support for retention of backups stored on S3 (Thanks to Davi S Evangelista for reporting this issue)
* :jirabug:`K8SPXC-697`: Add namespace support in the script used to :ref:`copy-backup`
* :jirabug:`K8SPXC-683`: Throw an error if both backupName and backupSource options are specified to restore a backup
* :jirabug:`K8SPXC-637`: Improve crash recovery message
* :jirabug:`K8SPXC-627`: Add logic for choosing host with the oldest binlogs
* :jirabug:`K8SPXC-618`: Debug image doesn't have debug symbols
* :jirabug:`K8SPXC-599`: Add support for point-in-time recovery to specific transaction
* :jirabug:`K8SPXC-598`: Add support for compressed backups for PITR
* :jirabug:`K8SPXC-558`: Add a message to operator log if configuration changed from unsafe to safe
* :jirabug:`K8SPXC-536`: Keep major version for newly created clusters
* :jirabug:`K8SPXC-522`: Add support for runtimeClassName
* :jirabug:`K8SPXC-519`: operator logs need improvements for cluster wide operation

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-645`: Fix a bug causing point-in-time recovery error at collecting binlog files
* :jirabug:`K8SPXC-614`: Fix a bug due to which serviceAnnotations were not applied on cr.yaml change
* :jirabug:`K8SPXC-596`: Fix a bug due to which liveness probe for pxc container could cause zombie processes
* :jirabug:`K8SPXC-619`: Fix a bug due to which changing toleration didn't trigger reconfigure of Pods
* :jirabug:`K8SPXC-541`: Fix a bug causing proxysql-admin to crash kubernetes nodes (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-632`: PITR binlog apply error for the sequence backup-restore-backup-restore
* :jirabug:`K8SPXC-573`: Pod cluster1-pxc-0 fails with error: sed: -e expression #1, char 65: unterminated `s' command on OpenShift 4.6.9 (Thanks to Gertjan Bijl for reporting this issue)
* :jirabug:`K8SPXC-571`: Fix a bug due to which backup was bale to Percona XtraDB Cluster in unusable stage (Thanks to Dimitrij Hilt for reporting this issue)
* :jirabug:`K8SPXC-545`: Fix a bug which prevented imagePullSecret sync with the Percona XtraDB Cluster statefulset (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-620`: Fix a bug due to which backup cronjobs were created for disabled backups (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-641`: Fix a bug due to which update of secret for proxyadmin user does not work properly
* :jirabug:`K8SPXC-430`: Stop the unsafe way of using Galera Arbitrator for backups
* :jirabug:`K8SPXC-684`: Fix a bug due to which point-in-time recovery backup didn't allow specifying endpointUrl for Amazon S3 storage
* :jirabug:`K8SPXC-681`: Fix operator crash which occured if non-existing storage name specified for PITR
* :jirabug:`K8SPXC-638`: Fix unneeded delay in showing logs with ``kubectl logs`` command
* :jirabug:`K8SPXC-609`: Fix frequent HAProxy service NodePort updates which were causing issues with load balancers
* :jirabug:`K8SPXC-576`: Fix a bug which prevented adding/removing labels to Pods without downtime
* :jirabug:`K8SPXC-542`: fix a bug due to which daily backups were not taken from all clusters
* :jirabug:`CLOUD-611`: Stop using the already deprecated runtime/scheme package (Thanks to Jerome Küttner for reporting this issue)
