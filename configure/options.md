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

### Use a ConfigMap

You can create and apply a configmap.yaml file to set Percona XtraDB Cluster configuration options. The ConfigMap allows Kubernetes to pass or update configuration data inside a containerized application.

You can use the `kubectl create configmap` command to create a configmap, see [Configure a Pod to use a ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap). For example, you can create a ConfigMap.yaml file from a my.cnf file for the cluster1-pxc cluster:

```bash
kubectl create configmap cluster1-pxc --from-file=my.cnf
```
In the configmap.yaml file, the `data` section contains the configuration settings for the Percona XtraDB Cluster.

```
apiVersion:v1
kind: ConfigMap
...
data:
  init.cnf: |
    [mysqld]
    max_connections=250
```

The user applies the ConfigMap to the cluster.
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
