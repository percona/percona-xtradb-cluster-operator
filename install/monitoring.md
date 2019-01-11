Monitoring
------------------------------------------------

The Percona Monitoring and Management (PMM) [provides an excellent solution](https://www.percona.com/doc/percona-xtradb-cluster/LATEST/manual/monitoring.html#using-pmm) to monitor Percona XtraDB Cluster.

Following steps are needed to install PMM on Kubernetes.

1. The recommended installation approach is based on using [Helm](https://github.com/helm/helm) - the Kubernetes Package Manager, which will substantially simplify further steps. So first thing to do is to install helm following its [official installation instructions](https://docs.helm.sh/using_helm/#installing-helm).

2. When the helm is installed, use following commands to add Percona chart repository and update information of available charts:

   ```
   $ helm repo add percona https://percona-charts.storage.googleapis.com
   $ helm repo update
   ```

3. Now helm can be used to install PMM Server in the following way:

   ```
   $ helm install percona/pmm-server --name monitoring --set platform=openshift --set credentials.username=pmm --set “credentials.password=supa|^|pazz”
   ```
   It is important to specify correct options in the installation command:
   * `platform` should be either `kubernetes` or `openshift` depending on which platform are you using.
   * `credentials.password` should correspond to the `pmmserver` one specified in `deploy/secrets.yaml` secrets file. Password specified in the mentioned above example command is default development mode password, and it is not intended to be used on production systems.
   * set pmmserver password in secrets and apply **TODO**

4. When the PMM Server is installed, it is the time to update ``pmm`` section in the [deploy/cr.yaml](https://github.com/Percona-Lab/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file and apply it.
   * set `pmm.enabled=true`
   * set `serverUser` to the PMM Server user name, or leave the default `pmm` one.
   * service name in CR  and apply it **TODO**

5. Check PMM Client error log for PXC Pods restarting:

   ```
   $ kubectl logs cluster1-pxc-node-0 -c pmm-client
   ```

6. Find the external IP address (`EXTERNAL-IP` field in the output of `kubectl get service/monitoring-service -o wide`). This IP address can be used to access PMM via *https* in a web browser, with login/password authentication.
