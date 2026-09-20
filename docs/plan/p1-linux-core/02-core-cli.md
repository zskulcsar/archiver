# Phase 1.2: Portable Core CLI Plan

## Objective

Implement the portable Go CLI behavior for planning and creating independently recoverable, encrypted archive images in image-only mode. The core orchestrates external-tool adapters but contains no Linux optical-drive code.

## Prerequisites

- Phase 1.1 project setup is complete.
- The command, event, error, domain, and adapter boundaries exist.

## Completion Criteria

- The CLI accepts positional source paths or a versioned SQLite manifest of absolute file paths.
- The CLI accepts arbitrary capacity targets expressed as strict whole-number `MB` or `GB` values.
- It produces an inspectable plan before any archive artifacts are created.
- It allocates each disc as a separate complete archive payload; it never creates one archive volume spanning multiple discs.
- It orchestrates archive/encryption, local PAR2, image-generation, and verification adapters through stable contracts.
- It creates output only through a staging area and leaves final output untouched on failure.
- It emits both terminal output and stable JSON Lines events without exposing secrets.
- All core behavior is testable without installed external tools or optical hardware.

## Command Surface

The first-release command surface is:

| Operation | Responsibility |
|---|---|
| `archiver backends [--events-file FILE]` | Report discovered adapters and their capabilities. |
| `archiver plan --capacity CAPACITY --output DIR [--archive-name NAME] [--manifest FILE \| SOURCE...] [--loss-tolerance PERCENT] [--min-split-size THRESHOLD] [--external-symlink=materialize] [--events-file FILE]` | Validate inputs, print the archive allocation plan, and write a new `<set-id>.plan.json` in the output parent. |
| `archiver create --capacity CAPACITY --output DIR [--archive-name NAME] [--manifest FILE \| SOURCE...] [--loss-tolerance PERCENT] [--min-split-size THRESHOLD] [--external-symlink=materialize] [--passphrase-file FILE] [--events-file FILE]` | Validate, create, and verify disc images. |
| `archiver verify ARCHIVE_SET_DIR [--events-file FILE]` | Verify an existing published archive set using its manifest. |

`plan` and `create` accept exactly one source-selection mechanism: one or more positional source paths or `--manifest FILE`. The manifest remains the GUI integration boundary. Source input requires `--archive-name`; manifest input rejects that option because it derives the name from the manifest filename. `--external-symlink` defaults to rejection and accepts only `materialize` when supplied.

`create` prompts for a passphrase from a TTY when `--passphrase-file` is absent. `--passphrase-file FILE` reads the passphrase from a local regular file and is the non-interactive interface. The file must be owned by the invoking user and must not grant any group or other access; the platform adapter must enforce the equivalent access-control policy where POSIX mode bits do not apply. The passphrase file is read once and never copied, logged, included in events, plans, manifests, or output media. It is the caller's responsibility to remove it after use.

`--events-file FILE` writes the stable JSON Lines event stream to a new local file. It refuses an existing path and must not be inside the archive-set staging or final output directory. Human-readable output remains on the terminal; diagnostics remain on standard error.

## Manifest Storage

The GUI-to-CLI input manifest and the CLI working manifest are versioned SQLite database files, conventionally named `manifest.sqlite`. SQLite supports millions of source records, indexed validation and allocation queries, transactional updates, and batch processing without loading the complete manifest into memory.

The manifest records absolute source paths, source-file identity metadata, optional content hashes, generated file parts, and disc allocations. Its schema must include an explicit format version and reject unsupported future versions rather than attempting to interpret them.

The GUI-created plaintext manifest remains local because absolute paths and file metadata may expose private information. The CLI must not copy it to output media by default. Any archive-set or per-disc metadata that retains source paths must be encrypted; metadata not needed for recovery should be omitted from the archive instead.

The Phase 1.2 implementation uses `modernc.org/sqlite`, a CGo-free SQLite driver that supports the target Linux, Windows, and macOS architectures. Pin a stable v1 release when the manifest implementation is added. Do not override its transitive `modernc.org/libc` dependency independently from the selected SQLite release.

## Logical Archive Paths

