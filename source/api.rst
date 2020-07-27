PXC API Documentation
=====================

Prerequisites
-------------

1. prepare

   .. code-block:: yaml

      # set correct API adress
      KUBE_CLUSTER=$(kubectl config view --minify -o jsonpath='{.clusters[0].name}')
      API_SERVER=$(kubectl config view -o jsonpath="{.clusters[?(@.name==\"$KUBE_CLUSTER\")].cluster.server}")

      # create service account and get token
      kubectl apply -f deploy/crd.yaml -f deploy/rbac.yaml
      KUBE_TOKEN=$(kubectl get secret $(kubectl get serviceaccount percona-xtradb-cluster-operator -o jsonpath='{.secrets[0].name}') -o jsonpath='{.data.token}' | base64 --decode )

2. get a list of PXC clusters

   .. code-block:: bash

      # run "kubectl --v=9 get pxc" command find curl command in the output
      curl -k -XGET  -H "Authorization: Bearer $KUBE_TOKEN" "$APISERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters?limit=500" | python -mjson.tool

.. raw:: html <div style="page-break-after: always;"></div>

Create new PXC cluster
----------------------

**Description:**

.. code-block:: bash

   The command to create a new PXC cluster creating all of its resources and it depends on the pxc operator

**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 apply -f percona-xtradb-cluster-operator/deploy/cr.yaml

**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1-4-0/namespaces/default/perconaxtradbclusters

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN


**cURL Request:**

.. code-block:: bash

   curl -k -v -XPOST 'https://$API_SERVER/apis/pxc.percona.com/v1-4-0/namespaces/default/perconaxtradbclusters' \
               -H "Content-Type: application/json" \
               -H "Accept: application/json" \
               -H "Authorization: Bearer $KUBE_TOKEN" \
               -d "@cluster.json"

.. raw:: html

	<details>
	<summary> Request Body (cluster.json) : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1-4-0",
	   "kind":"PerconaXtraDBCluster",
	   "metadata":{
	      "name":"cluster1",
	      "finalizers":[
		 "delete-pxc-pods-in-order"
	      ]
	   },
	   "spec":{
	      "secretsName":"my-cluster-secrets",
	      "vaultSecretName":"keyring-secret-vault",
	      "sslSecretName":"my-cluster-ssl",
	      "sslInternalSecretName":"my-cluster-ssl-internal",
	      "allowUnsafeConfigurations":true,
	      "pxc":{
		 "size":3,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0",
		 "resources":{
		    "requests":null
		 },
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"6Gi"
		          }
		       }
		    }
		 },
		 "gracePeriod":600
	      },
	      "proxysql":{
		 "enabled":true,
		 "size":3,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-proxysql",
		 "resources":{
		    "requests":null
		 },
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"2Gi"
		          }
		       }
		    }
		 },
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "gracePeriod":30
	      },
	      "pmm":{
		 "enabled":false,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pmm",
		 "serverHost":"monitoring-service",
		 "serverUser":"pmm"
	      },
	      "backup":{
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup",
		 "serviceAccountName":"percona-xtradb-cluster-operator",
		 "storages":{
		    "s3-us-west":{
		       "type":"s3",
		       "s3":{
		          "bucket":"S3-BACKUP-BUCKET-NAME-HERE",
		          "credentialsSecret":"my-cluster-name-backup-s3",
		          "region":"us-west-2"
		       }
		    },
		    "fs-pvc":{
		       "type":"filesystem",
		       "volume":{
		          "persistentVolumeClaim":{
		             "accessModes":[
		                "ReadWriteOnce"
		             ],
		             "resources":{
		                "requests":{
		                   "storage":"6Gi"
		                }
		             }
		          }
		       }
		    }
		 },
		 "schedule":[
		    {
		       "name":"sat-night-backup",
		       "schedule":"0 0 * * 6",
		       "keep":3,
		       "storageName":"s3-us-west"
		    },
		    {
		       "name":"daily-backup",
		       "schedule":"0 0 * * *",
		       "keep":5,
		       "storageName":"fs-pvc"
		    }
		 ]
	      }
	   }
	}
	</pre></div></div>

	</details>

Inputs:

  Metadata:
  
  1. Name (String, min-length: 1) : ``contains name of cluster``
  2. Finalizers (list of string, Default: [ “delete-pxc-pods-in-order” ]) ``contains steps to do when deleting the cluster ``
  
Spec:

  1. secretsName (String, min-length: 1) : ``contains name of secret to create for the cluster``
  2. vaultSecretName (String, min-length: 1) : ``contains name of vault secret to create for the cluster``
  3. sslInternalSecretName (String, min-length: 1) : ``contains name of ssl secret to create for the cluster``
  4. allowUnsafeConfigurations (Boolean, Default: false) : ``allow unsafe configurations to run``

  pxc:
    1. Size (Int , min-value: 1, default, 3) : ``number of pxc nodes to create``
    2. Image (String, min-length: 1) : ``contains image name to use for pxc nodes``
    3. volumeSpec : storage (SizeString, default: “6Gi”) : ``contains the size for the storage volume of pxc nodes``
    4. gracePeriod (Int, default: 600, min-value: 0 ) : ``contains the time to wait for pxc node to shutdown in milliseconds``

  proxysql:
  
    1. Enabled (Boolean, default: true) : ``enabled or disables proxysql``

  pmm:
  
    1. serverHost (String, min-length: 1) : ``serivce name for monitoring``
    2. serverUser (String, min-length: 1) : ``name of pmm user``
    3. image (String, min-length: 1) : ``name of pmm image``
    
  backup:
  
    1. Storages (Object) : ``contains the storage destinations to save the backups in``
    2. schedule:
       1. name (String, min-length: 1) : ``name of backup job``
       2. schedule (String, Cron format: "* * * * *") : ``contains cron schedule format for when to run cron jobs``
       3. keep (Int, min-value = 1) : ``number of backups to keep``
       4. storageName (String, min-length: 1) : ``name of storage object to use``

