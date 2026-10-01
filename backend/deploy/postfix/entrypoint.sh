#!/bin/sh
set -eu

MAIL_HOSTNAME="${MAIL_HOSTNAME:-mail.sta-tw.org}"
MAIL_DOMAIN="${MAIL_DOMAIN:-sta-tw.org}"

sed -e "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" -e "s|__MAIL_DOMAIN__|${MAIL_DOMAIN}|g" \
    /etc/postfix/main.cf.template > /etc/postfix/main.cf
sed -e "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" -e "s|__MAIL_DOMAIN__|${MAIL_DOMAIN}|g" \
    /etc/opendkim.conf.template > /etc/opendkim.conf

for name in local-recipients transport; do
    sed -e "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" \
        -e "s|__STA_POSTGRES_USER__|${STA_POSTGRES_USER:-sta}|g" \
        -e "s|__STA_POSTGRES_PASSWORD__|${STA_POSTGRES_PASSWORD:-sta}|g" \
        -e "s|__STA_POSTGRES_DB__|${STA_POSTGRES_DB:-sta}|g" \
        "/etc/postfix/pgsql-${name}.cf.template" > "/etc/postfix/pgsql-${name}.cf"
    chmod 600 "/etc/postfix/pgsql-${name}.cf"
done

# Static hash: maps (see main.cf.template) — just noreply@ for now, so its
# own bounce-of-a-bounce doesn't double-bounce. hash: maps need postmap's
# .db built explicitly, unlike the live pgsql: maps above.
for name in local-recipients-extra transport-extra; do
    sed "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" \
        "/etc/postfix/${name}.template" > "/etc/postfix/${name}"
    postmap "hash:/etc/postfix/${name}"
done

mkdir -p /etc/postfix/tls
CADDY_CERT="/caddy-data/caddy/certificates/acme-v02.api.letsencrypt.org-directory/${MAIL_HOSTNAME}/${MAIL_HOSTNAME}.crt"
CADDY_KEY="/caddy-data/caddy/certificates/acme-v02.api.letsencrypt.org-directory/${MAIL_HOSTNAME}/${MAIL_HOSTNAME}.key"
if [ -f "$CADDY_CERT" ] && [ -f "$CADDY_KEY" ]; then
    echo "Using Let's Encrypt cert for ${MAIL_HOSTNAME} from Caddy's storage."
    cp "$CADDY_CERT" /etc/postfix/tls/cert.pem
    cp "$CADDY_KEY" /etc/postfix/tls/key.pem
elif [ ! -f /etc/postfix/tls/cert.pem ]; then
    echo "No Let's Encrypt cert found yet for ${MAIL_HOSTNAME}; using a self-signed cert."
    openssl req -x509 -nodes -newkey rsa:2048 -days 3650 \
        -keyout /etc/postfix/tls/key.pem -out /etc/postfix/tls/cert.pem \
        -subj "/CN=${MAIL_HOSTNAME}"
fi
chmod 600 /etc/postfix/tls/key.pem

mkdir -p /etc/opendkim/keys
if [ ! -f /etc/opendkim/keys/mail.private ]; then
    # Signed as d=${MAIL_DOMAIN} — outbound mail's From/envelope domain is
    # MAIL_DOMAIN (e.g. account@sta-tw.org), not MAIL_HOSTNAME (the SMTP
    # server's own hostname). Without a SigningTable, OpenDKIM's Domain
    # setting also acts as the sender-domain allowlist: a mismatch here
    # means it silently skips signing entirely, not just misalignment.
    opendkim-genkey -b 2048 -d "${MAIL_DOMAIN}" -s mail -D /etc/opendkim/keys
    echo "============================================================"
    echo "New DKIM key generated. Add this DNS TXT record:"
    echo "  Host: mail._domainkey.${MAIL_DOMAIN}"
    cat /etc/opendkim/keys/mail.txt
    echo "============================================================"
fi
chown root:root /etc/opendkim/keys/mail.private
chmod 600 /etc/opendkim/keys/mail.private

mkdir -p /run/opendkim
newaliases || true
postfix set-permissions || true

# Inbound intake: any address registered in mail_routes (see
# pgsql-local-recipients.cf.template) pipes the raw message to the API,
# along with the envelope recipient so the API knows which mail_routes row
# matched (${recipient} is Postfix's pipe(8) transport substitution, not a
# shell variable — expanded by master.cf, not here).
sed "s|__STA_ACCOUNT_APPLICATION_MAIL_TOKEN__|${STA_ACCOUNT_APPLICATION_MAIL_TOKEN:-}|g" \
    /etc/postfix/intake.sh.template > /usr/local/bin/intake.sh
chmod 755 /usr/local/bin/intake.sh

if ! grep -q '^intake ' /etc/postfix/master.cf; then
    cat >> /etc/postfix/master.cf <<'EOF'
intake    unix  -       n       n       -       -       pipe
  flags=q user=nobody argv=/usr/local/bin/intake.sh ${recipient}
EOF
fi

opendkim -x /etc/opendkim.conf &

postfix start-fg
