Changing MySQL Options
============================================================================

MySQL allows the option to configure the database with a configuration file. You can pass the MySQL configuration options to the cluster in the following ways:
* CR.yaml
* ConfigMap

### Edit the CR.yaml

Edit the configuration section of the deploy/cr.yaml. See the [PXC section]( https://percona.github.io/percona-xtradb-cluster-operator/configure/operator).


### Use a Configmap

A configmap allows a user to separate configurations from the pods. Configmaps store unencrypted configuration settings. For sensitive information, the user should use Secrets.

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
