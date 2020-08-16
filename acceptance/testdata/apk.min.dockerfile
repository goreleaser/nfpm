FROM alpine
ARG package
COPY ${package} /tmp/foo.apk
#RUN apk add /tmp/foo.apk
# TODO make this work without --allow-untrusted
RUN apk add --allow-untrusted /tmp/foo.apk
RUN apk del foo
