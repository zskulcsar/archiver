# Phase 1.2: Portable Core CLI Plan

## Objective

Implement the portable Go CLI behavior for planning and creating independently recoverable, encrypted archive images in image-only mode. The core orchestrates external-tool adapters but contains no Linux optical-drive code.

## Prerequisites

- Phase 1.1 project setup is complete.
- The command, event, error, domain, and adapter boundaries exist.

## Completion Criteria

- The CLI accepts a source directory or a versioned manifest of absolute file paths.
- The CLI accepts arbitrary capacity targets expressed as exact bytes or unambiguous binary units.
- It produces an inspectable plan before any archive artifacts are created.
- It allocates each disc as a separate complete archive payload; it never creates one archive volume spanning multiple discs.
- It orchestrates archive/encryption, local PAR2, image-generation, and verification adapters through stable contracts.
- It creates output only through a staging area and leaves final output untouched on failure.
- It emits both terminal output and stable JSON Lines events without exposing secrets.
- All core behavior is testable without installed external tools or optical hardware.

## Command Surface

Define the exact flags in the detailed implementation design, but reserve these operations now:

| Operation | Responsibility |
|---|---|
| `archiver backends` | Report discovered adapters and their capabilities. |
| `archiver plan` | Validate inputs and print/write the archive allocation plan without creating artifacts. |
| `archiver create` | Execute a previously validated configuration to create and verify disc images. |
| `archiver verify` | Verify an existing archive image or staged disc layout using its manifest. |

`plan` and `create` accept exactly one source-selection mechanism: a directory or `--manifest`. The manifest remains the GUI integration boundary. Password material is supplied via a prompt or secure standard-input/file-descriptor mechanism, never a command-line argument, environment variable, event, plan, or log.

## Work

### [ ] 1. Define Versioned Portable Formats

1. Define a manifest schema that records absolute source paths, file identity metadata, and optional precomputed hashes.
2. Define an archive-set configuration schema containing capacity, image profile, encryption configuration reference, local parity policy, output policy, and format version.
3. Define an archive plan schema containing ordered disc assignments, expected artifact sizes, reserved overhead, and the source-file parts created by splitting.
4. Define per-disc and archive-set manifests. They identify the archive set, disc number, payload files, hashes, tool formats and versions, recovery instructions, and recovery-tool bundle identity.
5. Version all persisted schemas from their first use. Reject unsupported future versions rather than guessing their semantics.

### [ ] 2. Implement Input Validation and Snapshotting

1. Resolve and validate every source path before allocation.
2. Reject duplicate, missing, unreadable, non-regular, or changed source files according to the selected manifest policy.
3. Record size and modification metadata at plan time; revalidate immediately before data is read during creation.
4. Define symlink behavior explicitly before implementation. The initial behavior should be conservative and must not permit paths to escape the selected source policy unexpectedly.
5. Validate output paths, ensure staging and final output are distinct, and prevent accidental overwrite unless an explicit policy permits it.

### [ ] 3. Implement Capacity-Aware Planning

1. Parse arbitrary capacity values to exact bytes and reject ambiguous, negative, or impractical values.
2. Obtain image-profile constraints from the selected image adapter, including effective capacity, filesystem overhead, supported file-size limits, and required recovery-tool bundle size.
3. Reserve overhead before allocating payload data. Never plan against nominal media capacity alone.
4. Split an oversized input file into ordered recoverable parts when it cannot fit in one independent disc payload. Preserve enough metadata to reassemble it after extraction.
5. Allocate sources deterministically into separate disc units. Define and test the selected packing policy, including stable ordering and treatment of equal-size files.
6. Report unallocatable input and the exact capacity shortfall before archive creation.

### [ ] 4. Define Adapter Contracts and Orchestration

1. Define narrow interfaces for archive creation, encryption, PAR2, image generation, recovery-tool bundling, and verification.
2. Keep writer discovery, burning, and read-back verification as optional capabilities; the portable create path requires only image creation and image verification.
3. Require adapters to return structured capabilities, command version, output artifact paths, actual sizes, hashes where available, and actionable failure details.
4. Use application orchestration to sequence: stage disc layout, create archive payload, encrypt it, generate local PAR2 over the final encrypted payload, write manifests/instructions/tool bundle, create image, then verify the image.
5. Do not calculate parity over plaintext. Do not retain temporary plaintext archives after the encrypted artifact has been verified.
6. Keep an incomplete staging directory for diagnosis only when explicitly requested; otherwise clean it on success and failure according to the configured safety policy.

### [ ] 5. Implement Reporting, Cancellation, and Recovery Instructions

1. Emit lifecycle events for validation, planning, staging, archive creation, parity, image creation, verification, cleanup, and completion.
2. Make event payloads stable, documented, and free of passphrases or plaintext file names unless the caller explicitly requests a privacy-reducing detail level.
3. Propagate cancellation through contexts to every adapter process and leave final output absent or clearly incomplete.
4. Generate per-disc recovery instructions from the actual selected formats and tool bundle, rather than hard-coding Linux shell commands.
5. Produce a final archive-set report with planned and actual capacities, generated image paths, hashes, tool versions, and verification outcomes.

## Test Strategy

Implement the portable core test-first. Use fake archive, PAR2, image, verification, and recovery-tool adapters for unit and application tests. Reserve real-tool tests for Phase 1.3.

1. Unit test capacity parsing, overhead reservation, deterministic allocation, and oversized-file splitting with table-driven cases.
2. Unit test manifest validation for missing, changed, duplicate, and unreadable source entries.
3. Application test that each planned disc creates one independent archive payload and never references another disc's payload during normal recovery.
4. Application test that parity receives GnuPG-encrypted artifact paths, not plaintext or unencrypted archive inputs.
5. Application test that a failure at every orchestration stage does not publish a final image and handles staging according to policy.
6. Contract test JSON Lines event order, required fields, exit-code mapping, and secret redaction.
7. Cross-compile and run portable tests without Linux-only build tags or direct process dependencies.

## Verification

1. A synthetic manifest with files that span several arbitrary capacities produces a stable plan on repeated runs.
2. A fake end-to-end create operation produces independently structured disc layouts, manifests, instructions, and verified placeholder images.
3. Altering a source after planning causes creation to stop before archive creation.
4. A simulated archive, parity, image, or verification failure does not leave an apparently complete output set.
5. The core compiles for Linux, Windows, and macOS targets before any OS-specific adapter is linked.

## Explicitly Deferred

- Concrete `7zz`, GnuPG, `par2cmdline`, and image-tool process adapters.
- Optical writer discovery, burning, and read-back verification.
- Validation with real optical media.
- Windows and macOS adapter implementations.
- Cross-disc parity that reconstructs a wholly lost disc.
