Changing MySQL Options
============================================================================

MySQL allows the option to configure the database with a configuration file.

The cluster can change the configuration in the following ways:
* Edit the configuration section of the operator. See the [PXC section]( https://percona.github.io/percona-xtradb-cluster-operator/configure/operator).
* Create a configmap.yaml

A configmap allows a user to separate configurations from the pods. Configmaps store unencrypted configuration settings. For sensitive information, the user should use Secrets.

### Data Section

The `data` section of the configmap file contains the configuration settings for the Percona XtraDB Cluster.

Key               |  Value Type | Example           | Description
------------------|-------------|-------------------|-------------------------
init.cnf  | String | |
                      [mysqld]
                       max_connections = 250 | The `my.cnf` file options passed to the Percona XtrDB Cluster nodes

```bash
kubectl apply -f configmap.yaml
```

Restart the cluster and connect to the MySQL instance.

Verify that the max_connections value has changed:
```bash
show variables like "max_connections";

Variable_name     Value
max_connections   250
```  
