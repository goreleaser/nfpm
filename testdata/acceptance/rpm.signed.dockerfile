FROM centos:7
ARG package
COPY keys/pubkey.asc /tmp/pubkey.asc
COPY ${package} /tmp/foo.rpm

RUN rpm --import /tmp/pubkey.asc
RUN rpm -q gpg-pubkey --qf '%{NAME}-%{VERSION}-%{RELEASE}\t%{SUMMARY}\n'
RUN rpm -K /tmp/foo.rpm
RUN rpm -K /tmp/foo.rpm | grep "pgp OK"
RUN rpm -vK /tmp/foo.rpm
RUN rpm -vK /tmp/foo.rpm | grep "RSA/SHA256 Signature, key ID 15bd80b3: OK"

# Test with a repo
RUN yum install -y createrepo yum-utils
RUN rm -rf /etc/yum.repos.d/*.repo
COPY keys/test.rpm.repo /etc/yum.repos.d/test.rpm.repo
RUN createrepo /tmp
RUN yum install -y foo
