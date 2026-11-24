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
  mount while the app is open, except cards, USB sticks and card readers
  (removable media, an SD or CFexpress slot, or a camera's folder layout),
  which get formatted and reused: those show up with a **Log now** button
  and are logged only when you press it. Network volumes are never listed.
- **Logged**: every drive Lumpy has seen, as cards or a list, grouped by
  project (click a group's header to fold it away), with capacity,
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

The first tab is a map of islands. A project whose drives hold nothing
else gets a **project island**: the project's header on top (how many
copies it wants, its size, file and drive counts, its backup bar and
what is still to copy) and those drives in a grid beneath, tinted in
the project's colour. A drive that holds several projects gets a
**drive island** instead: the drive on top, and a node per project
beneath it saying how much of that project is on the drive and how
it's backed up. A project that has drives of its own and also sits on
a shared drive gets both, with a wire from its header to its node in
the drive island. Being inside an island is what says what belongs
where, so there is no wire per drive; each node carries its own line
instead: green verified, yellow partly, red not at all, grey unproven,
a **backup** tag on backup drives, and a green dot for plugged in.
Drives in no project share a grey island. **This Mac** sits in the
middle with the islands packed either side of it, and a blue lead runs
from the Mac to each island that has a drive plugged in (and to each
plugged-in drive in no project), the way power reaches a hub: inside
the island every plugged-in drive carries a lit blue ring, and nothing
else is drawn, so the leads never cross one another or pass over a
node. The Mac's node says how many drives hang off which bus. The pill
in the top corner counts what is plugged in and, when Keep drives
awake is on, how many are kept awake. Click a node and its island
lights up, with a card that opens the project or the drive's details;
click a plugged-in drive and its lead lights; click the Mac and all of
them do. Double-click a
drive for its details, a project for its page. Drag to pan, ⌘ + scroll
to zoom, drag a drive out of its island to take it out of the project
(Lumpy asks first) or onto another project's island to add it there,
drag a project header to move its whole island (Lumpy remembers),
**Fit** to see everything, **Arrange** to put every island back.

### Drive library

Every drive is a card with a picture of what it is (a hard drive, an
SSD, a camera card or a RAID enclosure, from what macOS says about it;
change it under **Details → Labels → Picture on the card**), its light on
when it is plugged in, and a ring showing how full it is: pale until
70%, yellow after, red in the last 10%. Cards are tinted with the
project's colour. A plugged-in drive has an **Eject** button on its
card, in its details and on the map, which unmounts it the way the
Finder does so it can be unplugged; Lumpy refuses while one of its own
jobs is still reading or writing the drive, and says so when something
else has files open on it.

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
- Each folder can be marked a **backup drive** (the plan then copies only onto backup drives, and files found only in a backup folder are left alone), can **leave sub-folders out** (scratch, drafts, renders), and says where its **copies go**: where the project already has a folder on the destination drive, copies land there at the same relative paths (`01_1TB/09_OUTPUT/Final/x.mov` becomes `BS_8TB/Project Backup/Job/09_OUTPUT/Final/x.mov`). Lumpy works the pairing out from the project's folders and lets you override it per folder.
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

### Safe to format

A camera card (a drive with camera folders at its root, pictured as a
card, or marked *no backups here*) says **Safe to format** on its card,
in its details, on the map and in the menu bar once every file on it is
checksummed and has at least two verified copies on other drives. With
exactly one copy everywhere it says **Backed up once · keep the card**,
because the card is still the second copy.

**Safe to format?** on a card's sheet opens the one screen for that
question: every drive holding copies with how many were read back and
matched, every file that still lacks two verified copies and why, and
one verdict. When the verdict is safe, **Erase** formats the card in
place (same name, same filesystem, as the camera would), and only after
the card's name is typed; the erase runs as a job in Activity and the
library is brought up to date afterwards.

### Shoot-day report

On the Offload tab, pick a day and **Open** or **Save…** its report:
one page listing every card offloaded that day with its files, clips
and running time, where each copy went and how many were verified,
which cards are safe to format, the files still without two verified
copies, every file's xxh64 checksum, and the day's checks and scans.
It prints cleanly and is what a producer or an insurer asks for.

### Stills and raws without Resolve

Previews (the Filmstrip, a clip's details, duplicates in Space) are
made by Lumpy itself for JPEG, PNG and GIF, for **DPX** frames
(DaVinci Resolve's gallery stills and film scans: 8, 10, 12 and 16-bit
RGB, log stills brought up to a viewable picture) and for **DNG** and
other camera raws that carry a developed preview inside (Adobe's and
Blackmagic's do), and by macOS for HEIC, Canon CR3, Fujifilm RAF,
Photoshop and OpenEXR. Video frames come from ffmpeg.

### Filmstrip

The Clips page has a **Filmstrip** view beside the table: a tile per
clip with a frame from it, its start timecode and length, format and
drive. Move the mouse across a tile to step through four frames of the
clip; click it for the file's sheet. Frames come from plugged-in drives
(a tile says which drive to plug in otherwise), through ffmpeg for
video (`brew install ffmpeg`), and are kept in a `thumbs` folder next
to the library so each is made once. RED, BRAW and ARRIRAW clips have
no frame; stills do.

### Drive health

With smartmontools installed (`brew install smartmontools`), Lumpy
reads each plugged-in drive's own health report (SMART) once a day and
shows it under **Drive health** on the drive's sheet: the drive's
verdict, hours powered on, temperature, reallocated and pending
sectors, write life used on SSDs. A drive that reports it is failing,
or has sectors going bad, gets a red or amber badge on its card.
**Read now** asks again. Thunderbolt, SATA and internal drives answer;
most USB enclosures don't on macOS, and the sheet says so. Every
reading is kept: the sheet draws the temperature and bad-sector counts
over time, and a reading that is worse than the one before (more
reallocated or pending sectors, new uncorrectable or interface errors,
a jump in temperature) is called out as a trend, before the counts
alone would look bad.

### Notifications

When an offload, transfer or check ends, Lumpy posts a macOS
notification with how it went ("Done in 4m 12s", or the number of
problems); rescans and metadata reads only when something went wrong.
Settings → Notifications turns it off or keeps it to problems, and takes
a **webhook** URL (a Slack, Discord or Teams incoming webhook): the same
line is posted there, with the Mac's name, so the team hears when a
copy is done. **Send a test** checks the URL. The first notification
asks for permission, like any app.

### Keep drives awake

A spinning drive that nothing touches for a few minutes is spun down,
by macOS or by its own firmware, and the next read waits ten seconds
for it to come back, or finds it gone. Settings → Drives → **Keep
spinning drives awake while Lumpy is open** does what Amphetamine's
Drive Alive does: on a timer (every minute to every 10 minutes) Lumpy
writes and removes a tiny file, `.lumpy-awake`, at the root of every
plugged-in hard drive and RAID, so none goes idle long enough. SSDs and
camera cards don't spin and are left alone, as is a drive that is
busy with a job or can't be written. Any drive can opt out with **Let
it spin down** in its details. The top bar says how many drives are
being kept awake, and the map's legend too, and the menu bar panel has
the same two switches, so it can be turned on for a long copy and off
again without opening the app. Drives can only be kept
awake while the Mac is: **Keep the Mac awake too** runs macOS's
`caffeinate` while Lumpy is open, so the Mac never idle-sleeps (the
display still can), and it stops the moment Lumpy quits.

### Starting up

The window draws as soon as the engine answers, before any drive is
touched. Each card's backup badge is computed behind the request and
arrives a moment later (the page asks again until it does), so a big
library never holds the first paint. Drives plugged in at launch are
marked seen; one whose last scan is more than a day old is rescanned
as an ordinary job in Activity, one at a time, rather than every
drive being walked silently before the app is usable, as earlier
versions did. The ⟳ beside the page title rescans every plugged-in
drive the same way, one at a time in Activity, whenever you want the
library brought up to date by hand; a single drive has **Details →
Rescan**.

### One job at a time per drive

A drive does one thing at a time. Start a check while an offload is
still writing to that drive, or a second offload onto the same drive,
and the new job waits in the queue (shown as **Queued** in Activity and
the menu bar, with Cancel) and starts by itself when the drive is free.
Jobs on other drives run alongside. **Gentle mode** (Settings → Drives)
reads one file at a time and holds reads to about 150 MB/s, so the Mac
and the drive stay responsive for whoever is editing; checks take
longer.

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
reels, scenes, notes and tags. Opening Clips from a project shows that
project's clips (the filter can still be changed). Clicking a clip opens
it: a player when its drive is plugged in, the technical details, every
drive holding a copy, and a log section for scene, shot, take, tags,
notes and a QC mark (OK, flag or reject). Log entries follow the
content: set them on one copy and every copy has them. **Export CSV…**
writes the current list with every column; a project page has the same
button for just its clips.

The player is an edit-suite viewer, not a web one: space or **K**
plays and pauses, **L** plays forward and doubles the speed on each
press (up to 8×), **J** shuttles backwards the same way, **,** and
**.** step one frame (with shift, ten), **-** and **=** halve and double
the speed, Home and End jump to either end, **M** mutes, **F** fills the
screen, and the readout shows the clip's own timecode and the frame
count. **←** and **→** go to the previous and next clip in the list,
whether or not the current one plays. Playback itself is macOS's
(whatever QuickTime Player plays, Lumpy plays); RED, BRAW, ARRIRAW and
Canon RAW have no player or frame, because decoding them needs each
maker's own SDK, which is not something Lumpy can ship.

### Pull shots from an edit

The other way round from Archive: a selects reel, a trailer or a
festival cut names twenty shots from across a feature, and the job is
to gather their camera originals from wherever they are. **Projects →
Pull shots from an edit…** takes the export (FCPXML, Premiere or FCP 7
XML, an EDL, or a plain list of clip names) and matches every clip
against the whole library, every drive, plugged in or not. An edit cut
on proxies still points at the raws: `A001_C003_proxy.mp4` finds
`A001_C003.braw`, and a RED chunk finds its whole .RDC folder. Each
clip shows the copy Lumpy would pull (originals before proxies, camera
formats before transcodes) with a menu of the other copies, a
**proxy** badge when nothing better was found, and which drives to
plug in. Choose a folder and a name and **Copy**: one verified job per
source drive, each clip landing under a folder named after its drive
with its path kept, so nothing collides and the provenance is plain.
Clips whose drive isn't plugged in wait; choose the export again when
it is. Clips already on the destination drive are left where they are.

Or don't copy at all. **Copy paths** puts one full path per line on
the clipboard and **Save list…** writes a CSV (clip, path, drive,
plugged in), for Resolve, Premiere or whoever asked. **Relink in
DaVinci Resolve** goes one further: with Resolve open on the project
and *Preferences → System → General → External scripting using* set to
*Local*, Lumpy asks Resolve to replace each media pool clip with its
original, keeping every timeline use, so a cut made on proxies points
at the raws without a copy. It needs `python3` on the Mac (Xcode's
command line tools provide one). Activity shows each clip relinked, and
which were skipped because their drive isn't plugged in.

### Find projects

**Projects → Find projects…** looks through every logged drive's
index for folders that look like projects: numbered folders at a
drive's root the way Post Haste makes them (`04_BS_REDHAND_2025-03-01`)
and anything inside a folder named like `01_PROJECTS`. Template
subfolders (01_RAW, 09_OUTPUT…) and size-named folders (01_1TB) are
skipped. Each folder gets a name with the number, a house prefix (BS by
default; change the words to leave out and **Look again**), dates and
trailing initials dropped, and folders that start with the same word
are grouped as one project, so ICEMAN_EP1 and ICEMAN_EP2 on one drive
and REDHAND dailies on another land together. Rename a group (FT to
Family Ties), move a folder to another group or its own project with
the menu beside it, untick what isn't a project, and **Create**: one
project per ticked group, two copies wanted, every folder a source.
Folders already in a project are shown greyed with the project's name.
Nothing on any drive is touched.

### New project folder

**Projects → New project folder…** makes a job's folders from a
template: the Post Haste templates on this Mac are listed (any `.pht`
in Post Haste's Templates folder), or choose any folder. Fill in
Project Number, Client, Project Name, Date and Editor; the folder is
named by a pattern with those tokens (`[Project Number]_[Client]_
[Project Name]_[Date]_[Editor]` by default, with a live preview) and
tokens in the template's own file and folder names and small text
files are filled in too. Choose where it goes, and whether Lumpy should
make a project of it (with the copies wanted). The folder is created as
a job, the drive is logged, and the new project opens. The number steps
on and the client, editor, destination and template are remembered.

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
the timeline exports: FCPXML (Final Cut Pro, DaVinci Resolve), Premiere
Pro or FCP 7 XML, a CMX 3600 EDL, or a plain list of clip names (one
per line, or a CSV with a file-name column). Pick several at once
(⌘-click in the file dialog) or add them one at a time with **Add
another export…**; each shows as a chip with an × to leave it out
again. A clip any of them uses is kept. Lumpy matches the clips the
edits refer to against every video, audio and still in the project's
folders and shows what is used, what isn't (by folder, with sizes), and
what the edit refers to that isn't in the project at all (stock, music,
graphics). Matching is by clip name, so a name the edit uses anywhere
keeps every clip with that name, and a RED clip's chunks count as one.

Tick the folders to clear in the table (every folder with something
unused starts ticked; output, graphics and audio folders are often best
left alone) and the counts, the list and the buttons follow. Then either
**Move … unused into the folder** (an `_Unused` folder,
or any name you type, inside each project folder, keeping the clips'
paths so they can be put back) or **Move … unused to the Trash**,
which asks first. Every drive involved must be plugged in. Both run as
jobs in Activity, and the library follows the files: moved clips keep
their checksums and metadata at the new path, trashed ones are
forgotten. The Trash is the Mac's own, so nothing is gone until you
empty it. The first time, macOS may ask whether Lumpy may control the
Finder, which is how files reach the Trash.

### Space

The Space tab is for the Mac itself, the way DaisyDisk works: pick
Macintosh HD, Home or a plugged-in drive, **Measure**, and every
folder is drawn as rings, the bigger the slice the more it holds, with
a list beside it of what's inside the folder in view, biggest first.
Click a slice (or a row's chevron, or double-click a row) to look
inside, hover for a size, click the centre to go back up, and the
crumbs at the top of the list jump anywhere. Every row carries a verdict from where it
sits: **safe to clear** (an app makes it again), **look first** (your
own files, downloads, apps) or **critical** (macOS, an app's data,
keychains, mail, Photos). Tick what should go and **Move to the
Trash**, and before anything goes Lumpy looks at each item properly:
where it sits, when it last changed, whether a running app has it
open, whether it is a folder of a Lumpy project, and whether the
library holds a copy; critical items come unticked. Then it goes
through the Finder, so it can be put back, and the rings update.

Beside **Folders**, two more views of the same measurement.
**Duplicates** finds the same file twice: every file of 1 MB or more
is compared by length, the ones that share a length are read and
checksummed in full, and only identical contents count; pick which
copy to keep in each set and the rest are ticked for the Trash (two
copies on one disk protect nothing; a backup lives on another drive).
It runs in Activity, since a big disk takes a while. The sets can be
narrowed to pictures, clips, audio, documents or archives, and to one
top-level folder; pictures and clips show their previews side by side
so a match can be seen as well as proven; **Apply to all shown** keeps
the first copy of every set in view and ticks the rest. **Not touched in
a year** lists the big files neither changed nor opened in 365 days,
biggest first (opened is the access time macOS keeps for its own
disk), and apps not opened in a year by Spotlight's last-used date,
with their sizes. Old isn't the same as unneeded, so nothing is ticked
until you tick it, and the same check runs before anything goes. Show in Finder opens the item, and the eye on a picture or clip opens
it large, with ← and → stepping through the folder; a file Lumpy can't
render says why. Sizes are
what files take on disk; each folder keeps its 60 biggest items apart
and rolls the rest into "smaller items". The measurement runs as a job
(Activity shows the folder being read), is kept until Lumpy quits, and
**Measure again** refreshes it. Symlinks aren't followed and other
volumes mounted below the one measured are skipped; on the startup
disk that includes the Data volume's second mount at
/System/Volumes/Data (your files are counted once, under Users,
Applications and Library), the Preboot, VM and Update volumes, swap,
and everything under /Volumes. For Macintosh HD
or Home, **Usual suspects** lists the places a Mac fills with things
nobody needs (app caches, logs, Resolve's CacheClip, Adobe media
cache, Xcode derived data and simulators, iOS backups, Downloads, the
Trash, npm, Go, Cargo and Homebrew caches…) with their sizes, marked
*safe to clear* (the app makes it again) or *look first*, each with
its own tick box. A file the library already holds, by name and size,
on logged drives gets an **on N drives** badge, so footage copied to
the Mac for an edit can go once its originals are checked. The Mac's own
disk also holds system files, snapshots and purgeable space no app can
measure, so Finder's "used" can be more than what Lumpy finds. When
the measurement covers much less than the disk's used space, Lumpy
says so and offers to measure Macintosh HD; when macOS kept folders
from it (parts of Library are off limits without **Full Disk Access**),
it says how many and opens Privacy & Security so Lumpy can be added.

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

Under the gear: **General** (Appearance: **System**, **Light** or **Dark**, and a Style: **Modern** or **Classic** (System 7 chrome on either: square windows with a hard shadow, pixel glyphs, Chicago, a grey desktop in Light, with a **Desktop shade** slider from pale to charcoal); Lumpy is dark by default, the terminal, previews and the menu
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
