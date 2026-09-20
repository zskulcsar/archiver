# Archiver

Archiver is a Go CLI for planning and creating encrypted, independently recoverable multi-disc archive images.

The portable planning core and the initial Linux image-only backend are implemented. Windows and macOS adapters will follow.

## Current State

Phase 1.3 provides portable planning and a Linux image-creation workflow:

- Validates positional source paths or versioned SQLite source manifests.
- Rejects network filesystems and unsafe source inputs; supports archive-relative symlinks and optional materialization of external local symlinks.
- Produces deterministic, capacity-aware multi-disc plans using canonical logical archive-path order.
- Supports decimal capacity values such as `8GB`, local recovery policy through `--loss-tolerance`, and configurable splitting through `--min-split-size`.
- Writes an inspectable `<set-id>.plan.json` in the output parent, without exposing absolute source paths.
- Uses staging directories, no-overwrite preflight, structured JSON Lines events, and actionable precondition failures.

On Linux, `create` discovers installed `gpg`, `par2`, and `xorriso` before staging output. It streams PAX tar payloads, including exact byte ranges for split files, through GnuPG AES-256 encryption with a private `--passphrase-file`, protects encrypted payloads with PAR2, creates ISO9660 Level 3 images, and verifies them before publication. `verify` checks published image integrity. Direct optical writing remains disabled pending an approved hardware compatibility-matrix entry.

When local recovery is enabled, each disc also undergoes a disposable PAR2 repair drill: Archiver copies the encrypted payload and recovery files into staging, corrupts the payload copy, repairs it with PAR2, and confirms the repaired bytes match the original. This provides an end-to-end recovery proof before publication.

## Linux Dependencies

The current image-only backend requires these installed tools:

- `gpg` 2.2 or newer, with loopback pinentry and `--passphrase-fd` support.
- `par2` from par2cmdline 0.8 or newer, with create, verify, and repair support.
- `xorriso` 1.5 or newer, with ISO9660 Level 3 image creation and filesystem inspection support.

Distribution package names commonly include `gnupg`, `par2cmdline`, and `xorriso`. Archiver discovers tools locally and never installs or upgrades them. PAX tar creation uses Go's standard library and has no external archive-tool dependency.

## Planning Example

Build the local Linux binary, then plan a source directory:

```sh
make build

./bin/archiver plan "$HOME/Downloads" \
  --archive-name downloads \
  --capacity 1GB \
  --loss-tolerance 20% \
  --min-split-size 0MB \
  --events-file .tmp/downloads-plan.events.jsonl \
  --output .tmp/downloads-output
```

## Linux Creation Example

Install the required system tools, create a private passphrase file with mode `0600`, then run:

```sh
./bin/archiver create "$HOME/Downloads" \
  --archive-name downloads \
  --capacity 8GB \
  --passphrase-file "$HOME/.config/archiver/passphrase" \
  --output .tmp/downloads-output
```

See `docs/linux-image-profile.md` for supported tool interfaces and image-profile limits. See `docs/linux-hardware-compatibility.md` before considering any optical-media operation.

## Observability

Archiver can export optional OpenTelemetry traces and I/O metrics to an OTLP/HTTP endpoint. See `docs/observability.md` for the local Grafana stack and `--otel-endpoint` usage.

The command prints the generated plan path and writes a layout such as:

```text
.tmp/downloads-output/downloads_2026-09-13_22-02.plan.json
```

The plan file contains the archive-set ID, usable capacity, split policy, disc usage, and each member's logical archive path, byte offset, and size. Existing plan, event, staging, and final-output paths are never overwritten.

## Development

Requirements:

- Go 1.25.13
- `golangci-lint` 2.11.4
- GNU Make

Run the required local checks:

```sh
make verify
```

Inspect dependencies:

```sh
make deps-verify
make deps-outdated

go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
make deps-vuln
```

`deps-vuln` scans reachable code for known vulnerabilities. GitHub Actions installs the pinned scanner and runs it before building.

Build the CLI:

```sh
make build
./bin/archiver --help
```

Cross-compile the portable CLI without platform integrations:

```sh
make build-cross
```

External archive, encryption, parity, and image tools are required only for Linux `create` and `verify`. They are discovered from the local system; Archiver does not download, install, or upgrade them automatically.

Small synthetic test fixtures may be committed. Generated archives, images, and large fixtures must not be committed.

## Repository Layout

- `cmd/archiver`: CLI composition root.
- `internal/domain`: portable archive types, capacity and split-policy parsing, and deterministic disc allocation.
- `internal/app`: source validation, output preflight, plan output, archive orchestration, metadata, events, and reporting.
- `internal/adapters`: SQLite source-manifest storage and contracts for archive, encryption, parity, and image backends.
- `internal/cli`: Cobra command parsing, terminal output, event-file handling, and exit-code mapping.
- `internal/platform/linux`: Linux filesystem classifier, tool discovery, secure passphrase-file validation, and external-tool image-only adapters.
- `internal/*/*_test.go`: unit and fake-adapter application tests; no real archive tools or optical hardware are required.
- `apps`: reserved for future native GUIs.
- `packaging`: reserved for future OS-specific packages.
- `docs/plan`: product and implementation plans.
- `docs/cli-conventions.md`: CLI output, exit-code, and build-identity conventions.
