.. _install-minikube:

Install Percona XtraDB Cluster on Minikube
============================================

Installing the Percona XtraDB Cluster Operator on `minikube <https://github.com/kubernetes/minikube>`_
is the easiest way to try it locally without a cloud provider. Minikube runs
Kubernetes on GNU/Linux, Windows, or macOS system using a system-wide
hypervisor, such as VirtualBox, KVM/QEMU, VMware Fusion or Hyper-V. Using it is
a popular way to test the Kubernetes application locally prior to deploying it
on a cloud.

The following steps are needed to run Percona XtraDB Cluster Operator on
Minikube:

#. `Install Minikube <https://kubernetes.io/docs/tasks/tools/install-minikube/>`_,
   using a way recommended for your system. This includes the installation of
   the following three components:

   #. kubectl tool,
   #. a hypervisor, if it is not already installed,
   #. actual Minikube package

   After the installation, run ``minikube start --memory=4096 --cpus=3``
   (parameters increase the virtual machine limits for the CPU cores and memory,
   to ensure stable work of the Operator). Being executed, this command will
   download needed virtualized images, then initialize and run the
   cluster. After Minikube is successfully started, you can optionally run the
   Kubernetes dashboard, which visually represents the state of your cluster.
   Executing ``minikube dashboard`` will start the dashboard and open it in your
   default web browser.

#. Clone the percona-xtradb-cluster-operator repository::

     git clone -b v{{{release}}} https://github.com/percona/percona-xtradb-cluster-operator
     cd percona-xtradb-cluster-operator

#. Deploy the operator with the following command::

     kubectl apply -f deploy/bundle.yaml

#. Because minikube runs locally, the default ``deploy/cr.yaml`` file should
   be edited to adapt the Operator for the the local installation with limited
   resources. Change the following keys in ``pxc`` and ``proxysql`` sections:

   #. comment ``resources.requests.memory`` and ``resources.requests.cpu`` keys
      (this will fit the Operator in minikube default limitations)
   #. set ``affinity.antiAffinityTopologyKey`` key to ``"none"`` (the Operator
      will be unable to spread the cluster on several nodes)

   Also, switch ``allowUnsafeConfigurations`` key to ``true`` (this option turns
   off the Operator’s control over the cluster configuration, making it possible to
   deploy Percona XtraDB Cluster as a one-node cluster).

#. Now apply the ``deploy/cr.yaml`` file with the following command::

     kubectl apply -f deploy/cr.yaml

   Creation process will take some time. The process is over when both
   operator and replica set pod have reached their Running status:

   .. code:: bash

      $ kubectl get pods
      NAME                                              READY   STATUS    RESTARTS   AGE
      cluster1-haproxy-0                                1/1     Running   0          5m
      cluster1-haproxy-1                                1/1     Running   0          5m
      cluster1-haproxy-2                                1/1     Running   0          5m
      cluster1-pxc-0                                    1/1     Running   0          5m
      cluster1-pxc-1                                    1/1     Running   0          4m
      cluster1-pxc-2                                    1/1     Running   0          2m
      percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

#. During previous steps, the Operator has generated several `secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_, including the
   password for the ``root`` user, which you will definitely need to access the
   cluster. Use ``kubectl get secrets`` to see the list of Secrets objects (by
   default Secrets object you are interested in has ``my-cluster-secrets`` name).
   Then ``kubectl get secret my-cluster-secrets -o yaml`` will return the YAML
   file with generated secrets, including the root password which should look as
   follows::

     ...
     data:
       ...
       root: cm9vdF9wYXNzd29yZA== 

   Here the actual password is base64-encoded, and
   ``echo 'cm9vdF9wYXNzd29yZA==' | base64 --decode`` will bring it back to a
   human-readable form.

#. Check connectivity to a newly created cluster.

   First of all, run percona-client and connect its console output to your
   terminal (running it may require some time to deploy the correspondent Pod): 
   
   .. code:: bash

      kubectl run -i --rm --tty percona-client --image=percona:8.0 --restart=Never -- bash -il
   
   Now run ``mysql`` tool in the percona-client command shell using the password
   obtained from the secret:
   
   .. code:: bash

      mysql -h cluster1-haproxy -uroot -proot_password

   This command will connect you to the MySQL monitor.

   .. code:: text

      mysql: [Warning] Using a password on the command line interface can be insecure.
      Welcome to the MySQL monitor.  Commands end with ; or \g.
      Your MySQL connection id is 1976
      Server version: 8.0.19-10 Percona XtraDB Cluster (GPL), Release rel10, Revision 727f180, WSREP version 26.4.3

      Copyright (c) 2009-2020 Percona LLC and/or its affiliates
      Copyright (c) 2000, 2020, Oracle and/or its affiliates. All rights reserved.

      Oracle is a registered trademark of Oracle Corporation and/or its
      affiliates. Other names may be trademarks of their respective
      owners.

      Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.
