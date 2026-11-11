# Lumpy

A workspace for Claude Code sessions outside the CTD agent.

## DaVinci Resolve OFX plugins

| Plugin | Folder | What it does |
| --- | --- | --- |
| EriksTV | `datamosh-ofx/` | Datamosh: deleted I-frames, repeated P-frames, block, time and colour glitches |
| FilmFlash | `filmflash-ofx/` | A film camera's run-out / run-up exposure flash (transition, or effect with speed-up and blur), with negative-film highlight roll-off |
| GateFlare | `gateflare-ofx/` | Film-gate edge flare: bright sources just outside the frame flare in from the edge as they enter or leave the shot |

Shared: `third_party/openfx/` (OpenFX headers), `shared/Encodings.h`
(colour encodings / camera log curves), `ofx-testhost/` (the test host the
plugins' tests use) and `scripts/install-macos.sh`.

The **OFX plugins** GitHub Actions workflow builds both for macOS
(universal) and uploads `EriksTV-macOS`, `FilmFlash-macOS` and
`GateFlare-macOS`. Unzip one,
run `bash install-macos.sh` and restart Resolve.
