Binding Percona XtraDB Cluster components to Specific Kubernetes/OpenShift Nodes
================================================================================

The operator does good job automatically assigning new Pods to nodes with sufficient to achieve balanced distribution accross the cluster. Still there are situations when it worth to ensure that pods will land on specific nodes: for example, to get speed advantages of the SSD equipped machine, or to reduce costs choosing nodes in a same availability zone.

Both ``pxc`` and ``proxysql`` sections of the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file contain keys which can be used to do this, depending on what is best suited for a particular situation.

### nodeSelector

``nodeSelector`` contains one or more key-value pairs. If the node is not labeled with each key-value pair from the Pod's ``nodeSelector``, the Pod will not be able to land on it.

The following example binds the Pod to any node having a self-explanatory ``disktype: ssd`` label:

   ```
   nodeSelector:
     disktype: ssd
   ```

### affinity

Affinity makes Pod eligible (or not eligible - so called "anti-affinity") to be scheduled on the node which already has Pods with specific labels. Particularly this approach is good to make sure Pods of a specific service will be in the same availability zone, or to make sure several Pods with intensive data exchange will occupy the same availability zone or, better, the same node.

The following lines use the [topologyKey buil-in node label](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#interlude-built-in-node-labels) to make Percona XtraDB Cluster Pods occupy the same node:

   ```
   affinity:
     topologyKey: "kubernetes.io/hostname"
     # advanced:
   ```

See more details on affinity [in Kubernetes documentation](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature).

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
