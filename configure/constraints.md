Binding Percona XtraDB Cluster components to Specific Kubernetes/OpenShift Nodes
================================================================================

The operator does good job automatically assigning new Pods to nodes with sufficient to achieve balanced distribution accross the cluster. Still there are situations when it worth to ensure that pods will land on specific nodes: for example, to get speed advantages of the SSD equipped machine, or to reduce costs choosing nodes in a same availability zone.

Percona XtraDB Cluster Operator provides two approaches for doing this:

* simple way to schedule Pods to specific nodes, built-in into the Operator,
* more advanced approach based on using standard Kubernetes constraints. 


Both simple and advanced approaches are toggled in ``pxc`` and ``proxysql`` sections of the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file. Exact choice which way to use depends on what is best suited for a particular situation. 

## Simple approach - use topologyKey of the Percona XtraDB Cluster Operator 

Percona XtraDB Cluster Operator provides a ``topologyKey`` option, which may have one of the following values:

* ``kubernetes.io/hostname`` - the cluster will be within the same host,
* ``failure-domain.beta.kubernetes.io/zone`` - the cluster will be within the same zone,
* ``failure-domain.beta.kubernetes.io/region`` - the cluster will be within the same region,
* ``none`` - no constraints are applied.

The following example makes Percona XtraDB Cluster Pods occupy the same node:

   ```
   affinity:
     topologyKey: "kubernetes.io/hostname"
   ```
   
## Advanced approach - use standard Kubernetes constraints

Previous way can be used with no special knowledge of the Kubernetes way of assigning Pods to specific nodes. Still in some cases more complex tuning may be needed. In this case ``advanced`` option placed in the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file turns off the effect of the ``topologyKey`` and allows to use Kubernetes constraints which will be described in the following sections:

   ```
   affinity:
     topologyKey: "kubernetes.io/hostname"
     advanced:
   ```

### nodeSelector

``nodeSelector`` contains one or more key-value pairs. If the node is not labeled with each key-value pair from the Pod's ``nodeSelector``, the Pod will not be able to land on it.

The following example binds the Pod to any node having a self-explanatory ``disktype: ssd`` label:

   ```
   nodeSelector:
     disktype: ssd
   ```

### tolerations

*Tolerations* allow Pods having them to be able to land onto nodes with matching *taints*. Toleration is expressed as a ``key`` with and ``operator``, which is either ``exists`` or ``equal`` (the latter variant also requires a ``value`` the key is equal to). Moreover, toleration should have a specified ``effect``, which may be a self-explanatory ``NoSchedule``, less strict ``PreferNoSchedule``, or ``NoExecute``. The last variant means that if a *taint* with ``NoExecute`` is assigned to node, then any Pod not tolerating this *taint* will be removed from the node, immediately or after the ``tolerationSeconds`` interval, like in the following example:

   ```
   tolerations: 
   - key: "node.alpha.kubernetes.io/unreachable"
     operator: "Exists"
     effect: "NoExecute"
     tolerationSeconds: 6000
   ```

The [Kubernetes Taints and Toleratins](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/) contains more examples on this topic.
