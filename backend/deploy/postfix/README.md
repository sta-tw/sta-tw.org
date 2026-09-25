# Self-hosted outbound mail (Postfix + OpenDKIM)

Direct-delivery Postfix (no smarthost — it resolves MX records and delivers
straight to the recipient's mail server) with OpenDKIM signing. It is not
exposed outside the docker compose network; `notification-worker` is the only
client, connecting to `mail.sta-tw.org:25` (a compose network alias for this
service — matches the cert's hostname) with STARTTLS.

The container reuses Caddy's Let's Encrypt cert for `STA_MAIL_HOSTNAME`
(mounted read-only from the shared `caddy-data` volume) instead of a
self-signed one, so STARTTLS passes normal certificate verification. Caddy
only obtains that cert because of the placeholder site block for it in the
Caddyfile.

This only works because the host's public IP has a matching PTR record for
`STA_MAIL_HOSTNAME` (`mail.sta-tw.org` by default) — Postfix uses that same
name as its HELO/EHLO identity, and mail providers check that PTR(ip) ==
HELO name before trusting the connection.

## First boot: DKIM key

On first start the container generates a 2048-bit DKIM keypair (persisted in
the `postfix-dkim` volume, so it survives rebuilds) and prints the DNS TXT
record to add:

```sh
docker compose logs postfix | grep -A5 "New DKIM key generated"
```

Add it in Cloudflare DNS as a TXT record at `mail._domainkey.mail.sta-tw.org`
(DNS only, not proxied — TXT records can't be proxied anyway).

## DNS records needed (once)

All DNS-only (grey cloud). Everything is on `mail.sta-tw.org`, not the apex,
because that's `STA_SMTP_FROM`'s domain (`noreply@mail.sta-tw.org`) and the
DKIM signing domain:

| Type | Host | Value |
|---|---|---|
| TXT | `mail.sta-tw.org` | `v=spf1 ip4:140.115.154.67 ~all` |
| TXT | `mail._domainkey.mail.sta-tw.org` | (printed on first boot, see above) |
| TXT | `_dmarc.sta-tw.org` | `v=DMARC1; p=quarantine; rua=mailto:postmaster@sta-tw.org` |

Start DMARC at `p=none` if you want to only monitor before enforcing.

## Rotating the DKIM key

Delete the `postfix-dkim` volume and restart the service — a new key is
generated and printed the same way. Update the DNS TXT record afterwards.