Every source entry has an absolute source path and a separate logical archive path. The CLI derives the logical paths from the deepest common directory ancestor of normalized source paths. Paths are stored relative to that ancestor, omitting the leading filesystem root. The common ancestor is always a directory, so a selection containing one file preserves that file's basename as its logical archive path.

Materialized external symlink targets remain beneath the logical archive path of the symlink that introduced them. They do not alter the common ancestor or the logical paths of existing source entries.

Windows selections spanning local volumes have no common filesystem ancestor. The CLI creates a virtual archive root and prefixes each logical path with a portable volume identifier, such as `C_/Users/alice/photos/a.jpg` and `D_/archive/report.pdf`. The source manifest retains original volume identity for validation, while archive paths avoid Windows-only `C:` syntax.

Canonical logical archive paths use `/` as the separator, preserve original filename case, and contain no leading root, `.` segment, or `..` segment. The planner sorts all source entries by bytewise UTF-8 lexical comparison of this path; SQLite row insertion order and GUI collection order have no effect. Path-case behavior is filesystem-dependent, so platform adapters detect case-insensitive source collisions without changing the original archive-path spelling.

## Source Filesystem and Symlink Policy

The first release accepts local filesystems only. The CLI must reject a source path, manifest entry, symlink target, or recursively collected child that resides on a network filesystem. Validation failures must identify the logical archive path, resolved filesystem path, and detected network filesystem so the operator can correct the source selection. No override is provided, including when an initially selected source path is network-backed.

When a symlink resolves to a target already represented by an archive member, the CLI preserves the link using an archive-relative target. It records the original link target for traceability but must not retain an absolute source-machine target that would be invalid after recovery.

An external symlink is reported and rejected by default. `--external-symlink=materialize` permits the CLI to materialize a local target at the symlink's logical archive path:

- A regular-file target is stored as a regular archive file.
- A directory target is recursively collected beneath the symlink's logical archive path.
- Nested symlinks apply the same policy.
- The manifest records each materialized entry, its original link target, and its materialization status.

Dangling links and links to devices, FIFOs, sockets, or other unsupported target types are rejected. Directory traversal must detect cycles using physical directory identity rather than path text. It must fail, rather than skip a subtree, when it encounters a network filesystem.

The portable core defines this policy and consumes local/network filesystem classification through an adapter contract. Phase 1.2 provides test doubles for this contract and a minimal Linux implementation so real Linux source-directory workflows enforce the local-filesystem-only policy. Windows and macOS implementations remain deferred to their platform backends.

## Capacity Input

The CLI accepts positive whole-number media capacities using this exact, case-sensitive grammar:

```text
^[1-9][0-9]*(MB|GB)$
```

Examples include `700MB`, `25GB`, `100GB`, and `128GB`. Whitespace, fractional values, raw byte counts, lowercase units, and binary units such as `MiB` or `GiB` are rejected.

`MB` means 1,000,000 bytes and `GB` means 1,000,000,000 bytes. The CLI converts the input to an exact byte count once; adapters do not change the unit meaning. The planner applies filesystem, manifest, recovery, parity, and image overhead to derive the usable payload capacity for the selected backend profile.

## Sequential Allocation and File Splitting

The planner allocates input sequentially; it does not reorder files to minimize the disc count. This preserves an understandable relationship between selected source order and archive-disc order.

All input, including source-directory, manifest, and generated materialized-symlink entries, is ordered by canonical logical archive path using bytewise UTF-8 lexical comparison. Generated entries participate at the logical path where the symlink was encountered.

The minimum split-size policy is configured with `--min-split-size=THRESHOLD` and defaults to `0MB`. The strict grammar is:

```text
^(0|[1-9][0-9]*)(MB|GB|%)$
```

Absolute values use the same decimal units as capacity input. Percentage values are whole numbers from `0%` through `100%`, inclusive, and resolve to `ceil(usable-payload-capacity * percentage / 100)` bytes for each disc. Whitespace, fractions, lowercase units, binary units, bare numbers, and percentages above `100%` are rejected. An absolute threshold greater than usable payload capacity is rejected before allocation because it can make a required split impossible.

`0MB` and `0%` always permit splitting at a nonzero remaining-capacity boundary, maximizing media use. Operators who prefer fewer parts may select a larger absolute or percentage threshold.

