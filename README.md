# Orbit

A unified CLI for monitoring services deployed across multiple cloud platforms.

Orbit gives indie developers and small teams a single-pane view of services scattered across Vercel, Koyeb, and Supabase. Check status, tail logs, track deployments, and scale services — all from one command line.

## Install

**Linux / macOS:**

```bash
source <(curl -sSL https://raw.githubusercontent.com/humanetools/orbit/main/install.sh)
```

**Homebrew (macOS):**

```bash
brew install humanetools/tap/orbit
```

**Go install:**

```bash
go install github.com/humanetools/orbit@latest
```

**From source:**

```bash
git clone https://github.com/humanetools/orbit.git
cd orbit
make build
```

## Quick Start

```bash
# Interactive setup — connect platforms, discover services, create a project
orbit init

# Or connect platforms individually
orbit connect vercel
orbit connect koyeb
orbit connect supabase

# Check everything at a glance
orbit status
```

## Commands

### Monitoring

| Command | Description |
|---------|-------------|
| `orbit status` | Overview of all projects |
| `orbit status <project>` | Detailed metrics for a project |
| `orbit status <project> --service api` | Single service detail card |
| `orbit logs <project> --service api` | View service logs |
| `orbit logs <project> --service api -f` | Stream logs in real time |

### Deployments

| Command | Description |
|---------|-------------|
| `orbit deploys <project>` | Deployment history |
| `orbit watch <project> --service api` | Watch for new deploys after a push |
| `orbit redeploy <project> --service api` | Trigger a redeployment |
| `orbit rollback <project> --service api` | Rollback to previous deployment |

### Scaling (Koyeb)

| Command | Description |
|---------|-------------|
| `orbit scale <project> --service api` | View current scaling config |
| `orbit scale <project> --service api --min 3` | Set minimum instances |
| `orbit scale <project> --service api --type small` | Change instance type |

### Platform Management

| Command | Description |
|---------|-------------|
| `orbit init` | Interactive setup wizard |
| `orbit connect <platform>` | Connect a platform with API token |
| `orbit connections` | List connected platforms |
| `orbit disconnect <platform>` | Remove a platform connection |

## Watch + CI/CD

`orbit watch` is designed for use with CI/CD pipelines and AI coding assistants. Push code, then watch the deployment:

```bash
git push origin main
orbit watch myshop --service api --format json
```

Exit codes tell you what happened:

| Code | Meaning |
|------|---------|
| 0 | Deploy successful |
| 1 | Build/deploy failed |
| 2 | No new deployment detected |
| 3 | Timeout |

JSON output includes deploy ID, commit, duration, error logs — everything needed for automated responses.

## Supported Platforms

| Platform | Status | Logs | Deploys | Scale | Watch |
|----------|--------|------|---------|-------|-------|
| **Vercel** | Health, metrics | Build events | Full history | Auto (N/A) | Polling |
| **Koyeb** | Health, metrics | Runtime (SSE) | Full history | Min/max, instance type | Polling |
| **Supabase** | Health check | Dashboard only | N/A | N/A | N/A |

## Configuration

Orbit stores config in `~/.orbit/`:

- `config.yaml` — projects, platform tokens (AES-256 encrypted), thresholds
- `key` — encryption key (auto-generated, permissions 0600)

```yaml
default_project: myshop
platforms:
  vercel:
    token: "ENC:..."
  koyeb:
    token: "ENC:..."
projects:
  myshop:
    topology:
      - name: frontend
        platform: vercel
        id: "prj_xxxx"
      - name: api
        platform: koyeb
        id: "svc_xxxx"
      - name: db
        platform: supabase
        id: "ref_xxxx"
thresholds:
  response_time_ms: 500
  cpu_percent: 80
  memory_percent: 85
```

## Project Structure

```
orbit/
├── cmd/                     # Cobra commands
│   ├── root.go
│   ├── init.go              # Interactive setup wizard
│   ├── status.go            # orbit status
│   ├── logs.go              # orbit logs
│   ├── watch.go             # orbit watch
│   ├── deploys.go           # orbit deploys
│   ├── redeploy.go          # orbit redeploy
│   ├── rollback.go          # orbit rollback
│   ├── scale.go             # orbit scale
│   ├── connect.go           # orbit connect
│   ├── connections.go       # orbit connections
│   └── disconnect.go        # orbit disconnect
├── internal/
│   ├── config/              # Config + AES-256 encryption
│   ├── platform/            # Platform adapters (Vercel, Koyeb, Supabase)
│   ├── ui/                  # TUI components (Lipgloss, Bubbletea)
│   └── version/             # Build version info
├── main.go
├── Makefile
└── go.mod
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
