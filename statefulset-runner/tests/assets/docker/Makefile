PUBLIC_IMAGES = custom-port dorini busybox busybox-root
PRIVATE_IMAGES = notdora
ALL_IMAGES = $(PUBLIC_IMAGES) $(PRIVATE_IMAGES)

TAG ?= latest
DOCKER_DIR := ${CURDIR}
EIRINI_DIR := ${DOCKER_DIR}/../../..
REVISION := $(shell git -C $(EIRINI_DIR) rev-parse HEAD)

.PHONY: $(ALL_IMAGES)

all: $(ALL_IMAGES)

$(PUBLIC_IMAGES):
	docker build $(EIRINI_DIR) \
		--file "$(DOCKER_DIR)/$@/Dockerfile" \
		--build-arg GIT_SHA=$(REVISION) \
		--tag "eirini/$@:$(TAG)"

$(PRIVATE_IMAGES):
	docker build $(EIRINI_DIR) \
		--file "$(DOCKER_DIR)/$@/Dockerfile" \
		--build-arg GIT_SHA=$(REVISION) \
		--tag "eiriniuser/$@:$(TAG)"

push:
	$(foreach img, $(PUBLIC_IMAGES), docker push eirini/$(img):$(TAG);)
	$(foreach img, $(PRIVATE_IMAGES), docker push eiriniuser/$(img):$(TAG);)

push-%:
	docker push eirini/$*:$(TAG)

push-notdora:
	docker push eiriniuser/notdora:$(TAG)

clean:
	docker container prune --force
	docker image prune --force
	$(foreach img, $(PUBLIC_IMAGES), docker rmi eirini/$(img):$(TAG) || true;)
	$(foreach img, $(PRIVATE_IMAGES), docker rmi eiriniuser/$(img):$(TAG) || true;)

