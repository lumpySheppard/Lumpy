# Lumpy

A macOS command-line tool for backing up camera cards and shoot drives and
proving every copy is intact.

- **Offload** a card to two or more drives at once. The source is read a
  single time and written to every destination in parallel.
- **Verify** each copy by reading it back from the drive itself (macOS's file
  cache is bypassed, so the check really touches the disk) and comparing its
  **xxHash64** checksum with the source's.
- **Record** the result as an [ASC MHL](https://theasc.com/society/ascmitc/asc-media-hash-list)
  manifest (the industry-standard media hash list) in an `ascmhl` folder
  inside each copy. Lumpy's manifests are tested against the ASC's own XML
  schema and its reference `ascmhl` tool.
- **Re-check** a drive later against its manifest, or **compare** two copies
  that have no manifest.

## Install

Download the `lumpy-macos-universal` build from the latest
[CI run](../../actions/workflows/ci.yml) (or build it yourself, below), then:

```sh
chmod +x lumpy
xattr -d com.apple.quarantine lumpy   # only needed for a downloaded binary
sudo mv lumpy /usr/local/bin/
```

## Use

### Offload a card

```sh
lumpy offload /Volumes/A001 /Volumes/SHUTTLE_1/Day_04 /Volumes/RAID/Day_04
```

This creates `A001` in each destination (use `--name` to pick another name),
copies everything, verifies every copy, and writes
`A001/ascmhl/0001_A001_<date>Z.mhl` plus `ascmhl_chain.xml` in each one.

- If the card already carries an MHL (from the camera or an earlier
  offload), its history is copied too, and each source file is checked
  against it, so a card that was damaged after its first offload is caught.
- Running the same command again is safe: files already on a destination are
  not rewritten but are verified again, files that are missing are copied,
  and a new MHL generation records the check.
- Files are written under a temporary name and only renamed once flushed to
  the drive, so an interrupted offload (Ctrl-C, unplugged drive) never
  leaves a half-written clip under its real name.
- Lumpy refuses to start if a destination doesn't have room, or if a
  destination is inside the source.
- `.DS_Store`, `.Spotlight-V100`, `.Trashes`, `.fseventsd` and similar
  macOS housekeeping files are skipped. Empty folders are recreated, since
  some cameras need their card structure intact.

### Check a drive later

```sh
lumpy verify /Volumes/SHUTTLE_1/Day_04/A001
```

Reads every file listed in the `ascmhl` history and reports anything that
changed, is missing, or was added. It also detects a manifest that was
edited after it was written. It reads legacy MHL v1 files too: pass the
folder that holds the `.mhl` file, or the file itself. Manifests may use
xxh64, xxh3, xxh128, MD5, SHA-1 or SHA-256.

### Compare two copies

```sh
lumpy compare /Volumes/SHUTTLE_1/Day_04 /Volumes/RAID/Day_04
```

Checks both folders hold the same files with identical contents, reading
both drives at once. No manifest is needed.

### Reports and exit status

Every command prints what it finds; add `--report day04.txt` to save a copy.
Problems are marked `FAIL`, and the run ends with `OK` or `BAD` per
destination. The exit status is 0 when everything matched, 1 when any file
failed, 2 for a usage or setup error, and 130 when interrupted, so Lumpy can
be scripted.

## Build

Requires Go 1.24 or later.

```sh
go test ./...
go build -o lumpy ./cmd/lumpy
```
