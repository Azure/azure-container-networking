# Simple Makefile to build and tag the Docker image

IMAGE_NAME = acnpublic.azurecr.io/nncscale
TAG = latest
CONTAINER_ENGINE ?= podman

.PHONY: build
build:
	go build -o nncscale latency.go

.PHONY: cbuild
cbuild: build
	$(CONTAINER_ENGINE) build -t $(IMAGE_NAME):$(TAG) .

.PHONY: push
push:
	$(CONTAINER_ENGINE) push $(IMAGE_NAME):$(TAG)

.PHONY: build-push
build-push: cbuild push