For each input file, the planner applies these rules against the current disc's usable payload capacity after all reserved overhead:

1. If the complete file fits, allocate it to the current disc.
2. If it does not fit and the portion that would fill the current disc is at least the resolved minimum split size, split the file at that boundary. Allocate the first part to the current disc and continue the remaining bytes on the next disc.
3. If that current-disc portion is smaller than the resolved minimum split size, leave the remaining capacity unused and begin the complete file on the next disc.

Split parts retain ordered part metadata so recovery can reassemble the original file. The planner must record the requested threshold, resolved byte threshold, and unused capacity caused by the policy in the archive plan and final report.

## Local Loss Tolerance

The CLI exposes local PAR2 configuration as a user-facing loss-tolerance policy:

```text
--loss-tolerance=<integer>%
```

The value is a whole-number percentage from `0%` through `50%`; no whitespace, fractional value, bare number, or value above `50%` is accepted. The default is `10%`. `0%` disables local recovery data.

The percentage is recovery data relative to the size of protected final encrypted artifacts, not a guarantee that the same percentage of physical disc sectors can be lost. Local recovery data must protect the final encrypted archive and other encrypted artifacts required for recovery, excluding PAR2 files themselves. It cannot recover a wholly missing disc because the recovery data is stored on that same disc.

For usable disc capacity `C` and tolerance `p`, expressed as a decimal ratio, the maximum protected payload is `C / (1 + p)`. The planner reserves this capacity before allocation, records the requested tolerance and actual recovery-data size in per-disc metadata, and verifies the generated PAR2 set before reporting success.

## Archive Storage Mode

The first release creates archives in store mode without compression. It does not expose a compression option and does not use extension, MIME, sampling, or content heuristics to estimate compression ratios.

The archive adapter must calculate a conservative maximum artifact size from raw source bytes plus bounded archive structural overhead. The planner combines that bound with bounded GnuPG encryption overhead, selected loss-tolerance recovery data, manifest/recovery metadata, and image filesystem overhead. This bound, rather than an expected compressed size, determines disc allocation and preflight capacity.

The initial portable archive adapter writes PAX tar in store mode. Compression may be introduced only in a later version with an explicit capacity-planning strategy that remains safe for all input data.

## Output Layout and Publication

`--output` is the parent directory for archive sets. The final archive-set directory is named `<set-id>`, where:

- Manifest input derives the name portion from the manifest filename with its final extension removed.
- Source-directory input requires `--archive-name` as the name portion. This value must be a safe single path component and must not contain separators or traversal segments.
- The remaining portion is the execution timestamp formatted as `YYYY-MM-DD_HH-MM`.

The CLI creates `<output>/.archiver-staging-<set-id>` directly beneath the output parent and writes final images and reports there. This directory also holds temporary encrypted payload and PAR2 artifacts for only the current disc; those artifacts are removed after that disc image is verified. It is the sole workspace in the first release; no `--work-dir` or `--workspace` option is provided.

Preflight must calculate output capacity for the complete final image set plus approximately one disc of temporary processing space. The archive adapter streams its store-mode output directly to the encryption adapter, so it must not create a separate plaintext archive file in staging. The staging directory and final `<output>/<set-id>` directory must both be absent during preflight; if either exists, the CLI stops with a clear error before creating archive artifacts. It does not generate automatic suffixes and provides no overwrite, reuse, or resume behavior in the first release.

After all images and reports pass verification, the CLI atomically renames the staging directory to the final archive-set directory. This rename must occur within the output filesystem and must not copy generated images. Incomplete staging directories are retained only when explicitly requested for diagnosis; otherwise they are cleaned according to the configured safety policy.

## Open Design Questions

Resolve these decisions before implementing the corresponding Phase 1.2 workstream.

Recovery-tool bundle distribution remains deferred to Phase 1.3. Phase 1.2 defines only its portable adapter contract and test double.

## Work

### [x] 1. Define Versioned Portable Formats

