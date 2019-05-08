Custom Resource options
==============================================================

The operator is configured via the spec section of the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file. This file contains the following spec sections to configure three main subsystems of the cluster: 

| Key      | Value Type | Description                               |
|----------|------------|-------------------------------------------|
| pxc      | subdoc     | Percona XtraDB Cluster general section    |
| proxysql | subdoc     | ProxySQL section                          |
| pmm      | subdoc     | Percona Monitoring and Management section |
| backup   | subdoc     | Percona XtraDB Cluster backups section    |

### PXC Section

The ``pxc`` section in the deploy/cr.yaml file contains general configuration options for the Percona XtraDB Cluster.

| Key                            | Value Type | Example   | Description |
|--------------------------------|------------|-----------|-------------|
|size                            | int        | `3`       |  The size of the Percona XtraDB Cluster, must be >= 3 for [High-Availability](hhttps://www.percona.com/doc/percona-xtradb-cluster/5.7/intro.html) |
| allowUnsafeConfigurations      | string     | `false`   | Prevents users from configuring a cluster with unsafe parameters |
|image                           | string     |`percona/percona-xtradb-cluster-operator:1.0.0-pxc` | Percona XtraDB Cluster docker image to use                                                                     |
|configuration                   | string     |<code>&#124;</code><br>`      [mysqld]`<br>`      wsrep_debug=ON`<br>`      [sst]`<br>`      wsrep_debug=ON` | The `my.cnf` file options which are to be passed to Percona XtraDB Cluster nodes                                                                   |
|imagePullSecrets.name           | string     | `private-registry-credentials` | [Kubernetes imagePullSecret](https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets) for the Percona XtraDB Cluster docker image |
|priorityClassName               | string     | `high-priority`  | The [Kuberentes Pod priority class](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass) |
|annotations | label |`iam.amazonaws.com/role: role-arn`| The [Kubernetes annotations](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/) metadata                             |
|labels                          | label      | `rack: rack-22` | The [Kubernetes affinity labels](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/)    
|resources.requests.memory       | string     | `1G`      | [Kubernetes Memory requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a PXC container                                                               |
|resources.requests.cpu          | string     | `600m`    | [Kubernetes CPU requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a PXC container |
|resources.limits.memory         | string     | `1G`      | [Kubernetes Memory limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a PXC container |
|resources.limits.cpu            | string     | `1`       | [Kubernetes CPU limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a PXC container |
|nodeSelector                    | label      | `disktype: ssd`        | The [Kubernetes nodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector) constraint|
|affinity.topologyKey            | string     |`kubernetes.io/hostname`| The [Operator topologyKey](./constraints) node anti-affinity constraint|
|affinity.advanced               | subdoc     |           | If available, it makes [topologyKey](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature) node affinity constraint to be ignored |
|affinity.tolerations                     | subdoc     | `node.alpha.kubernetes.io/unreachable` | The [Kubernetes Pod tolerations] (https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/#concepts)            |
| podDisruptionBudget.maxUnavailable   | int  | `1`    | [Kubernetes Disruption Budget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) The number of pods unavailable after eviction| 
| podDisruptionBudet.minAvailable      | int  | `0`   | [Kubernetes Disruption Budget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) The number of pods available after eviction |
|volumeSpec.emptyDir      | string     | `{}`    | [Kubernetes emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir), i.e. the directory which will be created on a node, and will be accessible to the PXC Pod containers|
|volumeSpec.hostPath.path | string     | `/data` | [Kubernetes hostPath volume](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath), i.e. the file or directory of a node that will be accessible to the PXC Pod containers|
|volumeSpec.hostPath.type | string     |`Directory`| The [Kubernetes hostPath volume type](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath) |
|volumeSpec.persistentVolumeClaim.storageClassName | string     | `standard`| Set the [Kubernetes Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/) to use with the PXC [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims)                     |
|volumeSpec.persistentVolumeClaim.accessModes | array      | `[ "ReadWriteOnce" ]` | [Kubernetes Persistent Volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) access modes for the PerconaXtraDB Cluster  |
|volumeSpec.resources.requests.storage | string     | `6Gi`     | The [Kubernetes Persistent Volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) size for the Percona XtraDB Cluster                            |

### ProxySQL Section

The ``proxysql`` section in the deploy/cr.yaml file contains configuration options for the ProxySQL daemon.

| Key                            | Value Type | Example   | Description |
|--------------------------------|------------|-----------|-------------|
|enabled                         | boolean    | `true`    | Enables or disables [load balancing with ProxySQL](https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html) [Service](https://kubernetes.io/docs/concepts/services-networking/service/) |
|size                            | int        | `1`       | The number of the ProxySQL daemons [to provide load balancing](https://www.percona.com/doc/percona-xtradb-cluster/5.7/howtos/proxysql.html), must be = 1 in current release|
|image                           | string     |`percona/percona-xtradb-cluster-operator:1.0.0-proxysql` | ProxySQL docker image to use |
|imagePullSecrets.name           | string     | `private-registry-credentials` | [Kubernetes imagePullSecret](https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets) for the ProxySQL docker image |
|annotations | label |`iam.amazonaws.com/role: role-arn`| The [Kubernetes annotations](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/) metadata                             |
|labels                          | label      | `rack: rack-22` | The [Kubernetes affinity labels](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/)                       |
|resources.requests.memory       | string     | `1G`      | [Kubernetes Memory requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a ProxySQL container                                                      |
|resources.requests.cpu          | string     | `600m`    | [Kubernetes CPU requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a ProxySQL container                                                               |
|resources.limits.memory| string     | `1G`      | [Kubernetes Memory limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a ProxySQL container                                                               |
|resources.limits.cpu   | string     | `700m`    | [Kubernetes CPU limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for a ProxySQL container                                                               |
|priorityClassName               | string     | `high-priority`  | The [Kuberentes Pod priority class](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#priorityclass) for ProxySQL |
|nodeSelector           | label      | `disktype: ssd`        | The [Kubernetes nodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector) affinity constraint|
|affinity.topologyKey            | string     |`failure-domain.beta.kubernetes.io/zone`| The [Operator topologyKey](./constraints) node anti-affinity constraint|
|affinity.advanced               | subdoc     |           | If available, it makes [topologyKey](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature) node affinity constraint to be ignored |
|affinity.tolerations            | subdoc     | `node.alpha.kubernetes.io/unreachable` | The [Kubernetes Pod tolerations] (https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/#concepts)            |
|volumeSpec.emptyDir      | string     | `{}`    | [Kubernetes emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir), i.e. the directory which will be created on a node, and will be accessible to the ProxySQL Pod containers|
|volumeSpec.hostPath.path | string     | `/data` | [Kubernetes hostPath volume](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath), i.e. the file or directory of a node that will be accessible to the ProxySQL Pod containers|
|volumeSpec.hostPath.type | string     |`Directory`| The [Kubernetes hostPath volume type](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath) |
|volumeSpec.persistentVolumeClaim.storageClassName | string     | `standard`| The [Kubernetes Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/) to use with the ProxySQL [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims)            |
|volumeSpec.persistentVolumeClaim.accessModes | array      | `[ "ReadWriteOnce" ]` | [Kubernetes Persistent Volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) access modes for ProxySQL  |
|volumeSpec.resources.requests.storage | string     | `2Gi`     | The [Kubernetes Persistent Volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) size for ProxySQL                             |
| podDisruptionBudget.maxUnavailable   | int  | `1`    | [Kubernetes Disruption Budget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) The number of pods unavailable after eviction| 
| podDisruptionBudet.minAvailable      | int  | `0`   | [Kubernetes Disruption Budget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) The number of pods available after eviction |
| gracePeriod | int | `30` | [Kubernetes Grace period.](https://kubernetes.io/docs/concepts/workloads/pods/pod/#termination-of-pods)|

### PMM Section

The ``pmm`` section in the deploy/cr.yaml file contains configuration options for Percona Monitoring and Management.

| Key       | Value Type | Example               | Description                    |
|-----------|------------|-----------------------|--------------------------------|
|enabled    | boolean    | `false`               | Enables or disables [monitoring Percona XtraDB Cluster with PMM](https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm) |
|image      | string     |`perconalab/pmm-client`| PMM Client docker image to use |
|serverHost | string     | `monitoring-service`  | Address of the PMM Server to collect data from the Cluster |
|serverUser | string     | `pmm`                 | The [PMM Server user](https://www.percona.com/doc/percona-monitoring-and-management/glossary.option.html#term-server-user). The PMM Server Password should be configured via secrets. |

## backup section

The ``backup`` section in the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file contains the following configuration options for the regular Percona XtraDB Cluster backups.

| Key                            | Value Type | Example   | Description |
|--------------------------------|------------|-----------|-------------|
|image                           | string     | `perconalab/percona-xtradb-cluster-operator:0.4.0-backup` | Percona XtraDB Cluster docker image to use for the backup functionality                                                                       |
|imagePullSecrets.name           | string     | `private-registry-credentials`  | [Kubernetes imagePullSecret](https://kubernetes.io/docs/concepts/configuration/secret/#using-imagepullsecrets) for the specified docker image |
|storages.type                   | string     | `s3`      | Type of the cloud storage to be used for backups. Currently only `s3` and `filesystem` types are supported |
|storages.s3.credentialsSecret   | string     | `my-cluster-name-backup-s3`| [Kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/) for backups. It should contain `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` keys. |
|storages.s3.bucket              | string     |           | The [Amazon S3 bucket](https://docs.aws.amazon.com/en_us/AmazonS3/latest/dev/UsingBucket.html) name for backups     |
|storages.s3.region              | string     |`us-east-1`| The [AWS region](https://docs.aws.amazon.com/en_us/general/latest/gr/rande.html) to use. Please note **this option is mandatory** not only for Amazon S3, but for all S3-compatible storages.|
|storage.s3.endpointUrl | string|           | The endpoint URL of the S3-compatible storage to be used (not needed for the original Amazon S3 cloud) |
|storages.persistentVolumeClaim.type                    | string    | `filesystem` | persistent volume type |
|storages.persistentVolumeClaim.storageClassName | string | `standard`| Set the [Kubernetes Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/) to use with the PXC backups [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) for the `filesystem` storage type                    |
|storages.persistentVolumeClaim.accessModes | array | ["ReadWriteOnce"] | The [Kubernetes Persistent Volume access modes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes) |
|storage | string | `6Gi`| the storage size |
|schedule.name                      | string     | `sat-night-backup` | The backup name    |
|schedule.schedule                  | string     | `0 0 * * 6`        | Scheduled time to make a backup, specified in the [crontab format](https://en.wikipedia.org/wiki/Cron)                                                        |
|schedule.keep                   | int        | `3`       | Number of backups to store     |
|schedule.storageName               | string     | `st-us-west`       | Name of the storage for backups, configured in the `storages` or `fs-pvc` subsection                |
