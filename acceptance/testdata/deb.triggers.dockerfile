FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb

# simulate another package that activates the trigger
RUN dpkg-trigger --by-package foo manual-trigger
RUN dpkg --triggers-only foo

RUN test -f /tmp/trigger-proof

RUN dpkg -r foo
