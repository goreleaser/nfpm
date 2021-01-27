FROM alpine:3.12
ARG package
COPY ${package} /tmp/foo.apk
RUN apk add --allow-untrusted /tmp/foo.apk
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"
