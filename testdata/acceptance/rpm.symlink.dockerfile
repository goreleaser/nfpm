FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
RUN rpm -ivh /tmp/foo.rpm
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"
