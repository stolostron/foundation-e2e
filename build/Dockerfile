FROM  registry.ci.openshift.org/stolostron/builder:go1.19-linux AS builder

COPY COMPONENT_VERSION /COMPONENT_VERSION

# fetch and build work e2e tests
RUN export VERSION=$(cat /COMPONENT_VERSION); git clone -b work --single-branch --branch backplane-$VERSION https://github.com/stolostron/work.git /opt/work

WORKDIR /opt/work

RUN GOOS=${OS} \
    GOARCH=${ARCH} \
    make build-e2e --warn-undefined-variables

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
ENV USER_UID=10001

COPY --from=builder /opt/work/e2e.test /work-e2e

USER ${USER_UID}
