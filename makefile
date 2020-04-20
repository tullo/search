SHELL := /bin/bash

export PROJECT = tullo-starter-kit
export REGISTRY_HOSTNAME = docker.io
export REGISTRY_ACCOUNT = tullo
export VERSION = 0.1.0
export DOCKER_BUILDKIT = 1

all: search test-cover-profile test-cover-text

go-run:
	go run ./cmd/search --db-disable-tls=1

search:
	docker build \
		-f Dockerfile \
		-t $(REGISTRY_HOSTNAME)/$(REGISTRY_ACCOUNT)/search-amd64:$(VERSION) \
		--build-arg PACKAGE_NAME=search \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.
	docker image tag \
		$(REGISTRY_ACCOUNT)/search-amd64:$(VERSION) \
		gcr.io/$(PROJECT)/search-amd64:$(VERSION)

up:
	docker-compose up --remove-orphans

down:
	docker-compose down

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
