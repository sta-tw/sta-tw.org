#!/bin/sh
set -eu

MAIL_HOSTNAME="${MAIL_HOSTNAME:-mail.sta-tw.org}"
MAIL_DOMAIN="${MAIL_DOMAIN:-sta-tw.org}"

sed -e "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" -e "s|__MAIL_DOMAIN__|${MAIL_DOMAIN}|g" \
    /etc/postfix/main.cf.template > /etc/postfix/main.cf
sed -e "s|__MAIL_HOSTNAME__|${MAIL_HOSTNAME}|g" -e "s|__MAIL_DOMAIN__|${MAIL_DOMAIN}|g" \
    /etc/opendkim.conf.template > /etc/opendkim.conf

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
    # Signed as d=${MAIL_HOSTNAME} (matches the sending/HELO/PTR domain), not
    # MAIL_DOMAIN, so it has to be published under that host in DNS.
    opendkim-genkey -b 2048 -d "${MAIL_HOSTNAME}" -s mail -D /etc/opendkim/keys
    echo "============================================================"
    echo "New DKIM key generated. Add this DNS TXT record:"
    echo "  Host: mail._domainkey.${MAIL_HOSTNAME}"
    cat /etc/opendkim/keys/mail.txt
    echo "============================================================"
fi
chown root:root /etc/opendkim/keys/mail.private
chmod 600 /etc/opendkim/keys/mail.private

mkdir -p /run/opendkim
newaliases || true
postfix set-permissions || true

# Inbound intake: account@${MAIL_HOSTNAME} pipes the raw message to the API.
sed "s|__STA_ACCOUNT_APPLICATION_MAIL_TOKEN__|${STA_ACCOUNT_APPLICATION_MAIL_TOKEN:-}|g" \
    /etc/postfix/intake.sh.template > /usr/local/bin/intake.sh
chmod 755 /usr/local/bin/intake.sh

# Used as local_recipient_maps: an existence check, not an alias rewrite —
# the RHS value is irrelevant, only presence of the key matters. See the
# comment on local_recipient_maps in main.cf.template for why this isn't
# virtual_alias_maps.
printf 'account@%s OK\n' "${MAIL_HOSTNAME}" > /etc/postfix/virtual
postmap /etc/postfix/virtual
printf 'account@%s intake:\n' "${MAIL_HOSTNAME}" > /etc/postfix/transport
postmap /etc/postfix/transport

if ! grep -q '^intake ' /etc/postfix/master.cf; then
    cat >> /etc/postfix/master.cf <<'EOF'
intake    unix  -       n       n       -       -       pipe
  flags=q user=nobody argv=/usr/local/bin/intake.sh
EOF
fi

opendkim -x /etc/opendkim.conf &

postfix start-fg
