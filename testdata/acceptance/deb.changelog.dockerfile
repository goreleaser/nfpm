FROM ubuntu
ARG package
# the dpkg configuration of the docker
# image filters out changelogs by default
# so we have to remove that rule
RUN rm /etc/dpkg/dpkg.cfg.d/excludes
COPY ${package} /tmp/foo.deb
RUN apt update -y
RUN apt install -y gzip
RUN dpkg -i /tmp/foo.deb
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 1"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 2"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 3"
