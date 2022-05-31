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
during hardware failure. |operator|
provides out-of-the-box functionality to automate provisioning and management of
highly available MySQL database clusters on Kubernetes.

How can I contact the developers?
================================================================================

The best place to discuss |pxcoperator|
with developers and other community members is the `community forum <https://forums.percona.com/categories/kubernetes-operator-percona-xtradb-cluster>`_.

If you would like to report a bug, use the `Percona Operator for MySQL project in JIRA <https://jira.percona.com/projects/K8SPXC>`_.

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
specific platform and/or you are new to the Percona Distribution for MySQL
Operator as a whole.

Which versions of MySQL the |operator| supports?
================================================================================

|pxcoperator| provides a ready-to-use installation of the
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

|pxcoperator| supports both HAProxy and ProxySQL as a load
balancer. HAProxy is turned on by default, but both solutions are similar in
terms of their configuration and operation under the control of the Operator.

Still, they have technical differences. HAProxy is a general and widely used
high availability, load balancing, and proxying solution for TCP and HTTP-based
applications. ProxySQL provides similar functionality but is specific to MySQL
clusters. As an SQL-aware solution, it is able to provide more tight
internal integration with MySQL instances.

Both projects do a really good job with the Operator. The
proxy choice should depend mostly on application-specific workload (including
object-relational mapping), performance requirements, advanced routing and
caching needs with one or another project, components already in use in the
current infrastructure, and any other specific needs of the application.

.. _faq-hostpath:

How can I add create a directory on the node to use it as a local storage
================================================================================

You can :ref:`configure hostPath volume<storage-hostpath>` to mount some
existing file or directory from the node’s filesystem into the Pod and use it
as a local storage. The directory used for local storage should already exist
in the node's filesystem. You can create it through the shell access to the node,
with ``mkdir`` command, as all other directories. Alternatively you can create
a Pod which will do this job. Let's suppose you are going to use
``/var/run/data-dir`` directory as your local storage, describing it in the
``deploy/cr.yaml`` configuration file as follows:

.. code:: yaml

	...
	pxc:
	  ...
	  volumeSpec:
	     hostPath:
	       path: /var/run/data-dir
	       type: Directory
	  containerSecurityContext:
	    privileged: false
	  podSecurityContext:
	    runAsUser: 1001
	    runAsGroup: 1001
	    supplementalGroups: [1001]
	  nodeSelector:
	    kubernetes.io/hostname: a.b.c

Create the yaml file (e.g. ``mypod.yaml``), with the following contents:

.. code:: yaml

	apiVersion: v1
	kind: Pod
	metadata:
	  name: hostpath-helper
	spec:
	  containers:
	  - name: init
	    image: busybox
	    command: ["install", "-o", "1001", "-g", "1001", "-m", "755", "-d", "/mnt/data-dir"]
	    volumeMounts:
	    - name: root
	      mountPath: /mnt
	    securityContext: 
	      runAsUser: 0
	  volumes:
	  - name: root
	    hostPath:
	      path: /var/run
	  restartPolicy: Never
	  nodeSelector:
	    kubernetes.io/hostname: a.b.c

Don't forget to apply it as usual:

.. code:: bash

   $ kubectl apply -f mypod.yaml

.. _faq-sidecar:

How can I add custom sidecar containers to my cluster?
================================================================================

The Operator allows you to deploy additional (so-called *sidecar*) containers to
the Pod. You can use this feature to run debugging tools, some specific
monitoring solutions, etc. Add such sidecar container to the ``deploy/cr.yaml``
configuration file, specifying its name and image, and possibly a command to
run:

.. code:: yaml

   spec:
     pxc:
       ....
       sidecars:
       - image: busybox
         command: ["/bin/sh"]
         args: ["-c", "while true; do echo echo $(date -u) 'test' >> /dev/null; sleep 5; done"]
         name: my-sidecar-1
       ....

You can add ``sidecars`` subsection to ``pxc``, ``haproxy``, and ``proxysql``
sections.

.. note::  Custom sidecar containers `can easily access other components of your cluster <https://kubernetes.io/docs/concepts/workloads/pods/#resource-sharing-and-communication>`_. 
   Therefore they should be used carefully and by experienced users only.

Find more information on sidecar containers in the appropriate
:ref:`documentation page<operator-sidecar>`.

How to get core dumps in case of the Percona XtraDB Cluster crash
================================================================================

In the Percona XtraDB Cluster crash case, gathering all possible information for
enhanced diagnostics to be shared with Percona Support helps to solve an issue
faster. One of such helpful artifacts is `core dump <https://en.wikipedia.org/wiki/Core_dump>`_.

