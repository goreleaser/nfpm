FROM quay.io/centos/centos:stream9 AS test_base
RUN yum install -y createrepo yum-utils
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.rpm

FROM test_base AS signed
COPY keys/pubkey.asc /tmp/pubkey.asc
RUN rpm --version
RUN rpm --import /tmp/pubkey.asc
RUN rpm -q gpg-pubkey --qf '%{NAME}-%{VERSION}-%{RELEASE}\t%{SUMMARY}\n'
RUN rpm -vK /tmp/foo.rpm
RUN rpm -vK /tmp/foo.rpm | grep "RSA/SHA256 Signature, key ID 15bd80b3: OK"
RUN rpm -K /tmp/foo.rpm
RUN rpm -K /tmp/foo.rpm | grep -E "(?:pgp|digests signatures) OK"

RUN rm -rf /etc/yum.repos.d/*.repo
COPY keys/test.rpm.repo /etc/yum.repos.d/test.rpm.repo
RUN createrepo /tmp
RUN yum install -y foo

