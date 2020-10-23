FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
RUN dnf install -y /tmp/foo.rpm
RUN command -v zsh
RUN command -v fish
