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
- **Clip metadata**: codec, frame size, frame rate, length, start timecode,
  audio format, camera and reel are read from every clip as it is offloaded
  or scanned (QuickTime/MP4, BWF, stills; anything else through `ffprobe`
  when installed) and kept with the library. Filter, preview, log scene /
  take / notes / QC, and export a CSV for post.
- **A status page for the team**: one self-contained web page with every
  drive, where it was last seen, each project's backup health and a file
  search, written to a shared folder or uploaded to a server whenever a job
  finishes, on a timer, and when the app quits.

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

### Map

The first tab is a map: this Mac on the left, the projects in the
middle, and every drive on the right, with a line from each project to
each drive that holds part of it. The line's label says how much of the
project is on that drive and whether that part is backed up (green
verified, yellow partly, red not at all, dotted unproven); dotted green
lines from the Mac show what is plugged in and on which bus. Click a
node and what it touches lights up, with a card that opens the project,
the drive's details, or the Connected tab; double-click opens it
straight away. Drag to pan, ⌘ + scroll to zoom, drag a node to move it
(Lumpy remembers), **Fit** to see everything, **Arrange** to put the
nodes back.

### Drive library

Every drive is a card with a picture of what it is (a hard drive, an
SSD, a camera card or a RAID enclosure, from what macOS says about it;
change it under **Details → Labels → Picture on the card**), its light on
when it is plugged in, and a ring showing how full it is: pale until
70%, yellow after, red in the last 10%. Cards are tinted with the
project's colour.

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

### Clips and metadata

While a card is offloaded, Lumpy reads each clip's container headers
(never the picture data, so it costs nothing noticeable) and records what
post will ask about: codec, frame size, frame rate, duration, start
timecode (drop-frame aware), audio codec, channels, sample rate and bit
depth, camera make and model, reel, and the file's own creation date.
QuickTime/MP4 and WAV/BWF (including iXML scene and take) are read
natively; MXF, R3D and the rest go through `ffprobe` when it is on the
Mac. Stills get their size and camera from JPEG/EXIF, PNG and TIFF/DNG
headers. Drives logged before this feature get theirs with **Details →
Read metadata** or `lumpy drive metadata DRIVE`.

The **Clips** tab lists every distinct clip across the library (the same
checksum on three drives is one row), with filters by project, drive,
kind, codec, frame rate and QC state, and a search over names, cameras,
reels, scenes, notes and tags. Clicking a clip opens it: a playback
preview when its drive is plugged in, the technical details, every drive
holding a copy, and a log section for scene, shot, take, tags, notes and
a QC mark (OK, flag or reject). Log entries follow the content: set them
on one copy and every copy has them. **Export CSV…** writes the current
list with every column; a project page has the same button for just its
clips.

### Proxies

Post keeps camera originals in an **Online** folder and the proxies cut
from them in an **Offline** folder beside it (any spelling: `Online`,
`00_ONLINE`, `Online footage`). Lumpy pairs the two wherever they sit on
a drive and shows, on the project page and in the drive's details, how
many originals have a proxy, which still need one, and how big those
are. **Copy list** puts the names of the unproxied clips on the
clipboard for whoever makes them. An original counts as proxied when a
video with the same name, with or without a `_proxy`, `_prx`, `S03` or
`_1` suffix, is anywhere in the Offline folder; RED clips count once,
not per chunk.

### Archive: what the edit didn't use

When the cut is locked, open the project and, under **Archive**, choose
the timeline export: FCPXML (Final Cut Pro, DaVinci Resolve), Premiere
Pro or FCP 7 XML, a CMX 3600 EDL, or a plain list of clip names (one
per line, or a CSV with a file-name column). Several at once is fine:
the cut, the trailer, the socials. Lumpy matches the clips the edit
refers to against every video, audio and still in the project's
folders and shows what is used, what isn't (by folder, with sizes), and
what the edit refers to that isn't in the project at all (stock, music,
graphics). Matching is by clip name, so a name the edit uses anywhere
keeps every clip with that name, and a RED clip's chunks count as one.

