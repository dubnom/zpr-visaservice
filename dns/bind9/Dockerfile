FROM debian:trixie-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends bind9 bind9-utils dnsutils iproute2 util-linux procps \
    && rm -rf /var/lib/apt/lists/*

COPY named.conf.example /etc/bind/named.conf
COPY entrypoint.sh /usr/local/bin/zpr-bind-entrypoint
RUN chmod 0755 /usr/local/bin/zpr-bind-entrypoint

ENTRYPOINT ["/usr/local/bin/zpr-bind-entrypoint"]
