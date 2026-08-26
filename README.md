# zoraxy-tunnel

A self-hosted, Cloudflare-Tunnel-style reverse tunnel for [Zoraxy](https://github.com/tobychui/zoraxy).
Run the plugin on your Zoraxy box, expose one TLS port, and reach services
behind NAT/firewalls from anywhere — without an external account.

```
Browser ──▶ Zoraxy (TLS) ──▶ plugin ingress :9080
                                   │  (yamux stream over TLS)
                                   ▼
                            tunnel-client ──▶ http://127.0.0.1:3000 (your service)
```

## How it works

The plugin runs **three** listeners:

| Port | Kind | Purpose |
|------|------|---------|
| dynamic | HTTP | Dashboard UI (`/ui`) + JSON API (`/ui/api/*`), proxied by Zoraxy |
| `9080` (static) | HTTP | Ingress — public traffic Zoraxy routes here, dispatched by `Host` |
| `9443` (static) | TLS | Control plane — tunnel clients dial this |

On first start the plugin mints a **self-signed cert valid 99 years** and shows
its **SHA256 fingerprint** in the dashboard. Clients pin that fingerprint during
the TLS handshake — if the cert doesn't match, the connection is killed before
any data is exchanged. Authorization is a per-tunnel **token** (stored only as
a hash).

## Trust model

1. Plugin generates cert → fingerprint shown in UI.
2. Client connects to `:9443`, computes SHA256 of the presented cert, compares to
   `--fingerprint`. Mismatch → connection dropped.
3. Client authenticates with `--token`; plugin maps it to a tunnel by hash.
4. One live client per tunnel (a reconnecting client replaces the previous one).

## Setup

### 1. Build

```bash
git clone https://github.com/sniffingsugar/zoraxy-tunnel
cd zoraxy-tunnel
go build -o zoraxy-tunnel .          # the plugin
go build -o tunnel-client ./client   # the client
```

Pre-built binaries for both are published under
[releases](https://github.com/sniffingsugar/zoraxy-tunnel/releases).

### 2. Install the plugin

Drop the binary into Zoraxy's plugin folder (folder name must equal binary name):

```
plugins/zoraxy-tunnel/zoraxy-tunnel
```

Restart Zoraxy. Open the plugin UI.

### 3. Configure the node

In the dashboard:

1. Set **Server address** to where clients reach your control port, e.g.
   `tunnel.example.com:9443` (port-forward/expose `9443` to the internet).
2. Optionally set the **Default Zoraxy TAG** used for newly registered services.
   It defaults to `ZoraxyTunnel` and can be overridden per service.
3. Note the **fingerprint**.

### 4. Create a tunnel + connect a client

Click **Create tunnel**, name it, and copy the command the modal shows. There are
three ready-to-paste variants:

**CLI**
```bash
tunnel-client \
  --server tunnel.example.com:9443 \
  --token zt_… \
  --fingerprint "AB:CD:EF:…"
```

**Docker**
```bash
docker run -d --name tunnel-client --restart unless-stopped \
  --network host \
  ghcr.io/sniffingsugar/tunnel-client:latest \
  --server tunnel.example.com:9443 \
  --token zt_… \
  --fingerprint "AB:CD:EF:…"
```

**docker-compose.yml**
```yaml
services:
  tunnel-client:
    image: ghcr.io/sniffingsugar/tunnel-client:latest
    container_name: tunnel-client
    restart: unless-stopped
    network_mode: host          # Linux: 127.0.0.1 targets work as-is
    command:
      - --server=tunnel.example.com:9443
      - --token=zt_…
      - --fingerprint=AB:CD:EF:…
```

> The token is shown **once**. Only a hash is stored afterwards — regenerate it
> from the tunnel's menu if you lose it.

> With Docker, the client reaches services on the **host**. On Linux
> `network_mode: host` makes `127.0.0.1:3000` work directly. On macOS/Windows
> Docker Desktop, drop `network_mode: host` and target `host.docker.internal`
> instead in each service's target.

### 5. Register a service + install the route

Inside a tunnel, **Register service**:

- **Public host** — e.g. `app.example.com` (the domain the world visits)
- **Path prefix** — optional, e.g. `/api`
- **Local target** — what the *client* dials, e.g. `http://127.0.0.1:3000`
- **Zoraxy TAG(s)** — optional comma-separated tags applied when the route is installed
- **Skip TLS certificate verification** — optional for HTTPS targets using a self-signed or otherwise untrusted certificate

Then click **Install route**. The plugin creates a Zoraxy proxy rule
`app.example.com → 127.0.0.1:9080` for you and applies the configured tags.
Existing services can be edited without deleting and recreating them; when the
public host of an installed service changes, the managed Zoraxy route is moved
to the new host. Deleting the service or tunnel removes that route automatically.

Public HTTP(S) and WebSockets are both supported and streamed. TLS verification
is enabled for HTTPS targets by default and can be disabled per service when
needed.

## Requirements

- Zoraxy 3.2.0+ (plugin system)
- One publicly reachable TCP port for `:9443` (port-forward / expose)
- Go 1.23+ to build from source

## License

MIT
