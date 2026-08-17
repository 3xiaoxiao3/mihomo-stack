# syntax=docker/dockerfile:1.7

ARG ALPINE_VERSION=3.21
ARG GO_VERSION=1.26
ARG MIHOMO_GO_VERSION=1.26
ARG NODE_VERSION=22

FROM node:${NODE_VERSION}-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
RUN npm run build

FROM golang:${GO_VERSION}-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/mihomo-guardian ./cmd/mihomo-guardian

FROM --platform=$BUILDPLATFORM golang:${MIHOMO_GO_VERSION}-alpine AS mihomo-build
ARG TARGETARCH
ARG TARGETVARIANT
ARG MIHOMO_VERSION=v1.19.30
ARG MIHOMO_COMMIT=ac017cdd246ce8bd547653d927e7bf77d7ee73d5
ARG MIHOMO_X_CRYPTO_VERSION=v0.53.0
ARG MIHOMO_X_NET_VERSION=v0.56.0
ARG MIHOMO_X_TEXT_VERSION=v0.39.0
ARG MIHOMO_BUILD_VERSION=v1.19.30-stack.2
ARG MIHOMO_BUILD_TIME=2026-08-17T00:00:00Z
RUN apk add --no-cache ca-certificates git \
    && git clone --branch "${MIHOMO_VERSION}" --depth 1 \
      https://github.com/MetaCubeX/mihomo.git /src/mihomo \
    && test "$(git -C /src/mihomo rev-parse HEAD)" = "${MIHOMO_COMMIT}"
WORKDIR /src/mihomo
RUN --mount=type=cache,target=/go/pkg/mod \
    go get \
      "golang.org/x/crypto@${MIHOMO_X_CRYPTO_VERSION}" \
      "golang.org/x/net@${MIHOMO_X_NET_VERSION}" \
      "golang.org/x/text@${MIHOMO_X_TEXT_VERSION}" \
    && go mod tidy \
    && test "$(go list -m -f '{{.Version}}' golang.org/x/crypto)" = "${MIHOMO_X_CRYPTO_VERSION}" \
    && test "$(go list -m -f '{{.Version}}' golang.org/x/net)" = "${MIHOMO_X_NET_VERSION}" \
    && test "$(go list -m -f '{{.Version}}' golang.org/x/text)" = "${MIHOMO_X_TEXT_VERSION}"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cp -f /etc/ssl/certs/ca-certificates.crt component/ca/ca-certificates.crt \
    && case "${TARGETARCH}/${TARGETVARIANT}" in \
         amd64/) export GOARCH=amd64 GOAMD64=v1 ;; \
         arm64/) export GOARCH=arm64 ;; \
         arm/v7) export GOARCH=arm GOARM=7 ;; \
         *) echo "unsupported platform ${TARGETARCH}/${TARGETVARIANT}" >&2; exit 1 ;; \
       esac \
    && CGO_ENABLED=0 GOOS=linux go build -tags with_gvisor -trimpath \
      -ldflags="-s -w -buildid= -X github.com/metacubex/mihomo/constant.Version=${MIHOMO_BUILD_VERSION} -X github.com/metacubex/mihomo/constant.BuildTime=${MIHOMO_BUILD_TIME}" \
      -o /out/mihomo

FROM alpine:${ALPINE_VERSION} AS dashboard-download
ARG METACUBEXD_VERSION=v1.273.0
ARG METACUBEXD_SHA256=076e05d2e3dc6641a0ec281aa4b97a18193fbcc379d139762c32d90adb22793c
RUN apk add --no-cache ca-certificates wget \
    && wget -q -O /tmp/dashboard.tgz "https://github.com/MetaCubeX/metacubexd/releases/download/${METACUBEXD_VERSION}/compressed-dist.tgz" \
    && echo "${METACUBEXD_SHA256}  /tmp/dashboard.tgz" | sha256sum -c - \
    && mkdir -p /out/dashboard \
    && tar -xzf /tmp/dashboard.tgz -C /out/dashboard

