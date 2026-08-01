# NotifyProject — EC2 Deployment Guide (Amazon Linux + Docker)

Complete guide to deploy **Notify** (gRPC + REST gateway + MySQL) on an **Amazon Linux** EC2 instance using Docker, including running **alongside an existing Docker app** and attaching a **custom domain**.

---

## Table of contents

1. [Architecture overview](#1-architecture-overview)
2. [What this project exposes](#2-what-this-project-exposes)
3. [Prerequisites](#3-prerequisites)
4. [EC2 security group](#4-ec2-security-group)
5. [Install Docker on Amazon Linux](#5-install-docker-on-amazon-linux)
6. [Project files used for deploy](#6-project-files-used-for-deploy)
7. [Configure credentials](#7-configure-credentials)
8. [Copy the project to EC2](#8-copy-the-project-to-ec2)
9. [Check for port conflicts (important)](#9-check-for-port-conflicts-important)
10. [Build and start with Docker Compose](#10-build-and-start-with-docker-compose)
11. [Verify the app is running](#11-verify-the-app-is-running)
12. [Share EC2 with an existing Docker app](#12-share-ec2-with-an-existing-docker-app)
13. [Attach a domain (Nginx reverse proxy)](#13-attach-a-domain-nginx-reverse-proxy)
14. [Enable HTTPS (Let's Encrypt)](#14-enable-https-lets-encrypt)
15. [Day-2 operations](#15-day-2-operations)
16. [Troubleshooting](#16-troubleshooting)

---

## 1. Architecture overview

```
Internet
   │
   ▼
┌─────────────────────────────────────────────┐
│  EC2 (Amazon Linux)                         │
│                                             │
│  Nginx (:80 / :443)                         │
│    ├─ existing-app.yourdomain.com           │
│    │     └─► existing container (:xxxx)     │
│    └─ notify-backend.yourdomain.com         │
│          └─► host :8082 → notify-app :8081  │
│                 └─► notify-mysql (:3306)    │
└─────────────────────────────────────────────┘
```

- **Notify app** listens on REST `:8081` **inside** the container.
- On this EC2, **host port `8081` is already used by another backend**, so Compose maps **`8082:8081`** (host `8082` → container `8081`).
- gRPC is on `:50051` (change host side if that port is also taken).
- **Nginx** proxies the domain to `127.0.0.1:8082` (not 8081).
- You do **not** change Go code for the domain. Domain is DNS + Nginx.

---

## 2. What this project exposes

| Protocol | Container port | Host port (this EC2) | Purpose |
|----------|----------------|----------------------|---------|
| HTTP REST (grpc-gateway) | `8081` | **`8082`** (8081 taken by other backend) | Main API |
| gRPC | `50051` | `50051` (remap if busy) | Direct gRPC (optional) |
| MySQL | `3306` | prefer unpublished | Database |

Config is loaded from:

- `conf/config.env` → sets `CONFIG_FILE=../conf/config.yaml`
- In Docker, those files are overridden by:
  - `conf/config.docker.env`
  - `conf/config.docker.yaml`

Working directory inside the container is `/app/cmd` (required because the app loads `../conf/config.env` relative to CWD).

---

## 3. Prerequisites

- EC2 instance running **Amazon Linux 2023** or **Amazon Linux 2**
- SSH access as `ec2-user`
- An **Elastic IP** attached to the instance (recommended so DNS does not break on reboot)
- A domain you control (for custom hostname)
- This repo on the instance (git clone or scp)

---

## 4. EC2 security group

Inbound rules:

| Port | Protocol | Source | Why |
|------|----------|--------|-----|
| 22 | TCP | Your IP only | SSH |
| 80 | TCP | `0.0.0.0/0` | HTTP (Nginx / domain) |
| 443 | TCP | `0.0.0.0/0` | HTTPS |
| 8082 | TCP | Your IP (optional) | Direct Notify testing without Nginx |
| 50051 | TCP | As needed | Only if external gRPC is required |
| 3306 | TCP | **Do not open publicly** | MySQL |

Do **not** point Notify at host `8081` — that belongs to the other backend. After Nginx + domain work, you can close public `8082`.

---

## 5. Install Docker on Amazon Linux

### Amazon Linux 2023

```bash
sudo dnf update -y
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
```

Log out of SSH and log back in, then verify:

```bash
docker --version
```

### Amazon Linux 2

```bash
sudo yum update -y
sudo amazon-linux-extras install docker -y
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
```

Log out / log in, then verify Docker.

### Install Docker Compose plugin (both)

```bash
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo curl -SL https://github.com/docker/compose/releases/download/v2.29.7/docker-compose-linux-x86_64 \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
docker compose version
```

> On ARM (Graviton) instances, use the `aarch64` Compose binary instead of `x86_64`.

---

## 6. Project files used for deploy

| File | Role |
|------|------|
| `Dockerfile` | Multi-stage build of the Go binary |
| `docker-compose.yml` | Runs `notify-app` + `notify-mysql` |
| `conf/config.docker.yaml` | MySQL host `mysql`, REST `:8081`, gRPC `:50051` |
| `conf/config.docker.env` | Points app at config YAML |
| `scripts/init.sql` | Creates base tables on first MySQL start |
| `.dockerignore` | Keeps build context small |

---

## 7. Configure credentials

**Before production**, change default passwords in both places:

1. `docker-compose.yml` → `MYSQL_ROOT_PASSWORD`, `MYSQL_PASSWORD`
2. `conf/config.docker.yaml` → `mysql_details.username` / `password`

They must match.

Example (`conf/config.docker.yaml`):

```yaml
mysql_details:
  username: "notify"
  password: "CHANGE_ME_STRONG_PASSWORD"
  address: "mysql"          # Docker Compose service name — do not use 127.0.0.1 here
  port: "3306"
  db_name: "notify_db"

grpc_details:
  network: "tcp"
  address: ":50051"
  endpoint: "localhost:50051"

http_details:
  port: ":8081"
```

> `address: "mysql"` is correct inside Compose. The app container reaches MySQL by service name on the Docker network.

If you already have a real DB dump, replace `scripts/init.sql` with your dump (or mount it under `/docker-entrypoint-initdb.d/`). Init scripts run **only on first volume create**.

---

## 8. Copy the project to EC2

### Option A — Git

```bash
cd ~
git clone <YOUR_REPO_URL> NotifyProject
cd NotifyProject
```

### Option B — SCP from your laptop

```bash
scp -i /path/to/key.pem -r "/Users/bharathd/Desktop/All Files/NotifyProject" \
  ec2-user@<EC2_PUBLIC_IP>:~/NotifyProject
```

Then SSH in:

```bash
ssh -i /path/to/key.pem ec2-user@<EC2_PUBLIC_IP>
cd ~/NotifyProject
```

---

## 9. Check for port conflicts (important)

Because another Docker image may already be running on this EC2:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
sudo ss -tulpn | grep -E ':80|:443|:8081|:50051|:3306'
```

### Common conflicts and fixes

| Conflict | Fix |
|----------|-----|
| Host `8081` already used | **Already handled:** Compose uses `"8082:8081"`. Nginx must `proxy_pass` to `8082` |
| Host `8082` also used | Pick another free port, e.g. `"8083:8081"`, and update Nginx |
| Host `3306` already used | Change `"3306:3306"` → `"3307:3306"` **or** remove the ports mapping entirely (recommended: MySQL only on Docker network) |
| Host `80`/`443` used by existing Nginx/app | Reuse that Nginx and add a new `server_name` block for Notify (do not start a second Nginx on 80) |

Recommended MySQL publish change (keep DB private to Docker):

```yaml
# docker-compose.yml — mysql service
ports: []   # or delete the ports: section entirely
```

App still connects via `mysql:3306` on the internal network.

---

## 10. Build and start with Docker Compose

```bash
cd ~/NotifyProject
docker compose up -d --build
```

Expected containers:

- `notify-mysql`
- `notify-app`

Check status:

```bash
docker compose ps
docker compose logs -f app
```

Healthy signs in logs:

- `Connected to MySQL`
- `Starting gRPC server on :50051`
- `Starting REST server on :8081` (inside container; host access is **:8082**)

---

## 11. Verify the app is running

On the EC2 (use **8082**, not 8081):

```bash
curl -i http://127.0.0.1:8082/
# or hit a known REST route from your Postman collection
```

From your laptop (only if SG allows 8082):

```bash
curl -i http://<EC2_PUBLIC_IP>:8082/
```

`http://127.0.0.1:8081/` is the **other** backend. Domain comes in the next sections.

---

## 12. Share EC2 with an existing Docker app

You do **not** need one giant Compose file. Run Notify beside the existing stack.

Rules:

1. Different **container names** / Compose projects (already true: `notify-app`, `notify-mysql`)
2. No overlapping **host ports**
3. One shared **reverse proxy** (Nginx) on `80`/`443`
4. Route by **subdomain** (preferred) or path

```
existing-app.yourdomain.com   ──Nginx──► existing backend (e.g. host :8081)
notify-backend.yourdomain.com ──Nginx──► notify host :8082 → container :8081
```

Both DNS A records point to the **same Elastic IP**.

---

## 13. Attach a domain (Nginx reverse proxy)

### 13.1 DNS

Create A records:

| Host / name | Type | Value |
|-------------|------|--------|
| `notify-backend` | A | EC2 Elastic IP |
| `existing-app` (example) | A | **same** Elastic IP |

Verify:

```bash
dig +short notify-backend.yourdomain.com
```

It must return your EC2 IP.

### 13.2 Install Nginx (if not already installed)

Amazon Linux 2023:

```bash
sudo dnf install -y nginx
sudo systemctl enable --now nginx
```

Amazon Linux 2:

```bash
sudo yum install -y nginx
sudo systemctl enable --now nginx
```

If Nginx is **already** serving the other app, skip install and only add a new server block.

### 13.3 Add Notify server block

Create `/etc/nginx/conf.d/notify.conf`:

```nginx
server {
    listen 80;
    server_name notify-backend.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8082;   # Notify host port (NOT 8081)
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Replace `notify-backend.yourdomain.com` with your real hostname.

Example for the **existing** backend already on **8081**:

```nginx
server {
    listen 80;
    server_name existing-app.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8081;  # other backend
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Test and reload:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

### 13.4 Call via domain

```bash
curl -i http://notify-backend.yourdomain.com/
```

Use this URL in Postman / frontend instead of `http://localhost:8081/`.

**Nothing in Go config needs a domain string** for basic REST hosting.

---

## 14. Enable HTTPS (Let's Encrypt)

```bash
# Amazon Linux 2023 example
sudo dnf install -y certbot python3-certbot-nginx

sudo certbot --nginx -d notify-backend.yourdomain.com
```

Follow prompts. Certbot will adjust the Nginx config for `:443`.

Then:

```bash
curl -i https://notify-backend.yourdomain.com/
```

Renewal is typically automatic via certbot timer/cron. Check:

```bash
sudo certbot renew --dry-run
```

---

## 15. Day-2 operations

### View logs

```bash
cd ~/NotifyProject
docker compose logs -f app
docker compose logs -f mysql
```

### Restart

```bash
docker compose restart app
```

### Redeploy after code changes

```bash
cd ~/NotifyProject
git pull   # if using git
docker compose up -d --build
```

### Stop stack

```bash
docker compose down
```

### Stop but keep MySQL data

```bash
docker compose down        # keeps named volume mysql_data
docker compose down -v     # DANGER: deletes DB volume
```

### Enter MySQL shell

```bash
docker exec -it notify-mysql mysql -unotify -p notify_db
```

---

## 16. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Bind for 0.0.0.0:8082 failed` | Host 8082 also taken | Change Compose to another free port; update Nginx |
| `Bind for 0.0.0.0:3306 failed` | Another MySQL on host | Remove/remap mysql `ports` |
| App: `Failed to ping MySQL` | Wrong password / MySQL not healthy / wrong host | Match passwords; wait for healthy; ensure `address: mysql` |
| Hitting 8081 gets wrong API | 8081 is the other backend | Use **8082** or the Notify domain |
| Domain opens wrong app | Nginx `proxy_pass` points at 8081 | Point Notify server block at **8082** |
| Domain does not resolve | DNS / Elastic IP | Fix A record; wait for TTL |
| `curl` works on EC2 but not from laptop | Security group | Open 80/443 (or 8082 for direct test) |
| Tables missing | Volume already existed before `init.sql` change | Import SQL manually into running MySQL |
| Permission denied talking to Docker | User not in `docker` group | `sudo usermod -aG docker ec2-user` then re-login |

Useful debug commands:

```bash
docker compose ps
docker compose logs --tail=200 app
curl -v http://127.0.0.1:8082/          # Notify
curl -v http://127.0.0.1:8081/          # other backend
curl -v -H "Host: notify-backend.yourdomain.com" http://127.0.0.1/
sudo nginx -t
sudo tail -n 100 /var/log/nginx/error.log
```

---

## Quick start checklist

- [ ] Elastic IP attached
- [ ] SG: 22 (your IP), 80, 443
- [ ] Docker + Compose installed on Amazon Linux
- [ ] Passwords updated in Compose + `config.docker.yaml`
- [ ] Port conflicts checked against existing containers
- [ ] `docker compose up -d --build` succeeds
- [ ] Local curl to `127.0.0.1:8082` works (Notify)
- [ ] Confirm `127.0.0.1:8081` is still the other backend
- [ ] DNS A record → Elastic IP
- [ ] Nginx Notify `proxy_pass` → `127.0.0.1:8082`
- [ ] `curl http://notify-backend.yourdomain.com/` works
- [ ] (Optional) Certbot HTTPS

---

## Notes / caveats

1. `scripts/init.sql` is a **starter schema** inferred from service queries. Prefer your real production dump if you have one.
2. Default Compose passwords (`notify@pass`, `root@root`) are for bootstrap only — change them.
3. Publishing MySQL `3306` on the host is optional and usually unnecessary; prefer internal Docker networking only.
4. Host **8081** = other backend; Notify public test port = **8082**. Container still listens on **8081** internally — that is fine.
5. gRPC over a domain typically needs HTTP/2 / TLS passthrough or grpc-web; for most HTTP clients, use the REST gateway via Nginx.
