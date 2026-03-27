FROM ghcr.io/void-linux/void-glibc-full:latest AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/
RUN mkdir -p /var/cache/xbps && cp /tmp/*.xbps /var/cache/xbps/
RUN ls -la /var/cache/xbps/
RUN file /var/cache/xbps/*.xbps || true
RUN cd /tmp && tar -tf /var/cache/xbps/*.xbps 2>&1 | head -20 || true
RUN cd /tmp && tar -xf /var/cache/xbps/*.xbps ./props.plist 2>&1 && cat /tmp/props.plist || true
RUN xbps-rindex -a /var/cache/xbps/*.xbps


# ---- lifecycle test ----
FROM test_base AS lifecycle
RUN xbps-install -yR /var/cache/xbps foo
RUN test -x /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -L /etc/foo/whatever-link.conf
RUN test "$(readlink /etc/foo/whatever-link.conf)" = "/etc/foo/whatever.conf"
RUN xbps-query --repository=/var/cache/xbps foo | grep '^pkgver: foo-'
RUN xbps-query foo | grep '^state: installed$'
RUN test -f /tmp/postinstall-proof
RUN xbps-reconfigure -f foo
RUN test -f /tmp/postinstall-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN xbps-remove -Ry foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -e /usr/bin/fake
RUN test -f /tmp/postremove-proof
