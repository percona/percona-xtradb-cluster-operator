Install Percona server for MongoDB on OpenShift
-----------------------------------------------

0. First of all, clone the percona-xtradb-cluster-operator repository:

   ```bash
   git clone -b release-0.1.0 https://github.com/Percona-Lab/percona-xtradb-cluster-operator
   cd percona-xtradb-cluster-operator
   ```

1. The next thing to do is to create a new `pxc` project:

   ```bash
   $ oc new-project pxc
   ```

2. Now that’s time to add the PXC Users secrets to OpenShift. They should be placed in the data section of the `deploy/secrets.yaml` file as base64-encoded logins and passwords for the MongoDB specific user accounts (see https://kubernetes.io/docs/concepts/configuration/secret/ for details).

   **Note:** *the following command can be used to get base64-encoded password from a plain text string:* `$ echo -n 'plain-text-password' | base64`

   After editing is finished, mongodb-users secrets should be created (or updated with the new passwords) using the following command:

   ```bash
   $ oc create -f deploy/secrets.yaml
   ```

   More details about secrets can be found in a [separate section](../configure/secrets).

3. Now RBAC (role-based access control) and Custom Resource Definition for PXC should be created from the following two files: `deploy/rbac.yaml` and `deploy/crd.yaml`. Briefly speaking, role-based access is based on specifically defined roles and actions corresponding to them, allowed to be done on specific Kubernetes resources (details about users and roles can be found in [OpenShift documentation](https://docs.openshift.com/enterprise/3.0/architecture/additional_concepts/authorization.html)). Custom Resource Definition extends the standard set of resources which Kubernetes “knows” about with the new items (in our case ones which are the core of the percona-server-mongodb-operator).

   ```bash
   $ oc project psmdb
   $ oc apply -f deploy/crd.yaml -f deploy/rbac.yaml
   ```

   **Note:** *This step requires your user to have cluster-admin role privileges.*

4. An extra step is needed if you want to manage PXC cluster from a non-privileged user. Necessary permissions can be granted by applying the next clusterrole:

   ```bash
   $ oc create clusterrole psmdb-admin --verb="*" --resource=perconaservermongodbs.psmdb.percona.com
   $ oc adm policy add-cluster-role-to-user pxc-admin <some-user>
   ```

5. Finally, it’s time to start the percona-server-mongodb-operator within OpenShift:

   ```bash
   $ oc create -f deploy/operator.yaml
   ```

6. After the operator is started, Percona Server for MongoDB cluster can be created at any time with the following two steps:

   a. Uncomment the `deploy/cr.yaml` field `#platform:` and set it to `platform: openshift`. The result should be like this:

     ```yaml
     apiVersion: psmdb.percona.com/v1alpha1
     kind: PerconaServerMongoDB
     metadata:
       name: my-cluster-name
     spec:
       platform: openshift
     ...
     ```

   b. Create/apply the CR file:

      ```bash
      $ oc apply -f deploy/cr.yaml
      ```

   Creation process will take some time. The process is over when both operator and replica set pod have reached their Running status:

   ```bash
   $ oc get pods
   NAME                                               READY   STATUS    RESTARTS   AGE
   my-cluster-name-rs0-0                              1/1     Running   0          8m
   my-cluster-name-rs0-1                              1/1     Running   0          8m
   my-cluster-name-rs0-2                              1/1     Running   0          7m
   percona-server-mongodb-operator-754846f95d-sf6h6   1/1     Running   0          9m
   ```

7. Check connectivity to newly created cluster 

   ```bash
   $ oc run -i --rm --tty percona-client --image=percona/percona-server-mongodb:3.6 --restart=Never -- bash percona-client:/$ mongo "mongodb+srv://userAdmin:userAdmin123456@my-cluster-name-rs0.psmdb.svc.cluster.local/admin?ssl=false"
   ```
