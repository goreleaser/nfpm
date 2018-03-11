FROM ubuntu
COPY tmp/simple_deb/foo.deb /tmp/foo.deb
RUN dpkg -i /tmp/foo.deb && \
		test -e /usr/bin/fake && \
		test -f /etc/foo/whatever.conf && \
		echo wat >> /etc/foo/whatever.conf && \
		dpkg -r foo && \
		test -f /etc/foo/whatever.conf && \
		test ! -f /usr/bin/fake
