FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb && \
		test -e /usr/local/bin/fake && \
		test -f /etc/foo/whatever.conf && \
		echo wat >> /etc/foo/whatever.conf && \
		dpkg -r foo && \
		test -f /etc/foo/whatever.conf && \
		test ! -f /usr/local/bin/fake
