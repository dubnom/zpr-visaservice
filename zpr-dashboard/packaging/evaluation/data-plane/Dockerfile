# EVALUATION ONLY data-plane runtime: Visa Service + Valkey + one node + the VS
# adapter in Linux network namespaces. Binaries come from build.sh.
FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates iproute2 iputils-ping libpcap0.8t64 libssl3t64 procps socat valkey-server \
    && rm -rf /var/lib/apt/lists/*
COPY ph ph-cli vs vs-admin zplc /usr/local/bin/
COPY data-plane.sh /usr/local/bin/zpr-evaluation-data-plane
RUN chmod 0755 /usr/local/bin/ph /usr/local/bin/ph-cli /usr/local/bin/vs /usr/local/bin/vs-admin \
        /usr/local/bin/zplc /usr/local/bin/zpr-evaluation-data-plane
LABEL org.zpr.evaluation-data-plane="1"
ENTRYPOINT ["/usr/local/bin/zpr-evaluation-data-plane"]
