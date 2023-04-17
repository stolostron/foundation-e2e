FROM  registry.ci.openshift.org/stolostron/builder:go1.19-linux AS builder

COPY COMPONENT_VERSION /COMPONENT_VERSION

# fetch and build work e2e tests
RUN export VERSION=$(cat /COMPONENT_VERSION); export COMPONENT_VERSION=${VERSION%.0}; git clone -b work --single-branch --branch backplane-$COMPONENT_VERSION https://github.com/stolostron/work.git /opt/work

WORKDIR /opt/work

RUN GOOS=${OS} \
    GOARCH=${ARCH} \
    make build-e2e --warn-undefined-variables

# fetch and build placement e2e tests
RUN export VERSION=$(cat /COMPONENT_VERSION); export COMPONENT_VERSION=${VERSION%.0}; git clone -b placement --single-branch --branch backplane-$COMPONENT_VERSION https://github.com/stolostron/placement.git /opt/placement
WORKDIR /opt/placement

RUN GOOS=${OS} \
    GOARCH=${ARCH} \
    make build-e2e --warn-undefined-variables

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=10001

COPY build/start_tests.sh /start_tests.sh
COPY --from=builder /opt/work/e2e.test /work-e2e
COPY --from=builder /opt/placement/e2e.test /placement-e2e

USER ${USER_UID}
