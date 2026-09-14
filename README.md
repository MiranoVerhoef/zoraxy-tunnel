# Zoraxy Tunnel Enhanced

Self-hosted reverse tunnels for Zoraxy with automated routing, redundant connectors, service health monitoring, activity history, and TLS controls.

## Highlights

- Multiple connectors per logical tunnel with active/standby redundancy.
- Preferred connector selection with automatic failover and failback.
- Per-connector telemetry and service health checks.
- Persistent activity history.
- Zoraxy route automation and optional ACME certificate requests.
- Stable Docker client configuration using `ghcr.io/miranoverhoef/zoraxy-tunnel-client:latest`.
- Non-destructive connector enrollment: adding a connector does not rotate credentials already in use.
- Optional automatic Docker client updates via a dedicated What's Up Docker (WUD) updater sidecar.

## Connector enrollment

Create a tunnel once, then add as many connectors as you need. Each additional connector can have its own one-time credential, so existing connectors remain online when another host is added.

The dashboard offers two Docker setup modes when adding a connector:

### Automatic updates

The generated Compose stack includes a WUD updater sidecar. WUD watches the tunnel client's mutable `:latest` image by digest and recreates that client when a new image is available.

Only the updater sidecar receives the Docker socket:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

> Docker socket access is highly privileged and can effectively control the Docker host. Use this mode only on hosts where that access is acceptable.

### Manual updates

No Docker socket is mounted. The connector continues to use `:latest`, but you decide when it is recreated:

```bash
docker compose pull
docker compose up -d
```

Tunnel credentials remain unchanged during normal client or plugin upgrades.

## Build

Requirements: Go 1.23+

```bash
go build -o zoraxy-tunnel .
go build -o tunnel-client ./client
```

## Zoraxy plugin installation

Zoraxy 3.2.0+ is recommended. Add this repository's plugin index as a community source:

```text
https://raw.githubusercontent.com/MiranoVerhoef/zoraxy-tunnel/refs/heads/main/directories/index2.json
```

The plugin store handles the correct binary for the Zoraxy host platform.

## Client

The dashboard generates the correct command after a tunnel or connector credential is created. A client uses:

```text
--server HOST[:9443]
--token TOKEN
--fingerprint SHA256_FINGERPRINT
--connector-id UNIQUE_CONNECTOR_ID
```

Use a different stable connector ID for each redundant host.

## License

This project remains licensed under the repository's existing MIT license and is based on the original `sniffingsugar/zoraxy-tunnel` project.
