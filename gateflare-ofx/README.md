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
3. **Composite.** By default it's added as exposure on a film negative
   (odds space, soft shoulder), so it glows into the highlights without
   clipping. Then the picture is cropped to the gate.

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

The controls run in setup order: **Presets**, **Sources**, **Composite &
Input**, **Gate**, then the looks (**Top & Bottom**, **Left & Right**, and
a **Custom Look** for each) and **Edges**.

### Presets

| Control | What it does |
| --- | --- |
| Preset | Choosing one sets the controls to it. **Gummo Side Blast**: the side flare matched to the *Gummo* shot, loaded into the Custom Look controls so you can adjust it (Left & Right use Custom, Top & Bottom Soft Haze, Gate Crop 16, Light Size 8). It renders exactly like the Side Blast look. **Defaults**: everything back to how it starts. **User 1-8**: your own |
| Apply Preset | Sets the controls to the chosen preset again |
| Save To / Save Preset | Saves every control as it is now to that user slot |

A preset doesn't change Show Gate & Sources. The Gummo preset also leaves
Source Threshold, Input Encoding and Seed alone, since they depend on the
footage. User presets are small text files, shared by all projects, in
`~/Library/Application Support/Lumpy/GateFlare/` (`user1.gfpreset` ...);
copy them to another machine to share them.

### Looks, and top/bottom vs left/right

Flares from the side edges often look different from those at the top
(the side ones in *Gummo* blast in wide, blue and streaky just after the
light has gone). So the **Top & Bottom** and **Left & Right** edges each
have their own look:

| Control | What it does |
| --- | --- |
| Look A / Look B | A built-in look, or **Custom** (the Custom Look controls below) |
| Blend A > B | 0 = all Look A, 1 = all Look B, between = a mix of the two flares |
| Intensity | Strength of this pair's flares (0 turns them off) |
| Flare Start Offset (%) | Added to the looks' Flare Start: negative = later |
| Lean | Tilts this pair's flares, in degrees, on top of the look's own (Pinch and Blade looks) |
| Reach x / Depth x / Spread x | Scale the look's Reach (how far beyond the edge a light keeps flaring), Depth (the flare's length into the frame) and Spread (its width where it starts). They work on the built-in looks as well as Custom |
| Focus Offset (%) | Pinch looks (Side Blast, or Custom with Shape = Pinch): moves the convergence point further in (+), or toward the edge and off screen (-) |
| Tint | Colours this pair's flares |

By default both pairs use Look A = Custom. Look B is **Soft Haze** for
Top & Bottom and **Side Blast** for Left & Right, so you can raise
**Blend** to bring a look in.

| Look | From | Character |
| --- | --- | --- |
| Soft Haze | The warehouse test | A soft, streaky plume in the lights' own colour (the default settings) |
| Thin Streak | *Road to Perdition* | A long, thin, faint streak straight in, for a light leaving the side |
| Cool Column | Stills | A cool column straight down from the top |
| Blue Blade | Stills, *A Clockwork Orange* | A hard-edged blue wedge, leaning slightly |
| Side Blast | *Gummo*, matched frame by frame | Once the light and its glow have left the frame, a thin violet streak builds over a few frames into a long wedge of rays converging on a hot point at the edge, aimed where the gate edge reflects the light; then it shortens and goes teal. Its direction, length, width and rays change every frame. Needs room beyond the gate: Gate Crop 12-16% |


Side Blast starts after the light has left the frame (Flare Start −0.8%).
A light can only flare as far past the edge as **Gate Crop** leaves picture
for, so on footage with a small margin, raise Gate Crop or the Flare Start
Offset if Side Blast doesn't fire.

### Gate

| Control | What it does |
| --- | --- |
| Gate Crop (%) | How much of the picture lies beyond the gate; the shot is zoomed in by this much |
| Crop to Gate | Scale the gate up to fill the frame. Turn off to keep the full picture and crop it yourself |
| Show Gate & Sources | Setup view: the gate, the margin, and the detected sources |

### Custom Look: Top & Bottom, and Custom Look: Left & Right

Each edge pair has its own Custom Look, with the same controls, used
wherever that pair's Look A or B is **Custom**.

| Control | What it does |
| --- | --- |
| Shape | **Plume**: a fan that widens into the frame. **Beam**: a narrow, parallel-sided column straight in from the edge. **Blade**: a hard-edged wedge that can lean. **Pinch**: a real focus. Each light's rays converge from the edge to a focal point Pinch Length in, cross there and diverge again at the same angle (Fan); Spread sets how tight the focus is. The rays break up along their length, carry fine filaments, have ragged edges and blur together at the focus |
| Aim | Pinch: 1 = each flare points where the gate edge reflects its light. Light from the lens reaches the edge travelling outward from the frame's centre; the edge reflects it back in, so the flare tilts away from the centre line (by its offset along the edge over half the frame across) and swings as the light moves. 0 = straight in |
| Evolve | Pinch: the flare grows as its light gets clear of the edge, then shortens and narrows as the light goes further out |
| Hue Drift | Pinch: degrees the colour turns over the flare's life. Negative: violet, blue, cyan, green (Side Blast) |
| Wait for Exit | 1: a light doesn't flare while it, or its glow, still shows inside the edge. The flare then builds up over **Onset** of travel, measured from when the glow clears the edge, and its life (Evolve, Hue Drift) runs from there until the light leaves the picture. Needs Gate Crop room beyond the edge |
| Saturation | The flare's colour saturation (also per edge pair, multiplying the look's) |
| Onset (%) | How gradually a light's flare comes in once it passes Flare Start: small switches on, large builds over a few frames |
| Pinch Length (%) / Lean | Pinch: how far in the focal point is (-10 to 10; negative puts it beyond the edge, off screen, so only the fanning-out part shows), and how many degrees the whole hourglass leans (positive = up on the left and right edges, left on the top and bottom) |
| Blade Angle / Blade Softness | Blade: how far it leans from straight in, and how soft its sides are (0 = razor sharp) |
| Flare Colour | **Source** (the lights' own colour), **Cool Blue**, **Cyan**, **Warm**, or **Custom** (white coloured by Tint). Tint applies on top of all of them |
| Intensity | Strength of the flare. Lights are usually clipped in the footage, so they get a built-in boost (about 6×) that tapers off for lights that are genuinely very bright in scene-linear footage |
| Flare Start (%) | How far from the gate edge a light starts to flare. **0** (default): as it reaches the edge. **Positive**: earlier, while it's still inside the frame. **Negative**: later, only once it's past the edge |
| Reach (%) | How far beyond the edge a light keeps flaring; it fades out over this distance (% of frame height) |
| Depth (%) | How far the flare reaches into the frame |
| Spread (%) / Fan | Width of the flare along the edge (beam and blade width), and how much wider it gets as it goes in (plume and blade; a beam never widens) |
| Edge Glow | A thin glow right along the gate edge |
| Source Colour / Tint | With Flare Colour = Source: take the lights' colour (1) or go white (0). Tint colours every mode |
| Exit Tint | The colour the flare shifts toward as the light goes further out of frame, as it dies (white = none). Side Blast goes green-teal, like Gummo |
| Far Tint | The colour the flare shifts toward as it goes into the frame (white = none). Side Blast goes from cyan-white at the neck to deep blue, the way red drops out first in the reference |
| Falloff | 0: the flare fades gradually from the edge. 1: it holds its strength, then drops away at about Depth (like the Gummo side flare) |
| Veil | Veiling glare: a wide, low haze around the flare that lifts the blacks |
| Iridescence / Rainbow Bands | Gentle rainbow banding like thin-film or prism colours: same brightness, shifted hue (the top of the slider is a half-strength shift) |
| Rainbow Direction | 0: bands across the depth. 1: bands running along the flare, parallel to it (the middle staying white) |
| Prism Edges | 1: the rainbow only on the rays' edges, as thin spectral lines |
| Streaks | Striations from the gate edge's nicks and roughness, radiating from the light and softening as they go in. 0 = a smooth glow |
| Streak Detail | Broad (0) to fine (1) streaks |
| Shimmer / Shimmer Speed | How much the flare changes from frame to frame: the streaks re-form, its strength flickers, and (Pinch) its direction, length and width move, like light catching a rough edge. 0 = still |
| Core | A hot, tight core right at the edge with a long subtle tail (1), versus one even falloff (0) |
| Fringe | Colour fringing: red scatters a little wider than blue |

The shimmer depends only on the frame number, so a frame always renders
the same way.

### Sources, Edges, Composite & Input

| Control | What it does |
| --- | --- |
| Source Threshold / Softness | How bright (as encoded) something must be to flare (default 0.9) |
| Isolate Lights | 1 (default): only small things much brighter than their surroundings flare, such as lamps, practicals and speculars. Big bright areas like lit walls don't. 0: anything over the threshold flares, windows and walls included |
| Light Size (%) | With Isolate Lights: the biggest thing that still counts as a light. Raise it for big practicals or windows |
| Source Mask | Limits which lights can flare. **Ellipse** / **Rectangle**: only lights inside the shape, set with Mask X / Y (centre, % of the whole picture from the left / top), Mask Width / Height and Mask Softness; Show Gate & Sources draws it in green and dims everything outside; keyframe it to follow a light. **Clip Alpha**: only where the incoming image's alpha is on. **Invert Mask**: everything but |
| Edges | Which edges flare |
| Composite | **Film** (default): the flare is extra exposure on the negative, and like film, the brighter it gets the whiter it gets (the dye layers bleed into each other) rather than staying a saturated colour. It's added in odds space and brought back through a soft shoulder, so it glows into the highlights and never flattens into clipped white. **Screen**: brighter, but strong flares hit white. **Add**: plain linear add, for log or scene-linear footage that goes through an output transform afterwards |
| Input Encoding | How the images are encoded, so the flare is added in linear light. Same list as FilmFlash: Rec.709, DaVinci Intermediate, Cineon, camera logs (LogC3/4, S-Log3, V-Log, Log3G10, Apple Log, BMD Film Gen 5) and HLG (iPhone footage) |
| Seed | A different streak pattern |

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
