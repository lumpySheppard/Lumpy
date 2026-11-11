# dpwillsheppard.com

Plain static site: HTML + one stylesheet, no framework (a few lines of JavaScript for the black / white toggle only).
Upload this whole folder to any static host (Netlify, GitHub Pages, etc.).

Pages: Work (`/`, a 3-column grid of thumbnails, one page per project), Photography, About, Contact.
The menu on the right of every page links them all and has a black / white toggle (remembered per visitor).

- `build.py` holds the content (the `WORK` list, `ABOUT` text, contact details) and writes every page.
  Give a project `notes=[...]` to put paragraphs under its video.
- `python3 build.py` rebuilds everything. It also saves any new photos from the tumblr into
  `img/photo/` and `photos.json` (newest 40 kept), so the Photography page never links out to tumblr.
- `python3 build.py --offline` rebuilds from what is already saved.
- Add a project: drop a still in `img/`, add an entry to `WORK`, run the build.
- Each project is `<slug>/index.html`, so the old URLs (`/aura`, `/tower`, …) still work.
