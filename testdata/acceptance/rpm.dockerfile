FROM fedora AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.rpm
RUN echo -e "fastestmirror=true\ndeltarpm=true\n" | tee -a /etc/dnf/dnf.conf
RUN dnf check-update


# ---- minimal test ----
FROM test_base AS min
RUN rpm -ivh /tmp/foo.rpm


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake


# ---- no-glob test ----
FROM min AS no-glob
RUN test -d /usr/share/whatever/
RUN test -f /usr/share/whatever/file1
RUN test -f /usr/share/whatever/file2
RUN test -d /usr/share/whatever/folder2
RUN test -f /usr/share/whatever/folder2/file1
RUN test -f /usr/share/whatever/folder2/file2


# ---- complex test ----
FROM min AS complex
RUN test "$(rpm -qp --recommends /tmp/foo.rpm)" = "fish"
RUN test "$(rpm -qp --suggests /tmp/foo.rpm)" = "zsh"
RUN test "$(rpm -qp --requires /tmp/foo.rpm)" = "bash"
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /usr/share/whatever/folder
RUN test -f /usr/share/whatever/folder/file1
RUN test -f /usr/share/whatever/folder/file2
RUN test -d /usr/share/whatever/folder/folder2
RUN test -f /usr/share/whatever/folder/folder2/file1
RUN test -f /usr/share/whatever/folder/folder2/file2
RUN test -d /var/log/whatever
RUN test -d /usr/share/foo
RUN test -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /var/log/whatever
RUN test ! -d /usr/share/foo


# ---- signed test ----
FROM test_base AS signed
COPY keys/pubkey.asc /tmp/pubkey.asc
RUN rpm --import /tmp/pubkey.asc
RUN rpm -q gpg-pubkey --qf '%{NAME}-%{VERSION}-%{RELEASE}\t%{SUMMARY}\n'
RUN rpm -K /tmp/foo.rpm
RUN rpm -K /tmp/foo.rpm | grep -E "(?:pgp|digests signatures) OK"
RUN rpm -vK /tmp/foo.rpm
RUN rpm -vK /tmp/foo.rpm | grep "RSA/SHA256 Signature, key ID 15bd80b3: OK"

# Test with a repo
RUN dnf install -y createrepo yum-utils
RUN rm -rf /etc/yum.repos.d/*.repo
COPY keys/test.rpm.repo /etc/yum.repos.d/test.rpm.repo
RUN createrepo /tmp
RUN dnf install -y foo


# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /usr/share/whatever/folder
RUN test -f /usr/share/whatever/folder/file1
RUN test -f /usr/share/whatever/folder/file2
RUN test -d /usr/share/whatever/folder/folder2
RUN test -f /usr/share/whatever/folder/folder2/file1
RUN test -f /usr/share/whatever/folder/folder2/file2
RUN test -f /tmp/preinstall-proof
RUN test ! -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake
RUN test ! -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof


# ---- meta test ----
FROM test_base AS meta
RUN dnf install -y /tmp/foo.rpm
RUN command -v zsh
RUN command -v fish


# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER="Version : 1.0.0~0.1.b1+git.abcdefgh" \
    EXPECTREL="Release : 1"
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Version" > found.ver
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Release" > found.rel
RUN export FOUND_VER="$(cat found.ver)" && \
    echo "Expected: ${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
    test "${FOUND_VER}" = "${EXPECTVER}"
RUN export FOUND_REL="$(cat found.rel)" && \
    echo "Expected: '${EXPECTREL}' :: Found: '${FOUND_REL}'" && \
    test "${FOUND_REL}" = "${EXPECTREL}"


# ---- changelog test ----
FROM min AS withchangelog
RUN rpm -qp /tmp/foo.rpm --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 1$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 2$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 3$"
RUN rpm -q foo --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -q foo --changelog | grep -E "^- note 1$"
RUN rpm -q foo --changelog | grep -E "^- note 2$"
RUN rpm -q foo --changelog | grep -E "^- note 3$"


# ---- compression test ----
FROM min AS compression
ARG compression
RUN test "${compression}" = "$(rpm -qp --qf '%{PAYLOADCOMPRESSOR}' /tmp/foo.rpm)"
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake

# ---- config-noreplace test ----
FROM test_base AS config-noreplace
COPY tmp/noreplace_old_rpm.rpm /tmp/old_foo.rpm
RUN rpm -ivh /tmp/old_foo.rpm

RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf

RUN rpm -ivh /tmp/foo.rpm --upgrade

RUN cat /etc/regular.conf | grep foo=baz
RUN test -f /etc/regular.conf.rpmsave
RUN cat /etc/regular.conf.rpmsave | grep modified

RUN cat /etc/noreplace.conf | grep modified
RUN test -f /etc/noreplace.conf.rpmnew
RUN cat /etc/noreplace.conf.rpmnew | grep foo=baz

# ---- release test ----
FROM min AS release
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/local/bin/fake
