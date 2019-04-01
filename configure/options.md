Changing MySQL Options
============================================================================

MySQL allows the option to configure the database with a configuration file. You can pass the MySQL options from the [my.cnf](https://dev.mysql.com/doc/refman/8.0/en/option-files.html) configuration file to the cluster in one of the following ways:
* CR.yaml
* ConfigMap

### Edit the CR.yaml

You can add options from the [my.cnf](https://dev.mysql.com/doc/refman/8.0/en/option-files.html) by editing the configuration section of the deploy/cr.yaml.

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
See the [Custom Resource options, PXC section](https://percona.github.io/percona-xtradb-cluster-operator/configure/operator.html) for more details

### Use a ConfigMap

You create or apply a configmap file to set Percona XtraDB Cluster configuration options. The ConfigMap allows Kubernetes to pass or update configuration data inside a containerized application.


For example, to increase your max_connections setting in MySQL, you create a my.cnf file:
```
[mysqld]
...
max_connections=250
```
You can create a configmap in a text editor and apply it with the `kubectl apply` command or use the `kubectl` command to create the configmap from a directory, files, or literal values, see [Configure a Pod to use a ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap).

In this example, we use `kubectl` to create a configmap, add cnf-options as the configmap name, and use the my-cnf file as the data source:

```bash
kubectl create configmap cnf-options --from-file=my.cnf
```
In the configmap, the `data` section contains the configuration settings for the Percona XtraDB Cluster:

```
apiVersion:v1
kind: ConfigMap
...
data:
  my.cnf: |
    [mysqld]
    ...
    max_connections=250
```
### Make changed options visible to the Percona XtraDB Cluster

Do not forget to restart Percona XtraDB Cluster to ensure the cluster has updated the configuration (see details on how to connect in the [Install Percona XtraDB Cluster on Kubernetes page.](https://percona.github.io/percona-xtradb-cluster-operator/install/kubernetes)).