.. raw:: html

	<details>
	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1-4-0",
	   "kind":"PerconaXtraDBCluster",
	   "metadata":{
	      "creationTimestamp":"2020-05-27T22:23:58Z",
	      "finalizers":[
		 "delete-pxc-pods-in-order"
	      ],
	      "generation":1,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1-4-0",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:metadata":{
		          "f:finalizers":{

		          }
		       },
		       "f:spec":{
		          ".":{

		          },
		          "f:allowUnsafeConfigurations":{

		          },
		          "f:backup":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:schedule":{

		             },
		             "f:serviceAccountName":{

		             },
		             "f:storages":{
		                ".":{

		                },
		                "f:fs-pvc":{
		                   ".":{

		                   },
		                   "f:type":{

		                   },
		                   "f:volume":{
		                      ".":{

		                      },
		                      "f:persistentVolumeClaim":{
		                         ".":{

		                         },
		                         "f:accessModes":{

		                         },
		                         "f:resources":{
		                            ".":{

		                            },
		                            "f:requests":{
		                               ".":{

		                               },
		                               "f:storage":{

		                               }
		                            }
		                         }
		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   ".":{

		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      },
		                      "f:region":{

		                      }
		                   },
		                   "f:type":{

		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             ".":{

		             },
		             "f:enabled":{

		             },
		             "f:image":{

		             },
		             "f:serverHost":{

		             },
		             "f:serverUser":{

		             }
		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:enabled":{

		             },
		             "f:gracePeriod":{

		             },
		             "f:image":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{
		                ".":{

		                },
		                "f:requests":{

		                }
		             },
		             "f:size":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:gracePeriod":{

		             },
		             "f:image":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{
		                ".":{

		                },
		                "f:requests":{

		                }
		             },
		             "f:size":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:secretsName":{

		          },
		          "f:sslInternalSecretName":{

		          },
		          "f:sslSecretName":{

		          },
		          "f:vaultSecretName":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-05-27T22:23:58Z"
		 }
	      ],
	      "name":"cluster1",
	      "namespace":"default",
	      "resourceVersion":"8694",
	      "selfLink":"/apis/pxc.percona.com/v1-4-0/namespaces/default/perconaxtradbclusters/cluster1",
	      "uid":"e9115e2a-49df-4ebf-9dab-fa5a550208d3"
	   },
	   "spec":{
	      "allowUnsafeConfigurations":false,
	      "backup":{
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup",
		 "schedule":[
		    {
		       "keep":3,
		       "name":"sat-night-backup",
		       "schedule":"0 0 * * 6",
		       "storageName":"s3-us-west"
		    },
		    {
		       "keep":5,
		       "name":"daily-backup",
		       "schedule":"0 0 * * *",
		       "storageName":"fs-pvc"
		    }
		 ],
		 "serviceAccountName":"percona-xtradb-cluster-operator",
		 "storages":{
		    "fs-pvc":{
		       "type":"filesystem",
		       "volume":{
		          "persistentVolumeClaim":{
		             "accessModes":[
		                "ReadWriteOnce"
		             ],
		             "resources":{
		                "requests":{
		                   "storage":"6Gi"
		                }
		             }
		          }
		       }
		    },
		    "s3-us-west":{
		       "s3":{
		          "bucket":"S3-BACKUP-BUCKET-NAME-HERE",
		          "credentialsSecret":"my-cluster-name-backup-s3",
		          "region":"us-west-2"
		       },
		       "type":"s3"
		    }
		 }
	      },
	      "pmm":{
		 "enabled":false,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pmm",
		 "serverHost":"monitoring-service",
		 "serverUser":"pmm"
	      },
	      "proxysql":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "enabled":true,
		 "gracePeriod":30,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-proxysql",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"2Gi"
		          }
		       }
		    }
		 }
	      },
	      "pxc":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "gracePeriod":600,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"6Gi"
		          }
		       }
		    }
		 }
	      },
	      "secretsName":"my-cluster-secrets",
	      "sslInternalSecretName":"my-cluster-ssl-internal",
	      "sslSecretName":"my-cluster-ssl",
	      "vaultSecretName":"keyring-secret-vault"
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

List PXC cluster
----------------

**Describtion :**

.. code-block:: bash

   Lists all pxc clusters that exist in your kubernetes cluster

**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 get pxc

**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters?limit=500

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN

cURL Request :

.. code-block:: bash

   curl -k -v -XGET 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters?limit=500' \
               -H "Accept: application/json;as=Table;v=v1;g=meta.k8s.io,application/json;as=Table;v=v1beta1;g=meta.k8s.io,application/json" \
               -H "Authorization: Bearer $KUBE_TOKEN"

**Request Body:**

.. code-block:: bash

   None

.. raw:: html

	<details>
	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "kind":"Table",
	   "apiVersion":"meta.k8s.io/v1",
	   "metadata":{
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters",
	      "resourceVersion":"10528"
	   },
	   "columnDefinitions":[
	      {
		 "name":"Name",
		 "type":"string",
		 "format":"name",
		 "description":"Name must be unique within a namespace. Is required when creating resources, although some resources may allow a client to request the generation of an appropriate name automatically. Name is primarily intended for creation idempotence and configuration definition. Cannot be updated. More info: http://kubernetes.io/docs/user-guide/identifiers#names",
		 "priority":0
	      },
	      {
		 "name":"Endpoint",
		 "type":"string",
		 "format":"",
		 "description":"Custom resource definition column (in JSONPath format): .status.host",
		 "priority":0
	      },
	      {
		 "name":"Status",
		 "type":"string",
		 "format":"",
		 "description":"Custom resource definition column (in JSONPath format): .status.state",
		 "priority":0
	      },
	      {
		 "name":"PXC",
		 "type":"string",
		 "format":"",
		 "description":"Ready pxc nodes",
		 "priority":0
	      },
	      {
		 "name":"proxysql",
		 "type":"string",
		 "format":"",
		 "description":"Ready pxc nodes",
		 "priority":0
	      },
	      {
		 "name":"Age",
		 "type":"date",
		 "format":"",
		 "description":"Custom resource definition column (in JSONPath format): .metadata.creationTimestamp",
		 "priority":0
	      }
	   ],
	   "rows":[
	      {
		 "cells":[
		    "cluster1",
		    "cluster1-proxysql.default",
		    "ready",
		    "3",
		    "3",
		    "8m37s"
		 ],
		 "object":{
		    "kind":"PartialObjectMetadata",
		    "apiVersion":"meta.k8s.io/v1",
		    "metadata":{
		       "name":"cluster1",
		       "namespace":"default",
		       "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1",
		       "uid":"e9115e2a-49df-4ebf-9dab-fa5a550208d3",
		       "resourceVersion":"10517",
		       "generation":1,
		       "creationTimestamp":"2020-05-27T22:23:58Z",
		       "finalizers":[
		          "delete-pxc-pods-in-order"
		       ],
		       "managedFields":[
		          {
		             "manager":"kubectl",
		             "operation":"Update",
		             "apiVersion":"pxc.percona.com/v1-4-0",
		             "time":"2020-05-27T22:23:58Z",
		             "fieldsType":"FieldsV1",
		             "fieldsV1":{
		                "f:metadata":{
		                   "f:finalizers":{

		                   }
		                },
		                "f:spec":{
		                   ".":{

		                   },
		                   "f:allowUnsafeConfigurations":{

		                   },
		                   "f:backup":{
		                      ".":{

		                      },
		                      "f:image":{

		                      },
		                      "f:schedule":{

		                      },
		                      "f:serviceAccountName":{

		                      },
		                      "f:storages":{
		                         ".":{

		                         },
		                         "f:fs-pvc":{
		                            ".":{

		                            },
		                            "f:type":{

		                            },
		                            "f:volume":{
		                               ".":{

		                               },
		                               "f:persistentVolumeClaim":{
		                                  ".":{

		                                  },
		                                  "f:accessModes":{

		                                  },
		                                  "f:resources":{
		                                     ".":{

		                                     },
		                                     "f:requests":{
		                                        ".":{

		                                        },
		                                        "f:storage":{

		                                        }
		                                     }
		                                  }
		                               }
		                            }
		                         },
		                         "f:s3-us-west":{
		                            ".":{

		                            },
		                            "f:s3":{
		                               ".":{

		                               },
		                               "f:bucket":{

		                               },
		                               "f:credentialsSecret":{

		                               },
		                               "f:region":{

		                               }
		                            },
		                            "f:type":{

		                            }
		                         }
		                      }
		                   },
		                   "f:pmm":{
		                      ".":{

		                      },
		                      "f:image":{

		                      },
		                      "f:serverHost":{

		                      },
		                      "f:serverUser":{

		                      }
		                   },
		                   "f:proxysql":{
		                      ".":{

		                      },
		                      "f:affinity":{
		                         ".":{

		                         },
		                         "f:antiAffinityTopologyKey":{

		                         }
		                      },
		                      "f:enabled":{

		                      },
		                      "f:gracePeriod":{

		                      },
		                      "f:image":{

		                      },
		                      "f:podDisruptionBudget":{
		                         ".":{

		                         },
		                         "f:maxUnavailable":{

		                         }
		                      },
		                      "f:resources":{

		                      },
		                      "f:size":{

		                      },
		                      "f:volumeSpec":{
		                         ".":{

		                         },
		                         "f:persistentVolumeClaim":{
		                            ".":{

		                            },
		                            "f:resources":{
		                               ".":{

		                               },
		                               "f:requests":{
		                                  ".":{

		                                  },
		                                  "f:storage":{

		                                  }
		                               }
		                            }
		                         }
		                      }
		                   },
		                   "f:pxc":{
		                      ".":{

		                      },
		                      "f:affinity":{
		                         ".":{

		                         },
		                         "f:antiAffinityTopologyKey":{

		                         }
		                      },
		                      "f:gracePeriod":{

		                      },
		                      "f:image":{

		                      },
		                      "f:podDisruptionBudget":{
		                         ".":{

		                         },
		                         "f:maxUnavailable":{

		                         }
		                      },
		                      "f:resources":{

		                      },
		                      "f:size":{

		                      },
		                      "f:volumeSpec":{
		                         ".":{

		                         },
		                         "f:persistentVolumeClaim":{
		                            ".":{

		                            },
		                            "f:resources":{
		                               ".":{

		                               },
		                               "f:requests":{
		                                  ".":{

		                                  },
		                                  "f:storage":{

		                                  }
		                               }
		                            }
		                         }
		                      }
		                   },
		                   "f:secretsName":{

		                   },
		                   "f:sslInternalSecretName":{

		                   },
		                   "f:sslSecretName":{

		                   },
		                   "f:vaultSecretName":{

		                   }
		                }
		             }
		          },
		          {
		             "manager":"percona-xtradb-cluster-operator",
		             "operation":"Update",
		             "apiVersion":"pxc.percona.com/v1",
		             "time":"2020-05-27T22:32:31Z",
		             "fieldsType":"FieldsV1",
		             "fieldsV1":{
		                "f:spec":{
		                   "f:backup":{
		                      "f:storages":{
		                         "f:fs-pvc":{
		                            "f:podSecurityContext":{
		                               ".":{

		                               },
		                               "f:fsGroup":{

		                               },
		                               "f:supplementalGroups":{

		                               }
		                            },
		                            "f:s3":{
		                               ".":{

		                               },
		                               "f:bucket":{

		                               },
		                               "f:credentialsSecret":{

		                               }
		                            }
		                         },
		                         "f:s3-us-west":{
		                            "f:podSecurityContext":{
		                               ".":{

		                               },
		                               "f:fsGroup":{

		                               },
		                               "f:supplementalGroups":{

		                               }
		                            }
		                         }
		                      }
		                   },
		                   "f:pmm":{
		                      "f:resources":{

		                      }
		                   },
		                   "f:proxysql":{
		                      "f:podSecurityContext":{
		                         ".":{

		                         },
		                         "f:fsGroup":{

		                         },
		                         "f:supplementalGroups":{

		                         }
		                      },
		                      "f:sslInternalSecretName":{

		                      },
		                      "f:sslSecretName":{

		                      },
		                      "f:volumeSpec":{
		                         "f:persistentVolumeClaim":{
		                            "f:accessModes":{

		                            }
		                         }
		                      }
		                   },
		                   "f:pxc":{
		                      "f:podSecurityContext":{
		                         ".":{

		                         },
		                         "f:fsGroup":{

		                         },
		                         "f:supplementalGroups":{

		                         }
		                      },
		                      "f:sslInternalSecretName":{

		                      },
		                      "f:sslSecretName":{

		                      },
		                      "f:vaultSecretName":{

		                      },
		                      "f:volumeSpec":{
		                         "f:persistentVolumeClaim":{
		                            "f:accessModes":{

		                            }
		                         }
		                      }
		                   }
		                },
		                "f:status":{
		                   ".":{

		                   },
		                   "f:conditions":{

		                   },
		                   "f:host":{

		                   },
		                   "f:observedGeneration":{

		                   },
		                   "f:proxysql":{
		                      ".":{

		                      },
		                      "f:ready":{

		                      },
		                      "f:size":{

		                      },
		                      "f:status":{

		                      }
		                   },
		                   "f:pxc":{
		                      ".":{

		                      },
		                      "f:ready":{

		                      },
		                      "f:size":{

		                      },
		                      "f:status":{

		                      }
		                   },
		                   "f:state":{

		                   }
		                }
		             }
		          }
		       ]
		    }
		 }
	      }
	   ]
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Get status of PXC cluster
-------------------------

