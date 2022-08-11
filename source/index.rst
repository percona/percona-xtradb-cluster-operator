|pxcoperator|
=======================================================================

Kubernetes and the OpenShift platform, based on Kubernetes, have added a way to
manage containerized systems, including database clusters. This management is
achieved by controllers, declared in configuration files. These controllers
provide automation with the ability to create objects, such as a container or a
group of containers called pods, to listen for an specific event and then
perform a task.

This automation adds a level of complexity to the container-based architecture
and stateful applications, such as a database. A Kubernetes Operator is a
special type of controller introduced to simplify complex deployments. The
Operator extends the Kubernetes API with custom resources.

`Percona XtraDB Cluster <https://www.percona.com/software/mysql-database/percona-xtradb-cluster>`_
is an open-source enterprise MySQL solution that helps you to ensure data
availability for your applications while improving security and simplifying the
development of new applications in the most demanding public, private, and
hybrid cloud environments.

Following our best practices for deployment and configuration, `Percona Operator for MySQL based on Percona XtraDB Cluster <https://github.com/percona/percona-xtradb-cluster-operator>`_ 
contains everything you need to quickly and consistently deploy and scale
Percona XtraDB Cluster instances in a Kubernetes-based environment on-premises
or in the cloud.

Requirements
============

.. toctree::
   :maxdepth: 1

   System Requirements <System-Requirements>
   Design and architecture <architecture>

Quickstart guides
=================

.. toctree::
   :maxdepth: 1

   Install with Helm <helm.rst>
   Install on Minikube <minikube.rst>
   Install on Google Kubernetes Engine (GKE) <gke.rst>
   Install on Amazon Elastic Kubernetes Service (AWS EKS) <eks.rst>

Advanced Installation Guides
============================

.. toctree::
   :maxdepth: 1

   Generic Kubernetes installation <kubernetes.rst>
   Install on OpenShift <openshift.rst>
   Use private registry <custom-registry.rst>

Configuration
=============

.. toctree::
   :maxdepth: 1

   Local Storage support <storage.rst>
   Anti-affinity and tolerations <constraints.rst>
   Changing MySQL Options <options.rst>
   Defining environment variables <containers-conf>
   Load Balancing with HAProxy <haproxy-conf>
   Load Balancing with ProxySQL <proxysql-conf>
   Transport Encryption (TLS/SSL) <TLS.rst>
   Data at rest encryption <encryption.rst>
   Application and system users <users.rst>

Management
==========

.. toctree::
   :maxdepth: 1

   Backup and restore <backups.rst>
   Upgrade Percona XtraDB Cluster and the Operator <update.rst>
   Horizontal and vertical scaling <scaling.rst>
   Multi-cluster and multi-region deployment <replication.rst>
   Monitor with Percona Monitoring and Management (PMM) <monitoring.rst>
   Add sidecar containers <sidecar.rst>
   Restart or pause the cluster <pause.rst>
   Crash recovery <recovery>
   Debug and troubleshoot <debug.rst>

.. toctree::
   :maxdepth: 1

HOWTOs
======

.. toctree::
   :maxdepth: 1

   Install Percona XtraDB Cluster in multi-namespace (cluster-wide) mode <cluster-wide>

Reference
=============

.. toctree::
   :maxdepth: 1

   Custom Resource options <operator.rst>
   Percona certified images <images.rst>
   Operator API <api.rst>
   Frequently Asked Questions <faq.rst>
   Release Notes <ReleaseNotes/index>
