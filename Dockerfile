FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/nfpm*.apk /tmp/
RUN apk add --allow-untrusted /tmp/nfpm_*.apk
ENTRYPOINT ["/usr/bin/nfpm"]
