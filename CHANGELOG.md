# Changelog
All notable changes to this project will be documented in this file.

## [1.12.0] - 14.09.26
### Fixed
- Restore automatic Docker updater startup with WUD 9.x by generating the required administrator credentials in every new Automatic Compose configuration.
- Authenticate dashboard **Update now** requests to the WUD API instead of relying on the anonymous API access removed in WUD 9.
- Older automatic connectors that do not provide updater-control credentials are now identified explicitly instead of exposing a button that cannot work.

### Added
- Per-connector random WUD administrator password generated only when Automatic update mode is selected.
- The tunnel client securely reports updater-control credentials over the existing TLS-protected control connection; the plugin keeps them in memory only and never returns them through telemetry or the dashboard API.
- Connector telemetry now reports whether authenticated updater control is available, without exposing any credential values.

### Changed
- Automatic Compose setup configures matching WUD and tunnel-client updater credentials automatically; users no longer have to add WUD authentication variables themselves.
- CI now runs automatically on `main` and pull requests instead of every intermediate `release/**` commit. This avoids expected red builds while multi-file release metadata is still being assembled.
- CI uses concurrency cancellation and clearer metadata mismatch messages.
- Plugin and client version bumped to v1.12.0.

## [1.11.0] - 14.09.26
### Added
- Live connector status refresh every five seconds, with immediate refresh when the dashboard regains focus.
- First tunnel setup now offers the same Automatic or Manual update choice as additional connector enrollment.
- Automatic Docker connectors can expose an **Update now** action from their connector row.

### Changed
- Primary dashboard actions such as Download client, Create tunnel, and Add connector use stronger interactive button styling.
- Automatic connector Compose generation includes the updater-control endpoint required by the dashboard.

## [1.10.2] - 14.09.26
### Fixed
- Replace the oversized/cropped plugin icon with a compact padded square asset suitable for Zoraxy plugin and sidebar rendering.
- Serve the dashboard icon locally from the embedded plugin instead of loading it from raw GitHub.
- Cache-bust dashboard CSS, JavaScript and icon assets per plugin version so updates do not leave stale UI resources in the browser cache.

## [1.10.0] - 14.09.26
### Added
- Non-destructive **Add connector** flow for adding redundant hosts without rotating or invalidating credentials already in use.
- Independent per-connector credentials. The original tunnel credential remains valid for backwards compatibility, while every additional connector can receive its own one-time token.
- Connector enrollment wizard with two Docker update modes: **Automatic updates** and **Manual updates**.
- Automatic-update Compose option using a dedicated What's Up Docker (WUD) sidecar. Only the updater receives `/var/run/docker.sock`; the tunnel client itself does not.
- Explicit Docker-socket security warning in the enrollment wizard.
- New Zoraxy-inspired bidirectional tunnel icon based closely on the original Zoraxy mark.

### Changed
- Connector setup now guides users through connector ID and update behavior before showing installation commands.
- Automatic mode tracks the mutable `:latest` client image by digest and recreates the connector when a new image is available.
- Plugin and client version bumped to v1.10.0.

## [1.9.0] - 14.09.26
### Added
- Automatic service health checks through the active tunnel connector every 30 seconds.
- Per-service Healthy, Unhealthy, Offline and Disabled status in the tunnel UI, including response latency when available.
- Health checks use the same target and TLS verification settings as real tunneled traffic and treat reachable HTTP 1xx-4xx responses as healthy.
- Persistent Activity view for connector lifecycle, preferred-connector changes, authentication failures and service health transitions.
- Activity history is stored locally in `events.json` and retains up to 500 events across plugin restarts.

### Changed
- Health probes follow the current Primary connector and therefore automatically follow failover/failback behavior.
- Plugin and client version bumped to v1.9.0 while keeping tunnel credentials and connector IDs unchanged.
- Product description now highlights redundant connectors and service health monitoring.

## [1.8.0] - 09.09.26
### Added
- Multiple simultaneous connectors per tunnel for active/standby redundancy using one persistent tunnel credential.
- Stable `--connector-id` support so each host can be tracked independently and reconnect without replacing other connectors.
- Preferred connector selection with automatic failback: standby takes over when the preferred connector is unavailable, and new traffic returns to the preferred connector when it reconnects.
- Per-connector telemetry for hostname, platform, version, remote address, uptime, activity, traffic and reconnect count.
- Connector management UI showing Primary, Standby, Preferred and Offline states under each tunnel.
- CI validation for Go tests/builds, browser JavaScript and plugin-store metadata.

### Changed
- Tunnel rows now aggregate the status and traffic of all connectors while showing the currently active connector.
- Generated CLI, Docker and Compose examples include a stable connector ID; redundant hosts reuse the same token with different connector IDs.
- Action menus no longer contribute hidden overflow to the tunnel table, removing the unwanted vertical scrollbar while retaining viewport-level floating menus.

## [1.7.2] - 09.09.26
### Fixed
- Render tunnel and service action menus in a viewport-level floating layer so they are no longer clipped by the tunnel table, expanded rows, or pagination footer.
- Automatically position action menus above or below their trigger depending on available viewport space.
- Close floating menus on outside click, Escape, scrolling, or window resize for more predictable interaction.

## [1.7.1] - 09.09.26
### Changed
- Redesign the plugin dashboard to match the compact Zoraxy management-table concept with a clear header, Control Node summary, tabs, search, filtering and pagination.
- Present tunnels as dense management rows with status, service count, uptime, version, last activity and traffic visible without expanding them.
- Show published services in compact nested rows with route state, target information, TLS warnings and focused action menus.
- Move Control Node configuration into a dedicated Settings tab while keeping the active endpoint and ingress port visible at the top of the dashboard.
- Make credential regeneration an explicit destructive action instead of presenting it as a normal update workflow.
- Generate Docker and Compose setups with `ghcr.io/miranoverhoef/zoraxy-tunnel-client:latest`; Compose also uses `pull_policy: always` so the configuration does not need to change for every client release.
- Keep tunnel credentials stable across normal plugin and client updates.

## [1.7.0] - 09.09.26
### Added
- Guided Control Node setup with endpoint validation and a live client-endpoint preview.
- Automatic control-port handling when a hostname or IP address is entered without a port.
- Client-side server address normalization so `--server host` automatically uses port `9443` instead of failing with a missing-port error.

### Changed
- Redesign the dashboard into a cleaner, denser full-width management layout.
- Replace the four large status cards with a compact status bar.
- Make Control Node a compact full-width collapsible section instead of a permanent side column.
- Make tunnel and service rows smaller and easier to scan, while keeping detailed controls available when expanded.
- Simplify client telemetry into four primary metrics with secondary connection details underneath.
- Block tunnel creation until the Control Node endpoint is valid so generated client commands cannot contain an unusable address.
- Use only generic example hostnames in UI placeholders and documentation-style hints.

## [1.6.0] - 08.09.26
### Added
- Active connector failover and preferred connector selection.

## [1.5.0] - 08.09.26
### Changed
- Refined dashboard description and plugin icon handling.
