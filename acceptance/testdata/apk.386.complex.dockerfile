FROM alpine
ARG package
COPY ${package} /tmp/foo.apk
#RUN apk add /tmp/foo.apk
# TODO make this work without --allow-untrusted
RUN apk add --allow-untrusted /tmp/foo.apk
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /usr/share/whatever/
RUN test -d /usr/share/whatever/folder
RUN test -f /usr/share/whatever/folder/file1
RUN test -f /usr/share/whatever/folder/file2
RUN test -d /usr/share/whatever/folder/folder2
RUN test -f /usr/share/whatever/folder/folder2/file1
RUN test -f /usr/share/whatever/folder/folder2/file2
RUN test -d /usr/share/foo
RUN test -d /usr/share/whatever
RUN test -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN apk del foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /usr/share/foo
RUN test ! -d /usr/share/whatever
