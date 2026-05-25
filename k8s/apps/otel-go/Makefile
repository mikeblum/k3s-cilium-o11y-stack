IMAGE  ?= ghcr.io/mblum/otel-go
TAG    ?= latest
NS     := otel-demo
DOMAIN ?= example.local

APP_DIR := ../../../apps/otel-go

.PHONY: build push deploy delete

## build: Build the Docker image locally.
build:
	docker build -t $(IMAGE):$(TAG) $(APP_DIR)

## push: Build and push the Docker image.
push: build
	docker push $(IMAGE):$(TAG)

## deploy: Deploy the workload and wire it into the o11y stack.
##
##   IMAGE=ghcr.io/you/otel-go TAG=latest DOMAIN=example.local make deploy
##
## After deploying, hot-reload Alloy so it starts scraping otel-go metrics:
##
##   kubectl apply -f ../../o11y/manifests/alloy-configmap.yaml
##
deploy:
	kubectl apply -f namespace.yaml
	IMAGE=$(IMAGE):$(TAG) envsubst '$${IMAGE}' < deployment.yaml | kubectl apply -f -
	kubectl apply -f service.yaml
	kubectl apply -f reference-grant.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < httproute.yaml | kubectl apply -f -

## delete: Tear down the otel-demo namespace and all resources in it.
delete:
	kubectl delete namespace $(NS) --ignore-not-found
