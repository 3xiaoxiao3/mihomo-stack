# syntax=docker/dockerfile:1.7

ARG ALPINE_VERSION=3.21
ARG GO_VERSION=1.24
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

FROM alpine:${ALPINE_VERSION} AS mihomo-download
ARG TARGETARCH
ARG TARGETVARIANT
ARG MIHOMO_VERSION=v1.19.30
ARG MIHOMO_SHA_AMD64=cbe553d0319a414bd3a372c5976a252155b2c4882b66bce88a4d6bba9571a553
ARG MIHOMO_SHA_ARM64=58896873736d28628f66de3677c8654fa0f180662523148e136cff4f6e890069
ARG MIHOMO_SHA_ARMV7=7b4e8793f9f8fa4a21ea66528890ebde5a73d557673fb94d3047eebb60ad7591
RUN apk add --no-cache ca-certificates wget \
    && case "${TARGETARCH}/${TARGETVARIANT}" in \
         amd64/) asset="mihomo-linux-amd64-v1-${MIHOMO_VERSION}.gz"; sha="${MIHOMO_SHA_AMD64}" ;; \
         arm64/) asset="mihomo-linux-arm64-${MIHOMO_VERSION}.gz"; sha="${MIHOMO_SHA_ARM64}" ;; \
         arm/v7) asset="mihomo-linux-armv7-${MIHOMO_VERSION}.gz"; sha="${MIHOMO_SHA_ARMV7}" ;; \
         *) echo "unsupported platform ${TARGETARCH}/${TARGETVARIANT}" >&2; exit 1 ;; \
       esac \
    && wget -q -O /tmp/mihomo.gz "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/${asset}" \
    && echo "${sha}  /tmp/mihomo.gz" | sha256sum -c - \
    && gzip -d /tmp/mihomo.gz \
    && install -m 0755 /tmp/mihomo /usr/local/bin/mihomo

FROM alpine:${ALPINE_VERSION} AS dashboard-download
ARG METACUBEXD_VERSION=v1.273.0
ARG METACUBEXD_SHA256=076e05d2e3dc6641a0ec281aa4b97a18193fbcc379d139762c32d90adb22793c
RUN apk add --no-cache ca-certificates wget \
    && wget -q -O /tmp/dashboard.tgz "https://github.com/MetaCubeX/metacubexd/releases/download/${METACUBEXD_VERSION}/compressed-dist.tgz" \
    && echo "${METACUBEXD_SHA256}  /tmp/dashboard.tgz" | sha256sum -c - \
    && mkdir -p /out/dashboard \
    && tar -xzf /tmp/dashboard.tgz -C /out/dashboard

FROM alpine:${ALPINE_VERSION} AS license-download
ARG GPL3_SHA256=3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986
RUN apk add --no-cache ca-certificates wget \
    && wget -q -O /GPL-3.0.txt https://www.gnu.org/licenses/gpl-3.0.txt \
    && echo "${GPL3_SHA256}  /GPL-3.0.txt" | sha256sum -c -

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
COPY --from=mihomo-download /usr/local/bin/mihomo /usr/local/bin/mihomo
COPY --from=web-build /src/web/dist/ /app/web/
COPY --from=dashboard-download /out/dashboard/ /app/dashboard/
COPY --from=license-download /GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
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
COPY --from=mihomo-download /usr/local/bin/mihomo /usr/local/bin/mihomo
COPY --from=dashboard-download /out/dashboard/ /app/dashboard/
COPY --from=license-download /GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
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
COPY --from=license-download /GPL-3.0.txt /usr/share/licenses/GPL-3.0.txt
COPY THIRD_PARTY.md /usr/share/doc/mihomo-stack/THIRD_PARTY.md
USER 10002:10002
WORKDIR /app/subconverter
EXPOSE 25500
ENTRYPOINT ["/app/subconverter/subconverter"]
