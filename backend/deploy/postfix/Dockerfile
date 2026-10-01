FROM debian:trixie-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        postfix postfix-pgsql opendkim opendkim-tools openssl libsasl2-modules ca-certificates curl && \
    rm -rf /var/lib/apt/lists/* && \
    rm -f /etc/postfix/main.cf

COPY main.cf.template /etc/postfix/main.cf.template
COPY intake.sh.template /etc/postfix/intake.sh.template
COPY opendkim.conf.template /etc/opendkim.conf.template
COPY pgsql-local-recipients.cf.template /etc/postfix/pgsql-local-recipients.cf.template
COPY pgsql-transport.cf.template /etc/postfix/pgsql-transport.cf.template
COPY local-recipients-extra.template /etc/postfix/local-recipients-extra.template
COPY transport-extra.template /etc/postfix/transport-extra.template
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 25

ENTRYPOINT ["/entrypoint.sh"]
