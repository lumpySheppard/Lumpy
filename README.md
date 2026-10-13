# Lumpy

A macOS command-line tool for backing up camera cards and shoot drives and
proving every copy is intact.

- **Offload** a card to two or more drives at once. The source is read a
  single time and written to every destination in parallel.
- **Verify** each copy by reading it back from the drive itself (macOS's file
  cache is bypassed, so the check really touches the disk) and comparing its
  **xxHash64** checksum with the source's. The card is read a second time
  alongside, so a faulty card or reader can't slip the same bad data into
  every copy.
- **Record** the result as an [ASC MHL](https://theasc.com/society/ascmitc/asc-media-hash-list)
  manifest (the industry-standard media hash list) in an `ascmhl` folder
  inside each copy. Lumpy's manifests are tested against the ASC's own XML
  schema and its reference `ascmhl` tool.
- **Re-check** a drive later against its manifest, or **compare** two copies
  that have no manifest.
- **A Mac app** with drive cards, details, search, offloads and live
  progress, plus the same features on the command line.
- **Projects**: the folders that hold one job, on one drive or many, and
  how well they are backed up anywhere in the library: bytes with two or
  more copies, sole copies, and files with no checksum yet. Lumpy plans the
  copies that get every file onto two drives, using free space on any drive
  (or the drives you choose), and runs each step from the app.
- **Keep a drive library**: every drive you plug in is logged with its files
  and checksums, so you can see where a clip lives and whether a drive is
  properly backed up while it sits on a shelf, and queue a full check that
  runs by itself the next time the drive is plugged in.

## Install

### The app

Download **Lumpy-macOS** from the latest [CI run](../../actions/workflows/ci.yml),
unzip it, and move `Lumpy.app` to Applications. It isn't notarized yet, so
the first time, clear the download flag before opening it:

```sh
xattr -dr com.apple.quarantine /Applications/Lumpy.app
```

The app has four tabs:

- **Connected**: drives plugged in now. New drives are logged as soon as they
  mount while the app is open.
- **Logged**: every drive Lumpy has seen, as cards or a list, with capacity,
  labels (display name, client, project, contact, notes, color), backup
  status and last check. Search finds drives by any label and files by name
  across every drive, plugged in or not. **Export CSV** saves the library.
  **Details** shows a drive's storage breakdown by top-level folder, a file
  browser with each file's checksum status, its backup coverage, and its
  history. From there you can check the drive now, or queue a check for the
  next time it's plugged in.
- **Projects**: group the folders (or drives) that hold one job. A project
  page shows its health, which folder holds the sole copies, a plan to fix
  it, and a Run button on each step whose drives are plugged in.
- **Offload**: pick a card and destinations (or click the drive chips), then
  start. Progress, speed and time left are shown live in a terminal view.
- **Activity**: every offload, check and rescan with its full log.

The command-line tool is inside the app. To use it in Terminal:

```sh
sudo ln -sf /Applications/Lumpy.app/Contents/Resources/lumpy /usr/local/bin/lumpy
```

`lumpy ui` opens the same screens in a browser instead.

### Command line only

Download the `lumpy-macos-universal` build from the same CI run, then:

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
Add `--verbose` for a line per file copied and verified (the app always
shows these in its terminal view).

How a copy is made safe, step by step:

1. The card is read once; the data goes to every destination at the same
   time and through an xxHash64 checksum.
2. Each copy is written under a hidden temporary name, flushed to the drive
   (`F_FULLFSYNC` on macOS, so the drive's own cache is flushed too), given
   the source's modification date and extended attributes (Finder tags,
   comments), and only then renamed to its real name.
3. Every copy is read back from its drive with caching turned off and its
   checksum compared with the card's. At the same time the card is read
   again (`--no-source-check` skips this), and any file that reads
   differently is treated as untrustworthy everywhere.
4. Only files that passed are written into each copy's MHL manifest and the
   drive library.

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

### Drive library

Lumpy keeps a library of every drive it has seen in
`~/Library/Application Support/Lumpy/catalog.db`. Drives are recognised by
their volume UUID, so a renamed drive is still the same drive, and two cards
both called `Untitled` are kept apart.

Install the background agent once and every drive you plug in is logged
automatically:

```sh
lumpy agent install
```

It runs whenever a drive is mounted, lists its files (fast: names, sizes and
dates, no reading of footage), picks up checksums from any MHL manifests on
it, and notices files that have disappeared since last time. If macOS asks
whether `lumpy` may access removable volumes, allow it; adding the binary to
System Settings → Privacy & Security → Full Disk Access stops the question.
Its log is `~/Library/Logs/Lumpy/agent.log`.

Offloads are recorded too: the card and every destination get the checksums
Lumpy just verified, so their files count as proven copies straight away.

```sh
lumpy drives                    # every drive, when it was last seen and checked
lumpy drive SHUTTLE_1           # one drive's details and history
lumpy find A003C012             # which drives hold this clip
lumpy backups A001              # does every file on card A001 have a copy elsewhere?
lumpy backups --min 2 SHUTTLE_1 # ...at least two copies?
```

`backups` works entirely from the library, so the drive doesn't need to be
plugged in. Files count as copies when another drive holds a file with the
same xxh64 checksum and size. A copy that failed its last check doesn't
count. Files with no checksum yet are matched by name and size, and
reported as probable but not proven.

#### Projects: is this job backed up?

A project is the folders that hold one piece of work, wherever they are:
a feature spread over a RAID, three shuttles and an edit SSD, or one music
video that shares a portable drive with four other jobs. A whole drive
counts as one folder. Copies anywhere in the library count, so a job on
a shuttle whose camera originals also sit on the office RAID is backed up
even though nobody put the RAID "in" the project.

Every distinct file (matched by checksum, so a clip counts once however
many folders it sits in) should be on at least the project's target number
of drives, two by default. The project page shows:

- **Health**: one bar for the project. Green is backed up (on two or more
  drives), red is a sole copy, grey has no checksum yet so can't be
  proven either way.
- **What's in this project**: each folder, split the same way, with its
  drive and whether it's plugged in. **Add a folder or drive** browses a
  drive's index to pick a folder. On a drive's Details page, the **+** on
  any folder in the file browser does the same.
- **Files with too few copies**, by folder.
- **Back up onto**: where the plan may put copies. By default any drive in
  the library with room; or only the drives you tick.
- **Plan to reach 2 copies**: the copies that would fix it, grouped into
  steps of "from this drive to that drive". Lumpy prefers destinations
  that are plugged in now, keeps the number of drive pairs small, keeps
  2 GB free on every drive, and never copies a file onto a drive that
  already holds it. If no drive has room for something, it says how much
  more space is needed.
- **Run this step** appears when both drives are plugged in. It copies
  only the files that need another copy into a folder named after the
  project on the destination (editable), keeping their paths (with a
  folder per source inside it when the project has several). Each copy
  is read back and checked, the source is read a second time, an MHL
  manifest is written into the destination folder, and the library is
  updated, so the health bar moves on the next refresh.

Make a project from the Projects tab, from the **+** on a folder in a
drive's file browser, or select whole drives on the Logged tab
(**Select**) and choose **Add to project**. Files without checksums show
as unproven until the drive is plugged in and checksummed.

### Checking a drive

```sh
lumpy drive verify SHUTTLE_1
```

If the drive is plugged in, Lumpy re-reads every file and compares it with
the library, reporting files that changed, were damaged, or disappeared. If
it's on a shelf, the check is **queued**: the agent runs it the next time
the drive is mounted and posts a notification when it's done (or tells you
if the drive was unplugged part-way, in which case it stays queued).
`lumpy drive cancel SHUTTLE_1` cancels a queued check.

