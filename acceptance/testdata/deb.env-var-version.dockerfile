FROM ubuntu
ARG package
COPY ${package} /tmp/foo.deb
ENV EXPECTVER=" Version: 1.0.0~0.1.b1+git.abcdefgh"
RUN dpkg --info /tmp/foo.deb | grep "Version" > found
RUN export FOUND_VER="$(cat found)" && \
    echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
    test "${FOUND_VER}" = "${EXPECTVER}"
