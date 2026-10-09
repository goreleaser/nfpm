FROM ghcr.io/void-linux/void-glibc-full:latest AS scenario

ARG package
ARG pkgfile
ARG oldpackage
ARG oldpkgfile
ARG scenario

RUN test -n "${package}"
RUN test -n "${pkgfile}"
RUN test -n "${oldpackage}"
RUN test -n "${oldpkgfile}"
RUN test -n "${scenario}"

RUN mkdir -p /repo
COPY ${oldpackage} /repo/${oldpkgfile}
COPY ${package} /repo/${pkgfile}

RUN set -eu; \
    if [ "${scenario}" = "upgrade" ]; then \
      xbps-rindex -a "/repo/${oldpkgfile}"; \
      xbps-install -i -R /repo -y foo; \
      grep 'v1' /usr/share/foo/current.txt; \
      test -f /usr/share/foo/preserve.txt; \
      xbps-rindex -a "/repo/${pkgfile}"; \
      xbps-install -i -R /repo -y -u foo; \
    else \
      xbps-rindex -a "/repo/${pkgfile}"; \
      xbps-install -i -R /repo -y foo; \
    fi

RUN set -eu; \
    case "${scenario}" in \
      lifecycle) \
        test -x /usr/bin/fake; \
        test -f /etc/foo/whatever.conf; \
        xbps-query -p pkgver foo | grep 'foo-1.2.3_1'; \
        xbps-query -p architecture foo | grep 'x86_64'; \
        xbps-query -p conf_files foo | grep '/etc/foo/whatever.conf'; \
        test -f /tmp/preinstall-proof; \
        test -f /tmp/postinstall-proof; \
        rm -f /tmp/postinstall-proof; \
        xbps-reconfigure -f foo; \
        test -f /tmp/postinstall-proof; \
        xbps-remove -y foo; \
        test -f /tmp/preremove-proof; \
        test -f /tmp/postremove-proof; \
        test ! -e /usr/bin/fake; \
        ;; \
      metadata) \
        xbps-query -p short_desc foo | grep 'XBPS metadata package'; \
        xbps-query -p preserve foo | grep 'yes'; \
        xbps-query -p tags foo | grep 'cli'; \
        xbps-query -p tags foo | grep 'utilities'; \
        xbps-query -p reverts foo | grep '1.2.2_1'; \
        xbps-query -p alternatives foo | grep 'fake-alt'; \
        xbps-query -p alternatives foo | grep '/usr/bin/fake'; \
        ;; \
      noarch) \
        xbps-query -p architecture foo | grep 'noarch'; \
        test -x /usr/bin/fake; \
        ;; \
      upgrade) \
        xbps-query -p pkgver foo | grep 'foo-1.2.4_1'; \
        xbps-query -p preserve foo | grep 'yes'; \
        grep 'v2' /usr/share/foo/current.txt; \
        test -f /usr/share/foo/preserve.txt; \
        ;; \
      *) \
        echo "unknown scenario: ${scenario}" >&2; \
        exit 1; \
        ;; \
    esac
