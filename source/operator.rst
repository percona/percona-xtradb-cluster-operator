Custom Resource options
=======================

The operator is configured via the spec section of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file. This file contains the following spec sections to configure three
main subsystems of the cluster:

.. csv-table::
    :header: "Key", "Value Type", "Description"
    :widths: 15, 15, 40


    "pxc", "subdoc", "Percona XtraDB Cluster general section"
    "proxysql", "subdoc", "ProxySQL section"
    "pmm", "subdoc", "Percona Moonitoring and Management section"
    "backup", "subdoc", "Percona XtraDB Cluster backups section"




PXC Section
-----------

The ``pxc`` section in the deploy/cr.yaml file contains general
configuration options for the Percona XtraDB Cluster.

  .. list-table:: 
      :widths: 20 30
      :header-rows: 1

      * - Key
        - Value 
      * - size
        - int
      * - allowUnsafeConfigurations
        - string
      * - image
        - string
      * - readinessDelaySec
        - int
      * - livenessDelaySec
        - int
      * - forceUnsafeBootstrap
        - string
      * - configuration
        - string
      * - imagePullSecrets.name
        - string
      * - priorityClassName
        - string
      * - annotations
        - labels
      * - labels
        - label
      * - resources.requests.memory
        - string
      * - resources.requests.cpu
        - string
      * - resources.limits.memory
        - string
      * - nodeSelector
        - label
      * - affinity.topologyKey
        - string
      * - affinity.advanced
        - subdoc
      * - affinity.tolerations
        - subdoc
      * - podDisruptionBudet.maxUnavailable
        - int
      * - podDisruptionBudet.minAvailable
        - int
      * - volumeSpec.emptyDir
        - string
      * - volumeSpec.hostPath.path
        - string
      * - volumeSpec.hostPath.type
        - string
      * - volumeSpec.persistentVolumeClaim.storageClassName
        - string
      * - volumeSpec.PersistentVolumeClaim.accessModes
        - array
      * - volumeSpec.resources.requests.storage
        - string
      * - gracePeriod
        - int
  


  ``size`` description: The size of the Percona XtraDB cluster must be >= 3 for `High Availability <https://www.percona.com/doc/percona-xtradb-cluster/5.7/intro.html>`_

  ``allowUnsafeConfigurations`` description: Prevents users from configuring a cluster with unsafe parameters such as starting the cluster with less than 3 nodes or starting the cluster without TLS/SSL certificates

  ``image`` description:  The Docker image of the Percona cluster used.

  ``readinessDelaySec`` description: Adds a delay before a run check to verify the application is ready to process traffic

  ``livenessDelaySec`` description: Adds a delay before the run check ensures the application is healthy and capable of processing requests

  ``forceUnsafeBootstrap`` description: The setting can be reset in case of a sudden crash when all nodes may be considered unsafe to bootstrap from. The setting lets a node be selected and set to `safe_to_bootstrap` and provides data recovery.

  ``configuration`` description: The ``my.cnf`` file options to be passed to Percona XtraDB cluster nodes.

  ``imagePullSecrets.name`` description: The `Kubernetes ImagePullSecret <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_

  ``priorityClassName`` description: The `Kubernetes Pod priority class <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass>`_

  ``annotations`` description: The `Kubernetes annotations <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_

  ``labels`` description: The `Labels are key-value pairs attached to objects. <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_

  ``resources.requests.memory`` description: The `Kubernetes memory requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.

  ``resources.requests.cpu`` description: The `Kubernetes CPU requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.

  ``resources.limits.memory`` description: The `Kubernetes memory limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a PXC container.

  ``nodeSelector`` description: The `Kubernetes nodeSelector <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_

  ``affinity.topologyKey`` description: "The Operator topology key `constraints`_ node anti-affinity constraint"

  ``affinity.advanced`` description: "In cases where the pods require complex tuning the `advanced` option turns off the `topologykey` effect. This setting allows the standard Kubernetes affinity constraints of any complexity to be used."

  ``affinity.tolerations`` description: The `Kubernetes pod tolerations <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_

  ``podDisruptionBudet.maxUnavailable`` description: The `Kubernetes podDisruptionBudget <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ specifies the number of pods from the set unavailable after the eviction.

  ``podDisruptionBudet.minAvailable`` description: The `Kubernetes podDisruptionBudet <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ defines the number of pods that must be available after an eviction.

  ``volumeSpec.emptyDir`` description: The `Kubernetes emptyDir volume <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created on a node and accessible to the PXC pod containers.

  ``volumeSpec.hostPath.path`` description: The `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ The volume that mounts a directory from the host node's filesystem into your pod. The path property is required.

  ``volumeSpec.hostPath.type`` description: The `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ An optional property for the hostPath.

  ``volumeSpec.persistentVolumeClaim.storageClassName`` description: "Set the `Kubernetes storage class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC `PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_"

  ``volumeSpec.PersistentVolumeClaim.accessModes`` description: The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster.

  ``volumeSpec.resources.requests.storage`` description: The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ size for the Percona XtraDB cluster.

  ``gracePeriod`` description: The `Kubernetes grace period when terminating a pod <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_

