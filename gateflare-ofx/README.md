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
   - **Isolate Lights** uses a morphological top-hat: it removes every
     bright feature bigger than a light. Lit walls and ceilings, often
     nearly as bright as the clipped lamps in them, don't flare.
2. **Throw it in.** That light becomes a plume that fades with distance
   from the edge and widens as it goes. A thin glow runs right along the
   edge, where the light hits the gate.
3. **Composite.** It's added in linear light, then the picture is cropped
   to the gate. On display encodings it's screened in, so it never goes
   past white.

## Use

Drop **GateFlare** (Effects → OpenFX → Filters → Lumpy) on a clip.

Each light's flare appears as it reaches the gate edge and lingers after
it's gone.
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
| Shape | **Plume**: a soft fan that widens into the frame. **Beam**: a narrow, parallel-sided column straight in from the edge (the streak in *Road to Perdition*, or a cool column from the top). **Blade**: a hard-edged wedge that can lean. **Ring**: a faint ghost arc around the light beyond the edge (*A Clockwork Orange*) |
| Blade Angle / Blade Softness | Blade: how far it leans from straight in, and how soft its sides are (0 = razor sharp) |
| Ring Size / Ring Width | Ring: radius and line thickness |
| Flare Colour | **Source** (the lights' own colour), **Cool Blue**, **Cyan**, **Warm**, or **Custom** (white coloured by Tint). Tint applies on top of all of them |
| Source Threshold / Softness | How bright (as encoded) something must be to flare (default 0.9) |
| Isolate Lights | 1 (default): only small things much brighter than their surroundings flare, such as lamps, practicals and speculars. Big bright areas like lit walls don't. 0: anything over the threshold flares, windows and walls included |
| Light Size (%) | With Isolate Lights: the biggest thing that still counts as a light. Raise it for big practicals or windows |
| Intensity | Strength of the flare. Lights are usually clipped in the footage, so they get a built-in boost (about 6×) that tapers off for lights that are genuinely very bright in scene-linear footage |
| Reach (%) | How far beyond the edge a source still flares (% of frame height) |
| Depth (%) | How far the flare reaches into the frame |
| Spread (%) / Fan | Width of the flare along the edge (beam and blade width), and how much wider it gets as it goes in (plume and blade; a beam never widens) |
| Edge Glow | A thin glow right along the gate edge |
| Source Colour / Tint | With Flare Colour = Source: take the lights' colour (1) or go white (0). Tint colours every mode |
| Edges | Which edges flare |
| Input Encoding | How the images are encoded, so the flare is added in linear light. Same list as FilmFlash: Rec.709, DaVinci Intermediate, Cineon, camera logs (LogC3/4, S-Log3, V-Log, Log3G10, Apple Log, BMD Film Gen 5) and HLG (iPhone footage) |

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
