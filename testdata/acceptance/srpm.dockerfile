FROM fedora AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.src.rpm


# ---- rebuild test ----
# Proves the generated source package is a real, rebuildable .src.rpm:
# rpmbuild --rebuild reproduces the binary RPM, which then installs cleanly.
FROM test_base AS rebuild
RUN dnf install -y rpm-build

# The source package is arch "src" and is flagged as a source package.
RUN test "$(rpm -qp --qf '%{ARCH}' /tmp/foo.src.rpm)" = "src"
RUN test "$(rpm -qp --qf '%{SOURCEPACKAGE}' /tmp/foo.src.rpm)" = "1"
RUN test "$(rpm -qp --qf '%{SOURCERPM}' /tmp/foo.src.rpm)" = "(none)"

# Its file list is the generated spec plus the bundled source tarball (source
# packages list flat basenames, not absolute paths).
RUN rpm -qpl /tmp/foo.src.rpm | grep -E 'foo\.spec$'
RUN rpm -qpl /tmp/foo.src.rpm | grep -E 'foo-1\.2\.3\.tar\.gz$'

# Rebuild the binary RPM from source.
RUN rpmbuild --rebuild /tmp/foo.src.rpm
RUN cp "$(find /root/rpmbuild/RPMS -name 'foo-*.rpm' | head -1)" /tmp/foo.rpm

# The rebuilt binary RPM carries the expected metadata and file set.
RUN test "$(rpm -qp --qf '%{EPOCH}:%{NAME}-%{VERSION}-%{RELEASE}' /tmp/foo.rpm)" = "1:foo-1.2.3-4"
RUN rpm -qp -c /tmp/foo.rpm | grep -E '^/etc/foo/whatever\.conf$'
RUN rpm -qp -d /tmp/foo.rpm | grep -E '^/usr/share/doc/foo/README$'
RUN rpm -qp --qf '[%{FILENAMES} %{FILEFLAGS}\n]' /tmp/foo.rpm | grep -E '^/var/lib/foo/state 64$'
RUN test "$(rpm -qp --provides /tmp/foo.rpm | grep '^foo-tool$')" = "foo-tool"
RUN rpm -qp --requires /tmp/foo.rpm | grep -E '^bash$'
RUN rpm -qp --scripts /tmp/foo.rpm | grep -E 'postinstall scriptlet'
# %-sequences in scriptlets must survive the rebuild literally; without the
# spec escaping, ${host%%.*} would be silently mangled to ${host%.*}.
RUN rpm -qp --scripts /tmp/foo.rpm | grep -F '${host%%.*}'

# Install it and verify the payload landed on disk.
RUN rpm -ivh /tmp/foo.rpm
RUN test -f /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /var/log/whatever
RUN test -L /usr/bin/fake-link
RUN test "$(readlink /usr/bin/fake-link)" = "/usr/bin/fake"
RUN test ! -e /var/lib/foo/state
RUN rpm -e foo
RUN test ! -f /usr/bin/fake