FROM alpine:${ALPINE_VERSION} AS subconverter-download
ARG TARGETARCH
ARG TARGETVARIANT
ARG SUBCONVERTER_VERSION=v0.9.0
ARG SUBCONVERTER_SHA_AMD64=884a6d1168267eba076fcdd5171215bacf98c17948ab526e4cbbdcad5f7a0217
ARG SUBCONVERTER_SHA_ARM64=0914688a0af211360271a4eef8a731f09852b47edf094d3758070b660544659e
ARG SUBCONVERTER_SHA_ARMV7=fd1e6f41616be6948fd988b46c3de81ac7c70bf7470d9b029f9e163c86cdb50f
RUN apk add --no-cache ca-certificates wget \
    && case "${TARGETARCH}/${TARGETVARIANT}" in \
         amd64/) asset="subconverter_linux64.tar.gz"; sha="${SUBCONVERTER_SHA_AMD64}" ;; \
         arm64/) asset="subconverter_aarch64.tar.gz"; sha="${SUBCONVERTER_SHA_ARM64}" ;; \
         arm/v7) asset="subconverter_armv7.tar.gz"; sha="${SUBCONVERTER_SHA_ARMV7}" ;; \
         *) echo "unsupported platform ${TARGETARCH}/${TARGETVARIANT}" >&2; exit 1 ;; \
       esac \
    && wget -q -O /tmp/subconverter.tar.gz "https://github.com/tindy2013/subconverter/releases/download/${SUBCONVERTER_VERSION}/${asset}" \
    && echo "${sha}  /tmp/subconverter.tar.gz" | sha256sum -c - \
    && mkdir -p /out \
    && tar -xzf /tmp/subconverter.tar.gz -C /out \
    && chmod 0755 /out/subconverter/subconverter

FROM alpine:${ALPINE_VERSION} AS guardian
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Mihomo Stack" \
      org.opencontainers.image.description="Complete Mihomo stack with a transactional configuration control plane" \
      org.opencontainers.image.source="https://github.com/3xiaoxiao3/mihomo-stack" \
      org.opencontainers.image.licenses="MIT AND GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"
RUN apk add --no-cache ca-certificates su-exec tzdata \
    && addgroup -S -g 10001 guardian \
    && adduser -S -D -H -u 10001 -G guardian guardian \
    && mkdir -p /app/web /app/dashboard /data \
    && chown -R guardian:guardian /app /data
COPY --from=go-build /out/mihomo-guardian /usr/local/bin/mihomo-guardian
COPY --from=mihomo-build /out/mihomo /usr/local/bin/mihomo
COPY --from=web-build /src/web/dist/ /app/web/
COPY --from=dashboard-download /out/dashboard/ /app/dashboard/
COPY licenses/GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
COPY LICENSE THIRD_PARTY.md /usr/share/doc/mihomo-stack/
COPY --chown=guardian:guardian config.example.yaml /app/config.yaml
COPY --chmod=0755 deploy/guardian-entrypoint.sh /usr/local/bin/guardian-entrypoint.sh
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/v1/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/guardian-entrypoint.sh"]
CMD ["serve", "-config", "/app/config.yaml"]

FROM alpine:${ALPINE_VERSION} AS mihomo-runtime
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Mihomo Stack Mihomo Runtime" \
      org.opencontainers.image.source="https://github.com/3xiaoxiao3/mihomo-stack" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"
RUN apk add --no-cache ca-certificates su-exec tzdata \
    && addgroup -S -g 10001 guardian \
    && adduser -S -D -H -u 10001 -G guardian guardian \
    && mkdir -p /app/dashboard /data \
    && chown -R guardian:guardian /app /data
COPY --from=mihomo-build /out/mihomo /usr/local/bin/mihomo
COPY --from=dashboard-download /out/dashboard/ /app/dashboard/
COPY licenses/GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
COPY THIRD_PARTY.md /usr/share/doc/mihomo-stack/THIRD_PARTY.md
COPY --chmod=0755 deploy/mihomo-entrypoint.sh deploy/mihomo-healthcheck.sh /usr/local/bin/
EXPOSE 7890 9090
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/mihomo-healthcheck.sh"]
ENTRYPOINT ["/usr/local/bin/mihomo-entrypoint.sh"]

FROM alpine:${ALPINE_VERSION} AS subconverter-runtime
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Mihomo Stack Subconverter Runtime" \
      org.opencontainers.image.source="https://github.com/3xiaoxiao3/mihomo-stack" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10002 converter \
    && adduser -S -D -H -u 10002 -G converter converter
COPY --from=subconverter-download --chown=converter:converter /out/subconverter /app/subconverter
COPY licenses/GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
COPY THIRD_PARTY.md /usr/share/doc/mihomo-stack/THIRD_PARTY.md
USER 10002:10002
WORKDIR /app/subconverter
EXPOSE 25500
ENTRYPOINT ["/app/subconverter/subconverter"]
