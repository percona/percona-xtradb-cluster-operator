=====================
Managing MySQL Users
=====================

MySQL user accounts within the Cluster can be divided into two different groups:

* application-level users,
* system-level users (accounts needed to automate the deployment and management of PXC and ProxySQL).

As these two groups of user accounts serve different purposes, they are considered separately in the following sections.

.. contents:: :local:

Application Users Management
==============================

These users can be either managed manually or in an automated way, bound to a Kubernetes secret.

Method 1: Manually Manage Users with MySQL Client
-------------------------------------------------

In order to manually manage users, you can issue ``CREATE USER`` and ``GRANT`` statements as you normally would with any MySQL client connected to the cluster running in the context of the Operator. 

We provide a special Docker image containing a client setup which is intended for running BASH and the MySQL client or executing SQL statements directly with ``kubectl run``. The name of this image is ``percona-client``.  

An example of how you might use it to run a grant statement is as below:

.. code:: bash
 
    kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- mysql -hcluster1-pxc -uroot -proot_password
    mysql> GRANT ALL PRIVILEGES ON database1.* TO 'user1'@'%';

You can refer to the official documentation to find out more about the `CREATE USER <https://dev.mysql.com/doc/refman/8.0/en/create-user.html>`_ and `GRANT <https://dev.mysql.com/doc/refman/8.0/en/grant.html>`_ statements.

After you've created your users and issued grants, it's recommended to also manually sync these users to ProxySQL using ``proxysql-admin``.  The following one-liner can do this for you by running ``proxysql-admin --syncusers`` on the first ProxySQL instance in your cluster.

.. code:: bash

   kubectl exec -it cluster1-proxysql-0 -- proxysql-admin --config-file=/etc/proxysql-admin.cnf --syncusers

You can validate a user synced successfully by connecting through ProxySQL with the MySQL client and then running a simple query.  A simple example of this is below:

.. code:: bash

   kubectl run -it --rm percona-client --image=percona:5.7 --restart=Never -- bash -il
   percona-client:/$ mysql -h cluster1-proxysql -uuser1 -ppassword1
   mysql> SELECT * FROM database1.table1 LIMIT 1;

Method 2: Automatically Manage Users with Kubernetes Secret
-----------------------------------------------------------

YAML Object Format
******************

The YAML Object is stored as a `Kubernetes Secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_ object, containing two sets of data:

1. Some **username:password** pairs that are compatible with `Kubernetes secretKeyRef reference <https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#define-a-container-environment-variable-with-data-from-a-single-secret>`_,
2. A `stringData <https://kubernetes.io/docs/concepts/configuration/secret/#creating-a-secret-manually>`_ field with a specially composed and formatted text.

The text stored in ``stringData`` field is a *YAML fragment*, which contains two *dictionaries of dictionaries*: 

1. **roles dictionary** specifies role grants,
2. **users dictionary** specifies user grants.

The following example illustrates the whole YAML object:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: secrets-for-users
     annotations:
       status: applied
       checksum:
   type: Opaque
   stringData:
     myuser1: password1
     myuser2: password2
     grants.yaml: |-
       roles:
       - rolename: role1
         tables:
          - name: db.table2
   	        privileges: SELECT
       users:
       - username: myuser1
         tables:
           - name: db.table1
             privileges: SELECT
           - name: db.table2
             privileges: DELETE
         hosts:
         - "12.34.56.78"
         - "91.11.12.13"
       - username: myuser2
         tables:
           - name: db.table2
             roles: role1
         hosts:
         - "14.15.16.17"

.. note:: As you can see from the example above, users must be listed in **both** the grants subsection and the *username:password* pairs.

.. note:: The passwords are stored in plain text as `stringData <https://kubernetes.io/docs/concepts/configuration/secret/#creating-a-secret-manually>`_ which is converted by Kubernetes to base64 on commit.  Depending on how you retrieve the data later this may need to be unencoded.

The Operator automatically tracks changes in the ``stringData`` field, if any.

Valid Privileges for Automation
*******************************

This methodology allows the privileges field to be free-form. All valid privileges listed `in the official MySQL documentation <https://dev.mysql.com/doc/refman/8.0/en/grant.html>`_ are supported.

The Operator **does not support** the ``AS user``, ``WITH GRANT OPTION``, ``PROXY``, or ``WITH ADMIN OPTION`` functionality, so this limits some of the use cases for specific privileges. Please use the manual approach above if needed.

Managing and Mapping MySQL Roles
********************************

