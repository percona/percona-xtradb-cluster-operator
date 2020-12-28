.. _faq:

================================================================================
Frequently Asked Questions
================================================================================

.. contents::
   :local:
   :depth: 1

Why do we need to follow "the Kubernetes way" when Kubernetes was never intended to run databases?
=====================================================================================================

As it is well known, the Kubernetes approach is targeted at stateless
applications but provides ways to store state (in Persistent Volumes, etc.) if
the application needs it. Generally, a stateless mode of operation is supposed
to provide better safety, sustainability, and scalability, it makes the
already-deployed components interchangeable. You can find more about substantial
benefits brought by Kubernetes to databases in `this blog post <https://www.percona.com/blog/2020/10/08/the-criticality-of-a-kubernetes-operator-for-databases/>`_.

The architecture of state-centric applications (like databases) should be
composed in a right way to avoid crashes, data loss, or data inconsistencies
during hardware failure. Percona Kubernetes Operator for Percona XtraDB Cluster
provides out-of-the-box functionality to automate provisioning and management of
highly available MySQL database clusters on Kubernetes.

How can I contact the developers?
================================================================================

The best place to discuss Percona Kubernetes Operator for Percona XtraDB Cluster
with developers and other community members is the `community forum <https://forums.percona.com/categories/kubernetes-operator-percona-xtradb-cluster>`_.

If you would like to report a bug, use the `Percona Kubernetes Operator for Percona XtraDB Cluster project in JIRA <https://jira.percona.com/projects/K8SPXC>`_.

What is the difference between the Operator quickstart and advanced installation ways?
=======================================================================================

As you have noticed, the installation section of docs contains both quickstart
and advanced installation guides.

The quickstart guide is simpler. It has fewer installation steps in favor of
predefined default choices. Particularly, in advanced installation guides, you
separately apply the Custom Resource Definition and Role-based Access Control
configuration files with possible edits in them. At the same time, quickstart
guides rely on the all-inclusive bundle configuration.

At another point, quickstart guides are related to specific platforms you are
going to use (Minikube, Google Kubernetes Engine, etc.) and therefore include
some additional steps needed for these platforms.

Generally, rely on the quickstart guide if you are a beginner user of the
specific platform and/or you are new to the Percona XtraDB Cluster Operator as
a whole.

Which versions of MySQL Percona XtraDB Cluster Operator supports?
================================================================================

Percona XtraDB Cluster Operator provides a ready-to-use installation of the
MySQL-based Percona XtraDB Cluster inside your Kubernetes installation. It works
with both MySQL 8.0 and 5.7 branches, and the exact version is determined by the
Docker image in use.

Percona-certified Docker images used by the Operator are listed `here <https://www.percona.com/doc/kubernetes-operator-for-pxc/images.html>`_.
As you can see, both Percona XtraDB Cluster 8.0 and 5.7 are supported with the
following recommended versions: {{{pxc80recommended}}} and
{{{pxc57recommended}}}. Three major numbers in the XtraDB Cluster version refer
to the version of Percona Server in use. More details on the exact Percona
Server version can be found in the release notes (`8.0 <https://www.percona.com/doc/percona-server/8.0/release-notes/release-notes_index.html>`_, `5.7 <https://www.percona.com/doc/percona-server/5.7/release-notes/release-notes_index.html>`_).

How HAProxy is better than ProxySQL?
================================================================================

Percona XtraDB Cluster Operator supports both HAProxy and ProxySQL as a load
balancer. HAProxy is turned on by default, but both solutions are similar in
terms of their configuration and operation under the control of the Operator.

Still, they have technical differences. HAProxy is a general and widely used
high availability, load balancing, and proxying solution for TCP and HTTP-based
applications. ProxySQL provides similar functionality but is specific to MySQL
clusters. As an SQL-aware solution, it is able to provide more tight
internal integration with MySQL instances.

Both projects do a really good job with Percona XtraDB Cluster Operator. The
proxy choice should depend mostly on application-specific workload (including
object-relational mapping), performance requirements, advanced routing and
caching needs with one or another project, components already in use in the
current infrastructure, and any other specific needs of the application.