`lumpy drive hash SHUTTLE_1` checksums only the files that don't have a
checksum yet, which is useful for drives filled by other tools without MHLs.
`lumpy catalog` logs the mounted drives by hand if you don't use the agent.
When a card is reformatted, `lumpy drive forget A001` removes it so it stops
counting as a copy. (If it's plugged in again, the agent notices its files
are gone anyway.)

### Reports and exit status

Every command prints what it finds. Add `--report day04.txt` to save a copy.
Problems are marked `FAIL`, and the run ends with `OK` or `BAD` per
destination. The exit status is 0 when everything matched, 1 when any file
failed or isn't backed up enough, 2 for a usage or setup error, and 130 when interrupted, so Lumpy can
be scripted.

## Testing with real drives

See [docs/TESTING.md](docs/TESTING.md) for a step-by-step test with a card
and two drives, including deliberately damaging a copy to see it caught.

## Build

Requires Go 1.24 or later.

```sh
go test ./...
go build -o lumpy ./cmd/lumpy
```

To build `Lumpy.app` on a Mac (needs the Xcode command line tools as well):

```sh
macos/build-app.sh      # writes dist/Lumpy.app
```

The app is a small Swift window (`macos/main.swift`) around the same
engine: it starts `lumpy ui --app`, which serves the interface from
`internal/ui/web` on 127.0.0.1 with a per-launch access token, and shows it
in a WKWebView with native folder pickers and Save dialogs.
