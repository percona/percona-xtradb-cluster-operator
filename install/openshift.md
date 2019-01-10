Install Percona XtraDB Cluster on OpenShift
-----------------------------------------------

0. First of all, clone the percona-xtradb-cluster-operator repository:

   ```bash
   git clone -b release-0.2.0 https://github.com/Percona-Lab/percona-xtradb-cluster-operator
   cd percona-xtradb-cluster-operator
   ```
   **Note:** *It is crucial to checkout code by right tag on this step. Please be careful.*

1. The next thing to do is to create a new `pxc` project:

   ```bash
   $ oc new-project pxc
   ```

2. Now that’s time to add the PXC Users secrets to OpenShift. They should be placed in the data section of the `deploy/secrets.yaml` file as base64-encoded logins and passwords for the user accounts (see [Kubernetes documentation](https://kubernetes.io/docs/concepts/configuration/secret/) for details).

   **Note:** *the following command can be used to get base64-encoded password from a plain text string:* `$ echo -n 'plain-text-password' | base64`

   After editing is finished, users secrets should be created (or updated with the new passwords) using the following command:

   ```bash
   $ oc apply -f deploy/secrets.yaml
   ```

   More details about secrets can be found in a [separate section](../configure/users).

3. Now RBAC (role-based access control) and Custom Resource Definition for PXC should be created from the following two files: `deploy/rbac.yaml` and `deploy/crd.yaml`. Briefly speaking, role-based access is based on specifically defined roles and actions corresponding to them, allowed to be done on specific Kubernetes resources (details about users and roles can be found in [OpenShift documentation](https://docs.openshift.com/enterprise/3.0/architecture/additional_concepts/authorization.html)). Custom Resource Definition extends the standard set of resources which Kubernetes “knows” about with the new items (in our case ones which are the core of the operator).

   ```bash
   $ oc project pxc
   $ oc apply -f deploy/crd.yaml -f deploy/rbac.yaml
   ```

   **Note:** *This step requires your user to have cluster-admin role privileges.*

4. An extra step is needed if you want to manage PXC cluster from a non-privileged user. Necessary permissions can be granted by applying the next clusterrole:

   ```bash
   $ oc create clusterrole pxc-admin --verb="*" --resource=perconaxtradbclusters.pxc.percona.com
   $ oc adm policy add-cluster-role-to-user pxc-admin <some-user>
   ```

5. Finally, it’s time to start the operator within OpenShift:

   ```bash
   $ oc apply -f deploy/operator.yaml
   ```

6. After the operator is started, Percona XtraDB Cluster can be created at any time with the following command:

      ```bash
      $ oc apply -f deploy/cr.yaml
      ```

   Creation process will take some time. The process is over when both operator and replica set pod have reached their Running status:

   ```bash
   $ oc get pods
   NAME                                              READY   STATUS    RESTARTS   AGE
   cluster1-pxc-node-0                               1/1     Running   0          5m
   cluster1-pxc-node-1                               1/1     Running   0          4m
   cluster1-pxc-node-2                               1/1     Running   0          2m
   cluster1-pxc-proxysql-0                           1/1     Running   0          5m
   percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m
   ```

7. Check connectivity to newly created cluster

   ```bash
   $ oc run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
   percona-client:/$ mysql -h cluster1-pxc-proxysql -uroot -proot_password
   ```
