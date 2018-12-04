Custom Resource options
==============================================================

The operator is configured via the spec section of the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-server-mongodb-operator/blob/master/deploy/cr.yaml) file. This file contains the following spec sections: 

| Key | Value Type | Default | Description |
|-----|------------|---------|-------------|
|platform | string | kubernetes | Override/set the Kubernetes platform: *kubernetes* or *openshift*. Set openshift on OpenShift 3.11+ |
| version | string | 3.6.8      | The Dockerhub tag of [percona/percona-server-mongodb](https://hub.docker.com/r/perconalab/percona-server-mongodb-operator/tags/) to deploy |
| secrets | subdoc |            | Operator secrets section  |
|replsets | array  |            | Operator MongoDB Replica Set section |
| mongod  | subdoc |            | Operator MongoDB Mongod configuration section |

### Secrets section

Each spec in its turn may contain some key-value pairs. The secrets one has only two of them:

| Key | Value Type | Default | Description |
|-----|------------|---------|-------------|
|key  | string     | my-cluster-name-mongodb-key   | The secret name for the [MongoDB Internal Auth Key](https://docs.mongodb.com/manual/core/security-internal-authentication/). This secret is auto-created by the operator if it doesn't exist |
|users| string     | my-cluster-name-mongodb-users | The secret name for the MongoDB users required to run the operator. **This secret is required to run the operator!** |

### Replsets section

The replsets section controls the MongoDB Replica Set. 

| Key | Value Type | Default | Description |
|-----|------------|---------|-------------|
|name | string     | rs0     | The name of the [MongoDB Replica Set](https://docs.mongodb.com/manual/replication/) |
|size | int        | 3       | The size of the MongoDB Replica Set, must be >= 3 for [High-Availability](https://docs.mongodb.com/manual/replication/#redundancy-and-data-availability) |
|storageClass|string|        | Set the [Kubernetes Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/) to use with the MongoDB [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) |
|resources.limits.cpu|string| |[Kubernetes CPU limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for MongoDB container |
|resources.limits.memory|string| | [Kubernetes Memory limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for MongoDB container |
|resources.limits.storage|string| | [Kubernetes Storage limit](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) |
|resources.requests.cpu |string|  | [Kubernetes CPU requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for MongoDB container |
|resources.requests.memory|string| | [Kubernetes Memory requests](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#resource-requests-and-limits-of-pod-and-container) for MongoDB container |

### Mongod Section

The largest section in the deploy/cr.yaml file contains the Mongod configuration options.

| Key | Value Type | Default | Description |
|-----|------------|---------|-------------|
|net.port |       int | 27017    | Sets the MongoDB ['net.port' option](https://docs.mongodb.com/manual/reference/configuration-options/#net.port)    |
|net.hostPort|    int | 0        | Sets the Kubernetes ['hostPort' option](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/#support-hostport) |
security.redactClientLogData|bool|false|Enables/disables [PSMDB Log Redaction](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/log-redaction.html)|
|setParameter.ttlMonitorSleepSecs|int|60|Sets the PSMDB 'ttlMonitorSleepSecs' option|
|setParameter.wiredTigerConcurrentReadTransactions| int|128|Sets the ['wiredTigerConcurrentReadTransactions' option](https://docs.mongodb.com/manual/reference/parameters/#param.wiredTigerConcurrentReadTransactions) |
|setParameter.wiredTigerConcurrentWriteTransactions|int|128|Sets the ['wiredTigerConcurrentWriteTransactions' option](https://docs.mongodb.com/manual/reference/parameters/#param.wiredTigerConcurrentWriteTransactions)|
|storage.engine|string|wiredTiger| Sets the ['storage.engine' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.engine)|
|storage.inMemory.inMemorySizeRatio|float|0.9|Ratio used to compute the ['storage.engine.inMemory.inMemorySizeGb' option|
|storage.mmapv1.nsSize|int|16    | Sets the 'storage.mmapv1.nsSize' option](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/inmemory.html#--inMemorySizeGB)|
|storage.mmapv1.smallfiles|bool|false| Sets the ['storage.mmapv1.smallfiles' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.mmapv1.smallFiles) |
|storage.wiredTiger.engineConfig.cacheSizeRatio|float|0.5|Ratio used to compute the ['storage.wiredTiger.engineConfig.cacheSizeGB' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.wiredTiger.engineConfig.cacheSizeGB) |
|storage.wiredTiger.engineConfig.directoryForIndexes|bool|false|Sets the ['storage.wiredTiger.engineConfig.directoryForIndexes' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.wiredTiger.engineConfig.directoryForIndexes)|
|storage.wiredTiger.engineConfig.journalCompressor|string|snappy|Sets the ['storage.wiredTiger.engineConfig.journalCompressor' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.wiredTiger.engineConfig.journalCompressor)|
|storage.wiredTiger.collectionConfig.blockCompressor|string|snappy|Sets the ['storage.wiredTiger.collectionConfig.blockCompressor' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.wiredTiger.collectionConfig.blockCompressor)|
|storage.wiredTiger.indexConfig.prefixCompression|bool|true|Sets the ['storage.wiredTiger.indexConfig.prefixCompression' option](https://docs.mongodb.com/manual/reference/configuration-options/#storage.wiredTiger.indexConfig.prefixCompression)|
|operationProfiling.mode|string|slowOp|Sets the ['operationProfiling.mode' option](https://docs.mongodb.com/manual/reference/configuration-options/#operationProfiling.mode)|
|operationProfiling.slowOpThresholdMs|int|100| Sets the ['operationProfiling.slowOpThresholdMs'](https://docs.mongodb.com/manual/reference/configuration-options/#operationProfiling.slowOpThresholdMs) option |
|operationProfiling.rateLimit|int|1|Sets the ['operationProfiling.rateLimit' option](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/rate-limit.html)|
|auditLog.destination|string| | Sets the ['auditLog.destination' option](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/audit-logging.html)|
|auditLog.format |string|BSON|Sets the ['auditLog.format' option](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/audit-logging.html)|
|auditLog.filter |string|{}  | Sets the ['auditLog.filter' option](https://www.percona.com/doc/percona-server-for-mongodb/LATEST/audit-logging.html)|

