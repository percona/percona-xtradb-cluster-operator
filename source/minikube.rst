Install Percona XtraDB Cluster on minikube
============================================

Installing PXC Operator on `minikube <https://github.com/kubernetes/minikube>`_
is the easiest way to try it locally without a cloud provider. Minikube runs
Kubernetes on GNU/Linux, Windows, or macOS system using a system-wide
hypervisor, such as VirtualBox, KVM/QEMU, VMware Fusion or Hyper-V. Using it is
a popular way to test Kubernetes application locally prior to deploying it on a
cloud.

Following steps are needed to run PXC Operator on minikube:

0. `Install minikube <https://kubernetes.io/docs/tasks/tools/install-minikube/>`_, using a way recommended for your system. This includes the installation of the following three components:
   #. kubectl tool,
   #. a hypervisor, if it is not already installed,
   #. actual minikube package

   After the installation running ``minikube start`` should download needed
   virtualized images, initialize and run the cluster. After minikube is
   successfully started, you can optionally run Kubernetes dashboard, which
   visually represents the state of your cluster. Executing
   ``minikube dashboard`` will start the dashboard and open it in your
   default web browser.

1. Clone the percona-xtradb-cluster-operator repository::

     git clone -b release-1.1.0 https://github.com/percona/percona-xtradb-cluster-operator
     cd percona-xtradb-cluster-operator

2. Deploy the operator with the following command::

     kubectl apply -f deploy/bundle.yaml

3. Edit the ``deploy/cr.yaml`` file to change the following keys in
   ``pxc`` and ``proxysql`` sections, which would otherwise prevent running
   Percona XtraDB Cluster on your local Kubernetes installation:

   #. comment ``resources.requests.memory`` and ``resources.requests.cpu`` keys 
   #. set ``affinity.antiAffinityTopologyKey`` key to ``"none"``

   Also, switch ``allowUnsafeConfigurations`` key to ``true``. 

4. Now apply the ``deploy/cr.yaml`` file with the following command::

     kubectl apply -f deploy/cr.yaml

5. During previous steps Operator have generated several secrets, including the
   password for the ``root`` user, which you will definitely need to access the
   cluster. Use ``kubectl get secrets`` to see the list of secrets objects (by
   default secrets object you are interested in has ``my-cluster-secrets`` name). 
   Then ``kubectl get secret my-cluster-secrets -o yaml`` will bring the YAML file
   with generated secrets, including the root password which would look as 
   follows::

     ...
     data:
       ...
       root: cm9vdF9wYXNzd29yZA== 

   Here actual password is base64-encoded, and
   ``echo 'cm9vdF9wYXNzd29yZA==' | base64 --decode`` will bring it back to a
   human-readable form.

6. Check connectivity to a newly created cluster:

   First of all, run percona-client and connect it's console output to your
   terminal (running it may require some time to deploy the correspondent Pod): 
   
   .. code:: bash

      kubectl run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
   
   Next, run ``mysql`` tool in the percona-client command shell using the password
   obtained from the secret:
   
   .. code:: bash

      mysql -h cluster1-proxysql -uroot -proot_password
