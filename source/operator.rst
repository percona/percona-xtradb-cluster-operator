Custom Resource options
=======================

The operator is configured via the spec section of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file. This file contains the following spec sections to configure three
main subsystems of the cluster:


.. csv-table:: Custom Resource options
    :header: "Key", "Value Type", "Description"
    :widths: 15, 15, 40
    :delim: ,

    "pxc", "subdoc", "Percona XtraDB Cluster general section"
    "proxysql", "subdoc", "ProxySQL section"
    "pmm", "subdoc", "Percona Moonitoring and Management section"
    "backup", "subdoc", "Percona XtraDB Cluster backups section"




PXC Section
-----------

The ``pxc`` section in the deploy/cr.yaml file contains general
configuration options for the Percona XtraDB Cluster.



.. csv-table:: PXC Section
  :header: "Key", "Value", "Example", "Description"
  :widths: 25, 8, 15, 25
  :delim: ,

  size, int, ``3``, The size of the Percona XtraDB cluster must be >= 3 for `High Availability <https://www.percona.com/doc/percona-xtradb-cluster/5.7/intro.html>`_
  allowUnsafeConfigurations, string,``false``, Prevents users from configuring a cluster with unsafe parameters such as starting the cluster with less than 3 nodes or starting the cluster without TLS/SSL certificates"
  image, string, ``percona/percona-xtradb-cluster-operator:1.0.0-pxc``, The Docker image of the Percona cluster used.
  readinessDelaySec, int, ``15``, Adds a delay before a run check to verify the application is ready to process traffic
  livenessDelaySec, int, ``300``, Adds a delay before the run check ensures the application is healthy and capable of processing requests
  forceUnsafeBootstrap, string, ``false``, The setting can be reset in case of a sudden crash when all nodes may be considered unsafe to bootstrap from. The setting lets a node be selected and set to `safe_to_bootstrap` and provides data recovery.
  configuration, string, ``|``   ``[mysqld]``    ``wsrep_debug=ON`` ``wsrep-provider_options=gcache.size=1G;gcache.recover=yes``, The ``my.cnf`` file options to be passed to Percona XtraDB cluster nodes.
  imagePullSecrets.name, string, ``private-registry-credentials``, The `Kubernetes ImagePullSecret <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_
  priorityClassName, string, ``high-priority``, The `Kubernetes Pod priority class <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass>`_
  annotations, label, ``iam.amazonaws.com/role: role-arn``, The `Kubernetes annotations <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_
  labels, label, ``rack: rack-22``, `Labels are key-value pairs attached to objects. <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_
  resources.requests.memory, string, ``1G``, The `Kubernetes memory requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.
  resources.requests.cpu, string, ``600m``, `Kubernetes CPU requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.
  resources.limits.memory, string, ``1G``, `Kubernetes memory limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.
  nodeSelector, label, ``disktype: ssd``, `Kubernetes nodeSelector <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_
  affinity.topologyKey, string, ``kubernetes.io/hostname``, "The Operator topology key `constraints`_ node anti-affinity constraint"
  affinity.advanced, subdoc,  , "In cases where the pods require complex tuning the `advanced` option turns off the `topologykey` effect. This setting allows the standard Kubernetes affinity constraints of any complexity to be used."
  affinity.tolerations, subdoc, ``node.alpha.kubernetes.io/unreachable``, `Kubernetes pod tolerations <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_
  podDisruptionBudet.maxUnavailable, int, ``1``, The `Kubernetes podDisruptionBudget <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ specifies the number of pods from the set unavailable after the eviction.
  podDisruptionBudet.minAvailable, int, ``0``, The `Kubernetes podDisruptionBudet <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ defines the number of pods that must be available after an eviction.
  volumeSpec.emptyDir, string, ``{}``, The `Kubernetes emptyDir volume <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created on a node and accessible to the PXC pod containers.
  volumeSpec.hostPath.path, string, ``/data``, `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ The volume that mounts a directory from the host node's filesystem into your pod. The path property is required.
  volumeSpec.hostPath.type, string, ``Directory``, The `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ An optional property for the hostPath.
  volumeSpec.persistentVolumeClaim.storageClassName, string, ``standard``, "Set the `Kubernetes storage class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC `PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_"
  volumeSpec.PersistentVolumeClaim.accessModes, array, ``[ReadWriteOnce]``, The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster.
  volumeSpec.resources.requests.storage, string, ``6Gi``, The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ size for the Percona XtraDB cluster.
  gracePeriod, int, ``600``, The `Kubernetes grace period when terminating a pod <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_

ProxySQL Section
----------------

The ``proxysql`` section in the deploy/cr.yaml file contains
configuration options for the ProxySQL daemon.

