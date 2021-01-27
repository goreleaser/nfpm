FROM alpine:3.12
ARG package
COPY ${package} /tmp/foo.apk
RUN apk add -vvv --allow-untrusted /tmp/foo.apk
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN apk del foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake
RUN test -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