**Describtion:**

.. code-block:: bash

   Gets all information about specified pxc cluster

**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 get pxc/cluster1 -o json

**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN

**cURL Request:**

.. code-block:: bash

   curl -k -v -XGET 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1' \
               -H "Accept: application/json" \
               -H "Authorization: Bearer $KUBE_TOKEN"

**Request Body:**

.. code-block:: bash

   None

.. raw:: html

	<details>
	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBCluster",
	   "metadata":{
	      "annotations":{
		 "kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"pxc.percona.com/v1\",\"kind\":\"PerconaXtraDBCluster\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":\"2020-05-27T22:23:58Z\",\"finalizers\":[\"delete-pxc-pods-in-order\"],\"generation\":1,\"managedFields\":[{\"apiVersion\":\"pxc.percona.com/v1-4-0\",\"fieldsType\":\"FieldsV1\",\"fieldsV1\":{\"f:metadata\":{\"f:finalizers\":{}},\"f:spec\":{\".\":{},\"f:allowUnsafeConfigurations\":{},\"f:backup\":{\".\":{},\"f:image\":{},\"f:schedule\":{},\"f:serviceAccountName\":{},\"f:storages\":{\".\":{},\"f:fs-pvc\":{\".\":{},\"f:type\":{},\"f:volume\":{\".\":{},\"f:persistentVolumeClaim\":{\".\":{},\"f:accessModes\":{},\"f:resources\":{\".\":{},\"f:requests\":{\".\":{},\"f:storage\":{}}}}}},\"f:s3-us-west\":{\".\":{},\"f:s3\":{\".\":{},\"f:bucket\":{},\"f:credentialsSecret\":{},\"f:region\":{}},\"f:type\":{}}}},\"f:pmm\":{\".\":{},\"f:image\":{},\"f:serverHost\":{},\"f:serverUser\":{}},\"f:proxysql\":{\".\":{},\"f:affinity\":{\".\":{},\"f:antiAffinityTopologyKey\":{}},\"f:enabled\":{},\"f:gracePeriod\":{},\"f:image\":{},\"f:podDisruptionBudget\":{\".\":{},\"f:maxUnavailable\":{}},\"f:resources\":{},\"f:size\":{},\"f:volumeSpec\":{\".\":{},\"f:persistentVolumeClaim\":{\".\":{},\"f:resources\":{\".\":{},\"f:requests\":{\".\":{},\"f:storage\":{}}}}}},\"f:pxc\":{\".\":{},\"f:affinity\":{\".\":{},\"f:antiAffinityTopologyKey\":{}},\"f:gracePeriod\":{},\"f:image\":{},\"f:podDisruptionBudget\":{\".\":{},\"f:maxUnavailable\":{}},\"f:resources\":{},\"f:size\":{},\"f:volumeSpec\":{\".\":{},\"f:persistentVolumeClaim\":{\".\":{},\"f:resources\":{\".\":{},\"f:requests\":{\".\":{},\"f:storage\":{}}}}}},\"f:secretsName\":{},\"f:sslInternalSecretName\":{},\"f:sslSecretName\":{},\"f:vaultSecretName\":{}}},\"manager\":\"kubectl\",\"operation\":\"Update\",\"time\":\"2020-05-27T22:23:58Z\"},{\"apiVersion\":\"pxc.percona.com/v1\",\"fieldsType\":\"FieldsV1\",\"fieldsV1\":{\"f:spec\":{\"f:backup\":{\"f:storages\":{\"f:fs-pvc\":{\"f:podSecurityContext\":{\".\":{},\"f:fsGroup\":{},\"f:supplementalGroups\":{}},\"f:s3\":{\".\":{},\"f:bucket\":{},\"f:credentialsSecret\":{}}},\"f:s3-us-west\":{\"f:podSecurityContext\":{\".\":{},\"f:fsGroup\":{},\"f:supplementalGroups\":{}}}}},\"f:pmm\":{\"f:resources\":{}},\"f:proxysql\":{\"f:podSecurityContext\":{\".\":{},\"f:fsGroup\":{},\"f:supplementalGroups\":{}},\"f:sslInternalSecretName\":{},\"f:sslSecretName\":{},\"f:volumeSpec\":{\"f:persistentVolumeClaim\":{\"f:accessModes\":{}}}},\"f:pxc\":{\"f:podSecurityContext\":{\".\":{},\"f:fsGroup\":{},\"f:supplementalGroups\":{}},\"f:sslInternalSecretName\":{},\"f:sslSecretName\":{},\"f:vaultSecretName\":{},\"f:volumeSpec\":{\"f:persistentVolumeClaim\":{\"f:accessModes\":{}}}}},\"f:status\":{\".\":{},\"f:conditions\":{},\"f:host\":{},\"f:observedGeneration\":{},\"f:proxysql\":{\".\":{},\"f:ready\":{},\"f:size\":{},\"f:status\":{}},\"f:pxc\":{\".\":{},\"f:ready\":{},\"f:size\":{},\"f:status\":{}},\"f:state\":{}}},\"manager\":\"percona-xtradb-cluster-operator\",\"operation\":\"Update\",\"time\":\"2020-05-27T23:06:47Z\"}],\"name\":\"cluster1\",\"namespace\":\"default\",\"resourceVersion\":\"15878\",\"selfLink\":\"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1\",\"uid\":\"e9115e2a-49df-4ebf-9dab-fa5a550208d3\"},\"spec\":{\"allowUnsafeConfigurations\":true,\"backup\":{\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-debug-backup\",\"schedule\":[{\"keep\":3,\"name\":\"sat-night-backup\",\"schedule\":\"0 0 * * 6\",\"storageName\":\"s3-us-west\"},{\"keep\":5,\"name\":\"daily-backup\",\"schedule\":\"0 0 * * *\",\"storageName\":\"fs-pvc\"}],\"serviceAccountName\":\"percona-xtradb-cluster-operator\",\"storages\":{\"fs-pvc\":{\"type\":\"filesystem\",\"volume\":{\"persistentVolumeClaim\":{\"accessModes\":[\"ReadWriteOnce\"],\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"s3-us-west\":{\"s3\":{\"bucket\":\"S3-BACKUP-BUCKET-NAME-HERE\",\"credentialsSecret\":\"my-cluster-name-backup-s3\",\"region\":\"us-west-2\"},\"type\":\"s3\"}}},\"pmm\":{\"enabled\":false,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pmm\",\"serverHost\":\"monitoring-service\",\"serverUser\":\"pmm\"},\"proxysql\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"enabled\":true,\"gracePeriod\":30,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-proxysql\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":5,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"2Gi\"}}}}},\"pxc\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"gracePeriod\":600,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":5,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"secretsName\":\"my-cluster-secrets\",\"sslInternalSecretName\":\"my-cluster-ssl-internal\",\"sslSecretName\":\"my-cluster-ssl\",\"vaultSecretName\":\"keyring-secret-vault\"},\"status\":{\"conditions\":[{\"lastTransitionTime\":\"2020-05-27T22:23:58Z\",\"status\":\"True\",\"type\":\"Initializing\"},{\"lastTransitionTime\":\"2020-05-27T22:25:43Z\",\"status\":\"True\",\"type\":\"Ready\"}],\"host\":\"cluster1-proxysql.default\",\"observedGeneration\":1,\"proxysql\":{\"ready\":3,\"size\":5,\"status\":\"ready\"},\"pxc\":{\"ready\":3,\"size\":5,\"status\":\"ready\"},\"state\":\"ready\"}}\n"
	      },
	      "creationTimestamp":"2020-05-27T22:23:58Z",
	      "finalizers":[
		 "delete-pxc-pods-in-order"
	      ],
	      "generation":6,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1-4-0",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:metadata":{
		          "f:finalizers":{

		          }
		       },
		       "f:spec":{
		          ".":{

		          },
		          "f:allowUnsafeConfigurations":{

		          },
		          "f:backup":{
		             ".":{

		             },
		             "f:schedule":{

		             },
		             "f:serviceAccountName":{

		             },
		             "f:storages":{
		                ".":{

		                },
		                "f:fs-pvc":{
		                   ".":{

		                   },
		                   "f:type":{

		                   },
		                   "f:volume":{
		                      ".":{

		                      },
		                      "f:persistentVolumeClaim":{
		                         ".":{

		                         },
		                         "f:accessModes":{

		                         },
		                         "f:resources":{
		                            ".":{

		                            },
		                            "f:requests":{
		                               ".":{

		                               },
		                               "f:storage":{

		                               }
		                            }
		                         }
		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   ".":{

		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      },
		                      "f:region":{

		                      }
		                   },
		                   "f:type":{

		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:serverHost":{

		             },
		             "f:serverUser":{

		             }
		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:enabled":{

		             },
		             "f:gracePeriod":{

		             },
		             "f:image":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:gracePeriod":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:secretsName":{

		          },
		          "f:sslInternalSecretName":{

		          },
		          "f:sslSecretName":{

		          },
		          "f:vaultSecretName":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-05-27T22:23:58Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:metadata":{
		          "f:annotations":{
		             ".":{

		             },
		             "f:kubectl.kubernetes.io/last-applied-configuration":{

		             }
		          }
		       },
		       "f:spec":{
		          "f:backup":{
		             "f:image":{

		             }
		          },
		          "f:proxysql":{
		             "f:size":{

		             }
		          },
		          "f:pxc":{
		             "f:image":{

		             },
		             "f:size":{

		             }
		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-05-27T23:38:49Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          "f:backup":{
		             "f:storages":{
		                "f:fs-pvc":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             "f:resources":{

		             }
		          },
		          "f:proxysql":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:vaultSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          }
		       },
		       "f:status":{
		          ".":{

		          },
		          "f:conditions":{

		          },
		          "f:host":{

		          },
		          "f:message":{

		          },
		          "f:observedGeneration":{

		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:message":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:state":{

		          }
		       }
		    },
		    "manager":"percona-xtradb-cluster-operator",
		    "operation":"Update",
		    "time":"2020-05-28T10:42:00Z"
		 }
	      ],
	      "name":"cluster1",
	      "namespace":"default",
	      "resourceVersion":"35660",
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1",
	      "uid":"e9115e2a-49df-4ebf-9dab-fa5a550208d3"
	   },
	   "spec":{
	      "allowUnsafeConfigurations":true,
	      "backup":{
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-debug-backup",
		 "schedule":[
		    {
		       "keep":3,
		       "name":"sat-night-backup",
		       "schedule":"0 0 * * 6",
		       "storageName":"s3-us-west"
		    },
		    {
		       "keep":5,
		       "name":"daily-backup",
		       "schedule":"0 0 * * *",
		       "storageName":"fs-pvc"
		    }
		 ],
		 "serviceAccountName":"percona-xtradb-cluster-operator",
		 "storages":{
		    "fs-pvc":{
		       "type":"filesystem",
		       "volume":{
		          "persistentVolumeClaim":{
		             "accessModes":[
		                "ReadWriteOnce"
		             ],
		             "resources":{
		                "requests":{
		                   "storage":"6Gi"
		                }
		             }
		          }
		       }
		    },
		    "s3-us-west":{
		       "s3":{
		          "bucket":"S3-BACKUP-BUCKET-NAME-HERE",
		          "credentialsSecret":"my-cluster-name-backup-s3",
		          "region":"us-west-2"
		       },
		       "type":"s3"
		    }
		 }
	      },
	      "pmm":{
		 "enabled":false,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pmm",
		 "serverHost":"monitoring-service",
		 "serverUser":"pmm"
	      },
	      "proxysql":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "enabled":true,
		 "gracePeriod":30,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-proxysql",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{

		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"2Gi"
		          }
		       }
		    }
		 }
	      },
	      "pxc":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "gracePeriod":600,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-debug",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{

		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"6Gi"
		          }
		       }
		    }
		 }
	      },
	      "secretsName":"my-cluster-secrets",
	      "sslInternalSecretName":"my-cluster-ssl-internal",
	      "sslSecretName":"my-cluster-ssl",
	      "vaultSecretName":"keyring-secret-vault"
	   },
	   "status":{
	      "conditions":[
		 {
		    "lastTransitionTime":"2020-05-27T22:25:43Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:06:48Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:08:58Z",
		    "message":"ProxySQL upgrade error: context deadline exceeded",
		    "reason":"ErrorReconcile",
		    "status":"True",
		    "type":"Error"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:08:59Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:29:59Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:30:04Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:35:27Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:35:42Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:47:00Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-27T23:47:05Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T09:58:25Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T09:58:31Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:03:54Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:04:14Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:15:28Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:15:38Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:26:56Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:27:01Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:38:28Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-05-28T10:38:33Z",
		    "status":"True",
		    "type":"Initializing"
		 }
	      ],
	      "host":"cluster1-proxysql.default",
	      "message":[
		 "PXC: pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-1_default(5b9b16e6-d0f8-4c97-a2d0-294feb9d014b); pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-2_default(b8ebedd7-42f0-440b-aa5e-509d28926a5e); pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-4_default(2dce12f2-9ebc-419c-a92a-9cec68912004); "
	      ],
	      "observedGeneration":6,
	      "proxysql":{
		 "ready":3,
		 "size":3,
		 "status":"ready"
	      },
	      "pxc":{
		 "message":"pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-1_default(5b9b16e6-d0f8-4c97-a2d0-294feb9d014b); pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-2_default(b8ebedd7-42f0-440b-aa5e-509d28926a5e); pxc: back-off 5m0s restarting failed container=pxc pod=cluster1-pxc-4_default(2dce12f2-9ebc-419c-a92a-9cec68912004); ",
		 "ready":2,
		 "size":3,
		 "status":"initializing"
	      },
	      "state":"initializing"
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Scale up/down PXC cluster
-------------------------

**Describtion:**

.. code-block:: bash

   Increase or decrease the size of the pxc cluster nodes to fit the current high availability needs

**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 patch pxc cluster1 --type=merge --patch '{
   "spec": {"pxc":{ "size": "5" }  
   }}'

