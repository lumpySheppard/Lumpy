# Testing Lumpy with real drives

A first hands-on test with one card and two drives. Plan on about an hour,
most of it waiting on copies.

**Use footage you can afford to lose.** Shoot a few minutes of test clips
onto the card, or use footage that is already backed up elsewhere. Keep
using your usual offload tool for real work until these tests pass.

## Before you start

- A card with test footage on it (a few GB is enough; one long clip
  over 4 GB is a useful extra).
- Two drives with free space. Note each one's format (Disk Utility →
  select the drive → format, e.g. APFS, exFAT, Mac OS Extended).
- Lumpy.app from the latest CI run (**Lumpy-macOS**), moved to
  Applications, then in Terminal:
  ```sh
  xattr -dr com.apple.quarantine /Applications/Lumpy.app
  sudo ln -sf /Applications/Lumpy.app/Contents/Resources/lumpy /usr/local/bin/lumpy
  ```

Write down anything that looks wrong, slow or confusing. For problems, the
**Copy** button on a terminal window copies the whole log.

## 1. Drives show up

1. Open Lumpy. If macOS asks whether Lumpy may access removable volumes,
   click **Allow**.
2. Plug in the card and both drives, one at a time.

**Expect:** each appears under **Connected** within a few seconds with the
right name, capacity and free space. In **Details**, *Volume UUID* is a
long ID like `A1B2C3D4-…`, not `name:…`.

3. Each card shows a picture of the drive: a hard drive, an SSD, a
   camera card (DCIM, CLIPS, PRIVATE… at the root) or a RAID enclosure,
   with its light on while plugged in, and a ring for how full it is.
   **Expect:** the pictures match the hardware; if one doesn't, change
   it under **Details → Labels → Picture on the card**.
4. Open a drive's **Details** and give it a display name, client, project
   and color. Close and reopen the app: the labels are kept.

## 1a. Eject

1. On a plugged-in drive's card, click the eject arrow next to the
   pencil (or **Eject** in its details). **Expect:** the drive
   disappears from Finder's sidebar, its card says *Last seen* a moment
   later, and the toast says it is safe to unplug.
2. Start a checksum on a drive and try to eject it. **Expect:** a
   refusal naming the running job.

## 1b. The map

1. Open the **Map** tab. **Expect:** this Mac on the left with a dotted
   green line to each plugged-in drive (click the Mac to see each one's
   bus),
   the drives in the middle, each project on the right with lines to
   the drives that hold it, labelled with the size on that drive and
   whether it is backed up. Double-click a drive: its details open.
   Double-click a project: its page opens.
2. Click a project. **Expect:** everything else dims, its drives stay
   lit, and a card lists them; **Open project** goes to its page.
   Click a drive: its projects stay lit; **Details** opens the sheet.
3. Drag a project to the left of its drives. **Expect:** its wires now
   leave it on the right and enter the drives on the left, never under
   a node. Switch tabs and come back: it stays where you put it.
   **Arrange** puts everything back.

## 2. A normal offload

1. **Offload** tab → source: the card. Destinations:
   `/Volumes/DRIVE_1/LUMPY_TEST` and `/Volumes/DRIVE_2/LUMPY_TEST`
   (type the folder name after choosing the drive). Leave *Read the card a
   second time* ticked.
2. **Start offload** and watch the terminal.

**Expect:** a cyan `copied` line per file, then `verified` lines for each
drive and a `source ok` line per file, and finally two green `OK … all N
files verified` lines. Note the copy speed and total time compared with
your usual tool.

3. In Finder, check that `LUMPY_TEST/<card name>` on both drives has the
   same number of items and size as the card (select → **Get Info**), and
   contains an `ascmhl` folder.

## 3. Check the copies independently

Don't just take Lumpy's word for it. In Terminal:

```sh
lumpy verify "/Volumes/DRIVE_1/LUMPY_TEST/<card name>"
lumpy compare "/Volumes/<card>" "/Volumes/DRIVE_2/LUMPY_TEST/<card name>"
md5 "/Volumes/<card>/<some clip>" "/Volumes/DRIVE_1/LUMPY_TEST/<card name>/<same clip>"
```

**Expect:** both Lumpy commands end with `OK`, and the two `md5` values
are identical (md5 is computed by macOS, not Lumpy). If you have
Silverstack, Hedge or ShotPut Pro, open or verify the `ascmhl` folder with
it too: it should accept Lumpy's manifest.

## 4. Running the same offload again

Start exactly the same offload again.

**Expect:** `exists … (checked in the verify pass)` lines instead of
`copied`, nothing rewritten, and `OK` for both drives. Each `ascmhl`
folder now has a second `.mhl` file.

## 5. Cancelling part-way

