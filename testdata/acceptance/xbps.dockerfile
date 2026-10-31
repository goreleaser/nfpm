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

# Every run_depends entry must be resolvable by xbps itself: a bare name such
# as "ca-certificates" is rejected by xbps-uhelper getpkgdepname, while the
# "name>=0" form nFPM emits is accepted.
RUN set -eu; \
    xbps-uhelper getpkgname "$(xbps-uhelper binpkgver "/repo/${pkgfile}")" | grep -x 'foo'

# -i only ignores the repositories configured in xbps.d; it does not disable
# any verification. Packages from a local (file path) repository are checked
# by SHA256, and RSA signatures are only verified for remote repositories.
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

# INSTALL/REMOVE are metadata; they must never be extracted into the root.
RUN test ! -e /INSTALL && test ! -e /REMOVE

RUN set -eu; \
    case "${scenario}" in \
      lifecycle) \
        test -x /usr/bin/fake; \
        test -f /etc/foo/whatever.conf; \
        xbps-query -p pkgver foo | grep -x 'foo-1.2.3_1'; \
        xbps-query -p architecture foo | grep -x 'x86_64'; \
        xbps-query -p conf_files foo | grep -x '/etc/foo/whatever.conf'; \
        xbps-query -p run_depends foo | grep -x 'ca-certificates>=0'; \
        xbps-query -p run_depends foo | grep -x 'zlib>=1.0_1'; \
        for dep in $(xbps-query -p run_depends foo); do xbps-uhelper getpkgdepname "$dep"; done; \
        test -d /var/lib/foo; \
        test "$(stat -c %a /var/lib/foo)" = 750; \
        test -f /tmp/preinstall-proof; \
        test -f /tmp/postinstall-proof; \
        test "$(sed -n 1p /tmp/postinstall-args)" = 6; \
        test "$(sed -n 2p /tmp/postinstall-args)" = post; \
        test "$(sed -n 3p /tmp/postinstall-args)" = foo; \
        test "$(sed -n 4p /tmp/postinstall-args)" = 1.2.3_1; \
        test "$(sed -n 5p /tmp/postinstall-args)" = no; \
        rm -f /tmp/postinstall-proof; \
        xbps-reconfigure -f foo; \
        test -f /tmp/postinstall-proof; \
        xbps-remove -y foo; \
        test -f /tmp/preremove-proof; \
        test -f /tmp/postremove-proof; \
        test ! -e /usr/bin/fake; \
        test ! -e /var/lib/foo; \
        ;; \
      metadata) \
        xbps-query -p pkgver foo | grep -x 'foo-1.2.3.beta.1.git.1_1'; \
        xbps-uhelper getpkgname "$(xbps-query -p pkgver foo)" | grep -x 'foo'; \
        xbps-uhelper getpkgversion "$(xbps-query -p pkgver foo)" | grep -x '1.2.3.beta.1.git.1_1'; \
        xbps-query -p short_desc foo | grep 'XBPS metadata package'; \
        xbps-query -p preserve foo | grep 'yes'; \
        xbps-query -p tags foo | grep 'cli'; \
        xbps-query -p tags foo | grep 'utilities'; \
        xbps-query -p reverts foo | grep '1.2.2_1'; \
        xbps-query -p alternatives foo | grep 'fake-alt'; \
        xbps-query -p alternatives foo | grep '/usr/bin/fake'; \
        ;; \
      noarch) \
        xbps-query -p architecture foo | grep -x 'noarch'; \
        test -x /usr/bin/fake; \
        ;; \
      upgrade) \
        xbps-query -p pkgver foo | grep -x 'foo-1.2.4_1'; \
        xbps-query -p preserve foo | grep 'yes'; \
        grep 'v2' /usr/share/foo/current.txt; \
        test -f /usr/share/foo/preserve.txt; \
        test "$(sed -n 5p /tmp/postinstall-args)" = yes; \
        ;; \
      *) \
        echo "unknown scenario: ${scenario}" >&2; \
        exit 1; \
        ;; \
    esac


# ---- signed test ----
FROM ghcr.io/void-linux/void-glibc-full:latest AS signed

ARG package
ARG pkgfile

RUN test -n "${package}"
RUN test -n "${pkgfile}"

RUN mkdir -p /repo /native /keys
COPY ${package} /repo/${pkgfile}
COPY ${package}.sig2 /repo/${pkgfile}.sig2
COPY keys/rsa_unprotected.pub /keys/pub.pem
COPY keys/rsa_unprotected.priv /keys/priv.pem

# xbps verifies RSA signatures only when installing from a remote repository
# (lib/transaction_fetch.c), which cannot be exercised inside a docker build.
# Instead, check the sidecar against the native tooling:
#  1. it is a valid RSA PKCS#1 v1.5 signature over the SHA-256 of the package,
#     verified with the same library libxbps uses;
#  2. it is byte-for-byte identical to the sidecar xbps-rindex --sign-pkg
#     produces for the same key (RSA PKCS#1 v1.5 is deterministic);
#  3. a signed repository created with the same key accepts the package.
RUN set -eu; \
    openssl dgst -sha256 -verify /keys/pub.pem -signature "/repo/${pkgfile}.sig2" "/repo/${pkgfile}"; \
    cp "/repo/${pkgfile}" /native/; \
    xbps-rindex --privkey /keys/priv.pem --sign-pkg "/native/${pkgfile}"; \
    cmp "/native/${pkgfile}.sig2" "/repo/${pkgfile}.sig2"; \
    xbps-rindex -a "/repo/${pkgfile}"; \
    xbps-rindex --privkey /keys/priv.pem --signedby "nFPM acceptance" --sign /repo; \
    xbps-install -i -R /repo -y foo; \
    xbps-query -p pkgver foo | grep -x 'foo-1.2.3_1'; \
    test -x /usr/bin/fake

# A modified package must no longer verify against the sidecar.
RUN set -eu; \
    printf 'x' >> "/native/${pkgfile}"; \
    if openssl dgst -sha256 -verify /keys/pub.pem -signature "/repo/${pkgfile}.sig2" "/native/${pkgfile}" >/dev/null 2>&1; then \
      echo "tampered package verified unexpectedly" >&2; exit 1; \
    fi
