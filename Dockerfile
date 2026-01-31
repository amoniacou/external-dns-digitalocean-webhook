FROM gcr.io/distroless/static:nonroot AS production

ARG VERSION="dev"
ENV VERSION=${VERSION}

ARG TARGETARCH

LABEL org.opencontainers.image.title="external-dns-digitalocean-webhook" \
      org.opencontainers.image.description="DigitalOcean Webhook for External-DNS" \
      org.opencontainers.image.vendor="Amoniac OÜ" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.source="https://github.com/amoniacou/external-dns-digitalocean-webhook" \
      org.opencontainers.image.authors="Amoniac OÜ"

WORKDIR /

USER nonroot:nonroot

COPY --chown=nonroot:nonroot --chmod=0755 ./dist/external-dns-digitalocean-webhook_linux_${TARGETARCH}*/webhook .

ENTRYPOINT ["/webhook"]