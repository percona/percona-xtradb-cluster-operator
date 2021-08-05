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
For example, To have ``HA_CONNECTION_TIMEOUT`` variable equal to ``100``, you
can run ``echo -n "100" | base64`` in your local shell and get ``MTAwMA==``.

.. note:: Similarly, you can read the list of options from a Base64-encoded
   string:

   .. code:: bash

      $ echo "MTAwMAo=" | base64 --decode

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

     kubectl apply -f deploy/cr.yaml

