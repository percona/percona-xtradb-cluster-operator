Creating a private S3-compatible cloud for backups
===============================================================================

As it is mentioned in [backups](https://percona.github.io/percona-xtradb-cluster-operator/configure/backups) any cloud storage which implements the S3 API can be used for backups - as third-party one, so your own.
The most simple way to setup and use such storage on Kubernetes or OpenShift is [Minio](https://www.minio.io/) - the S3-compatible object storage server deployed via Docker on your own infrastructure.

Setting up Minio to be used with Percona XtraDB Cluster Operator backups involves following steps:

1. First of all, install Minio in your Kubernetes or OpenShift environment and create the correspondent Kubernetes Service as follows:

   ```bash
      helm install \
        --name minio-service \
        --set accessKey=some-access-key \
        --set secretKey=some-secret-key \
        --set service.type=ClusterIP \
        --set configPath=/tmp/.minio/ \
        --set persistence.size=2G \
        --set persistence.storageClass=aws-io1 \
        --set environment.MINIO_REGION=us-east-1 \
        stable/minio
   ```

   Don't forget to substitute default `some-access-key` and `some-secret-key` strings in this command with some actual unique key values which can be used later for the access control.
   `storageClass` option is needed if you are going to use special [Kubernetes Storage Class](https://kubernetes.io/docs/concepts/storage/storage-classes/) for backups. Otherwise it may be omitted.
   You may also notice `MINIO_REGION` value which is not of much sense within the private cloud. Just use the same region value here and on later steps (`us-east-1` is a good default choice).

2. The next thing to do is to create an S3 bucket for backups:

   ```bash
      kubectl run -i --rm aws-cli --image=perconalab/awscli --restart=Never -- \
       /usr/bin/env AWS_ACCESS_KEY_ID=some-access-key AWS_SECRET_ACCESS_KEY=some-secret-key AWS_DEFAULT_REGION=us-east-1 \
       /usr/bin/aws --endpoint-url http://minio-service:9000 s3 mb s3://operator-testing
   ```

   This command creates the bucket named `operator-testing` with already chosen access and secret keys (substitute `some-access-key` and `some-secret-key` with the values used on the previous step).

3. Now edit the backup section of the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file to set proper values for the `bucket` (the S3 bucket for backups created on the previous step), `region`, `credentialsSecret` and the `endpointUrl` (which should point to the previously created Minio Service). 

   ```
   ...
   backup:
     ...
     storages:
       minio:
         type: s3
         s3:
           bucket: operator-testing
           region: us-west-1
           credentialsSecret: my-cluster-name-backup-minio
           endpointUrl: http://minio-service:9000
     ...
   ```

   The option which should be specially mentioned is `credentialsSecret` which is a [Kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/) for backups. Sample [backup-s3.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/backup-s3.yaml) can be used to create this secret object. Check that it contains proper `name` value (equal to the one specified for `credentialsSecret`, i.e. `my-cluster-name-backup-s3` in the last example), and also proper `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` keys. After editing is finished, secrets object should be created (or updated with the new name and/or keys) using the following command:

   ```bash
   $ kubectl apply -f deploy/backup-s3.yaml
   ```

4. When the setup process is over, making backup is rather simple. Following example illustrates how to make an on-demand backup:

   ```bash
      cat <<EOF | kubectl apply -f-
      apiVersion: "pxc.percona.com/v1alpha1"
      kind: "PerconaXtraDBBackup"
      metadata:
        name: "backup1"
      spec:
        pxcCluster: <cluster-name>
        storageName: <storage>
      EOF
   ```

   Don't forget to specify the name of your cluster instead of the `<cluster-name>` (the same cluster name which is specified in the [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file). Also `<storage>` should be substituted with the actual storage name, which is featured as a subsection inside of the `backups` one in [deploy/cr.yaml](https://github.com/percona/percona-xtradb-cluster-operator/blob/master/deploy/cr.yaml) file. In the upper example it is `minio`.

5. To restore a previously saved backup you will need to specify the backup name. List of available backups can be obtained as follows:

   ```bash
      kubectl get pxc-backup
   ```
   Now, restore the backup, using its name instead of the `backup-name` parameter:

   ```bash
      ./deploy/backup/restore-backup.sh <backup-name> <cluster-name>
   ```

