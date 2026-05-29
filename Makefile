IMAGE_REPO ?= quay.io/rhpds/cert-manager-webhook-ibmcis
IMAGE_TAG  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: test build lint docker-build helm-template helm-lint

test:
	go test ./... -v -count=1

build:
	CGO_ENABLED=0 go build -o webhook ./cmd/webhook

lint:
	go vet ./...

docker-build:
	docker build -t "$(IMAGE_REPO):$(IMAGE_TAG)" .

helm-template:
	helm template cert-manager-webhook-ibmcis helm/ --set deploy=true

helm-lint:
	helm lint helm/