**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN

**cURL Request:**

.. code-block:: bash

   curl -k -v -XPATCH 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1' \
               -H "Authorization: Bearer $KUBE_TOKEN" \
               -H "Content-Type: application/merge-patch+json" 
               -H "Accept: application/json" \
               -d '{  
                     "spec": {"pxc":{ "size": "5" }  
                     }}'

.. raw:: html

	<details>
	<summary> Request Body : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{  
	"spec": {"pxc":{ "size": "5" }  
	}}
	</pre></div></div>

	</details>

Input:
******

spec:

   pxc

   1. size (Int or String, Defaults: 3) : ``Specifiy the sie of the pxc cluster to scale up or down to``

.. raw:: html

	<details>
	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBCluster",
	   "metadata":{
	      "annotations":{
		 "kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"pxc.percona.com/v1-4-0\",\"kind\":\"PerconaXtraDBCluster\",\"metadata\":{\"annotations\":{},\"finalizers\":[\"delete-pxc-pods-in-order\"],\"name\":\"cluster1\",\"namespace\":\"default\"},\"spec\":{\"allowUnsafeConfigurations\":true,\"backup\":{\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup\",\"schedule\":[{\"keep\":3,\"name\":\"sat-night-backup\",\"schedule\":\"0 0 * * 6\",\"storageName\":\"s3-us-west\"},{\"keep\":5,\"name\":\"daily-backup\",\"schedule\":\"0 0 * * *\",\"storageName\":\"fs-pvc\"}],\"serviceAccountName\":\"percona-xtradb-cluster-operator\",\"storages\":{\"fs-pvc\":{\"type\":\"filesystem\",\"volume\":{\"persistentVolumeClaim\":{\"accessModes\":[\"ReadWriteOnce\"],\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"s3-us-west\":{\"s3\":{\"bucket\":\"S3-BACKUP-BUCKET-NAME-HERE\",\"credentialsSecret\":\"my-cluster-name-backup-s3\",\"region\":\"us-west-2\"},\"type\":\"s3\"}}},\"pmm\":{\"enabled\":false,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pmm\",\"serverHost\":\"monitoring-service\",\"serverUser\":\"pmm\"},\"proxysql\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"enabled\":true,\"gracePeriod\":30,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-proxysql\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":3,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"2Gi\"}}}}},\"pxc\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"gracePeriod\":600,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":3,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"secretsName\":\"my-cluster-secrets\",\"sslInternalSecretName\":\"my-cluster-ssl-internal\",\"sslSecretName\":\"my-cluster-ssl\",\"updateStrategy\":\"RollingUpdate\",\"vaultSecretName\":\"keyring-secret-vault\"}}\n"
	      },
	      "creationTimestamp":"2020-06-01T16:50:05Z",
	      "finalizers":[
		 "delete-pxc-pods-in-order"
	      ],
	      "generation":4,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1-4-0",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:metadata":{
		          "f:annotations":{
		             ".":{

		             },
		             "f:kubectl.kubernetes.io/last-applied-configuration":{

		             }
		          },
		          "f:finalizers":{

		          }
		       },
		       "f:spec":{
		          ".":{

		          },
		          "f:allowUnsafeConfigurations":{

		          },
		          "f:backup":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:schedule":{

		             },
		             "f:serviceAccountName":{

		             },
		             "f:storages":{
		                ".":{

		                },
		                "f:fs-pvc":{
		                   ".":{

		                   },
		                   "f:type":{

		                   },
		                   "f:volume":{
		                      ".":{

		                      },
		                      "f:persistentVolumeClaim":{
		                         ".":{

		                         },
		                         "f:accessModes":{

		                         },
		                         "f:resources":{
		                            ".":{

		                            },
		                            "f:requests":{
		                               ".":{

		                               },
		                               "f:storage":{

		                               }
		                            }
		                         }
		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   ".":{

		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      },
		                      "f:region":{

		                      }
		                   },
		                   "f:type":{

		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:serverHost":{

		             },
		             "f:serverUser":{

		             }
		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:enabled":{

		             },
		             "f:gracePeriod":{

		             },
		             "f:image":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:size":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:gracePeriod":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:secretsName":{

		          },
		          "f:sslInternalSecretName":{

		          },
		          "f:sslSecretName":{

		          },
		          "f:updateStrategy":{

		          },
		          "f:vaultSecretName":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-06-01T16:52:30Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          "f:backup":{
		             "f:storages":{
		                "f:fs-pvc":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             "f:resources":{

		             }
		          },
		          "f:proxysql":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:vaultSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          }
		       },
		       "f:status":{
		          ".":{

		          },
		          "f:conditions":{

		          },
		          "f:host":{

		          },
		          "f:observedGeneration":{

		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:state":{

		          }
		       }
		    },
		    "manager":"percona-xtradb-cluster-operator",
		    "operation":"Update",
		    "time":"2020-06-03T15:32:11Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          "f:pxc":{
		             "f:image":{

		             },
		             "f:size":{

		             }
		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-06-03T15:32:14Z"
		 }
	      ],
	      "name":"cluster1",
	      "namespace":"default",
	      "resourceVersion":"129605",
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1",
	      "uid":"15e5e7d6-10b2-46cf-85d0-d3fdea3412ca"
	   },
	   "spec":{
	      "allowUnsafeConfigurations":true,
	      "backup":{
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup",
		 "schedule":[
		    {
		       "keep":3,
		       "name":"sat-night-backup",
		       "schedule":"0 0 * * 6",
		       "storageName":"s3-us-west"
		    },
		    {
		       "keep":5,
		       "name":"daily-backup",
		       "schedule":"0 0 * * *",
		       "storageName":"fs-pvc"
		    }
		 ],
		 "serviceAccountName":"percona-xtradb-cluster-operator",
		 "storages":{
		    "fs-pvc":{
		       "type":"filesystem",
		       "volume":{
		          "persistentVolumeClaim":{
		             "accessModes":[
		                "ReadWriteOnce"
		             ],
		             "resources":{
		                "requests":{
		                   "storage":"6Gi"
		                }
		             }
		          }
		       }
		    },
		    "s3-us-west":{
		       "s3":{
		          "bucket":"S3-BACKUP-BUCKET-NAME-HERE",
		          "credentialsSecret":"my-cluster-name-backup-s3",
		          "region":"us-west-2"
		       },
		       "type":"s3"
		    }
		 }
	      },
	      "pmm":{
		 "enabled":false,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pmm",
		 "serverHost":"monitoring-service",
		 "serverUser":"pmm"
	      },
	      "proxysql":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "enabled":true,
		 "gracePeriod":30,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-proxysql",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"2Gi"
		          }
		       }
		    }
		 }
	      },
	      "pxc":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "gracePeriod":600,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc5.7",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":"5",
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"6Gi"
		          }
		       }
		    }
		 }
	      },
	      "secretsName":"my-cluster-secrets",
	      "sslInternalSecretName":"my-cluster-ssl-internal",
	      "sslSecretName":"my-cluster-ssl",
	      "updateStrategy":"RollingUpdate",
	      "vaultSecretName":"keyring-secret-vault"
	   },
	   "status":{
	      "conditions":[
		 {
		    "lastTransitionTime":"2020-06-01T16:50:37Z",
		    "message":"create newStatefulSetNode: StatefulSet.apps \"cluster1-pxc\" is invalid: spec.updateStrategy: Invalid value: apps.StatefulSetUpdateStrategy{Type:\"SmartUpdate\", RollingUpdate:(*apps.RollingUpdateStatefulSetStrategy)(nil)}: must be 'RollingUpdate' or 'OnDelete'",
		    "reason":"ErrorReconcile",
		    "status":"True",
		    "type":"Error"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T16:52:31Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T16:55:59Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T17:19:15Z",
		    "status":"True",
		    "type":"Initializing"
		 }
	      ],
	      "host":"cluster1-proxysql.default",
	      "observedGeneration":3,
	      "proxysql":{
		 "ready":3,
		 "size":3,
		 "status":"ready"
	      },
	      "pxc":{
		 "ready":1,
		 "size":3,
		 "status":"initializing"
	      },
	      "state":"initializing"
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Update PXC cluster image
------------------------

**Description:**

.. code-block:: bash

   Change the image of pxc containers inside the cluster

**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 patch pxc cluster1 --type=merge --patch '{  
"spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:1.4.0-pxc5.7" }  
}}'

