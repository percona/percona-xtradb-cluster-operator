Secrets
------------------------

As it was already written in the installation part, the operator requires Kubernetes Secrets to be deployed before it is started. The name of the required secrets can be set in `deploy/cr.yaml` under the `spec.secrets` section.

### MongoDB System Users

*Default Secret name:* `my-cluster-secrets`

*Secret name field:* `spec.secrets.users`

The operator requires system-level MongoDB Users to automate the MongoDB deployment. 

**Warning:** *These users should not be used to run an application.*


|User Purpose   | Username Secret Key | Password Secret Key     | MongoDB Role                    |
|---------------|---------------------|-------------------------|---------------------------------|
|Backup/Restore | MONGODB_BACKUP_USER | MONGODB_BACKUP_PASSWORD | [backup](https://docs.mongodb.com/manual/reference/built-in-roles/#backup), [clusterMonitor](https://docs.mongodb.com/manual/reference/built-in-roles/#clusterMonitor), [restore](https://docs.mongodb.com/manual/reference/built-in-roles/#restore) |
|Cluster Admin  | MONGODB_CLUSTER_ADMIN_USER | MONGODB_CLUSTER_ADMIN_PASSWORD | [clusterAdmin](https://docs.mongodb.com/manual/reference/built-in-roles/#clusterAdmin)      |
|Cluster Monitor| MONGODB_CLUSTER_MONITOR_USER| MONGODB_CLUSTER_MONITOR_PASSWORD | [clusterMonitor](https://docs.mongodb.com/manual/reference/built-in-roles/#clusterMonitor) |
|User Admin     | MONGODB_USER_ADMIN_USER    | MONGODB_USER_ADMIN_PASSWORD    | [userAdmin](https://docs.mongodb.com/manual/reference/built-in-roles/#userAdmin)         |

### Development Mode

To make development and testing easier, a secrets file with default MongoDB System User/Passwords is located at `deploy/mongodb-users.yaml`.

The development-mode credentials from `deploy/mongodb-users.yaml` are:

|Secret Key   | Secret Value                    |
|-------------|---------------------------------|
|root         | <pre>root_password</pre>        |
|xtrabackup   | <pre>backup_password</pre>      |
|monitor      | <pre>monitor</pre>              |
|clustercheck | <pre>clustercheckpassword</pre> |
|proxyuser    | <pre>s3cret</pre>               |
|pmmserver    | <pre>supa|^|pazz</pre>          |

**Warning:** *Do not use the default PXC Users in production!*


