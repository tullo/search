FROM golang:1.14.2-alpine3.11 as build_stage
ENV CGO_ENABLED 0
ARG VCS_REF
ARG PACKAGE_NAME

# Create a location in the image for the source code.
RUN mkdir -p /app
WORKDIR /app

# Copy the module files
COPY go.* ./

# Copy the source code into the image.
COPY cmd cmd
COPY internal internal
COPY tls tls
COPY ui ui
COPY vendor vendor

# Build the search binary.
WORKDIR /app/cmd/${PACKAGE_NAME}
RUN go build -ldflags "-X main.build=${VCS_REF}"
# The linker sets 'var build' in main.go to the specified git revision
# See https://golang.org/cmd/link/ for supported linker flags


# Build production image with Go binaries based on Alpine.
FROM alpine:3.11
ARG BUILD_DATE
ARG VCS_REF
ARG PACKAGE_NAME
COPY --from=build_stage /app/cmd/${PACKAGE_NAME}/${PACKAGE_NAME} /app/main
COPY --from=build_stage /app/ui /app/ui
COPY --from=build_stage /app/tls /app/tls
WORKDIR /app
CMD ["/app/main"]

LABEL org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.title="${PACKAGE_NAME}" \
      org.opencontainers.image.authors="Andreas <tullo@pm.me>" \
      org.opencontainers.image.source="https://github.com/tullo/search/cmd/${PACKAGE_NAME}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.vendor="Amstutz-IT"