Then either **Move … unused into the folder** (an `_Unused` folder,
or any name you type, inside each project folder, keeping the clips'
paths so they can be put back) or **Move … unused to the Trash**,
which asks first. Every drive involved must be plugged in. Both run as
jobs in Activity, and the library follows the files: moved clips keep
their checksums and metadata at the new path, trashed ones are
forgotten. The Trash is the Mac's own, so nothing is gone until you
empty it. The first time, macOS may ask whether Lumpy may control the
Finder, which is how files reach the Trash.

### Menu bar

Lumpy.app also lives in the menu bar: the NAS glyph shows the running
job's percentage beside it, and clicking it drops a panel with the job's
progress (speed, time left, Cancel), the last finished job (**Run
again**), how many drives are plugged in and in the library, how many
clips, each plugged-in drive's free space and backup state, and **Open
Lumpy** / **Quit**. Closing the window leaves Lumpy running in the menu
bar — logging drives, finishing jobs, publishing the status page; the
Dock icon, the menu bar panel or ⌘0 bring the window back.

### Settings

Under the gear: **General** (Appearance: **System**, **Light** or
**Dark**; Lumpy is dark by default, the terminal, previews and the menu
bar panel stay dark either way); **Drives** (hide the Mac's internal volumes and mounted
disk images, log new drives automatically or not, read clip metadata
during scans, rescan plugged-in drives on a timer, folder exclusions
such as `.git, node_modules, Render*` left out of scans, and a list of
drives set to *Never log* with a **Log again** button); **Library**
(where it lives and how big it is, **Move library here** to keep it on
an external drive or a synced folder, **Optimise** to compact it, and
**Clear history** / **Forget every drive** — which never touch the
drives themselves); **Updates**; and the **Status page**.

### Status page for coworkers

Under the gear icon, turn on **Publish the status page**. Lumpy writes
`index.html` (self-contained, works from a folder or any web server) and
`status.json` into the folder you choose, and can also HTTP PUT both to a
URL (with a bearer token) or run a command such as `rsync` afterwards.
It publishes whenever a job finishes, on the interval you pick while the
app is open, and once more when the app quits, so the page always shows
the last known state; `lumpy publish` does the same from Terminal. Put
the folder in Dropbox or iCloud Drive and the team can check which drive
holds what, where it was last seen and whether a project is backed up,
without asking.

### Reports and exit status

Every command prints what it finds. Add `--report day04.txt` to save a copy.
Problems are marked `FAIL`, and the run ends with `OK` or `BAD` per
destination. The exit status is 0 when everything matched, 1 when any file
failed or isn't backed up enough, 2 for a usage or setup error, and 130 when interrupted, so Lumpy can
be scripted.

## Updating

Lumpy checks GitHub Releases twice a day while it is open and shows an
**Update to x.y.z** button in the top bar when there is one. Click it →
**Install**: the new `Lumpy.app` is downloaded, checked against the
release's published SHA-256, swapped into place, and **Relaunch Lumpy**
finishes the job. The library is untouched, the `lumpy` command and the
background agent follow the app automatically. `lumpy update` does the
same from Terminal (`lumpy update check` only looks).

The repository is public, so no sign-in is needed. (If it is ever made
private, the check needs a GitHub token that can read it: Settings →
**GitHub token**, or `lumpy update token TOKEN`; a fine-grained token with
*Contents: read* on this repository is enough.)

To publish a version: GitHub → **Actions** → **Release** → *Run
workflow* → enter the version (e.g. `0.6.0`). The workflow tags the
commit, builds the app, attaches `Lumpy-macOS.zip`,
`lumpy-macos-universal.zip` and `SHA256SUMS`, and writes the notes from
the merged pull requests.

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
