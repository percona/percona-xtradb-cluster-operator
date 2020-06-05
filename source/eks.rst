==========================================================================================
Install Percona XtraDB Cluster on Amazon Elastic Kubernetes Service (EKS)
==========================================================================================

This quickstart shows you how to deploy Percona XtraDB cluster operator on Amazon Elastic Kubernetes Service (EKS). The document assumes some experience with Amazon EKS. For more information on the EKS, see the `Amazon EKS official documentation <https://aws.amazon.com/eks/>`_.

Prerequisites
=============

The following tools are used in this guide and therefore should be preinstalled:

1. **AWS Command Line Interface (AWS CLI)** for interacting with the different
   parts of AWS. You can install it following the `official installation instructions for your system <https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html>`_.

2. **eksctl** to simplify cluster creation on EKS. It can be installed
   along its `installation notes on GitHub <https://github.com/weaveworks/eksctl#installation>`.

3. **kubectl**  to manage and deploy applications on Kubernetes. Install
   it `following the official installation instructions <https://kubernetes.io/docs/tasks/tools/install-kubectl/>`_.

Create a new IAM role with EKS permissions
===========================================

The first thing to use is configuring the `AWS-IAM-Authenticator <https://github.com/kubernetes-sigs/aws-iam-authenticator>`_ to allow IAM authentication with your Kubernetes cluster.

Open the IAM console, select Roles on the left and then click the Create Role button at the top of the page.
From the list of AWS services, select EKS and then Next: Permissions at the bottom of the page.
Leave the selected policies as-is, and proceed to the Review page. Enter a name for the role (e.g. eksrole) and hit the Create role button at the bottom of the page to create the IAM role. The Role ARN specified on the page is to be used later.


Create the EKS cluster
======================

To create your cluster, you will need the following data:

* name of your EKS cluster,
* AWS region in which you wish to deploy your cluster,
* the amount of nodes you would like tho have,
* the ratio of `spot https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-spot-instances.html`_ and `on-demand <https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-on-demand-instances.html>`_ instances to use.

The most easy and visually clear way is to describe the desired cluster in YAML
and to pass this configuration to the ``eksctl`` command. 

The following example configures a EKS cluster with one `managed node group <https://docs.aws.amazon.com/eks/latest/userguide/managed-node-groups.html>`_ , including .... **ToDo**:

.. code:: yaml

   apiVersion: eksctl.io/v1alpha5
   kind: ClusterConfig

   metadata:
       name: test-cluster
       region: eu-west-2

   nodeGroups:
       - name: ng-1
         minSize: 3
         maxSize: 5
         instancesDistribution:
           maxPrice: 0.15
           instanceTypes: ["m5.xlarge", "m5.2xlarge"] # At least two instance types should be specified
           onDemandBaseCapacity: 0
           onDemandPercentageAboveBaseCapacity: 50
           spotInstancePools: 2
         tags:
           'iit-billing-tag': 'cloud'
         preBootstrapCommands:
             - "echo 'OPTIONS=\"--default-ulimit nofile=1048576:1048576\"' >> /etc/sysconfig/docker"
             - "systemctl restart docker"

.. note:: Please note, that ``preBootstrapCommands`` section is used in the
          above example to increase the limits for the amount of opened files:
          this is important and shouldn't be omited, taking into account the
          default EKS soft limit of 65536 files.

When the cluster configuration file is ready, you can actually create your cluster
by the following command:

.. code:: bash

   $ eksctl create cluster -f ~/cluster.yaml


**is it needed?**

Now update your ``kubectl`` configuration to allow it communicating with the new cluster. You can do it using the AWS CLI ``update-kubeconfig`` command. Run it with the correct cluster-name and region:

.. code:: bash

   $ aws eks --region eu-west-2 update-kubeconfig --name test-cluster


Install the Operator
=======================

1. IAM? Is it already used by kubectl? What about creating Roles and RoleBindings?

2. Create a namespace and set the context for the namespace. The resource names must be unique within the namespace and provide a way to divide cluster resources between users spread across multiple projects.

   So, create the namespace and save it in the namespace context for subsequent commands as follows (replace the ``<namespace name>`` placeholder with some descriptive name):

   .. code:: bash

      $ kubectl create namespace <namespace name>
      $ kubectl config set-context $(kubectl config current-context) --namespace=<namespace name>

   At success, you will see the message that namespace/<namespace name> was created, and the context was modified.

3. Use the following ``git clone`` command to download the correct branch of the percona-xtradb-cluster-operator repository:

   .. code:: bash

      git clone -b release-{{{release}}} https://github.com/percona/percona-xtradb-cluster-operator

   After the repository is downloaded, change the directory to run the rest of the commands in this document:

   .. code:: bash

      cd percona-xtradb-cluster-operator

4. Deploy the Operator with the following command:

   .. code:: bash

      kubectl apply -f deploy/bundle.yaml

   The following confirmation is returned:

   .. code:: text

      customresourcedefinition.apiextensions.k8s.io/perconaxtradbclusters.pxc.percona.com created
      customresourcedefinition.apiextensions.k8s.io/perconaxtradbclusterbackups.pxc.percona.com created
      customresourcedefinition.apiextensions.k8s.io/perconaxtradbclusterrestores.pxc.percona.com created
      customresourcedefinition.apiextensions.k8s.io/perconaxtradbbackups.pxc.percona.com created
      role.rbac.authorization.k8s.io/percona-xtradb-cluster-operator created
      serviceaccount/percona-xtradb-cluster-operator created
      rolebinding.rbac.authorization.k8s.io/service-account-percona-xtradb-cluster-operator created
      deployment.apps/percona-xtradb-cluster-operator created

5. The operator has been started, and you can create the Percona XtraDB cluster:

   .. code:: bash

      $ kubectl apply -f deploy/cr.yaml

   The process could take some time.
   The return statement confirms the creation:

   .. code:: text

      perconaxtradbcluster.pxc.percona.com/cluster1 created

6. During previous steps, the Operator has generated several `secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_, including the password for the ``root`` user, which you will need to access the cluster.

   Use ``kubectl get secrets`` command to see the list of Secrets objects (by default Secrets object you are interested in has ``my-cluster-secrets`` name). Then ``kubectl get secret my-cluster-secrets -o yaml`` will return the YAML file with generated secrets, including the root password which should look as follows:

   .. code:: yaml

     ...
     data:
       ...
       root: cm9vdF9wYXNzd29yZA==

   Here the actual password is base64-encoded, and ``echo 'cm9vdF9wYXNzd29yZA==' | base64 --decode`` will bring it back to a human-readable form.

