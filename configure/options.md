Changing MySQL Options
============================================================================

During application deployments on an XtraDB cluster, we may require a change to MySQL configuration. Changing the configuration would require a source code change, commit the change, and perform the complete deployment process. This process could be considered unwieldy for a simple set of changes.

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

With a configuration change and a restart of the cluster, you can use a configmap to set configuration options. A configmap allows Kubernetes to pass or update configuration data inside a containerized application.

There are several ways you can add a configmap to the cluster:
* Apply a configmap as a yaml file
* Create from an external resource

#### Apply a file
With a text editor, you can define a configmap and save the configmap as a yaml file to the deploy folder.

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
Use the `kubectl` command to create the configmap from external resources, for more information see [Configure a Pod to use a ConfigMap](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#create-a-configmap).

Your application requires more connections. To increase your max_connections setting in MySQL, you define a my.cnf configuration file with the following setting:
```
[mysqld]
...
max_connections=250
```
To add the configuration setting to the XtraDB Cluster, you can create a configmap from the my.cnf file with the 'kubectl create configmap' command.

You should use the XtraDB Cluster naming convention which is a combination of the cluster name with the `-pxc` suffix to name the configmap. To find the cluster name, you can use the following command:
```bash
kubectl get pxc
```
The syntax for `kubectl create configmap` command is:
```
kubectl create configmap <cluster-name-pxc> <resource-type=resource-name>
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
