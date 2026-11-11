# EriksTV (datamosh OpenFX plugin for DaVinci Resolve)

A datamosh effect you can apply like any other Resolve effect. It simulates
what happens when you delete an I-frame from compressed video: the next shot's
motion vectors keep pushing the previous shot's pixels around.

Resolve hands plugins fully decoded frames, so the plugin can't edit the
codec's real motion vectors. Instead it rebuilds them the way an encoder
does, with block matching on 8, 16 or 32 px macroblocks, then decodes them
onto the wrong picture. You get the same blocky drag and the new shot's edges
bleeding through, without leaving Resolve.

## Install (macOS)

1. Get the `EriksTV-macOS` artifact from the latest **OFX plugins** run in
   GitHub Actions and unzip it.
2. Run `bash install-macos.sh` in that folder. It copies
   `EriksTV.ofx.bundle` to `/Library/OFX/Plugins` and clears the macOS
   download quarantine.
3. Restart Resolve. The effect is in **Effects → OpenFX → Lumpy → EriksTV**.

It's a universal binary (Apple silicon and Intel) for macOS 11 or later, and
works in Resolve and Resolve Studio.

## Use

**Classic mosh across a cut (Drag mode):**

1. Put both shots in a **compound clip**, or apply the effect to the
   second shot.
   - The effect needs the frame *before* the cut. On the second shot alone,
     that frame doesn't exist, so the mosh starts from the shot's own first
     frame instead.
   - A compound clip (or a pre-rendered clip) gives the classic look of
     shot A dragged by shot B.
2. Drop **EriksTV** on it.
3. Park the playhead on the **first frame of the new shot** and press
   **Start Mosh Here**. That frame is the "deleted I-frame": it keeps
   showing the last frame of shot A.
4. Play. Shot B's motion smears shot A until **Mosh Length** runs out, then
   the picture snaps clean, like a fresh I-frame.

**Bloom mode** freezes the frame at Mosh Start and re-applies its motion
over and over, like a duplicated P-frame. Pixels stream out in the
direction things were moving.

**Looks.** Pick one from **Look** and press **Apply Look** to set every
Motion, Time, Block and Colour control at once, then tweak from there.
Your placement settings (Mode, Mosh Start, Length, Re-mosh Every, Seed) and
Mix are left alone.

| Look | What it's for |
| --- | --- |
| Classic Drag | The plain mosh: resets all glitches to off |
| Film Melt | Small mixed blocks, drips and drifting colour blotches; the painterly melt |
| Game Tear | Big mixed blocks, flat colour tiles, vertical bars, saturated colour |
| Stutter & Jump | Motion repeats, freezes, jumps back and forward, reverses |
| Colour Riot | Hue and saturation build up, colour slides off shapes, colour errors |
| Liquid Smear | Smooth, over-driven smear with colour sliding |

### Controls

**Top level**

| Control | What it does |
| --- | --- |
| Mode | Drag (drop I-frame) or Bloom (repeat P-frame) |
| Look / Apply Look | Presets, see above |
| Mosh Start Frame / Start Mosh Here | Where the mosh begins |
| Mosh Length | Frames until the picture cleans up |
| Re-mosh Every | 0 = mosh once. Otherwise the mosh restarts every N frames from Mosh Start: it cleans up, then breaks again |
| Seed | Changes the random pattern of every glitch; the same seed always gives the same result |
| Mix | Blend with the original (this is the only animatable control) |

**Motion**

| Control | What it does |
| --- | --- |
| Block Size | Base macroblock size: 4, 8, 16, 32 or 64 px |
| Block Variety | 0 = all blocks the same size. Higher values mix 4–64 px blocks like a real encoder: small on detail, big on flat areas, plus some randomness |
| Search Range | How far to look for motion (px). Raise it for fast motion; it costs render time |
| Motion Scale | Over-drive (>1) or slow (<1) the smear |
| Residual Bleed | 0 = pure smear of the old picture; 1 = what a real decoder does (lots of new-shot ghosting) |
| Heal | Per-frame blend back toward the real picture, so the mosh resolves gradually |
| Smooth Vectors | Liquid smear instead of macroblock tearing |

