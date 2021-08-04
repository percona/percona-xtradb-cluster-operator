.. _faq-env:

How to define environment variables via Custom Resource
========================================================

Sometimes you need to define new environment variables to provide additional
configuration for the components of your cluster. For example, this may help you
to customize the configuration of HAProxy. Also you can follow this way to add
some additional options for PMM Client.

The Operator can also store environment variables in `Kubernetes Secrets <https://kubernetes.io/docs/concepts/configuration/secret/>`_. Environment variables should be placed in 
Here is an example of such Secret with few HAProxy
options:

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

You can get a Base64 encoded string from your options via the command line as
follows:

.. code:: bash

   $ echo "1000" | base64

.. note:: Similarly, you can read the list of options from a Base64 encoded
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

