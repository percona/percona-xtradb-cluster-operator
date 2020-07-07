.. _users:

Users
==============================

MySQL user accounts within the Cluster can be divided into two different groups:

* application-level (unprivileged) users,
* system-level users (accounts needed to automate the deployment and management
  of the cluster components, such as PXC Health checks or ProxySQL integration).

As these two groups of user accounts serve different purposes, they are
considered separately in the following sections.

.. contents:: :local:

.. _users.unprivileged-users:

`Unprivileged users <users.html#unprivileged-users>`_
------------------------------------------------------

There are no unprivileged (general purpose) user accounts created by
default. If you need general purpose users, please run commands below:

.. code-block:: bash

   $ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- mysql -hcluster1-pxc -uroot -proot_password
   mysql> GRANT ALL PRIVILEGES ON database1.* TO 'user1'@'%' IDENTIFIED BY 'password1';

.. note:: MySQL password here should not exceed 32 characters due to the `replication-specific limit introduced in MySQL 5.7.5 <https://dev.mysql.com/doc/relnotes/mysql/5.7/en/news-5-7-5.html>`_.

Sync users on the ProxySQL node:

.. code-block:: bash

   $ kubectl exec -it cluster1-proxysql-0 -- proxysql-admin --config-file=/etc/proxysql-admin.cnf --syncusers

Verify that the user was created successfully. If successful, the
following command will let you successfully login to MySQL shell via
ProxySQL:

.. code:: bash

   $ kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- bash -il
   percona-client:/$ mysql -h cluster1-proxysql -uuser1 -ppassword1
   mysql> SELECT * FROM database1.table1 LIMIT 1;

You may also try executing any simple SQL statement to ensure the 
permissions have been successfully granted.

.. _users.system-users:

`System Users <users.html#system-users>`_
-------------------------------------------

To automate the deployment and management of the cluster components,
the Operator requires system-level PXC users.

Credentials for these users are stored as a `Kubernetes Secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_ object.
The Operator requires to be deployed before the PXC Cluster is started. The name
of the required secrets (``my-cluster-secrets`` by default) should be set in
in the ``spec.secretsName`` option of the ``deploy/cr.yaml`` configuration file.

The following table shows system users' names and purposes.

.. warning:: These users should not be used to run an application.

.. tabularcolumns:: |p{1.5cm}|p{1.5cm}|p{1.5cm}|p{2.5cm}|L|

.. list-table::
    :header-rows: 1

    * - User Purpose
      - Username
      - Password Secret Key
      - Description
    * - Admin
      - root
      - root
      - Database administrative user, should only be used for maintenance tasks
    * - ProxySQLAdmin   
      - proxyadmin
      - proxyadmin
      - ProxySQL administrative user, can be used to `add general-purpose ProxySQL users <https://github.com/sysown/proxysql/wiki/Users-configuration>`__
    * - Backup
      - xtrabackup
      - xtrabackup
      - `User to run backups <https://www.percona.com/doc/percona-xtrabackup/2.4/using_xtrabackup/privileges.html>`__
    * - Cluster Check
      - clustercheck
      - clustercheck
      - `User for liveness checks and readiness checks <http://galeracluster.com/library/documentation/monitoring-cluster.html>`__
    * - Monitoring
      - monitor
      - monitor 
      - User for internal monitoring purposes and `PMM agent <https://www.percona.com/doc/percona-monitoring-and-management/security.html#pmm-security-password-protection-enabling>`__
    * - PMM Server Password
      - should be set through the `operator options <operator>`__
      - pmmserver
      - `Password used to access PMM Server <https://www.percona.com/doc/percona-monitoring-and-management/security.html#pmm-security-password-protection-enabling>`__

YAML Object Format
******************

The default name of the Secrets object for these users is
``my-cluster-secrets`` and can be set in the CR for your cluster in
``spec.secretName`` to something different. When you create the object yourself,
it should match the following simple format:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: my-cluster-secrets
   type: Opaque
   data:
     root: cm9vdF9wYXNzd29yZA==
     xtrabackup: YmFja3VwX3Bhc3N3b3Jk
     monitor: bW9uaXRvcg==
     clustercheck: Y2x1c3RlcmNoZWNrcGFzc3dvcmQ=
     proxyadmin: YWRtaW5fcGFzc3dvcmQ=
     pmmserver: c3VwYXxefHBheno=

The example above matches
:ref:`what is shipped in deploy/secrets.yaml<users.development-mode>` which
contains default passwords. You should NOT use these in production, but they are
present to assist in automated testing or simple use in a development
environment.

As you can see, because we use the ``data`` type in the Secrets object, all
values for each key/value pair must be encoded in base64. To do this you can
simply run ``echo -n "password" | base64`` in your local shell to get valid
values.

Password Rotation Policies and Timing
*************************************

When there is a change in user secrets or ``secretName`` option, the Operator
creates the necessary transaction to change passwords. This rotation happens
almost instantly (the delay can be up to a few seconds), and it's not needed to
take any action beyond changing the password.

Marking System Users In MySQL
*****************************

Starting with MySQL 8.0.16, a new feature called Account Categories has been
implemented, which allows us to mark our system users as such.
See `the official documentation on this feature <https://dev.mysql.com/doc/refman/8.0/en/account-categories.html>`_
for more details.

.. _users.development-mode:

`Development Mode <users.html#development-mode>`_
--------------------------------------------------

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

.. warning:: Do not use the default PXC user passwords in production!
