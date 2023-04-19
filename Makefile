export PROJECT_DIR            = $(shell 'pwd')
export COMPONENT_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export DOCKER_FILE        = $(PROJECT_DIR)/build/Dockerfile
export DOCKER_REGISTRY   ?= quay.io/stolostron
export DOCKER_IMAGE      ?= $(COMPONENT_NAME)
export DOCKER_BUILD_TAG  ?= latest
export DOCKER_BUILDER    ?= docker

.PHONY: build-image
build-image:
	@$(DOCKER_BUILDER) build -t ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:$(DOCKER_BUILD_TAG) -f $(DOCKER_FILE) .
	echo "${DOCKER_REGISTRY}/${DOCKER_IMAGE}:$(DOCKER_BUILD_TAG)"


.PHONY: build
build:
	echo "Build binary from foundation-e2e repo..."


test-performance:
	rm -rf output/*
	mkdir -p output
	go test -c ./performance 
	./performance.test -test.v -ginkgo.v --ginkgo.timeout=10h --cluster-count=20 --placement-count=20
