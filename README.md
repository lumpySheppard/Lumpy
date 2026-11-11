# Lumpy

A workspace for Claude Code sessions outside the CTD agent.

## DaVinci Resolve OFX plugins

| Plugin | Folder | What it does |
| --- | --- | --- |
| EriksTV | `datamosh-ofx/` | Datamosh: deleted I-frames, repeated P-frames, block, time and colour glitches |
| FilmFlash | `filmflash-ofx/` | Transition: a film camera's run-out / run-up exposure flash, with negative-film highlight roll-off |

Shared: `third_party/openfx/` (OpenFX headers), `ofx-testhost/` (the
test host both plugins' tests use) and `scripts/install-macos.sh`.

The **OFX plugins** GitHub Actions workflow builds both for macOS
(universal) and uploads `EriksTV-macOS` and `FilmFlash-macOS`. Unzip one,
run `bash install-macos.sh` and restart Resolve.
