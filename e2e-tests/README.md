# Building and testing the Operator

## Requirements

You need to install a number of software packages on your system to satisfy the build dependencies for building the Operator and/or to run its automated tests.

### CentOS

Run the following commands to install the required components:

```
sudo yum -y install epel-release https://repo.percona.com/yum/percona-release-latest.noarch.rpm
sudo yum -y install coreutils sed jq curl docker percona-xtrabackup-24
sudo curl -s -L https://github.com/mikefarah/yq/releases/download/4.27.2/yq_linux_amd64 -o /usr/bin/yq
sudo chmod a+x /usr/bin/yq
curl -s -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz \
    | tar -C /usr/bin --strip-components 1 --wildcards -zxvpf - '*/oc' '*/kubectl'
curl -s https://get.helm.sh/helm-v3.2.4-linux-amd64.tar.gz \
    | tar -C /usr/bin --strip-components 1 -zxvpf - '*/helm'
curl https://sdk.cloud.google.com | bash
```

### MacOS

Install [Docker](https://docs.docker.com/docker-for-mac/install/), and run the following commands for the other required components:

```
brew install coreutils gnu-sed jq yq kubernetes-cli openshift-cli kubernetes-helm percona-xtrabackup
curl https://sdk.cloud.google.com | bash
```

### Runtime requirements

Also, you need a Kubernetes platform of [supported version](https://www.percona.com/doc/kubernetes-operator-for-pxc/System-Requirements.html#officially-supported-platforms), available via [EKS](https://www.percona.com/doc/kubernetes-operator-for-pxc/eks.html), [GKE](https://www.percona.com/doc/kubernetes-operator-for-pxc/gke.html), [OpenShift](https://www.percona.com/doc/kubernetes-operator-for-pxc/openshift.html) or [minikube](https://www.percona.com/doc/kubernetes-operator-for-pxc/minikube.html) to run the Operator.

**Note:** there is no need to build an image if you are going to test some already-released version.

## Building and testing the Operator

There are scripts which build the image and run tests. Both building and testing
needs some repository for the newly created docker images. If nothing is
specified, scripts use Percona's experimental repository `perconalab/percona-xtradb-cluster-operator`, which
requires decent access rights to make a push.

To specify your own repository for the Operator docker image, you can use IMAGE environment variable:

```
export IMAGE=bob/my_repository_for_test_images:K8SPXC-622-fix-feature-X
```
We use linux/amd64 platform by default. To specify another platform, you can use DOCKER_DEFAULT_PLATFORM environment variable

```
export DOCKER_DEFAULT_PLATFORM=linux/amd64
```

Use the following script to build the image:

```
./e2e-tests/build
```

You can also build the image and run your cluster in one command:

```
./e2e-tests/build-and-run
```

Running all tests at once can be done with the following command:

```
./e2e-tests/run
```

(see how to configure the testing infrastructure [here](#using-environment-variables-to-customize-the-testing-process)).

Tests can also be run one-by-one using the appropriate scripts (their names should be self-explanatory):

```
./e2e-tests/init-deploy/run
./e2e-tests/recreate/run
./e2e-tests/limits/run
./e2e-tests/scaling/run
./e2e-tests/monitoring/run
./e2e-tests/affinity/run
./e2e-tests/demand-backup/run
./e2e-tests/scheduled-backup/run
./e2e-tests/storage/run
./e2e-tests/self-healing/run
./e2e-tests/operator-self-healing/run
....
```

## Using environment variables to customize the testing process

### Re-declaring default image names

You can use environment variables to re-declare all default docker images used for testing. The
full list of variables is the following one:

* `IMAGE` - the Operator, `perconalab/percona-xtradb-cluster-operator:main` by default,
* `IMAGE_PXC` - Percona XtraDB Cluster, `perconalab/percona-xtradb-cluster-operator:main-pxc8.0` by default,
* `IMAGE_PMM` - Percona Monitoring and Management (PMM) client, `perconalab/pmm-client:dev-latest` by default,
* `IMAGE_PROXY` - ProxySQL, `perconalab/percona-xtradb-cluster-operator:main-proxysql` by default,
* `IMAGE_HAPROXY` - HA Proxy, `perconalab/percona-xtradb-cluster-operator:main-haproxy` by default,
* `IMAGE_BACKUP` - backups, `perconalab/percona-xtradb-cluster-operator:main-pxc8.0-backup` by default,
* `IMAGE_LOGCOLLECTOR` - Log Collector, `perconalab/percona-xtradb-cluster-operator:main-logcollector` by default,

### Running the Operator cluster-wide

Also, you can run the Operator for the tests in a [cluster-wide mode](https://www.percona.com/doc/kubernetes-operator-for-pxc/cluster-wide.html) if needed. This feature is turned on if the following variable is declared:

```
export OPERATOR_NS=pxc-operator
```

### Using automatic clean-up after testing

By default, each test creates its own namespace and does not clean up objects in case of failure.

To avoid manual deletion of such leftovers, you can run tests on a separate cluster and use the following environment variable to make the ultimate clean-up:

```
export CLEAN_NAMESPACE=1
```

**Note:** this will cause **deleting all namespaces** except default and system ones!

### Skipping backup tests on S3-compatible storage

Making backups [on S3-compatible storage](https://www.percona.com/doc/kubernetes-operator-for-pxc/backups.html#making-scheduled-backups) needs creating Secrets to have the access to the S3 buckets. There is an environment variable enabled by default, which skips all tests requiring such Secrets:

```
SKIP_BACKUPS_TO_AWS_GCP=1
```

The backups tests will use only [MinIO](https://min.io/) if this variable is declared,
which is enough for local testing.
