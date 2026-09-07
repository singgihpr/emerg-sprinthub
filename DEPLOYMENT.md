# Deployment — Ubuntu VM with Docker

Sprint Hub runs as three containers behind one port:

| Service | Image / build | Exposed | Notes |
| --- | --- | --- | --- |
| `web` | `frontend/Dockerfile` → nginx:1.27-alpine | `80` | Serves the CRA build, proxies `/api/` → `backend:8000`, SPA fallback |
| `backend` | `backend/Dockerfile` → python:3.11-slim | internal only | `uvicorn server:app` on 8000, non-root |
| `mongo` | `mongo:7` | internal only | Named volume `mongo_data` |

Files that make this work: `docker-compose.yml`, `backend/Dockerfile`, `backend/.dockerignore`, `frontend/Dockerfile`, `frontend/.dockerignore`, `frontend/nginx.conf`, `.env.example`.

Because nginx proxies `/api/`, the frontend is built with `REACT_APP_BACKEND_URL=""` → the browser calls same-origin `/api/...`. No CORS, no second public port, no baked-in hostname.

## 1. VM prerequisites

Ubuntu 22.04 or 24.04, 2 vCPU / 2 GB RAM minimum (4 GB comfortable — the CRA build is the memory hog; `NODE_OPTIONS=--max-old-space-size=2048` is already set in the frontend Dockerfile). Open inbound `22` and `80` (plus `443` if you add TLS) in the cloud firewall/security group.

## 2. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker          # or log out and back in
docker compose version # must print v2.x
```

## 3. Get the code onto the VM

Git (preferred):

```bash
git clone <your-repo-url> sprinthub && cd sprinthub
```

No remote? Copy from your machine instead:

```bash
rsync -az --exclude node_modules --exclude .venv --exclude .git \
  ./ user@VM_IP:sprinthub/
```

## 4. Configure environment

```bash
cd ~/sprinthub
cp .env.example .env
sed -i "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" .env
nano .env    # set ADMIN_EMAIL / ADMIN_PASSWORD, optional email + cron keys
chmod 600 .env
```

| Var | Required | Purpose |
| --- | --- | --- |
| `DB_NAME` | yes | Mongo database name |
| `JWT_SECRET` | yes | Token signing key. Backend exits if missing. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | no | Seeded admin, defaults `widiardhana@gmail.com` / `Admin@1234` — **change these** |
| `EMERGENT_EMAIL_KEY`, `EMAIL_FROM_NAME` | no | Transactional email; skipped with a log warning if unset |
| `WEBHOOK_CRON_SECRET` | no | Bearer token for the two `/api/cron/*` endpoints; they 401 without it |

`MONGO_URL` is set by compose (`mongodb://mongo:27017`) — do not put it in `.env`.

## 5. Build and start

```bash
docker compose build          # first run: ~5-10 min (npm ci + craco build)
docker compose up -d
docker compose ps             # mongo + backend should reach "healthy"
```

First backend start seeds the admin user, an "Acme Corp" org, and sample projects/sprints/tasks.

## 6. Verify

```bash
curl -I http://localhost/                          # 200, index.html
curl -s http://localhost/api/docs -o /dev/null -w '%{http_code}\n'   # 200
docker compose logs backend | grep -i "startup complete"
```

Then browse to `http://VM_IP/` and log in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

Google OAuth login will not work here — it validates sessions against the Emergent preview environment. Use email/password.

## 7. TLS (optional)

Terminates in front of `web`; nothing in the stack changes.

```bash
sudo apt install -y caddy
sudo tee /etc/caddy/Caddyfile >/dev/null <<'EOF'
your.domain.tld {
    reverse_proxy localhost:80
}
EOF
sudo systemctl reload caddy
```

Open `443`, point DNS at the VM, Caddy fetches and renews the cert.

## 8. Cron endpoints (optional)

`/api/cron/weekly-digest` and `/api/cron/spawn-recurring` need `WEBHOOK_CRON_SECRET` in `.env`. Drive them from any scheduler:

```bash
curl -sf -X POST http://localhost/api/cron/weekly-digest \
  -H "Authorization: Bearer $WEBHOOK_CRON_SECRET" \
  -H "X-Webhook-Id: $(date +%Y-%m-%d)"
```

`X-Webhook-Id` is the idempotency key (stored in `cron_runs`) — reuse the same value and the job no-ops. Host crontab example, Mondays 08:00:

```
0 8 * * 1 WEBHOOK_CRON_SECRET=... curl -sf -X POST http://localhost/api/cron/weekly-digest -H "Authorization: Bearer $WEBHOOK_CRON_SECRET" -H "X-Webhook-Id: digest-$(date +\%Y\%m\%d)"
```

## 9. Operations

```bash
docker compose logs -f backend          # or web / mongo
docker compose restart backend
docker compose down                     # stop, keep data
docker compose up -d --build            # redeploy after git pull
docker system prune -f                  # reclaim disk from old images
```

Redeploy:

```bash
git pull && docker compose build && docker compose up -d
```

Backup Mongo (volume survives `down`, but take real dumps):

```bash
docker compose exec -T mongo mongodump --archive --gzip --db sprinthub > backup-$(date +%F).gz
# restore:
gunzip -c backup-2026-01-01.gz | docker compose exec -T mongo mongorestore --archive --gzip
```

## 10. Troubleshooting

| Symptom | Cause / fix |
| --- | --- |
| `docker compose build` fails on `npm ci` fetching `@emergentbase/visual-edits` | devDependency served from `assets.emergent.sh`; VM needs outbound HTTPS. It is unused in production builds — `npm pkg delete devDependencies.@emergentbase/visual-edits && npm install --package-lock-only` if the host stays unreachable. |
| Frontend build killed / exit 137 | OOM. Bigger VM, or build the image on your machine and `docker save`/`docker load` it onto the VM. |
| `backend` exits immediately, `KeyError: 'JWT_SECRET'` | `.env` missing or not readable by compose. It must sit next to `docker-compose.yml`. |
| `502 Bad Gateway` from nginx | backend unhealthy or still starting: `docker compose logs backend`. |
| `413 Request Entity Too Large` on logo upload | `client_max_body_size` in `frontend/nginx.conf` is 6m; raise it if you raise `MAX_LOGO_UPLOAD` in `server.py`. |
| Deep link (e.g. `/tasks`) 404s after redeploy | nginx SPA fallback missing — confirm `frontend/nginx.conf` was copied into the image. |
| Emails not sent | `EMERGENT_EMAIL_KEY` unset; backend logs `EMERGENT_EMAIL_KEY missing; skipping email send`. |
| Reset everything | `docker compose down -v` deletes the Mongo volume. Irreversible. |

## Notes on the backend image

`backend/Dockerfile` installs an explicit dependency list rather than `requirements.txt`. That file pins `emergentintegrations==0.2.0`, a private Emergent.sh SDK that is not on PyPI (404) and is not imported by `server.py`, so `pip install -r requirements.txt` fails outright. It also omits `httpx` and `Pillow`, both of which `server.py` imports, and carries unused heavy packages (pandas, numpy, boto3, jq, typer, plus pytest/black/flake8/mypy). If `requirements.txt` is ever fixed, switch the Dockerfile back to `pip install --no-cache-dir -r requirements.txt`.
