FROM alpine
COPY nfpm /usr/local/bin/nfpm
ENTRYPOINT ["/usr/local/bin/nfpm"]
