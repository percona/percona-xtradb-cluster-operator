.. _faq-env:

Define environment variables
============================

Sometimes you need to define new environment variables to provide additional
configuration for the components of your cluster. For example, you can use it to
customize the configuration of HAProxy, or to add additional options for PMM
Client.

The Operator can store environment variables in `Kubernetes Secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_. Here is an example with several HAProxy options:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: my-env-var-secrets
   type: Opaque
   data:
     HA_CONNECTION_TIMEOUT: MTAwMA==
     OK_IF_DONOR: MQ==
     HA_SERVER_OPTIONS: Y2hlY2sgaW50ZXIgMzAwMDAgcmlzZSAxIGZhbGwgNSB3ZWlnaHQgMQ==

As you can see, environment variables are stored as ``data`` - i.e.,
base64-encoded strings, so you'll need to encode the value of each variable.
For example, To have ``HA_CONNECTION_TIMEOUT`` variable equal to ``1000``, you
can run ``echo -n "1000" | base64 --wrap=0`` in your local shell and get ``MTAwMA==``.

.. note:: Similarly, you can read the list of options from a Base64-encoded
   string:

   .. code:: bash

      $ echo "MTAwMA==" | base64 --decode

When ready, apply the YAML file with the following command:

.. code:: bash

   $ kubectl create -f deploy/my-env-secret.yaml

Put the name of this Secret to the ``envVarsSecret`` key either in ``pxc``,
``haproxy`` or ``proxysql`` section of the `deploy/cr.yaml`` configuration file:

.. code:: yaml

     haproxy:
       ....
       envVarsSecret: my-env-var-secrets
       ....

Now apply the ``deploy/cr.yaml`` file with the following command:

.. code:: bash

   $ kubectl apply -f deploy/cr.yaml

.. _faq-allocator:

Another example shows how to pass ``LD_PRELOAD`` environment variable with the
alternative memory allocator library name to mysqld. It's often a recommended
practice to try using an alternative allocator library for mysqld in case the
memory usage is suspected to be higher than expected, and you can use jemalloc
allocator already present in Percona XtraDB Cluster Pods with the following
environment variable:

.. code:: bash

   LD_PRELOAD=/usr/lib64/libjemalloc.so.1

Create a new YAML file with the contents similar to the previous example, but
with ``LD_PRELOAD`` variable, stored as base64-encoded strings:

.. code:: yaml

   apiVersion: v1
   kind: Secret
   metadata: 
     name: my-new-env-var-secrets
   type: Opaque
   data: 
     LD_PRELOAD: L3Vzci9saWI2NC9saWJqZW1hbGxvYy5zby4x

If this YAML file was named ``deploy/my-new-env-var-secret``, the command
to apply it will be the following one:

.. code:: bash

   $ kubectl create -f deploy/my-new-env-secret.yaml

Now put the name of this new Secret to the ``envVarsSecret`` key in ``pxc``
section of the `deploy/cr.yaml`` configuration file:

.. code:: yaml

     pxc:
       ....
       envVarsSecret: my-new-env-var-secrets
       ....

Don't forget to apply the ``deploy/cr.yaml`` file, as usual:

.. code:: bash

   $ kubectl apply -f deploy/cr.yaml
