FROM fedora
COPY tmp/simple_rpm.rpm /tmp/foo.rpm
RUN rpm -ivh /tmp/foo.rpm && \
		test -e /usr/local/bin/fake && \
		test -f /etc/foo/whatever.conf && \
		echo wat >> /etc/foo/whatever.conf && \
		rpm -e foo && \
		test -f /etc/foo/whatever.conf.rpmsave && \
		test ! -f /usr/local/bin/fake
