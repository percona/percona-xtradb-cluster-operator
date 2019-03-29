Monitoring
------------------------------------------------

The Percona Monitoring and Management (PMM) [provides an excellent solution](https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm) to monitor Percona XtraDB Cluster.

### Installing the PMM Server

Following steps are needed to install the PMM Server to monitor Percona XtraDB Cluster on Kubernetes or OpenShift.

1. The recommended installation approach is based on using [helm](https://github.com/helm/helm) - the package manager for Kubernetes, which will substantially simplify further steps. So first thing to do is to install helm following its [official installation instructions](https://docs.helm.sh/using_helm/#installing-helm).

2. When the helm is installed, add Percona chart repository and update information of available charts as follows:

   ```
   $ helm repo add percona https://percona-charts.storage.googleapis.com
   $ helm repo update
   ```

3. Now helm can be used to install PMM Server:

   ```
   $ helm install percona/pmm-server --name monitoring --set platform=openshift --set credentials.username=pmm --set "credentials.password=supa|^|pazz"
   ```
   It is important to specify correct options in the installation command:
   * `platform` should be either `kubernetes` or `openshift` depending on which platform are you using.
   * `name` should correspond to the `serverHost` key in the `pmm` section of the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file with a "-service" suffix, so default `--name monitoring` part of the shown above command corresponds to a `monitoring-service` value of the `serverHost` key.
   * `credentials.username` should correspond to the `serverUser` key in the `pmm` section of the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file.
   * `credentials.password` should correspond to a value of the `pmmserver` secret key specified in `deploy/secrets.yaml` secrets file. Note that password specified in this example is the default development mode password not intended to be used on production systems.

### Installing the PMM Client


   The following steps initiate the PMM client installation:

1. The PMM client is initiated by updating the ``pmm`` section in the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file.
   * set `pmm.enabled=true`
   * make sure that `serverUser` (the PMM Server user name, `pmm` by default) is the same as one specified for the `credentials.username` parameter on the previous step.
   * make sure that `serverHost` (the PMM service name, `monitoring-service` by default) is the same as one specified for the `name` parameter on the previous step, but with additional `-service` suffix.
   * make sure that `pmmserver` secret key in the `deploy/secrets.yaml` secrets file is the same as one specified for the `credentials.password` parameter on the previous step (if not, fix it and apply with the `kubectl apply -f deploy/secrets.yaml` command).

   When done, apply the edited `deploy/cr.yaml` file:

      ```
      $ kubectl apply -f deploy/cr.yaml
      ```

2. To make sure everything gone right, check that correspondent Pods are not continuously restarting (which would occur in case of any errors on the previous two steps):

   ```
   $ kubectl get pods
   $ kubectl logs cluster1-pxc-node-0 -c pmm-client
   ```

3. Find the external IP address (`EXTERNAL-IP` field in the output of `kubectl get service/monitoring-service -o wide`). This IP address can be used to access PMM via *https* in a web browser, with the login/password authentication, already configured and able to [show Percona XtraDB Cluster metrics](https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm).
