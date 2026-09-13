# Archiver

Archiver is a Go CLI for planning and creating encrypted, independently recoverable multi-disc archive images.

The portable planning core is implemented. The initial real archive backend targets Linux; Windows and macOS adapters will follow.

## Current State

Phase 1.2 provides a portable, testable planning core:

- Validates positional source paths or versioned SQLite source manifests.
- Rejects network filesystems and unsafe source inputs; supports archive-relative symlinks and optional materialization of external local symlinks.
- Produces deterministic, capacity-aware multi-disc plans using canonical logical archive-path order.
- Supports decimal capacity values such as `8GB`, local recovery policy through `--loss-tolerance`, and configurable splitting through `--min-split-size`.
- Writes an inspectable `<set-id>.plan.json` in the output parent, without exposing absolute source paths.
- Uses staging directories, no-overwrite preflight, structured JSON Lines events, and actionable precondition failures.

The real 7-Zip, GnuPG, PAR2, and image-generation adapters are deferred to Phase 1.3. As a result, `create` validates and plans its input but stops with exit code `4` before it can create archive media. `verify` also requires the future backend.

## Planning Example

Build the local Linux binary, then plan a source directory:

```sh
make build

./bin/archiver plan "$HOME/Downloads" \
  --archive-name downloads \
  --capacity 8GB \
  --loss-tolerance 20% \
  --min-split-size 0MB \
  --events-file .tmp/downloads-plan.events.jsonl \
  --output .tmp/downloads-output
```

The command prints the generated plan path and writes a layout such as:

```text
.tmp/downloads-output/downloads_2026-09-13_22-02.plan.json
```

The plan file contains the archive-set ID, usable capacity, split policy, disc usage, and each member's logical archive path, byte offset, and size. Existing plan, event, staging, and final-output paths are never overwritten.

## Development

Requirements:

- Go 1.25.3
- `golangci-lint` 2.11.4
- GNU Make

Run the required local checks:

```sh
make verify
```

Build the CLI:

```sh
make build
./bin/archiver --help
```

Cross-compile the portable CLI without platform integrations:

```sh
make build-cross
```

External archive, encryption, parity, image, and optical-writing tools are not required for the portable core. They are discovered from the local system in Phase 1.3; Archiver does not download or execute them automatically.

Small synthetic test fixtures may be committed. Generated archives, images, and large fixtures must not be committed.

## Repository Layout

- `cmd/archiver`: CLI composition root.
- `internal/domain`: portable archive types, capacity and split-policy parsing, and deterministic disc allocation.
- `internal/app`: source validation, output preflight, plan output, archive orchestration, metadata, events, and reporting.
- `internal/adapters`: SQLite source-manifest storage and contracts for archive, encryption, parity, and image backends.
- `internal/cli`: Cobra command parsing, terminal output, event-file handling, and exit-code mapping.
- `internal/platform/linux`: Linux local/network filesystem classifier; real external-tool adapters are Phase 1.3 work.
- `internal/*/*_test.go`: unit and fake-adapter application tests; no real archive tools or optical hardware are required.
- `apps`: reserved for future native GUIs.
- `packaging`: reserved for future OS-specific packages.
- `docs/plan`: product and implementation plans.
- `docs/cli-conventions.md`: CLI output, exit-code, and build-identity conventions.
