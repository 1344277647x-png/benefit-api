# Benefit API IP deployment

This guide deploys the customized frontend with New API `v1.0.0-rc.21`
behind the BaoTa Nginx service. It keeps the existing database, Redis, logs,
and Docker volumes unchanged.

## Target topology

```text
Internet -> http://8.218.169.39:80 -> Nginx -> 127.0.0.1:3000 -> New API
```

- Use `http://8.218.169.39` as the public server address.
- Bind the container port to `127.0.0.1:3000:3000` after Nginx is working.
- Do not put the Epay merchant key, API keys, or admin credentials in this
  repository, the Docker image, or Nginx configuration.
- Keep the existing New API data directory and database connection settings.

## Build the customized image

Copy this source tree to a dedicated server directory such as
`/www/wwwroot/benefit-api`, then build it from that directory:

```bash
cd /www/wwwroot/benefit-api
docker build -t benefit-api:newapi-v1.0.0-rc.21 .
```

In the existing BaoTa Compose project, change only the New API service image:

```yaml
services:
  new-api:
    image: benefit-api:newapi-v1.0.0-rc.21
    ports:
      - "127.0.0.1:3000:3000"
```

Preserve the existing `environment`, `volumes`, `depends_on`, database, Redis,
and log settings. Back up the current Compose file and database before replacing
the running container. Recreate only the New API service:

```bash
docker compose up -d --no-deps new-api
docker compose ps
docker compose logs --tail=100 new-api
```

## BaoTa Nginx configuration

Create an HTTP site for `8.218.169.39` and use this reverse-proxy configuration:

```nginx
server {
    listen 80;
    server_name 8.218.169.39;
    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

After Nginx reports a valid configuration, visit `http://8.218.169.39/api/status`
and confirm that the JSON response contains `"success":true`.

## New API public settings

Open the administrator console and set:

- System name: `Benefit API`
- Server address: `http://8.218.169.39`
- Logo URL: `/benefit-api-logo.svg`
- Theme: `default`

Do not use `http://localhost:3000` as the server address. Payment return URLs,
password-reset links, Passkey settings, and other public links derive from this
value.

Because this deployment uses plain HTTP and a bare IP:

- Do not enable `SESSION_COOKIE_SECURE=true` until HTTPS is available.
- Do not enable Passkey login until its RP ID and origin can be validated.
- Validate Cloudflare Turnstile against the final public host before enabling it.
  If hostname validation fails for the bare IP, keep it disabled and restrict
  registration until a domain is available.

## Epay configuration

In **System settings -> Payment settings**:

1. Configure recharge units, exchange rate, minimum top-up, and preset amounts.
2. Add Epay Alipay and/or Epay WeChat Pay with the visual payment-method editor.
3. Under the Epay tab, enter the provider base address in **Epay endpoint**.
4. Set **Callback address** to `http://8.218.169.39`, or leave it empty after
   confirming the server address above is correct.
5. Enter the merchant ID and merchant secret directly in the administrator
   console. Never send the secret to the browser or commit it to Git.

For balance recharge, New API generates these destinations:

- Browser return: `http://8.218.169.39/console/log`
- Asynchronous notification: `http://8.218.169.39/api/user/epay/notify`

The Epay provider must allow public HTTP callbacks to a bare IP. If it requires
HTTPS or a domain, payment cannot be considered production-ready with this
topology.

## Acceptance checks

1. Register a test user and sign in.
2. Create an API key with a small quota.
3. Open the wallet, select an Epay method, and create the minimum-value order.
4. Complete one small payment and confirm both browser return and asynchronous
   notification.
5. Confirm the order changes from pending to completed exactly once and the
   balance increases once.
6. Send one streaming API request through `http://8.218.169.39/v1` and confirm
   that Nginx does not buffer the response.
7. Verify that the administrator console, usage logs, pricing, and language
   switcher still work.

Payment should remain disabled for real users until all acceptance checks pass.
