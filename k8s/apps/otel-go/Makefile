IMAGE   ?= ghcr.io/mblum/otel-go
# Default tag is the short git SHA — never use 'latest'.
# Override: make build TAG=v0.1.0
TAG     := $(shell git rev-parse --short HEAD)
NS      := otel-demo
DOMAIN  ?= example.local

APP_DIR := ../../../apps/otel-go

.PHONY: build load push deploy delete

## build: Build the Docker image with the current git SHA tag.
build:
	docker build -t $(IMAGE):$(TAG) $(APP_DIR)

## load: Build and import the image directly into k3s containerd.
##
## Use this on a single-node cluster to skip the push/pull round-trip.
## No registry credentials required.
##
##   make load                                    # builds + imports in one step
##   sudo k3s ctr images ls | grep otel-go        # verify
load: build
	docker save $(IMAGE):$(TAG) | sudo k3s ctr images import -

## push: Build and push the image to ghcr.io (requires docker login).
##
##   echo $$GITHUB_TOKEN | docker login ghcr.io -u <user> --password-stdin
##   make push
push: build
	docker push $(IMAGE):$(TAG)

## deploy: Apply k8s manifests for the otel-demo workload.
##
## Run `make load` (local) or `make push` (registry) first, then:
##
##   make deploy                        # uses git SHA tag automatically
##   make deploy DOMAIN=example.local
##
## After deploying, hot-reload Alloy so it starts scraping otel-go metrics:
##
##   kubectl apply -f ../../o11y/manifests/alloy-configmap.yaml
##
deploy:
	kubectl apply -f namespace.yaml
	IMAGE=$(IMAGE):$(TAG) TAG=$(TAG) DOMAIN=$(DOMAIN) envsubst '$${IMAGE},$${TAG},$${DOMAIN}' < deployment.yaml | kubectl apply -f -
	kubectl apply -f service.yaml
	kubectl apply -f reference-grant.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < httproute.yaml | kubectl apply -f -

## delete: Tear down the otel-demo namespace and all resources in it.
delete:
	kubectl delete namespace $(NS) --ignore-not-found
