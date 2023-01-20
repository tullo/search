FROM golang:1.16.2-alpine3.13 as build_stage
ENV CGO_ENABLED 0
ARG VCS_REF

# Create a location in the image for the source code.
RUN mkdir -p /app
WORKDIR /app

# Copy all files not filtered out via .dockerignore
COPY . .

# Build the search binary.
WORKDIR /app/cmd/search
RUN go build -ldflags "-X main.build=${VCS_REF}" -mod=vendor
# The linker sets 'var build' in main.go to the specified git revision
# See https://golang.org/cmd/link/ for supported linker flags


# Build production image with Go binary, ui and tls.
FROM alpine:3.17.1
ARG BUILD_DATE
ARG VCS_REF
RUN addgroup -g 3000 -S app && adduser -u 100000 -S app -G app --no-create-home --disabled-password
USER 100000
WORKDIR /app
COPY --from=build_stage --chown=app:app /app/cmd/search/search /app/search
COPY --from=build_stage --chown=app:app /app/ui /app/ui
COPY --from=build_stage --chown=app:app /app/tls /app/tls
CMD ["/app/search"]

LABEL org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.title="search" \
      org.opencontainers.image.authors="Andreas <tullo@pm.me>" \
      org.opencontainers.image.source="https://github.com/tullo/search/tree/master/cmd/search" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.vendor="Amstutz-IT"
