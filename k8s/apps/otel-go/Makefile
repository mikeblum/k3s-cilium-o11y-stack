# LOCAL_IMAGE: used by `make load` — imported directly into k3s containerd.
# The localhost/ prefix tells containerd this image is never fetched from a remote registry.
LOCAL_IMAGE := localhost/otel-go

# REMOTE_IMAGE: used by `make push` — override on the command line.
#   make push REMOTE_IMAGE=ghcr.io/you/otel-go
REMOTE_IMAGE ?= ghcr.io/mblum/otel-go

# Tag is always the short git SHA — never use 'latest'.
TAG    := $(shell git rev-parse --short HEAD)
NS     := otel-demo
DOMAIN ?= example.local

APP_DIR := ../../../apps/otel-go

.PHONY: build load push deploy delete

## build: Build the image locally as localhost/otel-go:<sha>.
build:
	docker build -t $(LOCAL_IMAGE):$(TAG) $(APP_DIR)

## load: Build and import directly into k3s containerd. No registry involved.
##
##   make load
##   sudo k3s ctr images ls | grep otel-go    # verify
##   make deploy                               # deploy using the local image
##
load: build
	docker save $(LOCAL_IMAGE):$(TAG) | sudo k3s ctr images import -

## push: Tag and push to ghcr.io. Requires `docker login ghcr.io` first.
##
##   echo $$GITHUB_TOKEN | docker login ghcr.io -u <user> --password-stdin
##   make push REMOTE_IMAGE=ghcr.io/you/otel-go
##   make deploy IMAGE=ghcr.io/you/otel-go
##
push: build
	docker tag $(LOCAL_IMAGE):$(TAG) $(REMOTE_IMAGE):$(TAG)
	docker push $(REMOTE_IMAGE):$(TAG)

## deploy: Apply k8s manifests. IMAGE defaults to the local image (after make load).
##         Override IMAGE for a registry-sourced image (after make push).
##
##   make deploy                                        # local image (make load path)
##   make deploy IMAGE=ghcr.io/you/otel-go              # registry image (make push path)
##   make deploy DOMAIN=mylab.local
##
## Hot-reload Alloy after first deploy:
##   kubectl apply -f ../../o11y/manifests/alloy-configmap.yaml
##
IMAGE  ?= $(LOCAL_IMAGE)
deploy:
	kubectl apply -f namespace.yaml
	IMAGE=$(IMAGE):$(TAG) TAG=$(TAG) DOMAIN=$(DOMAIN) envsubst '$${IMAGE},$${TAG},$${DOMAIN}' < deployment.yaml | kubectl apply -f -
	kubectl apply -f service.yaml
	kubectl apply -f reference-grant.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < httproute.yaml | kubectl apply -f -

## delete: Tear down the otel-demo namespace and all resources in it.
delete:
	kubectl delete namespace $(NS) --ignore-not-found
