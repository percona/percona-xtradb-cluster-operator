### prerequirements
CentOS
```
sudo yum -y install epel-release
sudo yum -y install coreutils sed jq
```
MacOS
```
brew install coreutils gnu-sed jq
```
### Build images
```
./e2e-tests/build
```
### Build images and run cluster
```
./e2e-tests/build-and-run
```
### Run tests
```
./e2e-tests/run
```
