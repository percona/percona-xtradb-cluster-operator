#FROM registry.access.redhat.com/ubi7/ubi-minimal
FROM centos:7

#RUN microdnf update && microdnf clean all
#RUN microdnf install yum
#RUN yum -y install wget
#RUN wget http://repo.mysql.com/mysql-community-release-el7-5.noarch.rpm
#RUN rpm -ivh mysql-community-release-el7-5.noarch.rpm
#RUN yum update
#RUN yum -y --skip-broken install mysql-server

# check repository package signature in secure way
RUN export GNUPGHOME="$(mktemp -d)" \
	&& gpg --keyserver pool.sks-keyservers.net --recv-keys 430BDF5C56E7C94E848EE60C1C4CBDCDCD2EFD2A 6341AB2753D78A78A7C27BB124C6A8A7F4A80EB5 91E97D7C4A5E96F17F3E888F6A2FAEA2352C64E5 \
	&& gpg --export --armor 430BDF5C56E7C94E848EE60C1C4CBDCDCD2EFD2A > ${GNUPGHOME}/RPM-GPG-KEY-Percona \
	&& gpg --export --armor 6341AB2753D78A78A7C27BB124C6A8A7F4A80EB5 > ${GNUPGHOME}/RPM-GPG-KEY-CentOS-7 \
	&& gpg --export --armor 91E97D7C4A5E96F17F3E888F6A2FAEA2352C64E5 > ${GNUPGHOME}/RPM-GPG-KEY-EPEL-7 \
	&& rpmkeys --import ${GNUPGHOME}/RPM-GPG-KEY-Percona ${GNUPGHOME}/RPM-GPG-KEY-CentOS-7 ${GNUPGHOME}/RPM-GPG-KEY-EPEL-7 \
	&& curl -L -o /tmp/percona-release.rpm https://repo.percona.com/yum/percona-release-latest.noarch.rpm \
	&& rpmkeys --checksig /tmp/percona-release.rpm \
	&& rpm -i /tmp/percona-release.rpm \
	&& rm -rf "$GNUPGHOME" /tmp/percona-release.rpm \
	&& rpm --import /etc/pki/rpm-gpg/PERCONA-PACKAGING-KEY

# install exact version for repeatability
ENV PS_VERSION 5.7.28-31.1.el7
ENV PROXYSQL_VERSION 2.0.7-1.2.el7

#RUN curl -L -o /tmp/jq.rpm https://download.fedoraproject.org/pub/epel/7/x86_64/Packages/j/jq-1.6-1.el7.x86_64.rpm \
#	&& curl -L -o /tmp/oniguruma.rpm https://download.fedoraproject.org/pub/epel/7/x86_64/Packages/o/oniguruma-5.9.5-3.el7.x86_64.rpm \
#	&& rpmkeys --checksig /tmp/jq.rpm /tmp/oniguruma.rpm \
#	&& rpm -i /tmp/jq.rpm /tmp/oniguruma.rpm \
#	&& rm -rf /tmp/jq.rpm /tmp/oniguruma.rpm

RUN yum update -y \
        && yum install -y \
                Percona-Server-client-57-${PS_VERSION} \
                Percona-Server-shared-57-${PS_VERSION} \
                shadow-utils \
                which \
                hostname \
                perl-DBD-MySQL \
                perl-DBI \
        && yum clean all

#RUN yum install -y gnu-getopt

RUN groupadd -g 1001 proxysql \
    && useradd -u 1001 -r -g 1001 -s /sbin/nologin \
        -c "Default Application User" proxysql

LABEL name="Percona XtraDB Cluster Operator" \
      vendor="Percona" \
      summary="Percona XtraDB Cluster is an active/active high availability and high scalability open source solution for MySQL clustering" \
      description="Percona XtraDB Cluster is a high availability solution that helps enterprises avoid downtime and outages and meet expected customer experience." \
      maintainer="Percona Development <info@percona.com>"

COPY LICENSE /licenses/
COPY build/_output/bin/percona-xtradb-cluster-operator /usr/local/bin/percona-xtradb-cluster-operator
COPY build/_output/bin/user-manager /usr/local/bin/user-manager
COPY build/_output/bin/proxysql-admin-tool /percona/proxysql-admin-tool
COPY build/_output/bin/proxysql-admin-tool/proxysql-admin.cnf /etc/proxysql-admin.cnf
COPY build/_output/bin/proxysql/ /var/lib/proxysql/
RUN chown 1001:1001 /etc/proxysql-admin.cnf /var/lib/proxysql\
    && chmod 664 /etc/proxysql-admin.cnf && chmod 777 /var/lib/proxysql

USER nobody
