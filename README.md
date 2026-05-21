<div align="center">

<img src="https://capsule-render.vercel.app/api?type=waving&color=0:0d1117,50:1a1a2e,100:16213e&height=220&section=header&text=Chaturbate%20DVR&fontSize=80&fontColor=e94560&fontAlignY=35&desc=Multi-channel%20stream%20recorder%20%E2%80%94%20Docker%20%7C%20Cloud%20%7C%20Local&descSize=18&descAlignY=55&animation=fadeIn" width="100%"/>

<br/>

[![Stars](https://img.shields.io/github/stars/vasud3v/chaturbate-recorder?style=flat&color=e94560&logo=github)](https://github.com/vasud3v/chaturbate-recorder/stargazers)
[![Forks](https://img.shields.io/github/forks/vasud3v/chaturbate-recorder?style=flat&color=0f3460&logo=github)](https://github.com/vasud3v/chaturbate-recorder/network/members)
[![Issues](https://img.shields.io/github/issues/vasud3v/chaturbate-recorder?style=flat&color=533483&logo=github)](https://github.com/vasud3v/chaturbate-recorder/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat&logo=github)](https://github.com/vasud3v/chaturbate-recorder/pulls)
[![License](https://img.shields.io/github/license/vasud3v/chaturbate-recorder?style=flat&color=00b894&logo=opensourceinitiative&logoColor=white)](LICENSE)
[![Last Commit](https://img.shields.io/github/last-commit/vasud3v/chaturbate-recorder?style=flat&color=6c5ce7&logo=github)](https://github.com/vasud3v/chaturbate-recorder/commits/main)
[![Code Size](https://img.shields.io/github/languages/code-size/vasud3v/chaturbate-recorder?style=flat&color=fdcb6e&logo=github)](https://github.com/vasud3v/chaturbate-recorder)
[![Workflow Status](https://img.shields.io/github/actions/workflow/status/vasud3v/chaturbate-recorder/recorder.yml?style=flat&label=recorder&color=0984e3&logo=githubactions&logoColor=white)](https://github.com/vasud3v/chaturbate-recorder/actions/workflows/recorder.yml)

<br/>

<a href="#rocket-quick-start"><img src="https://img.shields.io/badge/-Quick%20Start-e94560?style=for-the-badge&logo=rocket&logoColor=white" /></a>
<a href="#zap-features"><img src="https://img.shields.io/badge/-Features-0984e3?style=for-the-badge&logo=starship&logoColor=white" /></a>
<a href="#book-deployment-guides"><img src="https://img.shields.io/badge/-Deployment-6c5ce7?style=for-the-badge&logo=readthedocs&logoColor=white" /></a>
<a href="#heart-support"><img src="https://img.shields.io/badge/-Support-e94560?style=for-the-badge&logo=githubsponsors&logoColor=white" /></a>

<br/>

**Record live streams automatically** — multi-channel DVR with Cloudflare bypass, auto-uploads, and a web dashboard. Run it on your server, in Docker, or for free on GitHub Actions.

</div>

---

## :zap: Features

<table>
<tr>
<td width="50%">

### :movie_camera: Recording
- Multi-channel simultaneous capture
- HLS `.ts` + LL-HLS `.m4s` support
- Auto-split by duration or file size
- ffmpeg compression to `.mkv`

</td>
<td width="50%">

### :globe_with_meridians: Deployment
- **Docker Compose** — one command, full stack
- **GitHub Actions** — free 24/7 cloud recording
- **Binary** — single portable executable
- **Web UI** — manage everything from browser

</td>
</tr>
<tr>
<td>

### :shield: Cloudflare Bypass
- Byparr with load balancing (2 workers)
- Auto cookie refresh daemon
- Proxy support (SOCKS5/HTTP)

</td>
<td>

### :cloud: Uploads & Storage
- 6+ hosting providers in parallel
- Thumbnail & sprite generation
- Supabase metadata storage
- Browse everything in the dashboard

</td>
</tr>
</table>

---

## :rocket: Quick Start

```bash
git clone https://github.com/vasud3v/chaturbate-recorder.git
cd chaturbate-recorder
cp .env.example .env        # add your API keys (optional)
docker compose up -d --build
```

Open **http://localhost:8080** — add channels, hit record. Done.

---

## :book: Deployment Guides

### :whale: Option A — Docker (Recommended)

**Prerequisites:** [Docker](https://docs.docker.com/get-docker/) + [Docker Compose](https://docs.docker.com/compose/install/)

```bash
# 1. Clone the repo
git clone https://github.com/vasud3v/chaturbate-recorder.git
cd chaturbate-recorder

# 2. Configure environment
cp .env.example .env
# Edit .env with your API keys (see Environment Variables below)

# 3. Start all services
docker compose up -d --build

# 4. View logs
docker compose logs -f chaturbate-dvr
```

**Services started:**

| Service | Port | Purpose |
|---------|------|---------|
| `chaturbate-dvr` | `8080` | Recorder + Web UI |
| `byparr-lb` | `8191` | Cloudflare bypass (nginx) |
| `byparr` (x2) | internal | Byparr workers |
| `cookie-refresher` | — | Auto cookie renewal |
| `uploader` | — | Background uploads |

**Volumes:** `./videos` · `./conf` · `./database`

---

### :octocat: Option B — GitHub Actions (Free Cloud Recording)

Run the full stack on GitHub-hosted runners — **no server needed**.

```mermaid
flowchart LR
    subgraph GHA [GitHub Actions Runner]
        DC[docker compose]
        DVR[chaturbate-dvr]
        BYP[Byparr x2]
        TUN[cloudflared tunnel]
    end
    subgraph Out [Outputs]
        UI[Public Dashboard URL]
        ART[Emergency Backup]
        SB[Supabase Metadata]
    end
    DC --> BYP --> DVR
    DVR --> TUN --> UI
    DVR --> ART
    DVR --> SB
```

**Step-by-step setup:**

1. **Fork** this repository

2. **Add repository secrets** — go to `Settings` → `Secrets and variables` → `Actions` → `New repository secret`

    | Secret | Required | Purpose |
    |--------|----------|---------|
    | `SUPABASE_URL` | Yes | Your Supabase project URL |
    | `SUPABASE_API_KEY` | Yes | Supabase anon/service key |
    | `VOESX_API_KEY` | For uploads | Voe.sx API key |
    | `SENDCM_API_KEY` | For uploads | SendCM API key |
    | `GOFILE_API_KEY` | Optional | GoFile (guest works without) |
    | `BYSE_API_KEY` | Optional | Byse.sx API key |
    | `PROXY_SERVER` | Optional | Proxy URL for Byparr |
    | `PROXY_USERNAME` | Optional | Proxy auth |
    | `PROXY_PASSWORD` | Optional | Proxy auth |

3. **Configure channels** — edit [`conf/channels.json`](conf/channels.json) in your fork:

   ```json
   [
     {
       "is_paused": false,
       "username": "channel_name",
       "framerate": 30,
       "resolution": 1080,
       "compress": true
     }
   ]
   ```

4. **Set your INSTANCE_ID** — edit `.github/workflows/recorder.yml` and change `INSTANCE_ID: "a"` to a unique value per fork (e.g., `"b"`, `"c"`). This isolates each fork's channels while sharing the same Supabase database.

5. **Run the workflow** — `Actions` tab → `24/7 Recorder` → `Run workflow` → choose duration (1–5 hours)

5. **Get your dashboard URL** — check the run summary, or query your Supabase `tunnel_sessions` table

**What happens automatically:**
- Byparr starts with 2 workers for Cloudflare bypass
- A **Cloudflare Tunnel** exposes your dashboard on a public `trycloudflare.com` URL
- Recording runs for the chosen duration with health checks every 60s
- If the recorder crashes, it auto-restarts
- Uploads run in the background to configured hosts
- Emergency backup saved as a GitHub artifact (30-day retention)
- **Next run auto-queues** for continuous 24/7 operation

> **Limits:** GitHub-hosted runners cap at ~5 hours per run. Videos aren't persisted on GitHub long-term — use upload hosts + Supabase for permanent storage.

---

### :repeat: Multi-Instance Setup (Record 100+ Channels)

GitHub Actions has a ~14 GB disk limit (~3–5 channels max per runner). To record more channels, fork this repo to multiple GitHub accounts — all sharing the same Supabase database.

```mermaid
flowchart TB
    subgraph ForkA [Fork A · INSTANCE_ID=a]
        A1[channels: alice, bob]
    end
    subgraph ForkB [Fork B · INSTANCE_ID=b]
        B1[channels: charlie, dave]
    end
    subgraph ForkC [Fork C · INSTANCE_ID=c]
        C1[channels: eve, frank]
    end
    subgraph Supabase [Shared Supabase]
        DB[(recordings)]
        DB2[(upload_links)]
        DB3[(preview_images)]
        AS[app_settings]
    end
    A1 --> DB
    B1 --> DB
    C1 --> DB
    A1 -.->|channels_a| AS
    B1 -.->|channels_b| AS
    C1 -.->|channels_c| AS
```

**How it works:**

| Component | Behavior |
|-----------|----------|
| Channels | Isolated per instance via `channels_<instance_id>` in `app_settings` |
| Tunnels | Isolated per instance — each fork gets its own dashboard URL |
| Recordings | Shared — all recordings aggregate into one unified video library |
| Uploads | Shared — all instances upload to the same hosts |
| Cookies | Shared — one instance's cookie refresh benefits all |

**Setup:**

1. **Fork** this repo to each GitHub account
2. **Edit `.github/workflows/recorder.yml`** — set a unique `INSTANCE_ID` per fork:
   ```yaml
   env:
     INSTANCE_ID: "b"  # unique per fork: "a", "b", "c", ...
   ```
3. **Add the same Supabase secrets** to each fork
4. **Run `database/migrate.sql`** in your Supabase SQL Editor (one-time setup)

> **Note:** Don't add the same channel username to multiple instances — both would record it simultaneously and create duplicate uploads.

---

### :desktop_computer: Option C — Local Binary

**Prerequisites:** [Go 1.23+](https://go.dev/dl/), ffmpeg

```bash
# Build from source
go build -o chaturbate-dvr .
./chaturbate-dvr

# Or with options
./chaturbate-dvr --port 8123 --admin-username admin --admin-password secret
```

**CLI mode** (no dashboard, single channel):

```bash
./chaturbate-dvr -u CHANNEL_USERNAME -resolution 1080 -framerate 30
```

---

## :database: Database Setup

Run [`database/migrate.sql`](database/migrate.sql) once in your Supabase SQL Editor. It creates all tables, indexes, RLS policies, and permissions. Safe to re-run — uses `IF NOT EXISTS` everywhere.

```sql
-- Paste the entire contents of database/migrate.sql
```

This sets up:
- `channels`, `recordings`, `upload_links` — core DVR data
- `tunnels`, `tunnel_sessions` — Cloudflare tunnel tracking (per-instance)
- `app_settings` — channel configs and cookies (per-instance via `channels_<id>`)
- `channel_logs`, `preview_images`, `disk_usage` — monitoring and metadata

---

## :gear: Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `INSTANCE_ID` | For multi-instance | Unique ID per fork/runner (e.g., `"a"`, `"b"`, `"c"`) |
| `SUPABASE_URL` | For Actions | Supabase project URL |
| `SUPABASE_API_KEY` | For Actions | Supabase anon key |
| `VOESX_API_KEY` | For uploads | Voe.sx API key |
| `SENDCM_API_KEY` | For uploads | SendCM API key |
| `GOFILE_API_KEY` | Optional | GoFile API key |
| `BYSE_API_KEY` | Optional | Byse.sx API key |
| `COOKIES` | Optional | Browser cookies for auth |
| `USER_AGENT` | Optional | Custom User-Agent string |
| `FLARESOLVERR_URL` | Auto in Docker | Byparr endpoint |
| `PROXY_SERVER` | Optional | HTTP/SOCKS5 proxy |
| `PROXY_USERNAME` | Optional | Proxy username |
| `PROXY_PASSWORD` | Optional | Proxy password |

---

## :camera: Dashboard Preview

<p align="center">
  <img src="docs/images/dashboard.png" alt="Dashboard" width="85%" />
</p>

---

## :shield: Cloudflare Bypass

### Automatic (Docker / GitHub Actions)

Byparr handles it — set `FLARESOLVERR_URL=http://byparr-lb/v1` (included in `docker-compose.yml`). The cookie-refresher keeps `cf_clearance` fresh.

### Manual (Local)

1. Open [chaturbate.com](https://chaturbate.com), pass the Cloudflare check
2. **F12** → **Application** → **Cookies** → copy `cf_clearance`
3. Run with cookies:

```bash
./chaturbate-dvr -u CHANNEL -cookies "cf_clearance=YOUR_VALUE" -user-agent "YOUR_UA"
```

---

## :movie_camera: Upload Hosts

| Host | Environment Variable |
|------|---------------------|
| GoFile | `GOFILE_API_KEY` |
| Voe.sx | `VOESX_API_KEY` |
| SendCM | `SENDCM_API_KEY` |
| Byse | `BYSE_API_KEY` |

Thumbnails use a fallback chain: **Pixhost** → **Catbox** → **Freeimage**.

---

## :bar_chart: Repo Pulse

<p align="center">
  <img src="https://repobeats.axiom.co/api/embed/e8ec122a9f6217881b46ffee305942fc99b8c008.svg" alt="Repobeats analytics" />
</p>

---

## :star: Star History

<a href="https://star-history.com/#vasud3v/chaturbate-recorder&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=vasud3v/chaturbate-recorder&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=vasud3v/chaturbate-recorder&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=vasud3v/chaturbate-recorder&type=Date" width="70%" />
 </picture>
</a>

---

## :heart: Support

If this project helps you, consider giving it a :star::

<p align="center">
  <a href="https://github.com/vasud3v/chaturbate-recorder/stargazers">
    <img src="https://img.shields.io/badge/⭐_Star_This_Repo-e94560?style=for-the-badge&logo=github&logoColor=white" alt="Star" height="40" />
  </a>
</p>

---

## :trophy: Credits & Acknowledgements

<div align="center">

### Original Creator

**[TeaCat](https://github.com/teacat)** — the brilliant mind behind the original [chaturbate-dvr](https://github.com/teacat/chaturbate-dvr)

Without TeaCat's foundational work, none of this would exist. The core recording engine, HLS capture logic, and initial architecture are all thanks to their dedication. This project continues their vision with extended features.

<br/>

<a href="https://github.com/teacat/chaturbate-dvr">
  <img src="https://img.shields.io/badge/⭐_Original_Project-teacat%2Fchaturbate--dvr-ffd700?style=for-the-badge&logo=github&logoColor=white" alt="Original Project" />
</a>

<br/><br/>

### Also Thanks To

| Project | Purpose |
|---------|---------|
| [Byparr](https://github.com/ThePhaseless/Byparr) | Cloudflare bypass engine |
| [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) | Public dashboard tunneling |
| [Twemoji](https://github.com/jdecked/twemoji) | UI favicon |

</div>

---

## :scroll: License

[MIT](LICENSE) — originally by [TeaCat](https://github.com/teacat) (2024), continued here.

<div align="center">
  <sub>Built with :heart: in 2026</sub>
</div>
