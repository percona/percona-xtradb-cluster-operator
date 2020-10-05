## Prerequirements
CentOS
```
sudo yum -y install epel-release https://repo.percona.com/yum/percona-release-latest.noarch.rpm
sudo yum -y install coreutils sed jq curl percona-xtrabackup-24 yq
curl -s -L https://github.com/openshift/origin/releases/download/v3.11.0/openshift-origin-client-tools-v3.11.0-0cbc58b-linux-64bit.tar.gz \
    | tar -C /usr/bin --strip-components 1 --wildcards -zxvpf - '*/oc' '*/kubectl'
curl -s https://get.helm.sh/helm-v3.2.4-linux-amd64.tar.gz \
    | tar -C /usr/bin --strip-components 1 -zxvpf - '*/helm'
curl https://sdk.cloud.google.com | bash
```
MacOS
```
brew install coreutils gnu-sed jq kubernetes-cli openshift-cli kubernetes-helm percona-xtrabackup yq
curl https://sdk.cloud.google.com | bash
```
## With DockerHub
### Build image
```
./e2e-tests/build
```
### Build images and run cluster
```
./e2e-tests/build-and-run
```
### Run tests
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
## Without DockerHub
### fix image names
e.g. `172.30.162.173:5000` custom docker registry
```
sed -i -e 's^perconalab^172.30.162.173:5000/namespace^' e2e-tests/functions e2e-tests/conf/*.yml e2e-tests/*/conf/*.yml
```
### Build image (not needed if released version testing)
```
./e2e-tests/build
```
### Run tests
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
if test failed, rerun it at least 3 times
*NB*: each test creates own namespace and doesn't cleanup objects in case of failure
