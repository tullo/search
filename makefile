SHELL = /bin/bash -o pipefail

export PROJECT = tullo-starter-kit
export REGISTRY_HOSTNAME = docker.io
export REGISTRY_ACCOUNT = tullo
export VERSION = 0.1.0
export DOCKER_BUILDKIT = 1
export SALES_URL = http://0.0.0.0:3000/v1
export SESSION_SECRET := $(shell openssl rand -base64 32)

.DEFAULT_GOAL := config

all: search test-cover-profile test-cover-text

go-run:
	go run ./cmd/search --web-enable-tls=true

search:
	docker build \
		-f deploy/Dockerfile \
		-t $(REGISTRY_HOSTNAME)/$(REGISTRY_ACCOUNT)/search-amd64:$(VERSION) \
		--build-arg PACKAGE_NAME=search \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.
	docker image tag \
		$(REGISTRY_ACCOUNT)/search-amd64:$(VERSION) \
		gcr.io/$(PROJECT)/search-amd64:$(VERSION)

okteto-build:
	okteto build \
		-f deploy/Dockerfile \
		-t registry.cloud.okteto.net/$(REGISTRY_ACCOUNT)/search-app-amd64:$(VERSION) \
		--build-arg PACKAGE_NAME=search \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.

config:
	docker-compose -f deploy/docker-compose.yml config

up:
	docker-compose -f deploy/docker-compose.yml up --remove-orphans

down:
	docker-compose -f deploy/docker-compose.yml down

test:
	go test -count=1 -failfast -test.timeout=30s ./...

test-cover-profile:
	go test -test.timeout=30s -coverprofile=/tmp/profile.out ./...

test-cover-text:
	go tool cover -func=/tmp/profile.out

test-cover-html:
	go tool cover -html=/tmp/profile.out

stop-all:
	docker container stop $$(docker container ls -q --filter name=search)

remove-all:
	docker container rm $$(docker container ls -aq --filter "name=search")

tidy:
	go mod tidy
	go mod vendor

deps-upgrade:
	go get -d -t -u -v ./...
#   -d flag ...download the source code needed to build ...
#   -t flag ...consider modules needed to build tests ...
#   -u flag ...use newer minor or patch releases when available 

deps-cleancache:
	go clean -modcache

dry-run:
	kubectl apply --dry-run=client -f .deploy/k8s/deploy-search-app.yaml -o yaml

deployment:
	kubectl apply -f ./deploy/k8s/deploy-search-app.yaml
	@echo
	watch kubectl get pod,svc
#	kubectl logs --tail=20 -f deployment/search-app --container search-app

delete:
	kubectl delete -f ./deploy/k8s/deploy-search-app.yaml
	@echo
	watch kubectl get pod,svc

#rollout:
#	kubectl rollout restart deployment/search-app
#	kubectl exec -it pod/search-app-644654fddb-nsl9h -- env

ping:
	curl -k -H "X-Probe: LivenessProbe" https://0.0.0.0:4200/ping; echo

# inspect okteto secrets
secrets:
	kubectl get secrets/okteto-secrets -o json | jq -r .data[\"SEARCH_WEB_SESSION_SECRET\"] | base64 -d; echo
