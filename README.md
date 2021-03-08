# Percona XtraDB Cluster Operator

A Kubernetes operator for [Percona XtraDB Cluster](https://www.percona.com/software/mysql-database/percona-xtradb-cluster) based on the [Operator SDK](https://github.com/operator-framework/operator-sdk).

## Installation
See the [Official Documentation](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html) for more information.

[![Official Documentation](https://via.placeholder.com/260x60/419bdc/FFFFFF/?text=Documentation)](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html)

## Contributing to the Operator
Percona welcomes and encourages community contributions to help improve Percona Kubernetes Operator for Percona XtraDB Cluster.

See the [Contribution Guide](https://github.com/percona/percona-xtradb-cluster-operator/blob/main/CONTRIBUTING.md) and [Building and Testing Guide](https://github.com/percona/percona-xtradb-cluster-operator/blob/main/e2e-tests/README.md) for more information.

## Submitting Bug Reports

If you find a bug in Percona Docker Images or in one of the related projects, please submit a report to that project's [JIRA](https://jira.percona.com) issue tracker.

Your first step should be [search](https://jira.percona.com/issues/?jql=project%20in%20(%22Cloud%20Dev%22%2C%20%22Percona%20Kubernetes%20Operator%20for%20Percona%20XtraDB%20Cluster%22))  for a similar report in the existing set of open tickets. If someone else has already reported your problem, upvote that report to increase its visibility.

If there is no existing report, submit a report following these steps:

1. [Sign in to Percona JIRA.](https://jira.percona.com/login.jsp) You will need to create an account if you do not have one.
2. [Go to the Create Issue screen and select the relevant project.](https://jira.percona.com/secure/CreateIssueDetails!init.jspa?pid=12702&issuetype=1&priority=3)
3. Fill in the fields of Summary, Description, Steps To Reproduce, and Affects Version to the best you can. If the bug corresponds to a crash, attach the stack trace from the logs.

An excellent resource is [Elika Etemad's article on filing good bug reports.](http://fantasai.inkedblade.net/style/talks/filing-good-bugs/).

As a general rule of thumb, please try to create bug reports that are:

- *Reproducible.* Include steps to reproduce the problem.
- *Specific.* Include as much detail as possible: which version, what environment, etc.
- *Unique.* Do not duplicate existing tickets.
- *Scoped to a Single Bug.* One bug per report.

