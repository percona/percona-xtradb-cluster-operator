# Percona Operator for MySQL based on Percona XtraDB Cluster

![Percona Kubernetes Operators](kubernetes.svg)

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![Docker Pulls](https://img.shields.io/docker/pulls/percona/percona-xtradb-cluster-operator)
![Docker Image Size (latest by date)](https://img.shields.io/docker/image-size/percona/percona-xtradb-cluster-operator)
![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/percona/percona-xtradb-cluster-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/percona/percona-xtradb-cluster-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/percona/percona-xtradb-cluster-operator)](https://goreportcard.com/report/github.com/percona/percona-xtradb-cluster-operator)

[Percona Operator for MySQL based on Percona XtraDB Cluster](https://docs.percona.com/percona-operator-for-mysql/pxc/index.html) (PXC) automates the creation and management of highly available, enterprise-ready MySQL database clusters on Kubernetes.

Within the [Percona Operator for MySQL based on Percona XtraDB Cluster](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html) we have implemented our best practices for deployment and configuration of Percona XtraDB Cluster instances in a Kubernetes-based environment on-premises or in the cloud. The Operator provides the following capabilities to keep the cluster healthy:

* Easy deployment with no single point of failure
* Load balancing and proxy service with either HAProxy or ProxySQL
* Scheduled and manual backups
* Integrated monitoring with [Percona Monitoring and Management](https://www.percona.com/software/database-tools/percona-monitoring-and-management)
* Smart Update to keep your database software up to date automatically
* Automated Password Rotation – use the standard Kubernetes API to enforce password rotation policies for system user
* Private container image registries

While the Percona Operator is primarily managed through the command line, you can also use **[Percona Everest](https://docs.percona.com/everest/index.html)** for a web-based user interface. This open-source tool provides a streamlined experience for provisioning and managing your databases, simplifying day-to-day tasks and reducing administrative overhead. Learn more about Percona Everest in the [documentation](https://docs.percona.com/everest/index.html) or jump right in with the [quickstart guide](https://docs.percona.com/everest/quickstart-guide/quick-install.html).

# Architecture

Percona Operators are based on the [Operator SDK](https://github.com/operator-framework/operator-sdk) and leverage Kubernetes primitives to follow best CNCF practices. 

Please read more about [architecture and design decisions](https://www.percona.com/doc/kubernetes-operator-for-pxc/architecture.html).

## Documentation

To learn more about the Operator, check the [Percona Operator for MySQL based on Percona XtraDB Cluster documentation](https://docs.percona.com/percona-operator-for-mysql/pxc/index.html).

# Quickstart installation

Ready to try out the Operator? Check the [Quickstart tutorial](https://docs.percona.com/percona-operator-for-mysql/pxc/quickstart.html) for easy-to follow steps. 

Below is one of the ways to deploy the Operator using `kubectl`.

## kubectl

1. Deploy the Operator from `deploy/bundle.yaml`:

```sh
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/main/deploy/bundle.yaml
```

2. Deploy the database cluster itself from `deploy/cr.yaml`:

```sh
kubectl apply -f https://raw.githubusercontent.com/percona/percona-xtradb-cluster-operator/main/deploy/cr.yaml

```

See full documentation with examples and various advanced cases on [percona.com](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html).

# Need help?

**Commercial Support**  | **Community Support** |
:-: | :-: |
| <br/>Enterprise-grade assistance for your mission-critical MySQL deployments with the Percona Operator for MySQL. Get expert guidance for complex tasks like multi-cloud replication, database migration and building platforms.<br/><br/>  | <br/>Connect with our engineers and fellow users for general questions, troubleshooting, and sharing feedback and ideas.<br/><br/>  | 
| **[Get Percona Support](https://hubs.ly/Q02ZTH940)** | **[Visit our Forum](https://forums.percona.com/c/mysql-mariadb/percona-kubernetes-operator-for-mysql/28)** |

# Contributing

Percona welcomes and encourages community contributions to help improve Percona Operator for MySQL.

See the [Contribution Guide](CONTRIBUTING.md) and [Building and Testing Guide](e2e-tests/README.md) for more information on how you can contribute.

## Roadmap

We have a public roadmap which can be found [here](https://github.com/orgs/percona/projects/10). Please feel free to contribute and propose new features by following the roadmap [guidelines](https://github.com/percona/roadmap).
 
## Submitting Bug Reports

If you find a bug in Percona Docker Images or in one of the related projects, please submit a report to that project's [JIRA](https://jira.percona.com/browse/K8SPXC) issue tracker or [create a GitHub issue](https://docs.github.com/en/issues/tracking-your-work-with-issues/creating-an-issue#creating-an-issue-from-a-repository) in this repository. 

Learn more about submitting bugs, new features ideas and improvements in the [Contribution Guide](CONTRIBUTING.md).