1. Offload the card again, to a new folder name (`LUMPY_TEST_2`).
2. When the progress bar is about a third of the way, click **Cancel**.

**Expect:** the job says *Cancelled*. In Finder, `LUMPY_TEST_2` holds only
complete files: no half-written clip under its real name (Lumpy writes to
a hidden `.name.lumpy-partial` file and renames it only when finished).

3. Start the same offload again. **Expect:** it finishes with `OK`.

## 6. A damaged copy is caught

Damage one copy on purpose, on the test folder only. In Terminal, pick a
clip on DRIVE_2 and change one byte in the middle of it:

```sh
printf 'X' | dd of="/Volumes/DRIVE_2/LUMPY_TEST/<card name>/<path to clip>" bs=1 seek=100000 conv=notrunc
```

1. **Logged** → DRIVE_2 → **Details** → **Check drive now**.

**Expect:** the check ends with a red `FAIL … does not match its recorded
checksum` line for that clip. The DRIVE_2 card shows **1 damaged**. In the
card's **Details**, that clip now has only one good copy.

2. Delete the damaged clip from DRIVE_2, run the offload from step 2
   again, then **Check drive now** on DRIVE_2.

**Expect:** the clip is copied again, the check passes, and the damage
badge is gone.

## 7. Checking a drive that isn't plugged in

1. Eject DRIVE_1 (Finder → Eject).
2. **Logged** → DRIVE_1 shows *Last seen …*. Open **Details** →
   **Queue check**. The card shows *Check queued*.
3. Plug DRIVE_1 back in with Lumpy open.

**Expect:** within a few seconds Lumpy starts checking it by itself and
posts a notification when it's done. The card then shows *Checked* with
today's date.

4. Eject the card. Open the card's **Details** in **Logged**.

**Expect:** *2× backed up*, with copies listed on both drives. Type a clip
name into the search box: it's found on all three.

## 8. A project: is the whole setup backed up?

1. **Logged** → DRIVE_1 → **Details** → in *Browse drive*, hover the
   `LUMPY_TEST` folder and click its **+** → **New project…**. Name it,
   leave *2 copies*, **Create**.

**Expect:** the project page opens with one folder in it. The health bar
is green: the test footage is also on the card and on DRIVE_2, and copies
anywhere in the library count.

2. Put something new on DRIVE_1 only: copy a small folder from your Mac
   to `/Volumes/DRIVE_1/LUMPY_TEST/extra`. In the drive's **Details** click
   **Rescan**, then **Checksum new files**.

**Expect:** the project's sole-copy figure grows by that folder's size, and
a plan step appears: *DRIVE_1 → DRIVE_2* (or the card, whichever has
more room) with **Run this step**. Under *Back up onto*, choose *Only
these drives* and tick DRIVE_2 to make sure it goes there.

3. Click **Run this step**.

**Expect:** a job in **Activity** with the terminal; the files land in
`/Volumes/DRIVE_2/<project name>/extra/…` with an `ascmhl` folder beside
them; back on the project page the step is gone and the sole-copy figure
has dropped.

4. Eject DRIVE_2 and look at the project page again.

**Expect:** the drive shows *last seen …*; the plan still lists any step
that needs it, as *Plug in DRIVE_2*.

## 8b. A project folder from the Post Haste template

1. **Projects → New project folder…**. **Expect:** your Post Haste
   templates listed, with their top-level folders; the number, date and
   editor filled in; the preview updating as you type.
2. Fill in the client and a project name, choose a drive folder to
   create it in, leave *Add it to Lumpy as a project* on, and create.
   **Expect:** a job in Activity, the folder tree in Finder with the
   film leader in 04_Graphics Sources, and the new project page opening
   with that folder as its source. Next time the number is one higher.

## 9. Clips: codec, frame rate, timecode

1. Open the **Clips** tab.

**Expect:** every clip from the offload is listed with its codec and
frame size, frame rate, length, start timecode, audio format and camera,
read from the files during the offload (Lumpy reads QuickTime/MP4, MXF
via ffprobe when installed, WAV/BWF, JPEG/PNG/TIFF/DNG). Compare a few
rows against what the camera or Silverstack says — frame rate and
timecode especially.

2. Click a clip.

**Expect:** a preview plays (the card is plugged in), with the clip's
technical details, every drive holding a copy, and a log section. Fill
in scene, take and a note, set QC to **Flag**, then open the same clip
on DRIVE_1 (search for its name in **Logged**, or from the Clips tab
filtered by drive).

**Expect:** the scene, note and flag are there too: edits follow the
content to every copy.

3. Filter by frame rate or codec, then **Export CSV…**.

**Expect:** a spreadsheet with one row per clip and all the columns,
ready to send to post.