**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN


**cURL Request:**

.. code-block:: bash

   curl -k -v -XPATCH 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1' \
               -H "Authorization: Bearer $KUBE_TOKEN" \
               -H "Accept: application/json" \
               -H "Content-Type: application/merge-patch+json" 
               -d '{  
                 "spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:1.4.0-pxc5.7" }  
                 }}'

.. raw:: html

	<details>

	<summary> Request Body : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{  
	"spec": {"pxc":{ "image": "percona/percona-xtradb-cluster-operator:1.4.0-pxc5.7" }  
	}}
	</pre></div></div>

	</details>

Input:
******

  spec:
  
  pxc:
  
  1. image (String, min-length: 1) : ``name of the image to update for pxc``

.. raw:: html

	<details>

	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBCluster",
	   "metadata":{
	      "annotations":{
		 "kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"pxc.percona.com/v1-4-0\",\"kind\":\"PerconaXtraDBCluster\",\"metadata\":{\"annotations\":{},\"finalizers\":[\"delete-pxc-pods-in-order\"],\"name\":\"cluster1\",\"namespace\":\"default\"},\"spec\":{\"allowUnsafeConfigurations\":true,\"backup\":{\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup\",\"schedule\":[{\"keep\":3,\"name\":\"sat-night-backup\",\"schedule\":\"0 0 * * 6\",\"storageName\":\"s3-us-west\"},{\"keep\":5,\"name\":\"daily-backup\",\"schedule\":\"0 0 * * *\",\"storageName\":\"fs-pvc\"}],\"serviceAccountName\":\"percona-xtradb-cluster-operator\",\"storages\":{\"fs-pvc\":{\"type\":\"filesystem\",\"volume\":{\"persistentVolumeClaim\":{\"accessModes\":[\"ReadWriteOnce\"],\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"s3-us-west\":{\"s3\":{\"bucket\":\"S3-BACKUP-BUCKET-NAME-HERE\",\"credentialsSecret\":\"my-cluster-name-backup-s3\",\"region\":\"us-west-2\"},\"type\":\"s3\"}}},\"pmm\":{\"enabled\":false,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pmm\",\"serverHost\":\"monitoring-service\",\"serverUser\":\"pmm\"},\"proxysql\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"enabled\":true,\"gracePeriod\":30,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-proxysql\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":3,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"2Gi\"}}}}},\"pxc\":{\"affinity\":{\"antiAffinityTopologyKey\":\"none\"},\"gracePeriod\":600,\"image\":\"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0\",\"podDisruptionBudget\":{\"maxUnavailable\":1},\"resources\":{\"requests\":null},\"size\":3,\"volumeSpec\":{\"persistentVolumeClaim\":{\"resources\":{\"requests\":{\"storage\":\"6Gi\"}}}}},\"secretsName\":\"my-cluster-secrets\",\"sslInternalSecretName\":\"my-cluster-ssl-internal\",\"sslSecretName\":\"my-cluster-ssl\",\"updateStrategy\":\"RollingUpdate\",\"vaultSecretName\":\"keyring-secret-vault\"}}\n"
	      },
	      "creationTimestamp":"2020-06-01T16:50:05Z",
	      "finalizers":[
		 "delete-pxc-pods-in-order"
	      ],
	      "generation":3,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1-4-0",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:metadata":{
		          "f:annotations":{
		             ".":{

		             },
		             "f:kubectl.kubernetes.io/last-applied-configuration":{

		             }
		          },
		          "f:finalizers":{

		          }
		       },
		       "f:spec":{
		          ".":{

		          },
		          "f:allowUnsafeConfigurations":{

		          },
		          "f:backup":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:schedule":{

		             },
		             "f:serviceAccountName":{

		             },
		             "f:storages":{
		                ".":{

		                },
		                "f:fs-pvc":{
		                   ".":{

		                   },
		                   "f:type":{

		                   },
		                   "f:volume":{
		                      ".":{

		                      },
		                      "f:persistentVolumeClaim":{
		                         ".":{

		                         },
		                         "f:accessModes":{

		                         },
		                         "f:resources":{
		                            ".":{

		                            },
		                            "f:requests":{
		                               ".":{

		                               },
		                               "f:storage":{

		                               }
		                            }
		                         }
		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   ".":{

		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      },
		                      "f:region":{

		                      }
		                   },
		                   "f:type":{

		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             ".":{

		             },
		             "f:image":{

		             },
		             "f:serverHost":{

		             },
		             "f:serverUser":{

		             }
		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:enabled":{

		             },
		             "f:gracePeriod":{

		             },
		             "f:image":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:size":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:affinity":{
		                ".":{

		                },
		                "f:antiAffinityTopologyKey":{

		                }
		             },
		             "f:gracePeriod":{

		             },
		             "f:podDisruptionBudget":{
		                ".":{

		                },
		                "f:maxUnavailable":{

		                }
		             },
		             "f:resources":{

		             },
		             "f:size":{

		             },
		             "f:volumeSpec":{
		                ".":{

		                },
		                "f:persistentVolumeClaim":{
		                   ".":{

		                   },
		                   "f:resources":{
		                      ".":{

		                      },
		                      "f:requests":{
		                         ".":{

		                         },
		                         "f:storage":{

		                         }
		                      }
		                   }
		                }
		             }
		          },
		          "f:secretsName":{

		          },
		          "f:sslInternalSecretName":{

		          },
		          "f:sslSecretName":{

		          },
		          "f:updateStrategy":{

		          },
		          "f:vaultSecretName":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-06-01T16:52:30Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          "f:pxc":{
		             "f:image":{

		             }
		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-06-01T17:18:58Z"
		 },
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          "f:backup":{
		             "f:storages":{
		                "f:fs-pvc":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   },
		                   "f:s3":{
		                      ".":{

		                      },
		                      "f:bucket":{

		                      },
		                      "f:credentialsSecret":{

		                      }
		                   }
		                },
		                "f:s3-us-west":{
		                   "f:podSecurityContext":{
		                      ".":{

		                      },
		                      "f:fsGroup":{

		                      },
		                      "f:supplementalGroups":{

		                      }
		                   }
		                }
		             }
		          },
		          "f:pmm":{
		             "f:resources":{

		             }
		          },
		          "f:proxysql":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          },
		          "f:pxc":{
		             "f:podSecurityContext":{
		                ".":{

		                },
		                "f:fsGroup":{

		                },
		                "f:supplementalGroups":{

		                }
		             },
		             "f:sslInternalSecretName":{

		             },
		             "f:sslSecretName":{

		             },
		             "f:vaultSecretName":{

		             },
		             "f:volumeSpec":{
		                "f:persistentVolumeClaim":{
		                   "f:accessModes":{

		                   }
		                }
		             }
		          }
		       },
		       "f:status":{
		          ".":{

		          },
		          "f:conditions":{

		          },
		          "f:host":{

		          },
		          "f:message":{

		          },
		          "f:observedGeneration":{

		          },
		          "f:proxysql":{
		             ".":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:pxc":{
		             ".":{

		             },
		             "f:message":{

		             },
		             "f:ready":{

		             },
		             "f:size":{

		             },
		             "f:status":{

		             }
		          },
		          "f:state":{

		          }
		       }
		    },
		    "manager":"percona-xtradb-cluster-operator",
		    "operation":"Update",
		    "time":"2020-06-01T17:21:36Z"
		 }
	      ],
	      "name":"cluster1",
	      "namespace":"default",
	      "resourceVersion":"41149",
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusters/cluster1",
	      "uid":"15e5e7d6-10b2-46cf-85d0-d3fdea3412ca"
	   },
	   "spec":{
	      "allowUnsafeConfigurations":true,
	      "backup":{
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc8.0-backup",
		 "schedule":[
		    {
		       "keep":3,
		       "name":"sat-night-backup",
		       "schedule":"0 0 * * 6",
		       "storageName":"s3-us-west"
		    },
		    {
		       "keep":5,
		       "name":"daily-backup",
		       "schedule":"0 0 * * *",
		       "storageName":"fs-pvc"
		    }
		 ],
		 "serviceAccountName":"percona-xtradb-cluster-operator",
		 "storages":{
		    "fs-pvc":{
		       "type":"filesystem",
		       "volume":{
		          "persistentVolumeClaim":{
		             "accessModes":[
		                "ReadWriteOnce"
		             ],
		             "resources":{
		                "requests":{
		                   "storage":"6Gi"
		                }
		             }
		          }
		       }
		    },
		    "s3-us-west":{
		       "s3":{
		          "bucket":"S3-BACKUP-BUCKET-NAME-HERE",
		          "credentialsSecret":"my-cluster-name-backup-s3",
		          "region":"us-west-2"
		       },
		       "type":"s3"
		    }
		 }
	      },
	      "pmm":{
		 "enabled":false,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pmm",
		 "serverHost":"monitoring-service",
		 "serverUser":"pmm"
	      },
	      "proxysql":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "enabled":true,
		 "gracePeriod":30,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-proxysql",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"2Gi"
		          }
		       }
		    }
		 }
	      },
	      "pxc":{
		 "affinity":{
		    "antiAffinityTopologyKey":"none"
		 },
		 "gracePeriod":600,
		 "image":"percona/percona-xtradb-cluster-operator:1.4.0-pxc5.7",
		 "podDisruptionBudget":{
		    "maxUnavailable":1
		 },
		 "resources":{
		    "requests":null
		 },
		 "size":3,
		 "volumeSpec":{
		    "persistentVolumeClaim":{
		       "resources":{
		          "requests":{
		             "storage":"6Gi"
		          }
		       }
		    }
		 }
	      },
	      "secretsName":"my-cluster-secrets",
	      "sslInternalSecretName":"my-cluster-ssl-internal",
	      "sslSecretName":"my-cluster-ssl",
	      "updateStrategy":"RollingUpdate",
	      "vaultSecretName":"keyring-secret-vault"
	   },
	   "status":{
	      "conditions":[
		 {
		    "lastTransitionTime":"2020-06-01T16:50:37Z",
		    "message":"create newStatefulSetNode: StatefulSet.apps \"cluster1-pxc\" is invalid: spec.updateStrategy: Invalid value: apps.StatefulSetUpdateStrategy{Type:\"SmartUpdate\", RollingUpdate:(*apps.RollingUpdateStatefulSetStrategy)(nil)}: must be 'RollingUpdate' or 'OnDelete'",
		    "reason":"ErrorReconcile",
		    "status":"True",
		    "type":"Error"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T16:52:31Z",
		    "status":"True",
		    "type":"Initializing"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T16:55:59Z",
		    "status":"True",
		    "type":"Ready"
		 },
		 {
		    "lastTransitionTime":"2020-06-01T17:19:15Z",
		    "status":"True",
		    "type":"Initializing"
		 }
	      ],
	      "host":"cluster1-proxysql.default",
	      "message":[
		 "PXC: pxc: back-off 40s restarting failed container=pxc pod=cluster1-pxc-2_default(87cdf1a8-0fb3-4bc0-b50d-f66a0a73c087); "
	      ],
	      "observedGeneration":3,
	      "proxysql":{
		 "ready":3,
		 "size":3,
		 "status":"ready"
	      },
	      "pxc":{
		 "message":"pxc: back-off 40s restarting failed container=pxc pod=cluster1-pxc-2_default(87cdf1a8-0fb3-4bc0-b50d-f66a0a73c087); ",
		 "ready":2,
		 "size":3,
		 "status":"initializing"
	      },
	      "state":"initializing"
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Pass custom my.cnf during the creation of PXC cluster
-----------------------------------------------------

**Describition:**

.. code-block:: bash

   Create a custom config map containing the contents of the file my.cnf to be passed on to the pxc containers we they are created


**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 create configmap cluster1-pxc3 --from-file=my.cnf


**my.cnf (Contains mysql configuration):**

.. code-block:: text

   [mysqld] 
   max_connections=250


**URL:**

.. code-block:: bash

   https://$API_SERVER/api/v1/namespaces/default/configmaps


**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN

**cURL Request:**

.. code-block:: bash

   curl -k -v -XPOST 'https://$API_SERVER/api/v1/namespaces/default/configmaps' \
               -H "Accept: application/json" \
               -H "Authorization: Bearer $KUBE_TOKEN" \
               -d '{"apiVersion":"v1","data":{"my.cnf":"[mysqld]\nmax_connections=250\n"},"kind":"ConfigMap","metadata":{"creationTimestamp":null,"name":"cluster1-pxc3"}}' \
               -H "Content-Type: application/json"

.. raw:: html

	<details>

	<summary> Request Body : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"v1",
	   "data":{
	      "my.cnf":"[mysqld]\nmax_connections=250\n"
	   },
	   "kind":"ConfigMap",
	   "metadata":{
	      "creationTimestamp":null,
	      "name":"cluster1-pxc3"
	   }
	}
	</pre></div></div>

	</details>

