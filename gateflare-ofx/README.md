# GateFlare (OpenFX effect for DaVinci Resolve)

Film-gate edge flare. Bright sources just outside the frame flare in from
the edge of the gate as they enter or leave the shot.

## What it imitates

A lens throws an image circle bigger than the film gate. A light that's
just outside the frame line still lands on the gate: it hits the edge of
the aperture plate and scatters back onto the film. A soft plume pours
into the frame from the edge, right where the light is. It builds as the
light approaches the edge, peaks as it crosses, and lingers for a moment
after it's left the shot.

To recreate it, GateFlare crops in slightly. The cropped-off margin
stands for the part of the image circle beyond the gate. Then, for each
edge:

1. **Find the light.** Bright sources in a band straddling the edge
   (mostly just outside it) are found in linear light, keeping their
   colour.
2. **Throw it in.** That light becomes a plume that fades with distance
   from the edge and widens as it goes. A thin glow runs right along the
   edge, where the light hits the gate.
3. **Composite.** It's added in linear light, then the picture is cropped
   to the gate. On display encodings it's screened in, so it never goes
   past white.

## Use

Drop **GateFlare** (Effects → OpenFX → Filters → Lumpy) on a clip.
Then:
1. Turn on **Show Gate & Sources**. It shows the whole picture with the
   gate in red, the margin darkened, and the lights that will flare in
   magenta.
2. Adjust **Gate Crop** so the margin holds the lights you want flaring
   in, and **Source Threshold** until the right things are magenta.
3. Turn Show Gate & Sources off and shape the flare.

| Control | What it does |
| --- | --- |
| Gate Crop (%) | How much of the picture lies beyond the gate; the shot is zoomed in by this much |
| Crop to Gate | Scale the gate up to fill the frame. Turn off to keep the full picture and crop it yourself |
| Show Gate & Sources | Setup view: the gate, the margin, and the detected sources |
| Source Threshold / Softness | How bright (as encoded) something must be to flare |
| Intensity | Strength of the flare |
| Reach (%) | How far beyond the edge a source still flares (% of frame height) |
| Depth (%) | How far the flare reaches into the frame |
| Spread (%) / Fan | Width of the flare along the edge, and how much wider it gets as it goes in |
| Edge Glow | A thin glow right along the gate edge |
| Source Colour / Tint | Take the lights' colour (1) or go white (0), then tint |
| Edges | Which edges flare |
| Input Encoding | How the images are encoded, so the flare is added in linear light. Same list as FilmFlash: Rec.709, DaVinci Intermediate, Cineon, camera logs (LogC3/4, S-Log3, V-Log, Log3G10, Apple Log, BMD Film Gen 5) |

On log or a colour-managed timeline, set Input Encoding to match. The flare
is then added in scene-linear light, and your output transform handles the
highlights.

It works on single frames, so it doesn't need handles or other frames,
and it renders the same however the timeline is played or scrubbed.

## Build

```
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

The tests generate a dark test clip with a bright bar that drifts out
through the top edge (`tools/make_test_clip`). They check:

- with the flare and the crop off, frames come back untouched
- the frame where the bar sits just beyond the gate is clearly brighter
  than without the flare
- nothing goes past white on a display encoding
- every option renders deterministically

`tools/gate_cli` previews it on raw video piped through ffmpeg.
