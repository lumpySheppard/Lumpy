# DataMosh (OpenFX plugin for DaVinci Resolve)

A datamosh effect you can apply like any other Resolve effect. It simulates
what happens when you delete an I-frame from compressed video: the next shot's
motion vectors keep pushing the previous shot's pixels around.

Resolve hands plugins fully decoded frames, so the plugin can't edit the
codec's real motion vectors. Instead it rebuilds them the way an encoder
does, with block matching on 8, 16 or 32 px macroblocks, then decodes them
onto the wrong picture. You get the same blocky drag and the new shot's edges
bleeding through, without leaving Resolve.

## Install (macOS)

1. Get the `DataMosh-macOS` artifact from the latest **DataMosh OFX** run in
   GitHub Actions and unzip it.
2. Run `bash install-macos.sh` in that folder. It copies
   `DataMosh.ofx.bundle` to `/Library/OFX/Plugins` and clears the macOS
   download quarantine.
3. Restart Resolve. The effect is in **Effects → OpenFX → Lumpy → DataMosh**.

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
2. Drop **DataMosh** on it.
3. Park the playhead on the **first frame of the new shot** and press
   **Start Mosh Here**. That frame is the "deleted I-frame": it keeps
   showing the last frame of shot A.
4. Play. Shot B's motion smears shot A until **Mosh Length** runs out, then
   the picture snaps clean, like a fresh I-frame.

**Bloom mode** freezes the frame at Mosh Start and re-applies its motion
over and over, like a duplicated P-frame. Pixels stream out in the
direction things were moving.

| Control | What it does |
| --- | --- |
| Mode | Drag (drop I-frame) or Bloom (repeat P-frame) |
| Mosh Start Frame / Start Mosh Here | Where the mosh begins |
| Mosh Length | Frames until the picture cleans up |
| Block Size | 8 px tracks motion closely; 32 px is chunky and glitchier |
| Search Range | How far to look for motion (px). Raise it for fast motion; it costs render time |
| Motion Scale | Over-drive (>1) or slow (<1) the smear |
| Residual Bleed | 0 = pure smear of the old picture; 1 = what a real decoder does (lots of new-shot ghosting) |
| Heal | Per-frame blend back toward the real picture, so the mosh resolves gradually |
| Smooth Vectors | Liquid smear instead of macroblock tearing |
| Mix | Blend with the original (this is the only animatable control) |

### Performance

- Each frame of a mosh depends on every frame before it, back to Mosh Start.
- Playing forward is cheap: the plugin caches its last results, so each new
  frame costs one more step, about 40 ms at 1080p on top of the normal
  render.
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

- On macOS this builds `build/DataMosh.ofx.bundle` (universal). To install
  your own build, copy that bundle to `/Library/OFX/Plugins`.
- CI (`.github/workflows/datamosh-ofx.yml`) builds and tests on macOS and
  Linux, ad-hoc signs the bundle and uploads it.

### What's here

- `src/MoshEngine.*`: the mosh itself (block-matching motion estimation,
  motion-compensated prediction, drag/bloom steps). Doesn't depend on OFX.
- `src/DataMoshPlugin.cpp`: the OFX glue (parameters, frame fetching,
  caching), using the plain OFX C API.
- `tests/minihost.cpp`: a tiny OFX host. CTest uses it to load the built
  plugin, render synthetic two-shot footage in both modes, and check that
  out-of-order rendering (scrubbing) matches playing straight through.
- `tools/mosh_cli`: runs the engine on raw video piped through ffmpeg, for
  previewing settings outside Resolve:

  ```
  ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
    build/mosh_cli 1920 1080 --start 48 --length 60 --residual 0.3 |
    ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - -pix_fmt yuv420p out.mp4
  ```

- `third_party/openfx/include`: OpenFX 1.5.1 headers (BSD-3-Clause, see
  `third_party/openfx/LICENSE.md`).
