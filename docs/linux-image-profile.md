# Linux Image Profile

The initial Linux image-only backend uses `xorriso` to create ISO9660 Level 3 images with Rock Ridge extensions. The profile supports long Linux file names but limits each image to 4,000,000,000 bytes. Larger media profiles require a future UDF-capable image backend.

## Tool Requirements

Archiver discovers installed executables and never installs or upgrades them. The tested command interfaces require:

- `gpg` 2.2 or newer, with loopback pinentry and `--passphrase-fd` support.
- `par2` from par2cmdline 0.8 or newer, with create and verify support.
- `xorriso` 1.5 or newer, with ISO9660 Level 3 image creation and filesystem inspection support.

Distribution package names commonly include `gnupg`, `par2cmdline`, and `xorriso`. PAX tar creation uses Go's standard library and requires no external archive executable.

## Capacity Model

The `linux-iso9660-level3-v1` profile reserves filesystem/image, PAX tar structural, manifest/recovery-material, GnuPG-envelope, and configured PAR2 capacity before allocation. The resulting usable payload capacity is recorded in the plan.

## Current File-Splitting Limit

The portable PAX tar creator streams every planned byte range directly from its source file. Split parts are stored under deterministic internal member names and a full encrypted reassembly manifest is duplicated in every disc payload.

## Direct Optical Operations

Image-only output works without an optical drive. Direct writer discovery, burning, and read-back are intentionally disabled until an approved entry exists in the hardware compatibility matrix.