1. Define a manifest schema that records absolute source paths, logical archive paths, file identity metadata, original volume identity where applicable, and optional precomputed hashes.
2. Define an archive-set configuration schema containing set ID, capacity, image profile, encryption configuration reference, loss tolerance, output policy, and format version.
3. Define an archive plan schema containing ordered disc assignments, expected artifact sizes, reserved overhead, requested and resolved minimum split sizes, and the source-file parts created by splitting.
4. Define per-disc and archive-set manifests. They identify the archive set, disc number, payload files, hashes, tool formats and versions, recovery instructions, and recovery-tool bundle identity.
5. Version all persisted schemas from their first use. Reject unsupported future versions rather than guessing their semantics.

### [x] 2. Implement Input Validation and Snapshotting

1. Resolve and validate every source path before allocation.
2. Reject duplicate, missing, unreadable, non-regular, or changed source files according to the selected manifest policy.
3. Record size and modification metadata at plan time; revalidate immediately before data is read during creation.
4. Apply the documented local-filesystem and symlink policy, including archive-relative link preservation, explicit external-target materialization, physical-identity cycle detection, and network-filesystem rejection. Define the portable classification contract, implement test doubles, and provide the minimum Linux classifier required for real source-directory validation.
5. Validate the output parent, calculated set ID, staging path, final path, and free capacity for final images plus one-disc temporary processing. Refuse any existing staging or final path.

### [x] 3. Implement Capacity-Aware Planning

1. Parse the documented strict capacity grammar to exact bytes and reject all other values.
2. Obtain image-profile constraints from the selected image adapter, including effective capacity, filesystem overhead, supported file-size limits, required recovery-tool bundle size, and the selected loss tolerance's recovery-data reservation.
3. Reserve conservative store-mode archive, encryption, recovery, manifest, and image overhead before allocating payload data. Never plan against nominal media capacity alone.
4. Parse and resolve the documented minimum split-size policy, then split input files according to the sequential boundary policy. Preserve enough metadata to reassemble each original file after extraction.
5. Allocate all sources deterministically into separate disc units using canonical relative logical archive-path lexical ordering.
6. Report unallocatable input and the exact capacity shortfall before archive creation.

### [x] 4. Define Adapter Contracts and Orchestration

1. Define narrow interfaces for archive creation, encryption, PAR2-based loss tolerance, image generation, recovery-tool bundling, and verification.
2. Keep writer discovery, burning, and read-back verification as optional capabilities; the portable create path requires only image creation and image verification.
3. Require adapters to return structured capabilities, command version, output artifact paths, actual sizes, hashes where available, and actionable failure details.
4. Use application orchestration to sequence: create the output staging directory, stage each disc layout, stream the archive payload directly into encryption, generate local PAR2 over the final encrypted payload, write manifests/instructions/tool bundle, create and verify each image, remove its temporary artifacts, then atomically publish the archive set.
5. Do not calculate parity over plaintext. Do not retain temporary plaintext archives after the encrypted artifact has been verified.
6. Keep an incomplete staging directory for diagnosis only when explicitly requested; otherwise clean it on success and failure according to the configured safety policy.

### [x] 5. Implement Reporting, Cancellation, and Recovery Instructions

1. Emit lifecycle events for validation, planning, staging, archive creation, parity, image creation, verification, cleanup, and completion.
2. Make event payloads stable, documented, and free of passphrases or plaintext file names unless the caller explicitly requests a privacy-reducing detail level.
3. Propagate cancellation through contexts to every adapter process and leave final output absent or clearly incomplete.
4. Generate per-disc recovery instructions from the actual selected formats and tool bundle, rather than hard-coding Linux shell commands.
5. Produce a final archive-set report with planned and actual capacities, generated image paths, hashes, tool versions, and verification outcomes.

## Test Strategy

Implement the portable core test-first. Use fake archive, PAR2, image, verification, and recovery-tool adapters for unit and application tests. Reserve real-tool tests for Phase 1.3.

1. Unit test capacity and minimum split-size parsing, overhead reservation, deterministic allocation, and oversized-file splitting with table-driven cases, including zero, absolute, percentage, and invalid thresholds.
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

- Concrete GnuPG, `par2cmdline`, and image-tool process adapters.
- Optical writer discovery, burning, and read-back verification.
- Validation with real optical media.
- Windows and macOS adapter implementations.
- Cross-disc parity that reconstructs a wholly lost disc.
