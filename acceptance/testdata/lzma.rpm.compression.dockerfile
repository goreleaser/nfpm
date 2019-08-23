FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
RUN test "lzma" = "$(rpm -qp --qf '%{PAYLOADCOMPRESSOR}' /tmp/foo.rpm)"
RUN rpm -ivh /tmp/foo.rpm
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake