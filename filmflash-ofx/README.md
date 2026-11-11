# FilmFlash (OpenFX transition for DaVinci Resolve)

The exposure flash of a film camera running down to a stop and back up to
speed, as a transition you drop on a cut. It also works as a plain effect.

## What it imitates

A film camera's exposure time per frame is shutter angle ÷ (360 × fps). As
the motor slows at the end of a take, each frame gets more light than the
last. The extra exposure in stops is log₂(normal fps ÷ current fps): it
creeps up, then shoots off as the camera stops. On the next take the
camera gets back up to speed and the flash clears.

That light lands on a negative, not a sensor, so nothing clips:

- **Shoulder.** Each extra stop halves the distance left to white.
  Highlights slide into a creamy white and keep their detail longer than
  you'd expect, while shadows gain a true stop each time.
- **Colour.** Saturation drains as the layers converge. The layers differ
  in speed, so the flash drifts warm (Colour Shift).
- **Stray light.** It fogs the base, giving a milky, orange lift in the
  shadows (Base Fog). It also halates, a red glow around bright areas
  (Halation), and can leak in at one edge (Edge Leak).
- **Speed.** At 1/k of normal speed the camera records 1/k as many frames
  per real second, so played back the action runs k times fast. The
  outgoing shot accelerates into the cut and the incoming shot starts fast
  and settles (Speed Up).
- **Motion blur.** The shutter stays open k times longer in real time, so
  each frame smears k times as much motion. The plugin averages the real
  source frames across that exposure window, in linear light (Motion Blur).
- **Unsteady camera.** The shutter at the wrong speed lights one side more
  than the other (Uneven Exposure), and an unsteady motor flickers
  (Flicker).

The model works in "odds" space, light ÷ (1 − light). Exposure multiplies
it and fog and halation add to it. Converting back can approach 1.0 but
never reach it. At 0 stops the picture is returned exactly as it came in.

## Install (macOS)

1. Get the `FilmFlash-macOS` artifact from the latest **OFX plugins** run
   in GitHub Actions and unzip it.
2. Run `bash install-macos.sh` in that folder, then restart Resolve.

It's a universal binary for macOS 11 or later.

## Use

**As a transition:**
1. Find it in **Effects → OpenFX → Transitions** (Resolve lists OFX
   transitions with the other transitions; look for **FilmFlash**).
2. Drag it onto a cut.
3. Make the transition as long as you want the whole run-out/run-up to
   last. 1–2 seconds feels right. It needs handles (extra frames) on both
   clips, like any transition.

The outgoing shot builds to the flash, the cut happens at the peak (where
it's hidden), and the incoming shot clears as the camera gets back up to
speed.

**As an effect:**
1. Drop it on a clip like any other effect.
2. Keyframe **Flash (stops)**.

| Control | What it does |
| --- | --- |
| Peak (stops) | How far over the flash goes at the cut. 6–8 burns nearly to white |
| Cut Point | Where in the transition the shots change over (and the flash peaks) |
| Peak Hold | Share of the transition held at the peak |
| Run-out Shape | 1 = a motor slowing evenly (creeps up, then shoots off). Higher holds off longer, then rushes |
| Run-up Shape | Higher = the camera gets back up to speed faster, so the flash clears sooner |
| Cut Softness | 0 = a hard cut at the peak. Higher = a short dissolve across it |
| Flash (stops) | Effect only: the flash amount to keyframe |
| Flicker / Seed | Random frame-to-frame exposure wobble while flashing |

**Camera Speed**

| Control | What it does |
| --- | --- |
| Speed Up | Transition only. How much the action speeds up as the camera slows: 0 = off |
| Speed Curve | **Even Ramp** (default): the action speeds up steadily into the cut and settles steadily after it, where you can see it. **Follows Exposure**: exactly like a real camera (speed = 2^stops), where most of the speed-up happens near the peak, inside the flash |
| Max Speed | How fast the action runs at the cut |
| Spare Handle Frames | Handle frames each clip has *beyond what the transition itself uses*. The speed-up is scaled so it never reads further than this. More handles = a stronger speed-up |
| Motion Blur | Extra blur from the longer exposures: 1 = tied to the exposure, 0 = off |
| Shutter Angle | The camera's shutter at normal speed (180° is standard); a wider shutter smears more |
| Max Blur | Cap on the blur, in frames of action. Higher costs render time (one source frame per frame of blur) |
| Show Diagnostics | Prints the timing on the picture (see below) |

How speed-up works:

- **Outgoing shot.** The action plays normally at the start of the
  transition, then runs faster and faster into the cut. Running fast uses
  up footage, so it reads into the clip's tail past the edit point.
- **Incoming shot.** It's the mirror image: it starts fast at the cut and
  settles to normal by the end. That means starting further back, in the
  clip's head before the edit point.
- **No jumps.** Both are pinned at the transition's outer ends, so nothing
  jumps where the transition starts or ends.
- **Handles.** How much speed-up you get depends on how much footage
  there is past the edit. Spare Handle Frames tells the plugin how much it
  may use. A 1-second transition with 12 spare frames on each side gives
  roughly a 2× average speed-up into the cut.

**Show Diagnostics** prints, on the frame:
- where the transition is, and how the plugin found its frame step (from
  the Transition value, or from the effect's frame range)
- how far ahead and behind each shot is reading
- how much handle the speed-up wanted and how much it was scaled to fit
- whether Resolve supplied those frames. **HOST GAVE THE CURRENT FRAME**
  means Resolve isn't letting the transition read other moments of the
  clips. Speed-up and blur can't work then, and the plugin falls back to
  the current frame.

Both only add to what the footage already has. With no flash, there's no
extra speed and no extra blur.

**Film**

| Control | What it does |
| --- | --- |
| Input Encoding | How the images are encoded, so stops are real stops. Rec.709 / Gamma 2.4 for a standard timeline; DaVinci Intermediate for a DaVinci Wide Gamut timeline |
| Film Response | How quickly each stop eats into the picture |
| Colour Shift | Warm drift as the flash builds |
| Highlight Colour | The white the flash burns toward |
| Base Fog / Fog Colour | Milky, coloured lift of the shadows |
| Halation / Halation Size | Red glow around bright areas (size as % of frame width) |
| Scene Ceiling | Scene-referred encodings only: where the flash tops out, in stops above mid grey |

**Camera & Light**

| Control | What it does |
| --- | --- |
| Uneven Exposure | Extra stops on one side of the frame |
| Direction | Which side: 0° left, 90° top, 180° right, 270° bottom |
| Edge Leak / Leak Colour | Light leaking in on that side |

Every control except Input Encoding and Seed can be keyframed.

## Build

```
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

The tests load the built plugin in `../ofx-testhost/minihost.cpp`, as a
transition and as an effect. They check:

- both ends of the transition, and Flash 0, return the source untouched
- the cut is near white
- speed-up and motion blur actually read other frames, but never beyond
  the clips' handles (Spare Handle Frames). This includes hosts that don't
  animate the Transition value. Turning them off reads only the current
  frame
- nothing ever passes white, even at 18 stops with everything turned up
- frames rendered out of order match frames rendered in order

`tools/flash_cli` previews the flash on raw video piped through ffmpeg:

```
ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
  build/flash_cli 1920 1080 --cut 48 --length 36 --peak 8 --leak 0.5 |
  ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - -pix_fmt yuv420p out.mp4
```
