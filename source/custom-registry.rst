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

      $ docker pull docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0
      Trying to pull repository docker.io/perconalab/percona-xtradb-cluster-operator ...
      sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0: Pulling from docker.io/perconalab/percona-xtradb-cluster-operator
      Digest: sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0
      Status: Image is up to date for docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0

5. The following way is used to push an image to the custom registry
   (into the OpenShift pxc project):

   .. code:: bash

      $ docker tag \
          docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0 \
          172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:0.2.0
      $ docker push 172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:0.2.0

6. Check the image in the OpenShift registry with the following command:

   .. code:: bash

      $ oc get is
      NAME                              DOCKER REPO                                                            TAGS      UPDATED
      percona-xtradb-cluster-operator   docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator   0.2.0     2 hours ago

7. When the custom registry image is Ok, put a Docker Repo + Tag string
   (it should look like
   ``docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator:0.2.0``)
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

0.3.0
~~~~~

+--------------------------------+-------------------------------------+
| Image                          | Digest                              |
+================================+=====================================+
| percona/percona-xtradb-cluster | f4a0d604bb13678cbcd72fd261d1b2a287a |
| -operator:0.3.0                | 09e69270b1f91b04b46c85f9592dc       |
+--------------------------------+-------------------------------------+
| percona/percona-xtradb-cluster | 51a478ff24e6e16315e090e7c8b372ad589 |
| -operator:0.3.0-pxc            | 09d9560a8c5b428c1ca9588912bb2       |
+--------------------------------+-------------------------------------+
| percona/percona-xtradb-cluster | 673b954eec7395ca4571024a62f8faab389 |
| -operator:0.3.0-proxysql       | 7b183f3134e220ad5332866afa4a1       |
+--------------------------------+-------------------------------------+
| percona/percona-xtradb-cluster | a205e8f86993373ece95d9bcfc3068b7f83 |
| -operator:0.3.0-backup         | f96d61582dbe07d7a4b6cb359cc03       |
+--------------------------------+-------------------------------------+
| perconalab/pmm-client:1.17.1   | f762cda2eda9ef17bfd1242ede70ee72595 |
|                                | 611511d8d0c5c46931ecbc968e9af       |
+--------------------------------+-------------------------------------+

.. _section-1:

0.2.0
~~~~~

+------------------------------+---------------------------------------+
| Image                        | Digest                                |
+==============================+=======================================+
| perconalab/percona-xtradb-cl | 8895ff4647602dcbcabbf6ea5d1be1611e9d7 |
| uster-operator:0.2.0         | a9769c3bb3415c3a73aba2adda0           |
+------------------------------+---------------------------------------+
| perconalab/pxc-openshift:0.2 | a9f6568cc71e1e7b5bbfe69b3ea561e2c3bae |
| .0                           | 92a75caba7ffffa88bd3c730bc9           |
+------------------------------+---------------------------------------+
| perconalab/proxysql-openshif | cdd114b82f34312ef73419282a695063387c7 |
| t:0.2.0                      | 15d3e80677902938f991ef94f13           |
+------------------------------+---------------------------------------+
| perconalab/backupjob-openshi | 1ded5511a59fc2cc5a6b23234495e6d243d5f |
| ft:0.2.0                     | 8b55e1b6061781779e19887cdc9           |
+------------------------------+---------------------------------------+
| perconalab/pmm-client:1.17.0 | efdce369d5fb29b0a1b03a7026dfbc2efe07b |
|                              | 618471aba5db308d0c21b8e118d           |
+------------------------------+---------------------------------------+

.. _section-2:

0.1.0
~~~~~

+------------------------------+---------------------------------------+
| Image                        | Digest                                |
+==============================+=======================================+
| perconalab/percona-xtradb-cl | 9e4b44ef6859e995d70c0ef7db9be9b9c2875 |
| uster-operator:0.1.0         | d1116a2b6ff7e5a7f5e5fcb39b7           |
+------------------------------+---------------------------------------+
| perconalab/pxc-openshift:0.1 | c72eb45c3f103f105f864f05668a2b029bb6a |
| .0                           | 3ba9fc8a1d0467040c6c83f3e53           |
+------------------------------+---------------------------------------+
| perconalab/proxysql-openshif | 482b6f4161aafc78585b3e377a4aec9a983f4 |
| t:0.1.0                      | e4860e0bd8576f0e39eee52909d           |
+------------------------------+---------------------------------------+
| perconalab/pmm-client:1.17.0 | efdce369d5fb29b0a1b03a7026dfbc2efe07b |
|                              | 618471aba5db308d0c21b8e118d           |
+------------------------------+---------------------------------------+