Input:
******

  1. data (Object {filename : contents(String, min-length:0)}): ``contains filenames to create in config map and its contents``
  2. metadata: name(String, min-length: 1) : ``contains name of the configmap``
  3. kind (String): ``type of object to create``

.. raw:: html

	<details>

	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "kind":"ConfigMap",
	   "apiVersion":"v1",
	   "metadata":{
	      "name":"cluster1-pxc3",
	      "namespace":"default",
	      "selfLink":"/api/v1/namespaces/default/configmaps/cluster1-pxc3",
	      "uid":"d92c7196-f399-4e20-abc7-b5de62c0691b",
	      "resourceVersion":"85258",
	      "creationTimestamp":"2020-05-28T14:19:41Z",
	      "managedFields":[
		 {
		    "manager":"kubectl",
		    "operation":"Update",
		    "apiVersion":"v1",
		    "time":"2020-05-28T14:19:41Z",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:data":{
		          ".":{

		          },
		          "f:my.cnf":{

		          }
		       }
		    }
		 }
	      ]
	   },
	   "data":{
	      "my.cnf":""
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Backup PXC cluster
------------------

**Description:**

.. code-block:: bash

   Takes a backup of the pxc cluster containers data to be able to recover from disasters or make a roll-back later


**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 apply -f percona-xtradb-cluster-operator/deploy/backup/backup.yaml


**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterbackups


**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN


**cURL Request:**

.. code-block:: bash

   curl -k -v -XPOST 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterbackups' \
               -H "Accept: application/json" \
               -H "Content-Type: application/json" \
               -d "@backup.json" -H "Authorization: Bearer $KUBE_TOKEN"

.. raw:: html

	<details>

	<summary>Request Body (backup.json) : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBClusterBackup",
	   "metadata":{
	      "name":"backup1"
	   },
	   "spec":{
	      "pxcCluster":"cluster1",
	      "storageName":"fs-pvc"
	   }
	}
	</pre></div></div>

	</details>

