FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
RUN apt update && apt install -y /tmp/foo.deb
RUN command -v zsh
RUN command -v fish
