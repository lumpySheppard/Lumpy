#!/usr/bin/env python3
"""Builds dpwillsheppard.com as plain static HTML.

Edit the WORK list below, then run:  python3 build.py
That also pulls any new photos from the tumblr into img/photo/ and
photos.json. Add --offline to rebuild from what is already saved.

Every page is written as <slug>/index.html so the old URLs
(dpwillsheppard.com/aura, /tower, ...) keep working.
"""
import html
import json
import re
import sys
import urllib.request
import xml.etree.ElementTree as ET
from datetime import datetime
from email.utils import parsedate_to_datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parent

NAME = "Will Sheppard"
ROLE = "Director of Photography"
BLURB = ("Canadian cinematographer. Based out of Toronto and Vancouver "
         "specializing in narrative and documentary work.")
INSTAGRAM = "https://www.instagram.com/will_sheppard/"
EMAIL = "willsheppard99@gmail.com"
PHONE = "6046535556"
TUMBLR = "https://hitmykushandchoke.tumblr.com/"  # source for /photo only
ABOUT = [
    BLURB,
    'Co-founder of Blue Sheep, an independent production studio in Toronto: '
    '<a href="https://bluesheepfilms.com">bluesheepfilms.com</a> [&rarr;]',
]

# Order here is the order on the work page (left-to-right, top-to-bottom)
# and the order the NEXT links walk through.
# video: ("vimeo", id) or ("youtube", id) or None (then the still is shown).
# notes: optional list of paragraphs shown under the video, e.g.
#   notes=["Really proud of this one.", "We ran a lean crew ..."]
WORK = [
    dict(slug="bownownow", img="bownownow.jpg", title="Bow Now Now",
         credit="The Thick", video=("youtube", "FGugLiK9gQA")),
    dict(slug="aura", img="split.jpg", title="SPLIT",
         credit="Dir. Will Sheppard", page_title="AURA", video=None,
         stills=[f"aura/{i}.jpg" for i in range(1, 9)]),
    dict(slug="tower", img="tower.jpg", title="Tower Motorsport",
         credit="4-part series", page_credit="Four Part Documentary",
         video=("vimeo", "1025289003")),
    dict(slug="craneslikewhitegiraffes", img="cranes.jpg",
         title="Cranes Like White Giraffes", credit="Dir. Josiah Dyck",
         video=("vimeo", "912083144")),
    dict(slug="simplertimes", img="simplertimes.jpg",
         title="Simpler Times Feat. Gritfall", credit="Evo da Saint & Felix Fox",
         video=("youtube", "fQmIkUqNnqc")),
    dict(slug="lostpuppets", img="lostpuppets.jpg", title="Lost Puppets",
         credit="Jae Skeese", video=("youtube", "DlEm4bLixew")),
    dict(slug="parttime", img="parttime.jpg", title="Part Time",
         credit="Dir. Dylan Postage & Paul Villenave", video=None),
    dict(slug="greenpools", img="greenpools.jpg", title="Green Pools",
         credit="Dir. Farhiya Ahmed", video=("vimeo", "846627006")),
    dict(slug="manmaytell", img="manmaytell.jpg", title="Man May Tell",
         credit="Dir. Josiah Dyck", video=("vimeo", "513956830")),
]

e = html.escape


THEME_HEAD = ("<script>try{var t=localStorage.getItem('theme');"
              "if(t)document.documentElement.dataset.theme=t}catch(e){}</script>")
THEME_TOGGLE = """<p class="theme">
<button type="button" data-set="black">black</button> /
<button type="button" data-set="white">white</button>
</p>
<script>
document.querySelectorAll('.theme button').forEach(function (b) {
  b.onclick = function () {
    document.documentElement.dataset.theme = b.dataset.set;
    try { localStorage.setItem('theme', b.dataset.set); } catch (e) {}
  };
});
</script>"""


def sidebar(up, current):
    """Right-hand column on every page: name, the four sections, theme toggle."""
    def link(href, label, key):
        here = ' class="here"' if key == current else ""
        return f'<li><a href="{href}"{here}>{label}</a></li>'

    return f"""<aside class="side">
<div>
<a class="name" href="{up or './'}">{e(NAME.upper())}</a>
<span class="role">{e(ROLE)}</span>
<ul class="sections">
{link(up or "./", "Work", "work")}
{link(f"{up}about/", "About", "about")}
{link(f"{up}contact/", "Contact", "contact")}
</ul>
</div>
<div>
{THEME_TOGGLE}
<p class="copy">&copy; {datetime.now().year} {e(NAME)}</p>
</div>
</aside>"""


def page(title, body, depth=0, current=""):
    up = "../" * depth
    full_title = f"{title} | {NAME}" if title else f"{NAME} | {ROLE}"
    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{e(full_title)}</title>
<meta name="description" content="{e(BLURB)}">
<meta property="og:title" content="{e(full_title)}">
<meta property="og:description" content="{e(BLURB)}">
{THEME_HEAD}
<link rel="stylesheet" href="{up}style.css">
</head>
<body>
<div class="layout">
<div class="content">
{body}
</div>
{sidebar(up, current)}
</div>
</body>
</html>
"""


def build_index():
    tiles = "\n".join(
        f'<li><a href="{w["slug"]}/">'
        f'<img src="img/{w["img"]}" alt="" loading="lazy">'
        f'<span class="label">{e(w["title"])}</span></a></li>'
        for w in WORK)
    body = f"""<main>