Input:
******

1. metadata
     name(String, min-length:1) : ``name of backup to create``
2. spec:
  
     1. pxcCluster(String, min-length:1) : ``name of pxc cluster``
     2. storageName(String, min-length:1) : ``name of storage claim to use``

.. raw:: html

	<details>

	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBClusterBackup",
	   "metadata":{
	      "creationTimestamp":"2020-05-27T23:56:33Z",
	      "generation":1,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          ".":{

		          },
		          "f:pxcCluster":{

		          },
		          "f:storageName":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-05-27T23:56:33Z"
		 }
	      ],
	      "name":"backup1",
	      "namespace":"default",
	      "resourceVersion":"26024",
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterbackups/backup1",
	      "uid":"95a354b1-e25b-40c3-8be4-388acca055fe"
	   },
	   "spec":{
	      "pxcCluster":"cluster1",
	      "storageName":"fs-pvc"
	   }
	}
	</pre></div></div>

	</details>

	<div style="page-break-after: always;"></div>

Restore PXC cluster
-------------------

**Description:**

.. code-block:: bash

   Restores pxc cluster data to an earlier version to recover from a problem or to make a roll-back


**Kubectl Command:**

.. code-block:: bash

   kubectl --v=9 apply -f percona-xtradb-cluster-operator/deploy/backup/restore.yaml


