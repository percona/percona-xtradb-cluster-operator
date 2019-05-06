Users
------------------------

As it is written in the installation part, the operator requires Kubernetes Secrets to be deployed before it is started. The name of the required secrets can be set in `deploy/cr.yaml` under the `spec.secrets` section.

### Unprivileged users

There are no unprivileged (general purpose) user accounts created by default. If you need general purpose users, please run commands below:
```bash
$ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- mysql -hcluster1-pxc-nodes -uroot -proot_password
mysql> GRANT ALL PRIVILEGES ON database1.* TO 'user1'@'%' IDENTIFIED BY 'password1';
```

Sync users on the ProxySQL node:
```bash
$ kubectl exec -it some-name-pxc-proxysql-0 -- proxysql-admin --config-file=/etc/proxysql-admin.cnf --syncusers
```

Now check the newly created user. If everything is Ok with it, the following command will let you successfully login to MySQL shell via ProxySQL:
```bash
$ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- bash -il
percona-client:/$ mysql -h cluster1-pxc-proxysql -uuser1 -ppassword1
mysql> SELECT * FROM database1.table1 LIMIT 1;
```
You may also try executing any simple SQL statement to make sure permissions have been successfully granted.

### System Users

The Operator requires system-level PXC users to automate the PXC deployment.

To make development and testing easier, `deploy/secrets.yaml` secrets file contains default passwords for PXC system users.


**Warning:** *These users should not be used to run an application. Do not use the default PXC user passwords in production!*

| User name      | Password             |Unencrypted password | Description                             |
|----------------|----------------------|---------------------|-----------------------------------------|
| root           | cm9vdF9wYXNzd29yZA== | `root_password`       | Database administrative user - should be used only for maintenance tasks |
| xtrabackup     | YmFja3VwX3Bhc3N3b3Jk | `backup_password`     | [User able to run backups](https://www.percona.com/doc/percona-xtrabackup/2.4/using_xtrabackup/privileges.html) |
| monitor        | bW9uaXRvcg==         | `monitor`             | [User for PMM agent](https://percona.github.io/percona-xtradb-cluster-operator/configure/users) |
| clustercheck   | Y2x1c3RlcmNoZWNrcGFzc3dvcmQ= | `custercheckpassword` | [User for liveness and readiness checks](http://galeracluster.com/documentation-webpages/monitoringthecluster.html) |
| proxyadmin     | YWRtaW5fcGFzc3dvcmQ= | `admin_password`      | ProxySQL administrative user who can be used [for adding new general purpose ProxySQL users](https://github.com/sysown/proxysql/wiki/Users-configuration#creating-a-new-user)|
| pmmserver      | c3VwYXxefHBheno= | `supa|^|pazz` | Used to access PMM Server |
