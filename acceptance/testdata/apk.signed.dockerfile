FROM alpine
ARG package
COPY keys/rsa_unprotected.pub /etc/apk/keys/john@example.com.rsa.pub
COPY ${package} /tmp/foo.apk

RUN apk verify /tmp/foo.apk | grep "/tmp/foo.apk: 0 - OK"
RUN apk add  /tmp/foo.apk
