FROM alpine
ARG package
COPY ${package} /tmp/foo.apk
#RUN apk add /tmp/foo.apk
# TODO make this work without --allow-untrusted
RUN apk add --allow-untrusted /tmp/foo.apk
ENV EXPECTVER="foo-1.0.0~0.1.b1+git.abcdefgh description:"
RUN apk info foo | grep "foo-" | grep " description:" > found
RUN export FOUND_VER="$(cat found)" && \
    echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
    test "${FOUND_VER}" = "${EXPECTVER}"
