IMAGES = eirini-controller

TAG ?= latest
DOCKER_DIR := ${CURDIR}
EIRINI_CONTROLLER_DIR := ${DOCKER_DIR}/..
REVISION := $(shell git -C $(EIRINI_CONTROLLER_DIR) rev-parse HEAD)

.PHONY: $(IMAGES)

all: $(IMAGES)

$(IMAGES):
	DOCKER_BUILDKIT=1 docker build $(EIRINI_CONTROLLER_DIR) \
		--file "$(DOCKER_DIR)/$@/Dockerfile" \
		--build-arg GIT_SHA=$(REVISION) \
		--tag "eirini/$@:$(TAG)"

push:
	$(foreach img, $(IMAGES), docker push eirini/$(img):$(TAG);)

push-%:
	docker push eirini/$*:$(TAG)

clean:
	docker image prune --force
	docker rmi $(shell docker image ls --format="{{.Repository}}:{{.Tag}}" --filter=reference="eirini/*")

