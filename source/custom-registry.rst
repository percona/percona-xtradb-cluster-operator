Use docker images from a custom registry
========================================

Using images from a private Docker registry may be useful in different
situations: it may be related to storing images inside of a company, for
privacy and security reasons, etc. In such cases, Percona XtraDB Cluster
Operator allows to use a custom registry, and the following instruction
illustrates how this can be done by the example of the Operator deployed
in the OpenShift environment.

1. First of all login to the OpenShift and create project.

   .. code:: bash

      $ oc login
      Authentication required for https://192.168.1.100:8443 (openshift)
      Username: admin
      Password:
      Login successful.
      $ oc new-project pxc
      Now using project "pxc" on server "https://192.168.1.100:8443".

2. There are two things you will need to configure your custom registry
   access:

   -  the token for your user
   -  your registry IP address.

   The token can be find out with the following command:

   .. code:: bash

      $ oc whoami -t
      ADO8CqCDappWR4hxjfDqwijEHei31yXAvWg61Jg210s

   And the following one tells you the registry IP address:

   .. code:: bash

      $ kubectl get services/docker-registry -n default
      NAME              TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
      docker-registry   ClusterIP   172.30.162.173   <none>        5000/TCP   1d

3. Now you can use the obtained token and address to login to the
   registry:

   .. code:: bash

      $ docker login -u admin -p ADO8CqCDappWR4hxjfDqwijEHei31yXAvWg61Jg210s 172.30.162.173:5000
      Login Succeeded

4. Pull the needed image by its SHA digest:

   .. code:: bash

      $ docker pull docker.io/perconalab/percona-xtradb-cluster-operator@sha256:841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444
      Trying to pull repository docker.io/perconalab/percona-xtradb-cluster-operator ...
      sha256:841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444: Pulling from docker.io/perconalab/percona-xtradb-cluster-operator
      Digest: sha256:841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444
      Status: Image is up to date for docker.io/perconalab/percona-xtradb-cluster-operator@sha256:841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444

5. The following way is used to push an image to the custom registry
   (into the OpenShift pxc project):

   .. code:: bash

      $ docker tag \
          docker.io/perconalab/percona-xtradb-cluster-operator@sha256:841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444 \
          172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:1.2.0
      $ docker push 172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:1.2.0

6. Check the image in the OpenShift registry with the following command:

   .. code:: bash

      $ oc get is
      NAME                              DOCKER REPO                                                            TAGS      UPDATED
      percona-xtradb-cluster-operator   docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator   {{{release}}}     2 hours ago

7. When the custom registry image is Ok, put a Docker Repo + Tag string
   (it should look like
   ``docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator:{{{release}}}``)
   into the ``image:`` option in ``deploy/operator.yaml`` configuration
   file.

   Please note it is possible to specify ``imagePullSecrets`` option for
   all images, if the registry requires authentication.

8. Repeat steps 3-5 for other images, and update corresponding options
   in the ``deploy/cr.yaml`` file.

9. Now follow the standard `Percona XtraDB Cluster Operator installation
   instruction <./openshift>`__.

Percona certified images
------------------------

Following table presents Percona’s certified images to be used with the
Percona XtraDB Cluster Operator:


      .. list-table::
         :widths: 15 30
         :header-rows: 1

         * - Image
           - Digest
         * - percona/percona-xtradb-cluster-operator:1.2.0
           - 841c07eef30605080bfe80e549f9332ab6b9755fcbc42aacbf86e4ac9ef0e444
         * - percona/percona-xtradb-cluster-operator:1.2.0-pxc
           - d38482fcbe0d0f169e41eefd889404e967e8abc65a6890cbab4dd1f3ea2229df
         * - percona/percona-xtradb-cluster-operator:1.2.0-proxysql
           - 1385b77d3498cebc201426821fda620e0884e8fdaba6756240c9821948864af3
         * - percona/percona-xtradb-cluster-operator:1.2.0-backup
           - bd45486507321de67ff8ad2fa40c4f55fc20bd15db6369b61c73a5db11bb57cd
         * - percona/percona-xtradb-cluster-operator:1.2.0-broker
           - c0903f41539767fcfe49da815e1c3bfefe4e48a36912b64fb5350b09b51cab32
         * - percona/percona-xtradb-cluster-operator:1.2.0-pmm
           - 28bbb6693689a15c407c85053755334cd25d864e632ef7fed890bc85726cfb68
         * - percona/percona-xtradb-cluster-operator:1.1.0
           - fbfc2fc5c3afc80f18dddc5a1c3439fab89950081cf86c3439a226d4c97198eb
         * - percona/percona-xtradb-cluster-operator:1.1.0-pxc
           - a66a9212760e823af3c666a78e4b480cc7cc0d8be5cfa29c8141319c0036706e
         * - percona/percona-xtradb-cluster-operator:1.1.0-proxysql
           - ac952afb3721eafe86431155da7c3f7f90c4e800491c400a4222b650fd393357
         * - percona/percona-xtradb-cluster-operator:1.1.0-backup
           - 4852da039dd2a1d3ae9243ec634c14fd9f9e5af18a1fc6c7c9d25d4287dd6941
         * - percona/percona-xtradb-cluster-operator:1.0.0
           - b9e97c66a69f448898f8d43b92dd0314aaf53997b70824056dd3d0aec62488eb
         * - percona/percona-xtradb-cluster-operator:1.0.0-pxc
           - 6797c8492cff8092b39cdce75d7d85b9c2d4d08c4f6e0ba7b05c21562a54f168
         * - percona/percona-xtradb-cluster-operator:1.0.0-proxysql
           - b9360f1a8dc1e57e5ae7442373df02869ddc4da69ef9190190edde70b465235e
         * - percona/percona-xtradb-cluster-operator:1.0.0-backup
           - 652be455c8faf2d610de15e3568ff57fe8630fa353b6d97ff1c6b91d44741f8b
