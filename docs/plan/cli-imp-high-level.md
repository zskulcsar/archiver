# Cross-Platform CLI High-Level Implementation Plan

## Purpose

Deliver a Go command-line application that turns a source directory or file manifest into independently recoverable, encrypted multi-disc archive images. The CLI is the product core: later native GUIs invoke it and consume its structured progress output.

This plan defines product boundaries and implementation sequencing. Detailed plans for the portable core and the Linux, Windows, and macOS backends will follow separately.

## Scope and Decisions

The first release implements the following decisions from the archive workflow specification:

- Support Linux, Windows, and macOS from one Go codebase with OS-specific binaries.
- Accept either a source directory or a manifest of absolute source paths.
- Accept arbitrary media capacity targets. Common optical-media capacities are presets, not restrictions.
- Build independent per-disc archive payloads. An available disc must be recoverable without the rest of the archive set.
- Treat a wholly lost disc as unrecoverable in the first release. Cross-disc recovery parity is a later optional policy.
- Use proven external tools for archive/encryption, PAR2 integrity protection, image creation, and optical operations where practical.
- Always support image-only output to a folder. Direct optical writing is optional and depends on a validated OS-specific backend.

The first release does not implement native archive, encryption, PAR2, or optical-writing formats in Go, a GUI, cloud ingestion, fulfilment services, or cross-disc parity recovery.

## Product Contract

The CLI must provide commands or equivalent operations for the following lifecycle:

1. Inspect available external-tool backends and their capabilities.
2. Validate source input, capacity, output location, and requested policy before modifying output.
3. Produce a deterministic archive plan showing each disc's assigned source files, reserved overhead, and expected image size.
4. Create one encrypted archive payload per planned disc.
5. Generate integrity metadata and optional local PAR2 recovery data for each disc's final encrypted payload.
6. Produce a disc image containing the payload, recovery metadata, recovery instructions, required manifests, and a format-appropriate recovery-tool bundle.
7. Verify generated artifacts before reporting success.
8. When supported and explicitly requested, burn and read-back verify each image through an OS-specific backend.

The CLI must support non-interactive use and human-readable terminal output. It must also emit a stable machine-readable event stream, such as JSON Lines, for GUI callers. Secrets must never be accepted as ordinary command-line arguments or written to logs, events, manifests, or plans.

## Core Workstreams

- [ ] [Phase 1.1: Project setup](p1-linux-core/01-project-setup.md#work)
- [ ] [Phase 1.2: Portable core CLI](p1-linux-core/02-core-cli.md#work)

### 1. Command and Input Contract

Define the stable command surface, configuration model, exit codes, structured events, and error categories. Define source-directory and manifest schemas, including file path, expected size, modification time, and optional content hash. The CLI must reject changed, missing, unreadable, or out-of-policy source files before archive creation.

### 2. Archive Planning

Implement capacity-aware allocation using exact byte values and explicit overhead reservations for archive artifacts, integrity data, manifests, recovery instructions, and the image filesystem. The planner must split oversized source files into recoverable parts when necessary and allocate each disc as a separate complete archive unit. Planning must be inspectable before any expensive archive or image operation starts.

### 3. Artifact and Recovery Layout

Define a versioned per-disc layout and a versioned archive-set manifest. Every disc image must contain enough metadata to identify the archive set, its own disc number, its payload, the selected archive/encryption/PAR2 formats, integrity hashes, recovery instructions, and a recovery-tool bundle suitable for the selected format. The format must allow later recovery tooling to validate and extract a single available disc without needing the full set.

### 4. External Tool Adapters

Define a small capability-based adapter boundary for external programs. The portable core asks adapters to create/extract encrypted archives, create/verify/repair PAR2 data, generate an image, enumerate writers, burn media, and read-back verify media. Adapters must report executable discovery, version, supported capabilities, invocation failures, and parsed verification results consistently.

The initial adapter selection should favor 7-Zip archive creation, GnuPG encryption, and `par2cmdline` parity because they are available on all target operating systems. Image and optical adapters are selected per platform.

### 5. Artifact Verification and Reporting

Verify the encrypted archive, PAR2 set, manifests, and generated image before marking a disc complete. Produce a final archive-set report containing planned versus actual sizes, hashes, tool versions, verification status, and locations of all generated images. Retain enough logs and manifests for diagnosis without exposing secrets or source-file contents unnecessarily.

## Platform Workstreams

The portable core must not import operating-system-specific optical-drive code. Each platform plan implements adapters behind the same capability contract.

### Linux

- [ ] [Phase 1.3: Linux CLI backend](p1-linux-core/03-linux-cli.md#work)

Use 7-Zip, GnuPG, and `par2cmdline`, then implement and validate an `xorriso`-based image, writer-discovery, burn, and checksum/read-back verification backend. Define the required device permissions and supported drive/media matrix.

### Windows

Use 7-Zip, GnuPG, and a PAR2-compatible tool, then implement image generation through a supported UDF/ISO backend such as `Oscdimg`. Select and validate a separate BD-XL burner and verification backend. The Windows plan must document installation, licence, executable discovery, and hardware requirements.

### macOS

Use 7-Zip, GnuPG, and `par2cmdline`, then implement image generation through an available image backend such as `cdrtools` or `xorriso`. Treat direct writer discovery, burning, and read-back verification as experimental until validated with specific external drives, firmware, and BD-XL media.

## Delivery Sequence

1. Define the CLI, manifest, planner, event, artifact-layout, and adapter contracts.
2. Implement the portable core in image-only mode with mockable adapter boundaries.
3. Integrate and test cross-platform archive/encryption and PAR2 adapters.
4. Integrate image-generation adapters and verify image contents on Linux, Windows, and macOS.
5. Deliver the Linux optical backend and establish its hardware compatibility matrix.
6. Deliver validated Windows and macOS optical backends independently, without delaying image-only support on either platform.
7. Add GUI applications that invoke the CLI only after the CLI contract and structured events are stable.

## Quality Gates

- The same manifest and configuration must yield the same allocation plan on every supported OS, subject to explicitly recorded external-tool format differences.
- A successfully completed disc image must pass its recorded integrity verification before it is reported as ready to burn.
- A single available disc must be verifiably recoverable using its own contents and documented tooling. A wholly lost disc remains unrecoverable in the first release, as stated in Scope and Decisions.
- The CLI must fail before archive creation when source validation, capacity calculation, dependency discovery, or output preflight fails.
- Image-only workflows must be tested on Linux, Windows, and macOS without optical hardware.
- Direct-burning support must remain disabled for a platform/backend until it passes a documented real-hardware burn and read-back verification matrix.

## Future Work

Future work may add cross-disc parity as a selectable protection policy. It will reserve capacity across the archive set and reconstruct a configured number of wholly lost discs from surviving discs. This is intentionally excluded from the independent per-disc first release.

## Related Documents

- [Archive workflow and tooling specification](../bd-xl-multi-disc-archive-workflow-and-tooling-spec.md)
