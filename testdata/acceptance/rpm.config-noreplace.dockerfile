FROM fedora
ARG package
COPY ${package} /tmp/new_foo.rpm
COPY tmp/noreplace_old_rpm.rpm /tmp/old_foo.rpm

RUN rpm -ivh /tmp/old_foo.rpm

RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf

RUN rpm -ivh /tmp/new_foo.rpm --upgrade

RUN cat /etc/regular.conf | grep foo=baz
RUN test -f /etc/regular.conf.rpmsave
RUN cat /etc/regular.conf.rpmsave | grep modified

RUN cat /etc/noreplace.conf | grep modified
RUN test -f /etc/noreplace.conf.rpmnew
RUN cat /etc/noreplace.conf.rpmnew | grep foo=baz