ProxySQL Section
----------------

The ``proxysql`` section in the deploy/cr.yaml file contains
configuration options for the ProxySQL daemon.

  .. list-table:: 
      :header-rows: 1
      :widths: 20 30
    
      * - Key
        - Value
      * - enabled
        - boolean
      * - size
        - int
      * - image
        - string
      * - imagePullSecrets.name
        - string
      * - annotations
        - label
      * - labels
        - label
      * - resources.requests.memory
        - string
      * - resources.requests.cpu
        - string
      * - resources.limits.memory
        - string
      * - resources.limits.cpu
        - string
      * - priorityClassName
        - string
      * - nodeSelector
        - label
      * - affinity.topologyKey
        - string
      * - affinity.advanced
        - subdoc
      * - affinity.tolerations
        - subdoc
      * - volumeSpec.emptyDir
        - string
      * - volumeSpec.hostPath.path
        - string
      * - volumeSpec.hostPath.type
        - string
      * - volumeSpec.persistentVolumeClaim.storageClassName
        - string
      * - volumeSpec.PersistentVolumeClaim.accessModes
        - array
      * - volumeSpec.resources.requests.storage
        - string
      * - podDisruptionBudet.maxUnavailable
        - int
      * - podDisruptionBudet.minAvailable
        - int
      * - gracePeriod
        - int

  

  ``enabled`` description: "Enables or disables `load balancing with ProxySQL <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`_ `Services <https://kubernetes.io/docs/concepts/services-networking/service/>`_"

  ``size`` description: The number of the ProxySQL daemons `to provide load balancing <https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html>`_ must be = 1 in current release.

  ``image`` description: ProxySQL Docker image to use.

  ``imagePullSecrets.name`` description: The `Kubernetes imagePullSecrets <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for the ProxySQL image.

  ``annotations`` description: `Kubernetes annotations <https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/>`_ metadata.

  ``labels`` description: `Labels are key-value pairs attached to objects. <https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/>`_

  ``resources.requests.memory`` description: `Kubernetes memory requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.

  ``resources.requests.cpu`` description: `Kubernetes CPU requests <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.

  ``resources.limits.memory`` description: `Kubernetes memory limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.

  ``resources.limits.cpu`` description: `Kubernetes CPU limits <https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container>`_ for a ProxySQL container.

  ``priorityClassName`` description: The `Kubernetes Pod Priority class <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass>`_ for ProxySQL.

  ``nodeSelector`` description: `Kubernetes nodeSelector <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector>`_

  ``affinity.topologyKey`` description: "The Operator topology key `constraints`_ node anti-affinity constraint"

  ``affinity.advanced`` description: "If available it makes a `topologyKey <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature>`_ node affinity constraint to be ignored."

  ``affinity.tolerations`` description:  `Kubernetes pod tolerations <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`_

  ``volumeSpec.emptyDir`` description: `Kubernetes emptyDir volume <https://kubernetes.io/docs/concepts/storage/volumes/#emptydir>`_ The directory created on a node and accessible to the PXC pod containers.

  ``volumeSpec.hostPath.path`` description: `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ The volume that mounts a directory from the host node's filesystem into your pod. The path property is required.

  ``volumeSpec.hostPath.type`` description:  `Kubernetes hostPath <https://kubernetes.io/docs/concepts/storage/volumes/#hostpath>`_ An optional property for the hostPath.

  ``volumeSpec.persistentVolumeClaim.storageClassName`` description:  "Set the `Kubernetes storage class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC `PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_"

  ``volumeSpec.PersistentVolumeClaim.accessModes`` description:  The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ access modes for the Percona XtraDB cluster.

  ``volumeSpec.resources.requests.storage`` description:  The `Kubernetes PersistentVolumeClaim <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ size for the Percona XtraDB cluster.

  ``podDisruptionBudet.maxUnavailable`` description:  `Kubernetes podDisruptionBudget <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ specifies the number of pods from the set unavailable after the eviction.

  ``podDisruptionBudet.minAvailable`` description:  `Kubernetes podDisruptionBudet <https://kubernetes.io/docs/tasks/run-application/configure-pdb/#specifying-a-poddisruptionbudget>`_ the number of pods that must be available after an eviction.

  ``gracePeriod`` description:  The `Kubernetes grace period when terminating a pod <https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods>`_


