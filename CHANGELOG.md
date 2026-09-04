# Changelog
All notable changes to this project will be documented in this file.

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
