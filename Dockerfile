FROM alpine
COPY nfpm /nfpm
ENTRYPOINT ["/nfpm"]
