ARG DOCKERHUB_MIRROR=docker.m.daocloud.io
ARG ALPINE_REPO=https://mirrors.aliyun.com/alpine

FROM --platform=$BUILDPLATFORM ${DOCKERHUB_MIRROR}/library/node:22-alpine AS frontend
WORKDIR /frontend-build

ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    npm_config_registry=$NPM_REGISTRY \
    COREPACK_NPM_REGISTRY=$NPM_REGISTRY

RUN corepack enable

COPY web/package.json web/pnpm-lock.yaml ./web/
WORKDIR /frontend-build/web
RUN --mount=type=cache,target=/pnpm/store \
    pnpm install --frozen-lockfile

WORKDIR /frontend-build
COPY web ./web
RUN cd web && pnpm release

FROM --platform=$BUILDPLATFORM ${DOCKERHUB_MIRROR}/library/golang:1.25.7-alpine AS backend
WORKDIR /backend-build

ARG ALPINE_REPO=https://mirrors.aliyun.com/alpine

# Install build dependencies
RUN sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_REPO}#g" /etc/apk/repositories && \
    apk add --no-cache git ca-certificates

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=$GOPROXY \
    GOSUMDB=$GOSUMDB

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code (use .dockerignore to exclude unnecessary files)
COPY . .
COPY --from=frontend /frontend-build/server/router/frontend/dist ./server/router/frontend/dist

ARG TARGETOS TARGETARCH VERSION COMMIT
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
      -trimpath \
      -ldflags="-s -w -extldflags '-static'" \
      -tags netgo,osusergo \
      -o memos \
      ./cmd/memos

# Use minimal Alpine with security updates
FROM ${DOCKERHUB_MIRROR}/library/alpine:3.21 AS monolithic

ARG ALPINE_REPO=https://mirrors.aliyun.com/alpine

# Install runtime dependencies and create non-root user in single layer
RUN sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_REPO}#g" /etc/apk/repositories && \
    apk add --no-cache tzdata ca-certificates su-exec && \
    addgroup -g 10001 -S nonroot && \
    adduser -u 10001 -S -G nonroot -h /var/opt/memos nonroot && \
    mkdir -p /var/opt/memos /usr/local/memos && \
    chown -R nonroot:nonroot /var/opt/memos

# Copy binary and entrypoint to /usr/local/memos
COPY --from=backend /backend-build/memos /usr/local/memos/memos
COPY --from=backend --chmod=755 /backend-build/scripts/entrypoint.sh /usr/local/memos/entrypoint.sh

# Run as root to fix permissions, entrypoint will drop to nonroot
USER root

# Set working directory to the writable volume
WORKDIR /var/opt/memos

# Data directory
VOLUME /var/opt/memos

ENV TZ="UTC" \
    MEMOS_PORT="5230"

EXPOSE 5230

ENTRYPOINT ["/usr/local/memos/entrypoint.sh", "/usr/local/memos/memos"]
