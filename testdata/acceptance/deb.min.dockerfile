FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb
