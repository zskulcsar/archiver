# BD-XL Multi-Disc Archive Workflow & Tooling Specification

## 📌 Overview

This document outlines a **bulletproof workflow** for creating **encrypted, multi-disc BD-XL archives** with **partial recoverability** and **future-proofing**. The goal is to ensure that:
- Each disc is **self-contained** and can be recovered independently.
- Large files (e.g., videos >100GB) are **pre-split** to fit on a single disc.
- **Encryption** is applied at the **archive level** (or per-file) to protect data.
- **Recovery tools** (`gpg`, `dar`, `tar`) are included on every disc for future compatibility.

---

## 🎯 Workflow Summary

### **Core Principles**
1. **Pre-split large files** into chunks ≤ 80–90GB (to fit on a 100GB BD-XL disc).
2. **Archive each chunk** (e.g., with `tar` or `dar`).
3. **Encrypt each archive** with `gpg` (AES-256).
4. **Create an ISO** from the encrypted archive (optional but recommended for compatibility).
5. **Write to disc** + include recovery tools (`gpg`/`dar` sources/binaries + `README.txt`).

---

## 📋 Detailed Workflow

### **1. Pre-Split Large Files**
**Goal:** Ensure no file exceeds the disc capacity (100GB).

#### **For General Files**
```bash
# Split a large file into 80GB chunks
split -b 80G /path/to/large_file /path/to/chunks/file_part_
```
- **Result:** `file_part_aa`, `file_part_ab`, etc. (each ≤ 80GB).

#### **For Video Files (Optional: Keyframe Splitting)**
```bash
# Split video at keyframes (e.g., 30-minute chunks)
ffmpeg -i input.mp4 -f segment -segment_time 00:30:00 -reset_timestamps 1 -c copy chunk_%03d.mp4
```
- **Result:** `chunk_001.mp4`, `chunk_002.mp4`, etc.
- **Why:** Minimizes corruption risk by splitting at natural boundaries.

---

### **2. Archive Each Chunk**
**Goal:** Group pre-sized files/chunks into archives.

#### **Using `tar` (Simple)**
```bash
# Archive a single chunk
.tar cf archive_1.tar /path/to/chunks/file_part_aa
```

#### **Using `dar` (Advanced Metadata Support)**
```bash
# Archive a single chunk with metadata preservation
dar -c -R /path/to/chunks -A /path/to/archives/archive_1
```

---

### **3. Encrypt Each Archive**
**Goal:** Protect each archive with AES-256 encryption.

```bash
# Encrypt a single archive
gpg --symmetric --cipher-algo AES256 -o archive_1.tar.gpg archive_1.tar
```
- **Result:** `archive_1.tar.gpg` (encrypted).
- **Note:** Use a **strong password** and store it securely.

---

### **4. Create ISO (Optional but Recommended)**
**Goal:** Ensure compatibility with optical drives.

```bash
# Create ISO from encrypted archive
genisoimage -r -J -o disc1.iso archive_1.tar.gpg
```
- **Result:** `disc1.iso` (ready to burn to BD-XL).

---

### **5. Write to Disc + Add Recovery Tools**
**Goal:** Make each disc self-contained for recovery.

#### **Directory Structure for Each Disc**
```
disc_1/
├── disc1.iso              # ISO with encrypted archive
├── gpg_source/            # GPG source code (from gnupg.org)
├── gpg_static/            # Pre-compiled static GPG binary
├── dar_source/            # DAR source code (from SourceForge)
├── dar_static/            # Pre-compiled static DAR binary
└── README.txt             # Recovery instructions
```

#### **Example `README.txt`**
```text
BD-XL ARCHIVE RECOVERY INSTRUCTIONS
====================================

1. Mount this disc:
   mount /dev/sr0 /mnt/disc

2. Extract the ISO (if used):
   mkdir /tmp/recovery
   mount -o loop /mnt/disc/disc1.iso /tmp/recovery

3. Decrypt the archive:
   gpg --decrypt /tmp/recovery/archive_1.tar.gpg | tar xf - -C /recovery/path

4. If gpg/dar are unavailable:
   - Compile from source in /gpg_source or /dar_source
   - Or use the static binaries in /gpg_static or /dar_static

5. Verify data integrity:
   sha256sum /recovery/path/file_part_aa
```

---

## 🔧 Tooling Requirements

