.. rn:: 1.8.0

================================================================================
*Percona Kubernetes Operator for Percona XtraDB Cluster* 1.8.0
================================================================================

:Date: April 13, 2021
:Installation: `Installing Percona Kubernetes Operator for Percona XtraDB Cluster <https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html#quickstart-guides>`_

Release Highlights
================================================================================

* It is now `possible <https://www.percona.com/doc/kubernetes-operator-for-pxc/scaling.html>`_
  to use ``kubectl scale`` command to scale Percona XtraDB Cluster horizontally
  (adding or removing Replica Set instances). You can also use  `Horizontal Pod
  Autoscaler <https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/>`_
  which will scale your database cluster based on various metrics, such as CPU utilization. 
* Support for :ref:`custom sidecar containers<faq-sidecar>`. The Operator makes
  it possible now to deploy additional (sidecar) containers to the Pod. This
  feature can be useful to run debugging tools or some specific monitoring
  solutions, etc. The sidecar container can be added to 
  :ref:`pxc<pxc-sidecars-image>`,
  :ref:`haproxy<haproxy-sidecars-image>`, and
  :ref:`proxysql<proxysql-image>` sections of the ``deploy/cr.yaml``
  configuration file.

New Features
================================================================================

* :jirabug:`K8SPXC-528`: Support for :ref:`custom sidecar containers<faq-sidecar>`
  to extend the Operator capabilities
* :jirabug:`K8SPXC-647`: Allow the cluster :ref:`scale in and scale out<operator-scale>`
  with the ``kubectl scale`` command or Horizontal Pod Autoscaler
* :jirabug:`K8SPXC-643`: Operator can now automatically recover Percona XtraDB
  Cluster after the `network partitioning <https://en.wikipedia.org/wiki/Network_partition>`_

Improvements
================================================================================

* :jirabug:`K8SPXC-654`: Use MySQL administrative port for Kubernetes
  liveness/readiness probes to avoid false positive failures
* :jirabug:`K8SPXC-442`: The Operator can now automatically remove old backups
  from S3 if the retention period is set (thanks to Davi S Evangelista for reporting this issue)
* :jirabug:`K8SPXC-697`: Add namespace support in the script used to
  :ref:` script used to copy backups<backups-copy>` from remote storage to a
  local machine
* :jirabug:`K8SPXC-627`: Make log collector choosing Pod with the oldest binlog
  in the cluster in case of failed log uploading
* :jirabug:`K8SPXC-618`: Add debug symbols to Percona XtraDB Cluster debug
  docker image to simplify troubleshooting
* :jirabug:`K8SPXC-599`: It is now possible to recover databases up to specific
  transactions with the Point-in-time Recovery feature. Previously the user
  could only recover to specific date and time.
* :jirabug:`K8SPXC-598`: Point-in-time recovery feature now works with
  compressed backups
* :jirabug:`K8SPXC-536`: It is now possible to explicitly set the version of
  Percona XtraDB Cluster for newly provisioned clusters. Before that, all new
  clusters were started with the latest PXC version if Version Service was
  enabled
* :jirabug:`K8SPXC-522`: Add support for the ``runtimeClassName`` Kubernetes
  feature for selecting the container runtime
* K8SPXC-519, K8SPXC-558, and K8SPXC-637: Improve various log messages for better
  clearness and more precise description

Known Issues and Limitations
================================================================================

* Scheduled backups are not compatible with the Kubernetes 1.20 in cluster-wide
  mode.

Bugs Fixed
================================================================================

* :jirabug:`K8SPXC-645`: Fix a bug causing point-in-time recovery error at collecting binlog files
* :jirabug:`K8SPXC-614`, :jirabug:`K8SPXC-619`, :jirabug:`K8SPXC-545`, :jirabug:`K8SPXC-641`, :jirabug:`K8SPXC-576`: Fix multiple bugs due to which changes of various objects in ``deploy/cr.yaml`` were not applied to the running cluster (thanks to Sergiy Prykhodko for reporting some of these issues)
* :jirabug:`K8SPXC-596`: Fix a bug due to which liveness probe for pxc container could cause zombie processes
* :jirabug:`K8SPXC-632`: Fix a bug preventing point-in-time recovery if multiple clusters uploaded binary logs to a single S3 bucket 
* :jirabug:`K8SPXC-573`: Fix a bug that prevented using special characters in XtraBackup password (thanks to Gertjan Bijl for reporting this issue)
* :jirabug:`K8SPXC-571`: Fix a bug due to which backup was bale to Percona XtraDB Cluster in unusable stage (Thanks to Dimitrij Hilt for reporting this issue)
* :jirabug:`K8SPXC-545`: Fix a bug which prevented imagePullSecret sync with the Percona XtraDB Cluster statefulset (Thanks to Sergiy Prykhodko for reporting this issue)
* :jirabug:`K8SPXC-430`: Stop the unsafe way of using Galera Arbitrator for backups
* :jirabug:`K8SPXC-684`: Fix a bug due to which point-in-time recovery backup didn't allow specifying endpointUrl for Amazon S3 storage
* :jirabug:`K8SPXC-681`: Fix operator crash which occured if non-existing storage name specified for PITR
* :jirabug:`K8SPXC-638`: Fix unneeded delay in showing logs with ``kubectl logs`` command for the logs container
* :jirabug:`K8SPXC-609`: Fix frequent HAProxy service NodePort updates which were causing issues with load balancers
* :jirabug:`K8SPXC-542`: Fix a bug due to which  backups were taken only for one cluster out of many controlled by one Operator
* :jirabug:`CLOUD-611`: Stop using the already deprecated runtime/scheme package (Thanks to Jerome Küttner for reporting this issue)