Roles are defined in the *roles dictionary* within the YAML object. Each role has a set of grants associated with it and a defined name. 

.. note:: Instead of adding a privileges key to the *users dictionary*, you can specify the roles key, and that role will be added to that user by generating a series of one or more ``GRANT role TO user`` statements.

Using Kubernetes Selectors in Your Application
**********************************************

You can make use of Selectors to reference the content of the secret object within your application's Pods running in the same Kubernetes namespace assuming appropriate `RBAC rules <https://kubernetes.io/docs/reference/access-authn-authz/rbac/>`_ are set.

Within your application Pod configuration, you may have something like the following.

.. code:: yaml

   spec:
     containers:
       env:
     - name: SECRET_PASSWORD
       valueFrom:
         secretKeyRef:
           name: secrets-for-users
           key: myuser1

By doing this, your application Pod has an environment variable set named ``SECRET_PASSWORD`` which contains the value of the password for the specified user.  You can then use this in your application to build your connection string to the database.

Password Rotation Policies and Timing
*************************************

Password rotation is performed within moments of the password being changed in the Secrets object. The reason for this is that ProxySQL does not currently support dual passwords, which is a feature added in MySQL 8.0.14.  As such, it's imperative that you are prepared to update your application connection strings shortly after making the necessary password change for a user in the Secrets object.

How Method 1 and Method 2 Interact
----------------------------------

When the Operator detects that user management must happen it generates a single transaction that contains the following:

* one or more ``DROP USER IF EXISTS`` statements,
* one or more ``CREATE USER`` statements,
* one or more `GRANT` statements.

Together these are done in a single transaction for all users in the secret, followed by a ``FLUSH ALL PRIVILEGES;``, so there should be no interruption of existing client connections to the server.

.. note:: Automatic user management wins any conflict between Method 1 and Method 2.  So if you want to manually manage a user, ensure it isn't listed in the Secrets object.

How Users and Passwords Get Synced to ProxySQL
==============================================

Methodology
-----------

The Operator utilizes the ``proxysql-admin`` tool that Percona ships in our ProxySQL packages.  This script has an option called ``syncusers`` which diffs the users list from MySQL and ProxySQL, and imports users from MySQL into ProxySQL.  This is run after the creation of users inside MySQL as part of the user creation and grant process.

If you have added users manually (i.e. with the Method 1), the synchronization can be run manually by executing ``kubectl exec -it cluster1-proxysql-0 -- proxysql-admin --config-file=/etc/proxysql-admin.cnf --syncusers``, or you can wait until it gets run during cluster changes (Pod restart).

Propagation Delays and Other Caveats
------------------------------------

When the ``proxysql-admin --syncusers`` is ran, it deletes any users which no longer exist in MySQL, so it's imperative that it gets run after users are successfully added. As such, there is a short delay during the user creation and grants process, as we run all queries first before executing syncusers.  This delay can be up to a few seconds.

Additionally, if you are not making use of the automated processes for managing users, you are also responsible for manually syncing users to ProxySQL.

If you have manually added some users and some have been added afterward using the automated method, syncusers will cause both sets of users to sync to ProxySQL, and the Operator will not interact with or otherwise harm the users you created manually.

If you do the automated piece after the manual piece, syncusers gets run automatically.


Administration / System Users Management
========================================

Users Required by The Operator
------------------------------

In order to automate the deployment and management of Percona XtraDB Cluster and ProxySQL, the Operator requires system-level PXC users.  The minimal set of users is ``root``, ``proxyadmin``, ``xtrabackup``, ``clustercheck``, and ``monitor``.

The purposes are relatively self-evident from the names, but a detailed table can be found :ref:`in a dedicated section<users.system-users>` which describes each user and what they are utilized for with links to related documentation.

YAML Object Format
------------------

The default name of the Secrets object for these users is ``my-cluster-secrets`` and can be set in the CR for your cluster in ``spec.secretName`` to something different.  When you create the object, it should match the following simple format.

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

The example above matches what is shipped in ``deploy/secrets.yaml`` which contains default passwords. You should NOT use these in production, but they are present to assist in automated testing or simple use in a development environment.

As you can see, because we use the ``data`` type in the Secrets object, all values for each key/value pair must be encoded in base64.  To do this you can simply run ``echo -n "password" | base64`` in your local shell to get valid values.

Password Rotation Policies and Timing
-------------------------------------

As above with application users, when a change is detected, the Operator creates the necessary transaction to change passwords.  This rotation happens instantly, and it's not needed to take any action beyond changing the password.
