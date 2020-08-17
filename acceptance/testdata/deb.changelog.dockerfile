FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN apt upgrade -y
RUN apt install -y gzip
RUN dpkg -i /tmp/foo.deb
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 1"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 2"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 3"
