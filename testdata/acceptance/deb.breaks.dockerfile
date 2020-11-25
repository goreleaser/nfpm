FROM ubuntu
ARG package
COPY dummy.deb /tmp/dummy.deb
COPY ${package} /tmp/foo.deb

# install dummy package
RUN dpkg -i /tmp/dummy.deb

# make sure foo can't be installed
RUN dpkg -i /tmp/foo.deb 2>&1 | grep "foo breaks dummy"

# make sure foo can be installed if dummy is not installed
RUN dpkg -r dummy
RUN dpkg -i /tmp/foo.deb