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

You can create a configmap in a text editor and apply it with the `kubectl apply` command or use the `kubectl` command to create the configmap from external resources, for more information see [Configure a Pod to use a ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap).

#### Apply a file
You can create a configmap with a text editor and save the file to the deploy folder.

This example displays a configmap created with a text editor:

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: pxc
data:
  init.cnf: |
    [mysqld]
    wsrep_debug=ON
    [sst]
    wsrep_debug=ON
```
You apply the configmap to the cluster with the following command:
```bash
kubectl apply -f configmap.yaml
```


#### Create from external resource

To increase your max_connections setting in MySQL, you have a my.cnf file with the following setting:
```
[mysqld]
...
max_connections=250
```


An XtraDB Cluster naming convention is the configmap name is a combination of the cluster name with the `-pxc` suffix. The syntax for `kubectl create configmap` command is:
```
kubectl create configmap <cluster-name-pxc> <resource-type=resource-name>
```
 To find the cluster name, you can use the following command:
```bash
kubectl get pxc
```
The following example defines cluster1-pxc as the configmap name and the my-cnf file as the data source:

```bash
kubectl create configmap cluster1-pxc --from-file=my.cnf
```

To view the created configmap, use the following command:
```bash
kubectl describe configmaps cluster1-pxc
```

### Make changed options visible to the Percona XtraDB Cluster

Do not forget to restart Percona XtraDB Cluster to ensure the cluster has updated the configuration (see details on how to connect in the [Install Percona XtraDB Cluster on Kubernetes page.](https://percona.github.io/percona-xtradb-cluster-operator/install/kubernetes)).