<ul class="grid">
{tiles}
</ul>
</main>"""
    (ROOT / "index.html").write_text(page("", body, current="work"))


def embed(video):
    kind, vid = video
    if kind == "vimeo":
        src = f"https://player.vimeo.com/video/{vid}?dnt=1"
    else:
        src = f"https://www.youtube-nocookie.com/embed/{vid}"
    return (f'<div class="video"><iframe src="{src}" allow="fullscreen; '
            f'picture-in-picture" allowfullscreen '
            f'title="video"></iframe></div>')


def build_projects():
    for i, w in enumerate(WORK):
        nxt = WORK[(i + 1) % len(WORK)]
        title = w.get("page_title", w["title"])
        credit = w.get("page_credit", w["credit"])
        parts = [f"<h1>{e(title)}</h1>"]
        if w["video"]:
            parts.append(embed(w["video"]))
        else:
            parts.append(f'<img class="hero" src="../img/{w["img"]}" alt="{e(title)}">')
        parts.append(f'<p class="credit">{e(credit)}</p>')
        parts += [f"<p>{e(n)}</p>" for n in w.get("notes", [])]
        if w.get("stills"):
            parts.append('<ul class="stills">' + "".join(
                f'<li><a href="../img/{s}"><img src="../img/{s}" alt="" loading="lazy"></a></li>'
                for s in w["stills"]) + "</ul>")
        parts.append(f'<p class="next"><a href="../">&lt; all work</a>'
                     f'<a href="../{nxt["slug"]}/">next &gt;</a></p>')
        body = '<main class="project">\n' + "\n".join(parts) + "\n</main>"
        out = ROOT / w["slug"]
        out.mkdir(exist_ok=True)
        (out / "index.html").write_text(page(title, body, depth=1, current="work"))


PHOTOS = ROOT / "photos.json"
PHOTO_DIR = ROOT / "img" / "photo"


def fetch_photos(limit=40):
    """Pull new photo posts from the tumblr RSS and save the images locally."""
    saved = json.loads(PHOTOS.read_text()) if PHOTOS.exists() else []
    have = {p["id"] for p in saved}
    req = urllib.request.Request(TUMBLR + "rss", headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=30) as r:
        root = ET.fromstring(r.read())
    PHOTO_DIR.mkdir(parents=True, exist_ok=True)
    new = []
    for item in root.iter("item"):
        pid = (item.findtext("link") or "").rstrip("/").split("/")[-1]
        desc = item.findtext("description") or ""
        # the 1280px rendition from the srcset, else the plain src
        m = (re.search(r'(https://\S+/s1280x1920/\S+?\.(?:jpg|jpeg|png|gif))', desc)
             or re.search(r'<img src="([^"]+)"', desc))
        if not pid or not m or pid in have:
            continue
        url = m.group(1)
        name = f"{pid}{Path(url).suffix}"
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=60) as r:
            (PHOTO_DIR / name).write_bytes(r.read())
        text = re.sub(r"<[^>]+>", " ", re.sub(r"<figure.*?</figure>", "", desc, flags=re.S))
        date = item.findtext("pubDate")
        new.append(dict(
            id=pid, file=name, caption=" ".join(html.unescape(text).split()),
            date=parsedate_to_datetime(date).strftime("%Y-%m-%d") if date else ""))
    photos = sorted(saved + new, key=lambda p: p["date"], reverse=True)[:limit]
    keep = {p["file"] for p in photos}
    for f in PHOTO_DIR.iterdir():
        if f.name not in keep:
            f.unlink()
    PHOTOS.write_text(json.dumps(photos, indent=1) + "\n")
    print(f"photo: {len(new)} new, {len(photos)} total")


def build_photo():
    photos = json.loads(PHOTOS.read_text()) if PHOTOS.exists() else []
    items = []
    for p in photos:
        src = f"../img/photo/{p['file']}"
        items.append(
            f'<li><a href="{src}"><img src="{src}" alt="" loading="lazy"></a>'
            + (f'<p>{e(p["caption"])}</p>' if p["caption"] else "") + "</li>")
    body = f"""<main class="photos">
<p class="back"><a href="../about/">&lt; about</a></p>
<ul class="feed">
{chr(10).join(items)}
</ul>
</main>"""
    (ROOT / "photo").mkdir(exist_ok=True)
    (ROOT / "photo" / "index.html").write_text(
        page("Photo", body, depth=1, current="about"))


def build_about():
    paras = "\n".join(f"<p>{t}</p>" for t in ABOUT)
    body = f"""<main class="text">
{paras}
<p class="more"><a href="../photo/">photo &gt;</a></p>
</main>"""
    (ROOT / "about").mkdir(exist_ok=True)
    (ROOT / "about" / "index.html").write_text(page("About", body, depth=1, current="about"))


def build_contact():
    tel = f"{PHONE[:3]}-{PHONE[3:6]}-{PHONE[6:]}"
    body = f"""<main class="text">
<ul class="contact">
<li>Email: <a href="mailto:{EMAIL}">{EMAIL}</a></li>
<li>Phone: <a href="tel:{PHONE}">{tel}</a></li>
<li>Instagram: <a href="{INSTAGRAM}">@will_sheppard</a> [&rarr;]</li>
</ul>
</main>"""
    (ROOT / "contact").mkdir(exist_ok=True)
    (ROOT / "contact" / "index.html").write_text(page("Contact", body, depth=1, current="contact"))


if __name__ == "__main__":
    if "--offline" not in sys.argv:
        fetch_photos()
    build_index()
    build_projects()
    build_photo()
    build_about()
    build_contact()
    print("built", ROOT)
