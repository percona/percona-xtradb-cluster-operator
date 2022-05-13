# Contributing to the Operator

Percona welcomes and encourages community contributions to help improve [Percona Operator for MySQL based on Percona XtraDB Cluster](https://www.percona.com/doc/kubernetes-operator-for-pxc/index.html). The Operator automates the creation, modification, or deletion of items in your Percona XtraDB Cluster environment, and contains the necessary Kubernetes settings to maintain a consistent Percona XtraDB Cluster instance.

## Prerequisites

Before submitting code or documentation contributions, you should first complete the following prerequisites.

### 1. Sign the CLA

Before you can contribute, we kindly ask you to sign our [Contributor License Agreement](https://cla-assistant.percona.com/percona/percona-xtradb-cluster-operator) (CLA). You can do this using your GitHub account and one click.

### 2. Code of Conduct

Please make sure to read and observe the [Contribution Policy](code-of-conduct.md).

## Submitting a pull request

Improvement and bugfix tasks for the Operator are tracked in [Jira](https://Jira.percona.com/projects/K8SPXC/issues). Although not mandatory, it is a good practice to examine already open Jira issues before submitting a pull request. For bigger contributions, we suggest creating a Jira issue first and discussing it with the engineering team and community before proposing any code changes.

Another good place to discuss Percona Operator for MySQL based on Percona XtraDB Cluster with developers and other community members is the [community forum](https://forums.percona.com/categories/kubernetes-operator-percona-xtradb-cluster).

### 1. Contributing to the source tree

Contributions to the source tree should follow the workflow described below:

1. First, you need to [fork the repository on GitHub](https://docs.github.com/en/github/collaborating-with-issues-and-pull-requests/syncing-a-fork), clone your fork locally, and then [sync your local fork to upstream](https://docs.github.com/en/github/collaborating-with-issues-and-pull-requests/syncing-a-fork). After that, before starting to work on changes, make sure to always sync your fork with upstream. 
2. Create a branch for changes you are planning to make. If there is a Jira ticket related to your contribution, it is recommended to name your branch in the following way: `<Jira issue number>-<short description>`, where the issue number is something like `K8SPXC-622`.

   Create the branch in your local repo as follows:

   ```
   git checkout -b K8SPXC-622-fix-feature-X
   ```

   When your changes are [ready](e2e-tests/README.md), make a commit, mentioning the Jira issue in the commit message, if any:

   ```
   git add .
   git commit -m "K8SPXC-622 fixed by ......"
   git push -u origin K8SPXC-622-fix-feature X
   ```

3. Create a pull request to the main repository on GitHub.
4. When the reviewer makes some comments, address any feedback that comes and update the pull request.
5. When your contribution is accepted, your pull request will be approved and merged to the main branch.


### 2. Contributing to documentation

The workflow for documentation is similar. Please take into account a few things:

1. All documentation is written using the [Sphinx engine markup language](https://www.sphinx-doc.org/). Sphinx allows easy publishing of various output formats such as HTML, LaTeX (for PDF), ePub, Texinfo, etc.
2. We store the documentation as *.rst files in the [pxc-docs](https://github.com/percona/percona-xtradb-cluster-operator/tree/pxc-docs) branch of the Operator GitHub repository. The documentation is licensed under the [Attribution 4.0 International license (CC BY 4.0)](https://creativecommons.org/licenses/by/4.0/).

After [installing Sphinx](https://www.sphinx-doc.org/en/master/usage/installation.html) you can use `make html` or `make latexpdf` commands having the documentation branch as your current directory to build HTML and PDF versions of the documentation respectively.

## Code review

### 1. Automated code review

Your pull request will go through an automated build and testing process, and you will have a comment with the report once all tests are over (usually, it takes about 3 hours).

### 2. Code review by the Operator developers

Your contribution will be reviewed by other developers contributing to the project. The more complex your changes are, the more experts will be involved. You will receive feedback and recommendations directly on your pull request on GitHub, so keep an eye on your submission and be prepared to make further amendments. The developers might even provide some concrete suggestions on how to modify your code to better match the project’s expectations.

## Building and automated testing

See [e2e-tests/README.md](e2e-tests/README.md) to find out how to build the Operator and run tests.
