Scale Percona XtraDB Cluster on Kubernetes and OpenShift
----------------------------------------------------------

One of the great advantages brought by Kubernetes and the OpenShift platform is the ease of an application scaling. Scaling a Deployment up or down ensures new Pods are created and set to available Kubernetes nodes.

Size of the cluster is controlled by a `size` key in the Custom Resource options configuration, as specified in the [Operator Options section](../configure/operator). That's why scaling the cluster needs nothing more but changing this option and applying the updated configuration file. This may be done in a specifically saved config, or on the fly, using the following command, which saves the current configuration, updates it and applies the changed version:

   ```bash
   $ kubectl get pxc/my-cluster -o yaml | sed -e 's/size: 3/size: 5/' | kubectl apply -f -
   ```

In this example we have changed the size of the Percona XtraDB Cluster from `3`, which is a minimum recommended value, to `5` nodes.

**Note:** *Using ```kubectl scale StatefulSet_name``` command to rescale Percona XtraDB Cluster is not recommended, as it makes `size` configuration option out of sync, and the next config change may result in reverting the previous number of nodes.*

A user can increase the size of existing volume by editing the `PersistentVolumeClaim`(PVC)object. Shrinking a persistent volume is not supported.

```bash
# backup the CR configuration file
kubectl get pxc cluster cluster1 -o yaml --export > CR_backup.yaml

# remove the cluster
kubectl delete -f CR_backup.yaml

# resize the PVC for each module
kubectl edit pvc datadir-cluster1-pxc-node-0
kubectl edit pvc datadir-cluster1-pxc-node-0
kubectl edit pvc datadir-cluster1-pxc-node-0

# change the PVC size in the CR configuration file
vim CR_backup.yaml

# apply the updated configuration
kubectl apply -f CR_backup.yaml
```
For each node, edit the spec: storage line.
```bash
spec:
   accessModes:
   - ReadWriteOnce
   datasource: null
   resources:
     requests:
       storage: 6Gi
```
