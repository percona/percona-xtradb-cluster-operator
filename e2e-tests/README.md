# Building and testing the operator

## Requirements

You should install a number of dependencies on your system to build the Operator and/or run its automated tests.

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

## Building and testing the Operator with DockerHub

The following script builds the image:

```
./e2e-tests/build
```

You can also build the image and run your cluster in one command:

```
./e2e-tests/build-and-run
```

Tests can be executed one-by-one, with the following scripts (their self-explanatory names make no needs in additional explanations):


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

Tests can be executed one-by-one, with the following scripts (their self-explanatory names make no needs in additional explanations):

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
```

If the test failed, rerun it at least 3 times.

**Note:** Each test creates its own namespace and doesn't clean up objects in case of failure.

