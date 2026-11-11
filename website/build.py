#!/usr/bin/env python3
"""Builds dpwillsheppard.com as plain static HTML.

Edit the WORK list below, then run:  python3 build.py
Add --no-blog to skip refreshing the blog page from Tumblr (works offline).

Every page is written as <slug>/index.html so the old URLs
(dpwillsheppard.com/aura, /tower, ...) keep working.
"""
import html
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
TUMBLR = "https://hitmykushandchoke.tumblr.com/"

# Order here is the order on the work page (left-to-right, top-to-bottom)
# and the order the NEXT links walk through.
# video: ("vimeo", id) or ("youtube", id) or None. slug None = no page yet.
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
    dict(slug=None, img="parttime.jpg", title="Part Time",
         credit="Dir. Dylan Postage & Paul Villenave", video=None),
    dict(slug="greenpools", img="greenpools.jpg", title="Green Pools",
         credit="Dir. Farhiya Ahmed", video=("vimeo", "846627006")),
    dict(slug="manmaytell", img="manmaytell.jpg", title="Man May Tell",
         credit="Dir. Josiah Dyck", video=("vimeo", "513956830")),
]

e = html.escape


def sidebar(up, current):
    """Right-hand column on every page: name, the work list, then links."""
    work = []
    for w in WORK:
        if w["slug"]:
            here = ' class="here"' if w["slug"] == current else ""
            work.append(f'<li><a href="{up}{w["slug"]}/"{here}>{e(w["title"])}</a></li>')
        else:
            work.append(f'<li><span title="coming soon">{e(w["title"])}</span></li>')
    blog_here = ' class="here"' if current == "blog" else ""
    out = '<span class="ext">[&rarr;]</span>'
    return f"""<aside class="side">
<div>
<a class="name" href="{up or './'}">{e(NAME.upper())}</a>
<span class="role">{e(ROLE)}</span>
<ul class="work">
{chr(10).join(work)}
</ul>
</div>
<div>
<ul class="links">
<li><a href="{up}blog/"{blog_here}>Blog</a></li>
<li><a href="{INSTAGRAM}">Instagram {out}</a></li>
<li><a href="mailto:{EMAIL}">Email {out}</a></li>
<li><a href="tel:{PHONE}">Phone {out}</a></li>
</ul>
<p class="copy">&copy; {datetime.now().year} {e(NAME)}</p>
</div>
</aside>"""


def page(title, body, depth=0, current="", cls=""):
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
<link rel="stylesheet" href="{up}style.css">
</head>
<body{f' class="{cls}"' if cls else ''}>
<div class="layout">
<div class="content">
{body}
</div>
{sidebar(up, current)}
</div>
</body>
</html>
"""


def caption(w):
    return f"<i>{e(w['title'])}</i> | {e(w['credit'])}"


def build_index():
    tiles = []
    for w in WORK:
        img = f'<img src="img/{w["img"]}" alt="{e(w["title"])}" loading="lazy">'
        if w["slug"]:
            tiles.append(f'<li><a href="{w["slug"]}/">{img}<span>{caption(w)}</span></a></li>')
        else:
            tiles.append(f"<li>{img}<span>{caption(w)}</span></li>")
    body = f"""<main>
<ul class="grid">
{chr(10).join(tiles)}
</ul>
</main>"""
    (ROOT / "index.html").write_text(page("", body))


def embed(video):
    kind, vid = video
    if kind == "vimeo":
        src = f"https://player.vimeo.com/video/{vid}?dnt=1"
    else:
        src = f"https://www.youtube-nocookie.com/embed/{vid}"
    return (f'<div class="video"><iframe src="{src}" allow="fullscreen; '
            f'picture-in-picture" allowfullscreen loading="lazy" '
            f'title="video"></iframe></div>')


def build_projects():
    linked = [w for w in WORK if w["slug"]]
    for i, w in enumerate(linked):
        nxt = linked[(i + 1) % len(linked)]
        title = w.get("page_title", w["title"])
        credit = w.get("page_credit", w["credit"])
        parts = [f"<h2><i>{e(title)}</i> | {e(credit)}</h2>"]
        if w["video"]:
            parts.append(embed(w["video"]))
        stills = w.get("stills") or ([] if w["video"] else [w["img"]])
        if stills:
            parts.append('<ul class="stills">' + "".join(
                f'<li><a href="../img/{s}"><img src="../img/{s}" alt="" loading="lazy"></a></li>'
                for s in stills) + "</ul>")
        parts.append(f'<p class="next"><a href="../{nxt["slug"]}/">NEXT &gt;</a></p>')
        body = "<main class=\"project\">\n" + "\n".join(parts) + "\n</main>"
        out = ROOT / w["slug"]
        out.mkdir(exist_ok=True)
        (out / "index.html").write_text(page(title, body, depth=1, current=w["slug"]))


def fetch_tumblr(limit=20):
    req = urllib.request.Request(TUMBLR + "rss", headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=30) as r:
        root = ET.fromstring(r.read())
    posts = []
    for item in root.iter("item"):
        desc = item.findtext("description") or ""
        imgs = re.findall(r'<img src="([^"]+)"', desc)
        if not imgs:
            continue
        text = re.sub(r"<[^>]+>", " ", re.sub(r"<figure.*?</figure>", "", desc, flags=re.S))
        text = " ".join(html.unescape(text).split())
        date = item.findtext("pubDate")
        posts.append(dict(
            img=imgs[0],
            link=item.findtext("link"), text=text,
            date=parsedate_to_datetime(date).strftime("%b %-d, %Y") if date else "",
            tags=[c.text for c in item.findall("category") if c.text],
        ))
        if len(posts) >= limit:
            break
    return posts


def build_blog():
    posts = fetch_tumblr()
    items = []
    for p in posts:
        meta = [f'<a href="{e(p["link"])}">{e(p["date"])}</a>']
        meta += [f"#{e(t)}" for t in p["tags"]]
        items.append(
            f'<li><a href="{e(p["link"])}"><img src="{e(p["img"])}" alt="" loading="lazy"></a>'
            + (f'<p>{e(p["text"])}</p>' if p["text"] else "")
            + f'<p class="meta">{" &middot; ".join(meta)}</p></li>')
    body = f"""<header>
<h1><a href="{TUMBLR}">hit my kush &amp; choke</a></h1>
<p><i>Original Artwork by {e(NAME)}</i></p>
</header>
<main>
<ul class="feed">
{chr(10).join(items)}
</ul>
<p class="more"><a href="{TUMBLR}">more on tumblr &gt;</a></p>
<p class="stamp">updated {datetime.now():%b %-d, %Y}</p>
</main>"""
    (ROOT / "blog").mkdir(exist_ok=True)
    (ROOT / "blog" / "index.html").write_text(
        page("blog", body, depth=1, current="blog", cls="plain"))
    print(f"blog: {len(items)} posts")


if __name__ == "__main__":
    build_index()
    build_projects()
    if "--no-blog" not in sys.argv:
        build_blog()
    print("built", ROOT)
