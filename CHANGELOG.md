# Changelog
All notable changes to this project will be documented in this file.

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
- Client observability with reported client version, hostname, operating system, architecture and remote address.
- Live per-tunnel connection telemetry for uptime, last activity, request count, active streams, transferred tunnel bytes and reconnect count.
- Client/plugin version mismatch indicators in the dashboard.
- `tunnel-client --version` for quickly checking the installed client build.

### Changed
- Extend the client authentication handshake with optional telemetry metadata while keeping older tunnel clients compatible.
- Keep the v1.5.1 wide two-column dashboard layout and surface telemetry inside each expanded tunnel instead of adding more top-level panels.
- Keep connection metrics in memory only; counters reset when the plugin restarts and no tunnel tokens or service secrets are exposed through telemetry.

## [1.5.1] - 04.09.26
### Changed
- Restore the wider v1.4-style two-column dashboard layout so Control Node and Tunnels make better use of horizontal space.
- Keep Control Node collapsed by default with its server address and ports visible in the header summary.
- Keep each tunnel/client connection collapsible, with connection state and service count visible while collapsed.
- Keep individual services collapsible inside each tunnel for a cleaner overview without losing detailed controls.
- Use the enhanced plugin icon in the dashboard and ensure the icon is written to the plugin executable directory, which is where Zoraxy loads the plugin-bar icon.
- Add an icon cache-busting version to the custom store entry.

## [1.5.0] - 04.09.26
### Changed
- Refine the product description to: **Secure self-hosted reverse tunneling for Zoraxy with automated routing, managed clients, and granular TLS controls.**
- Use the enhanced tunnel icon in the dashboard header instead of the previous generic inline mark.
- Collapse the Control Node section by default while keeping server address and port information visible in its summary.
- Collapse each tunnel connection by default, with online state, service count, enabled state, token hint and connection ID visible at a glance.
- Add collapsible service details inside each tunnel so host, client target, route state, TLS verification and TAG information stay available without cluttering the page.
- Preserve expanded tunnel and service state during the dashboard's automatic refresh cycle.
- Keep all existing route installation, certificate, client download, token, service editing and TLS controls available from the expanded views.

## [1.4.0] - 04.09.26
### Changed
- Rename the plugin to **Zoraxy Tunnel Enhanced** while keeping the independent plugin ID `com.miranoverhoef.zoraxy-tunnel`.
- Redesign the dashboard with at-a-glance client, tunnel, control-port and ingress-port status.
- Make the control-node settings more compact and easier to scan.
- Replace the old tunnel table layout with clearer tunnel and service cards.
- Add a dedicated client download dialog and improve the generated client commands.
- Keep the existing Zoraxy light/dark theme integration.

## [1.3.0] - 26.08.26
### Changed
- Split the fork into its own Zoraxy plugin identity: `com.miranoverhoef.zoraxy-tunnel`.
- Publish as `Zoraxy Tunnel - Mirano` by Mirano Verhoef.
- Move plugin metadata, release links, UI downloads, and Docker examples to `MiranoVerhoef/zoraxy-tunnel`.
- Publish the client image as `ghcr.io/miranoverhoef/zoraxy-tunnel-client`.
- Add an independent Zoraxy plugin-store index.

## [1.2.0] - 26.08.26
### Added
- Edit existing registered services.
- Per-service Skip TLS certificate verification option.
- Configurable default Zoraxy TAG with per-service override.
- Automatic TAG assignment to installed Zoraxy routes.

### Fixed
- Improve light-mode text contrast and control colors.

## [1.1.1] - 01.08.26
### Fixed
- Config and TLS cert no longer disappear when the plugin runs under Docker.
- Declare the ACME API endpoints (Added into .introspec but missed in main.go)

## [1.1.0] - 01.08.26
### Added
- Offer to issue an SSL cert when installing a route.

### Fixed
- Client download button now offers a per-platform dropdown with the correct binary.

## [1.0.0] - 31.07.26
- Added Project
