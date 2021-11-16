System Requirements
+++++++++++++++++++

The Operator supports Percona XtraDB Cluster (PXC) 5.7 and 8.0.

The new ``caching_sha2_password`` authentication plugin which is default in 8.0
is not supported for the ProxySQL compatibility reasons. Therefore both Percona
XtraDB Cluster 5.7 and 8.0 are configured with
``default_authentication_plugin = mysql_native_password``.

Officially supported platforms
--------------------------------

The following platforms were tested and are officially supported by the Operator
{{{release}}}:

* `OpenShift <https://www.redhat.com/en/technologies/cloud-computing/openshift>`_ 4.7 - 4.9
* `Google Kubernetes Engine (GKE) <https://cloud.google.com/kubernetes-engine>`_ 1.19 - {{{gkerecommended}}}
* `Amazon Elastic Container Service for Kubernetes (EKS) <https://aws.amazon.com>`_ 1.17 - 1.21
* `Minikube <https://minikube.sigs.k8s.io/docs/>`_ 1.22

Other Kubernetes platforms may also work but have not been tested.

Resource Limits
-----------------------

A cluster running an officially supported platform contains at least three 
Nodes, with the following resources:

* 2GB of RAM,
* 2 CPU threads per Node for Pods provisioning,
* at least 60GB of available storage for Persistent Volumes provisioning.




