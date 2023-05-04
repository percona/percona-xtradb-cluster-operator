![Percona Operator for MySQL based on Percona XtraDB Cluster](operator.png)
<div align="center">

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![Docker Pulls](https://img.shields.io/docker/pulls/percona/percona-xtradb-cluster-operator)
![Docker Image Size (latest by date)](https://img.shields.io/docker/image-size/percona/percona-xtradb-cluster-operator)
![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/percona/percona-xtradb-cluster-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/percona/percona-xtradb-cluster-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/percona/percona-xtradb-cluster-operator)](https://goreportcard.com/report/github.com/percona/percona-xtradb-cluster-operator)
</div>

[Percona XtraDB Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster) (PXC) is an open-source enterprise MySQL solution that helps you to ensure data availability for your applications while improving security and simplifying the development of new applications in the most demanding public, private, and hybrid cloud environments.

Based on our best practices for deployment and configuration, [Percona Operator for MySQL based on Percona XtraDB Cluster](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html) contains everything you need to quickly and consistently deploy and scale Percona XtraDB Cluster instances in a Kubernetes-based environment on-premises or in the cloud. It provides the following capabilities:

* Easy deployment with no single point of failure
* Load balancing and proxy service with either HAProxy or ProxySQL
* Scheduled and manual backups
* Integrated monitoring with [Percona Monitoring and Management](https://www.percona.com/software/database-tools/percona-monitoring-and-management)
* Smart Update to keep your database software up to date automatically
* Automated Password Rotation – use the standard Kubernetes API to enforce password rotation policies for system user
* Private container image registries

# Architecture

Percona Operators are based on the [Operator SDK](https://github.com/operator-framework/operator-sdk) and leverage Kubernetes primitives to follow best CNCF practices. 

Please read more about architecture and design decisions [here](https://www.percona.com/doc/kubernetes-operator-for-pxc/architecture.html).

# Quickstart installation

## Helm

Install the Operator:

```sh
helm install my-op percona/pxc-operator
```

Install Percona XtraDB Cluster:
```sh
helm install my-db percona/pxc-db
```

See more details in:
- [Helm installation documentation](https://www.percona.com/doc/kubernetes-operator-for-pxc/helm.html)
- [Operator helm chart parameter reference](https://github.com/percona/percona-helm-charts/tree/main/charts/pxc-operator)
- [Percona XtraDB Cluster helm chart parameters reference](https://github.com/percona/percona-helm-charts/tree/main/charts/pxc-db)


## kubectl

It usually takes two steps to deploy Percona XtraDB Cluster on Kubernetes.

Deploy the Operator from `deploy/bundle.yaml`:

```sh
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/main/deploy/bundle.yaml
```

Deploy the database cluster itself from `deploy/cr.yaml`:

```sh
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/main/deploy/cr.yaml

```

See full documentation with examples and various advanced cases on [percona.com](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html).

# Contributing

Percona welcomes and encourages community contributions to help improve Percona Operator for MySQL.

See the [Contribution Guide](CONTRIBUTING.md) and [Building and Testing Guide](e2e-tests/README.md) for more information.

# Join Percona Kubernetes Squad!                                                                              
```                                                                                     
                    %                        _____                
                   %%%                      |  __ \                                          
                 ###%%%%%%%%%%%%*           | |__) |__ _ __ ___ ___  _ __   __ _             
                ###  ##%%      %%%%         |  ___/ _ \ '__/ __/ _ \| '_ \ / _` |            
              ####     ##%       %%%%       | |  |  __/ | | (_| (_) | | | | (_| |            
             ###        ####      %%%       |_|   \___|_|  \___\___/|_| |_|\__,_|           
           ,((###         ###     %%%        _      _          _____                       _
          (((( (###        ####  %%%%       | |   / _ \       / ____|                     | | 
         (((     ((#         ######         | | _| (_) |___  | (___   __ _ _   _  __ _  __| | 
       ((((       (((#        ####          | |/ /> _ </ __|  \___ \ / _` | | | |/ _` |/ _` |
      /((          ,(((        *###         |   <| (_) \__ \  ____) | (_| | |_| | (_| | (_| |
    ////             (((         ####       |_|\_\\___/|___/ |_____/ \__, |\__,_|\__,_|\__,_|
   ///                ((((        ####                                  | |                  
 /////////////(((((((((((((((((########                                 |_|   Join @ percona.com/k8s   
```

You can get early access to new product features, invite-only ”ask me anything” sessions with Percona Kubernetes experts, and monthly swag raffles. Interested? Fill in the form at [percona.com/k8s](https://www.percona.com/k8s).

# Roadmap

We have an experimental public roadmap which can be found [here](https://github.com/percona/roadmap/projects/1). Please feel free to contribute and propose new features by following the roadmap [guidelines](https://github.com/percona/roadmap).
 
# Submitting Bug Reports

If you find a bug in Percona Docker Images or in one of the related projects, please submit a report to that project's [JIRA](https://jira.percona.com/browse/K8SPXC) issue tracker. Learn more about submitting bugs, new features ideas and improvements in the [Contribution Guide](CONTRIBUTING.md).

