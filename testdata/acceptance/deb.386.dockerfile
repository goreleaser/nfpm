FROM i386/ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN dpkg -r foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake
