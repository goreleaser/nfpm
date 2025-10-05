FROM alpine:3.22.1@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/nfpm*.apk /tmp/
RUN apk add --allow-untrusted /tmp/nfpm_*.apk
ENTRYPOINT ["/usr/bin/nfpm"]
