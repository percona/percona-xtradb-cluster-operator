Percona XtraDB Cluster Operator
===================================

![PXC Replication](./assets/images/pxc-logo.png "Percona XtraDB Cluster replication"){: .align-center}

With the appearance of container orchestration systems, managing containerized database clusters has reached a new level of automation. Kubernetes and the OpenShift platform based on it have enriched the relatively new config-driven deployment approach with a set of strong features, such as scaling on demand, self-healing, and high availability. This is achieved by relatively-simple *controllers*, operating in the Kubernetes environment as declared in configuration files. They create various objects (including containers or container groups called pods) to do some job, listen for events and take actions based on them (e.g., re-create, delete, etc.).

The cost of this power is the complexity of the underlying container-based architecture. This level of automation makes it even more complex for the stateful applications, such as databases. A *Kubernetes Operator* is a special type of the controller introduced to simplify the complex deployments of a specific application. The Operator extends the Kubernetes API with new *Custom Resources* to deploy, configure, and manage the application. You can compare the Kubernetes Operator to a system administrator who deploys the application and watches the Kubernetes events related to it, taking administrative and operational actions when needed.

The percona-xtradb-cluster-operator is an application-specific controller created to effectively deploy and manage *Percona XtraDB Cluster* in the Kubernetes or OpenShift environment.
