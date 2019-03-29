Changing MySQL Options
============================================================================

MySQL allows the option to configure the database with a configuration file. You can pass the MySQL options from the [my.cnf](https://dev.mysql.com/doc/refman/8.0/en/option-files.html) configuration file to the cluster in one of the following ways:
* CR.yaml
* ConfigMap

### Edit the CR.yaml

Edit the configuration section of the deploy/cr.yaml. See the [PXC section]( https://percona.github.io/percona-xtradb-cluster-operator/configure/operator).

```
spec:
  secretsName: my-cluster-secrets
  pxc:
    ...
      configuration: |
        [mysqld]
        wsrep_debug=ON
        [sst]
        wsrep_debug=ON
```

### Use a Configmap

You can use a configmap.yaml file to set Percona XtraDB Cluster configuration options. [Configmap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/) allows Kubernetes to pass or update configuration data inside a containerized application.

#### Data Section

The `data` section of the configmap file contains the configuration settings for the Percona XtraDB Cluster.

| Key                            | Value Type | Example   | Description |
|--------------------------------|------------|-----------|---------                                                                   |
|configuration                   | string     |<code>&#124;</code><br>`      [mysqld]`<br>`        max_connections=250` | The `my.cnf` file options to be passed to Percona XtraDB Cluster nodes


The user applies the configmap to the cluster.
```bash
kubectl apply -f configmap.yaml
```

Restart the cluster and connect to the MySQL instance (see details on how to connect in the [Install Percona XtraDB Cluster on Kubernetes page.](https://percona.github.io/percona-xtradb-cluster-operator/install/kubernetes))

Verify that the max_connections value has changed:
```bash
show variables like "max_connections";

Variable_name     Value
max_connections   250
```  
