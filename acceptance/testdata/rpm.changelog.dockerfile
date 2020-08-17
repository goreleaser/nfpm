FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
RUN rpm -ivh /tmp/foo.rpm
RUN rpm -qp /tmp/foo.rpm --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 1$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 2$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 3$"
RUN rpm -q foo --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -q foo --changelog | grep -E "^- note 1$"
RUN rpm -q foo --changelog | grep -E "^- note 2$"
RUN rpm -q foo --changelog | grep -E "^- note 3$"
