# Phase 1.3: Linux CLI Backend Plan

## Objective

Deliver the first usable implementation on Linux by connecting the portable PAX tar creator to GnuPG encryption, `par2cmdline` local integrity recovery, and `xorriso` image generation, optional BD writing, and verification.

The image-only workflow is the required deliverable. Direct burning is enabled only after it passes the hardware validation gate in this plan.

## Prerequisites

- Phase 1.1 project setup is complete.
- Phase 1.2 portable core is complete and fully testable with fake adapters, including its minimal Linux filesystem classifier.
- A Linux test host, an external BD writer, known firmware revision, and supported test media are available for optical validation.

## Completion Criteria

- Linux discovers and validates required `gpg`, `par2`, and `xorriso` executables before creation starts.
- The CLI creates encrypted archive images from real source data and verifies the archive, PAR2 files, manifests, and image.
- The output can be extracted from an available image using the recovery instructions and bundled recovery material.
- Direct burn and read-back verification remain unavailable unless the tested drive/media combination is recognized by the backend's compatibility policy.
- The Linux hardware matrix records tested writer model, firmware, media type/capacity, tool versions, write outcome, and read-back verification outcome.

## Dependency Policy

| Capability | Initial Linux Tool | Required Behavior |
|---|---|---|
| Archive creation | Go standard-library PAX tar | Stream and validate archive payloads with exact source byte ranges. |
| Encryption | GnuPG (`gpg`) | Symmetrically encrypt the completed archive using AES-256 and a passphrase file descriptor. |
| Local integrity recovery | `par2cmdline` | Create, verify, and repair PAR2 files for the final encrypted payload. |
| Image generation and verification | `xorriso` | Generate the selected supported ISO profile and verify its recorded checksums/content. |
| Direct optical operations | `xorriso` | Discover writer, inspect media, write only after preflight, and read-back verify. |

Do not automatically install, download, or upgrade these tools. Discovery reports their resolved paths, versions, missing capabilities, and installation guidance. The detailed implementation must document the minimum tested versions and supported distributions.

## Work

### [x] 1. Implement Linux Tool Discovery

1. Discover `gpg`, `par2`, and `xorriso` using an explicit configured path first and `PATH` second.
2. Query each executable's version and supported options using non-destructive commands.
3. Convert discovery results into the portable capability model, including archive creation, encryption, PAR2 create/verify/repair, image generation, writer discovery, burning, and image/media verification.
4. Fail the requested operation before staging output when its required capability is absent.
5. Record resolved tool paths and versions in the final report and per-disc metadata where appropriate.

### [x] 2. Implement Archive and PAR2 Adapters

1. Stream PAX tar entries from typed adapter input, including exact source byte ranges and archive-relative symlinks.
2. Validate the resulting PAX tar stream before local recovery data is generated.
3. Use GnuPG symmetric AES-256 encryption with `--batch`, `--pinentry-mode loopback`, and a dedicated passphrase file descriptor. Ensure the passphrase is not visible in process arguments, logs, events, reports, or persisted files.
4. Verify that the encrypted artifact can be decrypted and that the recovered archive passes its archive test before PAR2 generation.
5. Generate PAR2 files over the final GnuPG-encrypted archive artifact only.
6. Verify the generated PAR2 set and test repair using a disposable copy with controlled byte corruption.
7. Define the recovery-tool bundle for the selected archive, encryption, and PAR2 formats, including licence-compliant distribution or source/retrieval material. Do not claim a bundled binary is portable unless it is tested on its advertised target.

### [x] 3. Implement Image Adapter

1. Select the initial `xorriso` image profile and document its filesystem, filename, file-size, and compatibility limits.
2. Expose those limits to the portable capacity planner so it can reserve correct overhead and split payload artifacts when needed.
3. Create an image from a staged per-disc layout containing encrypted payload, PAR2 files, manifests, recovery instructions, and the recovery-tool bundle.
4. Verify the created image by checking the expected contents and recorded checksums before publishing it to the final output folder.
5. Test mounting/extracting the image on Linux and performing the documented single-disc recovery procedure.

### [x] 4. Implement Optional Direct Burn Adapter

1. Enumerate candidate writers through `xorriso` without modifying media.
2. Inspect inserted media and reject blank, non-writable, incompatible, undersized, or unexpectedly non-empty media according to an explicit overwrite policy.
3. Require explicit user confirmation or a non-interactive force/confirmation mechanism before any destructive write.
4. Write only a previously verified image and record the selected device, media details, tool version, and image hash.
5. Perform read-back verification after writing. A successful write command alone is not a successful archive operation.
6. Surface partial or failed verification as a failed result and preserve the original image for retry/diagnosis.

### [x] 5. Establish Hardware Validation

1. Create a version-controlled hardware compatibility matrix document or data file.
2. For each tested combination, record Linux distribution/kernel, writer vendor/model, USB enclosure/bridge where applicable, firmware, `xorriso` version, media manufacturer/type/capacity, requested speed, and outcome.
3. Test image-only output before optical tests.
4. Test blank media preflight, a successful write, a read-back verification, mounted-image extraction, and full single-disc recovery.
5. Test expected failures, including no writer, no media, undersized media, incompatible media, and interrupted write.
6. Enable direct burning only for combinations that pass this matrix. Keep image-only mode available for all Linux hosts.

## Test Strategy

1. Use fake process runners for command construction, capability parsing, error mapping, cancellation, and redaction tests.
2. Use temporary files and the real installed GnuPG/`par2` tools for opt-in integration tests; do not require them for ordinary unit tests.
3. Use generated small fixtures for image integration tests. Do not add large archive or optical-image binaries to the repository.
4. Separate hardware tests from CI. They require explicit operator execution and record results in the hardware matrix.
5. Run the Go race detector for process/event orchestration if the implementation processes multiple discs concurrently.

## Verification

1. On a Linux host with the dependencies installed, create a multi-image archive from a synthetic source set using an arbitrary capacity target.
2. Verify every final image, mount one image, validate its manifest/PAR2 data, decrypt its archive, and extract its payload without using other images.
3. Corrupt a disposable encrypted payload copy and demonstrate PAR2 verification and repair within the configured local recovery budget.
4. On a matrix-approved drive/media pair, burn a verified image and complete read-back verification.
5. Confirm a missing or unsupported writer does not prevent image-only archive creation.

## Explicitly Deferred

- Cross-disc parity and reconstruction of wholly lost discs.
- Windows and macOS adapters.
- GUI integration.
- Packaged distribution, signing, and release publishing.
- Automatic external-tool installation or update management.
