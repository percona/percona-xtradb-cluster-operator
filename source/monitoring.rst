Monitoring
==========

The Percona Monitoring and Management (PMM) `provides an excellent
solution <https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm>`__
to monitor Percona XtraDB Cluster.

Installing the PMM Server
-------------------------

This first section installs the PMM Server to monitor Percona XtraDB
Cluster on Kubernetes or OpenShift. The following steps are optional if
you already have installed the PMM Server. The PMM Server available on
your network does not require another installation in Kubernetes.

1. The recommended installation approach is based on using
   `helm <https://github.com/helm/helm>`__ - the package manager for
   Kubernetes, which will substantially simplify further steps. So first
   thing to do is to install helm following its `official installation
   instructions <https://docs.helm.sh/using_helm/#installing-helm>`__.

2. When the helm is installed, add Percona chart repository and update
   information of available charts as follows:

   ::

      $ helm repo add percona https://percona-charts.storage.googleapis.com
      $ helm repo update

3. Now helm can be used to install PMM Server:

   OpenShift command:
   ::
      $ helm install monitoring percona/pmm-server --set platform=openshift --version 1.17.3 --set "credentials.password=supa|^|pazz"

   Kubernetes command:
   ::
      $ helm install monitoring percona/pmm-server --set platform=kubernetes --version 2.7.0 --set "credentials.password=supa|^|pazz"

Installing the PMM Client
-------------------------

The following steps are needed for the PMM client installation:

1. The PMM client installation is initiated by updating the ``pmm``
   section in the
   `deploy/cr.yaml <https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml>`__
   file.

   -  set ``pmm.enabled=true``
   -  make sure that ``serverHost`` (the PMM service name,
      ``monitoring-service`` by default) is the same as one specified
      for the ``name`` parameter on the previous step, but with
      additional ``-service`` suffix.
   -  make check that ``serverUser`` match the PMM Server user name
      (``pmm`` by default for PMM 1.x and ``admin`` for PMM 2.x).
   -  make sure that ``pmmserver`` field in the
      ``deploy/secrets.yaml`` secrets file is the same as one specified
      for the ``credentials.password`` parameter on the previous step
      (if not, fix it and apply with the
      ``kubectl apply -f deploy/secrets.yaml`` command).

   When done, apply the edited ``deploy/cr.yaml`` file:

   ::

      $ kubectl apply -f deploy/cr.yaml

2. To make sure everything gone right, check that correspondent Pods are
   not continuously restarting (which would occur in case of any errors
   on the previous two steps):

   ::

      $ kubectl get pods
      $ kubectl logs cluster1-pxc-node-0 -c pmm-client

3. Find the external IP address (``EXTERNAL-IP`` field in the output of
   ``kubectl get service/monitoring-service -o wide``). This IP address
   can be used to access PMM via *https* in a web browser, with the
   login/password authentication, already configured and able to `show
   Percona XtraDB Cluster
   metrics <https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm>`__.