PMM Section
-----------

The ``pmm`` section in the deploy/cr.yaml file contains configuration
options for Percona Monitoring and Management.

  .. list-table:: 
      :header-rows: 1
      :widths: 20 30
    
      * - Key
        - Value
      * - enabled
        - boolean
      * - image
        - string
      * - serverHost
        - string
      * - serverUser
        - string

  ``enabled`` description: Enables or disables `monitoring Percona XtraDB cluster with PMM <https://www.percona.com/doc/percona-xtradb-cluster/5.7/manual/monitoring.html>`_

  ``image`` description:  PMM client Docker image to use.

  ``serverHost`` description:  Address of the PMM Server to collect data from the cluster.

  ``serverUser`` description:  The `PMM Serve_User <https://www.percona.com/doc/percona-monitoring-and-management/glossary.option.html>`_. The PMM Server password should be configured using Secrets.


backup section
--------------

The ``backup`` section in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
file contains the following configuration options for the regular
Percona XtraDB Cluster backups.

  .. list-table:: 
      :header-rows: 1
      :widths: 20 30
    
      * - Key
        - Value
      * - image
        - string
      * - imagePullSecrets.name
        - string
      * - storages.type
        - string
      * - storages.s3.credentialsSecret
        - string
      * - storages.s3.bucket
        - string
      * - storages.s3.region
        - string
      * - storages.s3.endpointUrl
        - string
      * - storages.persistentVolumeClaim.type
        - string
      * - storages.persistentVolumeClaim.storageClassName
        - string
      * - storages.persistentVolumeClaim.accessModes
        - array
      * - storages.persistentVolumeClaim.storage
        - string
      * - schedule.name
        - string
      * - schedule.schedule
        - string
      * - schedule.keep
        - int
      * - schedule.storageName
        - string


  ``image`` descriptions: The Percona XtraDB cluster Docker image to use for the backup.

  ``imagePullSecrets.name`` descriptions:  The `Kubernetes imagePullSecrets <https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets>`_ for the specified image.

  ``storages.type`` descriptions:  The cloud storage type used for backups. Only ``s3`` and ``filesystem`` types are supported.

  ``storages.s3.credentialsSecret`` descriptions:  The `Kubernetes secret <https://kubernetes.io/docs/concepts/configuration/secret/>`_ for backups. It should contain ``AWS_ACCESS_KEY_ID`` and ``AWS_SECRET_ACCESS_KEY`` keys.

  ``storages.s3.bucket`` descriptions:  The `Amazon S3 bucket <https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingBucket.html>`_ name for backups.

  ``storages.s3.region`` descriptions:  The `AWS region <https://docs.aws.amazon.com/general/latest/gr/rande.html>`_ to use. Please note ** this option is mandatory** for Amazon and all S3-compatible storages.

  ``storages.s3.endpointUrl`` descriptions:  The endpoint URL of the S3-compatible storage to be used (not needed for the original Amazon S3 cloud).

  ``storages.persistentVolumeClaim.type`` descriptions:  The persistent volume claim storage type

  ``storages.persistentVolumeClaim.storageClassName`` descriptions:  Set the `Kubernetes Storage Class <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ to use with the PXC backups `PersistentVolumeClaims <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims>`_ for the ``filesystem`` storage type.

  ``storages.persistentVolumeClaim.accessModes`` descriptions:  The `Kubernetes PersistentVolume access modes <https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes>`_

  ``storages.persistentVolumeClaim.storage`` descriptions: Storage size for the PersistentVolume.

  ``schedule.name`` descriptions:  The backup name

  ``schedule.schedule`` descriptions:  Scheduled time to make a backup specified in the `crontab format <https://en.wikipedia.org/wiki/Cron>`_

  ``schedule.keep`` descriptions:  Number of stored backups

  ``schedule.storageName`` descriptions: The name of the storage for the backups configured in the ``storages`` or ``fs-pvc`` subsection.
