Configuring ProxySQL
======================

You can use `ProxySQL admin interface <https://www.percona.com/blog/2017/06/07/proxysql-admin-interface-not-typical-mysql-server/>`_ to  configure its settings.

Configuring ProxySQL in this way means connecting to it using the MySQL
protocol, and two things are needed to do it:

* the ProxySQL Pod name
* the ProxySQL admin password

You can find out ProxySQL Pod name with the ``kubectl get pods`` command,
which will have the following output::

  $ kubectl get pods
  NAME                                              READY   STATUS    RESTARTS   AGE
  cluster1-pxc-node-0                               1/1     Running   0          5m
  cluster1-pxc-node-1                               1/1     Running   0          4m
  cluster1-pxc-node-2                               1/1     Running   0          2m
  cluster1-pxc-proxysql-0                           1/1     Running   0          5m
  percona-xtradb-cluster-operator-dc67778fd-qtspz   1/1     Running   0          6m

The next command will print you the needed admin password::

  kubectl get secrets $(kubectl get pxc -o yaml | grep secretsName: | awk '{print$2}' | xargs echo) -o yaml | grep proxyadmin: | awk '{print$2}' | base64 -D

When both Pod name and admin password are known, connect to the ProxySQL as
follows, substituting ``cluster1-pxc-proxysql-0`` with the actual Pod name and
``admin_password`` with the actual password::

  kubectl exec -it cluster1-pxc-proxysql-0 -- mysql -h127.0.0.1 -P6032 -uproxyadmin -padmin_password

.



