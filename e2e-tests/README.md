# Building and testing the Operator

## Requirements

You need to install a number of software packages on your system to satisfy the build dependencies for building the Operator and/or to run its automated tests.

### CentOS

```
sudo yum -y install epel-release https://repo.percona.com/yum/percona-release-latest.noarch.rpm
sudo yum -y install coreutils sed jq curl percona-xtrabackup-24 yq
curl -s -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz \
    | tar -C /usr/bin --strip-components 1 --wildcards -zxvpf - '*/oc' '*/kubectl'
curl -s https://get.helm.sh/helm-v3.2.4-linux-amd64.tar.gz \
    | tar -C /usr/bin --strip-components 1 -zxvpf - '*/helm'
curl https://sdk.cloud.google.com | bash
```

### MacOS

```
brew install coreutils gnu-sed jq kubernetes-cli openshift-cli kubernetes-helm percona-xtrabackup yq
curl https://sdk.cloud.google.com | bash
```

Also, you need a Kubernetes platform of [supported version](https://www.percona.com/doc/kubernetes-operator-for-pxc/System-Requirements.html#officially-supported-platforms), available via [EKS](https://www.percona.com/doc/kubernetes-operator-for-pxc/eks.html), [GKE](https://www.percona.com/doc/kubernetes-operator-for-pxc/gke.html), [OpenShift](https://www.percona.com/doc/kubernetes-operator-for-pxc/openshift.html) or [minikube](https://www.percona.com/doc/kubernetes-operator-for-pxc/minikube.html) to run the Operator.

## Building and testing the Operator with DockerHub

There are scripts which build the image and run tests. Both building and testing
need some repository for the newly created docker images. If nothing
specified, scripts use Percona's experimental repository `perconalab/percona-xtradb-cluster-operator`, which
obviously requires decent access rights to make a push.

To specify your own repository for the Percona XtraDB Cluster Operator docker image, you can use IMAGE environment variable:

```
export IMAGE=bob/my_repository_for_test_images:K8SPXC-622-fix-feature-X
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

Tests can be executed one-by-one also, using the appropriate scripts (their names should be self-explanatory):


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

## Building and testing the Operator without DockerHub

The first thing you need to do is fixing image names, to use your custom Docker registry. For example, if your custom registry has the `172.30.162.173:5000` address, you can proceed with the following command:

```
export IMAGE=172.30.162.173:5000/namespace/repo:tag
```

Now you can use the following script to builds the image:

```
./e2e-tests/build
```

**Note:** there is no need to build an image if you are going to test some already-released version.

Running all tests at once can be done with the following command:

```
./e2e-tests/run
```

Tests can be executed one-by-one also, using the appropriate scripts (their names should be self-explanatory):

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

If the test failed, rerun it at least 3 times.

## Using environment variables to customize the building and testing process

### Re-declaring default image names

You can use environment variables to re-declare all default images used for testing. The
full list of variables is the following one:

```
IMAGE_PXC=${IMAGE_PXC:-"perconalab/percona-xtradb-cluster-operator:main-pxc8.0"}
IMAGE_PMM=${IMAGE_PMM:-"perconalab/pmm-client:dev-latest"}
IMAGE_PROXY=${IMAGE_PROXY:-"perconalab/percona-xtradb-cluster-operator:main-proxysql"}
IMAGE_HAPROXY=${IMAGE_HAPROXY:-"perconalab/percona-xtradb-cluster-operator:main-haproxy"}
IMAGE_BACKUP=${IMAGE_BACKUP:-"perconalab/percona-xtradb-cluster-operator:main-pxc8.0-backup"}
IMAGE_LOGCOLLECTOR=${IMAGE_LOGCOLLECTOR:-"perconalab/percona-xtradb-cluster-operator:main-logcollector"}
```

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

Making backups [on S3-compatible storage](https://www.percona.com/doc/kubernetes-operator-for-pxc/backups.html#making-scheduled-backups) needs creating Secrets to have the access to the S3 buckets. There is an environment variable which allows to skip all tests which require such Secrets.

```
SKIP_BACKUPS_TO_AWS_GCP=1
```

The backups tests will use only [MinIO](https://min.io/) if this variable is declared,
which is enough for local testing.