### **Required Tools**
| Tool       | Purpose                          | Source                          | Notes                                  |
|------------|----------------------------------|---------------------------------|----------------------------------------|
| `gpg`      | Encryption/decryption             | [gnupg.org](https://gnupg.org) | Static binary recommended for portability. |
| `tar`      | Archiving                        | Built into Linux                | Standard tool, widely available.       |
| `dar`      | Advanced archiving (metadata)     | [SourceForge](https://sourceforge.net/projects/dar/) | Optional but recommended. |
| `genisoimage` | ISO creation                  | `sudo apt install genisoimage` | For ISO compatibility.                |
| `split`    | File splitting                  | Built into Linux                | For pre-splitting large files.        |
| `ffmpeg`   | Video keyframe splitting         | `sudo apt install ffmpeg`      | Optional for video files.             |

---

## Cross-Platform Implementation Constraints

The archive workflow must support Linux, Windows, and macOS. The core workflow and optical-media operations have different portability characteristics.

### Portable Core Capabilities

Encrypted archive creation and integrity protection are available on all three operating systems through proven external tools. The implementation may use platform-specific tool adapters, but it must provide equivalent core behavior:

| Capability | Suitable Tooling | Cross-Platform Status |
|------------|------------------|-----------------------|
| Archive creation and AES encryption | 7-Zip (`7zz`), DAR, GnuPG | Available on Linux, Windows, and macOS. |
| Integrity verification and repair | `par2cmdline` / PAR2 | Available on Linux, Windows, and macOS. |
| Disc image generation | `xorriso`, `cdrtools`, `Oscdimg` | Available, but requires an OS-specific adapter. |

The first implementation should rely on these established tools where practical rather than reimplementing archival, encryption, or parity formats.

### Optical-Media Backends

Optical-drive discovery, BD-XL burning, and post-burn read-back verification are platform- and hardware-specific. They must be implemented as optional OS-specific backends rather than assumed capabilities of the cross-platform core.

- Linux: `xorriso` is the primary candidate for scripted ISO creation, BD writing, and checksum-based verification.
- Windows: image generation can use `Oscdimg`, while a separately validated burner backend is required for BD-XL writing and verification.
- macOS: image generation is available through tools such as `cdrtools` or `xorriso`, but direct BD-XL writing and verification require validation against the selected external drive, firmware, and media.

The CLI must always support generating disc images to an output folder without an optical drive. Direct burning is an additive capability: the CLI should report whether the selected backend can discover a writer, burn the selected media profile, and verify the written disc. A tested hardware compatibility matrix is required before advertising direct BD-XL support for a platform.

---

## 📦 Example: Full Workflow for a 200GB Video File

### **1. Split the Video**
```bash
split -b 80G large_video.mp4 video_chunk_
# Result: video_chunk_aa (80GB), video_chunk_ab (80GB), video_chunk_ac (40GB)
```

### **2. Archive Each Chunk**
```bash
tar cf archive_1.tar video_chunk_aa
tar cf archive_2.tar video_chunk_ab
tar cf archive_3.tar video_chunk_ac
```

### **3. Encrypt Each Archive**
```bash
gpg --symmetric --cipher-algo AES256 -o archive_1.tar.gpg archive_1.tar
gpg --symmetric --cipher-algo AES256 -o archive_2.tar.gpg archive_2.tar
gpg --symmetric --cipher-algo AES256 -o archive_3.tar.gpg archive_3.tar
```

### **4. Create ISOs (Optional)**
```bash
genisoimage -r -J -o disc1.iso archive_1.tar.gpg
genisoimage -r -J -o disc2.iso archive_2.tar.gpg
genisoimage -r -J -o disc3.iso archive_3.tar.gpg
```

### **5. Write to Discs**
For each disc (`disc1`, `disc2`, `disc3`):
- Write the ISO (`discX.iso`).
- Include `gpg_source/`, `gpg_static/`, `dar_source/`, `dar_static/`, and `README.txt`.

---

## 🔄 Recovery Process

### **Single Disc Recovery**
```bash
# Mount the disc
mount /dev/sr0 /mnt/disc

# Extract ISO (if used)
mkdir /tmp/recovery
mount -o loop /mnt/disc/disc1.iso /tmp/recovery

# Decrypt and extract
gpg --decrypt /tmp/recovery/archive_1.tar.gpg | tar xf - -C /final/recovery/path
```

### **Partial Recovery (One Disc Lost)**
- **Impact:** Only the chunks on the lost disc are unavailable.
- **Example:** If `disc2.iso` is lost, you lose `video_chunk_ab` but can still recover `video_chunk_aa` and `video_chunk_ac`.

---

## ⚠️ Edge Cases & Mitigations

| Edge Case                          | Mitigation                                                                                     |
|------------------------------------|-----------------------------------------------------------------------------------------------|
| **File > 100GB**                   | Pre-split into ≤ 80GB chunks before archiving.                                               |
| **Disc corruption**                | Verify burns with `sha256sum` after writing.                                                 |
| **GPG/DAR unavailable in future**   | Include sources/binaries on every disc.                                                     |
| **Password loss**                  | Store password securely (e.g., password manager, offline backup).                            |
| **ISO compatibility issues**       | Use `genisoimage` with `-r -J` for broad compatibility.                                       |

---

## 💡 Scripting Outline

### **1. Pre-Split Script (`pre_split.sh`)**
```bash
#!/bin/bash
# Usage: ./pre_split.sh /path/to/large_file 80G

INPUT_FILE=$1
CHUNK_SIZE=$2
OUTPUT_DIR="./chunks_$(basename $INPUT_FILE)"

mkdir -p "$OUTPUT_DIR"
split -b "$CHUNK_SIZE" "$INPUT_FILE" "$OUTPUT_DIR/$(basename $INPUT_FILE)_part_"

echo "Split $(basename $INPUT_FILE) into chunks in $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"
```

### **2. Archive & Encrypt Script (`archive_encrypt.sh`)**
```bash
#!/bin/bash
# Usage: ./archive_encrypt.sh /path/to/chunks_dir /path/to/output

CHUNKS_DIR=$1
OUTPUT_DIR=$2

mkdir -p "$OUTPUT_DIR"

for chunk in "$CHUNKS_DIR"/*; do
    archive_name="$(basename $chunk).tar"
    encrypted_name="$(basename $chunk).tar.gpg"
    
    tar cf "$OUTPUT_DIR/$archive_name" "$chunk"
    gpg --symmetric --cipher-algo AES256 -o "$OUTPUT_DIR/$encrypted_name" "$OUTPUT_DIR/$archive_name"
    rm "$OUTPUT_DIR/$archive_name"  # Clean up unencrypted archive
    
    echo "Processed: $chunk -> $encrypted_name"
done
```

### **3. ISO Creation Script (`create_iso.sh`)**
```bash
#!/bin/bash
# Usage: ./create_iso.sh /path/to/encrypted_archives /path/to/iso_output

ENCRYPTED_DIR=$1
ISO_OUTPUT_DIR=$2

mkdir -p "$ISO_OUTPUT_DIR"

for encrypted_archive in "$ENCRYPTED_DIR"/*.tar.gpg; do
    iso_name="$(basename $encrypted_archive .tar.gpg).iso"
    genisoimage -r -J -o "$ISO_OUTPUT_DIR/$iso_name" "$encrypted_archive"
    echo "Created ISO: $iso_name"
done
```

### **4. Full Workflow Script (`full_workflow.sh`)**
```bash
#!/bin/bash
# Usage: ./full_workflow.sh /path/to/large_file /output_dir 80G

INPUT_FILE=$1
OUTPUT_DIR=$2
CHUNK_SIZE=$3

# Step 1: Pre-split
./pre_split.sh "$INPUT_FILE" "$CHUNK_SIZE"
CHUNKS_DIR="./chunks_$(basename $INPUT_FILE)"

# Step 2: Archive and encrypt
./archive_encrypt.sh "$CHUNKS_DIR" "$OUTPUT_DIR/encrypted"

# Step 3: Create ISOs
./create_iso.sh "$OUTPUT_DIR/encrypted" "$OUTPUT_DIR/isos"

# Step 4: Prepare discs (manual step)
echo "Now copy ISOs from $OUTPUT_DIR/isos to discs and add recovery tools."
```

---

## 📌 Final Notes

### **Why This Works**
- **Self-contained discs:** Each disc has everything needed to recover its data.
- **Partial recovery:** Losing one disc only affects its own data.
- **Future-proof:** Tools and instructions are included on every disc.

### **When to Use This Workflow**
- **Long-term archival** (e.g., videos, backups).
- **Multi-disc BD-XL projects** (100GB+ files).
- **Scenarios where partial recovery is acceptable** (e.g., losing 1–2 minutes of video is okay).

### **When to Avoid This Workflow**
- **Single-disc projects** (use standard `dar` or `tar` + `gpg`).
- **Scenarios requiring 100% data integrity** (use RAID or parity tools like `par2`).

---

## 🔗 Resources
- [GNU GPG (gpg)](https://gnupg.org/)
- [DAR (Disk Archive)](https://sourceforge.net/projects/dar/)
- [genisoimage](https://wiki.debian.org/genisoimage)
- [FFmpeg](https://ffmpeg.org/)

---

*Last updated: $(date)*
*Workflow designed for BD-XL (100GB) multi-disc archives with partial recoverability.*
