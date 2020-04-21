# Search frontend to sales-api service

![Go](https://github.com/tullo/search/workflows/Go/badge.svg)

## Locale Go development

1. Run `make go-run` to start the webapp
1. Go to https://0.0.0.0:4200/ review product sales

## Build the docker image

`make search`

## Test the container using docker-compose

1. `make up`
1. `make down`

## Okteto deployment

1. `make okteto-build` builds and pushes the image.
1. `make deployment` applies k8s manifests to the cluster.
1. `make delete` deletes the deployment.
