FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
RUN rpm -ivh /tmp/foo.rpm
RUN rpm -e foo
RUN rpm -qvl /tmp/foo.rpm | grep -E "root\s+root"
