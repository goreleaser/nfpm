FROM fedora
ARG package
COPY ${package} /tmp/foo.rpm
ENV EXPECTVER="Version : 1.0.0~0.1.b1" \
    EXPECTREL="Release : 1"
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Version" > found.ver
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Release" > found.rel
RUN export FOUND_VER="$(cat found.ver)" && \
    echo "Expected: ${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
    test "${FOUND_VER}" = "${EXPECTVER}"
RUN export FOUND_REL="$(cat found.rel)" && \
    echo "Expected: '${EXPECTREL}' :: Found: '${FOUND_REL}'" && \
    test "${FOUND_REL}" = "${EXPECTREL}"
