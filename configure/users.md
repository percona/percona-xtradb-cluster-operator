Users
------------------------

As it was already written in the installation part, the operator requires Kubernetes Secrets to be deployed before it is started. The name of the required secrets can be set in `deploy/cr.yaml` under the `spec.secrets` section.

### General purpose users

Only one general purpose user is created - `proxyuser`.
If you need more general purpose users please run commands below:
```bash
$ kubectl exec -it cluster1-pxc-proxysql-0 -- mysql -h127.0.0.1 -P6032 -uproxyadmin -padmin_password
mysql> INSERT INTO mysql_users(username,password,default_hostgroup) VALUES ('user1','password1',10);
mysql> LOAD MYSQL USERS TO RUNTIME;
mysql> SAVE MYSQL USERS TO MEMORY;
mysql> SAVE MYSQL USERS TO DISK;
```

check newly created user
```bash
   $ kubectl run -i --rm --tty percona-client --image=percona:5.7 --restart=Never -- bash -il
   percona-client:/$ mysql -h cluster1-pxc-proxysql -uuser1 -ppassword1
```

### PXC System Users

*Default Secret name:* `my-cluster-secrets`

*Secret name field:* `spec.secretsName`

The operator requires system-level PXC Users to automate the PXC deployment.

**Warning:** *These users should not be used to run an application.*


|User Purpose        | Username         | Password Secret Key | Description                     |
|--------------------|------------------|---------------------|---------------------------------|
|Admin               | root             | root                | Database administrative user, should be used for maintenance tasks only |
|ProxySQL Admin      | proxyadmin       | proxyadmin          | ProxySQL administrative user, can be used for [adding new general purpouse ProxySQL users](https://github.com/sysown/proxysql/wiki/Users-configuration#creating-a-new-user). |
|Backup              | xtrabackup       | xtrabackup          | [User for run backups](https://www.percona.com/doc/percona-xtrabackup/2.4/using_xtrabackup/privileges.html) |
|Cluster Check       | clustercheckuser | clustercheck        | [User for liveness and readiness checks](http://galeracluster.com/documentation-webpages/monitoringthecluster.html) |
|PMM Client User     | monitor          | monitor             | [User for PMM agent](https://www.percona.com/doc/percona-monitoring-and-management/security.html#pmm-security-password-protection-enabling) |
|PMM Server Password | should be set via [operator options](operator) | pmmserver | [password to access PMM Server](https://www.percona.com/doc/percona-monitoring-and-management/security.html#pmm-security-password-protection-enabling) |

### Development Mode

To make development and testing easier, a secrets file with default PXC System User/Passwords is located at `deploy/secrets.yaml`.

The development-mode credentials from `deploy/secrets.yaml` are:

|Secret Key   | Secret Value                    |
|-------------|---------------------------------|
|root         | <pre>root_password</pre>        |
|xtrabackup   | <pre>backup_password</pre>      |
|monitor      | <pre>monitor</pre>              |
|clustercheck | <pre>clustercheckpassword</pre> |
|proxyuser    | <pre>s3cret</pre>               |
|proxyadmin   | <pre>admin_password</pre>       |
|pmmserver    | <pre>supa|^|pazz</pre>          |

**Warning:** *Do not use the default PXC Users in production!*


