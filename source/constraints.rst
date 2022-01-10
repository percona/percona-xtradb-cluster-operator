Binding Percona XtraDB Cluster components to Specific Kubernetes/OpenShift Nodes
================================================================================

The operator does good job automatically assigning new Pods to nodes
with sufficient to achieve balanced distribution across the cluster.
Still there are situations when it worth to ensure that pods will land
on specific nodes: for example, to get speed advantages of the SSD
equipped machine, or to reduce costs choosing nodes in a same
availability zone.

Both ``pxc`` and ``proxysql`` sections of the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file contain keys which can be used to do this, depending on what is the
best for a particular situation.

Node selector
-------------

``nodeSelector`` contains one or more key-value pairs. If the node is
not labeled with each key-value pair from the Pod’s ``nodeSelector``,
the Pod will not be able to land on it.

The following example binds the Pod to any node having a
self-explanatory ``disktype: ssd`` label:

::

   nodeSelector:
     disktype: ssd

Affinity and anti-affinity
--------------------------

Affinity makes Pod eligible (or not eligible - so called
“anti-affinity”) to be scheduled on the node which already has Pods with
specific labels. Particularly this approach is good to to reduce costs
making sure several Pods with intensive data exchange will occupy the
same availability zone or even the same node - or, on the contrary, to
make them land on different nodes or even different availability zones
for the high availability and balancing purposes.

Percona Distribution for MySQL Operator provides two approaches for doing this:

-  simple way to set anti-affinity for Pods, built-in into the Operator,
-  more advanced approach based on using standard Kubernetes
   constraints.

Simple approach - use topologyKey of the Percona Distribution for MySQL Operator
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Percona Distribution for MySQL Operator provides a ``topologyKey`` option, which
may have one of the following values:

-  ``kubernetes.io/hostname`` - Pods will avoid residing within the same
   host,
-  ``failure-domain.beta.kubernetes.io/zone`` - Pods will avoid residing
   within the same zone,
-  ``failure-domain.beta.kubernetes.io/region`` - Pods will avoid
   residing within the same region,
-  ``none`` - no constraints are applied.

The following example forces Percona XtraDB Cluster Pods to avoid
occupying the same node:

::

   affinity:
     topologyKey: "kubernetes.io/hostname"

Advanced approach - use standard Kubernetes constraints
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Previous way can be used with no special knowledge of the Kubernetes way
of assigning Pods to specific nodes. Still in some cases more complex
tuning may be needed. In this case ``advanced`` option placed in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file turns off the effect of the ``topologyKey`` and allows to use
standard Kubernetes affinity constraints of any complexity:

::

   affinity:
      advanced:
        podAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: security
                operator: In
                values:
                - S1
            topologyKey: failure-domain.beta.kubernetes.io/zone
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: security
                  operator: In
                  values:
                  - S2
              topologyKey: kubernetes.io/hostname
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/e2e-az-name
                operator: In
                values:
                - e2e-az1
                - e2e-az2
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 1
            preference:
              matchExpressions:
              - key: another-node-label-key
                operator: In
                values:
                - another-node-label-value

See explanation of the advanced affinity options `in Kubernetes
documentation <https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#inter-pod-affinity-and-anti-affinity-beta-feature>`__.

Tolerations
-----------

*Tolerations* allow Pods having them to be able to land onto nodes with
matching *taints*. Toleration is expressed as a ``key`` with and
``operator``, which is either ``exists`` or ``equal`` (the latter
variant also requires a ``value`` the key is equal to). Moreover,
toleration should have a specified ``effect``, which may be a
self-explanatory ``NoSchedule``, less strict ``PreferNoSchedule``, or
``NoExecute``. The last variant means that if a *taint* with
``NoExecute`` is assigned to node, then any Pod not tolerating this
*taint* will be removed from the node, immediately or after the
``tolerationSeconds`` interval, like in the following example:

::

   tolerations:
   - key: "node.alpha.kubernetes.io/unreachable"
     operator: "Exists"
     effect: "NoExecute"
     tolerationSeconds: 6000

The `Kubernetes Taints and
Toleratins <https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/>`__
contains more examples on this topic.

Priority Classes
----------------

Pods may belong to some *priority classes*. This allows scheduler to
distinguish more and less important Pods to resolve the situation when
some higher priority Pod cannot be scheduled without evicting a lower
priority one. This can be done adding one or more PriorityClasses in
your Kubernetes cluster, and specifying the ``PriorityClassName`` in the
`deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/main/deploy/cr.yaml>`__
file:

::

   priorityClassName: high-priority

See the `Kubernetes Pods Priority and Preemption
documentation <https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption>`__
to find out how to define and use priority classes in your cluster.

Pod Disruption Budgets
----------------------

Creating the *Pod Disruption Budget* is the Kubernetes style to limits
the number of Pods of an application that can go down simultaneously due
to such *voluntary disruptions* as cluster administrator’s actions
during the update of deployments or nodes, etc. By such a way
Distribution Budgets allow large applications to retain their high
availability while maintenance and other administrative activities.

We recommend to apply Pod Disruption Budgets manually to avoid situation
when Kubernetes stopped all your database Pods. See `the official
Kubernetes
documentation <https://kubernetes.io/docs/concepts/workloads/pods/disruptions/>`__
for details.