**Time Glitches** (Drag mode)

Each chance is per frame. Events last a random number of frames up to
Event Length, and jumps go up to Jump Distance.

| Control | What it does |
| --- | --- |
| Repeat | The same frame's motion is applied again for a few frames, so the smear lurches and blooms |
| Freeze | The picture holds still for a few frames (a repeated frame) |
| Jump Back | The motion jumps back in time and replays |
| Jump Forward | The motion skips ahead in one big lurch |
| Reverse | The motion runs backwards for a few frames |
| Rhythm (every N frames) | Events can only start every N frames from Mosh Start. Set a chance to 1 to hit every beat |

**Block Glitches** (each value is the share of blocks affected; Glitch Hold
is how long each glitch lasts)

| Control | What it does |
| --- | --- |
| Stuck | Blocks stop updating and hang in place |
| Scatter | Blocks grab pixels from a random place |
| Drip | Blocks smear their top row downward into vertical streaks |
| Flatten | Blocks collapse to one flat colour |

**Colour**

| Control | What it does |
| --- | --- |
| Chroma Drift | Colour moves further than brightness, so colour slides off shapes |
| Colour Bleed | Boosts the colour in the residual, so wrong colours pile up |
| Saturation Build | Saturation grows every frame |
| Hue Drift | Degrees the hue turns every frame |
| Colour Errors / Error Strength | Blotches of wrong colour land, then get dragged along by the motion (damaged DC data in a real codec) |

Everything random depends only on the seed, the frame number and the
block's position. Scrubbing, re-rendering and playing straight through
therefore always give identical frames. The exception is proxy or
reduced-resolution playback: blocks sit in different places at a lower
resolution, so the glitch pattern differs from the full-resolution render.

### Performance

- Each frame of a mosh depends on every frame before it, back to Mosh Start
  (or the latest restart, with Re-mosh Every).
- Playing forward is cheap: the plugin caches its last results, so each new
  frame costs one more step, about 50 ms at 1080p on top of the normal
  render (measured on a 4-core Linux machine). Block Variety and the colour
  controls add a little more.
- Jumping into the middle of a long mosh cold has to replay it from the
  start. Use Render Cache or render-in-place once you're happy with it.
- Proxy and reduced-resolution playback work: block size and search range
  scale with the render resolution.

## Build

```
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

- On macOS this builds `build/EriksTV.ofx.bundle` (universal). To install
  your own build, copy that bundle to `/Library/OFX/Plugins`.
- CI (`.github/workflows/ofx-plugins.yml`) builds and tests on macOS and
  Linux, ad-hoc signs the bundle and uploads it.

### What's here

- `src/MoshEngine.*`: the mosh itself (block-matching motion estimation,
  motion-compensated prediction, drag/bloom steps). Doesn't depend on OFX.
- `src/DataMoshPlugin.cpp`: the OFX glue (parameters, frame fetching,
  caching), using the plain OFX C API.
- `../ofx-testhost/minihost.cpp` (shared with the other plugins): a tiny OFX
  host. CTest uses it to load the built plugin, render synthetic two-shot footage in both modes, with every glitch on and
  with a Look applied, and check that out-of-order rendering (scrubbing)
  matches playing straight through.
- `tools/mosh_cli`: runs the engine on raw video piped through ffmpeg, for
  previewing settings outside Resolve:

  ```
  ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
    build/mosh_cli 1920 1080 --start 48 --length 60 --residual 0.3 --variety 0.6 --streak 0.05 |
    ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - -pix_fmt yuv420p out.mp4
  ```

- `../third_party/openfx/include` (shared): OpenFX 1.5.1 headers (BSD-3-Clause,
  see `third_party/openfx/LICENSE.md`).