4. For a drive that was logged before this build (no metadata yet):
   **Details** → **Read metadata**.

**Expect:** a job in Activity, and the drive's clips appear in Clips.

## 10. Proxies

1. On a drive, make `Online` and `Offline` folders side by side (any
   parent). Put two camera clips in Online and a proxy for one of them,
   named the same with `_proxy` on the end, in Offline. Rescan the drive.
2. Open the drive's **Details**. **Expect:** a **Proxies** section with
   "1 of 2 originals have a proxy", the missing clip listed under
   "1 without a proxy", and **Copy list** putting its name on the
   clipboard.
3. Make a project of the drive. **Expect:** the same card on the project
   page under the plan.

## 11. Archive: what the edit didn't use

1. Make a project of DRIVE_1's test folder. Export the timeline from
   Resolve or Premiere (File → Export → FCPXML or Final Cut Pro XML),
   or write a text file with the names of two of the clips, one per
   line. Make a second list with one more clip, to add afterwards.
2. On the project page, **Archive** → **Choose timeline exports…** and
   pick it. **Expect:** the clips named are "used", every other clip in
   the folder is "not used", sizes by folder, and a name that is not in
   the folder appears under "not in this project". **Add another
   export…** with the second list: one more clip becomes "used" and
   both chips show; the × on a chip takes its clips back out.
3. Untick one folder in the table. **Expect:** the buttons' counts drop
   and its clips leave the unused list. Tick it again.
4. **Move … unused into the folder**. **Expect:** a job in Activity;
   in Finder the unused clips are now under `_Unused` inside the
   project folder, in their old sub-folders; the drive's file browser
   in Lumpy shows them there with their checksums intact.
5. Choose the export again (the moved clips are still in the project)
   and **Move … unused to the Trash…**, then confirm. **Expect:** the
   clips are in the Trash, gone from the library, and the project's
   health no longer counts them.

## 12. Status page for coworkers

1. Click the gear (top right). Turn on **Publish the status page**,
   choose a folder in your Dropbox or iCloud Drive, click **Publish
   now**, then **Save**.

**Expect:** `index.html` and `status.json` appear in that folder. Open
`index.html` in Safari (or send its link to someone): it lists every
drive, where it was last seen, each project's health, and a file search
that says which drives hold a file. It needs no server and no Lumpy.

2. Run another offload, then reload the page.

**Expect:** it updated by itself when the job finished. Quit Lumpy and
reload once more: it updated again on the way out.

3. (Optional) If you have a web server that accepts HTTP PUT, enter its
   URL and token; or put an `rsync`/`scp` line in the command field. The
   page is sent after each write.

## 13. Updating in place

Needs a release to exist: GitHub → Actions → **Release** → *Run
workflow* → `0.6.0` (published in a few minutes). The repository is
public, so no token is needed.

1. Gear → **Check now**.

**Expect:** *Lumpy 0.6.0 available*, and an **Update to 0.6.0** button
in the top bar.

2. **Install 0.6.0**, then watch Activity.

**Expect:** download with a progress line, *Checksum OK*, *Replacing
/Applications/Lumpy.app*, then *installed*. The top bar says *relaunch
to finish*; **Relaunch Lumpy** quits and reopens the app, which now
reports the new version in Settings. Drives, labels, projects and clip
metadata are all still there.

## 14. Light mode

1. Gear → **General** → Appearance → **Light**. **Expect:** the whole
   window turns light at once, the title bar included; the terminal in
   Offload, clip previews and the menu bar panel stay dark. Quit and
   reopen: still light.
2. **System** follows the Mac's own setting (System Settings →
   Appearance), including Auto switching at sunset. **Dark** is the
   default.

## 15. Menu bar

1. Start an offload, then close the Lumpy window (not Quit).

**Expect:** the NAS glyph in the menu bar shows the offload's percentage
beside it; clicking it drops a panel with the progress bar, speed and
time left, the plugged-in drives and their free space, and **Open
Lumpy**, which brings the window back. The offload keeps running with
the window closed. **Quit** in the panel (or ⌘Q) quits, with a warning
if a job is running.

## 16. Logging without the app open

```sh
lumpy agent install
```

Quit Lumpy, unplug one drive and plug it back in. After a few seconds:

```sh
tail ~/Library/Logs/Lumpy/agent.log
lumpy drives
```

**Expect:** the log shows the drive being mounted and logged, and `lumpy
drives` lists it with a fresh *last seen* time.

## Afterwards

Delete the `LUMPY_TEST` folders from both drives when you're done. To
start the library from scratch, quit Lumpy and delete
`~/Library/Application Support/Lumpy`.

Send back: what passed, anything that failed (with the copied log),
copy and verify speeds next to your usual tool's, and anything that was
confusing.
