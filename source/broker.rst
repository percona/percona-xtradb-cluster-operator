Deploy Percona XtraDB Cluster with Service Broker
=====================================================

Percona Service Broker provides the `Open Service Broker <https://www.openservicebrokerapi.org/>`_ object to facilitate the operator deployment within high-level visual tools. Following steps are needed to use it while installing the Percona XtraDB Cluster on the OpenShift platform:

1. The Percona Service Broker is to be deployed based on the ``percona-broker.yaml`` file. To use it you should first enable the `Service Catalog <https://docs.openshift.com/container-platform/4.1/applications/service_brokers/installing-service-catalog.html>`_, which can be done with the following command:

   .. code:: bash

      $ oc patch servicecatalogapiservers cluster --patch '{"spec":{"managementState":"Managed"}}' --type=merge
      $ oc patch servicecatalogcontrollermanagers cluster --patch '{"spec":{"managementState":"Managed"}}' --type=merge

   When Service Catalog is enabled, download and install the Percona Service
   Broker in a typical OpenShift way:

   .. code:: bash

      $ oc apply -f https://raw.githubusercontent.com/Percona-Lab/percona-dbaas-cli/master/deploy/percona-broker.yaml

   .. note:: This step should be done only once; the step does not need to be repeated
      with any other Operator deployments. It will automatically create and setup
      the needed service and projects catalog with all necessary objects.

2. Now login to your `OpenShift Console Web UI <https://github.com/openshift/console>`_ and switch to the percona-service-broker project. You can check its Pod running on a correspondent page:

   .. image:: img/broker-pods.png
      :width: 800px
      :align: center
      :alt: Broker in the OpenShift Console

   Now switch to the Developer Catalog and select Percona XtraDB Cluster
   Operator:

   .. image:: img/broker-dev-catalog.png
      :width: 800px
      :align: center
      :alt: Developer Catalog

   Choose ``Percona XtraDB Cluster Operator`` item.
   This will lead you to the Operator page with the *Create Service Instance*
   button.

3. Clicking the *Create Service Instance* button guides you to the next page:

   .. image:: img/broker-create-service-instance.png
      :width: 800px
      :align: center
      :alt: Developer Catalog

   The two necessary fields are *Service Instance Name* and *Cluster Name*,
   which should be unique for your project.

4. Clicking the *Create* button gets you to the *Overview* page, which reflects
   the process of the cluster creation process:

   .. image:: img/broker-creation.png
      :width: 800px
      :align: center
      :alt: Developer Catalog

   You can also track Pods to see when they are deployed and track any errors.
