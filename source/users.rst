Users
=====

As it is written in the installation part, the operator requires
Kubernetes Secrets to be deployed before it is started. The name of the
required secrets can be set in ``deploy/cr.yaml`` under the
``spec.secrets`` section.

Unprivileged users
------------------

There are no unprivileged (general purpose) user accounts created by
default. If you need general purpose users, please run commands below:

.. code:: bash

   $ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- mysql -hcluster1-pxc -uroot -proot_password
   mysql> GRANT ALL PRIVILEGES ON database1.* TO 'user1'@'%' IDENTIFIED BY 'password1';

Sync users on the ProxySQL node:

.. code:: bash

   $ kubectl exec -it cluster1-pxc-proxysql-0 -- proxysql-admin --config-file=/etc/proxysql-admin.cnf --syncusers

Now check the newly created user. If everything is Ok with it, the
following command will let you successfully login to MySQL shell via
ProxySQL:

.. code:: bash

   $ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- bash -il
   percona-client:/$ mysql -h cluster1-proxysql -uuser1 -ppassword1
   mysql> SELECT * FROM database1.table1 LIMIT 1;

You may also try executing any simple SQL statement to make sure
permissions have been successfully granted.

System Users
------------

*Default Secret name:* ``my-cluster-secrets``

*Secret name field:* ``spec.secretsName``

The Operator requires system-level PXC users to automate the PXC
deployment.

**Warning:** *These users should not be used to run an application.*

+--------------+-------------+---------------+------------------------+
| User Purpose | Username    | Password      | Description            |
|              |             | Secret Key    |                        |
+==============+=============+===============+========================+
| Admin        | root        | root          | Database               |
|              |             |               | administrative user,   |
|              |             |               | should be used for     |
|              |             |               | maintenance tasks only |
+--------------+-------------+---------------+------------------------+
| ProxySQL     | proxyadmin  | proxyadmin    | ProxySQL               |
| Admin        |             |               | administrative user,   |
|              |             |               | can be used for        |
|              |             |               | `adding new general    |
|              |             |               | purpouse ProxySQL      |
|              |             |               | users <https://github. |
|              |             |               | com/sysown/proxysql/wi |
|              |             |               | ki/Users-configuration |
|              |             |               | #creating-a-new-       |
|              |             |               | user>`__               |
+--------------+-------------+---------------+------------------------+
| Backup       | xtrabackup  | xtrabackup    | `User for run          |
|              |             |               | backups <https://www.p |
|              |             |               | ercona.com/doc/percona |
|              |             |               | -xtrabackup/2.4/using_ |
|              |             |               | xtrabackup/privileges. |
|              |             |               | html>`__               |
+--------------+-------------+---------------+------------------------+
| Cluster      | clusterchec | clustercheck  | `User for liveness and |
| Check        | kuser       |               | readiness              |
|              |             |               | checks <http://galerac |
|              |             |               | luster.com/documentati |
|              |             |               | on-webpages/monitoring |
|              |             |               | thecluster.html>`__    |
+--------------+-------------+---------------+------------------------+
| PMM Client   | monitor     | monitor       | `User for PMM          |
| User         |             |               | agent <https://www.per |
|              |             |               | cona.com/doc/percona-m |
|              |             |               | onitoring-and-manageme |
|              |             |               | nt/security.html#pmm-s |
|              |             |               | ecurity-password-prote |
|              |             |               | ction-enabling>`__     |
+--------------+-------------+---------------+------------------------+
| PMM Server   | should be   | pmmserver     | `password to access    |
| Password     | set via     |               | PMM                    |
|              | `operator   |               | Server <https://www.pe |
|              | options <op |               | rcona.com/doc/percona- |
|              | erator>`__  |               | monitoring-and-managem |
|              |             |               | ent/security.html#pmm- |
|              |             |               | security-password-prot |
|              |             |               | ection-enabling>`__    |
+--------------+-------------+---------------+------------------------+

Development Mode
----------------

To make development and testing easier, ``deploy/secrets.yaml`` secrets
file contains default passwords for PXC system users.

These development mode credentials from ``deploy/secrets.yaml`` are:

============ ========================
Secret Key   Secret Value
============ ========================
root         ``root_password``
xtrabackup   ``backup_password``
monitor      ``monitor``
clustercheck ``clustercheckpassword``
proxyuser    ``s3cret``
proxyadmin   ``admin_password``
pmmserver    ``supa|^|pazz``
============ ========================

**Warning:** *Do not use the default PXC user passwords in production!*
