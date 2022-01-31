Percona Distribution for MySQL Operator based on Percona XtraDB Cluster
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

Following our best practices for deployment and configuration, `Percona Distribution for MySQL Operator  based on Percona XtraDB Cluster <https://github.com/percona/percona-xtradb-cluster-operator>`_ 
contains everything you need to quickly and consistently deploy and scale
Percona XtraDB Cluster instances in a Kubernetes-based environment on-premises
or in the cloud.

Requirements
============

.. toctree::
   :maxdepth: 1

   System-Requirements
   architecture

Quickstart guides
=================

.. toctree::
   :maxdepth: 1

   minikube
   gke
   eks
   helm

Advanced Installation Guides
============================

.. toctree::
   :maxdepth: 1

   kubernetes
   openshift
   custom-registry
   broker
   cluster-wide

Configuration
=============

.. toctree::
   :maxdepth: 1

   users
   storage
   constraints
   options
   containers-conf
   haproxy-conf
   proxysql-conf
   TLS
   encryption

Management
==========

.. toctree::
   :maxdepth: 1

   backups
   pause
   update
   scaling
   replication
   monitoring
   sidecar
   recovery
   debug

.. toctree::
   :maxdepth: 1

Reference
=============

.. toctree::
   :maxdepth: 1

   operator
   images
   api
   faq
   Release Notes <ReleaseNotes/index>
