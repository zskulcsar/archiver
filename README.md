# Archiver

Archiver creates encrypted, independently recoverable multi-disc archive images.

The initial development target is Linux. The portable CLI core is designed to support Windows and macOS adapters later.

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

External archive, encryption, parity, image, and optical-writing tools are not required for the project setup phase. They are discovered from the local system in later phases; Archiver does not download or execute them automatically.

Small synthetic test fixtures may be committed. Generated archives, images, and large fixtures must not be committed.

## Repository Layout

- `cmd/archiver`: CLI composition root.
- `internal`: portable domain, application, adapter contracts, and future platform implementations.
- `apps`: reserved for future native GUIs.
- `packaging`: reserved for future OS-specific packages.
- `docs/plan`: product and implementation plans.
- `docs/cli-conventions.md`: CLI output, exit-code, and build-identity conventions.