.. csv-table:: proxysql Section
  :header: "Key", "Value", "Example", "Description"
  :widths: 25, 8, 15, 25
  :delim: ,

  enabled, boolean, ``true``, "Enables or disables `load balancing with ProxySQL <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`_ `Services <https://kubernetes.io/docs/concepts/services-networking/service/>`_"
  size, int, ``1``, The number of the ProxySQL daemons `to provide load balancing <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`_ must be = 1 in current release.
  image, string, ``percona/percona-xtradb-cluster-operator:1.0.0-proxysql``, ProxySQL Docker image to use.
  imagePullSecrets.name, string, ``private-registry-credentials``, The `Kubernetes imagePullSecrets <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for the ProxySQL image.
  annotations, label, ``iam.amazonaws.com/role: role-arn``, `Kubernetes annotations <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_ metadata.
  labels, label, ``rack: rack-22``, `Labels are key-value pairs attached to objects. <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_
  resources.requests.memory, string, ``1G``, `Kubernetes memory requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.
  resources.requests.cpu, string, ``600m``, `Kubernetes CPU requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.
  resources.limits.memory, string, ``1G``, `Kubernetes memory limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.
  resources.limits.cpu, string, ``700m``, `Kubernetes CPU limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.
  priorityClassName,string,``high-priority``, The `Kubernetes Pod Priority class <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass>`_ for ProxySQL.
  nodeSelector, label, ``disktype: ssd``, `Kubernetes nodeSelector <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_
  affinity.topologyKey, string, ``kubernetes.io/hostname``, "The Operator topology key `constraints`_ node anti-affinity constraint"
  affinity.advanced, subdoc, , "If available it makes a `topologyKey <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature>`_ node affinity constraint to be ignored."
  affinity.tolerations, subdoc, """node.alpha.kubernetes.io/unreachable""", `Kubernetes pod tolerations <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_
  volumeSpec.emptyDir, string, ``{}``, `Kubernetes emptyDir volume <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created on a node and accessible to the PXC pod containers.
  volumeSpec.hostPath.path, string, ``/data``, `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ The volume that mounts a directory from the host node's filesystem into your pod. The path property is required.
  volumeSpec.hostPath.type, string, ``Directory``, `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ An optional property for the hostPath.
  volumeSpec.persistentVolumeClaim.storageClassName, string, ``standard``, "Set the `Kubernetes storage class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC `PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_"
  volumeSpec.PersistentVolumeClaim.accessModes, array, ``[ReadWriteOnce]``, The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster.
  volumeSpec.resources.requests.storage, string, ``6Gi``, The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ size for the Percona XtraDB cluster.
  podDisruptionBudet.maxUnavailable, int, ``1``, `Kubernetes podDisruptionBudget <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ specifies the number of pods from the set unavailable after the eviction.
  podDisruptionBudet.minAvailable, int, ``0``, `Kubernetes podDisruptionBudet <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ the number of pods that must be available after an eviction.
  gracePeriod, int, ``30``, The `Kubernetes grace period when terminating a pod <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_


PMM Section
-----------

The ``pmm`` section in the deploy/cr.yaml file contains configuration
options for Percona Monitoring and Management.

.. csv-table:: pmm Section
  :header: "Key", "Value", "Example", "Description"
  :widths: 25, 8,15,25
  :delim: ,

  enabled, boolean, ``false``, Enables or disables `monitoring Percona XtraDB cluster with PMM <https://www.percona.com/doc/percona-xtradb-cluster/5.7/manual/monitoring.html>`_
  image, string, ``perconalab/pmm-client:1.17.1``, PMM client Docker image to use.
  serverHost, string, ``monitoring-service``, Address of the PMM Server to collect data from the cluster.
  serverUser, string, ``pmm``, The `PMM Serve_User <https://www.percona.com/doc/percona-monitoring-and-management/glossary.option.html>`_. The PMM Server password should be configured using Secrets.


backup section
--------------

The ``backup`` section in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file contains the following configuration options for the regular
Percona XtraDB Cluster backups.

.. csv-table:: backup Section
  :header: "Key", "Value", "Example", "Description"
  :widths: 25 , 10, 15, 25
  :delim: ,

  "image", string, ``percona/percona-xtradb-cluster-operator:1.0.0-backup``, The Percona XtraDB cluster Docker image to use for the backup.
  imagePullSecrets.name, string, ``private-registry-credentials``, The `Kubernetes imagePullSecrets <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for the specified image.
  storages.type, string, ``s3``, The cloud storage type used for backups. Only ``s3`` and ``filesystem`` types are supported.
  storages.s3.credentialsSecret, string, ``my-cluster-name-backup-s3``, The `Kubernetes secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_ for backups. It should contain ``AWS_ACCESS_KEY_ID`` and ``AWS_SECRET_ACCESS_KEY`` keys.
  storages.s3.bucket, string, , The `Amazon S3 bucket <https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingBucket.html>`_ name for backups.
  storages.s3.region, string, ``us-east-1``, The `AWS region <https://docs.aws.amazon.com/general/latest/gr/rande.html>`_ to use. Please note ** this option is mandatory** for Amazon and all S3-compatible storages.
  storages.s3.endpointUrl, string, , The endpoint URL of the S3-compatible storage to be used (not needed for the original Amazon S3 cloud).
  storages.persistentVolumeClaim.type, string, ``filesystem``, The persistent volume claim storage type
  storages.persistentVolumeClaim.storageClassName, string, ``standard``, Set the `Kubernetes Storage Class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC backups `PersistentVolumeClaims <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ for the ``filesystem`` storage type.
  storages.persistentVolumeClaim.accessModes, array, ``[ReadWriteOne]``, The `Kubernetes PersistentVolume access modes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes>`_
  storages.persistentVolumeClaim.storage, string, ``6Gi``, Storage size for the PersistentVolume.
  schedule.name, string, ``sat-night-backup``, The backup name
  schedule.schedule, string, ``0 0 * * 6``, Scheduled time to make a backup specified in the `crontab format <https://en.wikipedia.org/wiki/Cron>`_
  schedule.keep, int, ``3``, Number of stored backups
  schedule.storageName, string, ``s3-us-west``, The name of the storage for the backups configured in the ``storages`` or ``fs-pvc`` subsection.
