FROM alpine
ARG package
COPY ${package} /tmp/foo.apk
RUN apk add --allow-untrusted /tmp/foo.apk
RUN command -v zsh
RUN command -v fish