Percona XtraDB Cluster can create core dumps on crush `using libcoredumper <https://www.percona.com/doc/percona-server/5.7/diagnostics/libcoredumper.html>`_. The Operator has this feature turned on by default. 
Core dumps are saved to  ``DATADIR`` (``var/lib/mysql/``). You can find
appropriate core files in the following way (substitute ``some-name-pxc-1`` with
the name of your Pod):

.. code:: bash

   kubectl exec some-name-pxc-1 -c pxc -it -- sh -c 'ls -alh /var/lib/mysql/ | grep core'
   -rw------- 1 mysql mysql 1.3G Jan 15 09:30 core.20210015093005 

When identified, the appropriate core dump can be downloaded as follows:

.. code:: bash

   kubectl cp some-name-pxc-1:/var/lib/mysql/core.20210015093005  /tmp/core.20210015093005

.. note:: It is useful to provide Build ID and Server Version in addition to core
   dump when Creating a support ticket. Both can be found from logs:
   
   .. code:: bash
   
      kubectl logs some-name-pxc-1 -c logs 

      [1] init-deploy-949.some-name-pxc-1.mysqld-error.log: [1610702394.259356066, {"log"=>"09:19:54 UTC - mysqld got signal 11 ;"}]
      [2] init-deploy-949.some-name-pxc-1.mysqld-error.log: [1610702394.259356829, {"log"=>"Most likely, you have hit a bug, but this error can also be caused by malfunctioning hardware."}]
      [3] init-deploy-949.some-name-pxc-1.mysqld-error.log: [1610702394.259457282, {"log"=>"Build ID: 5a2199b1784b967a713a3bde8d996dc517c41adb"}]
      [4] init-deploy-949.some-name-pxc-1.mysqld-error.log: [1610702394.259465692, {"log"=>"Server Version: 8.0.21-12.1 Percona XtraDB Cluster (GPL), Release rel12, Revision 4d973e2, WSREP version 26.4.3, wsrep_26.4.3"}]
      .....

How to choose between HAProxy and ProxySQL when configuring the cluster?
================================================================================

You can configure the Operator to use one of two different proxies, HAProxy
(the default choice) and ProxySQL. Both solutions are fully supported by the
Operator, but they have some differences in the architecture, which can make one
of them more suitable then the other one in some use cases.

The main difference is that HAProxy operates in TCP mode as an `OSI level 4 proxy <https://www.haproxy.com/blog/layer-4-and-layer-7-proxy-mode/>`_,
while ProxySQL implements OSI level 7 proxy, and thus can provide some additional
functionality like read/write split, firewalling and caching.

From the other side, utilizing HAProxy for the service is the easier way to go,
and getting use of the ProxySQL level 7 specifics requires good understanding of
Kubernetes and ProxySQL.

See more detailed functionality and performance comparison of using the Operator
with both solutions in `this blog post <https://www.percona.com/blog/2021/01/11/percona-kubernetes-operator-for-percona-xtradb-cluster-haproxy-or-proxysql/>`__.

.. _faq-validation:

Which additional access permissions are used by the Custom Resource validation webhook?
=======================================================================================

The ``spec.enableCRValidationWebhook`` key in the `deploy/cr.yaml <https://github.com/percona/percona-server-mongodb-operator/blob/main/deploy/cr.yaml>`__
file enables or disables schema validation done by the Operator before applying
``cr.yaml`` file. This feature works only in :ref:`cluster-wide mode<install-clusterwide>`
due to access restrictions. It uses the following additional `RBAC permissions <https://kubernetes.io/docs/reference/access-authn-authz/rbac/>`_:

.. code:: yaml

   - apiGroups:
     - admissionregistration.k8s.io
     resources:
     - validatingwebhookconfigurations
     verbs:
     - get
     - list
     - watch
     - create
     - update
     - patch
     - delete

.. _faq-allocator:

How can I use alternative memory allocator for MySQL?
================================================================================

It's often a recommended practice to try using an alternative allocator library
for mysqld in case the memory usage is suspected to be higher than expected.
You can pass ``LD_PRELOAD`` environment variable with the alternative allocator
library name to mysqld. The :ref:`pxc.envVarsSecret<pxc-envvarssecret>` Custom
Resource option :ref:`allows<faq-env>` you to specify the name of the Secret
with the environment variable for mysqld.

This allows you to use jemalloc allocator already present in Percona XtraDB
Cluster Pods with the following environment variable:

.. code:: bash

   LD_PRELOAD=/usr/lib64/libjemalloc.so.1

The following Secrets object can be used to set it (as you can see, environment
variables are stored as ``data`` - i.e., base64-encoded strings):

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata: 
     name: my-env-var-secrets
   type: Opaque
   data: 
     LD_PRELOAD: L3Vzci9saWI2NC9saWJqZW1hbGxvYy5zby4x

You can find more details on setting environment variables via the :ref:`pxc.envVarsSecret<pxc-envvarssecret>`
Custom Resource option on the :ref:`dedicated documentation page<faq-env>`. 
