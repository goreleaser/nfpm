FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"
