FROM fedora
ARG package
COPY pubkey.asc /tmp/pubkey.asc
COPY ${package} /tmp/foo.rpm

RUN rpm --import /tmp/pubkey.asc
RUN rpm -K /tmp/foo.rpm | grep ": digests signatures OK"
RUN rpm -K /tmp/foo.rpm -v | grep "RSA/SHA256 Signature, key ID 15bd80b3: OK"

RUN rpm -i /tmp/foo.rpm