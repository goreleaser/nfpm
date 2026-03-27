FROM ghcr.io/void-linux/void-glibc-full:latest AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/
RUN mkdir -p /var/cache/xbps && cp /tmp/*.xbps /var/cache/xbps/
RUN ls -la /var/cache/xbps/
RUN file /var/cache/xbps/*.xbps || true
RUN cd /tmp && tar -tf /var/cache/xbps/*.xbps 2>&1 | head -20 || true
RUN cd /tmp && tar -xf /var/cache/xbps/*.xbps ./props.plist 2>&1 && cat /tmp/props.plist || true
RUN cd /tmp && tar -xf /var/cache/xbps/*.xbps ./files.plist 2>&1 && cat /tmp/files.plist || true
RUN xbps-rindex -a /var/cache/xbps/*.xbps


# ---- lifecycle test ----
FROM test_base AS lifecycle
RUN xbps-install -yR /var/cache/xbps foo
RUN xbps-query -f foo | tee /tmp/foo-files-query.txt
RUN test -f /var/db/xbps/.foo-files.plist && cat /var/db/xbps/.foo-files.plist || true
RUN ls -ld /usr /usr/bin /etc /etc/foo || true
RUN ls -la /usr/bin/fake /bin/fake /etc/foo/whatever.conf /etc/foo/whatever-link.conf || true
RUN grep '/usr/bin/fake$' /tmp/foo-files-query.txt
RUN grep '^/etc/foo/whatever.conf$' /tmp/foo-files-query.txt
RUN grep '^/etc/foo/whatever-link.conf -> /etc/foo/whatever.conf$' /tmp/foo-files-query.txt
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -L /etc/foo/whatever-link.conf
RUN test "$(readlink /etc/foo/whatever-link.conf)" = "/etc/foo/whatever.conf"
RUN xbps-query --repository=/var/cache/xbps foo | grep '^pkgver: foo-'
RUN xbps-query foo | grep '^state: installed$'
RUN grep -F '<key>short_desc</key><string>Foo bar</string>' /tmp/props.plist
RUN grep -F '<key>conf_files</key><array><string>/etc/foo/whatever.conf</string></array>' /tmp/props.plist
RUN test -f /tmp/postinstall-proof
RUN rm -f /tmp/postinstall-proof
RUN xbps-reconfigure -f foo
RUN test -f /tmp/postinstall-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN xbps-remove -Ry foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -e /usr/bin/fake
RUN test -f /tmp/postremove-proof


# ---- upgrade test ----
FROM ghcr.io/void-linux/void-glibc-full:latest AS upgrade
ARG package
ARG oldpackage
RUN echo "${package}" && echo "${oldpackage}"
COPY ${package} /tmp/new/
COPY ${oldpackage} /tmp/old/
RUN mkdir -p /var/cache/xbps && cp /tmp/old/*.xbps /var/cache/xbps/
RUN xbps-rindex -a /var/cache/xbps/*.xbps
RUN xbps-install -yR /var/cache/xbps foo
RUN test -f /tmp/postinstall-proof
RUN grep 'Install' /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf
RUN cp /tmp/new/*.xbps /var/cache/xbps/
RUN xbps-rindex -a /var/cache/xbps/*.xbps
RUN xbps-install -yR /var/cache/xbps foo
RUN xbps-query foo | grep '^pkgver: foo-2.0.0_1$'
RUN mkdir -p /tmp/inspect && cd /tmp/inspect && tar -xf /tmp/new/*.xbps ./props.plist
RUN grep -F '<key>conflicts</key><array><string>foo-old</string></array>' /tmp/inspect/props.plist
RUN grep -F '<key>provides</key><array><string>fake</string></array>' /tmp/inspect/props.plist
RUN grep -F '<key>replaces</key><array><string>foo</string></array>' /tmp/inspect/props.plist
RUN grep -F '<key>reverts</key><array><string>1.2.3_1</string></array>' /tmp/inspect/props.plist
RUN grep -F '<key>tags</key><string>acceptance cli</string>' /tmp/inspect/props.plist
RUN grep -F '<key>conf_files</key><array><string>/etc/noreplace.conf</string><string>/etc/regular.conf</string></array>' /tmp/inspect/props.plist
RUN test -f /tmp/preremove-proof
RUN grep 'Upgrade' /tmp/preremove-proof
RUN test -f /tmp/postinstall-proof
RUN grep 'Upgrade' /tmp/postinstall-proof


# ---- preserve test ----
FROM ghcr.io/void-linux/void-glibc-full:latest AS preserve
ARG package
ARG oldpackage
RUN echo "${package}" && echo "${oldpackage}"
COPY ${package} /tmp/new/
COPY ${oldpackage} /tmp/old/
RUN mkdir -p /var/cache/xbps && cp /tmp/old/*.xbps /var/cache/xbps/
RUN xbps-rindex -a /var/cache/xbps/*.xbps
RUN xbps-install -yR /var/cache/xbps foo-preserve
RUN grep '^version 1$' /usr/share/foo-preserve/preserved/v1.txt
RUN grep '^version 1$' /usr/share/foo-preserve/current.txt
RUN cp /tmp/new/*.xbps /var/cache/xbps/
RUN xbps-rindex -a /var/cache/xbps/*.xbps
RUN xbps-install -yR /var/cache/xbps foo-preserve
RUN xbps-query foo-preserve | grep '^pkgver: foo-preserve-2.0.0_1$'
RUN grep '^version 1$' /usr/share/foo-preserve/preserved/v1.txt
RUN grep '^version 2$' /usr/share/foo-preserve/preserved/v2.txt
RUN grep '^version 2$' /usr/share/foo-preserve/current.txt
RUN mkdir -p /tmp/inspect && cd /tmp/inspect && tar -xf /tmp/new/*.xbps ./props.plist
RUN grep -F '<key>preserve</key><true/>' /tmp/inspect/props.plist
RUN test -f /tmp/preremove-proof
RUN grep 'Upgrade' /tmp/preremove-proof
RUN test -f /tmp/preinstall-proof
RUN grep 'Upgrade' /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN grep 'Upgrade' /tmp/postinstall-proof
