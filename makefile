#!make
SHELL = /bin/bash -o pipefail

export VERSION = 0.1.0
PROJECT = stackwise-starter-kit
REGISTRY_HOSTNAME = docker.io
export REGISTRY_ACCOUNT = tullo
CONTAINER_REGISTRY = eu.gcr.io
DOCKER_BUILDKIT = 1
SALES_URL = http://0.0.0.0:3000/v1
export SESSION_SECRET := $(shell openssl rand -base64 32)

.DEFAULT_GOAL := config

all: docker-build-search go-test-cover-profile go-test-cover-text

go-config:
	@go run ./cmd/search --help

go-run:
	go run ./cmd/search \
		--web-debug-mode=true --web-enable-tls=true \
		--web-session-secret=${SESSION_SECRET}
		--zipkin-reporter-uri=http://0.0.0.0:9411/api/v2/spans

docker-build-search:
	docker build \
		-f deploy/Dockerfile \
		-t $(REGISTRY_HOSTNAME)/$(REGISTRY_ACCOUNT)/search-app-amd64:$(VERSION) \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +"%Y-%m-%dT%H:%M:%SZ"` \
		.

docker-tag-gcr-image:
	set -e ; \
	docker image tag \
		$(REGISTRY_ACCOUNT)/search-app-amd64:$(VERSION) ${CONTAINER_REGISTRY}/$(PROJECT)/search-app-amd64:`git rev-parse HEAD`

docker-push-gcr-image:
	set -e ; \
	docker image push ${CONTAINER_REGISTRY}/$(PROJECT)/search-app-amd64:`git rev-parse HEAD`
	@echo '==>' listing tags for image: [$(CONTAINER_REGISTRY)/$(PROJECT)/search-app-amd64]:
	@gcloud container images list-tags $(CONTAINER_REGISTRY)/$(PROJECT)/search-app-amd64

okteto-build:
	okteto build \
		-f deploy/Dockerfile \
		-t registry.cloud.okteto.net/$(REGISTRY_ACCOUNT)/search-app-amd64:$(VERSION) \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +"%Y-%m-%dT%H:%M:%SZ"` \
		.

config:
	docker-compose -f deploy/docker-compose.yml config

up:
	docker-compose -f deploy/docker-compose.yml up --remove-orphans

down:
	docker-compose -f deploy/docker-compose.yml down

go-test:
	go test -count=1 -failfast -test.timeout=30s ./...

go-test-cover-profile:
	go test -test.timeout=30s -coverprofile=/tmp/profile.out ./...

# Display coverage percentages to stdout
go-test-cover-text:
	go tool cover -func=/tmp/profile.out

# 1. Writes out an HTML file instead of launching a web browser.
# 2. Uses Firefox to display the annotated source code.
# go tool cover -h
go-test-cover-html:
	go tool cover -html=/tmp/profile.out -o /tmp/coverage.html
	firefox /tmp/coverage.html

stop-all:
	docker container stop $$(docker container ls -q --filter name=search)

remove-all:
	docker container rm $$(docker container ls -aq --filter "name=search")

go-tidy:
	go mod tidy
	go mod vendor

go-deps-upgrade:
	go get -d -t -u -v ./...
#   -d flag ...download the source code needed to build ...
#   -t flag ...consider modules needed to build tests ...
#   -u flag ...use newer minor or patch releases when available 

go-deps-cleancache:
	go clean -modcache

kctl-dry-run:
	kubectl apply --dry-run=server -f ./deploy/k8s/gcp/deploy-search-app.yaml -o yaml --validate=true

kctl-deployment:
	kubectl apply -f ./deploy/k8s/gcp/deploy-search-app.yaml
	@echo
	@kubectl rollout status deployment/search-app --watch=true
	@echo
	@kubectl get pod,svc

kctl-delete:
	@kubectl delete -f ./deploy/k8s/gcp/deploy-search-app.yaml
	@echo
	watch kubectl get pod,svc

kctl-logs:
	@kubectl logs --tail=20 -f deployment/search-app --container search-app

kctl-rollout:
	@kubectl rollout status deployment/search-app
#	@kubectl rollout restart deployment/search-app
#	@kubectl exec -it pod/search-app-644654fddb-nsl9h -- env

ping:
	curl -k -H "X-Probe: LivenessProbe" https://0.0.0.0:4200/ping; echo

kctl-secret-get:
	kubectl get secrets/search-app -o json
#	kubectl get secrets/okteto-secrets -o json | jq -r .data[\"SEARCH_WEB_SESSION_SECRET\"] | base64 -d; echo

kctl-secret-create:
	kubectl create secret generic search-app --from-literal=session_secret=${SESSION_SECRET}

kctl-port-forward-app:
	set -e ; \
	POD=$$(kubectl get pod --selector="app=search-app" --output jsonpath='{.items[0].metadata.name}') ; \
	echo "===> kubectl port-forward $${POD} 8080:8080" ; \
	kubectl port-forward $${POD} 8080:8080

kctl-port-forward-argocd:
	@kubectl port-forward svc/argocd-server -n argocd 8080:443
