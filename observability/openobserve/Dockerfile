FROM public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba AS upstream

FROM debian:trixie-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates iproute2 procps python3 socat util-linux \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /data --no-create-home openobserve \
    && mkdir -p /runtime/observability \
    && chown 10001:10001 /runtime/observability

COPY --from=upstream /openobserve /usr/local/bin/openobserve
COPY entrypoint.sh /usr/local/bin/zpr-observability-entrypoint
RUN chmod 0755 /usr/local/bin/zpr-observability-entrypoint
LABEL zpr.observability.runtime="local-v1"

ENV ZO_LOCAL_MODE=true ZO_DATA_DIR=/data ZO_HTTP_PORT=5080 \
    ZO_HTTP_ADDR=127.0.0.1 ZO_HTTP_IPV6_ENABLED=false \
    ZO_GRPC_ADDR=127.0.0.1 ZO_TELEMETRY=false \
    ZO_COMPACT_DATA_RETENTION_DAYS=30

ENTRYPOINT ["/usr/local/bin/zpr-observability-entrypoint"]