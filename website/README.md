# dpwillsheppard.com

Plain static site: HTML + one stylesheet, no JavaScript, no framework.
Upload this whole folder to any static host (Netlify, GitHub Pages, Hostinger file manager, etc.).

- `build.py` holds the content (the `WORK` list, contact links) and writes every page.
- `python3 build.py` rebuilds everything and pulls the latest ~20 photos from the tumblr into `blog/`.
- `python3 build.py --no-blog` rebuilds without touching the blog (works offline).
- Add a project: drop a still in `img/`, add an entry to `WORK`, run the build.
- Each project is `<slug>/index.html`, so the old URLs (`/aura`, `/tower`, …) still work.
