Use custom registry
------------------------------------------------
lets illustrate on OpenShift registry example

1. lets login on OpenShift.
    ```bash
    $ oc login
    Authentication required for https://192.168.1.100:8443 (openshift)
    Username: admin
    Password:
    Login successful.
    You have one project on this server: "pxc"
    Using project "pxc".
    ```

2. first we need to get the token for our user
    ```bash
    $ oc whoami -t 
    ADO8CqCDappWR4hxjfDqwijEHei31yXAvWg61Jg210s
    ```

3. second we need registry IP
    ```bash
    $ kubectl get services/docker-registry -n default
    NAME              TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
    docker-registry   ClusterIP   172.30.162.173   <none>        5000/TCP   1d
    ```

3. use the token for the login to the registry.
    ```bash
    $ docker login -u admin -p ADO8CqCDappWR4hxjfDqwijEHei31yXAvWg61Jg210s 172.30.162.173:5000
    Login Succeeded
    ```

4. pull image by sha
    ```bash
    $ docker pull docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0
    Trying to pull repository docker.io/perconalab/percona-xtradb-cluster-operator ...
    sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0: Pulling from docker.io/perconalab/percona-xtradb-cluster-operator
    Digest: sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0
    Status: Image is up to date for docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0
    ```

5. push image to custom registry (into pxc project in OpenShift)
    ```bash
    $ docker tag \
    docker.io/perconalab/percona-xtradb-cluster-operator@sha256:8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0 \
    172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:0.2.0
    refusing to create a tag with a digest reference
    $ docker push 172.30.162.173:5000/pxc/percona-xtradb-cluster-operator:0.2.0
    ```

6. check image in OpenShift registry
    ```bash
    $ oc get is
    NAME                              DOCKER REPO                                                            TAGS      UPDATED
    percona-xtradb-cluster-operator   docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator   0.2.0     2 hours ago
    ```

7. put Docker Repo + Tag string (like `docker-registry.default.svc:5000/pxc/percona-xtradb-cluster-operator:0.2.0) into `image:` option in `deploy/operator.yaml`  config.
it is possible to specify imagePullSecrets option for all images if registry require authentification.

8. make 4-6 steps for other images, update corresponding params in `deploy/cr.yaml` file


## Percona certified images
### 0.2.0
| Image                                             | Digest                                                           |
|---------------------------------------------------|------------------------------------------------------------------|
| perconalab/percona-xtradb-cluster-operator:0.2.0  | 8895ff4647602dcbcabbf6ea5d1be1611e9d7a9769c3bb3415c3a73aba2adda0 |
| perconalab/pxc-openshift:0.2.0                    | a9f6568cc71e1e7b5bbfe69b3ea561e2c3bae92a75caba7ffffa88bd3c730bc9 |
| perconalab/proxysql-openshift:0.2.0               | cdd114b82f34312ef73419282a695063387c715d3e80677902938f991ef94f13 |
| perconalab/backupjob-openshift:0.2.0              | 1ded5511a59fc2cc5a6b23234495e6d243d5f8b55e1b6061781779e19887cdc9 |
| perconalab/pmm-client:1.17.0                      | efdce369d5fb29b0a1b03a7026dfbc2efe07b618471aba5db308d0c21b8e118d |
### 0.1.0
| Image                                             | Digest                                                           |
|---------------------------------------------------|------------------------------------------------------------------|
| perconalab/percona-xtradb-cluster-operator:0.1.0  | 9e4b44ef6859e995d70c0ef7db9be9b9c2875d1116a2b6ff7e5a7f5e5fcb39b7 |
| perconalab/pxc-openshift:0.1.0                    | c72eb45c3f103f105f864f05668a2b029bb6a3ba9fc8a1d0467040c6c83f3e53 |
| perconalab/proxysql-openshift:0.1.0               | 482b6f4161aafc78585b3e377a4aec9a983f4e4860e0bd8576f0e39eee52909d |
| perconalab/pmm-client:1.17.0                      | efdce369d5fb29b0a1b03a7026dfbc2efe07b618471aba5db308d0c21b8e118d |
