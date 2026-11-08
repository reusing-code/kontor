# CI image for the frontend/backend jobs. Versions come from backend/go.mod,
# engines.bun in frontend/package.json and .tool-versions; the ci-image job
# derives the image tag from them plus a hash of this file.
ARG GO_VERSION
ARG BUN_VERSION
ARG GOLANGCI_LINT_VERSION
ARG DOCKER_VERSION=29.9.3

FROM oven/bun:${BUN_VERSION} AS bun
FROM golangci/golangci-lint:v${GOLANGCI_LINT_VERSION} AS golangci-lint
FROM docker:${DOCKER_VERSION}-cli AS docker

FROM golang:${GO_VERSION}
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
COPY --from=golangci-lint /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker /usr/local/libexec/docker/cli-plugins/docker-buildx /usr/local/libexec/docker/cli-plugins/docker-buildx
ENV GOTOOLCHAIN=local
