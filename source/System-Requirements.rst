System Requirements
+++++++++++++++++++

Officially supported platforms
--------------------------------

The following platforms are supported:

* OpenShift 3.11
* OpenShift 4.2
* Google Kubernetes Engine (GKE) 1.13
* GKE 1.15
* Amazon Elastic Kubernetes Service (EKS) 1.15
* Minikube 1.16

Other Kubernetes platforms may also work but have not been tested.

Resource Limits
-----------------------

A cluster running an officially supported platform contains at least three 
Nodes, with the following resources:

* 2GB of RAM,
* 2 CPU threads per Node for Pods provisioning,
* at least 60GB of available storage for Private Volumes provisioning.

Platform-specific limitations
------------------------------

The Operator is subsequent to specific platform limitations.

* Minikube doesn't support multi-node cluster configurations because of its
  local nature, which is in collision with the default affinity requirements
  of the Operator. To arrange this, the :ref:`install-minikube` instruction
  includes an additional step which turns off the requirement of having not
  less than three Nodes.