**URL:**

.. code-block:: bash

   https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterrestores

**Authentication:**

.. code-block:: bash

   Authorization: Bearer $KUBE_TOKEN


**cURL Request:**

.. code-block:: bash

   curl -k -v -XPOST 'https://$API_SERVER/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterrestores' \
               -H "Accept: application/json" \
               -H "Content-Type: application/json" \
               -d "@restore.json" \
               -H "Authorization: Bearer $KUBE_TOKEN"

.. raw:: html

	<details>

	<summary> Request Body (restore.json) : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBClusterRestore",
	   "metadata":{
	      "name":"restore1"
	   },
	   "spec":{
	      "pxcCluster":"cluster1",
	      "backupName":"backup1"
	   }
	}
	</pre></div></div>

	</details>

Input:
******

1. metadata

     name(String, min-length:1): ``name of restore to create``

2. spec:

     1. pxcCluster(String, min-length:1) : ``name of pxc cluster``
     2. backupName(String, min-length:1) : ``name of backup to restore from``

.. raw:: html

	<details>
	
	<summary> Response : </summary>

	<div class="highlight-yaml notranslate"><div class="highlight"><pre>json
	{
	   "apiVersion":"pxc.percona.com/v1",
	   "kind":"PerconaXtraDBClusterRestore",
	   "metadata":{
	      "creationTimestamp":"2020-05-27T23:59:41Z",
	      "generation":1,
	      "managedFields":[
		 {
		    "apiVersion":"pxc.percona.com/v1",
		    "fieldsType":"FieldsV1",
		    "fieldsV1":{
		       "f:spec":{
		          ".":{

		          },
		          "f:backupName":{

		          },
		          "f:pxcCluster":{

		          }
		       }
		    },
		    "manager":"kubectl",
		    "operation":"Update",
		    "time":"2020-05-27T23:59:41Z"
		 }
	      ],
	      "name":"restore1",
	      "namespace":"default",
	      "resourceVersion":"26682",
	      "selfLink":"/apis/pxc.percona.com/v1/namespaces/default/perconaxtradbclusterrestores/restore1",
	      "uid":"770c3471-be17-46fb-b0a6-e706685ab2fc"
	   },
	   "spec":{
	      "backupName":"backup1",
	      "pxcCluster":"cluster1"
	   }
	}
	</pre></div></div>

	</details>
