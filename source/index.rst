Percona Kubernetes Operator for Percona XtraDB Cluster
======================================================

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

Requirements
============
.. toctree::
   :maxdepth: 1

   System-Requirements
   architecture

Installation
============
.. toctree::
   :maxdepth: 1

   kubernetes
   openshift
   minikube
   scaling
   update
   monitoring
   custom-registry
   broker

Configuration
=============
.. toctree::
   :maxdepth: 1

   users
   operator
   backups
   storage
   constraints
   options
   proxysql-conf
   TLS

Reference
=============
.. toctree::
   :maxdepth: 1

   Release Notes <ReleaseNotes/index>
