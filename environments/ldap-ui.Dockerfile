FROM ubuntu:26.04

ARG PHPLDAPADMIN_VERSION=1.2.6.7-4
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        "phpldapadmin=${PHPLDAPADMIN_VERSION}" \
        php-cli \
        php-ldap \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 zpr-ldap-ui \
    && useradd --system --uid 10001 --gid 10001 --no-create-home zpr-ldap-ui

COPY --chmod=0555 ldap-ui-entrypoint.sh /usr/local/bin/ldap-ui-entrypoint

USER 10001:10001
ENTRYPOINT ["/usr/local/bin/ldap-ui-entrypoint"]
