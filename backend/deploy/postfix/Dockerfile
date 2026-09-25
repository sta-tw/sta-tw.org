FROM debian:trixie-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        postfix opendkim opendkim-tools openssl libsasl2-modules ca-certificates curl && \
    rm -rf /var/lib/apt/lists/* && \
    rm -f /etc/postfix/main.cf

COPY main.cf.template /etc/postfix/main.cf.template
COPY intake.sh.template /etc/postfix/intake.sh.template
COPY opendkim.conf.template /etc/opendkim.conf.template
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 25

ENTRYPOINT ["/entrypoint.sh"]
