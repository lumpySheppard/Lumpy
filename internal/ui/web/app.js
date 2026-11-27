"use strict";

// ---------- basics ----------
const token = new URLSearchParams(location.hash.slice(1)).get("t") || sessionStorageGet("lumpy-token") || "";
sessionStorageSet("lumpy-token", token);
const native = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.lumpy;
// Space-only mode: Lumpy for iPad shows the Space tab and nothing else.
const spaceOnly = new URLSearchParams(location.hash.slice(1)).get("mode") === "space";
if (spaceOnly) document.documentElement.classList.add("space-only");
if (native || new URLSearchParams(location.search).has("native")) document.documentElement.classList.add("native");

function sessionStorageGet(k) { try { return sessionStorage.getItem(k); } catch { return null; } }
function sessionStorageSet(k, v) { try { sessionStorage.setItem(k, v); } catch {} }
function localGet(k, d) { try { return localStorage.getItem(k) ?? d; } catch { return d; } }
function localSet(k, v) { try { localStorage.setItem(k, v); } catch {} }

const $ = (s, el = document) => el.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

async function api(path, opts = {}) {
  const res = await fetch(path, {
    ...opts,
    headers: { "X-Lumpy-Token": token, ...(opts.body ? { "Content-Type": "application/json" } : {}) },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function bytes(n) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB", "PB"];
  let i = 0;
  while (n >= 1000 && i < u.length - 1) { n /= 1000; i++; }
  return (i === 0 ? n : n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1).replace(/\.0$/, "") : n.toFixed(2).replace(/\.?0+$/, "")) + " " + u[i];
}
function when(ts) {
  if (!ts) return "never";
  const d = new Date(ts * 1000);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }) + " at " +
    d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}
function short(ts) {
  if (!ts) return "—";
  const d = new Date(ts * 1000);
  return d.toLocaleDateString(undefined, { year: "numeric", month: "2-digit", day: "2-digit" }) + " " +
    d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}
function duration(s) {
  if (!isFinite(s) || s <= 0) return "";
  s = Math.round(s);
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  return h ? `${h}h ${m}m` : m ? `${m}m ${sec}s` : `${sec}s`;
}

let toastTimer;
function toast(msg, bad) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (bad ? " bad" : "");
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), bad ? 6000 : 3200);
}

// ---------- icons ----------
const ICON = {
  drive: '<svg viewBox="0 0 30 22" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6.5 3h17l4.5 9.5V18a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-5.5L6.5 3z"/><path d="M2 12.5h26"/><circle cx="23" cy="16.3" r="1" fill="currentColor" stroke="none"/></svg>',
  edit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 20h4L19 9l-4-4L4 16v4z"/></svg>',
  person: '<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="8" r="4"/><path d="M4 21c0-4.4 3.6-7 8-7s8 2.6 8 7z"/></svg>',
  mail: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 5h18a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zm9 7.5L4 7.3V8l8 5.2L20 8v-.7z"/></svg>',
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/></svg>',
  export: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M9 4v16"/></svg>',
  down: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 4v16M6 14l6 6 6-6"/></svg>',
  up: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 20V4M6 10l6-6 6 6"/></svg>',
  bell: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9a6 6 0 0 1 12 0c0 5 2 6.5 2 6.5H4S6 14 6 9z"/><path d="M10 20a2 2 0 0 0 4 0"/></svg>',
  history: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12a9 9 0 1 0 3-6.7M3 4v5h5M12 7v5l3 3"/></svg>',
  eject: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M5 15l7-9 7 9z"/><path d="M5 19h14"/></svg>',
  plug: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0V8zM12 18v4"/></svg>',
  info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.5"/></svg>',
  gauge: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 12l4-4"/></svg>',
  pie: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M11 3a9 9 0 1 0 9.95 10H11z"/><path d="M13 2v9h9a9 9 0 0 0-9-9z" opacity=".55"/></svg>',
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z"/><path d="M8.5 12l2.5 2.5 4.5-5"/></svg>',
  tag: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12V4h8l10 10-8 8L3 12z"/><circle cx="7.5" cy="8.5" r="1.3" fill="currentColor"/></svg>',
  folder: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 6a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6z"/></svg>',
  file: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/></svg>',
  chev: '<svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M9 6l6 6-6 6"/></svg>',
  check: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.8"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>',
  warn: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2.5L1.5 21h21L12 2.5zm-1 7h2v6h-2zm0 8h2v2h-2z"/></svg>',
  x: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  clock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>',
  dash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><circle cx="12" cy="12" r="8" stroke-dasharray="3 3"/></svg>',
  camera: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M4 8h3l2-3h6l2 3h3v11H4z"/><circle cx="12" cy="13" r="3.5"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/></svg>',
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M12 5v14M5 12h14"/></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/></svg>',
  activity: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h4l3-8 4 16 3-8h4"/></svg>',
  film: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4"/></svg>',
  back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M15 5l-7 7 7 7"/></svg>',
  arrow: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 12h16M13 5l7 7-7 7"/></svg>',
  play: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M7 4l13 8-13 8z"/></svg>',
  right: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5l7 7-7 7"/></svg>',
  eye: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12z"/><circle cx="12" cy="12" r="3"/></svg>',
  gear: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',
  globe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>',
  clapper: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9h18v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9zM3 9l1.5-5L20 6.5 18.5 9M8 4.5L10 8M13 5l2 3.5"/></svg>',
  reveal: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/></svg>',
};
const COLORS = { purple: "#a855f7", blue: "#3b82f6", green: "#34c759", yellow: "#f5b400", orange: "#f08a24", red: "#ff453a", pink: "#ec4899", teal: "#14b8a6", gray: "#8e8e93" };

// ---------- native bridge (Lumpy.app) ----------
const pending = new Map();
let nextReq = 1;
window.lumpyNative = {
  resolve(id, value) { const p = pending.get(id); if (p) { pending.delete(id); p(value); } },
};
function nativeCall(action, args = {}) {
  return new Promise((resolve) => {
    const id = nextReq++;
    pending.set(id, resolve);
    native.postMessage({ action, id, ...args });
  });
}

// ---------- state ----------
const S = {
  tab: spaceOnly ? "space" : localGet("tab", "map") === "activity" ? "logged" : localGet("tab", "map"),
  view: localGet("view", "grid"),
  mediaView: localGet("mediaView", "table"),
  sort: localGet("sort", "seen"),
  colorBy: localGet("colorBy", "project"),
  desc: localGet("desc", "1") === "1",
  query: "",
  state: null,
  jobs: [],
  fileHits: null,
  detail: null, // id of open drive
  project: null, // id of the open project page
  projectData: null,
  projects: null, // the list with health, fetched on the Projects tab
  selecting: false,
  selected: new Set(),
  offload: { source: "", dests: [""], name: "", sourceCheck: true, proxies: localGet("offload.proxies", "0") === "1", proxyScale: localGet("offload.proxyScale", "half") },
  media: { project: "", drive: "", kind: "", format: "", codec: "", rate: "", qc: "", q: "" },
  mediaSort: localGet("mediaSort", "name"),
  mediaDesc: localGet("mediaDesc", "0") === "1",
  mediaData: null,
  file: null, // { driveId, path, back } for the open file sheet
  settings: null, // publish settings and last result
};

// ---------- data refresh ----------
async function loadState() {
  try {
    const first = !S.state;
    const before = S.state ? S.state.drives.map((d) => d.id + (d.connected ? "+" : "-")).join() : "";
    S.state = await api("/api/state");
    if (first || !S.settings) S.settings = await api("/api/settings").catch(() => null);
    const after = S.state.drives.map((d) => d.id + (d.connected ? "+" : "-")).join();
    if (before !== after && S.onDrives) S.onDrives();
    showOffers();
    // The cards draw at once; their backup badges arrive when the
    // summaries are done, a moment later.
    if (S.state.summarising) { clearTimeout(S.sumTimer); S.sumTimer = setTimeout(loadState, 1500); }
    // The offload form keeps what's being typed; only the counts refresh.
    if (!first && (S.tab === "offload" || S.tab === "media" || S.tab === "space")) { renderCounts(); if (S.tab === "space") spaceTick(); }
    else if (!first && S.tab === "projects") {
      renderCounts();
      // The page is left alone while its master print is in use:
      // playing, parked on a frame, or with its stills open.
      const pl = document.querySelector("#main #player");
      const inUse = pl && pl.dataset.busy === "1";
      if (S.project) { if (!inUse) loadProject(S.project, true); } else loadProjects();
    }
    else if (!first && S.tab === "map") { renderCounts(); loadProjects(); }
    else render();
  } catch (e) {
    if (!S.state) $("#main").innerHTML = `<div class="empty"><h2>Can't reach Lumpy</h2><p>${esc(e.message)}</p></div>`;
  }
}
let lastJobSig = "";
async function loadJobs() {
  try {
    S.jobs = await api("/api/jobs");
    const running = S.jobs.filter((j) => j.state === "running" || j.state === "queued").length;
    $("#count-jobs").textContent = running || "";
    const sig = S.jobs.map((j) => j.id + j.state).join();
    if (sig !== lastJobSig) {
      // A job finished or started: drive numbers may have changed.
      if (lastJobSig) {
        for (const j of S.jobs) {
          const before = lastJobSig.includes(j.id + "running");
          if (before && j.state !== "running") {
            toast(`${j.title}: ${j.state === "ok" ? "done" : j.state === "failed" ? j.failures + " problem(s)" : j.state}`, j.state !== "ok");
          }
        }
        loadState();
        if (S.detail) openDetail(S.detail, true);
        if (S.tab === "media") loadMedia();
      }
      lastJobSig = sig;
    }
    if (S.tab === "offload") $(".offload")?.classList.toggle("busy", running > 0);
    if (S.tab === "offload") renderJobsInto($("#offload-jobs"), S.jobs.filter((j) => j.kind === "offload"), "No offloads yet.");
    if (!$("#activity-pop").hidden) renderJobsInto($("#activity-jobs"), S.jobs, "Nothing has run yet. Offloads, drive checks and rescans show their progress here.");
  } catch {}
}

// ---------- rendering ----------
// backupState sums up how well something is backed up. backed is what
// has a checksum-matched copy on another drive; likely is what has no
// checksum yet but the same name and size on another drive (the quick
// look, before a checksum pass proves it); unknown has no checksum and
// nothing like it elsewhere. "full" means everything is proven, "likely"
// everything is proven or probable, "partial" some of it, "none" nothing,
// "unknown" nothing can be told yet, "empty" no files.
function backupState(backed, total, unknown, likely = 0) {
  if (!total) return "empty";
  if (backed >= total) return "full";
  if (likely > 0 && backed + likely >= total) return "likely";
  if (unknown >= total) return "unknown";
  if (backed <= 0 && likely <= 0) return "none";
  return "partial";
}
const STATE_LABEL = { full: "Verified backup", likely: "Probably backed up · checksum to prove", partial: "", none: "No backup", unknown: "No checksums yet", empty: "Empty" };
const STATE_TITLE = { full: "Every file here has a checksum-matched copy on another drive", likely: "Every file here has either a verified copy or a same-name, same-size file on another drive. Checksum the drive to prove the copies.",
  none: "No file here has a copy on another drive", unknown: "Files have no checksum yet and nothing like them was found elsewhere", partial: "", empty: "" };
// backupBar draws one bar for a backup state: green fill when complete,
// yellow fill for the proven share, hatched yellow for the probable
// share (name and size match), hatched grey for files nothing is known
// about, red label when there is nothing.
function backupBar(backed, total, unknown, likely = 0, opts = {}) {
  if (typeof likely === "object") { opts = likely; likely = 0; }
  const st = backupState(backed, total, unknown, likely);
  const t = total || 1;
  const fill = st === "full" ? "fill-full" : "fill-partial";
  const n = (v) => (opts.unit === "files" ? v.toLocaleString() : bytes(v));
  const pc = (v) => Math.round((v / t) * 100) + "%";
  // Cards get percentages, which stay short however big the project is.
  const label = opts.label !== undefined ? opts.label : st === "partial"
    ? (opts.compact
      ? `${pc(backed)} backed up${likely ? ` · ${pc(likely)} probably` : ""}${unknown ? ` · ${pc(unknown)} unproven` : ""}`
      : `${n(backed)} of ${n(total)}${opts.unit === "files" ? " files" : ""} backed up${likely ? ` · ${n(likely)} probably` : ""}${unknown ? ` · ${n(unknown)} unproven` : ""}`)
    : STATE_LABEL[st];
  return `<div class="bbar s-${st} ${opts.size || ""}" title="${esc(STATE_TITLE[st] || (likely ? "Probably: a file with the same name and size is on another drive; checksum to prove it" : ""))}">
    <div class="track">${backed > 0 ? `<i class="${fill}" style="width:${((backed / t) * 100).toFixed(2)}%"></i>` : ""}${likely > 0 ? `<i class="fill-likely" style="width:${((likely / t) * 100).toFixed(2)}%"></i>` : ""}${unknown > 0 ? `<i class="fill-unknown" style="width:${((unknown / t) * 100).toFixed(2)}%"></i>` : ""}</div>
    <span class="lbl">${st === "full" ? ICON.check : st === "none" ? ICON.warn : ""}${esc(label)}</span></div>`;
}
// capBar shows how full a drive is against a fixed blue→yellow→red
// gradient; the element inside masks the free part.
function capBar(used, size) {
  const pct = size ? Math.max(0, Math.min(100, (used / size) * 100)) : 0;
  if (pct < 0.5) return `<div class="bar cap" title="${size ? "empty" : ""}"></div>`;
  // The gradient covers the full track: stretch it by the inverse of the fill.
  return `<div class="bar cap" title="${Math.round(pct)}% used"><i style="width:${pct.toFixed(1)}%;background-size:${(10000 / pct).toFixed(1)}% 100%"></i></div>`;
}
function lowSpace(d) { return d.size > 0 && d.free < d.size * 0.1; }


// A camera card is safe to format once every file on it is checksummed
// and has at least two verified copies on other drives. One copy
// everywhere is "backed up once": the card is still the second copy.
function isCard(d) { return driveKind(d) === "card" || !!(d.labels && d.labels.noBackups); }
function safeToFormat(d) {
  const b = d.backups;
  if (!b || !d.files || d.damaged || !isCard(d)) return "";
  if (b.unknown || b.likely || b.none) return "";
  if (b.minCopies >= 2) return "safe";
  if (b.minCopies >= 1) return "once";
  return "";
}
// A drive's SMART verdict: "bad" when it says it is failing, "warn"
// with counters worth a look, "ok" with none, "" when never read.
function healthState(d) {
  const h = d.health;
  if (!h || h.error) return "";
  if (h.passed === false) return "bad";
  if (h.warnings && h.warnings.length) return "warn";
  return "ok";
}
function healthBadge(d) {
  const st = healthState(d);
  if (st === "bad") return `<span class="badge bad" title="${esc(d.health.warnings.join(" · "))}">${ICON.warn}Drive failing</span>`;
  // The badge carries the finding; the advice after the colon stays in the tooltip.
  if (st === "warn") return `<span class="badge warn" title="${esc(d.health.warnings.join(" · "))}">${ICON.warn}${esc(d.health.warnings[0].split(":")[0])}${d.health.warnings.length > 1 ? ` +${d.health.warnings.length - 1}` : ""}</span>`;
  return "";
}
function hoursText(h) {
  if (!h) return "—";
  if (h < 48) return `${h} h`;
  const days = Math.round(h / 24);
  if (days < 60) return `${days} days`;
  const months = Math.round(days / 30.4);
  if (months < 24) return `${months} months`;
  return `${(days / 365.25).toFixed(1)} years`;
}
const SMART_HINT = `Lumpy reads the drive's own health report (SMART) through smartmontools: in Terminal, <code>brew install smartmontools</code>. Thunderbolt, SATA and internal drives answer; most USB enclosures don't on macOS.`;
// healthChart draws the kept readings: temperature as a line, bad
// sectors as bars beneath, oldest to newest.
function healthChart(hist) {
  const rs = [...hist].reverse();
  if (rs.length < 2) return "";
  const W = 420, H = 64, n = rs.length, x = (i) => 8 + (i * (W - 16)) / (n - 1);
  const temps = rs.map((r) => r.temp).filter(Boolean), tmin = Math.min(...temps, 20), tmax = Math.max(...temps, 50);
  const ty = (t) => H - 14 - ((t - tmin) / (tmax - tmin || 1)) * (H - 28);
  const bad = rs.map((r) => r.reallocated + r.pending + r.uncorrect), bmax = Math.max(...bad, 1);
  const line = rs.map((r, i) => r.temp ? `${i ? "L" : "M"}${x(i).toFixed(1)} ${ty(r.temp).toFixed(1)}` : "").filter(Boolean).join(" ");
  const bars = rs.map((r, i) => bad[i] ? `<rect x="${(x(i) - 2).toFixed(1)}" y="${(H - 6 - (bad[i] / bmax) * 22).toFixed(1)}" width="4" height="${((bad[i] / bmax) * 22).toFixed(1)}" fill="var(--bad)"/>` : "").join("");
  const first = rs[0], last = rs[n - 1];
  return `<div class="hchart"><svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none">${bars}${line ? `<path d="${line}" fill="none" stroke="var(--accent)" stroke-width="1.6" stroke-linejoin="round"/>` : ""}</svg>
    <div class="hchart-foot"><span>${when(first.at)}</span><span>${n} readings${temps.length ? ` · ${tmin}–${tmax} °C` : ""}${Math.max(...bad) ? ` · <span class="bad">bad sectors in red</span>` : " · no bad sectors"}</span><span>${when(last.at)}</span></div></div>`;
}
function healthBox(d, hist) {
  const h = d.health;
  if (!h) return `<p class="hint" style="margin:0">Not read yet. ${SMART_HINT}</p>`;
  if (h.error) return `<p class="hint" style="margin:0"><b>${esc(h.error)}.</b> ${h.error.includes("not installed") ? SMART_HINT : "Read " + when(h.readAt) + "."}</p>`;
  const st = healthState(d);
  const head = st === "bad" ? `<div class="alert bad" style="margin-bottom:10px"><b>This drive reports it is failing.</b> Copy everything off it now and stop using it: ${esc(h.warnings.join(" · "))}.</div>`
    : st === "warn" ? `<div class="alert warn" style="margin-bottom:10px"><b>Worth watching:</b> ${esc(h.warnings.join(" · "))}. Keep its files backed up elsewhere.</div>`
    : `<div class="alert ok" style="margin-bottom:10px"><b>Passed.</b> The drive reports no problems.</div>`;
  const row = (k, v) => v ? `<dt>${k}</dt><dd>${v}</dd>` : "";
  return head + `<dl class="kv">
    ${row("Model", esc([h.model, h.serial].filter(Boolean).join(" · ")))}
    ${row("Powered on", hoursText(h.hours))}
    ${row("Temperature", h.temp ? `${h.temp} °C` : "")}
    ${row("Reallocated sectors", h.reallocated || h.pending ? String(h.reallocated || 0) : "")}
    ${row("Pending sectors", h.pending ? String(h.pending) : "")}
    ${row("Uncorrectable", h.uncorrect ? String(h.uncorrect) : "")}
    ${row("Interface errors", h.crc ? String(h.crc) : "")}
    ${row("Write life used", h.percentUsed != null && h.percentUsed > 0 ? `${h.percentUsed}%` : "")}
    ${row("Last read", when(h.readAt))}
  </dl>${healthChart(hist || [])}`;
}

function backupBadge(d) {
  const b = d.backups;
  if (d.damaged) return `<span class="badge bad">${ICON.warn}${d.damaged} damaged</span>`;
  if (!b || !d.files) return "";
  const fmt = safeToFormat(d);
  if (fmt === "safe") return `<span class="badge ok strong" title="Every file on this card has at least two verified copies on other drives">${ICON.check}Safe to format · ${b.minCopies}× elsewhere</span>`;
  if (fmt === "once") return `<span class="badge warn" title="Every file has exactly one verified copy elsewhere; the card is still the second copy">${ICON.warn}Backed up once · keep the card</span>`;
  const st = backupState(b.enough + b.short, b.files, b.unknown, b.likely);
  if (st === "unknown") return `<span class="badge">${ICON.dash}No checksums yet</span>`;
  if (st === "none") return `<span class="badge bad">${ICON.warn}No backup</span>`;
  if (st === "full") return `<span class="badge ok">${ICON.check}Verified backup</span>`;
  return `<span class="badge warn">${(b.enough + b.short).toLocaleString()} of ${b.files.toLocaleString()} files backed up</span>`;
}
function checkBadge(d) {
  if (d.busy) return `<span class="badge info">${ICON.clock}Working…</span>`;
  if (d.verifyRequested) return `<span class="badge info">${ICON.clock}Check queued</span>`;
  if (!d.lastVerify) return "";
  return d.lastVerifyOk
    ? `<span class="badge ok">${ICON.shield}Checked ${new Date(d.lastVerify * 1000).toLocaleDateString(undefined, { month: "short", day: "numeric" })}</span>`
    : `<span class="badge bad">${ICON.warn}Check failed</span>`;
}
// cardColor is what a drive's card colour means under the "Colour by"
// menu: its project's colour (drives in no project keep their own), its
// own label colour, its backup state, or its client.
const CLIENT_PALETTE = ["#4fa3ff", "#ff9f43", "#3ddc84", "#ec4899", "#14b8a6", "#ffcc4d", "#b36cff", "#ff5c5c"];
function cardColor(d) {
  switch (S.colorBy) {
    case "project": {
      const p = (S.state.projects || []).find((x) => (d.projects || []).includes(x.id));
      if (p) return COLORS[p.color] || "var(--accent)";
      return COLORS[d.labels.color] || null;
    }
    case "drive": return COLORS[d.labels.color] || null;
    case "backup": {
      const b = d.backups;
      if (d.damaged) return "var(--bad)";
      if (!b || !d.files) return null;
      return { full: "var(--ok)", likely: "var(--warn)", partial: "var(--warn)", none: "var(--bad)", unknown: "#6b6b74", empty: null }[backupState(b.enough + b.short, b.files, b.unknown, b.likely)];
    }
    case "client": {
      const c = (d.labels.client || "").trim().toLowerCase();
      if (!c) return null;
      let h = 0;
      for (const ch of c) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
      return CLIENT_PALETTE[h % CLIENT_PALETTE.length];
    }
  }
  return null;
}
function projectBadges(d) {
  const ps = (S.state.projects || []).filter((p) => (d.projects || []).includes(p.id));
  return ps.map((p) => `<span class="badge proj" style="--pc:${COLORS[p.color] || "var(--accent)"}" data-open-project="${p.id}">${ICON.film}${esc(p.name)}</span>`).join("");
}
function summaryText(d) {
  const parts = [];
  if (d.topFolders && d.topFolders.length) parts.push("Folders: " + d.topFolders.join(", "));
  if (d.topFiles && d.topFiles.length) parts.push("Files: " + d.topFiles.join(", "));
  return parts.join(" • ");
}
function hl(text) {
  const q = S.query.trim();
  if (!q) return esc(text);
  const i = text.toLowerCase().indexOf(q.toLowerCase());
  if (i < 0) return esc(text);
  return esc(text.slice(0, i)) + "<mark>" + esc(text.slice(i, i + q.length)) + "</mark>" + esc(text.slice(i + q.length));
}

// ---------- drive pictures ----------
// Every drive gets a picture: a hard drive, an SSD, a camera card or a
// RAID enclosure, from what macOS says about it (or the label's Icon),
// tinted with the card's colour.
const KINDS = { hdd: "Hard drive", ssd: "SSD", card: "Camera card", raid: "RAID or enclosure" };
function driveKind(d) {
  const l = d.labels || {};
  if (l.icon && KINDS[l.icon]) return l.icon;
  const name = ((d.title || "") + " " + (d.name || "")).toLowerCase();
  const tops = (d.topFolders || []).map((x) => x.toUpperCase());
  if (/\braid\b|\bnas\b|pegasus|promise|areca|thunderbay|synology|qnap|enclosure/.test(name)) return "raid";
  if (tops.some((x) => ["DCIM", "PRIVATE", "CLIPS", "XDROOT", "BPAV", "M4ROOT", "AVCHD", "CONTENTS"].includes(x)) || /secure digital|sd card|cfexpress|cfast/i.test(d.bus || "")) return "card";
  if (d.size >= 16e12 && !d.solidState) return "raid";
  return d.solidState ? "ssd" : "hdd";
}
const PIC = {
  hdd: `<g class="body"><path d="M10 10h40l6 7v23a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3V17z" fill="#34343d" stroke="#5c5c68"/><path d="M10 10h40l6 7H4z" fill="#4a4a55"/></g>
    <g class="platter"><circle cx="24" cy="30" r="9.5" fill="#17171c" stroke="#6a6a77"/><circle cx="24" cy="30" r="6" fill="none" stroke="#2c2c34"/><circle cx="24" cy="30" r="2.2" fill="#8a8a96"/><path d="M24 20.5a9.5 9.5 0 0 1 9.5 9.5" fill="none" stroke="#a9a9b6" stroke-width="1.2" stroke-linecap="round"/></g>
    <path class="arm" d="M44 22l-11 8.5" stroke="#b9b9c4" stroke-width="2" stroke-linecap="round"/><circle cx="44" cy="22" r="2" fill="#6b6b78"/>
    <rect x="38" y="30" width="13" height="7" rx="1.5" fill="var(--pc, #8a5cf6)"/>
    <circle class="led" cx="53" cy="39.5" r="1.7"/>`,
  ssd: `<rect x="5" y="14" width="54" height="24" rx="5" fill="#34343d" stroke="#5c5c68"/><path d="M5 19a5 5 0 0 1 5-5h44a5 5 0 0 1 5 5v3H5z" fill="var(--pc, #8a5cf6)"/>
    <g fill="#4a4a55" stroke="#55555f" stroke-width=".8"><rect x="11" y="26" width="8" height="7" rx="1"/><rect x="22" y="26" width="8" height="7" rx="1"/><rect x="33" y="26" width="8" height="7" rx="1"/></g>
    <path d="M12 29.5h6M23 29.5h6M34 29.5h6" stroke="#8a8a96" stroke-width="1"/><path d="M46 25.5h8M46 29h8M46 32.5h5" stroke="#6b6b78" stroke-width="1.2" stroke-linecap="round"/>
    <circle class="led" cx="55" cy="35" r="1.6"/>`,
  card: `<path d="M22 5h22a3 3 0 0 1 3 3v33a3 3 0 0 1-3 3H20a3 3 0 0 1-3-3V13l3-3z" fill="#34343d" stroke="#5c5c68"/><path d="M17 13l5-5v7h-5z" fill="#4a4a55"/>
    <g fill="#d9b24a"><rect x="24" y="7" width="2.6" height="6" rx=".6"/><rect x="28" y="7" width="2.6" height="6" rx=".6"/><rect x="32" y="7" width="2.6" height="6" rx=".6"/><rect x="36" y="7" width="2.6" height="6" rx=".6"/><rect x="40" y="7" width="2.6" height="6" rx=".6"/></g>
    <rect x="21" y="21" width="22" height="16" rx="2" fill="var(--pc, #8a5cf6)"/><path d="M24 26h12M24 30h8" stroke="rgba(0,0,0,.35)" stroke-width="2" stroke-linecap="round"/>
    <circle class="led" cx="41" cy="40.5" r="1.4"/>`,
  mac: `<rect x="8" y="9" width="48" height="32" rx="6" fill="#a9a9b4" stroke="#7d7d8a"/><rect x="8" y="9" width="48" height="6" rx="3" fill="#c3c3cc"/><path d="M12 15h40" stroke="#8d8d99" stroke-width=".8"/>
    <g fill="#5f5f6b"><circle cx="16" cy="21" r="1"/><circle cx="20" cy="21" r="1"/><circle cx="24" cy="21" r="1"/><circle cx="28" cy="21" r="1"/><circle cx="32" cy="21" r="1"/><circle cx="36" cy="21" r="1"/><circle cx="40" cy="21" r="1"/><circle cx="44" cy="21" r="1"/><circle cx="48" cy="21" r="1"/><circle cx="18" cy="24.5" r="1"/><circle cx="22" cy="24.5" r="1"/><circle cx="26" cy="24.5" r="1"/><circle cx="30" cy="24.5" r="1"/><circle cx="34" cy="24.5" r="1"/><circle cx="38" cy="24.5" r="1"/><circle cx="42" cy="24.5" r="1"/><circle cx="46" cy="24.5" r="1"/></g>
    <g fill="#2a2a31" stroke="#55555f" stroke-width=".6"><rect x="13" y="33" width="5" height="2.6" rx="1.3"/><rect x="20" y="33" width="5" height="2.6" rx="1.3"/><rect x="27" y="32.6" width="9" height="3.2" rx=".8"/></g>
    <rect x="11" y="39" width="42" height="2" rx="1" fill="#6f6f7b"/><circle class="led" cx="50" cy="34.3" r="1.1"/>`,
  raid: `<rect x="12" y="3" width="40" height="42" rx="5" fill="#34343d" stroke="#5c5c68"/><rect x="12" y="3" width="40" height="7" rx="3" fill="var(--pc, #8a5cf6)"/>
    <g fill="#1c1c22" stroke="#62626e" stroke-width=".8"><rect x="17" y="14" width="24" height="5.5" rx="1.2"/><rect x="17" y="21.5" width="24" height="5.5" rx="1.2"/><rect x="17" y="29" width="24" height="5.5" rx="1.2"/><rect x="17" y="36.5" width="24" height="5.5" rx="1.2"/></g>
    <g class="leds"><circle class="led" cx="46" cy="16.75" r="1.5"/><circle class="led" cx="46" cy="24.25" r="1.5"/><circle class="led" cx="46" cy="31.75" r="1.5"/><circle class="led" cx="46" cy="39.25" r="1.5"/></g>`,
};

// ---------- Classic (System 7) glyphs ----------
// One-bit pixel art: "#" is a pixel, "x" a pixel of 50% dither, "." nothing.
// Drawn on 16 or 12 pixel grids and shown at that size (or 2× or 3×), so
// every pixel lands on whole device pixels.
function pxSvg(rows, cls = "") {
  const h = rows.length, w = rows[0].length;
  let solid = "", dith = "";
  rows.forEach((r, y) => {
    if (r.length !== w) throw new Error("pxSvg: ragged bitmap");
    for (let x = 0; x < w; x++) {
      if (r[x] === "#") solid += `M${x} ${y}h1v1h-1z`;
      else if (r[x] === "x") dith += `M${x} ${y}h1v1h-1z`;
    }
  });
  return `<svg class="px px${w} ${cls}" viewBox="0 0 ${w} ${h}" shape-rendering="crispEdges" aria-hidden="true">${dith ? `<defs><pattern id="px-d" width="2" height="2" patternUnits="userSpaceOnUse"><path d="M0 0h1v1H0zM1 1h1v1H1z" fill="currentColor"/></pattern></defs><path d="${dith}" fill="url(#px-d)"/>` : ""}<path d="${solid}" fill="currentColor"/></svg>`;
}
const PXI = {
  drive: ["................", "................", "................", ".##############.", "#..............#", "#..............#", "################", "#..............#", "#..............#", "#.##...........#", "#..............#", ".##############.", "................", "................", "................", "................"],
  edit: ["................", "...........##...", "..........#..#..", ".........#..#...", "........#..#....", ".......#..#.....", "......#..#......", ".....#..#.......", "....#..#........", "...#..#.........", "..#..#..........", "..#.#...........", "..##............", "................", "................", "................"],
  person: ["................", "......####......", ".....#....#.....", ".....#....#.....", ".....#....#.....", "......####......", "................", "...##########...", "..#..........#..", "..#..........#..", "..#..........#..", "..#..........#..", "..############..", "................", "................", "................"],
  mail: ["................", "................", "................", ".##############.", ".#............#.", ".##..........##.", ".#.#........#.#.", ".#..#......#..#.", ".#...#....#...#.", ".#....####....#.", ".#............#.", ".#............#.", ".##############.", "................", "................", "................"],
  search: ["................", ".....#####......", "....#.....#.....", "...#.......#....", "...#.......#....", "...#.......#....", "...#.......#....", "....#.....#.....", ".....#####......", "..........##....", "...........##...", "............##..", ".............##.", "................", "................", "................"],
  refresh: ["................", ".....######.....", "....#......#....", "...#........#...", "...#.......##...", "..........####..", "............#...", "................", "................", "...#............", "..####..........", "...##.......#...", "...#........#...", "....#......#....", ".....######.....", "................"],
  export: ["................", ".......#........", "......###.......", ".....#.#.#......", "....#..#..#.....", ".......#........", ".......#........", ".......#........", ".......#........", ".......#........", "..##.......##...", "..#.........#...", "..#.........#...", "..#.........#...", "..###########...", "................"],
  down: ["............", "............", "............", "..#......#..", "..##....##..", "...##..##...", "....####....", ".....##.....", "............", "............", "............", "............"],
  up: ["............", "............", "............", "............", ".....##.....", "....####....", "...##..##...", "..##....##..", "..#......#..", "............", "............", "............"],
  bell: ["................", ".......##.......", "......#..#......", ".....#....#.....", "....#......#....", "....#......#....", "....#......#....", "....#......#....", "....#......#....", "...#........#...", "..#..........#..", "..############..", "......####......", ".......##.......", "................", "................"],
  history: ["................", ".....######.....", "....#......#....", "...#........#...", "..#....#.....#..", "..#....#.....#..", "..#....#.....#..", "..#....####..#..", "..#..........#..", "..#..........#..", "...#........#...", "....#......#....", ".....######.....", "................", "................", "................"],
  eject: ["................", "................", ".......##.......", "......####......", ".....######.....", "....########....", "...##########...", "..############..", "................", "..############..", "..############..", "................", "................", "................", "................", "................"],
  plug: ["............", "...#...#....", "...#...#....", "..#######...", "..#######...", "..#######...", "...#####....", "....###.....", ".....#......", ".....#......", "............", "............"],
  info: ["................", ".....######.....", "....#......#....", "...#...##...#...", "..#....##....#..", "..#..........#..", "..#...###....#..", "..#.....#....#..", "..#.....#....#..", "..#.....#....#..", "..#....###...#..", "...#........#...", "....#......#....", ".....######.....", "................", "................"],
  gauge: ["................", "................", "......####......", "....##....##....", "...#........#...", "..#..........#..", "..#.....#....#..", ".#.....#......#.", ".#....#.......#.", ".#...#........#.", ".#...#........#.", ".#............#.", ".##############.", "................", "................", "................"],
  pie: ["................", ".....######.....", "....#......#....", "...#....#...#...", "..#.....#....#..", "..#.....#....#..", "..#.....######..", "..#..........#..", "..#..........#..", "..#..........#..", "...#........#...", "....#......#....", ".....######.....", "................", "................", "................"],
  shield: ["................", ".......##.......", "....###..###....", "...#........#...", "...#........#...", "...#........#...", "...#........#...", "...#........#...", "....#......#....", "....#......#....", ".....#....#.....", "......#..#......", ".......##.......", "................", "................", "................"],
  tag: ["................", "................", ".........######.", "........#.....#.", ".......#...#..#.", "......#.......#.", ".....#.......#..", "....#.......#...", "...#.......#....", "..#.......#.....", "..#......#......", "...#....#.......", "....####........", "................", "................", "................"],
  folder: ["................", "................", ".######.........", "#......#........", "#.......########", "#..............#", "################", "#..............#", "#..............#", "#..............#", "#..............#", "#..............#", "#..............#", "################", "................", "................"],
  file: ["................", "..#########.....", "..#........#....", "..#........##...", "..#........#.#..", "..#........####.", "..#...........#.", "..#...........#.", "..#...........#.", "..#...........#.", "..#...........#.", "..#...........#.", "..#...........#.", "..#############.", "................", "................"],
  chev: ["............", "...#........", "...##.......", "....##......", ".....##.....", "......##....", ".....##.....", "....##......", "...##.......", "...#........", "............", "............"],
  check: ["............", "............", "..........#.", ".........##.", "........##..", ".......##...", "..#...##....", "..##.##.....", "...###......", "....#.......", "............", "............"],
  warn: ["............", ".....#......", "....###.....", "....#.#.....", "...#.#.#....", "...#.#.#....", "..#..#..#...", "..#.....#...", ".#...#...#..", ".#########..", "............", "............"],
  x: ["............", "............", "..#.....#...", "..##...##...", "...##.##....", "....###.....", "...##.##....", "..##...##...", "..#.....#...", "............", "............", "............"],
  clock: ["............", "....####....", "...#....#...", "..#..#...#..", "..#..#...#..", "..#..##..#..", "..#......#..", "...#....#...", "....####....", "............", "............", "............"],
  dash: ["............", "............", "............", "............", "............", "..#######...", "..#######...", "............", "............", "............", "............", "............"],
  trash: ["................", "......####......", "..############..", "..#..........#..", "..############..", "...#........#...", "...#.#.#..#.#...", "...#.#.#..#.#...", "...#.#.#..#.#...", "...#.#.#..#.#...", "...#.#.#..#.#...", "...#.#.#..#.#...", "...#........#...", "....########....", "................", "................"],
  plus: ["................", "................", "................", ".......##.......", ".......##.......", ".......##.......", ".......##.......", "...##########...", "...##########...", ".......##.......", ".......##.......", ".......##.......", ".......##.......", "................", "................", "................"],
  copy: ["................", "..#######.......", "..#.....#.......", "..#.....#.......", "..#..#######....", "..#..#.....#....", "..#..#.....#....", "..#..#.....#....", "..###.#....#....", ".....#.....#....", ".....#.....#....", ".....#.....#....", ".....#######....", "................", "................", "................"],
  activity: ["................", "................", "................", "................", "................", "......#.........", ".....#.#........", ".....#.#........", "#####...#...####", ".........#.#....", "..........#.....", "................", "................", "................", "................", "................"],
  film: ["................", ".##############.", ".#.#........#.#.", ".###........###.", ".#.#........#.#.", ".###........###.", ".#.#........#.#.", ".###........###.", ".#.#........#.#.", ".###........###.", ".#.#........#.#.", ".###........###.", ".#.#........#.#.", ".##############.", "................", "................"],
  back: ["................", "................", "................", "......#.........", ".....##.........", "....###.........", "...############.", "..#############.", "...############.", "....###.........", ".....##.........", "......#.........", "................", "................", "................", "................"],
  arrow: ["................", "................", "................", ".........#......", ".........##.....", ".........###....", ".############...", ".#############..", ".############...", ".........###....", ".........##.....", ".........#......", "................", "................", "................", "................"],
  play: ["............", "...#........", "...##.......", "...###......", "...####.....", "...#####....", "...#####....", "...####.....", "...###......", "...##.......", "...#........", "............"],
  globe: ["................", ".....######.....", "....#..#...#....", "...#...#....#...", "..#....#.....#..", "..############..", "..#....#.....#..", "..#....#.....#..", "..############..", "..#....#.....#..", "...#...#....#...", "....#..#...#....", ".....######.....", "................", "................", "................"],
  clapper: ["................", ".#############..", ".#.#.#.#.#.#.#..", ".##.#.#.#.#.##..", ".#############..", ".#...........#..", ".#...........#..", ".#...........#..", ".#...........#..", ".#...........#..", ".#...........#..", ".#############..", "................", "................", "................", "................"],
  reveal: ["................", ".###########....", ".#.........#....", ".#..##..##.#....", ".#.........#....", ".#.#.....#.#....", ".#..#####..#....", ".#.........#....", ".###########....", ".#.........#....", ".#.........#....", ".###########....", "...#.....#......", "..###...###.....", "................", "................"],
};
const ICON_CLASSIC = Object.fromEntries(Object.entries(PXI).map(([k, rows]) => [k, pxSvg(rows)]));
const ICON_MODERN = { ...ICON };
// The drive pictures: 32×24 pixels, shown at 2× or 3×.
const PXP = {
  hdd: ["................................", "................................", "................................", "................................", ".....########################...", "....#xxxxxxxxxxxxxxxxxxxxxxxx#..", "...#xxxxxxxxxxxxxxxxxxxxxxxxx#..", "..############################..", "..#..........................#..", "..#..........................#..", "..#..........................#..", "..#..........................#..", "..#..........................#..", "..#..........................#..", "..#..##......................#..", "..#..##......................#..", "..############################..", "...xxxxxxxxxxxxxxxxxxxxxxxxxxx..", "................................", "................................", "................................", "................................", "................................", "................................"],
  ssd: ["................................", "................................", "................................", "................................", "................................", "..############################..", "..#..........................#..", "..#..######..######..######..#..", "..#..#xxxx#..#xxxx#..#xxxx#..#..", "..#..#xxxx#..#xxxx#..#xxxx#..#..", "..#..######..######..######..#..", "..#..........................#..", "..#.....................##...#..", "..############################..", "...xxxxxxxxxxxxxxxxxxxxxxxxxxx..", "................................", "................................", "................................", "................................", "................................", "................................", "................................", "................................", "................................"],
  card: ["................................", "...........###########..........", "..........#...........#.........", ".........#.#.#.#.#.#..#.........", "........#..#.#.#.#.#..#.........", "........#.............#.........", "........#.............#.........", "........#.###########.#.........", "........#.#xxxxxxxxx#.#.........", "........#.#xxxxxxxxx#.#.........", "........#.#xxxxxxxxx#.#.........", "........#.#xxxxxxxxx#.#.........", "........#.#xxxxxxxxx#.#.........", "........#.#xxxxxxxxx#.#.........", "........#.###########.#.........", "........#.............#.........", "........#.............#.........", "........#.............#.........", "........###############.........", ".........xxxxxxxxxxxxxx.........", "................................", "................................", "................................", "................................"],
  mac: ["................................", ".........################.......", ".........#..............#.......", ".........#.############.#.......", ".........#.#..........#.#.......", ".........#.#..........#.#.......", ".........#.#..##..##..#.#.......", ".........#.#..........#.#.......", ".........#.#.....#....#.#.......", ".........#.#...##.....#.#.......", ".........#.#..#....#..#.#.......", ".........#.#...####...#.#.......", ".........#.#..........#.#.......", ".........#.############.#.......", ".........#..............#.......", ".........#...########...#.......", ".........#..............#.......", ".........#..............#.......", ".........#..xxxxxxxxxx..#.......", ".........################.......", "........#................#......", "........##################......", ".........xxxxxxxxxxxxxxxx.......", "................................"],
  raid: ["................................", "........################........", "........#..............#........", "........#.############.#........", "........#.#xxxxxxxx#.#.#........", "........#.############.#........", "........#..............#........", "........#.############.#........", "........#.#xxxxxxxx#.#.#........", "........#.############.#........", "........#..............#........", "........#.############.#........", "........#.#xxxxxxxx#.#.#........", "........#.############.#........", "........#..............#........", "........#.############.#........", "........#.#xxxxxxxx#.#.#........", "........#.############.#........", "........#..............#........", "........################........", ".........xxxxxxxxxxxxxx.........", "................................", "................................", "................................"],
};
const PIC_CLASSIC = Object.fromEntries(Object.entries(PXP).map(([k, rows]) => [k, pxSvg(rows, "pic")]));
function isClassic() { return document.documentElement.dataset.style === "classic"; }
// useClassicIcons swaps the glyph set the page draws with, and repaints
// the two icons that live in the static chrome.
function useClassicIcons(on) {
  Object.assign(ICON, ICON_MODERN, on ? ICON_CLASSIC : {});
  paintChrome();
}
// paintChrome draws the two glyphs that live in the static top bar.
function paintChrome() {
  const st = $("#settings"), act = $("#activity-btn");
  if (st) st.innerHTML = ICON.gear;
  if (act) { act.querySelectorAll("svg").forEach((x) => x.remove()); act.insertAdjacentHTML("afterbegin", ICON.activity); }
}
function drivePic(kind, color, on) {
  if (isClassic()) return (PIC_CLASSIC[kind] || PIC_CLASSIC.hdd).replace('class="px pic"', `class="px pic ${on ? "on" : ""}"`);
  return `<svg class="pic ${on ? "on" : ""}" viewBox="0 0 64 48" style="--pc:${color || "var(--muted)"}" aria-hidden="true">${PIC[kind] || PIC.hdd}</svg>`;
}
// capRing is how full a drive is, as a ring: pale until 70%, then
// yellow, then red in the last 10%.
function capRing(used, size, cls = "") {
  const pct = size ? Math.max(0, Math.min(100, (used / size) * 100)) : 0;
  const r = 24, c = 2 * Math.PI * r;
  const col = pct >= 90 ? "var(--bad)" : pct >= 85 ? "var(--orange)" : pct >= 70 ? "var(--warn)" : "var(--ring)";
  return `<div class="ring ${cls}" title="${pct.toFixed(0)}% full"><svg viewBox="0 0 60 60"><circle cx="30" cy="30" r="${r}" class="track"/><circle cx="30" cy="30" r="${r}" class="fill" style="stroke:${col};stroke-dasharray:${c.toFixed(2)};stroke-dashoffset:${(c * (1 - pct / 100)).toFixed(2)}"/></svg><span class="pct" style="color:${pct >= 70 ? col : "var(--text)"}">${Math.round(pct)}</span></div>`;
}
// loadingGlyph is a hard drive being read, for every "loading" state.
function loadingGlyph(text, small) {
  if (isClassic()) return `<div class="loading ${small ? "small" : ""}">${PIC_CLASSIC.hdd.replace('class="px pic"', 'class="px pic reading"')}${text ? `<p>${esc(text)}</p>` : ""}</div>`;
  return `<div class="loading ${small ? "small" : ""}"><svg class="pic reading" viewBox="0 0 64 48" aria-hidden="true">${PIC.hdd}</svg>${text ? `<p>${esc(text)}</p>` : ""}</div>`;
}

function driveCard(d) {
  const l = d.labels;
  const color = cardColor(d);
  const tint = color || "transparent";
  const dot = d.connected ? "var(--ok)" : color || "var(--muted)";
  const used = Math.max(0, d.size - d.free);
  const pct = d.size ? Math.min(100, (used / d.size) * 100) : 0;
  const who = [l.client, l.project].filter(Boolean);
  const sum = summaryText(d);
  const kind = driveKind(d);
  return `<article class="card ${S.selecting && S.selected.has(d.id) ? "picked" : ""} ${d.connected ? "live" : ""}" style="--tint:${tint}" data-drive="${esc(d.id)}" title="${esc(KINDS[kind])}${d.connected ? " · plugged in" : ""}">
    <div class="card-head">
      <div class="card-pic">${drivePic(kind, color || dot, d.connected)}</div>
      <div class="card-main">
        <div class="card-title"><h3>${hl(d.title)}</h3>${l.nickname ? `<span class="vol">(${hl(d.name)})</span>` : ""}</div>
        ${who.length ? `<div class="meta-line">${ICON.person}<span>${who.map(hl).join(" &nbsp;·&nbsp; ")}</span></div>` : ""}
        ${l.contact ? `<div class="meta-line">${ICON.mail}<span>${hl(l.contact)}</span></div>` : ""}
        <div class="path" title="${esc(d.mount || d.lastMount)}">${esc(d.mount || d.lastMount)}</div>
      </div>
      ${capRing(used, d.size)}
    </div>
    <div class="cap"><b>${bytes(d.size)}</b><span class="sep">•</span><span class="${lowSpace(d) ? "low" : ""}">${bytes(d.free)} free</span><span class="sep">•</span>${d.files.toLocaleString()} files</div>
    ${sum ? `<div class="summary">${esc(sum)}</div><button class="more" data-act="open">More</button>` : ""}
    <div class="badges">${healthBadge(d)}${backupBadge(d)}${checkBadge(d)}${projectBadges(d)}</div>
    <div class="card-foot"><span><i class="dot ${d.connected ? "live" : ""}" style="background:${dot};color:${dot}"></i>${d.connected ? "Plugged in now" : "Last seen • " + when(d.lastSeen)}</span>
      <span class="card-acts">${S.selecting ? `<input type="checkbox" class="pick" data-pick="${esc(d.id)}" ${S.selected.has(d.id) ? "checked" : ""}>` : `${d.connected ? `<button class="icon-btn card-edit" data-act="eject" title="Eject, so it can be unplugged">${ICON.eject}</button>` : ""}<button class="icon-btn card-edit" data-act="edit" title="Edit labels">${ICON.edit}</button><button class="btn small" data-act="open">Details</button>`}</span></div>
  </article>`;
}

function sortDrives(list) {
  const key = {
    seen: (d) => (d.connected ? 1e12 : 0) + d.lastSeen,
    name: (d) => d.title.toLowerCase(),
    size: (d) => d.size,
    free: (d) => d.free,
    backup: (d) => (d.backups ? (d.damaged ? -2 : d.backups.short + d.backups.none + d.backups.unknown > 0 ? -1 : d.backups.minCopies) : -3),
  }[S.sort];
  const out = [...list].sort((a, b) => {
    const x = key(a), y = key(b);
    return x < y ? -1 : x > y ? 1 : 0;
  });
  return S.desc ? out.reverse() : out;
}

function matchesQuery(d) {
  const q = S.query.trim().toLowerCase();
  if (!q) return true;
  const l = d.labels;
  return [d.title, d.name, l.client, l.project, l.contact, l.notes, d.mount, d.lastMount, ...(d.topFolders || [])]
    .some((s) => (s || "").toLowerCase().includes(q));
}

function renderToolbar() {
  const tb = $("#toolbar");
  const show = S.tab === "logged" || S.tab === "connected";
  tb.hidden = !show;
  if (!show) return;
  const title = S.tab === "logged" ? ["Logged", ICON.history] : ["Connected", ICON.plug];
  $("#toolbar-title").innerHTML = title[1] + esc(title[0]);
  const drives = visibleDrives();
  const used = drives.reduce((a, d) => a + Math.max(0, d.size - d.free), 0);
  const free = drives.reduce((a, d) => a + d.free, 0);
  $("#pills").innerHTML = `<span class="pill">Drives:<b>${drives.length}</b></span><span class="pill">Used:<b>${bytes(used)}</b></span><span class="pill">Free:<b>${bytes(free)}</b></span>`;
  $("#sort-dir").innerHTML = S.desc ? ICON.down : ICON.up;
  for (const b of $("#view-seg").children) b.classList.toggle("on", b.dataset.view === S.view);
  $("#sort").value = S.sort;
  $("#color-by").value = S.colorBy;
  $("#select-mode").classList.toggle("on", S.selecting);
  $("#select-mode").textContent = S.selecting ? "Done" : "Select";
}

function visibleDrives() {
  if (!S.state) return [];
  let ds = S.state.drives;
  if (S.tab === "connected") ds = ds.filter((d) => d.connected);
  return ds;
}

function renderCounts() {
  const st = S.state;
  for (const b of document.querySelectorAll(".tabs button")) b.classList.toggle("on", b.dataset.tab === S.tab);
  $("#count-connected").textContent = st.volumes.length || "";
  $("#count-logged").textContent = st.drives.length || "";
  $("#count-projects").textContent = (st.projects || []).length || "";
  const pill = $("#agent-pill");
  const agentOn = st.agent.startsWith("installed and running");
  pill.className = "agent-pill" + (agentOn ? " on" : "");
  pill.textContent = agentOn ? "Auto-logging on" : "Logging while open";
  pill.title = "Background agent: " + st.agent;
  const ap = $("#awake-pill"), aw = st.awake || {};
  ap.hidden = !aw.on;
  if (aw.on) {
    const n = (aw.drives || []).length;
    ap.className = "agent-pill on";
    ap.textContent = `${ICON_TXT.coffee}${n ? n + (n === 1 ? " drive" : " drives") + " awake" : "Keeping drives awake"}${aw.mac ? " · Mac too" : ""}`;
    ap.title = `Keep drives awake is on (Settings → Drives). ${n ? "Last touched " + (aw.last ? when(aw.last) : "") : "No spinning drive is plugged in."}${aw.mac ? " The Mac is held out of idle sleep while Lumpy is open." : ""}`;
  }
  const pp = $("#publish-pill");
  if (S.settings && S.settings.publish.enabled) {
    const last = S.settings.last;
    pp.hidden = false;
    pp.className = "agent-pill" + (last && last.ok ? " on" : last ? " bad" : "");
    pp.textContent = S.settings.publishing ? "Publishing…" : last ? (last.ok ? "Status page · " + short(Math.floor(new Date(last.at).getTime() / 1000)) : "Status page failed") : "Status page";
    pp.title = last ? last.summary : "Not published yet";
  } else pp.hidden = true;
  const up = $("#update-pill"), u = st.update || {};
  if (u.installed) { up.hidden = false; up.className = "update-pill ready"; up.textContent = `Lumpy ${u.installed} installed · ${native ? "relaunch" : "restart"} to finish`; }
  else if (u.available) { up.hidden = false; up.className = "update-pill"; up.textContent = `Update to ${u.available}`; }
  else up.hidden = true;
}

function render() {
  if (!S.state) return;
  closePlayers();
  renderCounts();
  renderToolbar();
  const main = $("#main");
  const keepScroll = main.scrollTop;
  main.classList.toggle("map-main", S.tab === "map");
  if (S.tab === "map") return renderMap();
  if (S.tab === "offload") return renderOffload();
  if (S.tab === "space") return renderSpace();
  if (S.tab === "projects") return renderProjects();
  if (S.tab === "media") return renderMedia();
  main.innerHTML = (S.tab === "connected" ? renderConnected() : renderLogged()) + selectionBar();
  main.scrollTop = keepScroll;
}

// Drives stay with their project: grouped by the first project each
// belongs to, projects by name, drives in no project last; the chosen
// sort applies within each group.
function driveFamilies(ds) {
  const names = Object.fromEntries((S.state.projects || []).map((p) => [p.id, p.name]));
  const groups = new Map();
  for (const d of ds) {
    const pid = (d.projects || []).find((id) => names[id] !== undefined);
    const k = pid === undefined ? "" : String(pid);
    if (!groups.has(k)) groups.set(k, { name: pid === undefined ? "Not in a project" : names[pid], ds: [] });
    groups.get(k).ds.push(d);
  }
  return [...groups.entries()].sort(([a, x], [b, y]) => (a === "") - (b === "") || x.name.localeCompare(y.name)).map(([, g]) => g);
}
// Groups fold: click a header to collapse it, and it stays collapsed.
S.folded = new Set(JSON.parse(localGet("folded", "[]")));
function drivesHTML(ds) {
  const fams = driveFamilies(ds);
  if (fams.length <= 1) return drivesHTML1(ds);
  const list = S.view === "list";
  return (list ? listHead() : "") + fams.map((g) => {
    const off = S.folded.has(g.name);
    return `<div class="section-head fold ${off ? "off" : ""}" data-fold="${esc(g.name)}"><span class="chev">${ICON.chev}</span><h2>${esc(g.name)}</h2><span>${g.ds.length}</span></div>${off ? "" : drivesHTML1(g.ds, list)}`;
  }).join("");
}
const DRIVE_COLS = `<colgroup><col style="width:20%"><col style="width:14%"><col style="width:8%"><col style="width:11%"><col style="width:8%"><col style="width:6%"><col style="width:14%"><col style="width:8%"><col style="width:11%"></colgroup>`;
function listHead() {
  return `<table class="list drives head">${DRIVE_COLS}<thead><tr><th>Name</th><th>Client · Project</th><th class="num">Capacity</th><th>Used</th><th class="num">Free</th><th class="num">Files</th><th>Backups</th><th>Last check</th><th>Last seen</th></tr></thead></table>`;
}
function drivesHTML1(ds, headless) {
  if (S.view === "list") {
    return `<table class="list drives">${DRIVE_COLS}${headless ? "" : `<thead><tr><th>Name</th><th>Client · Project</th><th class="num">Capacity</th><th>Used</th><th class="num">Free</th><th class="num">Files</th><th>Backups</th><th>Last check</th><th>Last seen</th></tr></thead>`}<tbody>
      ${ds.map((d) => {
        const used = Math.max(0, d.size - d.free);
        return `<tr class="row" data-drive="${esc(d.id)}"><td><div class="name"><span class="dot" style="background:${d.connected ? "var(--ok)" : cardColor(d) || "var(--muted)"}"></span>${hl(d.title)}${d.labels.nickname ? ` <small>${esc(d.name)}</small>` : ""}</div></td>
          <td>${esc([d.labels.client, d.labels.project].filter(Boolean).join(" · "))}</td>
          <td class="num">${bytes(d.size)}</td><td>${capBar(used, d.size)}</td>
          <td class="num ${lowSpace(d) ? "cap" : ""}"><span class="${lowSpace(d) ? "low" : ""}">${bytes(d.free)}</span></td><td class="num">${d.files.toLocaleString()}</td>
          <td>${backupBadge(d)}</td><td>${checkBadge(d) || '<span class="badge">Never</span>'}</td>
          <td>${d.connected ? '<span class="badge live">Plugged in</span>' : short(d.lastSeen)}</td></tr>`;
      }).join("")}</tbody></table>`;
  }
  return `<div class="grid">${ds.map(driveCard).join("")}</div>`;
}

function fileHitsHTML() {
  if (!S.query.trim() || !S.fileHits) return "";
  const hits = S.fileHits;
  if (!hits.length) return `<div class="section-head"><h2>Files</h2><span>No indexed file names match “${esc(S.query)}”</span></div>`;
  const byId = Object.fromEntries((S.state.drives || []).map((d) => [d.id, d]));
  return `<div class="section-head"><h2>Files</h2><span>${hits.length >= 300 ? "first 300" : hits.length} matching “${esc(S.query)}”</span></div>
    <table class="list"><thead><tr><th>Drive</th><th>Path</th><th class="num">Size</th><th>xxh64</th></tr></thead><tbody>
    ${hits.map((m) => {
      const d = byId[m.DriveID];
      return `<tr class="row" data-file="${esc(m.Path)}" data-file-drive="${esc(m.DriveID)}"><td><div class="name"><span class="dot" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(d ? d.title : m.DriveName)}</div></td>
        <td class="mono">${hl(m.Path)}</td><td class="num">${bytes(m.Size)}</td><td class="mono">${esc(m.XXH64 || "—")}</td></tr>`;
    }).join("")}</tbody></table>`;
}

function selectionBar() {
  if (!S.selecting) return "";
  const n = S.selected.size;
  return `<div class="selbar"><b>${n}</b> drive${n === 1 ? "" : "s"} selected
    <button class="btn primary" id="sel-add" ${n ? "" : "disabled"}>${ICON.film}Add to project…</button>
    <button class="btn" id="sel-new" ${n ? "" : "disabled"}>${ICON.plus}New project from selection</button>
    <button class="btn" id="sel-all">Select all shown</button>
    <button class="btn" id="sel-none">Clear</button></div>`;
}

function renderLogged() {
  const all = S.state.drives;
  if (!all.length) {
    return `<div class="empty">${ICON.drive}<h2>No drives logged yet</h2>
      <p>Plug in a drive and Lumpy logs it automatically while this window is open. Install the background agent with <code>lumpy agent install</code> to log drives even when it's closed.</p></div>`;
  }
  const ds = sortDrives(all.filter(matchesQuery));
  let html = "";
  if (S.query.trim()) html += `<div class="section-head"><h2>Drives</h2><span>${ds.length} matching</span></div>`;
  html += ds.length ? drivesHTML(ds) : S.query.trim() ? "" : "";
  html += fileHitsHTML();
  return html;
}

function renderConnected() {
  const st = S.state;
  const logged = sortDrives(st.drives.filter((d) => d.connected && matchesQuery(d)));
  const unlogged = st.volumes.filter((v) => !v.logged);
  const neverLog = st.drives.filter((d) => d.labels.neverLog && st.volumes.some((v) => v.id === d.id));
  if (!st.volumes.length) {
    return `<div class="empty">${ICON.plug}<h2>No drives plugged in</h2><p>Connect a drive or card. Lumpy logs it as soon as it mounts.</p></div>`;
  }
  let html = logged.length ? drivesHTML(logged) : "";
  const extra = [...unlogged.map((v) => ({ v, never: false })), ...neverLog.filter((d) => !logged.includes(d)).map((d) => ({ v: st.volumes.find((x) => x.id === d.id), never: true }))];
  if (extra.length) {
    html += `<div class="section-head"><h2>Not logged</h2><span>${extra.length}</span></div><div class="grid">`;
    html += extra.map(({ v, never }) => `<article class="card live"><div class="card-head"><div class="card-pic">${drivePic(driveKind({ name: v.name, title: v.name, size: v.size, solidState: v.solidState, bus: v.bus }), "", true)}</div>
      <div class="card-main"><div class="card-title"><h3>${esc(v.name)}</h3></div><div class="path">${esc(v.mount)}</div></div>${capRing(v.size - v.free, v.size)}</div>
      <div class="cap"><b>${bytes(v.size)}</b><span class="sep">•</span>${bytes(v.free)} free</div>
      <div class="card-foot"><span><i class="dot live" style="background:var(--ok);color:var(--ok)"></i>${never ? "Set to never log" : v.capture ? "Card or stick · logged only by hand" : S.settings && S.settings.prefs && !S.settings.prefs.autoLog ? "New · logging by hand" : "Being logged…"}</span>
      <button class="btn small primary" data-log="${esc(v.id)}">Log now</button></div></article>`).join("");
    html += "</div>";
  }
  return html;
}

// ---------- details ----------
function openSheet(html, small) {
  const sh = $("#sheet");
  closePlayers();
  sh.className = "sheet" + (small ? " small" : "");
  // Classic windows carry the window's name in the title bar and a red
  // close button at its left; the modern × steps aside there.
  sh.innerHTML = `<button class="win-close" title="Close" aria-label="Close"></button>` + html;
  const h = sh.querySelector("h2, .sheet-head b");
  if (h) sh.insertAdjacentHTML("afterbegin", `<span class="win-title">${esc(h.textContent.trim())}</span>`);
  for (const b of sh.querySelectorAll(".sheet-head .icon-btn")) if ((b.id.endsWith("close") || b.title === "Close") && b.title !== "Back") b.classList.add("x-close");
  sh.querySelector(".win-close").onclick = () => { const x = sh.querySelector(".sheet-head .x-close"); if (x) x.click(); else if (S.file) closeFile(); else { sh.hidden = true; $("#scrim").hidden = true; } };
  sh.hidden = false;
  $("#scrim").hidden = false;
}
function closeSheet() {
  closePlayers();
  $("#sheet").hidden = true;
  $("#scrim").hidden = true;
  S.detail = null;
  S.file = null;
}

async function openDetail(id, refresh, focusLabels) {
  let data;
  try { data = await api(`/api/drives/${encodeURIComponent(id)}`); }
  catch (e) { toast(e.message, true); return; }
  if (refresh && S.detail !== id) return;
  S.detail = id;
  const d = data.drive, l = d.labels, b = data.backups;
  const used = Math.max(0, d.size - d.free);
  const folders = data.folders || [];
  const myProjects = new Set(data.projects || []);
  const missing = b.short + b.none + b.unknown;
  const holders = Object.entries(b.holders).sort((a, c) => c[1] - a[1]);
  const scroll = refresh ? ($(".sheet-body") || {}).scrollTop : 0;

  openSheet(`
    <div class="sheet-head">
      <span class="drive-ico">${ICON.drive}</span>
      <div><h2>${esc(d.title)}</h2><div class="sub">${esc(KINDS[driveKind(d)])}${l.nickname && d.name !== d.title ? " · " + esc(d.name) : ""} · ${d.connected ? "plugged in" : "last seen " + esc(when(d.lastSeen))}</div></div>
      <div class="actions">
        ${d.connected ? `<button class="icon-btn" id="eject" title="Eject: unmount it so it can be unplugged">${ICON.eject}</button>` : ""}
        <button class="btn danger" id="forget">Remove</button>
        <button class="icon-btn" id="close" title="Close">${ICON.x}</button>
      </div>
    </div>
    <div class="sheet-body">
      <div class="sec"><div class="sec-title">${ICON.info}Identity</div>
        <div class="box"><dl class="kv">
          <dt>Name</dt><dd>${esc(d.name)}</dd>
          <dt>Path</dt><dd>${esc(d.mount || d.lastMount)} ${d.connected && native ? `<button class="btn small" id="reveal">${ICON.reveal}Show in Finder</button>` : ""}</dd>
          <dt>${d.id.startsWith("part:") ? "Partition GUID" : d.id.startsWith("name:") ? "Known by" : "Volume UUID"}</dt><dd class="mono">${esc(d.id.startsWith("name:") ? "its name and size (this format has no UUID; renaming it makes a new drive)" : d.id.replace(/^part:/, ""))}</dd>
          ${(data.lookalikes || []).length ? `<dt>Renamed?</dt><dd>${data.lookalikes.map((x) => `<div class="adopt-row">If this is <b>${esc(x.title)}</b> under a new name (${x.files.toLocaleString()} files logged, ${x.hashed.toLocaleString()} checksummed, last seen ${when(x.lastSeen)}) <button class="btn small" data-adopt="${esc(x.id)}" data-adopt-title="${esc(x.title)}">Carry its log over</button></div>`).join("")}</dd>` : ""}
          <dt>Format</dt><dd>${esc(d.fstype || "—")}</dd>
          <dt>First seen</dt><dd>${when(d.firstSeen)}</dd>
          <dt>Last seen</dt><dd>${d.connected ? "Plugged in now" : when(d.lastSeen)}</dd>
        </dl></div></div>

      <div class="sec"><div class="sec-title">${ICON.shield}Rules for this drive</div>
        <div class="box rules">
          <label class="switch row"><span>Never put backups here<small>Project plans won't copy onto it: camera cards that get wiped, loaners, the edit SSD.</small></span><input type="checkbox" id="no-backups" ${l.noBackups ? "checked" : ""}></label>
          <label class="switch row"><span>Never log this drive<small>Lumpy leaves it alone when it's plugged in: no scan, no check, no card.</small></span><input type="checkbox" id="never-log" ${l.neverLog ? "checked" : ""}></label>
          ${!d.solidState && !["ssd", "card"].includes(l.icon || "") ? `<label class="switch row"><span>Let it spin down<small>Left out of Keep drives awake (Settings). ${S.state.awake && S.state.awake.on ? (S.state.awake.drives.includes(d.id) ? "Kept awake now." : l.letSleep ? "" : d.connected ? "Not touched yet." : "") : "Keep drives awake is off."}</small></span><input type="checkbox" id="let-sleep" ${l.letSleep ? "checked" : ""}></label>` : ""}
        </div></div>

      <div class="sec"><div class="sec-title">${ICON.tag}Labels</div>
        <div class="box"><div class="fields">
          <div class="field"><label>Display name</label><input data-label="nickname" value="${esc(l.nickname)}" placeholder="${esc(d.name)}"></div>
          <div class="field"><label>Contact</label><input data-label="contact" value="${esc(l.contact)}" placeholder="Producer: name@company.com"></div>
          <div class="field"><label>Client</label><input data-label="client" value="${esc(l.client)}" placeholder="Client"></div>
          <div class="field"><label>Project</label><input data-label="project" value="${esc(l.project)}" placeholder="Project or show"></div>
          <div class="field"><label>Picture on the card</label><select class="plain" data-label="icon"><option value="" ${l.icon ? "" : "selected"}>Automatic (${KINDS[driveKind({ ...d, labels: { ...l, icon: "" } })]}${d.bus ? ", " + esc(d.bus) : ""})</option>${Object.entries(KINDS).map(([k, v]) => `<option value="${k}" ${l.icon === k ? "selected" : ""}>${v}</option>`).join("")}</select></div>
          <div class="field wide"><label>Notes</label><textarea data-label="notes" placeholder="Where it lives, who has it, what's on it…">${esc(l.notes)}</textarea></div>
          <div class="field wide"><label>Card colour (used when the drive is in no project, or when colouring by drive label)</label><div class="swatches">
            <button class="swatch none ${l.color ? "" : "on"}" data-color="" title="None"></button>
            ${Object.entries(COLORS).map(([k, c]) => `<button class="swatch ${l.color === k ? "on" : ""}" data-color="${k}" title="${k}" style="background:${c}"></button>`).join("")}
          </div></div>
        </div></div></div>

      <div class="sec"><div class="sec-title">${ICON.gauge}Capacity</div>
        <div class="box"><dl class="kv"><dt>Total</dt><dd>${bytes(d.size)}</dd><dt>Used</dt><dd>${bytes(used)}</dd><dt>Free</dt><dd class="cap"><span class="${lowSpace(d) ? "low" : ""}">${bytes(d.free)}${lowSpace(d) ? " · under 10%" : ""}</span></dd></dl>
        <div style="margin-top:10px">${capBar(used, d.size).replace('class="bar cap"', 'class="bar cap" style="height:7px"')}</div></div></div>

      <div class="sec"><div class="sec-title">${ICON.activity}Drive health
          <div class="right">${d.connected ? `<button class="btn small" id="health-read" title="Ask the drive for its SMART report now">${ICON.refresh}Read now</button>` : ""}</div></div>
        <div class="box">${healthBox(d, data.health || [])}</div></div>

      <div class="sec"><div class="sec-title">${ICON.shield}Backups &amp; checks
          <div class="right">
            ${d.verifyRequested ? `<button class="btn small" id="cancel-check">Cancel queued check</button>` : ""}
            ${d.connected ? `<button class="btn small" id="hash" ${d.busy ? "disabled" : ""}>Checksum new files</button>` : ""}
            <button class="btn small primary" id="verify" ${d.busy || (d.verifyRequested && !d.connected) ? "disabled" : ""}>${d.connected ? "Check drive now" : d.verifyRequested ? "Check queued" : "Queue check"}</button>
          </div></div>
        ${isCard(d) ? `<div class="go-row" style="margin-bottom:12px"><button class="btn ${safeToFormat(d) === "safe" ? "primary" : ""}" id="card-check">${ICON.shield}Safe to format?</button><span class="note">Every file, every copy, one answer; and the erase button when it is.</span></div>` : ""}
        ${safeToFormat(d) === "safe" ? `<div class="alert ok" style="margin-bottom:12px"><b>Safe to format.</b> Every file on this card has at least ${b.minCopies} verified copies on other drives${holders.length ? ": " + holders.map(([t]) => esc(t)).join(", ") : ""}.</div>`
          : safeToFormat(d) === "once" ? `<div class="alert warn" style="margin-bottom:12px"><b>Backed up once.</b> Every file has one verified copy elsewhere, so this card is still the second copy. Make another before formatting it.</div>` : ""}
        ${backupBar(b.enoughBytes + b.shortBytes, b.bytes, b.unknownBytes, b.likelyBytes, { size: "big" })}
        <div class="stat-row" style="margin-top:12px">
          <div class="stat ${missing ? "bad" : b.minCopies > 0 ? "ok" : ""}"><div class="v">${b.minCopies < 0 ? "—" : missing ? missing.toLocaleString() : b.minCopies + "×"}</div><div class="l">${b.minCopies < 0 ? "no checksums yet" : missing ? "files not backed up" : "fewest copies elsewhere"}</div></div>
          <div class="stat"><div class="v">${d.files ? Math.round((d.hashed / d.files) * 100) : 0}%</div><div class="l">${d.hashed.toLocaleString()} of ${d.files.toLocaleString()} checksummed</div></div>
          <div class="stat ${d.damaged ? "bad" : ""}"><div class="v">${d.damaged}</div><div class="l">damaged files</div></div>
          <div class="stat ${d.lastVerify ? (d.lastVerifyOk ? "ok" : "bad") : ""}"><div class="v">${d.lastVerify ? (d.lastVerifyOk ? "OK" : "Failed") : "Never"}</div><div class="l">${d.lastVerify ? "last full check " + new Date(d.lastVerify * 1000).toLocaleDateString() : d.verifyRequested ? "check queued for next mount" : "full check"}</div></div>
        </div>
        ${holders.length ? `<p class="hint" style="margin:10px 0 0;color:var(--text-2)">Copies on ${holders.map(([n, c]) => `<b>${esc(n)}</b> (${c.toLocaleString()} files)`).join(", ")}</p>` : ""}
        ${b.likely ? `<p class="hint" style="margin:6px 0 0;color:var(--muted)">${b.likely.toLocaleString()} files without checksums match a same-name, same-size file elsewhere (probable copies). ${d.connected ? "Checksum new files to prove them." : "Plug the drive in and checksum them to prove it."}</p>` : ""}
        ${b.problems && b.problems.length ? `<p class="hint" style="margin:12px 0 0;color:var(--text-2);font-weight:550">Needs attention${b.problems.length >= 200 ? " (first 200)" : ""}: not backed up${b.damaged ? ", or damaged on this drive" : ""}</p><div class="problems">${b.problems.map((p) => `<div>${esc(p)}</div>`).join("")}</div>` : ""}
      </div>

      <div class="sec"><div class="sec-title">${ICON.pie}Contents by folder <span class="hint" style="font-weight:400">which top-level folders have a copy elsewhere</span>
          <div class="right">${d.connected ? `<button class="btn small" id="probe" ${d.busy ? "disabled" : ""} title="Read codec, frame rate, timecode and camera from clips that have none yet">${ICON.clapper}Read metadata</button><button class="btn small" id="rescan" ${d.busy ? "disabled" : ""}>${ICON.refresh}Rescan</button>` : ""}</div></div>
        ${folders.length ? `<table class="list compact"><thead><tr><th>Folder</th><th class="num">Size</th><th style="width:45%">Backup</th></tr></thead><tbody>
          ${folders.map((f) => { const st = backupState(f.backedBytes, f.bytes, f.unknownBytes - (f.likelyBytes || 0), f.likelyBytes || 0); return `<tr><td><div class="name" style="font-weight:500"><span class="sdot ${st}"></span>${f.folder ? ICON.folder.replace("<svg", '<svg style="width:14px;height:14px;color:var(--muted)"') : ICON.file.replace("<svg", '<svg style="width:14px;height:14px;color:var(--muted)"')}${esc(f.name)}</div></td>
            <td class="num">${bytes(f.bytes)}</td><td>${backupBar(f.backedBytes, f.bytes, f.unknownBytes - (f.likelyBytes || 0), f.likelyBytes || 0, { size: "tiny" })}</td></tr>`; }).join("")}</tbody></table>`
          : `<span class="hint">No files recorded on this drive.</span>`}
      </div>

      <div class="sec"><div class="sec-title">${ICON.film}Projects</div>
        <div class="box">${(S.state.projects || []).length ? `<div class="checks">${S.state.projects.map((p) => `<label><input type="checkbox" data-project="${p.id}" ${myProjects.has(p.id) ? "checked" : ""}><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span>${esc(p.name)}<span class="hint">· whole drive</span></label>`).join("")}</div><p class="hint" style="margin:8px 0 0">To add one folder instead, use the + on a folder below.</p>`
          : `<span class="hint">No projects yet. Make one on the Projects tab to see how this drive and others back each other up.</span>`}</div></div>

      ${summaryText(d) ? `<div class="sec"><div class="sec-title">${ICON.search}Top-level summary</div><div style="color:var(--text-2)">${esc(summaryText(d))}</div></div>` : ""}

      <div class="sec" id="sec-proxies" hidden></div>

      <div class="sec"><div class="sec-title">${ICON.folder}Browse drive <span style="color:var(--muted);font-weight:400;font-size:12px">Indexed • ${d.files.toLocaleString()} files${d.lastScan ? " • " + when(d.lastScan) : ""}</span></div>
        <div class="legend-line"><span><i class="sdot full"></i>Everything inside has a verified duplicate on another drive</span><span><i class="sdot partial"></i>Some of it does</span><span><i class="sdot likely"></i>Probably: same name and size elsewhere, not checksummed yet</span><span><i class="sdot none"></i>None of it does, or nothing known</span></div>
        <div class="search"><span class="search-icon">${ICON.search}</span><input id="tree-filter" type="search" placeholder="Filter by name…"></div>
        <div class="tree" id="tree">${loadingGlyph("", true)}</div></div>

      <div class="sec"><div class="sec-title">${ICON.history}History</div>
        <div class="history">${data.events.map((e) => `<div><span class="when">${short(e.at)}</span><span class="kind">${esc(e.kind)}</span><span class="${e.failures ? "fail" : ""}">${esc(e.summary)}</span></div>`).join("") || '<span style="color:var(--muted)">Nothing yet.</span>'}</div></div>
    </div>`);
  if (scroll) $(".sheet-body").scrollTop = scroll;
  wireDetail(d);
  loadTree(d.id, "", $("#tree"), 0);
  loadProxies(d.id);
  if (focusLabels) setTimeout(() => { const el = $('[data-label="nickname"]'); el.scrollIntoView({ block: "center" }); el.focus(); }, 30);
}

function wireDetail(d) {
  const save = async (patch) => {
    try { await api(`/api/drives/${encodeURIComponent(d.id)}`, { method: "PATCH", body: { ...d.labels, ...patch } }); Object.assign(d.labels, patch); loadState(); }
    catch (e) { toast(e.message, true); }
  };
  $("#close").onclick = closeSheet;
  $("#never-log").onchange = (e) => save({ neverLog: e.target.checked });
  $("#no-backups").onchange = (e) => save({ noBackups: e.target.checked });
  if ($("#let-sleep")) $("#let-sleep").onchange = (e) => save({ letSleep: e.target.checked });
  for (const el of document.querySelectorAll("[data-project]")) el.onchange = async () => {
    try { await api(`/api/projects/${el.dataset.project}`, { method: "PATCH", body: el.checked ? { add: [{ driveId: d.id, folder: "" }] } : { remove: [{ driveId: d.id, folder: "" }] } }); loadState(); }
    catch (e) { toast(e.message, true); el.checked = !el.checked; }
  };
  for (const el of document.querySelectorAll("[data-label]")) el.onchange = () => save({ [el.dataset.label]: el.value });
  for (const el of document.querySelectorAll("[data-color]")) el.onclick = () => {
    document.querySelectorAll("[data-color]").forEach((x) => x.classList.toggle("on", x === el));
    save({ color: el.dataset.color });
  };
  $("#forget").onclick = () => {
    const body = $(".sheet-body");
    const box = document.createElement("div");
    box.className = "confirm";
    box.innerHTML = `<div><b>Remove ${esc(d.title)} from the library?</b><br>Its file list, checksums and history are deleted. The drive itself is not touched, and it is logged again next time it's plugged in unless Never log is on.</div>
      <div class="confirm-actions"><button class="btn" id="forget-no">Cancel</button><button class="btn primary danger-fill" id="forget-yes">Remove</button></div>`;
    body.prepend(box);
    body.scrollTop = 0;
    $("#forget-no").onclick = () => box.remove();
    $("#forget-yes").onclick = async () => {
      try { await api(`/api/drives/${encodeURIComponent(d.id)}`, { method: "DELETE" }); closeSheet(); toast(`${d.title} removed`); loadState(); }
      catch (e) { toast(e.message, true); }
    };
  };
  const act = (sel, path, msg) => { const el = $(sel); if (el) el.onclick = async () => {
    try { const r = await api(`/api/drives/${encodeURIComponent(d.id)}/${path}`, { method: "POST" });
      toast(r.queued ? `${d.title} isn't plugged in. The check will run next time it's mounted.` : msg); loadJobs(); loadState(); openDetail(d.id, true); }
    catch (e) { toast(e.message, true); } }; };
  act("#verify", "verify", `Checking ${d.title}… (see Activity)`);
  act("#hash", "hash", `Checksumming new files on ${d.title}…`);
  act("#rescan", "rescan", `Rescanning ${d.title}…`);
  act("#probe", "metadata", `Reading metadata on ${d.title}… (see Activity)`);
  act("#cancel-check", "cancel", "Queued check cancelled");
  const rv = $("#reveal");
  if (rv) rv.onclick = () => nativeCall("reveal", { path: d.mount });
  for (const b of document.querySelectorAll("[data-adopt]")) b.onclick = async () => {
    if (!confirm(`Carry ${b.dataset.adoptTitle}'s log over to ${d.title}? Its files, checksums, projects and history become this drive's, and the old entry goes. Only do this if it really is the same drive, renamed.`)) return;
    b.disabled = true;
    try { await api(`/api/drives/${encodeURIComponent(d.id)}/adopt`, { method: "POST", body: { from: b.dataset.adopt } }); toast(`${b.dataset.adoptTitle}'s log is now ${d.title}'s`); await loadState(); openDetail(d.id, true); }
    catch (e) { toast(e.message, true); b.disabled = false; }
  };
  const ej = $("#eject");
  if (ej) ej.onclick = () => { ej.disabled = true; ejectDrive(d.id); };
  const cc = $("#card-check"); if (cc) cc.onclick = () => openCard(d.id);
  const hr = $("#health-read");
  if (hr) hr.onclick = async () => {
    hr.disabled = true; hr.textContent = "Reading…";
    try { await api(`/api/drives/${encodeURIComponent(d.id)}/health`, { method: "POST" }); await loadState(); openDetail(d.id, true); }
    catch (e) { toast(e.message, true); hr.disabled = false; hr.textContent = "Read now"; }
  };
  let t;
  $("#tree-filter").oninput = (e) => { clearTimeout(t); t = setTimeout(() => loadTree(d.id, "", $("#tree"), 0, e.target.value.trim()), 200); };
}

function statusIcon(e) {
  if (e.damaged && !e.folder) return `<span class="bad" title="Failed its last check">${ICON.warn}</span>`;
  if (e.folder) return "";
  if (e.verified) return `<span class="ok" title="Read back and matched">${ICON.check}</span>`;
  if (e.xxh64) return `<span class="none" title="xxh64 ${esc(e.xxh64)}">${ICON.check}</span>`;
  return `<span class="none" title="No checksum yet">${ICON.dash}</span>`;
}
// entryDot is the backup dot for a folder (rolled up from its contents)
// or a file, with a short count for folders.
function entryDot(e) {
  const st = e.folder ? backupState(e.backed, e.files, e.unknown - (e.likely || 0), e.likely || 0) : (e.damaged ? "damaged" : e.copies > 0 ? "full" : e.unknown && e.likely ? "likely" : e.unknown ? "unknown" : "none");
  const title = e.folder
    ? (st === "full" ? "Every file inside has a copy on another drive" : st === "likely" ? "Every file inside has a verified copy or a same-name, same-size file on another drive; checksum to prove it" : st === "partial" ? `${e.backed.toLocaleString()} of ${e.files.toLocaleString()} files have a copy elsewhere${e.likely ? `, ${e.likely.toLocaleString()} probably (same name and size)` : ""}${e.unknown - (e.likely || 0) ? `, ${(e.unknown - (e.likely || 0)).toLocaleString()} unproven` : ""}` : st === "unknown" ? "No checksums yet" : "No file inside has a copy on another drive")
    : (st === "full" ? `Copy on ${e.copies} other drive${e.copies === 1 ? "" : "s"}` : st === "likely" ? "Probably backed up: a file with the same name and size is on another drive. Checksum to prove it" : st === "damaged" ? "Failed its last check" : st === "unknown" ? "No checksum yet" : "No copy on another drive");
  const dot = `<span class="sdot ${st === "unknown" ? "none" : st}" title="${esc(title)}"></span>`;
  const text = e.folder && e.files ? `<span class="bk ${st}">${st === "full" ? "all " + e.files.toLocaleString() : st === "likely" ? "~all " + e.files.toLocaleString() : st === "none" || st === "unknown" ? "0 of " + e.files.toLocaleString() : (e.backed + (e.likely ? "+" + e.likely + "?" : "")) + " of " + e.files.toLocaleString()}</span>` : `<span class="bk"></span>`;
  return [dot, text];
}

async function loadTree(id, dir, container, depth, filter) {
  let entries;
  try { entries = await api(`/api/drives/${encodeURIComponent(id)}/list?dir=${encodeURIComponent(dir)}&filter=${encodeURIComponent(filter || "")}`); }
  catch (e) { container.innerHTML = `<div class="tree-note">${esc(e.message)}</div>`; return; }
  if (!entries.length) { container.innerHTML = `<div class="tree-note">${filter ? "No files match." : "Empty."}</div>`; return; }
  container.innerHTML = entries.map((e) => { const [dot, bk] = entryDot(e); return `<div class="tree-row ${e.folder ? "folder" : ""}" data-path="${esc(e.path)}" tabindex="0" style="padding-left:${10 + depth * 18}px">
      <div class="nm">${e.folder ? ICON.chev : '<span style="width:12px"></span>'}${dot}${e.folder ? ICON.folder : ICON.file}<span title="${esc(e.path)}">${esc(e.name)}</span></div>
      ${bk}<span class="sz">${bytes(e.bytes)}${e.folder ? ` · ${e.files.toLocaleString()}` : ""}</span><span class="st">${e.folder ? `<button class="icon-btn mini" data-addsrc="${esc(e.path)}" title="Add this folder to a project">${ICON.plus}</button>` : statusIcon(e)}</span></div>
      ${e.folder ? `<div class="kids" hidden></div>` : ""}`; }).join("") + (entries.length >= 2000 ? `<div class="tree-note">Showing the first 2,000.</div>` : "");
  for (const b of container.querySelectorAll(":scope > .tree-row.folder [data-addsrc]")) b.onclick = (e) => {
    e.stopPropagation();
    addToProjectDialog([{ driveId: id, folder: b.dataset.addsrc }], () => openDetail(id, false));
  };
  for (const row of container.querySelectorAll(":scope > .tree-row:not(.folder)")) row.onclick = () => openFile(id, row.dataset.path, id);
  // Space on a file is quick look, as in the Finder; ↑ ↓ walk the rows.
  for (const row of container.querySelectorAll(":scope > .tree-row")) row.onkeydown = (e) => {
    if (e.key === " " || e.key === "Enter") { e.preventDefault(); if (row.classList.contains("folder")) row.click(); else openFile(id, row.dataset.path, id); }
    else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const rows = [...document.querySelectorAll("#sheet .tree-row")].filter((r) => r.offsetParent !== null), i = rows.indexOf(row), next = rows[i + (e.key === "ArrowDown" ? 1 : -1)];
      if (next) next.focus();
    }
  };
  for (const row of container.querySelectorAll(":scope > .tree-row.folder")) {
    row.onclick = () => {
      const kids = row.nextElementSibling;
      const open = row.classList.toggle("open");
      kids.hidden = !open;
      if (open && !kids.dataset.loaded) { kids.dataset.loaded = "1"; kids.innerHTML = `<div style="padding-left:${28 + depth * 18}px">${loadingGlyph("", true)}</div>`; loadTree(id, row.dataset.path, kids, depth + 1); }
    };
  }
}


// ---------- projects ----------
function driveTitle(id) {
  const d = (S.state.drives || []).find((x) => x.id === id);
  return d ? d.title : id;
}
function driveOf(id) { return (S.state.drives || []).find((x) => x.id === id); }
function sourceLabel(src) {
  return esc(driveTitle(src.driveId)) + (src.folder ? ` <span class="hint">›</span> <span class="mono">${esc(src.folder)}</span>` : ` <span class="hint">(whole drive)</span>`);
}

// The list needs each project's health, which costs a pass over its
// files, so it is fetched when the tab is shown rather than with every
// state poll.
async function loadProjects() {
  try { S.projects = await api("/api/projects"); } catch (e) { toast(e.message, true); return; }
  if (S.openProjectNamed && !S.jobs.some((j) => j.kind === "template" && j.state === "running")) {
    const p = [...S.projects].reverse().find((x) => x.name === S.openProjectNamed);
    S.openProjectNamed = null;
    if (p) { S.tab = "projects"; S.project = p.id; localSet("tab", S.tab); return render(); }
  }
  if (S.tab === "projects" && !S.project) renderProjects();
  if (S.tab === "map") renderMap();
}

function renderProjects() {
  if (S.project) return renderProjectPage();
  const main = $("#main");
  if (!S.projects) { main.innerHTML = loadingGlyph("Reading the library…"); loadProjects(); return; }
  const ps = S.projects;
  if (!ps.length) {
    main.innerHTML = `<div class="empty">${ICON.film}<h2>No projects yet</h2>
      <p>A project is the folders that hold one piece of work: a feature across five drives, or one music video that shares a shuttle with three other jobs. Lumpy shows how well it's backed up, wherever the copies are, and plans the copies that get every file onto two drives.</p>
      <p><button class="btn primary" id="find-projects">${ICON.search}Find projects on my drives…</button> <button class="btn" id="new-project">${ICON.plus}New project</button> <button class="btn" id="new-folder">${ICON.folder}New project folder…</button></p></div>`;
  } else {
    main.innerHTML = `<div class="page-bar"><div class="toolbar-title">${ICON.film}Projects<span class="count">${ps.length}</span></div><div class="right"><button class="btn" id="pull-shots" title="Hand Lumpy an edit and it finds every clip's camera original across all drives">${ICON.clapper}Pull shots from an edit…</button><button class="btn" id="find-projects" title="Look through every logged drive for project folders">${ICON.search}Find projects…</button><button class="btn" id="new-folder" title="Make a project's folders from your Post Haste template">${ICON.folder}New project folder…</button><button class="btn primary" id="new-project">${ICON.plus}New project</button></div></div>
      <div class="grid">${ps.map((p) => {
        const h = p.health;
        const bad = h.soleBytes + h.shortBytes;
        return `<article class="card pcard" style="--tint:${COLORS[p.color] || "transparent"}" data-open-project="${p.id}">
          <div class="card-top"><span class="n-tag" style="--pc:${COLORS[p.color] || "var(--accent)"}">${ICON.film}</span><div class="card-title"><h3>${esc(p.name)}</h3><span class="vol">${p.sources.length} folder${p.sources.length === 1 ? "" : "s"} · ${p.target} copies wanted</span></div></div>
          ${backupBar(h.backedBytes, h.bytes, h.unknownBytes, h.likelyBytes || 0, { compact: true })}
          <div class="cap">${bytes(h.bytes)}<span class="sep">•</span>${h.contents.toLocaleString()} files${bad ? `<span class="sep">•</span><span style="color:var(--warn)">${bytes(bad)} still to copy</span>` : ""}</div>
          <div class="badges">${h.damaged ? `<span class="badge bad">${h.damaged} damaged</span>` : ""}</div>
          <div class="card-foot"><span>${p.sources.map((src) => driveTitle(src.driveId) + (src.folder ? "/" + src.folder : "")).join(", ") || "No folders yet"}</span><button class="btn small">Open</button></div></article>`;
      }).join("")}</div>`;
  }
  $("#new-project").onclick = () => newProjectDialog([]);
  $("#find-projects").onclick = () => findProjectsDialog();
  if ($("#pull-shots")) $("#pull-shots").onclick = () => pullDialog();
  const nf = $("#new-folder");
  if (nf) nf.onclick = () => templateDialog();
}

async function loadProject(id, refresh) {
  try { [S.projectData, S.projectProxies] = await Promise.all([api(`/api/projects/${id}`), api(`/api/projects/${id}/proxies`).catch(() => [])]); }
  catch (e) { toast(e.message, true); S.project = null; S.projectData = null; return render(); }
  if (S.project === id) renderProjectPage();
}

function renderProjectPage() {
  const main = $("#main");
  closePlayers();
  const data = S.projectData;
  if (!data || data.project.id !== S.project) {
    main.innerHTML = loadingGlyph("Reading the project…");
    loadProject(S.project);
    return;
  }
  const keepScroll = main.scrollTop;
  const p = data.project, h = data.health;
  const bad = h.soleBytes + h.shortBytes;
  const connected = (id) => { const d = driveOf(id); return d && d.connected; };
  const dest = new Set(p.destIds);
  const anyDest = p.destIds.length === 0;
  // One block per drive (or folder) in the project, its folders listed
  // underneath, largest first.
  const groupRows = (gs, color, extra) => {
    const bySrc = new Map();
    for (const g of gs) { if (!bySrc.has(g.source)) bySrc.set(g.source, []); bySrc.get(g.source).push(g); }
    return [...bySrc.entries()].map(([si, list]) => ({ si, list, files: list.reduce((a, g) => a + g.files, 0), total: list.reduce((a, g) => a + g.bytes, 0) }))
      .sort((a, b) => b.total - a.total)
      .map(({ si, list, files, total }) => {
        const src = p.sources[si];
        return `<tr class="grp"><td colspan="2"><div class="name" style="font-weight:600"><span class="dot" style="background:${connected(src.driveId) ? "var(--ok)" : "var(--muted)"}"></span><span>${sourceLabel(src)}</span><small>${list.length} folder${list.length === 1 ? "" : "s"}</small></div></td><td class="num">${files.toLocaleString()}</td><td class="num" style="color:${color};font-weight:600">${bytes(total)}</td>${extra ? extra(src) : ""}</tr>`
          + list.map((g) => `<tr class="sub"><td></td><td class="mono">${g.folder === "." ? '<span class="hint">(top level)</span>' : esc(g.folder)}</td><td class="num">${g.files.toLocaleString()}</td><td class="num">${bytes(g.bytes)}</td>${extra ? "<td></td>" : ""}</tr>`).join("");
      }).join("");
  };

  main.innerHTML = `
    <div class="phead page-bar">
      <button class="icon-btn" id="proj-back" title="All projects">${ICON.back}</button>
      <span class="n-tag" style="--pc:${COLORS[p.color] || "var(--accent)"}">${ICON.film}</span>
      <input class="inline-input" id="proj-name" value="${esc(p.name)}" title="Rename">
      <label class="hint">Copies wanted <select class="plain" id="proj-target">${[1, 2, 3, 4].map((n) => `<option value="${n}" ${p.target === n ? "selected" : ""}>${n}</option>`).join("")}</select></label>
      <div class="swatches">${Object.entries(COLORS).map(([k, c]) => `<button class="swatch ${p.color === k ? "on" : ""}" data-pcolor="${k}" style="background:${c}" title="${k}"></button>`).join("")}</div>
      <div class="right"><button class="btn" id="proj-consolidate" title="Plan a project that grew across many drives into one primary and one mirror per shooting day">${ICON.drive}Consolidate…</button><button class="btn" id="proj-tidy" title="Sort this project's loose files into the standard folders, then relink in Resolve">${ICON.folder}Tidy up…</button><button class="btn" id="proj-media" title="Every clip in this project with its codec, frame rate, timecode and log notes">${ICON.clapper}Clips</button><button class="btn" id="proj-media-csv" title="Clip list with metadata, for post">${ICON.export}Clips CSV…</button><button class="btn danger" id="proj-delete">Delete project</button></div>
    </div>

    ${masterSection(data)}
    <div class="pgrid">
    <div>
      <section class="panel">
        <h2>Health</h2>
        <p class="hint">Every distinct file in this project should be on at least ${p.target} different drive${p.target === 1 ? "" : "s"}. Copies anywhere in the library count. Files are matched by checksum, so the same clip in two folders counts once.</p>
        ${h.bytes ? backupBar(h.backedBytes, h.bytes, h.unknownBytes, h.likelyBytes || 0, { size: "big" }) : `<p class="hint">${p.sources.length ? "No files recorded in these folders yet." : "Add a folder or a drive on the right to get started."}</p>`}
        <div class="stat-row" style="margin-top:14px">
          <div class="stat ${h.bytes && !bad ? "ok" : ""}"><div class="v">${h.bytes ? Math.round((h.backedBytes / h.bytes) * 100) : 0}%</div><div class="l">backed up (${bytes(h.backedBytes)}, ${h.backed.toLocaleString()} files)</div></div>
          <div class="stat ${h.soleBytes ? "bad" : "dim"}"><div class="v">${bytes(h.soleBytes)}</div><div class="l">sole copies (${h.sole.toLocaleString()} files)</div></div>
          ${p.target > 2 ? `<div class="stat ${h.shortBytes ? "warn" : "dim"}"><div class="v">${bytes(h.shortBytes)}</div><div class="l">fewer than ${p.target} copies (${h.short.toLocaleString()} files)</div></div>` : ""}
          ${h.likelyBytes ? `<div class="stat warn" title="No checksum yet, but a file with the same name and size is on enough drives. Checksum to prove it."><div class="v">${bytes(h.likelyBytes)}</div><div class="l">probably backed up, by name (${h.likely.toLocaleString()} files)</div></div>` : ""}
          <div class="stat ${h.unknownBytes ? "warn" : "dim"}"><div class="v">${bytes(h.unknownBytes)}</div><div class="l">no checksum yet (${h.unknown.toLocaleString()} files)</div></div>
          <div class="stat ${h.damaged || h.lost ? "bad" : "dim"}"><div class="v">${h.damaged}</div><div class="l">damaged copies${h.lost ? ` · ${h.lost} with no good copy left` : ""}</div></div>
        </div>
      </section>

      <section class="panel">
        <h2>Plan to reach ${p.target} copies</h2>
        <p class="hint">Each step copies only the files that need another copy, from one drive to another. Plug both in and run it. Where the project already has a folder on the destination drive, copies go into it at the same relative paths; otherwise into a folder named after the project. ${bytes(data.reserve || 0)} is kept free on every drive.</p>
        ${data.transfers.length === 0 && !data.unplacedBytes ? `<div class="alert ok">${h.bytes ? `<b>Nothing to do.</b> Every checksummed file is on ${p.target} drives.` : "<b>Nothing to plan yet.</b>"}</div>` : ""}
        ${data.transfers.map((t) => {
          const ready = t.ready && !t.running && !t.busy;
          const need = [t.sourceId, t.destId].filter((id) => !connected(id)).map(driveTitle);
          return `<div class="step-card" data-transfer="${esc(t.sourceId)}|${esc(t.destId)}">
            <div class="step-head"><span class="dot ${connected(t.sourceId) ? "live" : ""}" style="background:${connected(t.sourceId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.sourceId))}</b><span class="arrow">${ICON.arrow}</span>
              <span class="dot ${connected(t.destId) ? "live" : ""}" style="background:${connected(t.destId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.destId))}</b>
              <span class="hint">${bytes(t.bytes)} · ${t.fileCount.toLocaleString()} file${t.fileCount === 1 ? "" : "s"}</span>
              <div class="right">${t.running ? `<span class="badge info">${ICON.clock}Running</span>` : t.busy ? `<span class="badge">${ICON.clock}Drive busy</span>` : ready ? `<button class="btn go small" data-run="${esc(t.sourceId)}|${esc(t.destId)}" data-into="${esc(t.folder || "")}">${ICON.play}Run this step</button>` : `<span class="badge">${ICON.plug}Plug in ${esc(need.join(" and "))}</span>`}</div></div>
            <div class="chips">${t.folderList.slice(0, 8).map((f) => `<span class="chip">${esc(f.name === "." ? "(top level)" : f.name)}<small>${bytes(f.bytes)}</small></span>`).join("")}${t.folderList.length > 8 ? `<span class="chip">+${t.folderList.length - 8} more</span>` : ""}</div>
            ${t.folder ? `<div class="step-run"><span class="hint">Into</span><b>${esc(driveTitle(t.destId))} › ${esc(t.folder)}</b><span class="hint">· same paths as on the source</span></div>`
              : ready ? `<div class="step-run"><span class="hint">Copy into folder</span><input class="path-input" data-folder="${esc(t.sourceId)}|${esc(t.destId)}" value="${esc(p.name)}" spellcheck="false"><span class="hint">on ${esc(driveTitle(t.destId))}</span></div>` : ""}
          </div>`;
        }).join("")}
        ${data.unplacedBytes ? `<div class="alert bad" style="margin-top:10px"><b>${bytes(data.unplacedBytes)} has nowhere to go.</b> No ${anyDest ? "drive in the project" : "chosen destination"} has room for these files (each needs a drive that doesn't already hold it). ${anyDest ? "Log a drive" : "Allow a drive"} with at least ${bytes(data.unplacedBytes + data.reserve)} free, or free up space.
          <div class="chips">${data.unplaced.map((g) => `<span class="chip">${driveTitle(p.sources[g.source].driveId)} / ${esc(g.folder === "." ? "(top level)" : g.folder)}<small>${bytes(g.bytes)}</small></span>`).join("")}</div></div>` : ""}
      </section>

      ${proxiesPanel(S.projectProxies || [])}

      ${data.sole.length ? `<section class="panel"><h2>Files with too few copies</h2><p class="hint">Where the ${bytes(bad)} that isn't backed up enough lives, by folder.</p>
        <table class="list compact grouped"><thead><tr><th colspan="2">Drive · folder</th><th class="num">Files</th><th class="num">Size</th></tr></thead><tbody>${groupRows(data.sole, "var(--bad)")}</tbody></table></section>` : ""}

      ${archivePanel(p)}

      ${data.unknown.length ? `<section class="panel"><h2>Files with no checksum</h2><p class="hint">These can't be counted as copies of anything until they're checksummed. Plug the drive in and checksum its new files.</p>
        <table class="list compact grouped"><thead><tr><th colspan="2">Drive · folder</th><th class="num">Files</th><th class="num">Size</th><th></th></tr></thead><tbody>${groupRows(data.unknown, "var(--warn)", (src) => { const id = src.driveId; return `<td class="num">${connected(id) ? `<button class="btn small" data-hash="${esc(id)}" ${driveOf(id).busy ? "disabled" : ""}>Checksum now</button>` : `<span class="hint">not plugged in</span>`}</td>`; })}</tbody></table></section>` : ""}
    </div>

    <div>
      <section class="panel">
        <h2>What's in this project</h2>
        <p class="hint">${p.sources.length ? `${p.sources.length} folder${p.sources.length === 1 ? "" : "s"}. Each bar shows how much of the folder is backed up elsewhere, a sole copy, or unproven.` : "Add the folders that hold this project. A whole drive counts as one folder."}</p>
        ${h.sources.map((sh, i) => {
          const d = driveOf(sh.driveId);
          const src = p.sources[i] || sh;
          const backup = src.role === "backup";
          const routes = (data.routes || [])[i] || [];
          const others = p.sources.filter((o) => o.driveId !== src.driveId);
          const srcName = (o) => `${driveTitle(o.driveId)}${o.folder ? " › " + o.folder : ""}`;
          const intoKey = src.into ? `${src.into.driveId}|${src.into.folder || ""}` : "";
          const inferred = routes.filter((r) => !r.explicit).map((r) => `${driveTitle(r.destId)} › ${r.folder || "(top level)"}`);
          return `<div class="step-card ${backup ? "backup" : ""}" style="margin-bottom:10px">
            <div class="step-head"><span class="dot ${d && d.connected ? "live" : ""}" style="background:${d && d.connected ? "var(--ok)" : (d && COLORS[d.labels.color]) || "var(--muted)"}"></span><b style="min-width:0;overflow:hidden;text-overflow:ellipsis">${sourceLabel(sh)}</b>${backup ? `<span class="badge info">${ICON.shield}Backup</span>` : ""}
              <div class="right"><button class="btn small" data-drive-open="${esc(sh.driveId)}">Open drive</button><button class="icon-btn" data-remove-src="${i}" title="Remove from project">${ICON.x}</button></div></div>
            ${backupBar(sh.backedBytes, sh.bytes, sh.unknownBytes, sh.likelyBytes || 0, { compact: true })}
            <div class="cap">${bytes(sh.bytes)} · ${sh.files.toLocaleString()} files${sh.soleBytes + sh.shortBytes ? `<span class="sep">•</span><span style="color:var(--warn)">${bytes(sh.soleBytes + sh.shortBytes)} still to copy</span>` : ""}${sh.extraBytes ? `<span class="sep">•</span><span class="hint">${bytes(sh.extraBytes)} on more drives than needed</span>` : ""}${sh.orphanBytes ? `<span class="sep">•</span><span class="hint" title="Files only in this backup folder: nothing in the project's own folders matches them, so they are left alone">${bytes(sh.orphanBytes)} only here</span>` : ""}${sh.excludedBytes ? `<span class="sep">•</span><span class="hint">${bytes(sh.excludedBytes)} excluded</span>` : ""}${d && !d.connected ? `<span class="sep">•</span><span class="hint">last seen ${new Date(d.lastSeen * 1000).toLocaleDateString()}</span>` : ""}</div>
            <div class="src-ctl">
              <label class="switch" style="white-space:nowrap" title="A folder that exists to hold copies: the plan copies only onto backup drives, and files found only here are left alone">Backup drive <input type="checkbox" data-role="${i}" ${backup ? "checked" : ""}></label>
              ${!backup && others.length ? `<label class="select"><span class="hint" style="white-space:nowrap">Copies go into</span><select class="plain" data-into="${i}">
                <option value="" ${intoKey ? "" : "selected"}>${inferred.length ? "Worked out: " + esc(inferred.join(", ")) : "A folder named after the project"}</option>
                ${others.map((o) => `<option value="${esc(o.driveId)}|${esc(o.folder || "")}" ${intoKey === `${o.driveId}|${o.folder || ""}` ? "selected" : ""}>${esc(srcName(o))} · same paths</option>`).join("")}
                </select></label>` : ""}
              <span class="chips">${(src.exclude || []).map((ex) => `<span class="chip">${esc(ex)}<button class="chip-x" data-unex="${i}|${esc(ex)}" title="Include it again">×</button></span>`).join("")}<button class="chip add" data-ex="${i}">${ICON.plus}Leave a folder out…</button></span>
            </div></div>`;
        }).join("")}
        <button class="btn" id="proj-add-src">${ICON.plus}Add a folder or drive…</button>
      </section>

      <section class="panel">
        <h2>Back up onto</h2>
        <p class="hint">Copies stay inside the project: on its backup drives, or on its own drives, or only the drives ticked here. Nothing is copied onto a drive outside the project. Lumpy never copies a file onto a drive that already holds it.${(data.backupDrives || []).length ? ` <b>This project has backup drives, so copies go only onto ${esc((data.backupDrives || []).map(driveTitle).join(", "))}.</b>` : ""}</p>
        <label class="check-row"><input type="radio" name="dest-mode" value="any" ${anyDest ? "checked" : ""}><span>The project's own drives (backup drives first)</span></label>
        <label class="check-row"><input type="radio" name="dest-mode" value="only" ${anyDest ? "" : "checked"}><span>Only these drives</span></label>
        <div class="checks" id="dest-list" style="margin-top:8px;${anyDest ? "opacity:.55" : ""}">${(S.state.drives || []).map((d) => `<label><input type="checkbox" data-dest="${esc(d.id)}" ${dest.has(d.id) ? "checked" : ""} ${anyDest ? "disabled" : ""}><span class="dot" style="background:${d.connected ? "var(--ok)" : COLORS[d.labels.color] || "var(--muted)"}"></span>${esc(d.title)}<span class="hint">${bytes(d.free)} free${d.connected ? " · plugged in" : ""}${d.labels.noBackups ? " · marked “no backups here”" : ""}</span></label>`).join("")}</div>
        <p class="hint" style="margin:8px 0 0">Drives marked “no backups here” in their details (camera cards, loaners) are skipped unless ticked above.</p>
      </section>
      ${cardsSection(p)}
      <section class="panel"><h2>Notes</h2><textarea class="path-input" id="proj-notes" rows="4" placeholder="Where the drives live, who has what, what's still to come…">${esc(p.notes)}</textarea></section>
    </div>
    </div>`;
  main.scrollTop = keepScroll;
  wireProjectPage(p);
}

// tidyDialog sorts a project's loose files into a template's folders:
// pick the template, look at the plan as groups, adjust where each
// goes, move, then relink in Resolve.
// consolidateDialog is the plan for a project spread across drives:
// the shooting days in order (matched by content, however their
// folders are named), cut into consecutive ranges, each on one
// primary drive and one mirror; what to copy to get there, and what
// becomes redundant once both copies verify. A day's folder can be
// renamed on every drive at once, and the pairs are the user's to
// change.
async function consolidateDialog(p) {
  openSheet(`<div class="sheet-head"><div><h2>Consolidate ${esc(p.name)}</h2><div class="sub">The days in order, one primary and one mirror per range</div></div><div class="actions"><button class="icon-btn" id="cs-close">${ICON.x}</button></div></div><div class="sheet-body" id="cs-body">${loadingGlyph("Reading every copy of every file…")}</div>`);
  $("#sheet").classList.add("wide");
  $("#cs-close").onclick = closeSheet;
  let plan, layout = null, editing = null;
  const load = async () => {
    try { plan = layout ? await api(`/api/projects/${p.id}/consolidate`, { method: "POST", body: layout }) : await api(`/api/projects/${p.id}/consolidate`); }
    catch (e) { closeSheet(); toast(e.message, true); return false; }
    return !$("#sheet").hidden;
  };
  if (!(await load())) return;
  const day = (t) => t ? new Date(t * 1000).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }) : "";
  const draw = () => {
    const body = $("#cs-body");
    if (!body) return;
    const dn = (id) => { const d = plan.drives.find((x) => x.id === id); return d ? d.title : id ? driveTitle(id) : "—"; };
    const drives = plan.drives;
    const allDrives = (S.state.drives || []).filter((d) => !d.noBackups).sort((a, b) => a.title.localeCompare(b.title));
    const pct = (f) => f >= 0.999 ? "●" : f > 0 ? Math.round(f * 100) + "%" : "";
    const cell = (st, d) => {
      const f = st.copies[d.id] || 0, role = st.primary === d.id ? "P" : st.mirror === d.id ? "M" : "";
      const cls = f >= 0.999 ? "full" : f > 0 ? "part" : "none";
      const after = role ? "keep" : f > 0 ? "clear" : "";
      return `<td class="cs-cell ${cls} ${after}" title="${esc(st.name)} on ${esc(d.title)}: ${f ? Math.round(f * 100) + "% here" : "nothing here"}${role === "P" ? " · primary" : role === "M" ? " · mirror" : f > 0 ? " · redundant once both copies verify" : ""}">${pct(f)}${role ? `<b>${role}</b>` : ""}</td>`;
    };
    const tags = (st) => st.status === "sole" ? `<span class="vd critical">sole copy</span>` : st.status === "partial" ? `<span class="vd check">partial</span>` : `<span class="vd safe">backed up</span>`;
    const totalCopy = plan.transfers.reduce((a, t) => a + t.bytes, 0);
    const totalClear = plan.sets.reduce((a, st) => a + st.clears.reduce((b, c) => b + c.bytes, 0), 0);
    const sel = (cur, i, kind) => `<label class="select"><select class="plain" data-cs-pair="${i}" data-cs-kind="${kind}"><option value="">—</option>${allDrives.map((d) => `<option value="${esc(d.id)}" ${d.id === cur ? "selected" : ""}>${esc(d.title)}${d.connected ? "" : " (away)"}</option>`).join("")}</select></label>`;
    const thru = (cur, i) => `<label class="select"><select class="plain" data-cs-pair="${i}" data-cs-kind="through">${plan.sets.map((st) => `<option value="${esc(st.name)}" ${st.name === cur ? "selected" : ""}>${esc(st.name)}</option>`).join("")}</select></label>`;
    const nameCell = (st) => editing === st.name
      ? `<span class="cs-edit"><input class="path-input" data-cs-name="${esc(st.name)}" value="${esc(st.name)}" placeholder="reel43"><button class="btn small primary" data-cs-save="${esc(st.name)}">Rename on every drive</button><button class="btn small" data-cs-cancel>Cancel</button></span>`
      : `<b>${esc(st.name)}</b><button class="icon-btn cs-pen" data-cs-rename="${esc(st.name)}" title="Rename this day's folder on every drive that holds it">${ICON.pencil || ICON.edit || "✎"}</button>${st.aliases.length ? `<small class="hint">also ${st.aliases.map(esc).join(", ")}</small>` : ""} ${tags(st)}`;
    let rows = "", lastPair = null;
    for (const st of plan.sets) {
      if (st.pair !== lastPair) {
        lastPair = st.pair;
        const pr = plan.pairs[st.pair];
        rows += `<tr class="cs-band"><td colspan="${3 + drives.length}">${pr ? `Pair ${st.pair + 1} · <b>${esc(dn(pr.primary))}</b> mirrored on <b>${esc(dn(pr.mirror))}</b> · ${esc(pr.first)}${pr.last !== pr.first ? ` to ${esc(pr.last)}` : ""} · ${bytes(pr.bytes)}${!pr.mirror ? ` <span class="vd critical">no drive has room to mirror this</span>` : ""}` : `<span class="vd critical">After the last pair: not placed</span>`}</td></tr>`;
      }
      rows += `<tr><td class="cs-name">${nameCell(st)}</td><td class="num cs-when">${day(st.first)}${st.last && day(st.last) !== day(st.first) ? `<small class="hint">to ${day(st.last)}</small>` : ""}</td><td class="num">${bytes(st.bytes)}<small class="hint">${st.files.toLocaleString()} files</small></td>${drives.map((d) => cell(st, d)).join("")}</tr>`;
    }
    body.innerHTML = `
      <p class="hint">Each row is a shooting day or card, matched by file content across every drive in the project, whatever its folder is called; the pencil renames a day's folder on every drive that holds it, so the names fall into order (reel01, reel02…). The days run in order through the pairs: each pair is one primary drive and its one mirror, holding a consecutive range. <b>P</b> is where a day stays, <b>M</b> its mirror. Nothing is copied or cleared from here yet.</p>
      <div class="cs-sum">
        <span class="pill">Days:<b>${plan.sets.length}</b></span>
        <span class="pill" style="${plan.soleBytes ? "color:var(--bad)" : ""}">Sole copies:<b>${bytes(plan.soleBytes)}</b></span>
        <span class="pill" style="${plan.partialBytes ? "color:var(--warn)" : ""}">Partly mirrored:<b>${bytes(plan.partialBytes)}</b></span>
        <span class="pill">To copy:<b>${bytes(totalCopy)}</b></span>
        <span class="pill">Redundant after:<b>${bytes(totalClear)}</b></span>
        ${plan.buy ? `<span class="pill" style="color:var(--bad)">No room to mirror:<b>${bytes(plan.buy)}</b> · buy a drive</span>` : ""}
      </div>
      ${plan.offline.length ? `<p class="hint">${plan.offline.length} drive${plan.offline.length === 1 ? "" : "s"} in this project ${plan.offline.length === 1 ? "is" : "are"} not in the library and ${plan.offline.length === 1 ? "is" : "are"} left out.</p>` : ""}
      <div class="sec"><div class="sec-title">${ICON.drive}Pairs <span class="hint" style="font-weight:400">· ${plan.proposed ? "Lumpy's proposal; change any of it" : "yours"}</span></div>
        <div class="box cs-pairs">${plan.pairs.map((pr, i) => `<div class="cs-pair"><span class="cs-role">${i + 1}</span>${sel(pr.primary, i, "primary")}<span class="hint">mirrored on</span>${sel(pr.mirror, i, "mirror")}<span class="hint">through</span>${i === plan.pairs.length - 1 ? `<span class="hint">the rest</span>` : thru(pr.through, i)}<button class="icon-btn" data-cs-drop="${i}" title="Remove this pair">${ICON.x}</button></div>`).join("")}
          <div class="go-row"><button class="btn small" id="cs-add">${ICON.plus}Another pair</button><button class="btn small primary" id="cs-apply">Recalculate</button>${!plan.proposed ? `<button class="btn small" id="cs-reset">Back to Lumpy's proposal</button>` : ""}</div></div></div>
      <div class="sec"><div class="sec-title">${ICON.film}Days across drives</div>
      <div class="cs-scroll"><table class="list cs-grid"><thead><tr><th>Day</th><th class="num">Shot</th><th class="num">Size</th>${drives.map((d) => `<th class="cs-dh" title="${esc(d.title)}"><span class="dot" style="background:${d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(d.title)}</th>`).join("")}</tr></thead><tbody>${rows}</tbody></table></div></div>
      <div class="sec"><div class="sec-title">${ICON.drive}Each drive, before and after</div>
      <table class="list cs-drives"><thead><tr><th>Drive</th><th>Holds</th><th class="num">Of this project</th><th class="num">Free now</th><th class="num">Copy in</th><th class="num">Clear out</th><th class="num">Free after</th></tr></thead>
        <tbody>${drives.map((d) => `<tr><td><b>${esc(d.title)}</b><small class="hint">${bytes(d.size)}${d.connected ? " · plugged in" : ""}</small></td><td>${d.primaryOf.length ? `<span class="cs-role">P</span> ${d.primaryOf.length > 3 ? `${esc(d.primaryOf[0])} to ${esc(d.primaryOf[d.primaryOf.length - 1])} (${d.primaryOf.length})` : d.primaryOf.map(esc).join(", ")}` : ""}${d.primaryOf.length && d.mirrorOf.length ? "<br>" : ""}${d.mirrorOf.length ? `<span class="cs-role m">M</span> ${d.mirrorOf.length > 3 ? `${esc(d.mirrorOf[0])} to ${esc(d.mirrorOf[d.mirrorOf.length - 1])} (${d.mirrorOf.length})` : d.mirrorOf.map(esc).join(", ")}` : ""}${!d.primaryOf.length && !d.mirrorOf.length ? `<span class="hint">redundant copies only</span>` : ""}</td><td class="num">${bytes(d.nowBytes)}</td><td class="num">${bytes(d.free)}</td><td class="num">${d.copyIn ? "+" + bytes(d.copyIn) : ""}</td><td class="num">${d.clearOut ? "−" + bytes(d.clearOut) : ""}</td><td class="num" style="${d.freeAfter < 0 ? "color:var(--bad)" : ""}">${bytes(Math.max(0, d.freeAfter))}</td></tr>`).join("")}</tbody></table></div>
      <div class="sec"><div class="sec-title">${ICON.copy}Copies to make <span class="hint" style="font-weight:400">· sole copies first, as Lumpy offloads with verification</span></div>
      ${plan.transfers.length ? `<table class="list"><thead><tr><th>Day</th><th>From</th><th>To</th><th class="num">Files</th><th class="num">Size</th></tr></thead><tbody>${plan.transfers.map((t) => `<tr><td><b>${esc(t.set)}</b></td><td>${esc(dn(t.from))}</td><td>${esc(dn(t.to))}</td><td class="num">${t.files.toLocaleString()}</td><td class="num">${bytes(t.bytes)}</td></tr>`).join("")}</tbody></table>` : `<p class="hint">Nothing to copy: every day is on its primary and its mirror.</p>`}</div>
      <div class="sec"><div class="sec-title">${ICON.trash}Redundant once both copies verify</div>
      ${totalClear ? `<table class="list"><thead><tr><th>Day</th><th>On</th><th class="num">Size</th></tr></thead><tbody>${plan.sets.flatMap((st) => st.clears.map((c) => `<tr><td><b>${esc(st.name)}</b></td><td>${esc(dn(c.driveId))}</td><td class="num">${bytes(c.bytes)}</td></tr>`)).join("")}</tbody></table><p class="hint">These are the third and partial copies. They stay until the day's primary and mirror are both verified, and go through the Trash when you say so.</p>` : `<p class="hint">No redundant copies.</p>`}</div>`;
    const readPairs = () => plan.pairs.map((pr, i) => ({ primary: (body.querySelector(`[data-cs-pair="${i}"][data-cs-kind=primary]`) || {}).value || "", mirror: (body.querySelector(`[data-cs-pair="${i}"][data-cs-kind=mirror]`) || {}).value || "", through: (body.querySelector(`[data-cs-pair="${i}"][data-cs-kind=through]`) || {}).value || "" }));
    const recalc = async (pairs) => { layout = { pairs }; if (await load()) draw(); };
    $("#cs-apply").onclick = () => recalc(readPairs());
    $("#cs-add").onclick = () => { const ps = readPairs(); const last = ps[ps.length - 1]; if (last && !last.through) last.through = plan.pairs[ps.length - 1].last || ""; ps.push({ primary: "", mirror: "", through: "" }); recalc(ps); };
    for (const b of body.querySelectorAll("[data-cs-drop]")) b.onclick = () => { const ps = readPairs(); ps.splice(+b.dataset.csDrop, 1); if (ps.length) recalc(ps); else { layout = null; load().then((ok) => ok && draw()); } };
    const reset = $("#cs-reset"); if (reset) reset.onclick = () => { layout = null; load().then((ok) => ok && draw()); };
    for (const b of body.querySelectorAll("[data-cs-rename]")) b.onclick = () => { editing = b.dataset.csRename; draw(); const inp = body.querySelector("[data-cs-name]"); if (inp) { inp.focus(); inp.select(); } };
    for (const b of body.querySelectorAll("[data-cs-cancel]")) b.onclick = () => { editing = null; draw(); };
    for (const b of body.querySelectorAll("[data-cs-save]")) b.onclick = async () => {
      const from = b.dataset.csSave, to = (body.querySelector("[data-cs-name]") || {}).value.trim();
      if (!to || to === from) { editing = null; draw(); return; }
      const st = plan.sets.find((x) => x.name === from);
      b.disabled = true;
      try {
        let renamed = [];
        for (const name of [from, ...(st ? st.aliases : [])]) { const r = await api(`/api/projects/${p.id}/consolidate/rename`, { method: "POST", body: { from: name, to } }); renamed = renamed.concat(r.renamed || []); for (const sk of r.skipped || []) toast(sk, true); }
        toast(`${from} is now ${to} on ${renamed.join(", ") || "no drive"}`);
        editing = null;
        // The renamed drives rescan; the plan is read once they have.
        for (let i = 0; i < 240; i++) { const js = await api("/api/jobs").catch(() => []); if (!js.some((j) => j.kind === "rescan" && (j.state === "running" || j.state === "queued"))) break; await new Promise((r) => setTimeout(r, 500)); }
        if (await load()) draw();
      } catch (e) { toast(e.message, true); b.disabled = false; }
    };
    const inp = body.querySelector("[data-cs-name]"); if (inp) inp.onkeydown = (e) => { if (e.key === "Enter") body.querySelector("[data-cs-save]").click(); else if (e.key === "Escape") { editing = null; draw(); } };
  };
  draw();
}
async function tidyDialog(p) {
  let data;
  try { data = await api("/api/templates"); } catch (e) { return toast(e.message, true); }
  const t = { template: data.settings.template || (data.templates[0] || {}).path || "", plan: null, busy: false, job: null, done: false };
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  const draw = () => {
    const tpl = data.templates.find((x) => x.path === t.template);
    const plan = t.plan;
    const moving = plan ? plan.groups.filter((g) => !g.inPlace && g.dest).reduce((a, g) => a + g.files, 0) : 0;
    openSheet(`<div class="sheet-head"><div><h2>Tidy up ${esc(p.name)}</h2><div class="sub">Loose files into the standard folders</div></div><div class="actions"><button class="icon-btn" id="td-close">${ICON.x}</button></div></div>
      <div class="sheet-body">
        <p class="hint" style="margin:10px 0 4px">Files move on their own drive, so it is instant and nothing is copied. A log of every old and new path is written beside them, and the library follows.</p>
        ${t.done ? `<div class="alert ok"><b>Done.</b> ${esc(t.done)} Resolve can now be told where everything went.</div>` : ""}
        <div class="sec"><div class="sec-title">${ICON.folder}Template</div><div class="box"><div class="path-row">
          <label class="select" style="flex:1"><select class="plain" id="td-template">${data.templates.map((x) => `<option value="${esc(x.path)}" ${x.path === t.template ? "selected" : ""}>${esc(x.name)}</option>`).join("")}${t.template && !tpl ? `<option value="${esc(t.template)}" selected>${esc(t.template.split("/").pop())}</option>` : ""}</select></label>
          <button class="btn" id="td-pick">Choose…</button><button class="btn primary" id="td-look" ${t.template && !t.busy ? "" : "disabled"}>${t.busy ? "Looking…" : "Look"}</button></div>
          ${tpl ? `<p class="hint" style="margin:8px 0 0">${tpl.folders.map((f) => `<span class="chip">${esc(f)}</span>`).join(" ")}</p>` : ""}</div></div>
        ${plan ? `<div class="sec"><div class="sec-title">${ICON.copy}The plan<span class="count">${plan.files.toLocaleString()} files</span></div><div class="box">
          ${plan.offline.length ? `<div class="alert warn" style="margin-bottom:10px">Not plugged in, so left out: ${esc(plan.offline.join(", "))}</div>` : ""}
          <p class="hint" style="margin:0 0 10px"><b>${moving.toLocaleString()}</b> to move · <b>${plan.inPlace.toLocaleString()}</b> already where they belong · <b>${plan.groups.filter((g) => !g.inPlace && !g.dest).reduce((a, g) => a + g.files, 0).toLocaleString()}</b> left alone. Change where any group goes.</p>
          <table class="list compact tidy"><thead><tr><th>From</th><th>What</th><th class="num">Files</th><th class="num">Size</th><th>To</th></tr></thead><tbody>
          ${plan.groups.map((g, i) => `<tr class="${g.inPlace ? "dim" : ""}"><td><div class="name"><span class="dot" style="background:var(--ok)"></span><span>${esc(g.drive)}${g.source ? "/" + esc(g.source) : ""}<small>${g.top ? esc(g.top) + "/" : "(top level)"}</small></span></div></td>
            <td>${esc(g.label)}<small class="hint" style="display:block">${g.samples.map(esc).join(", ")}${g.files > 3 ? "…" : ""}</small></td><td class="num">${g.files.toLocaleString()}</td><td class="num">${bytes(g.bytes)}</td>
            <td>${g.inPlace ? '<span class="ok">already there</span>' : `<label class="select"><select class="plain" data-td-dest="${i}"><option value="">Leave as is</option>${plan.folders.map((f) => `<option value="${esc(f)}" ${f === g.dest ? "selected" : ""}>${esc(f)}</option>`).join("")}</select></label>`}</td></tr>`).join("")}
          </tbody></table></div></div>` : ""}
      </div>
      <div class="sheet-foot">${t.done ? `${native ? `<button class="btn" id="td-finder">${ICON.reveal}Show in Finder</button>` : ""}<button class="btn primary" id="td-relink">${ICON.film}Relink in DaVinci Resolve</button><button class="btn" id="td-cancel">Close</button>` : `<button class="btn" id="td-cancel">Cancel</button>${plan ? `<button class="btn primary" id="td-go" ${moving && !t.busy ? "" : "disabled"}>${ICON.copy}Move ${moving.toLocaleString()} file${moving === 1 ? "" : "s"}</button>` : ""}`}</div>`);
    $("#td-close").onclick = $("#td-cancel").onclick = close;
    $("#td-template").onchange = (e) => { t.template = e.target.value; t.plan = null; draw(); };
    $("#td-pick").onclick = async () => { const f = await pickFolder(t.template); if (f) { t.template = f; t.plan = null; draw(); } };
    $("#td-look").onclick = async () => {
      t.busy = true; draw();
      try { t.plan = await api(`/api/projects/${p.id}/tidy/plan`, { method: "POST", body: { template: t.template } }); } catch (e) { toast(e.message, true); }
      t.busy = false; draw();
    };
    for (const sel of document.querySelectorAll("[data-td-dest]")) sel.onchange = () => { t.plan.groups[+sel.dataset.tdDest].dest = sel.value; draw(); };
    const go = $("#td-go"); if (go) go.onclick = async () => {
      const moves = t.plan.groups.filter((g) => !g.inPlace && g.dest).map((g) => ({ driveId: g.driveId, source: g.source, top: g.top, category: g.category, dest: g.dest }));
      t.busy = true; draw();
      try {
        const r = await api(`/api/projects/${p.id}/tidy/run`, { method: "POST", body: { template: t.template, moves } });
        toast(`Moving ${r.files} files; watch Activity`);
        const j = await waitForJob(r.job);
        t.done = j.state === "ok" ? `${r.files} files moved.` : `The move ended ${j.state}: ${esc(j.error || "see Activity")}.`;
        t.plan = null; t.busy = false; await loadProject(p.id); draw();
      } catch (e) { toast(e.message, true); t.busy = false; draw(); }
    };
    const rl = $("#td-relink"); if (rl) rl.onclick = async () => { rl.disabled = true; try { await api(`/api/projects/${p.id}/tidy/relink`, { method: "POST" }); toast("Asking Resolve to relink; watch Activity"); loadJobs(); } catch (e) { toast(e.message, true); rl.disabled = false; } };
    const fd = $("#td-finder"); if (fd) fd.onclick = () => { const src = p.sources.find((x) => x.role !== "backup" && driveOf(x.driveId) && driveOf(x.driveId).connected); if (src) nativeCall("reveal", { path: driveOf(src.driveId).mount + (src.folder ? "/" + src.folder : "") }); };
  };
  draw();
}
// waitForJob polls a job until it ends.
async function waitForJob(id) {
  for (let i = 0; i < 6000; i++) {
    const j = (await api("/api/jobs").catch(() => [])).find((x) => x.id === id);
    if (j && j.state !== "running" && j.state !== "queued") return j;
    await new Promise((r) => setTimeout(r, 500));
  }
  return { state: "running" };
}
// cardsSection is a project's standing orders for cards: watch for
// them, which ones, where each goes, and whether to ask first.
function cardsSection(p) {
  const c = p.cards || { watch: false, auto: false, match: "", dests: [] };
  const drives = (S.state.drives || []).filter((d) => !isCard(d));
  const row = (d, i) => `<div class="path-row card-dest" data-i="${i}"><label class="select"><select class="plain" data-cd-drive="${i}">${drives.map((x) => `<option value="${esc(x.id)}" ${x.id === d.driveId ? "selected" : ""}>${esc(x.title)}${x.connected ? "" : " (unplugged)"}</option>`).join("")}</select></label><input class="path-input" data-cd-folder="${i}" value="${esc(d.folder)}" placeholder="01_RAW/{date}"><button class="icon-btn" data-cd-rm="${i}" title="Remove">${ICON.x}</button></div>`;
  return `<section class="panel cards"><h2>Cards</h2>
    <p class="hint">When a camera card is plugged in, Lumpy can offer to offload it for this project, or just do it. The card's own folder is made inside each destination. <b>{date}</b> is today's date, <b>{day}</b> Day_MMDD, <b>{year}</b> the year.</p>
    <label class="switch row"><span>Watch for cards</span><input type="checkbox" id="cd-watch" ${c.watch ? "checked" : ""}></label>
    <div class="field" style="margin-top:8px"><label>Which cards <span class="hint">· a name pattern, comma-separated, or blank for every card</span></label><input class="path-input" id="cd-match" value="${esc(c.match || "")}" placeholder="A*, B*"></div>
    <div class="field" style="margin-top:8px"><label>Offload to</label>${(c.dests || []).map(row).join("")}<button class="btn small" id="cd-add" style="margin-top:4px">${ICON.plus}Another destination</button></div>
    <label class="switch row" style="margin-top:8px"><span>Offload without asking<small>When every destination drive is plugged in; otherwise Lumpy asks.</small></span><input type="checkbox" id="cd-auto" ${c.auto ? "checked" : ""}></label>
    <label class="switch row" style="margin-top:8px"><span>Make proxies with DaVinci Resolve once verified<small>ProRes 422 Proxy into the Offline folder beside the destination's Online folder. Resolve is opened if need be; its project is deleted after.</small></span><span style="display:flex;gap:8px;align-items:center"><label class="select"><select class="plain" id="cd-scale">${[["half", "Half size"], ["quarter", "Quarter size"], ["full", "Full size"]].map(([v, l]) => `<option value="${v}" ${c.proxyScale || "half" === v ? "selected" : ""}>${l}</option>`).join("")}</select></label><input type="checkbox" id="cd-proxies" ${c.proxies ? "checked" : ""}></span></label>
    <div class="go-row" style="margin-top:10px"><button class="btn primary" id="cd-save">Save card rules</button><span class="note">${c.watch && (c.dests || []).length ? `Watching for ${c.match ? esc(c.match) : "any card"} → ${(c.dests || []).map((d) => driveTitle(d.driveId) + "/" + d.folder).join(", ")}` : "Not watching"}</span></div>
  </section>`;
}
function wireCards(p) {
  const c = p.cards ? JSON.parse(JSON.stringify(p.cards)) : { watch: false, auto: false, match: "", dests: [] };
  if (!c.dests) c.dests = [];
  const read = () => {
    c.watch = $("#cd-watch").checked; c.auto = $("#cd-auto").checked; c.match = $("#cd-match").value.trim();
    c.proxies = $("#cd-proxies").checked; c.proxyScale = $("#cd-scale").value;
    c.dests = [...document.querySelectorAll(".card-dest")].map((r) => ({ driveId: r.querySelector("[data-cd-drive]").value, folder: r.querySelector("[data-cd-folder]").value.trim() }));
  };
  const redraw = () => { p.cards = c; const sec = document.querySelector(".panel.cards"); sec.outerHTML = cardsSection(p); wireCards(p); };
  $("#cd-add").onclick = () => { read(); c.dests.push({ driveId: (S.state.drives.find((d) => d.connected && !isCard(d)) || S.state.drives[0] || {}).id || "", folder: "01_RAW/{date}" }); redraw(); };
  for (const b of document.querySelectorAll("[data-cd-rm]")) b.onclick = () => { read(); c.dests.splice(+b.dataset.cdRm, 1); redraw(); };
  $("#cd-save").onclick = async () => {
    read();
    try { await api(`/api/projects/${p.id}`, { method: "PATCH", body: { cards: c } }); toast(c.watch ? "Card rules saved; Lumpy is watching" : "Card rules saved"); await loadProject(p.id); }
    catch (e) { toast(e.message, true); }
  };
}
// masterSection is the project's master print: the player, Show in
// Finder, and a switch to the open-matte version when there is one.
function masterSection(data) {
  const ms = data.masters || [];
  if (!ms.length) return `<section class="panel master"><h2>Master print</h2><p class="hint">None yet. A video with <b>master</b> in its name anywhere in the project (OUTPUT › final is where Lumpy looks first), or a clip tagged <b>master</b>, shows up here with a player.</p></section>`;
  const i = Math.min(S.masterIdx || 0, ms.length - 1), m = ms[i], matte = !!(m.matte && S.masterMatte);
  const cur = matte ? { driveId: m.matte.driveId, path: m.matte.path, name: m.matte.name } : { driveId: m.driveId, path: m.path, name: m.name };
  const stream = `/api/drives/${encodeURIComponent(cur.driveId)}/stream?path=${encodeURIComponent(cur.path)}&t=${encodeURIComponent(token)}`;
  const md = m.media || {};
  return `<section class="panel master"><div class="master-head"><h2>Master print${ms.length > 1 ? ` <span class="hint">· ${ms.length}</span>` : ""}</h2>
      ${ms.length > 1 ? `<label class="select"><select class="plain" id="master-pick">${ms.map((x, j) => `<option value="${j}" ${j === i ? "selected" : ""}>${esc(x.name)}</option>`).join("")}</select></label>` : ""}
      ${m.matte ? `<div class="seg" id="master-seg"><button data-matte="0" class="${matte ? "" : "on"}">Master</button><button data-matte="1" class="${matte ? "on" : ""}">Open matte</button></div>` : ""}
      <span class="hint">${esc(cur.name)} · ${esc(m.drive)}${md.width ? ` · ${md.width}×${md.height}` : ""}${md.durationMs ? ` · ${esc(dur(md.durationMs))}` : ""}${m.final ? "" : ' · <span class="warn">not in OUTPUT</span>'}</span>
      <div class="right">${native && m.connected ? `<button class="btn small" id="master-reveal">${ICON.reveal}Show in Finder</button>` : ""}<button class="btn small" id="master-open">${ICON.film}Details</button></div></div>
    ${m.connected ? `<div class="preview">${playerHTML(stream, md, m.connected && driveOf(cur.driveId) ? driveOf(cur.driveId).mount + "/" + cur.path : "", { driveId: cur.driveId, path: cur.path })}</div>` : `<div class="preview"><div class="no-preview">${ICON.film}<span>Plug in ${esc(m.drive)} to play it</span></div></div>`}</section>`;
}
function wireMaster(data) {
  const ms = data.masters || [];
  if (!ms.length) return;
  const i = Math.min(S.masterIdx || 0, ms.length - 1), m = ms[i], matte = !!(m.matte && S.masterMatte);
  const cur = matte ? m.matte : m;
  if ($("#player")) wirePlayer(m.media);
  const pick = $("#master-pick"); if (pick) pick.onchange = () => { S.masterIdx = +pick.value; S.masterMatte = false; render(); };
  for (const b of document.querySelectorAll("#master-seg button")) b.onclick = () => { S.masterMatte = b.dataset.matte === "1"; render(); };
  const rv = $("#master-reveal"); if (rv) rv.onclick = () => { const d = driveOf(cur.driveId); if (d) nativeCall("reveal", { path: d.mount + "/" + cur.path }); };
  $("#master-open").onclick = () => openFile(cur.driveId, cur.path, "project");
}
function wireProjectPage(p) {
  wireMaster(S.projectData);
  wireCards(p);
  $("#proj-tidy").onclick = () => tidyDialog(p);
  $("#proj-consolidate").onclick = () => consolidateDialog(p);
  $("#proj-media").onclick = () => { S.media = { project: String(p.id), drive: "", kind: "", format: "", codec: "", rate: "", qc: "", q: "" }; S.tab = "media"; S.project = null; localSet("tab", S.tab); render(); };
  $("#proj-media-csv").onclick = () => downloadCSV(`/api/media.csv?project=${p.id}`, `${p.name.replace(/[^\w.-]+/g, "_")}-clips-${new Date().toISOString().slice(0, 10)}.csv`);
  const patch = async (body, then) => {
    try { await api(`/api/projects/${p.id}`, { method: "PATCH", body }); await loadState(); await loadProject(p.id); if (then) then(); }
    catch (e) { toast(e.message, true); }
  };
  $("#proj-back").onclick = () => { S.project = null; S.projects = null; render(); };
  $("#proj-name").onchange = (e) => patch({ name: e.target.value.trim() || p.name });
  $("#proj-target").onchange = (e) => patch({ target: +e.target.value });
  $("#proj-notes").onchange = (e) => patch({ notes: e.target.value });
  for (const el of document.querySelectorAll("[data-pcolor]")) el.onclick = () => patch({ color: el.dataset.pcolor });
  $("#proj-add-src").onclick = () => addSourceDialog((src) => patch({ add: [src] }));
  for (const el of document.querySelectorAll("[data-remove-src]")) el.onclick = () => patch({ remove: [p.sources[+el.dataset.removeSrc]] });
  const setSrc = (i, changes) => { const s = p.sources[i]; patch({ set: { driveId: s.driveId, folder: s.folder, role: s.role || "", exclude: s.exclude || [], into: s.into || null, ...changes } }); };
  for (const el of document.querySelectorAll("[data-role]")) el.onchange = () => setSrc(+el.dataset.role, { role: el.checked ? "backup" : "" });
  for (const el of document.querySelectorAll("[data-into]")) el.onchange = () => { const [driveId, folder] = el.value.split("|"); setSrc(+el.dataset.into, { into: el.value ? { driveId, folder } : null }); };
  for (const el of document.querySelectorAll("[data-unex]")) el.onclick = () => { const [i, ex] = [+el.dataset.unex.split("|")[0], el.dataset.unex.slice(el.dataset.unex.indexOf("|") + 1)]; setSrc(i, { exclude: (p.sources[i].exclude || []).filter((x) => x !== ex) }); };
  for (const el of document.querySelectorAll("[data-ex]")) el.onclick = () => {
    const i = +el.dataset.ex, s = p.sources[i];
    addSourceDialog((pick) => {
      const prefix = s.folder ? s.folder + "/" : "";
      if (!pick.folder || (prefix && !pick.folder.startsWith(prefix))) return toast("Pick a folder inside " + (s.folder || driveTitle(s.driveId)), true);
      const rel = pick.folder.slice(prefix.length);
      if ((s.exclude || []).includes(rel)) return;
      setSrc(i, { exclude: [...(s.exclude || []), rel] });
    }, s.driveId, { lock: true, dir: s.folder || "", title: "Leave a folder out", ok: "Leave it out" });
  };
  for (const el of document.querySelectorAll("[data-drive-open]")) el.onclick = () => openDetail(el.dataset.driveOpen);
  const destIds = () => [...document.querySelectorAll("[data-dest]:checked")].map((el) => el.dataset.dest);
  for (const el of document.querySelectorAll('[name="dest-mode"]')) el.onchange = () => {
    if (el.value === "any") patch({ destIds: [] });
    else { document.querySelectorAll("[data-dest]").forEach((c) => (c.disabled = false)); $("#dest-list").style.opacity = ""; }
  };
  for (const el of document.querySelectorAll("[data-dest]")) el.onchange = () => patch({ destIds: destIds() });
  $("#proj-delete").onclick = () => {
    const box = document.createElement("div");
    box.className = "confirm";
    box.innerHTML = `<div><b>Delete ${esc(p.name)}?</b><br>The drives and everything recorded about them stay in the library.</div><div class="confirm-actions"><button class="btn" id="pd-no">Cancel</button><button class="btn danger-fill" id="pd-yes">Delete</button></div>`;
    $(".phead").after(box);
    $("#pd-no").onclick = () => box.remove();
    $("#pd-yes").onclick = async () => { try { await api(`/api/projects/${p.id}`, { method: "DELETE" }); S.project = null; S.projects = null; await loadState(); toast("Project deleted"); } catch (e) { toast(e.message, true); } };
  };
  for (const el of document.querySelectorAll("[data-run]")) el.onclick = async () => {
    const [sourceId, destId] = el.dataset.run.split("|");
    const folder = ($(`[data-folder="${CSS.escape(el.dataset.run)}"]`) || {}).value || "";
    const into = el.dataset.into || "";
    el.disabled = true;
    try {
      await api(`/api/projects/${p.id}/transfer`, { method: "POST", body: { sourceId, destId, folder, into } });
      toast(`Copying ${driveTitle(sourceId)} → ${driveTitle(destId)}. Watch it in Activity.`);
      await loadJobs();
      loadProject(p.id);
    } catch (e) { toast(e.message, true); el.disabled = false; }
  };
  for (const el of document.querySelectorAll("[data-hash]")) el.onclick = async () => {
    try { await api(`/api/drives/${encodeURIComponent(el.dataset.hash)}/hash`, { method: "POST" }); toast("Checksumming new files… (see Activity)"); loadJobs(); } catch (e) { toast(e.message, true); }
  };
  wireProxies(S.projectProxies || []);
  wireArchive(p);
}

// ---------- archive ----------
// When the cut is locked: the timeline export says which originals the
// edit uses, and the rest of the project's footage can go aside or to
// the Trash.
function archivePanel(p) {
  const a = S.archive && S.archive.project === p.id ? S.archive : null;
  return `<section class="panel" id="archive-panel"><h2>Archive</h2>
    <p class="hint">When the cut is locked, hand Lumpy the timeline exports and it says which originals the edits use and which they don't. FCPXML (Final Cut Pro, Resolve), Premiere Pro or FCP 7 XML, an EDL, or a plain list of clip names. Add as many as the job has: the cut, the trailer, the socials; a clip any of them uses is kept.</p>
    <div class="step-run" style="margin-top:0;flex-wrap:wrap"><button class="btn ${a ? "" : "primary"}" id="arc-pick">${ICON.plus}${a ? "Add another export…" : "Choose timeline exports…"}</button>${a ? a.files.map((f, i) => `<span class="chip">${esc(f)}<button class="chip-x" data-arc-drop="${i}" title="Leave this export out">${ICON.x}</button></span>`).join("") : ""}</div>
    <input type="file" id="arc-file" multiple hidden accept=".fcpxml,.xml,.edl,.txt,.csv">
    ${a ? archiveResult(p, a) : ""}</section>`;
}
// Which folders of the table an action touches: every folder with
// something unused, until some are unticked.
function archiveFolderKey(f) { return JSON.stringify([f.driveId, f.source, f.folder]); }
function archiveTopOf(src, path) {
  const rel = src.folder ? path.slice(src.folder.length + 1) : path;
  const i = rel.indexOf("/");
  return i >= 0 ? rel.slice(0, i) : ".";
}
function archiveSelected(a) {
  if (!a.sel) a.sel = new Set(a.folders.filter((f) => f.unusedFiles).map(archiveFolderKey));
  return a.sel;
}
function archiveResult(p, a) {
  const n = (v) => v.toLocaleString();
  const sel = archiveSelected(a);
  const picked = a.folders.filter((f) => f.unusedFiles && sel.has(archiveFolderKey(f)));
  const selFiles = picked.reduce((t, f) => t + f.unusedFiles, 0), selBytes = picked.reduce((t, f) => t + f.unusedBytes, 0);
  const pickedKeys = new Set(picked.map(archiveFolderKey));
  const inPick = (c) => { const src = p.sources[c.source] || { folder: "" }; return pickedKeys.has(JSON.stringify([c.driveId, src.folder, archiveTopOf(src, c.path)])); };
  const shown = a.unused.filter(inPick);
  const unplugged = a.drives.filter((d) => !d.connected && picked.some((f) => f.driveId === d.id)).map((d) => driveTitle(d.id));
  const all = a.folders.filter((f) => f.unusedFiles).length;
  const rows = a.folders.map((f) => { const d = driveOf(f.driveId); const k = archiveFolderKey(f); const on = f.unusedFiles && sel.has(k); return `<tr class="${f.unusedFiles ? "" : "dim"} ${f.unusedFiles && !on ? "off" : ""}"><td class="pickcell">${f.unusedFiles ? `<input type="checkbox" class="pick" data-arc-folder="${esc(k)}" ${on ? "checked" : ""} title="Include this folder">` : ""}</td><td><div class="name" style="font-weight:500"><span class="dot" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(driveTitle(f.driveId))} <span class="hint">›</span> <span class="mono">${esc(f.source ? f.source + "/" : "")}${f.folder === "." ? "" : esc(f.folder)}</span></div></td><td class="num">${n(f.files)}</td><td class="num" style="color:${f.unusedFiles ? "var(--warn)" : "var(--muted)"};font-weight:${f.unusedFiles ? 600 : 400}">${n(f.unusedFiles)}</td><td class="num">${f.unusedBytes ? bytes(f.unusedBytes) : "—"}</td></tr>`; }).join("");
  return `
    <div class="stat-row" style="margin-top:14px">
      <div class="stat ok"><div class="v">${n(a.usedFiles)}</div><div class="l">clips the edit uses (${bytes(a.usedBytes)})</div></div>
      <div class="stat ${a.unusedFiles ? "warn" : "dim"}"><div class="v">${n(a.unusedFiles)}</div><div class="l">not used (${bytes(a.unusedBytes)})</div></div>
      <div class="stat ${a.notFoundTotal ? "bad" : "dim"}"><div class="v">${n(a.notFoundTotal)}</div><div class="l">in the edit, not in this project (of ${n(a.refs)} referenced)</div></div>
    </div>
    ${a.notFoundTotal ? `<details class="notes" style="margin-top:10px"><summary>${n(a.notFoundTotal)} referenced clip${a.notFoundTotal === 1 ? "" : "s"} not in this project${a.notFound.length < a.notFoundTotal ? ` (first ${a.notFound.length})` : ""}</summary><div class="chips" style="margin-top:6px">${a.notFound.map((x) => `<span class="chip">${esc(x)}</span>`).join("")}</div><p class="hint" style="margin:6px 0 0">Stock, graphics, music, or footage in a folder outside this project. Nothing is done with these.</p></details>` : ""}
    ${a.folders.length ? `<p class="hint" style="margin:12px 0 4px">Tick the folders to clear. Output, graphics and audio folders are often best left alone.</p><table class="list compact arc-folders"><thead><tr><th class="pickcell">${all > 1 ? `<input type="checkbox" class="pick" id="arc-all" ${picked.length === all ? "checked" : ""} title="All or none">` : ""}</th><th>Drive · folder</th><th class="num">Clips</th><th class="num">Unused</th><th class="num">Unused size</th></tr></thead><tbody>${rows}</tbody></table>` : ""}
    ${a.unusedFiles ? `<details class="notes" style="margin-top:10px"><summary>${n(selFiles)} unused clip${selFiles === 1 ? "" : "s"} in the ${picked.length} folder${picked.length === 1 ? "" : "s"} ticked${shown.length < selFiles ? ` (first ${shown.length})` : ""}</summary><div class="chips" style="margin-top:6px">${shown.map((c) => `<span class="chip" title="${esc(driveTitle(c.driveId) + ": " + c.path)}">${esc(c.name)}<small>${bytes(c.bytes)}${c.files > 1 ? ` · ${c.files} parts` : ""}</small></span>`).join("") || '<span class="hint">Nothing ticked.</span>'}</div></details>
      <div class="step-run" style="margin-top:12px;flex-wrap:wrap"><span class="hint">Move the unused clips into a folder named</span><input class="path-input" id="arc-folder" value="_Unused" spellcheck="false" style="max-width:180px"><span class="hint">inside each project folder, keeping their paths, or to the Trash.</span></div>
      <div class="confirm-actions" style="margin-top:10px;justify-content:flex-start">${unplugged.length ? `<span class="badge">${ICON.plug}Plug in ${esc(unplugged.join(" and "))}</span>` : `<button class="btn primary" id="arc-move" ${selFiles ? "" : "disabled"}>${ICON.folder}Move ${n(selFiles)} unused into the folder</button><button class="btn danger" id="arc-trash" ${selFiles ? "" : "disabled"}>${ICON.trash}Move ${n(selFiles)} unused to the Trash…</button><span class="hint">${bytes(selBytes)} in ${picked.length} of ${all} folder${all === 1 ? "" : "s"}</span>`}</div>
      <p class="hint" style="margin:8px 0 0">Matching is by clip name, so a name the edit uses anywhere keeps every clip with that name. Clips in the Trash stay there until you empty it. The library follows the files either way.</p>`
    : `<div class="alert ok" style="margin-top:12px"><b>Everything in this project is used by the edit.</b> Nothing to move.</div>`}`;
}
function wireArchive(p) {
  // The exports chosen so far, in the order the chips show them: paths
  // picked natively first, then files read in the browser.
  const inputs = S.archiveInputs && S.archiveInputs.project === p.id ? S.archiveInputs : (S.archiveInputs = { project: p.id, paths: [], uploads: [] });
  const run = async () => {
    const btn = $("#arc-pick");
    btn.disabled = true;
    if (!inputs.paths.length && !inputs.uploads.length) { S.archive = null; renderProjectPage(); return; }
    try { const a = await api(`/api/projects/${p.id}/archive`, { method: "POST", body: { paths: inputs.paths, uploads: inputs.uploads } }); S.archive = { ...a, project: p.id }; renderProjectPage(); $("#archive-panel").scrollIntoView({ block: "nearest" }); }
    catch (e) { toast(e.message, true); btn.disabled = false; }
  };
  $("#arc-pick").onclick = async () => {
    if (native) { const paths = await nativeCall("pickFiles", {}); if (paths && paths.length) { for (const x of paths) if (!inputs.paths.includes(x)) inputs.paths.push(x); run(); } }
    else $("#arc-file").click();
  };
  $("#arc-file").onchange = async (e) => {
    const files = [...e.target.files];
    if (!files.length) return;
    const uploads = await Promise.all(files.map((f) => f.text().then((text) => ({ name: f.name, text }))));
    for (const u of uploads) if (!inputs.uploads.some((x) => x.name === u.name && x.text === u.text)) inputs.uploads.push(u);
    run();
  };
  for (const b of document.querySelectorAll("[data-arc-drop]")) b.onclick = () => {
    const i = +b.dataset.arcDrop;
    if (i < inputs.paths.length) inputs.paths.splice(i, 1); else inputs.uploads.splice(i - inputs.paths.length, 1);
    run();
  };
  const a = S.archive && S.archive.project === p.id ? S.archive : null;
  if (!a) return;
  const sel = archiveSelected(a);
  for (const el of document.querySelectorAll("[data-arc-folder]")) el.onchange = () => { if (el.checked) sel.add(el.dataset.arcFolder); else sel.delete(el.dataset.arcFolder); renderProjectPage(); };
  const allBox = $("#arc-all");
  if (allBox) allBox.onchange = () => { sel.clear(); if (allBox.checked) for (const f of a.folders) if (f.unusedFiles) sel.add(archiveFolderKey(f)); renderProjectPage(); };
  const folders = () => a.folders.filter((f) => f.unusedFiles && sel.has(archiveFolderKey(f))).map((f) => ({ driveId: f.driveId, source: f.source, folder: f.folder }));
  const apply = async (action) => {
    try {
      await api(`/api/projects/${p.id}/archive/apply`, { method: "POST", body: { token: a.token, action, folder: ($("#arc-folder") || {}).value || "_Unused", folders: folders() } });
      S.archive = null;
      toast(action === "trash" ? "Moving unused clips to the Trash… (see Activity)" : "Moving unused clips… (see Activity)");
      await loadJobs();
      loadProject(p.id);
    } catch (e) { toast(e.message, true); }
  };
  const mv = $("#arc-move");
  if (mv) mv.onclick = () => apply("move");
  const tr = $("#arc-trash");
  if (tr) tr.onclick = () => {
    const box = document.createElement("div");
    box.className = "confirm";
    const picked = a.folders.filter((f) => f.unusedFiles && sel.has(archiveFolderKey(f)));
    const files = picked.reduce((t, f) => t + f.unusedFiles, 0), size = picked.reduce((t, f) => t + f.unusedBytes, 0);
    box.innerHTML = `<div><b>Move ${files.toLocaleString()} clips (${bytes(size)}) from ${picked.length} folder${picked.length === 1 ? "" : "s"} to the Trash?</b><br>They stay in the Trash until you empty it, and can be put back from there. Lumpy's library forgets them now.</div><div class="confirm-actions"><button class="btn" id="arc-no">Cancel</button><button class="btn danger-fill" id="arc-yes">Move to Trash</button></div>`;
    $("#archive-panel h2").after(box);
    $("#arc-no").onclick = () => box.remove();
    $("#arc-yes").onclick = () => { box.remove(); apply("trash"); };
  };
}

// ---------- proxies ----------
// Post keeps camera originals in an "Online" folder and the proxies cut
// from them in an "Offline" folder beside it. Lumpy pairs the two and
// lists the originals that have no proxy yet.
function proxiesPanel(pairs) {
  if (!pairs.length) return "";
  return `<section class="panel"><h2>Proxies</h2>
    <p class="hint">Each <b>Online</b> folder of camera originals, paired with the <b>Offline</b> folder beside it. An original counts as proxied when a video with the same name (with or without a _proxy, _prx, S03 or _1 suffix) is anywhere in Offline. RED clips count once, not per chunk.</p>
    ${pairs.map(proxyPairCard).join("")}</section>`;
}
function proxyPairCard(x, i) {
  const pct = x.total ? (x.proxied / x.total) * 100 : 0;
  const missing = x.total - x.proxied;
  const d = driveOf(x.driveId);
  const done = x.total > 0 && !missing;
  return `<div class="step-card" style="margin-bottom:10px">
    <div class="step-head"><span class="dot ${d && d.connected ? "live" : ""}" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(x.driveId))}</b><span class="px-path" title="${esc(x.online)}${x.offline ? " → " + esc(x.offline) : " · no Offline folder yet"}">${esc(x.online)}${x.offline ? "" : ` <span class="hint">· no Offline folder yet</span>`}</span>
      <div class="right">${done ? `<span class="badge ok">${ICON.check}All proxied</span>` : x.total ? `${d && d.connected ? `<button class="btn small primary" data-make-proxies="${i}" title="Have DaVinci Resolve render ProRes 422 Proxy proxies of the originals that have none, into the Offline folder">${ICON.film}Make ${missing.toLocaleString()} prox${missing === 1 ? "y" : "ies"} via Resolve…</button>` : ""}<button class="btn small" data-copy-missing="${i}" title="Copy the names of the originals without a proxy, to hand to whoever makes them">${ICON.copy}Copy list</button>` : ""}</div></div>
    <div class="bbar ${done ? "s-full" : "s-partial"}"><div class="track">${x.proxied ? `<i class="${done ? "fill-full" : "fill-prog"}" style="width:${pct.toFixed(2)}%"></i>` : ""}</div><span class="lbl">${done ? ICON.check : ""}${x.total ? `${x.proxied.toLocaleString()} of ${x.total.toLocaleString()} originals have a proxy` : "No video in the Online folder"}</span></div>
    <div class="cap">${missing ? `<span style="color:var(--warn)">${missing.toLocaleString()} still to make · ${bytes(x.missingBytes)}</span>` : ""}${x.extra ? `<span class="sep">•</span><span class="hint">${x.extra.toLocaleString()} prox${x.extra === 1 ? "y" : "ies"} with no original</span>` : ""}<span class="sep">•</span><span class="hint">Offline: <span class="mono">${esc(x.offline)}</span></span></div>
    ${missing ? `<details class="notes"><summary>${missing.toLocaleString()} without a proxy${x.truncated ? ` (first ${x.missing.length} shown)` : ""}</summary><div class="chips" style="margin-top:6px">${x.missing.map((c) => `<span class="chip" title="${esc(c.path)}">${esc(c.name)}<small>${bytes(c.bytes)}${c.files > 1 ? ` · ${c.files} parts` : ""}</small></span>`).join("")}</div></details>` : ""}
  </div>`;
}
function wireProxies(pairs) {
  for (const el of document.querySelectorAll("[data-make-proxies]")) el.onclick = () => makeProxiesDialog(pairs[+el.dataset.makeProxies]);
  for (const el of document.querySelectorAll("[data-copy-missing]")) el.onclick = async () => {
    const x = pairs[+el.dataset.copyMissing];
    const text = x.missing.map((c) => c.name).join("\n") + "\n";
    try { await navigator.clipboard.writeText(text); toast(`Copied ${x.missing.length} clip name${x.missing.length === 1 ? "" : "s"}`); }
    catch { const ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta); ta.select(); document.execCommand("copy"); ta.remove(); toast(`Copied ${x.missing.length} clip name${x.missing.length === 1 ? "" : "s"}`); }
  };
}
// makeProxiesDialog asks the size and hands the Online folder's
// unproxied originals to Resolve as a job.
function makeProxiesDialog(x) {
  const missing = x.total - x.proxied;
  const offline = x.offline || (x.online.split("/").slice(0, -1).join("/") + "/Offline");
  openSheet(`<div class="sheet-head"><h2>Make ${missing.toLocaleString()} prox${missing === 1 ? "y" : "ies"} via DaVinci Resolve</h2><div class="actions"><button class="icon-btn" id="mp-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:12px">
      <p class="hint">Resolve decodes the camera's RAW where Lumpy can't. Lumpy makes a throwaway project in it, renders each original as <b>ProRes 422 Proxy</b> named like the original into <code>${esc(driveTitle(x.driveId))}/${esc(offline)}</code>, and deletes the project when it is done. Resolve is opened if it isn't running; its external scripting must be set to Local (Preferences › System › General).</p>
      <div class="field"><label>Size</label><label class="select"><select class="plain" id="mp-scale">${[["half", "Half size"], ["quarter", "Quarter size"], ["full", "Full size"]].map(([v, l]) => `<option value="${v}" ${localGet("offload.proxyScale", "half") === v ? "selected" : ""}>${l}</option>`).join("")}</select></label></div>
    </div>
    <div class="sheet-foot"><button class="btn" id="mp-cancel">Cancel</button><button class="btn primary" id="mp-go">${ICON.film}Render ${missing.toLocaleString()} via Resolve</button></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  $("#mp-close").onclick = $("#mp-cancel").onclick = close;
  $("#mp-go").onclick = async () => {
    $("#mp-go").disabled = true;
    try {
      const r = await api("/api/proxies", { method: "POST", body: { driveId: x.driveId, online: x.online, scale: $("#mp-scale").value } });
      localSet("offload.proxyScale", $("#mp-scale").value);
      close(); toast(`Rendering ${r.count} prox${r.count === 1 ? "y" : "ies"} via Resolve; see Activity`); loadJobs();
    } catch (e) { toast(e.message, true); $("#mp-go").disabled = false; }
  };
}
// loadProxies fills the drive sheet's Proxies section, if the drive has
// any Online/Offline pairs.
async function loadProxies(id) {
  let pairs = [];
  try { pairs = await api(`/api/drives/${encodeURIComponent(id)}/proxies`); } catch { return; }
  const sec = $("#sec-proxies");
  if (!sec || S.detail !== id || !pairs.length) return;
  sec.hidden = false;
  sec.innerHTML = `<div class="sec-title">${ICON.film}Proxies <span class="hint" style="font-weight:400">Online originals and the Offline proxies beside them</span></div>${pairs.map(proxyPairCard).join("")}`;
  wireProxies(pairs);
}

// ---------- new project folder ----------
// A project's folders from a template: the Post Haste templates on this
// Mac, or any folder. Post Haste's parameters name the folder.
const TEMPLATE_KEYS = ["Project Number", "Client", "Project Name", "Date", "Editor"];
function fillTemplate(s, params) {
  for (const [key, val] of Object.entries(params)) {
    for (const k of [key, key.replace(/ /g, "")]) {
      s = s.replace(new RegExp("[\\[{%<]\\s*" + k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "\\s*[\\]}%>]", "gi"), val);
    }
  }
  return s;
}
async function templateDialog() {
  let data;
  try { data = await api("/api/templates"); } catch (e) { return toast(e.message, true); }
  const st = data.settings;
  const f = { template: st.template || (data.templates[0] || {}).path || "", dest: st.dest || "", pattern: st.pattern, addProject: st.addProject, target: st.target,
    params: { "Project Number": String(st.nextNumber), "Client": st.client || "", "Project Name": "", "Date": data.today, "Editor": st.editor || "" } };
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  const render = () => {
    const tpl = data.templates.find((t) => t.path === f.template);
    const name = fillTemplate(f.pattern, f.params).replace(/[/:\\]/g, "-").trim();
    openSheet(`<div class="sheet-head"><h2>New project folder</h2><div class="actions"><button class="icon-btn" id="tp-close">${ICON.x}</button></div></div>
      <div class="sheet-body" style="padding-top:14px">
        <div class="sec"><div class="sec-title">${ICON.folder}Template</div><div class="box">
          <div class="fields"><div class="field"><label>Template</label><select class="plain" id="tp-template">${data.templates.map((t) => `<option value="${esc(t.path)}" ${t.path === f.template ? "selected" : ""}>${esc(t.name)}</option>`).join("")}${f.template && !tpl ? `<option value="${esc(f.template)}" selected>${esc(f.template.split("/").pop())}</option>` : ""}</select></div>
            <div class="field"><label>Or another template or folder</label><div class="path-row"><input class="path-input" id="tp-template-path" value="${esc(f.template)}" spellcheck="false" placeholder="A .pht bundle or any folder"><button class="btn" id="tp-pick-template">Choose…</button></div></div></div>
          ${tpl ? `<p class="hint" style="margin:10px 0 0">${tpl.folders.length} folder${tpl.folders.length === 1 ? "" : "s"}, ${tpl.files} file${tpl.files === 1 ? "" : "s"}: <span class="mono">${tpl.folders.map(esc).join("  ·  ")}</span></p>` : `<p class="hint" style="margin:10px 0 0">${data.templates.length ? "" : `No Post Haste templates found in <span class="mono">${esc(data.postHaste)}</span>. `}Choose a template or folder above.</p>`}
        </div></div>
        <div class="sec"><div class="sec-title">${ICON.tag}Parameters</div><div class="box"><div class="fields">
          ${TEMPLATE_KEYS.map((k) => `<div class="field ${k === "Project Name" ? "wide" : ""}"><label>${esc(k)}</label><input data-tp="${esc(k)}" value="${esc(f.params[k])}" ${k === "Date" ? 'type="date"' : ""} ${k === "Project Number" ? 'inputmode="numeric"' : ""} placeholder="${k === "Project Name" ? "ARIA" : ""}" spellcheck="false"></div>`).join("")}
          <div class="field wide"><label>Folder name · tokens: ${TEMPLATE_KEYS.map((k) => `<span class="mono">[${esc(k)}]</span>`).join(" ")}</label><input class="path-input" id="tp-pattern" value="${esc(f.pattern)}" spellcheck="false"></div>
        </div>
        <div class="step-run" style="margin-top:12px"><span class="hint">Preview</span><span class="chip">${ICON.folder}${esc(name || "…")}</span></div></div></div>
        <div class="sec"><div class="sec-title">${ICON.drive}Where</div><div class="box">
          <div class="field"><label>Create it in</label><div class="path-row"><input class="path-input" id="tp-dest" value="${esc(f.dest)}" spellcheck="false" placeholder="/Volumes/RAID/01_PROJECTS"><button class="btn" id="tp-pick-dest">Choose…</button></div></div>
          <label class="switch" style="margin-top:12px">Add it to Lumpy as a project <input type="checkbox" id="tp-add" ${f.addProject ? "checked" : ""}></label>
          <label class="hint" style="margin-left:14px">Copies wanted <select class="plain" id="tp-target">${[1, 2, 3, 4].map((n) => `<option value="${n}" ${f.target === n ? "selected" : ""}>${n}</option>`).join("")}</select></label>
        </div></div>
      </div>
      <div class="sheet-foot"><button class="btn" id="tp-cancel">Cancel</button><button class="btn primary" id="tp-ok" ${name && f.template && f.dest && f.params["Project Name"].trim() ? "" : "disabled"}>Create project folder</button></div>`, true);
    $("#tp-close").onclick = $("#tp-cancel").onclick = close;
    $("#tp-template").onchange = (e) => { f.template = e.target.value; render(); };
    $("#tp-template-path").onchange = (e) => { f.template = e.target.value.trim(); render(); };
    $("#tp-pick-template").onclick = async () => {
      if (native) { const paths = await nativeCall("pickFiles", { message: "Choose a Post Haste template (.pht) or a folder to copy" }); if (paths && paths.length) { f.template = paths[0]; render(); } }
      else { const p = await pickFolder(f.template); if (p) { f.template = p; render(); } }
    };
    for (const el of document.querySelectorAll("[data-tp]")) el.oninput = () => { f.params[el.dataset.tp] = el.value; const chip = $(".step-run .chip"); if (chip) chip.innerHTML = ICON.folder + esc(fillTemplate($("#tp-pattern").value, f.params).replace(/[/:\\]/g, "-").trim() || "…"); $("#tp-ok").disabled = !f.params["Project Name"].trim() || !f.template || !f.dest; };
    $("#tp-pattern").oninput = (e) => { f.pattern = e.target.value; const chip = $(".step-run .chip"); if (chip) chip.innerHTML = ICON.folder + esc(fillTemplate(f.pattern, f.params).replace(/[/:\\]/g, "-").trim() || "…"); };
    $("#tp-dest").onchange = (e) => { f.dest = e.target.value.trim(); render(); };
    $("#tp-pick-dest").onclick = async () => { const p = await pickFolder(f.dest); if (p) { f.dest = p; render(); } };
    $("#tp-add").onchange = (e) => { f.addProject = e.target.checked; };
    $("#tp-target").onchange = (e) => { f.target = +e.target.value; };
    $("#tp-ok").onclick = async () => {
      $("#tp-ok").disabled = true;
      try {
        const r = await api("/api/templates/create", { method: "POST", body: { template: f.template, dest: f.dest, pattern: f.pattern, params: f.params, addProject: f.addProject, target: f.target } });
        close();
        toast(`Creating ${r.name}… (see Activity)`);
        if (f.addProject) S.openProjectNamed = f.params["Project Name"];
        await loadJobs();
      } catch (e) { toast(e.message, true); $("#tp-ok").disabled = false; }
    };
    const first = $('[data-tp="Project Name"]');
    if (first && !f.params["Project Name"]) setTimeout(() => first.focus(), 30);
  };
  render();
}

// addSourceDialog lets the user pick a drive and, inside it, a folder
// from the library's index (or the whole drive), then calls done(source).
function addSourceDialog(done, startDrive, opts = {}) {
  const drives = S.state.drives || [];
  if (!drives.length) return toast("No drives logged yet", true);
  let driveId = startDrive || drives[0].id, dir = opts.dir || "";
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  const show = async () => {
    let entries = [];
    try { entries = await api(`/api/drives/${encodeURIComponent(driveId)}/list?dir=${encodeURIComponent(dir)}`); } catch (e) { toast(e.message, true); }
    const folders = entries.filter((e) => e.folder);
    const crumbs = dir ? dir.split("/") : [];
    openSheet(`<div class="sheet-head"><h2>${esc(opts.title || "Add to project")}</h2><div class="actions"><button class="icon-btn" id="as-close">${ICON.x}</button></div></div>
      <div class="sheet-body" style="padding-top:14px">
        <div class="field" ${opts.lock ? "hidden" : ""}><label>Drive</label><select class="plain" id="as-drive">${drives.map((d) => `<option value="${esc(d.id)}" ${d.id === driveId ? "selected" : ""}>${esc(d.title)}${d.connected ? " (plugged in)" : ""}</option>`).join("")}</select></div>
        <div class="picker-path" style="padding-top:12px"><button class="chip" data-crumb="">${esc(driveTitle(driveId))}</button>${crumbs.map((c, i) => ` <span class="hint">›</span> <button class="chip" data-crumb="${esc(crumbs.slice(0, i + 1).join("/"))}">${esc(c)}</button>`).join("")}</div>
        <div class="picker-list">${folders.length ? folders.map((f) => `<button data-go="${esc(f.path)}">${ICON.folder}${esc(f.name)}<span class="hint">${bytes(f.bytes)} · ${f.files.toLocaleString()} files</span></button>`).join("") : `<div class="tree-note">${dir ? "No folders inside this one." : "Nothing indexed on this drive yet."}</div>`}</div>
      </div>
      <div class="sheet-foot"><button class="btn" id="as-cancel">Cancel</button><button class="btn primary" id="as-ok">${opts.ok ? `${esc(opts.ok)}${dir ? ` “${esc(crumbs[crumbs.length - 1])}”` : ""}` : dir ? `Add “${esc(crumbs[crumbs.length - 1])}”` : "Add the whole drive"}</button></div>`, true);
    $("#as-close").onclick = $("#as-cancel").onclick = close;
    $("#as-drive").onchange = (e) => { driveId = e.target.value; dir = ""; show(); };
    for (const b of document.querySelectorAll("[data-go]")) b.onclick = () => { dir = b.dataset.go; show(); };
    for (const b of document.querySelectorAll("[data-crumb]")) b.onclick = () => { dir = b.dataset.crumb; show(); };
    $("#as-ok").onclick = () => { close(); done({ driveId, folder: dir }); };
  };
  show();
}

// pullDialog takes an edit (the twenty shots of a selects reel, a
// trailer) and says where each clip's camera original is across every
// drive, proxies seen through, then copies the chosen ones into one
// folder.
async function pullDialog() {
  const st = S.pull || (S.pull = { paths: [], uploads: [], a: null, pick: {}, dest: localGet("pull.dest", ""), name: "" });
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; S.onDrives = null; };
  const run = async () => {
    if (!st.paths.length && !st.uploads.length) { st.a = null; draw(); return; }
    draw(true);
    try { st.a = await api("/api/pull", { method: "POST", body: { paths: st.paths, uploads: st.uploads } }); st.pick = {}; }
    catch (e) { toast(e.message, true); }
    draw();
  };
  const hitOf = (ref, i) => st.pick[i] !== undefined ? ref.hits[st.pick[i]] : ref.hits[0];
  // Drives come and go while the sheet is open: redraw it live.
  S.onDrives = () => { if (!$("#sheet").hidden && $("#pl-pick")) draw(); else S.onDrives = null; };
  const draw = (busy) => {
    const a = st.a;
    const chosen = a ? a.refs.map((r, i) => r.hits.length ? { i, h: hitOf(r, i) } : null).filter(Boolean) : [];
    const total = chosen.reduce((t, x) => t + x.h.bytes, 0);
    const byDrive = {};
    for (const x of chosen) { const d = byDrive[x.h.driveId] || (byDrive[x.h.driveId] = { n: 0, b: 0 }); d.n++; d.b += x.h.bytes; }
    const unplugged = Object.keys(byDrive).filter((id) => !(driveOf(id) || {}).connected);
    const proxyOnly = chosen.filter((x) => x.h.proxy).length;
    openSheet(`<div class="sheet-head"><h2>Pull shots from an edit</h2><div class="actions"><button class="icon-btn" id="pl-close">${ICON.x}</button></div></div>
      <div class="sheet-body">
        <p class="hint" style="margin:0 0 10px">Hand Lumpy the cut, a selects reel or a list of clip names and it finds each clip's camera original across every drive in the library, whether the edit was cut on proxies or not, then copies the ones you choose into one folder. FCPXML, Premiere or FCP 7 XML, an EDL, or a plain list.</p>
        <div class="step-run" style="margin-top:0;flex-wrap:wrap"><button class="btn ${a ? "" : "primary"}" id="pl-pick" ${busy ? "disabled" : ""}>${ICON.plus}${a ? "Add another export…" : "Choose timeline exports…"}</button>${[...st.paths.map((x) => x.split("/").pop()), ...st.uploads.map((u) => u.name)].map((f, i) => `<span class="chip">${esc(a && a.files[i] ? a.files[i] : f)}<button class="chip-x" data-pl-drop="${i}" title="Leave this export out">${ICON.x}</button></span>`).join("")}</div>
        <input type="file" id="pl-file" multiple hidden accept=".fcpxml,.xml,.edl,.txt,.csv">
        ${busy ? loadingGlyph("Looking through every drive…") : a ? `
        <div class="stat-row" style="margin-top:14px">
          <div class="stat ok"><div class="v">${a.found}</div><div class="l">of ${a.refs.length} clips found (${bytes(total)} chosen)</div></div>
          <div class="stat ${proxyOnly ? "warn" : "dim"}"><div class="v">${proxyOnly}</div><div class="l">only as a proxy</div></div>
          <div class="stat ${a.missing.length ? "bad" : "dim"}"><div class="v">${a.missing.length}</div><div class="l">not in the library</div></div>
        </div>
        ${Object.keys(byDrive).length ? `<div class="chips" style="margin-top:12px">${Object.entries(byDrive).sort((x, y) => y[1].n - x[1].n).map(([id, d]) => { const dr = driveOf(id); return `<span class="chip" title="${esc(dr ? dr.title : id)}"><span class="dot ${dr && dr.connected ? "live" : ""}" style="background:${dr && dr.connected ? "var(--ok)" : "var(--muted)"};margin-right:6px"></span>${esc(dr ? dr.title : id)}<small>${d.n} clip${d.n === 1 ? "" : "s"} · ${bytes(d.b)}${dr && dr.connected ? "" : " · plug in"}</small></span>`; }).join("")}</div>` : ""}
        ${a.missing.length ? `<details class="notes" style="margin-top:10px"><summary>${a.missing.length} clip${a.missing.length === 1 ? "" : "s"} nowhere in the library</summary><div class="chips" style="margin-top:6px">${a.missing.map((x) => `<span class="chip">${esc(x)}</span>`).join("")}</div><p class="hint" style="margin:6px 0 0">Stock, graphics, music, or on a drive that was never logged. Log that drive and choose the export again.</p></details>` : ""}
        <table class="list compact" style="margin-top:12px"><thead><tr><th>Clip in the edit</th><th>Copy to pull</th><th class="num">Size</th></tr></thead><tbody>
          ${a.refs.map((r, i) => { if (!r.hits.length) return ""; const h = hitOf(r, i), dr = driveOf(h.driveId); return `<tr><td><div class="name">${esc(r.name)}</div></td><td>${r.hits.length > 1 ? `<select class="plain small" data-pl-ref="${i}" style="max-width:420px">${r.hits.map((x, j) => { const d2 = driveOf(x.driveId); return `<option value="${j}" ${(st.pick[i] ?? 0) === j ? "selected" : ""}>${esc(d2 ? d2.title : x.driveId)} › ${esc(x.path)}${x.proxy ? " · proxy" : ""}${d2 && !d2.connected ? " · not plugged in" : ""}</option>`; }).join("")}</select>` : `<span class="mono" style="font-size:12px">${esc(dr ? dr.title : h.driveId)} › ${esc(h.path)}</span>`}${h.proxy ? ` <span class="badge warn" title="No original found; this is a proxy">proxy</span>` : ""}${dr && !dr.connected ? ` <span class="badge" title="Plug this drive in to copy">${ICON.plug}</span>` : ""}</td><td class="num">${bytes(h.bytes)}${h.files > 1 ? ` <span class="hint">· ${h.files} parts</span>` : ""}</td></tr>`; }).join("")}
        </tbody></table>
        <div class="sec"><div class="sec-title">${ICON.folder}Pull into</div><div class="box">
          <div class="path-row"><input class="path-input" id="pl-dest" value="${esc(st.dest)}" placeholder="/Volumes/SHUTTLE_1" spellcheck="false"><button class="btn" id="pl-dest-pick">Choose…</button></div>
          <div class="path-row" style="margin-top:8px"><span class="hint" style="flex:none">in a folder named</span><input class="path-input" id="pl-name" value="${esc(st.name)}" placeholder="Pull ${new Date().toISOString().slice(0, 10)}" spellcheck="false"></div>
          <p class="hint" style="margin:8px 0 0">Each clip lands under a folder named after the drive it came from, keeping its path, so nothing collides and you can tell where it was. Every copy is read back and checked. Drives not plugged in are left for later: choose the export again when they are.</p>
        </div></div>
        <div class="sec"><div class="sec-title">${ICON.reveal}Or don't copy: point the edit at the originals where they are</div><div class="box">
          <div class="step-run" style="margin-top:0;flex-wrap:wrap"><button class="btn" id="pl-copy-paths">${ICON.copy}Copy paths</button><button class="btn" id="pl-save-csv">${ICON.export}Save list…</button><button class="btn" id="pl-relink" ${chosen.length - unplugged.length ? "" : ""}>${ICON.play}Relink in DaVinci Resolve</button></div>
          <p class="hint" style="margin:8px 0 0"><b>Copy paths</b> puts one full path per line on the clipboard; <b>Save list</b> writes a CSV with clip, path, drive and whether it's plugged in. <b>Relink in DaVinci Resolve</b> asks the open Resolve project to replace each media pool clip with its original, keeping every timeline use: Resolve must be open on the project with <i>Preferences → System → General → External scripting using: Local</i>, and the drives plugged in. A clip whose drive is unplugged is skipped and said so in Activity.</p>
        </div></div>` : ""}
      </div>
      <div class="sheet-foot"><button class="btn" id="pl-cancel">Close</button>${a ? `<button class="btn primary" id="pl-go" ${chosen.length && !busy ? "" : "disabled"}>${ICON.copy}Copy ${chosen.length - chosen.filter((x) => unplugged.includes(x.h.driveId)).length} of ${chosen.length} clips now</button>` : ""}</div>`);
    $("#pl-close").onclick = $("#pl-cancel").onclick = close;
    $("#pl-pick").onclick = async () => {
      if (native) { const paths = await nativeCall("pickFiles", {}); if (paths && paths.length) { for (const x of paths) if (!st.paths.includes(x)) st.paths.push(x); run(); } }
      else $("#pl-file").click();
    };
    $("#pl-file").onchange = async (e) => {
      const files = [...e.target.files];
      if (!files.length) return;
      const ups = await Promise.all(files.map((f) => f.text().then((text) => ({ name: f.name, text }))));
      for (const u of ups) if (!st.uploads.some((x) => x.name === u.name && x.text === u.text)) st.uploads.push(u);
      run();
    };
    for (const b of document.querySelectorAll("[data-pl-drop]")) b.onclick = () => { const i = +b.dataset.plDrop; if (i < st.paths.length) st.paths.splice(i, 1); else st.uploads.splice(i - st.paths.length, 1); run(); };
    for (const el of document.querySelectorAll("[data-pl-ref]")) el.onchange = () => { st.pick[+el.dataset.plRef] = +el.value; draw(); };
    if ($("#pl-dest")) {
      $("#pl-dest").onchange = (e) => { st.dest = e.target.value.trim(); localSet("pull.dest", st.dest); draw(); };
      $("#pl-name").onchange = (e) => { st.name = e.target.value; };
      $("#pl-dest-pick").onclick = async () => { const p = await pickFolder(st.dest); if (p) { st.dest = p; localSet("pull.dest", p); draw(); } };
    }
    const picks = () => chosen.map((x) => ({ driveId: x.h.driveId, path: x.h.path }));
    if ($("#pl-copy-paths")) {
      $("#pl-copy-paths").onclick = async () => {
        try {
          const list = await api("/api/pull/paths", { method: "POST", body: { token: a.token, picks: picks() } });
          const text = list.map((x) => x.full).join("\n");
          if (navigator.clipboard && navigator.clipboard.writeText) await navigator.clipboard.writeText(text);
          else { const ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta); ta.select(); document.execCommand("copy"); ta.remove(); }
          toast(`${list.length} path${list.length === 1 ? "" : "s"} copied${list.some((x) => !x.mounted) ? " · some drives aren't plugged in; their paths are where they were last seen" : ""}`);
        } catch (e) { toast(e.message, true); }
      };
      $("#pl-save-csv").onclick = () => downloadCSV("/api/pull/paths", `pull-${new Date().toISOString().slice(0, 10)}.csv`, { token: a.token, format: "csv", picks: picks() });
      $("#pl-relink").onclick = async () => {
        $("#pl-relink").disabled = true;
        try { await api("/api/pull/relink", { method: "POST", body: { token: a.token, picks: picks() } }); toast("Asking Resolve to relink… (see Activity)"); toggleActivity(true); loadJobs(); }
        catch (e) { toast(e.message, true); }
        $("#pl-relink").disabled = false;
      };
    }
    if ($("#pl-go")) $("#pl-go").onclick = async () => {
      st.dest = $("#pl-dest").value.trim(); st.name = $("#pl-name").value;
      if (!st.dest) { toast("Choose a folder to pull into", true); return; }
      $("#pl-go").disabled = true;
      try {
        const r = await api("/api/pull/run", { method: "POST", body: { token: a.token, dest: st.dest, name: st.name, picks: chosen.map((x) => ({ driveId: x.h.driveId, path: x.h.path })) } });
        close();
        toast(`${r.jobs.length} pull${r.jobs.length === 1 ? "" : "s"} started into ${r.into}${r.there.length ? ` · ${r.there.length} already on that drive` : ""}${r.unplugged.length ? ` · ${r.unplugged.map(driveTitle).join(", ")} not plugged in` : ""}`);
        toggleActivity(true); loadJobs();
      } catch (e) { toast(e.message, true); $("#pl-go").disabled = false; }
    };
  };
  draw();
}

// findProjectsDialog looks through every logged drive for folders that
// look like projects, groups the ones that share a first word, and
// creates a project per ticked group.
async function findProjectsDialog() {
  const ignoreKey = "discover.ignore";
  let ignore = localGet(ignoreKey, "BS");
  let groups;
  const load = async () => {
    openSheet(`<div class="sheet-head"><h2>Find projects</h2><div class="actions"><button class="icon-btn" id="fp-close">${ICON.x}</button></div></div><div class="sheet-body">${loadingGlyph("Looking through every drive…")}</div>`);
    $("#fp-close").onclick = close;
    try { groups = await api("/api/projects/discover?ignore=" + encodeURIComponent(ignore)); } catch (e) { toast(e.message, true); close(); return; }
    // Every folder remembers which group it sits in; moving one between
    // groups is a select.
    groups.forEach((g, gi) => { g.on = g.folders.some((f) => !f.inProject); g.folders.forEach((f) => { f.gi = gi; f.on = !f.inProject; }); });
    draw();
  };
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  const draw = () => {
    const all = groups.flatMap((g) => g.folders);
    const live = groups.map((g, gi) => ({ g, gi, fs: all.filter((f) => f.gi === gi) })).filter((x) => x.fs.length);
    const count = live.filter((x) => x.g.on && x.fs.some((f) => f.on)).length;
    openSheet(`<div class="sheet-head"><h2>Find projects</h2><div class="actions"><button class="icon-btn" id="fp-close">${ICON.x}</button></div></div>
      <div class="sheet-body">
        <p class="hint" style="margin:0 0 10px">Folders on your logged drives that look like projects: numbered ones at a drive's root (Post Haste style) and anything inside a projects folder. Folders that start with the same word are grouped as one project; rename a group, move a folder with its menu, or untick what isn't a project. Nothing on any drive is touched.</p>
        <div class="fields" style="margin-bottom:12px"><div class="field"><label>Words to leave out of names <span class="hint">· a house prefix, comma-separated</span></label><div style="display:flex;gap:8px"><input id="fp-ignore" value="${esc(ignore)}" placeholder="BS" spellcheck="false" style="flex:1"><button class="btn small" id="fp-again">Look again</button></div></div></div>
        ${live.length ? live.map(({ g, gi, fs }) => `<div class="box fp-group ${g.on ? "" : "off"}" style="margin-bottom:10px">
          <div class="fp-head"><input type="checkbox" data-fp-g="${gi}" ${g.on ? "checked" : ""}><input class="inline-input fp-name" data-fp-name="${gi}" value="${esc(g.name)}"><span class="hint">${fs.length} folder${fs.length === 1 ? "" : "s"} · ${bytes(fs.reduce((a, f) => a + f.bytes, 0))}</span></div>
          <div class="checks" style="margin-top:8px">${fs.map((f) => { const fi = all.indexOf(f); return `<label class="fp-row"><input type="checkbox" data-fp-f="${fi}" ${f.on ? "checked" : ""} ${f.inProject ? "disabled" : ""}><span class="fp-src">${sourceLabel(f)}</span><span class="hint">${bytes(f.bytes)} · ${f.files.toLocaleString()} files${f.inProject ? ` · already in ${esc((S.projects.find((p) => p.id === f.inProject) || {}).name || "a project")}` : ""}</span>
            <select class="plain small" data-fp-move="${fi}" title="Move to another group">${live.map(({ g: g2, gi: gi2 }) => `<option value="${gi2}" ${gi2 === gi ? "selected" : ""}>${esc(g2.name)}</option>`).join("")}<option value="new">Its own project</option></select></label>`; }).join("")}</div></div>`).join("")
        : `<div class="empty">${ICON.film}<h2>Nothing that looks like a project</h2><p>Lumpy looks for numbered folders at a drive's root and anything inside a folder named like 01_PROJECTS. Log the drives that hold your work and try again.</p></div>`}
      </div>
      <div class="sheet-foot"><button class="btn" id="fp-cancel">Cancel</button><button class="btn primary" id="fp-ok" ${count ? "" : "disabled"}>Create ${count} project${count === 1 ? "" : "s"}</button></div>`);
    $("#fp-close").onclick = $("#fp-cancel").onclick = close;
    $("#fp-again").onclick = () => { ignore = $("#fp-ignore").value.trim() || "BS"; localSet(ignoreKey, ignore); load(); };
    for (const el of document.querySelectorAll("[data-fp-g]")) el.onchange = () => { groups[+el.dataset.fpG].on = el.checked; draw(); };
    for (const el of document.querySelectorAll("[data-fp-name]")) el.onchange = () => { groups[+el.dataset.fpName].name = el.value.trim() || groups[+el.dataset.fpName].name; };
    for (const el of document.querySelectorAll("[data-fp-f]")) el.onchange = () => { all[+el.dataset.fpF].on = el.checked; draw(); };
    for (const el of document.querySelectorAll("[data-fp-move]")) el.onchange = () => {
      const f = all[+el.dataset.fpMove];
      if (el.value === "new") { groups.push({ key: f.name.toUpperCase(), name: f.name, folders: [], on: true }); f.gi = groups.length - 1; }
      else f.gi = +el.value;
      draw();
    };
    $("#fp-ok").onclick = async () => {
      $("#fp-ok").disabled = true;
      let made = 0;
      // Each new project gets a colour of its own, the least-used first.
      const used = {}; for (const p of S.projects || []) used[p.color] = (used[p.color] || 0) + 1;
      const palette = Object.keys(COLORS).filter((c) => c !== "gray").sort((a, b) => (used[a] || 0) - (used[b] || 0));
      let ci = 0;
      for (const { g, fs } of live) {
        const chosen = fs.filter((f) => f.on && !f.inProject).map((f) => ({ driveId: f.driveId, folder: f.folder }));
        if (!g.on || !chosen.length) continue;
        try {
          const p = await api("/api/projects", { method: "POST", body: { name: $(`[data-fp-name="${groups.indexOf(g)}"]`).value.trim() || g.name, target: 2, sources: chosen } });
          await api(`/api/projects/${p.id}`, { method: "PATCH", body: { color: palette[ci++ % palette.length] } });
          made++;
        } catch (e) { toast(`${g.name}: ${e.message}`, true); }
      }
      close();
      toast(`${made} project${made === 1 ? "" : "s"} created`);
      S.projects = null; await loadState(); render();
    };
  };
  await load();
}

function newProjectDialog(sources) {
  const drives = S.state.drives || [];
  const pending = [...sources];
  const render = () => {
    openSheet(`<div class="sheet-head"><h2>New project</h2><div class="actions"><button class="icon-btn" id="np-close">${ICON.x}</button></div></div>
      <div class="sheet-body" style="padding-top:14px">
        <div class="fields"><div class="field"><label>Name</label><input id="np-name" placeholder="Nike Holiday spot, Season 2, …" value="${esc(($("#np-name") || {}).value || "")}"></div>
          <div class="field"><label>Copies wanted of every file</label><select class="plain" id="np-target"><option value="2" selected>2 (RAID 1-style)</option><option value="3">3</option><option value="1">1 (just track it)</option></select></div></div>
        <div class="sec"><div class="sec-title">Folders in this project</div><div class="box">
          ${pending.length ? `<div class="checks">${pending.map((src, i) => `<label><input type="checkbox" checked data-np-src="${i}"><span>${sourceLabel(src)}</span></label>`).join("")}</div>` : '<span class="hint">None yet. Add the folder (or whole drive) that holds the project.</span>'}
          <div style="margin-top:10px"><button class="btn small" id="np-add">${ICON.plus}Add a folder or drive…</button></div></div></div>
      </div>
      <div class="sheet-foot"><button class="btn" id="np-cancel">Cancel</button><button class="btn primary" id="np-ok">Create</button></div>`, true);
    const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
    $("#np-close").onclick = $("#np-cancel").onclick = close;
    $("#np-add").onclick = () => { const name = $("#np-name").value; addSourceDialog((src) => { pending.push(src); render(); $("#np-name").value = name; }); };
    $("#np-ok").onclick = async () => {
      const chosen = [...document.querySelectorAll("[data-np-src]:checked")].map((el) => pending[+el.dataset.npSrc]);
      try {
        const p = await api("/api/projects", { method: "POST", body: { name: $("#np-name").value, target: +$("#np-target").value, sources: chosen } });
        close();
        S.selecting = false; S.selected.clear();
        S.tab = "projects"; S.project = p.id; S.projects = null; localSet("tab", S.tab);
        await loadState();
      } catch (e) { toast(e.message, true); }
    };
    setTimeout(() => $("#np-name").focus(), 30);
  };
  render();
}

function addToProjectDialog(sources, after) {
  const ps = S.state.projects || [];
  if (!ps.length) return newProjectDialog(sources);
  const what = sources.length === 1 ? sourceLabel(sources[0]) : `${sources.length} items`;
  openSheet(`<div class="sheet-head"><h2>Add ${what} to…</h2><div class="actions"><button class="icon-btn" id="ap-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:14px"><div class="picker-list">${ps.map((p) => `<button data-ap="${p.id}"><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span>${esc(p.name)}<span class="hint">${p.sources.length} folder${p.sources.length === 1 ? "" : "s"}</span></button>`).join("")}
      <button id="ap-new">${ICON.plus}New project…</button></div></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; if (after) after(); };
  $("#ap-close").onclick = close;
  $("#ap-new").onclick = () => { $("#sheet").hidden = true; newProjectDialog(sources); };
  for (const el of document.querySelectorAll("[data-ap]")) el.onclick = async () => {
    try { await api(`/api/projects/${el.dataset.ap}`, { method: "PATCH", body: { add: sources } }); S.selecting = false; S.selected.clear(); toast("Added"); await loadState(); close(); }
    catch (e) { toast(e.message, true); }
  };
}

// ---------- offload ----------
function renderOffload() {
  const o = S.offload;
  const vols = S.state.volumes;
  const chips = (target) => vols.map((v) => `<button class="chip" data-chip="${target}" data-path="${esc(v.mount)}">${esc(v.name)}<small>${bytes(v.free)} free</small></button>`).join("");
  const recent = (S.state.recent || {});
  const recentSel = (kind, i) => { const list = recent[kind] || []; return list.length ? `<select class="plain recent" data-recent="${kind}" data-i="${i ?? ""}" title="Recently used"><option value="">Recent…</option>${list.map((p) => `<option value="${esc(p)}">${esc(p)}</option>`).join("")}</select>` : ""; };
  const name = o.name || (o.source ? o.source.replace(/\/+$/, "").split("/").pop() : "");
  const busy = S.jobs.some((j) => j.kind === "offload" && j.state === "running");
  $("#main").innerHTML = `<div class="page-bar"><div class="toolbar-title">${ICON.copy}Offload</div></div><div class="offload split ${busy ? "busy" : ""}">
    <section class="panel">
      <h2>New offload</h2>
      <p class="hint">Lumpy reads the source once, writes every destination at the same time, reads each copy back from its drive and compares xxh64 checksums. Each copy gets an ASC MHL manifest and is recorded in the library.</p>
      <div class="step"><span class="step-n">1</span><div><h3>Source card or folder</h3>
        <div class="path-row"><input class="path-input" id="src" value="${esc(o.source)}" placeholder="/Volumes/A001">${recentSel("sources")}<button class="btn" data-pick="src">Choose…</button></div>
        <div class="chips">${chips("src")}</div></div></div>
      <div class="step"><span class="step-n">2</span><div><h3>Destinations <span style="color:var(--muted);font-weight:400">(at least two recommended)</span></h3>
        ${o.dests.map((p, i) => `<div class="path-row"><input class="path-input" data-dest="${i}" value="${esc(p)}" placeholder="/Volumes/SHUTTLE_1/Day_01">${recentSel("dests", i)}<button class="btn" data-pick="dest" data-i="${i}">Choose…</button>${o.dests.length > 1 ? `<button class="icon-btn" data-rmdest="${i}" title="Remove">${ICON.x}</button>` : ""}</div>`).join("")}
        <button class="btn small" id="add-dest" style="margin:2px 0 8px">${ICON.plus}Another destination</button>
        <div class="chips">${chips("dest")}</div></div></div>
      <div class="step"><span class="step-n">3</span><div><h3>Folder name in each destination</h3>
        <div class="path-row"><input class="path-input" id="oname" value="${esc(o.name)}" placeholder="${esc(name || "Same as the source")}"></div>
        <label class="check-row"><input type="checkbox" id="off-proxies" ${o.proxies ? "checked" : ""}><span>Make proxies with DaVinci Resolve once verified<span class="note">ProRes 422 Proxy, named like the originals, into the Offline folder beside the destination's Online folder (or a Proxies folder beside the copy). Resolve is opened if it isn't running; the project it uses is deleted after.</span></span><label class="select" style="margin-left:8px"><select class="plain" id="off-scale">${[["half", "Half size"], ["quarter", "Quarter size"], ["full", "Full size"]].map(([v, l]) => `<option value="${v}" ${o.proxyScale === v ? "selected" : ""}>${l}</option>`).join("")}</select></label></label>
        <label class="check-row"><input type="checkbox" id="source-check" ${o.sourceCheck ? "checked" : ""}><span>Read the card a second time while verifying<span class="note">Catches a faulty card or reader. Recommended; it runs alongside the copy checks.</span></span></label>
        <div class="go-row"><button class="btn primary" id="start-offload">${ICON.copy}Start offload</button>
          <span class="note" id="offload-note">${o.source && o.dests.some(Boolean) ? `Copies to ${o.dests.filter(Boolean).map((d) => esc(d.replace(/\/+$/, "") + "/" + (name || ""))).join(", ")}` : ""}</span></div></div></div>
    </section>
    <div class="jobs-col"><h2 class="col-title">${ICON.activity}Offloads</h2>
      <div class="day-report"><span>${ICON.export}Shoot-day report</span><input type="date" id="rep-date" value="${new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 10)}"><button class="btn small" id="rep-open">Open</button><button class="btn small" id="rep-save">Save…</button></div>
      <div class="jobs" id="offload-jobs"></div></div>
  </div>`;
  wireOffload();
  renderJobsInto($("#offload-jobs"), S.jobs.filter((j) => j.kind === "offload"), "No offloads yet.");
}

function wireOffload() {
  const o = S.offload;
  const repURL = () => `/api/report/day?date=${encodeURIComponent($("#rep-date").value)}`;
  $("#rep-open").onclick = () => window.open(repURL() + `&t=${encodeURIComponent(token)}`, "_blank");
  $("#rep-save").onclick = () => downloadCSV(repURL(), `Shoot day ${$("#rep-date").value}.html`, null, "text/html");
  const rerender = () => renderOffload();
  $("#src").onchange = (e) => { o.source = e.target.value.trim(); rerender(); };
  $("#oname").onchange = (e) => { o.name = e.target.value.trim(); rerender(); };
  $("#source-check").onchange = (e) => { o.sourceCheck = e.target.checked; };
  $("#off-proxies").onchange = (e) => { o.proxies = e.target.checked; localSet("offload.proxies", o.proxies ? "1" : "0"); };
  $("#off-scale").onchange = (e) => { o.proxyScale = e.target.value; localSet("offload.proxyScale", o.proxyScale); };
  for (const el of document.querySelectorAll("[data-dest]")) el.onchange = () => { o.dests[+el.dataset.dest] = el.value.trim(); rerender(); };
  for (const el of document.querySelectorAll("[data-rmdest]")) el.onclick = () => { o.dests.splice(+el.dataset.rmdest, 1); rerender(); };
  $("#add-dest").onclick = () => { o.dests.push(""); rerender(); };
  for (const el of document.querySelectorAll("[data-recent]")) el.onchange = () => {
    if (!el.value) return;
    if (el.dataset.recent === "sources") o.source = el.value; else o.dests[+el.dataset.i] = el.value;
    rerender();
  };
  for (const el of document.querySelectorAll("[data-chip]")) el.onclick = () => {
    if (el.dataset.chip === "src") o.source = el.dataset.path;
    else { const i = o.dests.findIndex((x) => !x); if (i >= 0) o.dests[i] = el.dataset.path; else o.dests.push(el.dataset.path); }
    rerender();
  };
  for (const el of document.querySelectorAll("[data-pick]")) el.onclick = async () => {
    const start = el.dataset.pick === "src" ? o.source : o.dests[+el.dataset.i];
    const p = await pickFolder(start);
    if (!p) return;
    if (el.dataset.pick === "src") o.source = p; else o.dests[+el.dataset.i] = p;
    rerender();
  };
  $("#start-offload").onclick = async () => {
    const dests = o.dests.filter(Boolean);
    if (!o.source || !dests.length) return toast("Choose a source and at least one destination", true);
    try {
      await api("/api/offload", { method: "POST", body: { source: o.source, dests, name: o.name, sourceCheck: o.sourceCheck, proxies: { on: !!o.proxies, scale: o.proxyScale } } });
      toast("Offload started");
      o.name = "";
      await loadJobs();
      renderOffload();
    } catch (e) { toast(e.message, true); }
  };
}

async function pickFolder(start) {
  if (native) return nativeCall("pickFolder", { start: start || "" });
  return new Promise((resolve) => {
    let current = "";
    const show = async (path) => {
      let data;
      try { data = await api("/api/browse?path=" + encodeURIComponent(path)); } catch (e) { toast(e.message, true); return; }
      current = data.path;
      openSheet(`<div class="sheet-head"><h2>Choose a folder</h2><div class="actions"><button class="icon-btn" id="pk-close">${ICON.x}</button></div></div>
        <div class="sheet-body" style="padding-top:14px"><div class="picker-path">${esc(data.path || "Drives and places")}</div>
        <div class="picker-list">${data.path ? `<button data-go="${esc(data.parent)}">${ICON.up} ..</button>` : ""}
        ${data.items.map((it) => `<button data-go="${esc(it.path)}">${data.path ? ICON.folder : ICON.drive}${esc(it.name)}</button>`).join("") || '<div class="tree-note">No folders here.</div>'}</div></div>
        <div class="sheet-foot"><button class="btn" id="pk-cancel">Cancel</button><button class="btn primary" id="pk-ok" ${data.path ? "" : "disabled"}>Choose this folder</button></div>`, true);
      for (const b of document.querySelectorAll("[data-go]")) b.onclick = () => show(b.dataset.go);
      const done = (v) => { $("#sheet").hidden = true; $("#scrim").hidden = true; resolve(v); };
      $("#pk-close").onclick = $("#pk-cancel").onclick = () => done(null);
      $("#pk-ok").onclick = () => done(current);
    };
    show(start || "");
  });
}

// ---------- activity ----------
// Activity is a panel that drops down from the top bar, so a running
// offload can be watched from any tab.
function toggleActivity(open) {
  const pop = $("#activity-pop");
  const show = open === undefined ? pop.hidden : open;
  pop.hidden = !show;
  $("#activity-btn").classList.toggle("on", show);
  if (show) renderJobsInto($("#activity-jobs"), S.jobs, "Nothing has run yet. Offloads, drive checks and rescans show their progress here.");
}

const STATE_BADGE = { running: ["info", "Running"], queued: ["", "Queued"], ok: ["ok", "Done"], cancelled: ["warn", "Cancelled"], error: ["bad", "Error"] };
function lineClass(l) {
  if (/^FAIL/.test(l)) return "t-fail";
  if (/^BAD /.test(l)) return "t-bad";
  if (/^WARN/.test(l)) return "t-warn";
  if (/^OK /.test(l)) return "t-ok";
  if (/^ {2}copied /.test(l)) return "t-copied";
  if (/^ {2}(verified|source ok) /.test(l)) return "t-verified";
  if (/^ {2}exists /.test(l)) return "t-dim";
  if (/^(Copying|Verifying|Checking|Hashing)\b.*(\.\.\.|files)/.test(l) || /^(Copied|Verified) /.test(l)) return "t-head";
  return "";
}
function lineEl(text) {
  const d = document.createElement("div");
  const c = lineClass(text);
  if (c) d.className = c;
  d.textContent = text || " ";
  return d;
}

function jobShell(j) {
  const el = document.createElement("div");
  el.className = "job";
  el.dataset.job = j.id;
  el.innerHTML = `<div class="job-head"><h3></h3><span class="badge"></span><span class="when"></span><div class="actions"></div></div>
    <div class="job-progress"><div class="bar"><i></i></div><div class="job-stats"></div><div class="job-current"></div></div>
    <div class="job-outcome"></div>
    <details class="term-wrap" ${j.state === "running" ? "open" : ""}><summary>Terminal</summary>
      <div class="term"><div class="term-bar"><span class="term-title"></span><button class="term-copy" title="Copy the log">${ICON.copy}</button></div>
      <div class="term-body"><div class="t-prompt"><span class="ps">$</span> <span class="cmd"></span></div><div class="t-lines"></div><span class="cursor"></span></div></div>
    </details>`;
  el._count = 0;
  el.querySelector(".term-copy").onclick = async (e) => {
    e.preventDefault();
    const text = "$ " + (j.command || "") + "\n" + [...el.querySelectorAll(".t-lines > div")].map((d) => d.textContent).join("\n");
    try { await navigator.clipboard.writeText(text); toast("Log copied"); }
    catch { const r = document.createRange(); r.selectNodeContents(el.querySelector(".term-body")); getSelection().removeAllRanges(); getSelection().addRange(r); toast("Log selected. Press ⌘C to copy"); }
  };
  return el;
}

function updateJob(el, j) {
  el.className = "job st-" + j.state;
  const running = j.state === "running";
  const queued = j.state === "queued";
  const p = j.progress;
  el.querySelector(".job-head h3").textContent = j.title;
  const [cls, label] = j.state === "failed" ? ["bad", `${j.failures} problem(s)`] : STATE_BADGE[j.state] || ["", j.state];
  const badge = el.querySelector(".job-head .badge");
  badge.className = "badge " + cls;
  badge.textContent = label;
  el.querySelector(".job-head .when").textContent = short(j.started) + (j.ended ? " · " + duration(j.ended - j.started || 1) : "");
  const actions = el.querySelector(".job-head .actions");
  const wantCancel = running || queued ? "1" : j.params ? "again" : "";
  if (actions.dataset.cancel !== wantCancel) {
    actions.dataset.cancel = wantCancel;
    actions.innerHTML = running || queued ? `<button class="btn small" data-cancel="${j.id}">${ICON.x}Cancel</button>`
      : j.params ? `<button class="btn small" title="Fill the offload form with this job's source and destinations">${ICON.refresh}Run again</button>` : "";
    const b = actions.querySelector("button");
    if (b && (running || queued)) b.onclick = async () => { try { await api(`/api/jobs/${j.id}/cancel`, { method: "POST" }); } catch (e) { toast(e.message, true); } };
    else if (b) b.onclick = () => {
      // Same source, same destinations: a disconnected card or a
      // cancelled run is picked up where it stopped.
      S.offload = { source: j.params.source || "", dests: (j.params.dests || []).slice(), name: j.params.name || "", sourceCheck: j.params.sourceCheck !== false, proxies: !!(j.params.proxies && j.params.proxies.on), proxyScale: (j.params.proxies && j.params.proxies.scale) || localGet("offload.proxyScale", "half") };
      toggleActivity(false);
      S.tab = "offload"; S.project = null; localSet("tab", S.tab); render();
      toast("Offload form filled in from that job. Check it and press Start offload.");
    };
  }

  const prog = el.querySelector(".job-progress");
  prog.hidden = !running && !queued;
  prog.querySelector(".bar").style.display = queued ? "none" : "";
  el.querySelector(".term-wrap").hidden = queued; // nothing to show until it starts
  if (queued) {
    prog.querySelector(".bar > i").style.width = "0%";
    prog.querySelector(".job-stats").innerHTML = `<span><b>Queued</b></span><span>waits for ${esc((j.needs || []).map(driveTitle).join(" and ") || "another job")} to be free</span>`;
    prog.querySelector(".job-current").textContent = "";
  }
  if (running) {
    const pct = p && p.total ? Math.min(100, (p.done / p.total) * 100) : 0;
    prog.querySelector(".bar > i").style.width = pct.toFixed(1) + "%";
    const eta = p && p.rate > 0 && p.total > p.done ? duration((p.total - p.done) / p.rate) + " left" : "";
    prog.querySelector(".job-stats").innerHTML = p
      ? `<span><b>${esc(p.label)}</b></span><span>${bytes(p.done)} of ${bytes(p.total)}</span><span>${pct.toFixed(0)}%</span><span>${bytes(p.rate)}/s</span><span>${eta}</span>${j.failures ? `<span style="color:var(--bad)">${j.failures} problem(s)</span>` : ""}`
      : "<span>Preparing…</span>";
    prog.querySelector(".job-current").textContent = p && p.current ? p.current : "";
  }

  const outcome = el.querySelector(".job-outcome");
  const results = running ? [] : (j.lines || []).filter((l) => /^(OK|BAD) /.test(l));
  outcome.hidden = !results.length && !j.error;
  outcome.innerHTML = (j.error ? `<div class="t-fail">${esc(j.error)}</div>` : "") +
    results.map((l) => `<div class="${lineClass(l)}">${esc(l.replace(/\s{2,}/g, "  "))}</div>`).join("");

  el.querySelector(".term-title").textContent = "lumpy — " + j.title;
  el.querySelector(".cmd").textContent = j.command || "";
  const body = el.querySelector(".term-body");
  const lines = el.querySelector(".t-lines");
  const atBottom = body.scrollHeight - body.scrollTop - body.clientHeight < 24;
  const fresh = j.lineCount - el._count;
  if (fresh > 0) {
    const frag = document.createDocumentFragment();
    if (fresh > j.lines.length) {
      lines.textContent = "";
      if (j.lineCount > j.lines.length) frag.append(lineEl(`… ${(j.lineCount - j.lines.length).toLocaleString()} earlier lines not shown (use Copy for what's here, or run with --report to keep everything)`));
      j.lines.forEach((l) => frag.append(lineEl(l)));
    } else {
      j.lines.slice(j.lines.length - fresh).forEach((l) => frag.append(lineEl(l)));
    }
    lines.append(frag);
    // Keep the terminal from growing without limit on huge offloads.
    while (lines.childElementCount > 4000) lines.firstElementChild.remove();
    el._count = j.lineCount;
  }
  el.querySelector(".cursor").hidden = !running;
  el.querySelector(".term-wrap summary").textContent = `Terminal · ${j.lineCount.toLocaleString()} lines`;
  if (atBottom || fresh === j.lineCount) body.scrollTop = body.scrollHeight;
}

function renderJobsInto(box, list, emptyText) {
  if (!box) return;
  if (!list.length) {
    box.innerHTML = `<div class="empty" style="margin:30px auto">${ICON.activity}<p>${emptyText}</p></div>`;
    return;
  }
  box.querySelector(".empty")?.remove();
  const keep = new Set(list.map((j) => String(j.id)));
  box.querySelectorAll(":scope > .job").forEach((el) => { if (!keep.has(el.dataset.job)) el.remove(); });
  let prev = null;
  for (const j of list) {
    let el = box.querySelector(`:scope > .job[data-job="${j.id}"]`);
    if (!el) {
      el = jobShell(j);
      if (prev) prev.after(el); else box.prepend(el);
    }
    updateJob(el, j);
    prev = el;
  }
}

// ---------- media ----------
// The clip library: every file with metadata, across drives, one row per
// distinct clip (the same checksum on three drives is one row).
const KIND_LABEL = { video: "Video", audio: "Audio", image: "Image", document: "Document", other: "Other" };
// The codec and format menus list what the industry shoots, with the
// library's own entries first. The rest are shown greyed, so the menu
// reads the same on every Mac and says what Lumpy can tell apart.
const CODECS = ["ProRes 422 Proxy", "ProRes 422 LT", "ProRes 422", "ProRes 422 HQ", "ProRes 4444", "ProRes 4444 XQ", "ProRes RAW", "ProRes RAW HQ",
  "H.264", "H.264 Intra", "HEVC", "AV1", "DNxHD", "DNxHR", "XDCAM HD 422", "XDCAM EX", "AVC-Intra 100", "AVC-Intra 50", "MPEG-2", "Motion JPEG", "CineForm",
  "DV", "DVCPRO HD", "Uncompressed 10-bit 4:2:2", "REDCODE RAW", "Blackmagic RAW", "ARRIRAW", "Cinema RAW Light", "CinemaDNG", "PCM", "AAC", "MP3", "FLAC"];
const FORMATS = ["QuickTime", "MP4", "MXF", "R3D", "BRAW", "ARRIRAW", "Canon RAW", "AVCHD", "Matroska", "AVI", "WAV", "BWF", "AIFF", "MP3", "FLAC", "JPEG", "PNG", "TIFF", "DNG", "HEIC", "OpenEXR", "DPX"];
const QC_LABEL = { "": "Not checked", ok: "OK", flag: "Flagged", reject: "Rejected" };
function fps(r) { return r ? (Math.round(r * 1000) / 1000).toString() : ""; }
function dur(ms) {
  if (!ms) return "";
  const s = Math.round(ms / 1000);
  return `${String(Math.floor(s / 3600)).padStart(2, "0")}:${String(Math.floor(s / 60) % 60).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}
// clipFormat sums up a clip's technical side in a few words.
function clipFormat(m) {
  const parts = [];
  if (m.videoCodec) parts.push(m.videoCodec);
  if (m.width) parts.push(`${m.width}×${m.height}`);
  if (!m.videoCodec && m.audioCodec) parts.push(m.audioCodec + (m.sampleRate ? ` ${m.sampleRate / 1000} kHz` : "") + (m.bitDepth ? ` ${m.bitDepth}-bit` : "") + (m.channels ? ` ${m.channels}ch` : ""));
  if (!parts.length && m.container) parts.push(m.container);
  return parts.join(" · ");
}
function qcBadge(qc, small) {
  if (!qc) return small ? "" : `<span class="badge">Not checked</span>`;
  return `<span class="badge qc-${qc}">${qc === "ok" ? ICON.check : qc === "flag" ? ICON.warn : ICON.x}${QC_LABEL[qc]}</span>`;
}
function mediaQuery() {
  const m = S.media, q = new URLSearchParams();
  for (const k of ["project", "drive", "kind", "format", "codec", "rate", "qc", "q"]) if (m[k]) q.set(k, m[k]);
  return q.toString();
}
async function loadMedia() {
  try { S.mediaData = await api("/api/media?" + mediaQuery()); } catch (e) { toast(e.message, true); return; }
  if (S.tab === "media") { renderMediaTable(); refreshMediaMenus(); }
}
const opt = (v, label, cur) => `<option value="${esc(v)}" ${String(cur) === String(v) ? "selected" : ""}>${esc(label)}</option>`;
function catalogue(present, all, cur, anyLabel) {
  const have = new Set(present);
  const rest = all.filter((c) => !have.has(c));
  const mine = present.map((c) => opt(c, c, cur)).join("");
  return opt("", anyLabel, cur) + (present.length && rest.length ? `<optgroup label="In the library">${mine}</optgroup>` : mine)
    + (rest.length ? `<optgroup label="${present.length ? "Not in the library" : "Nothing logged yet"}">${rest.map((c) => `<option value="${esc(c)}" disabled>${esc(c)}</option>`).join("")}</optgroup>` : "");
}
// refreshMediaMenus rebuilds the format, codec and frame-rate menus from
// the list that just arrived: the bar is drawn before the first load.
function refreshMediaMenus() {
  const d = S.mediaData, m = S.media;
  if (!d) return;
  const set = (id, html) => { const el = $(id); if (el && el.innerHTML !== html && document.activeElement !== el) el.innerHTML = html; };
  set("#m-format", catalogue(d.formats || [], FORMATS, m.format, "Any format"));
  set("#m-codec", catalogue(d.codecs || [], CODECS, m.codec, "Any codec"));
  set("#m-rate", opt("", "Any frame rate", m.rate) + (d.rates || []).map((r) => opt(r, fps(r) + " fps", m.rate)).join(""));
}
function renderMedia() {
  const main = $("#main");
  const m = S.media, st = S.state;
  const d = S.mediaData || { files: [], formats: [], codecs: [], rates: [] };
  const SORTS = [["name", "Name"], ["path", "Folder"], ["size", "Size"], ["format", "Format"], ["created", "Date created"], ["duration", "Length"], ["timecode", "Timecode"], ["rate", "Frame rate"], ["camera", "Camera"], ["scene", "Scene / take"], ["qc", "QC"], ["drive", "Drive"]];
  main.innerHTML = `<div class="media">
    <div class="media-bar">
      <div class="toolbar-title">${ICON.clapper}Clips</div>
      <div class="seg" id="m-view">${[["table", "Table"], ["strip", "Filmstrip"]].map(([v, l]) => `<button data-mview="${v}" class="${S.mediaView === v ? "on" : ""}">${l}</button>`).join("")}</div>
      <span class="sortgrp"><label class="select" title="Sort"><select id="m-sort">${SORTS.map(([k, v]) => opt(k, "Sort: " + v, S.mediaSort)).join("")}</select></label>
      <button class="icon-btn" id="m-dir" title="Reverse order">${S.mediaDesc ? ICON.down : ICON.up}</button></span>
      <div class="search"><span class="search-icon">${ICON.search}</span><input id="m-q" type="search" placeholder="Clip, scene, take, camera, reel, notes, tag…" value="${esc(m.q)}" autocomplete="off" spellcheck="false"></div>
      <button class="btn" id="m-export" title="This list with every column, for post">${ICON.export}Export CSV…</button>
    </div>
    <div class="media-filters">
      <label class="select"><select id="m-project">${opt("", "All projects", m.project)}${(st.projects || []).map((p) => opt(p.id, p.name, m.project)).join("")}</select></label>
      <label class="select"><select id="m-drive">${opt("", "All drives", m.drive)}${st.drives.map((x) => opt(x.id, x.title, m.drive)).join("")}</select></label>
      <label class="select"><select id="m-kind">${opt("", "Any kind", m.kind)}${Object.entries(KIND_LABEL).map(([k, v]) => opt(k, v, m.kind)).join("")}</select></label>
      <label class="select"><select id="m-format">${catalogue(d.formats || [], FORMATS, m.format, "Any format")}</select></label>
      <label class="select"><select id="m-codec">${catalogue(d.codecs || [], CODECS, m.codec, "Any codec")}</select></label>
      <label class="select"><select id="m-rate">${opt("", "Any frame rate", m.rate)}${(d.rates || []).map((r) => opt(r, fps(r) + " fps", m.rate)).join("")}</select></label>
      <label class="select"><select id="m-qc">${opt("", "Any QC", m.qc)}${opt("none", "Not checked", m.qc)}${opt("ok", "OK", m.qc)}${opt("flag", "Flagged", m.qc)}${opt("reject", "Rejected", m.qc)}</select></label>
      <span class="pills" id="m-pills"></span>
    </div>
    <div id="media-body"></div></div>`;
  const reload = () => { for (const k of ["project", "drive", "kind", "format", "codec", "rate", "qc"]) m[k] = $("#m-" + k).value; loadMedia(); };
  for (const k of ["project", "drive", "kind", "format", "codec", "rate", "qc"]) $("#m-" + k).onchange = reload;
  $("#m-sort").onchange = (e) => { S.mediaSort = e.target.value; localSet("mediaSort", S.mediaSort); renderMediaTable(); };
  for (const b of document.querySelectorAll("[data-mview]")) b.onclick = () => {
    S.mediaView = b.dataset.mview; localSet("mediaView", S.mediaView);
    document.querySelectorAll("[data-mview]").forEach((x) => x.classList.toggle("on", x === b));
    renderMediaTable();
  };
  $("#m-dir").onclick = () => { S.mediaDesc = !S.mediaDesc; localSet("mediaDesc", S.mediaDesc ? "1" : "0"); $("#m-dir").innerHTML = S.mediaDesc ? ICON.down : ICON.up; renderMediaTable(); };
  let t;
  $("#m-q").oninput = (e) => { m.q = e.target.value; clearTimeout(t); t = setTimeout(loadMedia, 220); };
  $("#m-export").onclick = () => downloadCSV("/api/media.csv?" + mediaQuery(), `lumpy-clips-${new Date().toISOString().slice(0, 10)}.csv`);
  if (S.mediaData) renderMediaTable(); else { $("#media-body").innerHTML = loadingGlyph("Reading the clips…"); loadMedia(); }
}
const CLIP_KEY = {
  name: (f) => f.path.split("/").pop().toLowerCase(), path: (f) => f.path.toLowerCase(), size: (f) => f.size, format: (f) => (f.container || "") + " " + (f.videoCodec || f.audioCodec || ""),
  created: (f) => f.created || "", duration: (f) => f.durationMs || 0, timecode: (f) => f.timecode || "", rate: (f) => f.frameRate || 0,
  camera: (f) => (f.camera || "") + " " + (f.reel || ""), scene: (f) => `${f.edits.scene || ""}|${f.edits.shot || ""}|${(f.edits.take || "").padStart(4, "0")}`,
  qc: (f) => ({ reject: 0, flag: 1, "": 2, ok: 3 })[f.edits.qc] ?? 2, drive: (f) => driveTitle(f.driveId).toLowerCase(),
};
function sortClips(list) {
  const key = CLIP_KEY[S.mediaSort] || CLIP_KEY.name, dir = S.mediaDesc ? -1 : 1;
  const cmp = (a, b) => (typeof a === "number" ? a - b : String(a).localeCompare(String(b), undefined, { numeric: true }));
  return list.sort((a, b) => dir * cmp(key(a), key(b)) || cmp(CLIP_KEY.name(a), CLIP_KEY.name(b)));
}
function renderMediaTable() {
  const body = $("#media-body");
  if (!body) return;
  const d = S.mediaData, files = sortClips(d.files.slice());
  const filtered = Object.values(S.media).some(Boolean);
  if (!files.length) {
    body.innerHTML = `<div class="empty">${ICON.clapper}<h2>${filtered ? "No clips match" : "No clip metadata yet"}</h2><p>${filtered ? "Try fewer filters." : "Lumpy reads codec, frame rate, timecode and camera from every clip it offloads or scans. For drives already in the library, open the drive and click <b>Read metadata</b>."}</p></div>`;
    return;
  }
  const total = files.reduce((a, f) => a + (f.durationMs || 0), 0);
  const bytesTotal = files.reduce((a, f) => a + f.size, 0);
  const flagged = files.filter((f) => f.edits.qc === "flag").length, rejected = files.filter((f) => f.edits.qc === "reject").length;
  const byId = Object.fromEntries(S.state.drives.map((x) => [x.id, x]));
  S.mediaOrder = files.map((f) => ({ driveId: f.driveId, path: f.path }));
  const mp = $("#m-pills"); if (mp) mp.innerHTML = `<span class="pill">Clips:<b>${files.length.toLocaleString()}${files.length >= d.limit ? "+" : ""}</b></span>
      <span class="pill">Running time:<b>${dur(total) || "—"}</b></span>
      <span class="pill">Size:<b>${bytes(bytesTotal)}</b></span>
      ${flagged ? `<span class="pill">Flagged:<b style="color:var(--warn)">${flagged}</b></span>` : ""}
      ${rejected ? `<span class="pill">Rejected:<b style="color:var(--bad)">${rejected}</b></span>` : ""}`;
  body.innerHTML = `
    ${S.mediaView === "strip" ? `<div class="strip">${files.map((f) => clipTile(f, byId[f.driveId])).join("")}</div>` : `<table class="list media-table"><thead><tr><th></th><th>Clip</th><th>Drive</th><th>Format</th><th class="num">fps</th><th class="num">Length</th><th>Start TC</th><th>Audio</th><th>Scene / take</th><th>QC</th><th>Camera</th></tr></thead><tbody>
    ${files.map((f) => {
      const dr = byId[f.driveId];
      const st = f.copies > 0 ? "full" : f.xxh64 ? "none" : "unknown";
      const name = f.path.split("/").pop(), folder = f.path.slice(0, -name.length);
      const audio = f.audioCodec ? (f.videoCodec ? `${f.channels ? f.channels + "ch " : ""}${f.sampleRate ? f.sampleRate / 1000 + "k" : ""}${f.bitDepth ? "/" + f.bitDepth : ""}` : clipFormat(f)) : "";
      const sc = [f.edits.scene && "Sc " + f.edits.scene, f.edits.shot && "Sh " + f.edits.shot, f.edits.take && "Tk " + f.edits.take].filter(Boolean).join(" ");
      return `<tr class="row" data-file="${esc(f.path)}" data-file-drive="${esc(f.driveId)}">
        <td><span class="sdot ${st}" title="${st === "full" ? `Copy on ${f.copies} other drive${f.copies === 1 ? "" : "s"}` : st === "none" ? "No copy on another drive" : "No checksum yet"}"></span></td>
        <td><div class="clip-name"><b>${esc(name)}</b><small title="${esc(f.path)}">${esc(folder)}</small></div></td>
        <td><div class="name" style="font-weight:500"><span class="dot" style="background:${dr && dr.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(dr ? dr.title : f.driveId)}${f.copies ? `<small>+${f.copies}</small>` : ""}</div></td>
        <td>${esc(f.videoCodec ? clipFormat(f) : f.kind === "audio" ? "" : (f.container || KIND_LABEL[f.kind] || ""))}${f.extra && f.extra.clip ? ` <small class="hint">${esc(f.extra.clip)}${f.extra.chunk ? " · part " + esc(f.extra.chunk) : ""}</small>` : ""}</td>
        <td class="num">${fps(f.frameRate)}</td><td class="num">${dur(f.durationMs)}</td><td class="mono">${esc(f.timecode)}</td>
        <td>${esc(audio)}</td><td>${esc(sc)}</td><td>${qcBadge(f.edits.qc, true)}</td><td>${esc(f.camera)}${f.reel ? `<small class="hint"> ${esc(f.reel)}</small>` : ""}</td></tr>`;
    }).join("")}</tbody></table>`}
    ${files.length >= d.limit ? `<p class="hint" style="margin:10px 0 0">Showing the first ${d.limit.toLocaleString()}. Filter to see the rest, or export the CSV.</p>` : ""}
    ${S.mediaView === "strip" && !S.state.ffmpeg ? `<p class="hint" style="margin:10px 0 0">Stills show here; video frames need ffmpeg: in Terminal, <code>brew install ffmpeg</code>.</p>` : ""}`;
}

// ---------- filmstrip ----------
// Four frames across each clip; moving the mouse across a tile steps
// through them.
const SCRUB = [0.1, 0.35, 0.6, 0.85];
const NO_THUMB = /\.(r3d|braw|ari|arx|crm|dng|cr2|cr3|arw|nef|raf|heic|heif|psd|exr)$/i;
function thumbURL(driveId, path, at) {
  return `/api/drives/${encodeURIComponent(driveId)}/thumb?path=${encodeURIComponent(path)}&at=${(at || 0).toFixed(1)}&t=${encodeURIComponent(token)}`;
}
function clipTile(f, dr) {
  const name = f.path.split("/").pop();
  const connected = !!(dr && dr.connected);
  const st = f.copies > 0 ? "full" : f.xxh64 ? "none" : "unknown";
  // clipFormat already carries the frame size for video.
  const line = [f.videoCodec ? clipFormat(f) : f.container || KIND_LABEL[f.kind] || "", !f.videoCodec && f.width ? `${f.width}×${f.height}` : "", f.frameRate ? fps(f.frameRate) + " fps" : ""].filter(Boolean).join(" · ");
  const secs = (f.durationMs || 0) / 1000;
  const can = connected && (f.kind === "video" || f.kind === "image") && !NO_THUMB.test(name);
  const pic = can ? `<img loading="lazy" alt="" src="${thumbURL(f.driveId, f.path, secs * SCRUB[0])}" data-pos="0">`
    : `<div class="noimg">${ICON.film}<span>${connected ? f.kind === "audio" ? "Audio" : "No preview" : "Plug in " + esc(dr ? dr.title : f.driveId)}</span></div>`;
  return `<div class="clip-tile" data-file="${esc(f.path)}" data-file-drive="${esc(f.driveId)}" title="${esc(f.path)}">
    <div class="thumb" data-dur="${secs > 0 && f.kind === "video" ? secs : ""}">${pic}${f.timecode ? `<span class="tc mono">${esc(f.timecode)}</span>` : ""}${f.durationMs ? `<span class="dur">${dur(f.durationMs)}</span>` : ""}<i class="scrub"></i></div>
    <div class="meta"><b>${esc(name)}</b><small>${esc(line) || "&nbsp;"}</small><small><span class="sdot ${st}"></span>${esc(dr ? dr.title : f.driveId)}${f.copies ? ` <span class="hint">+${f.copies}</span>` : ""}${f.edits.qc ? qcBadge(f.edits.qc, true) : ""}</small></div></div>`;
}
document.addEventListener("mousemove", (e) => {
  const th = e.target.closest && e.target.closest(".clip-tile .thumb");
  if (!th || !th.dataset.dur) return;
  const img = th.querySelector("img");
  if (!img) return;
  const rect = th.getBoundingClientRect();
  const i = Math.max(0, Math.min(SCRUB.length - 1, Math.floor(((e.clientX - rect.left) / rect.width) * SCRUB.length)));
  th.querySelector(".scrub").style.left = `${(i / SCRUB.length) * 100}%`;
  if (img.dataset.pos === String(i)) return;
  img.dataset.pos = String(i);
  const tile = th.closest(".clip-tile");
  img.src = thumbURL(tile.dataset.fileDrive, tile.dataset.file, +th.dataset.dur * SCRUB[i]);
});
document.addEventListener("mouseout", (e) => {
  const th = e.target.closest && e.target.closest(".clip-tile .thumb");
  if (!th || th.contains(e.relatedTarget)) return;
  const img = th.querySelector("img");
  if (!img || !th.dataset.dur || img.dataset.pos === "0") return;
  img.dataset.pos = "0";
  const tile = th.closest(".clip-tile");
  img.src = thumbURL(tile.dataset.fileDrive, tile.dataset.file, +th.dataset.dur * SCRUB[0]);
});
// A frame that can't be made (ffmpeg missing, an unreadable file)
// becomes a placeholder rather than a broken image.
document.addEventListener("error", (e) => {
  const img = e.target;
  if (!img || img.tagName !== "IMG" || !img.closest(".clip-tile")) return;
  img.replaceWith(Object.assign(document.createElement("div"), { className: "noimg", innerHTML: `${ICON.film}<span>No preview</span>` }));
}, true);

// ---------- card offers: a card is in, and a project wants it ----------
S.offered = new Set();
function showOffers() {
  const offers = S.state.offers || [];
  const o = offers.find((x) => !S.offered.has(x.id));
  if (!o || !$("#sheet").hidden) return;
  S.offered.add(o.id);
  const names = o.dests.map((d) => { const v = (S.state.volumes || []).find((x) => d.startsWith(x.mount + "/") || d === x.mount); return v ? v.name + d.slice(v.mount.length) : d; });
  openSheet(`<div class="sheet-head"><span class="n-tag" style="--pc:${COLORS[o.color] || "var(--accent)"}">${ICON.plug}</span><div><h2>${esc(o.card)} is in</h2><div class="sub">${bytes(o.bytes)} on it · ${esc(o.project)} watches for cards</div></div><div class="actions"><button class="icon-btn" id="of-close" title="Not now">${ICON.x}</button></div></div>
    <div class="sheet-body">
      ${o.missing.length ? `<div class="alert warn"><b>${esc(o.missing.join(" and "))} ${o.missing.length === 1 ? "is" : "are"} not plugged in.</b> ${o.dests.length ? "Lumpy can copy to the rest now, or wait." : "Plug it in and the offer comes back."}</div>` : ""}
      ${o.dests.length ? `<p>Copy everything on <b>${esc(o.card)}</b> to:</p><ul class="plain-list">${names.map((n) => `<li class="mono">${esc(n)}</li>`).join("")}</ul><p class="hint">Each copy is read back and checksummed; the card is never written to.</p>` : ""}
    </div>
    <div class="sheet-foot"><button class="btn" id="of-no">Not now</button>${o.dests.length ? `<button class="btn primary" id="of-go">${ICON.copy}Offload now</button>` : ""}</div>`, true);
  const drop = async () => { closeSheet(); try { await api(`/api/offers/${encodeURIComponent(o.id)}`, { method: "DELETE" }); } catch {} };
  $("#of-close").onclick = $("#of-no").onclick = drop;
  const go = $("#of-go"); if (go) go.onclick = async () => { go.disabled = true; try { await api(`/api/offers/${encodeURIComponent(o.id)}/go`, { method: "POST" }); closeSheet(); toast(`Offloading ${o.card}; watch Activity`); loadJobs(); } catch (e) { toast(e.message, true); go.disabled = false; } };
}

// ---------- the card sheet: can I format this? ----------
async function openCard(id) {
  let c;
  try { c = await api(`/api/drives/${encodeURIComponent(id)}/card`); } catch (e) { toast(e.message, true); return; }
  const v = c.verdict;
  const big = v === "safe" ? `<div class="card-verdict ok">${ICON.check}<b>Safe to format</b><span>Every one of ${c.files.toLocaleString()} files has at least ${c.minVerified} verified copies on other drives.</span></div>`
    : v === "once" ? `<div class="card-verdict warn">${ICON.warn}<b>Backed up once</b><span>${esc(c.why.join(". "))}. Make another copy before formatting.</span></div>`
    : `<div class="card-verdict bad">${ICON.warn}<b>Keep this card</b><span>${esc(c.why.join(". "))}.</span></div>`;
  openSheet(`<div class="sheet-head"><div><h2>${esc(c.drive)}</h2><div class="sub">${c.files.toLocaleString()} files · ${bytes(c.bytes)}${c.clips ? ` · ${c.clips.toLocaleString()} clips` : ""} · ${c.hashed.toLocaleString()} checksummed</div></div><div class="actions"><button class="icon-btn" id="cd-close" title="Close">${ICON.x}</button></div></div>
    <div class="sheet-body">
      ${big}
      <div class="sec"><div class="sec-title">${ICON.copy}Copies</div><div class="box">
        ${c.holders.length ? `<table class="list compact"><thead><tr><th>On</th><th class="num">Files</th><th class="num">Verified</th><th></th></tr></thead><tbody>${c.holders.map((h) => `<tr><td><div class="name"><span class="dot" style="background:${h.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(h.drive)}</div></td><td class="num">${h.files.toLocaleString()}</td><td class="num ${h.verified === h.files ? "ok" : "warn"}">${h.verified.toLocaleString()}</td><td class="hint">${h.verified === h.files ? "all read back and matched" : h.verified ? `${(h.files - h.verified).toLocaleString()} not read back yet` : "copied, not checked"}</td></tr>`).join("")}</tbody></table>` : `<p class="hint" style="margin:0">No copies of this card's files anywhere else in the library.</p>`}
      </div></div>
      ${c.short.length ? `<div class="sec"><div class="sec-title">${ICON.warn}Files without two verified copies<span class="count">${c.short.length}</span></div><div class="box"><table class="list compact"><tbody>${c.short.slice(0, 200).map((f) => `<tr><td class="mono" style="word-break:break-all">${esc(f.path)}</td><td class="num">${f.damaged ? '<span class="bad">damaged here</span>' : f.unhashed ? '<span class="warn">no checksum</span>' : f.verified ? `${f.verified} verified${f.copies > f.verified ? ` · ${f.copies - f.verified} unchecked` : ""}` : f.copies ? `${f.copies} unchecked` : f.likely ? `${f.likely} likely` : '<span class="bad">none</span>'}</td></tr>`).join("")}</tbody></table>${c.short.length > 200 ? `<p class="hint">and ${c.short.length - 200} more</p>` : ""}</div></div>` : ""}
      <div class="sec"><div class="sec-title">${ICON.trash}Erase the card</div><div class="box">
        <p class="hint" style="margin:0 0 10px">${v === "safe" ? "Formats the card in place, keeping its name and filesystem, the way the camera would. Type the card's name to confirm." : "Unlocks once every file has two verified copies on two other drives."}</p>
        <div class="path-row"><input class="path-input" id="cd-name" placeholder="${esc(c.drive)}" ${v === "safe" && c.connected && !c.busy ? "" : "disabled"}><button class="btn danger" id="cd-erase" disabled>${ICON.trash}Erase ${esc(c.drive)}</button></div>
        ${!c.connected ? '<p class="hint" style="margin:8px 0 0">Plug the card in to erase it.</p>' : c.busy ? '<p class="hint" style="margin:8px 0 0">A job is running on it; wait for it.</p>' : ""}
      </div></div>
    </div>`);
  $("#cd-close").onclick = closeSheet;
  const nm = $("#cd-name"), er = $("#cd-erase");
  nm.oninput = () => { er.disabled = nm.value.trim() !== c.drive; };
  er.onclick = async () => {
    er.disabled = true; er.textContent = "Erasing…";
    try { await api(`/api/drives/${encodeURIComponent(id)}/erase`, { method: "POST", body: { name: nm.value.trim() } }); toast(`Erasing ${c.drive}; watch Activity`); closeSheet(); loadJobs(); }
    catch (e) { toast(e.message, true); er.disabled = false; er.innerHTML = `${ICON.trash}Erase ${esc(c.drive)}`; }
  };
}

// ---------- file sheet ----------
async function openFile(driveId, path, back) {
  let data;
  try { data = await api(`/api/drives/${encodeURIComponent(driveId)}/file?path=${encodeURIComponent(path)}`); }
  catch (e) { toast(e.message, true); return; }
  S.detail = null;
  S.file = { driveId, path, back: back || (S.file && S.file.back) };
  const f = data.file, m = data.media, st = data.state || {}, dr = data.drive;
  const name = path.split("/").pop();
  const bst = st.Damaged ? "damaged" : st.Copies > 0 ? "full" : st.Unknown ? "unknown" : "none";
  const stream = `/api/drives/${encodeURIComponent(driveId)}/stream?path=${encodeURIComponent(path)}&t=${encodeURIComponent(token)}`;
  const kind = (m && m.kind) || (/\.(mov|mp4|m4v|mxf|r3d|braw|avi)$/i.test(name) ? "video" : /\.(wav|aif|aiff|mp3|m4a|flac)$/i.test(name) ? "audio" : /\.(jpe?g|png|gif|heic|webp|tiff?)$/i.test(name) ? "image" : "other");
  const px = data.proxy, pxStream = px ? `/api/drives/${encodeURIComponent(px.driveId)}/stream?path=${encodeURIComponent(px.path)}&t=${encodeURIComponent(token)}` : "";
  const rawNote = /\.(crm|braw|r3d|ari|arx|cine)$/i.test(name) ? `${name.split(".").pop().toUpperCase()} has no player without the camera maker's SDK` : "No preview for this file type";
  const preview = data.playable
    ? (kind === "video" ? playerHTML(stream, m, dr.connected && driveOf(driveId) ? driveOf(driveId).mount + "/" + path : "", { driveId, path }) : kind === "audio" ? `<div class="audio-box">${ICON.clapper}<audio controls preload="metadata" src="${stream}"></audio></div>` : `<img src="${stream}" alt="">`)
    : px ? playerHTML(pxStream, m, driveOf(px.driveId) ? driveOf(px.driveId).mount + "/" + px.path : "", { driveId, path }) + `<div class="proxy-note">${ICON.info}Playing the proxy <b>${esc(px.path.split("/").pop())}</b> on ${esc(px.drive)} · ${esc(rawNote)}</div>`
    : `<div class="no-preview">${ICON.film}<span>${dr.connected ? rawNote : `Plug in ${esc(dr.title)} to preview`}</span></div>`;
  const kv = [];
  if (m) {
    if (m.container) kv.push(["Container", m.container + (m.source && m.source !== "extension" ? ` <span class="hint">read by ${esc(m.source)}</span>` : "")]);
    if (m.videoCodec) kv.push(["Video", `${esc(m.videoCodec)}${m.width ? ` · ${m.width}×${m.height}` : ""}${m.frameRate ? ` · ${fps(m.frameRate)} fps` : ""}`]);
    else if (m.frameRate) kv.push(["Frame rate", fps(m.frameRate) + " fps"]);
    if (m.durationMs) kv.push(["Duration", dur(m.durationMs) + (m.frameRate ? ` <span class="hint">${Math.round(m.durationMs / 1000 * m.frameRate).toLocaleString()} frames</span>` : "")]);
    if (m.timecode) kv.push(["Start timecode", `<span class="mono">${esc(m.timecode)}</span>${m.timecode.includes(";") ? ' <span class="hint">drop frame</span>' : ""}`]);
    if (m.audioCodec) kv.push(["Audio", `${esc(m.audioCodec)}${m.channels ? ` · ${m.channels} ch` : ""}${m.sampleRate ? ` · ${m.sampleRate / 1000} kHz` : ""}${m.bitDepth ? ` · ${m.bitDepth}-bit` : ""}`]);
    if (m.camera) kv.push(["Camera", esc(m.camera)]);
    if (m.reel) kv.push(["Reel", esc(m.reel)]);
    if (m.created) kv.push(["Created", esc(m.created)]);
    for (const [k, v] of Object.entries(m.extra || {})) kv.push([esc(k), esc(v)]);
  }
  openSheet(`
    <div class="sheet-head">
      <span class="drive-ico file-ico">${ICON.file}</span>
      <div><h2>${esc(name)}</h2><div class="sub">${esc(dr.title)} › ${esc(path)}</div></div>
      <div class="actions">
        ${dr.connected && native ? `<button class="btn small" id="reveal">${ICON.reveal}Show in Finder</button>` : ""}
        <button class="icon-btn" id="close" title="${S.file.back ? "Back" : "Close"}">${S.file.back ? ICON.back : ICON.x}</button>
      </div>
    </div>
    <div class="sheet-body">
      <div class="preview">${preview}</div>

      <div class="sec"><div class="sec-title">${ICON.shield}Backup</div>
        <div class="box">
          <div class="name" style="display:flex;align-items:center;gap:8px;font-weight:600"><span class="sdot ${bst}"></span>${bst === "full" ? `Copy on ${st.Copies} other drive${st.Copies === 1 ? "" : "s"}` : bst === "damaged" ? "Failed its last check on this drive" : bst === "unknown" ? "No checksum yet" : "No copy on another drive"}</div>
          ${data.copies.length ? `<table class="list compact" style="margin-top:8px"><tbody>${data.copies.map((c) => { const cd = driveOf(c.driveId); return `<tr class="row" data-file="${esc(c.path)}" data-file-drive="${esc(c.driveId)}"><td><div class="name" style="font-weight:500"><span class="dot" style="background:${cd && cd.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(c.drive)}</div></td><td class="mono">${esc(c.path)}</td><td>${c.verified ? `<span class="badge ok">${ICON.check}read back</span>` : ""}</td></tr>`; }).join("")}</tbody></table>` : ""}
          <dl class="kv" style="margin-top:10px"><dt>Size</dt><dd>${bytes(f.Size)} <span class="hint">${f.Size.toLocaleString()} bytes</span></dd><dt>xxh64</dt><dd class="mono">${esc(f.XXH64 || "—")}</dd>${f.VerifiedAt && !f.VerifiedAt.startsWith("0001") ? `<dt>Read back</dt><dd>${new Date(f.VerifiedAt).toLocaleString()}</dd>` : ""}</dl>
        </div></div>

      <div class="sec"><div class="sec-title">${ICON.clapper}Clip
          <div class="right">${dr.connected ? `<button class="btn small" id="reprobe">${ICON.refresh}${m ? "Read again" : "Read metadata"}</button>` : ""}</div></div>
        <div class="box">${kv.length ? `<dl class="kv">${kv.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join("")}</dl>`
          : `<span class="hint">${m ? "Nothing readable inside this file." : dr.connected ? "Not read yet." : `Not read yet. Plug in ${esc(dr.title)} to read it.`}</span>`}</div></div>

      <div class="sec"><div class="sec-title">${ICON.edit}Log <span class="hint" style="font-weight:400">kept with every copy of this clip, and in the CSV for post</span></div>
        <div class="box"><div class="fields">
          <div class="field"><label>Scene</label><input data-edit="scene" value="${esc(m ? m.edits.scene : "")}"></div>
          <div class="field"><label>Shot</label><input data-edit="shot" value="${esc(m ? m.edits.shot : "")}"></div>
          <div class="field"><label>Take</label><input data-edit="take" value="${esc(m ? m.edits.take : "")}"></div>
          <div class="field"><label>Tags</label><input data-edit="tags" value="${esc(m ? m.edits.tags : "")}" placeholder="circle, vfx, mos"></div>
          <div class="field wide"><label>QC</label><div class="seg qc" id="qc">${["", "ok", "flag", "reject"].map((q) => `<button data-qc="${q}" class="${(m ? m.edits.qc : "") === q ? "on" : ""} q-${q || "none"}">${q === "" ? "Not checked" : q === "ok" ? "OK" : q === "flag" ? "Flag" : "Reject"}</button>`).join("")}</div></div>
          <div class="field wide"><label>Notes</label><textarea data-edit="notes" placeholder="Good for sound · boom in shot at 00:12 · director's pick…">${esc(m ? m.edits.notes : "")}</textarea></div>
        </div></div></div>
    </div>`);
  if (kind === "video" || kind === "image") $("#sheet").classList.add("wide");
  wireFile(driveId, path, m);
  if (kind === "video" && (data.playable || data.proxy)) wirePlayer(m);
}
// playerHTML is the clip player: the video with Lumpy's own transport
// under it, the way an edit suite's viewer works. J K L shuttle, space
// plays, , and . step a frame (⇧ ten), ← → go to the previous and
// next clip in the list, - and = change speed, F fills the screen.
function playerHTML(stream, m, file, clip) {
  const rate = (m && m.frameRate) || 0;
  return `<div class="player" id="player" tabindex="0" data-local="${esc(file || "")}" data-clip-drive="${esc(clip ? clip.driveId : "")}" data-clip-path="${esc(clip ? clip.path : "")}">
    ${native && file ? `<div class="native-slot" style="aspect-ratio:${m && m.width && m.height ? `${m.width} / ${m.height}` : "16 / 9"}"></div>` : `<video preload="auto" playsinline src="${stream}"></video>`}
    <div class="transport">
      <button class="icon-btn" data-pl="prev" title="Previous clip (←)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 5l-7 7 7 7"/></svg></button>
      <button class="icon-btn" data-pl="rev" title="Reverse (J)"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M11 6v12L2 12zM22 6v12l-9-6z"/></svg></button>
      <button class="icon-btn play" data-pl="play" title="Play / pause (space, K) · - and = step a frame">${ICON.play}</button>
      <button class="icon-btn" data-pl="fwd" title="Forward, faster each press (L)"><svg viewBox="0 0 24 24" fill="currentColor"><path d="M13 6v12l9-6zM2 6v12l9-6z"/></svg></button>
      <button class="icon-btn" data-pl="next" title="Next clip (→)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 5l7 7-7 7"/></svg></button>
      <span class="tc" id="pl-tc" title="Timecode · frame">${rate && m.timecode ? esc(m.timecode) : "00:00:00:00"}</span>
      <input type="range" class="scrub" id="pl-scrub" min="0" max="1000" value="0" step="1">
      <span class="tc dur" id="pl-dur">${m && m.durationMs ? esc(dur(m.durationMs)) : "—"}</span>
      <span class="speed" id="pl-speed" title="Speed ([ and ])">1×</span>
      <button class="icon-btn" data-pl="mute" title="Mute (M)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 9v6h4l5 4V5L8 9zM16 9a4 4 0 0 1 0 6"/></svg></button>
      ${clip ? `<button class="icon-btn" data-pl="grab" title="Grab a still of this frame, full size">${ICON.camera}</button><button class="btn small stills-btn" data-pl="stills" title="Stills grabbed from this clip" hidden>Stills <b>0</b></button>` : ""}
      ${native && file ? `<button class="icon-btn" data-pl="qt" title="Open in QuickTime Player">${ICON.clapper}</button>` : ""}
      <button class="icon-btn" data-pl="full" title="Fill the screen (F)"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/></svg></button>
    </div><div class="stills" id="pl-stills" hidden></div></div>`;
}
// A player backend: the page's own <video>, or the app's AVPlayer laid
// over the page (player.swift). The transport drives either through
// the same few calls, and both tick it with the clip's clock.
function htmlBackend(v, muted) {
  v.muted = muted;
  const B = { native: false, canReverse: false, get time() { return v.currentTime; }, get duration() { return v.duration || 0; }, get rate() { return v.paused ? 0 : v.playbackRate; } };
  B.play = (rate) => { v.playbackRate = Math.abs(rate) || 1; const p = v.play(); if (p && p.catch) p.catch(() => {}); };
  B.pause = () => v.pause();
  B.seek = (t) => { v.currentTime = Math.max(0, Math.min(B.duration || t, t)); };
  B.step = (n, fps) => { v.pause(); B.seek(v.currentTime + n / fps); };
  B.mute = (on) => { v.muted = on; };
  B.fullscreen = (box) => { if (box.requestFullscreen) box.requestFullscreen(); };
  B.close = () => {};
  B.grab = async () => {
    // A clip that has only just opened may not have decoded a frame yet.
    if (!v.videoWidth) await new Promise((ok) => { const t = setTimeout(ok, 3000); v.addEventListener("loadeddata", () => { clearTimeout(t); ok(); }, { once: true }); });
    if (!v.videoWidth) return { error: "no frame to grab yet" };
    const c = document.createElement("canvas");
    c.width = v.videoWidth; c.height = v.videoHeight;
    c.getContext("2d").drawImage(v, 0, 0, c.width, c.height);
    const blob = await new Promise((ok) => c.toBlob(ok, "image/png"));
    return blob ? { blob, width: c.width, height: c.height } : { error: "could not read the frame" };
  };
  // Every presented frame ticks the transport where the browser offers
  // that; timeupdate alone is four times a second and looks jumpy.
  const emit = () => { if (B.ontick) B.ontick(); };
  if (v.requestVideoFrameCallback) { const loop = () => { emit(); if (v.isConnected) v.requestVideoFrameCallback(loop); }; v.requestVideoFrameCallback(loop); }
  v.ontimeupdate = emit; v.onplay = emit; v.onpause = emit; v.onseeked = emit; v.onended = emit;
  v.onloadedmetadata = () => { if (B.onready) B.onready({ duration: v.duration, fps: 0, width: v.videoWidth, height: v.videoHeight }); emit(); };
  v.onclick = () => { if (B.onclick) B.onclick(1); };
  v.ondblclick = () => { if (B.onclick) B.onclick(2); };
  return B;
}
let playerSeq = 0;
function nativeBackend(box, slot, muted) {
  const pid = ++playerSeq;
  const B = { native: true, canReverse: false, time: 0, duration: 0, rate: 0, pid, closed: false };
  let shown = true, lastRect = "";
  const rectOf = () => { const r = slot.getBoundingClientRect(); return { x: r.left, y: r.top, w: r.width, h: r.height }; };
  const sendRect = () => {
    if (B.closed) return;
    const r = rectOf(), k = [r.x, r.y, r.w, r.h].map((n) => Math.round(n * 2) / 2).join();
    if (k !== lastRect) { lastRect = k; nativeCall("player.rect", { pid, rect: r }); }
  };
  window.lumpyPlayer = {
    pid,
    tick(id, t, rate) { if (id !== pid) return; B.time = t; B.rate = rate; if (B.ontick) B.ontick(); },
    ready(id, dur, fps, w, h, canRev) {
      if (id !== pid) return;
      B.duration = dur; B.canReverse = !!canRev;
      if (w && h) slot.style.aspectRatio = `${w} / ${h}`;
      sendRect();
      if (B.onready) B.onready({ duration: dur, fps, width: w, height: h });
    },
    ended(id) { if (id !== pid) return; B.rate = 0; if (B.ontick) B.ontick(); },
    failed(id, why) { if (id !== pid) return; toast("Can't play this clip: " + why, true); },
    click(id, n) { if (id !== pid) return; if (B.onclick) B.onclick(n); },
  };
  nativeCall("player.open", { pid, path: box.dataset.local, rect: rectOf(), muted: !!muted });
  const ro = new ResizeObserver(sendRect);
  ro.observe(slot);
  window.addEventListener("resize", sendRect);
  document.addEventListener("scroll", sendRect, true);
  // Every so often: gone from the page, close; under a sheet, hide.
  const timer = setInterval(() => {
    if (!slot.isConnected) { B.close(); return; }
    const sheet = $("#sheet"), covered = !sheet.hidden && !sheet.contains(slot);
    if (covered === shown) { shown = !covered; nativeCall("player.show", { pid, on: shown }); }
    sendRect();
  }, 200);
  B.play = (rate) => nativeCall("player.play", { rate });
  B.pause = () => nativeCall("player.pause");
  B.seek = (t, fast) => nativeCall("player.seek", { t: Math.max(0, Math.min(B.duration || t, t)), fast: !!fast });
  B.step = (n) => nativeCall("player.step", { n });
  B.mute = (on) => nativeCall("player.mute", { on });
  B.fullscreen = (box) => { box.classList.toggle("fill"); sendRect(); };
  B.grab = () => nativeCall("player.grab");
  B.close = () => {
    if (B.closed) return;
    B.closed = true;
    clearInterval(timer); ro.disconnect();
    window.removeEventListener("resize", sendRect); document.removeEventListener("scroll", sendRect, true);
    if (window.lumpyPlayer && window.lumpyPlayer.pid === pid) window.lumpyPlayer = null;
    nativeCall("player.close", { pid });
  };
  return B;
}
// closePlayers shuts any player the page is about to throw away, so
// the app's own player view never outlives its spot on the page.
function closePlayers() {
  for (const el of document.querySelectorAll("#player")) if (el.player) el.player.close();
}
function wirePlayer(m) {
  const box = $("#player"), v = box && box.querySelector("video"), slot = box && box.querySelector(".native-slot");
  if (!v && !slot) return;
  let rate = (m && m.frameRate) || 25;
  const startFrames = m && m.timecode ? tcFrames(m.timecode, rate) : 0;
  const B = slot ? nativeBackend(box, slot, !!S.playerMuted) : htmlBackend(v, !!S.playerMuted);
  box.player = B;
  const scrub = $("#pl-scrub"), tc = $("#pl-tc"), speedEl = $("#pl-speed"), playBtn = box.querySelector("[data-pl=play]");
  let speed = 1, dir = 1, rev = null; // rev: the timer stepping backwards where the backend can't
  const stopRev = () => { if (rev) { cancelAnimationFrame(rev); rev = null; } };
  const playing = () => B.rate !== 0 || !!rev;
  // Only what changed is written: the player ticks many times a second
  // and every write here is a repaint beside the video.
  let lastTc = "", lastScrub = -1, lastPlaying = null;
  const show = () => {
    const t = B.time || 0, f = Math.round(t * rate);
    const s = tcString(startFrames + f, rate) + ` · ${f}`;
    if (s !== lastTc) { tc.textContent = s; lastTc = s; }
    if (B.duration) { const sv = Math.round((t / B.duration) * 1000); if (sv !== lastScrub) { scrub.value = sv; lastScrub = sv; } }
    const on = playing();
    if (on !== lastPlaying) { playBtn.innerHTML = on ? `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>` : ICON.play; lastPlaying = on; }
    box.dataset.busy = on || t > 0 || !$("#pl-stills").hidden ? "1" : "";
  };
  const speedText = () => { speedEl.textContent = (dir < 0 ? "-" : "") + (speed % 1 ? speed.toFixed(2).replace(/0+$/, "") : speed) + "×"; };
  const play = (d) => { stopRev(); dir = d; speedText(); B.play(d * speed); };
  const pause = () => { stopRev(); B.pause(); speed = 1; dir = 1; speedText(); show(); };
  const step = (n) => { stopRev(); speed = 1; dir = 1; speedText(); B.step(n, rate); };
  const reverse = () => {
    if (B.canReverse) { speed = dir < 0 && playing() ? Math.min(8, speed * 2) : 1; play(-1); return; }
    // No reverse decode here: step frames back on a clock instead.
    B.pause();
    speed = rev ? Math.min(8, speed * 2) : 1; dir = -1; speedText();
    stopRev();
    let last = performance.now();
    const tick = (now) => { B.seek(Math.max(0, B.time - ((now - last) / 1000) * speed)); last = now; show(); if (B.time <= 0) { stopRev(); speed = 1; dir = 1; speedText(); } else rev = requestAnimationFrame(tick); };
    rev = requestAnimationFrame(tick);
  };
  const forward = () => { speed = dir > 0 && playing() ? Math.min(8, speed * 2) : 1; play(1); };
  const setSpeed = (x) => { speed = Math.max(0.25, Math.min(8, x)); speedText(); if (playing() && !rev) B.play(dir * speed); };
  const nav = fileNav;
  B.ontick = show;
  B.onready = (info) => { if (info.fps && !(m && m.frameRate)) rate = info.fps; if (!m || !m.durationMs) $("#pl-dur").textContent = dur((info.duration || 0) * 1000); show(); };
  B.onclick = (n) => { if (n >= 2) box.querySelector("[data-pl=full]").click(); else if (playing()) pause(); else play(1); };
  scrub.oninput = () => { if (B.duration) { stopRev(); B.seek((scrub.value / 1000) * B.duration, true); } };
  scrub.onchange = () => { if (B.duration) B.seek((scrub.value / 1000) * B.duration); };
  box.querySelector("[data-pl=play]").onclick = () => (playing() ? pause() : play(1));
  box.querySelector("[data-pl=rev]").onclick = reverse;
  box.querySelector("[data-pl=fwd]").onclick = forward;
  box.querySelector("[data-pl=prev]").onclick = () => nav(-1);
  box.querySelector("[data-pl=next]").onclick = () => nav(1);
  const muteBtn = box.querySelector("[data-pl=mute]");
  muteBtn.classList.toggle("on", !!S.playerMuted);
  const mute = () => { S.playerMuted = !S.playerMuted; B.mute(S.playerMuted); muteBtn.classList.toggle("on", S.playerMuted); };
  muteBtn.onclick = mute;
  const qt = box.querySelector("[data-pl=qt]"); if (qt) qt.onclick = () => { pause(); nativeCall("quicktime", { path: box.dataset.local }); };
  wireStills(box, B, () => { pause(); const f = Math.round((B.time || 0) * rate); return { tc: tcString(startFrames + f, rate), frame: f }; });
  box.querySelector("[data-pl=full]").onclick = () => B.fullscreen(box);
  const keys = (e) => {
    if (!box.isConnected) { document.removeEventListener("keydown", keys); return; }
    if (!$("#sheet").hidden && !$("#sheet").contains(box)) return; // a sheet is open over the page's player
    if (e.target.matches("input:not([type=range]), textarea, select")) return;
    const k = e.key.toLowerCase();
    if (k === " ") { e.preventDefault(); playing() ? pause() : play(1); }
    else if (k === "k") pause();
    else if (k === "l") forward();
    else if (k === "j") reverse();
    else if (k === "," || k === "-" || k === "_") step(e.shiftKey ? -10 : -1);
    else if (k === "." || k === "=" || k === "+") step(e.shiftKey ? 10 : 1);
    else if (k === "home") { pause(); B.seek(0); }
    else if (k === "end") { pause(); B.seek(B.duration || 0); }
    else if (k === "[") setSpeed(speed / 2);
    else if (k === "]") setSpeed(speed * 2);
    else if (k === "m") mute();
    else if (k === "f") B.fullscreen(box);
    else if (k === "escape" && box.classList.contains("fill")) B.fullscreen(box);
    else return;
    e.stopPropagation();
  };
  document.addEventListener("keydown", keys);
  speedText();
  show();
}

// wireStills is the camera in the transport: grab the frame on screen
// at the clip's own size as a PNG, send it to Lumpy to file, and keep
// the clip's grabs in a drop-down with Show in Finder on each.
function wireStills(box, B, atFrame) {
  const grab = box.querySelector("[data-pl=grab]"), btn = box.querySelector("[data-pl=stills]"), list = box.querySelector("#pl-stills");
  if (!grab || !btn) return;
  const driveId = box.dataset.clipDrive, path = box.dataset.clipPath;
  const q = `path=${encodeURIComponent(path)}`;
  let stills = [];
  const draw = () => {
    btn.hidden = !stills.length;
    btn.querySelector("b").textContent = stills.length;
    if (!stills.length) { list.hidden = true; list.innerHTML = ""; return; }
    list.innerHTML = `<div class="stills-head"><b>Stills from this clip</b><span class="hint">${esc(stills[0].file.split("/").slice(0, -1).join("/"))}</span>${native ? `<button class="btn small" data-still-dir="${esc(stills[0].file)}">${ICON.reveal}Show folder</button>` : ""}</div>` +
      stills.map((st) => `<div class="still-row" data-still="${st.id}"><img src="/api/stills/${st.id}/file?t=${encodeURIComponent(token)}" alt="" loading="lazy"><div class="still-meta"><b>${esc(st.file.split("/").pop())}</b><small>${esc(st.tc || "frame " + st.frame)} · ${st.width}×${st.height}</small></div>${native ? `<button class="icon-btn" data-still-reveal="${esc(st.file)}" title="Show in Finder">${ICON.reveal}</button>` : ""}<button class="icon-btn" data-still-drop="${st.id}" title="Delete this still">${ICON.trash}</button></div>`).join("");
    for (const b of list.querySelectorAll("[data-still-reveal]")) b.onclick = (e) => { e.stopPropagation(); nativeCall("reveal", { path: b.dataset.stillReveal }); };
    for (const b of list.querySelectorAll("[data-still-dir]")) b.onclick = (e) => { e.stopPropagation(); nativeCall("reveal", { path: b.dataset.stillDir }); };
    for (const b of list.querySelectorAll("[data-still-drop]")) b.onclick = async (e) => {
      e.stopPropagation();
      try { await api(`/api/stills/${b.dataset.stillDrop}`, { method: "DELETE" }); stills = stills.filter((x) => String(x.id) !== b.dataset.stillDrop); draw(); }
      catch (err) { toast(err.message, true); }
    };
    for (const row of list.querySelectorAll(".still-row")) row.ondblclick = () => { const b = row.querySelector("[data-still-reveal]"); if (b) b.click(); };
  };
  const load = async () => { try { stills = await api(`/api/drives/${encodeURIComponent(driveId)}/stills?${q}`); } catch { stills = []; } draw(); };
  btn.onclick = () => { list.hidden = !list.hidden; };
  grab.onclick = async () => {
    const at = atFrame();
    grab.disabled = true;
    try {
      const g = await B.grab();
      if (!g || g.error) throw new Error((g && g.error) || "no frame");
      const url = `/api/drives/${encodeURIComponent(driveId)}/still?${q}&tc=${encodeURIComponent(at.tc)}&frame=${at.frame}`;
      // The app renders the frame itself into a temp file and the engine
      // files it; the page's own player hands over the PNG it drew.
      const res = g.file
        ? await fetch(url + `&from=${encodeURIComponent(g.file)}`, { method: "POST", headers: { "X-Lumpy-Token": token } })
        : await fetch(url, { method: "POST", headers: { "X-Lumpy-Token": token, "Content-Type": "image/png" }, body: g.blob });
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error((data && data.error) || res.statusText);
      stills.unshift(data);
      draw();
      list.hidden = false;
      toast(`Still saved: ${data.file.split("/").pop()} (${data.width}×${data.height})`);
    } catch (e) { toast("Still not saved: " + e.message, true); }
    grab.disabled = false;
  };
  load();
}
// fileNav opens the previous or next clip in the order the Clips list shows.
function fileNav(dir) {
  if (!S.file) return;
  const list = S.mediaOrder || [], i = list.findIndex((x) => x.driveId === S.file.driveId && x.path === S.file.path);
  if (i < 0 || !list[i + dir]) { toast(dir > 0 ? "Last clip in the list" : "First clip in the list"); return; }
  openFile(list[i + dir].driveId, list[i + dir].path, S.file.back);
}
document.addEventListener("keydown", (e) => {
  if (!S.file || $("#sheet").hidden || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
  if (e.target.matches("input:not([type=range]), textarea, select")) return;
  e.preventDefault(); fileNav(e.key === "ArrowRight" ? 1 : -1);
});
function tcFrames(tc, rate) {
  const p = String(tc).split(/[:;]/).map(Number);
  if (p.length !== 4 || p.some(isNaN)) return 0;
  return ((p[0] * 60 + p[1]) * 60 + p[2]) * Math.round(rate) + p[3];
}
function tcString(frames, rate) {
  const r = Math.round(rate) || 25, f = frames % r, s = Math.floor(frames / r);
  const pad = (n) => String(n).padStart(2, "0");
  return `${pad(Math.floor(s / 3600))}:${pad(Math.floor(s / 60) % 60)}:${pad(s % 60)}:${pad(f)}`;
}
function closeFile() {
  const back = S.file && S.file.back;
  closeSheet();
  if (back) openDetail(back);
}
function wireFile(driveId, path, m) {
  const edits = { ...(m ? m.edits : { scene: "", shot: "", take: "", notes: "", tags: "", qc: "" }) };
  const save = async (patch) => {
    Object.assign(edits, patch);
    try { await api(`/api/drives/${encodeURIComponent(driveId)}/file?path=${encodeURIComponent(path)}`, { method: "PATCH", body: edits }); if (S.mediaData) loadMedia(); }
    catch (e) { toast(e.message, true); }
  };
  $("#close").onclick = closeFile;
  for (const el of document.querySelectorAll("[data-edit]")) el.onchange = () => save({ [el.dataset.edit]: el.value });
  for (const b of document.querySelectorAll("#qc button")) b.onclick = () => { document.querySelectorAll("#qc button").forEach((x) => x.classList.toggle("on", x === b)); save({ qc: b.dataset.qc }); };
  const rv = $("#reveal");
  if (rv) rv.onclick = () => { const d = driveOf(driveId); if (d) nativeCall("reveal", { path: d.mount + "/" + path }); };
  const rp = $("#reprobe");
  if (rp) rp.onclick = async () => {
    rp.disabled = true;
    try { await api(`/api/drives/${encodeURIComponent(driveId)}/file?path=${encodeURIComponent(path)}`, { method: "PATCH", body: { ...edits, reread: true } }); openFile(driveId, path); if (S.mediaData) loadMedia(); }
    catch (e) { toast(e.message, true); rp.disabled = false; }
  };
}

// ---------- appearance ----------
// Dark by default; light, or whatever the Mac is set to.
function applyAppearance() {
  let pref = localGet("appearance", "dark"), style = localGet("style", "modern");
  if (pref === "classic") { pref = "dark"; style = "classic"; localSet("appearance", pref); localSet("style", style); } // from before Classic was a style
  const light = pref === "light" || (pref === "system" && window.matchMedia("(prefers-color-scheme: light)").matches);
  document.documentElement.dataset.theme = light ? "light" : "dark";
  if (style === "classic") document.documentElement.dataset.style = "classic"; else delete document.documentElement.dataset.style;
  applyDesk(style === "classic" && light);
  if (native) nativeCall("appearance", { theme: light ? "light" : "dark" });
  const classic = style === "classic";
  const swapped = classic !== S.classicIcons;
  if (swapped) { S.classicIcons = classic; useClassicIcons(classic); }
  return swapped;
}
// applyDesk sets the shade of the grey desktop behind Classic's windows
// on Light, from the Desktop shade slider in Settings: 0 is pale, 100
// is charcoal.
function applyDesk(on) {
  const st = document.documentElement.style;
  if (!on) { st.removeProperty("--desk"); st.removeProperty("--bg"); return; }
  const dark = Math.max(0, Math.min(100, +localGet("desk", 15) || 0)), l = 88 - dark * 0.6;
  const g = (x) => `hsl(240 4% ${x.toFixed(0)}%)`;
  st.setProperty("--desk", `repeating-conic-gradient(${g(l - 8)} 0 25%, ${g(l)} 0 50%) 0 0 / 2px 2px`);
  st.setProperty("--bg", g(l - 4));
}
window.matchMedia("(prefers-color-scheme: light)").addEventListener("change", applyAppearance);
applyAppearance();

// ---------- settings ----------
async function openSettings(focus) {
  let u;
  try { [S.settings, u] = await Promise.all([api("/api/settings"), api("/api/update")]); } catch (e) { toast(e.message, true); return; }
  const p = S.settings.publish, last = S.settings.last, pf = S.settings.prefs || {}, lib = S.settings.library || {};
  const hiddenDrives = (S.state.drives || []).filter((d) => d.labels && d.labels.neverLog);
  S.detail = null; S.file = null;
  const when2 = (iso) => iso ? new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" }) : "";
  const updStatus = u.installed ? `<span class="badge ok">${ICON.check}Lumpy ${esc(u.installed)} installed</span> <span class="hint">${native ? "Relaunch to run it." : "Restart lumpy ui to run it."}</span>`
    : u.installing ? `<span class="badge info">Installing…</span> <span class="hint">see Activity</span>`
    : u.available ? `<span class="badge info">${ICON.up}Lumpy ${esc(u.latest.version)} available</span> <span class="hint">released ${when2(u.latest.publishedAt)} · you have ${esc(u.current)}</span>`
    : u.error ? `<span class="badge warn">${ICON.warn}Couldn't check</span> <span class="hint">${esc(u.error)}</span>`
    : u.latest ? `<span class="badge ok">${ICON.check}Up to date</span> <span class="hint">Lumpy ${esc(u.current)} is the latest, released ${when2(u.latest.publishedAt)}</span>`
    : `<span class="hint">Not checked yet.</span>`;
  openSheet(`
    <div class="sheet-head">
      <span class="drive-ico file-ico">${ICON.gear}</span>
      <div><h2>Settings</h2><div class="sub">Lumpy ${esc(S.state.version || "")} · library at ${esc(S.state.home || "")}</div></div>
      <div class="actions"><button class="icon-btn" id="close" title="Close">${ICON.x}</button></div>
    </div>
    <div class="sheet-body">
      <div class="sec"><div class="sec-title">${ICON.gear}General</div>
        <div class="box"><div class="fields"><div class="field"><label>Appearance</label>
          <div class="seg" id="appearance">${[["system", "System"], ["light", "Light"], ["dark", "Dark"]].map(([v, l]) => `<button data-appearance="${v}" class="${localGet("appearance", "dark") === v ? "on" : ""}">${l}</button>`).join("")}</div></div>
          <div class="field"><label>Style</label>
          <div class="seg" id="style-seg">${[["modern", "Modern"], ["classic", "Classic"]].map(([v, l]) => `<button data-style="${v}" class="${localGet("style", "modern") === v ? "on" : ""}">${l}</button>`).join("")}</div>
          <span class="hint">Classic is System 7 chrome: square windows, pixel glyphs, Chicago, on Light or Dark.</span></div>
          <div class="field"><label>Desktop shade</label>
          <input type="range" id="desk-shade" min="0" max="100" step="1" value="${localGet("desk", 15)}">
          <span class="hint">The grey behind the windows in Classic on Light, pale to charcoal.</span></div></div></div></div>

      <div class="sec"><div class="sec-title">${ICON.drive}Drives</div>
        <div class="box">
          <div class="fields">
            <div class="field"><label>Which volumes count as drives</label>
              <label class="switch" style="margin-top:7px">Hide the Mac's internal volumes <input type="checkbox" data-pref="hideInternal" ${pf.hideInternal ? "checked" : ""}></label>
              <label class="switch" style="margin-top:8px">Hide mounted disk images (.dmg, .iso) <input type="checkbox" data-pref="hideImages" ${pf.hideImages ? "checked" : ""}></label></div>
            <div class="field"><label>Logging</label>
              <label class="switch" style="margin-top:7px" title="Off: only drives already in the library are kept up to date when plugged in. Cards, sticks and card readers are never logged by themselves either way: Connected → Log now">Log new drives when plugged in <input type="checkbox" data-pref="autoLog" ${pf.autoLog ? "checked" : ""}></label>
              <label class="switch" style="margin-top:8px" title="Codec, frame rate, timecode and camera, read from clip headers during scans">Read clip metadata during scans <input type="checkbox" data-pref="autoMetadata" ${pf.autoMetadata ? "checked" : ""}></label></div>
            <div class="field"><label>Copies and checks</label>
              <label class="switch" style="margin-top:7px" title="One file at a time, reads held to about 150 MB/s: slower, but the Mac and the drive stay responsive for whoever is editing">Gentle mode <input type="checkbox" data-pref="gentle" ${pf.gentle ? "checked" : ""}></label></div>
            <div class="field"><label>Keep drives awake <span class="hint">· a tiny file is written and removed on every plugged-in spinning drive, so none spins down and stalls the next read; SSDs and cards are left alone, and any drive can opt out in its details</span></label>
              <label class="switch" style="margin-top:7px">Keep spinning drives awake while Lumpy is open <input type="checkbox" data-pref="keepAwake" ${pf.keepAwake ? "checked" : ""}></label>
              <div style="display:flex;gap:10px;align-items:center;margin-top:8px;flex-wrap:wrap"><label class="select"><select class="plain" data-pref="keepAwakeMinutes">${[[1, "Touch every minute"], [2, "Every 2 minutes"], [5, "Every 5 minutes"], [10, "Every 10 minutes"]].map(([v, l]) => `<option value="${v}" ${+(pf.keepAwakeMinutes || 5) === v ? "selected" : ""}>${l}</option>`).join("")}</select></label>
              <label class="switch" title="Runs macOS's caffeinate while Lumpy is open: the Mac won't idle-sleep, though the display still can. Drives can only be kept awake while the Mac is.">Keep the Mac awake too <input type="checkbox" data-pref="keepMacAwake" ${pf.keepMacAwake ? "checked" : ""}></label></div></div>
            <div class="field"><label>Rescan plugged-in drives while Lumpy is open <span class="hint">· off is recommended: a rescan reads every folder, which can get in the way of editing</span></label>
              <select class="plain" data-pref="rescanMinutes">${[[0, "Only when plugged in, or by hand"], [15, "Every 15 minutes"], [60, "Every hour"], [240, "Every 4 hours"]].map(([v, l]) => `<option value="${v}" ${+pf.rescanMinutes === v ? "selected" : ""}>${l}</option>`).join("")}</select></div>
            <div class="field"><label>Folder exclusions <span class="hint">· names left out of library scans at any depth; a trailing * matches a prefix</span></label><input data-pref="exclude" value="${esc(pf.exclude)}" placeholder=".git, node_modules, Private, Render*" spellcheck="false"></div>
          </div>
          <p class="hint" style="margin:10px 0 0">Exclusions apply to the next scan of each drive (Details → Rescan). Offloads always copy everything on the card except macOS housekeeping files.</p>
          ${hiddenDrives.length ? `<p class="hint" style="margin:12px 0 6px;color:var(--text-2);font-weight:550">Never logged</p><div class="checks">${hiddenDrives.map((d) => `<label><span class="dot" style="background:${d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(d.title)}<span class="hint">${esc(d.name !== d.title ? d.name : "")}</span><button class="btn small" data-unhide="${esc(d.id)}" style="margin-left:auto">Log again</button></label>`).join("")}</div>` : ""}
        </div></div>

      <div class="sec"><div class="sec-title">${ICON.bell}Notifications</div>
        <div class="box">
          <div class="fields">
            <div class="field"><label>On this Mac</label>
              <label class="switch" style="margin-top:7px" title="Rescans and metadata reads only when something went wrong">Notify when an offload, transfer or check finishes <input type="checkbox" data-pref="notify" ${pf.notify ? "checked" : ""}></label>
              <label class="switch" style="margin-top:8px">Only when something went wrong <input type="checkbox" data-pref="notifyProblemsOnly" ${pf.notifyProblemsOnly ? "checked" : ""}></label></div>
            <div class="field"><label>Webhook <span class="hint">· a Slack, Discord or Teams incoming webhook URL; the same line is posted there, so the team hears when a copy is done</span></label>
              <div style="display:flex;gap:8px;align-items:center"><input data-pref="webhook" value="${esc(pf.webhook || "")}" placeholder="https://hooks.slack.com/services/…" spellcheck="false" style="flex:1;min-width:0"><button class="btn small" id="webhook-test">Send a test</button></div></div>
          </div>
        </div></div>

      <div class="sec" id="sec-updates"><div class="sec-title">${ICON.up}Updates
          <div class="right">${u.installed && native ? `<button class="btn small primary" id="upd-relaunch">Relaunch Lumpy</button>` : u.available && !u.installing ? `<button class="btn small primary" id="upd-install">Install ${esc(u.latest.version)}</button>` : ""}<button class="btn small" id="upd-check" ${u.checking ? "disabled" : ""}>${u.checking ? "Checking…" : "Check now"}</button></div></div>
        <div class="box">
          <div class="pub-last" style="margin:0 0 10px">${updStatus}</div>
          ${u.available && u.latest.notes ? `<details class="notes"><summary>What's new in ${esc(u.latest.version)}</summary><pre>${esc(u.latest.notes)}</pre></details>` : ""}
          <p class="hint" style="margin:0 0 10px">Updates replace ${esc(u.target || "the app")} in place; the library is untouched. ${native ? "" : "From Terminal: <code>lumpy update</code>."}</p>
          <div class="fields">
            <div class="field"><label>Check for updates</label><label class="switch" style="margin-top:7px">Twice a day while Lumpy is open <input type="checkbox" id="upd-auto" ${u.auto ? "checked" : ""}></label></div>
            <div class="field"><label>GitHub token <span class="hint">· optional; only needed if the Lumpy repository is made private</span></label><input id="upd-token" type="password" value="${u.hasToken ? "••••••••••••" : ""}" placeholder="github_pat_…" autocomplete="off"></div>
          </div>
        </div></div>
      <div class="sec"><div class="sec-title">${ICON.history}Library
          <div class="right"><button class="btn small" id="lib-optimise" title="Compacts the database file">Optimise</button>${native ? `<button class="btn small" id="lib-reveal">${ICON.reveal}Show in Finder</button>` : ""}</div></div>
        <div class="box">
          <dl class="kv"><dt>Location</dt><dd class="mono">${esc(lib.path || "")}</dd><dt>Size</dt><dd>${bytes(lib.bytes || 0)} <span class="hint">· ${(lib.drives || 0).toLocaleString()} drives · ${(lib.files || 0).toLocaleString()} files · ${(lib.media || 0).toLocaleString()} clips · ${(lib.events || 0).toLocaleString()} history entries</span></dd></dl>
          <div class="path-row" style="margin:12px 0 0"><input class="path-input" id="lib-folder" placeholder="${esc(lib.default || "")}" value="${esc(lib.location || "")}">${native ? `<button class="btn" id="lib-pick">Choose…</button>` : ""}<button class="btn" id="lib-move">Move library here</button></div>
          <p class="hint" style="margin:6px 0 0">Keeps the library on an external drive or a synced folder instead of this Mac. The library is copied there and Lumpy relaunches on it; the old copy stays until you delete it. Leave empty to go back to the default folder.</p>
          <div class="pub-last" style="margin-top:14px"><button class="btn small danger" id="lib-clear-history">Clear history…</button><button class="btn small danger" id="lib-clear-drives">Forget every drive…</button><span class="hint">These only change Lumpy's library. Files on your drives are never touched.</span></div>
          <div id="lib-confirm"></div>
        </div></div>

      <div class="sec"><div class="sec-title">${ICON.globe}Status page for coworkers
          <div class="right">${last && last.ok && native ? `<button class="btn small" id="pub-open">${ICON.reveal}Open page</button>` : ""}<button class="btn small primary" id="pub-now" ${S.settings.publishing ? "disabled" : ""}>${S.settings.publishing ? "Publishing…" : "Publish now"}</button></div></div>
        <p class="hint" style="margin:0 0 10px">A single web page with every drive, where it was last seen, each project's backup health and a file search, so the team can check without asking you. It's written whenever a job finishes, on the interval below while Lumpy is open, and once more when Lumpy quits. <code>lumpy publish</code> does the same from Terminal.</p>
        <div class="box">
          <label class="switch" style="margin-bottom:12px">Publish the status page <input type="checkbox" id="pub-enabled" ${p.enabled ? "checked" : ""}></label>
          <div class="fields">
            <div class="field"><label>Page title</label><input id="pub-title" value="${esc(p.title)}" placeholder="Lumpy status"></div>
            <div class="field"><label>Refresh while Lumpy is open</label><select class="plain" id="pub-interval">${[[0, "Only after jobs and on quit"], [5, "Every 5 minutes"], [15, "Every 15 minutes"], [30, "Every 30 minutes"], [60, "Every hour"], [240, "Every 4 hours"]].map(([v, l]) => `<option value="${v}" ${+p.interval === v ? "selected" : ""}>${l}</option>`).join("")}</select></div>
            <div class="field wide"><label>Write the page into this folder <span class="hint">· a Dropbox, iCloud Drive or shared folder is the simplest way to share it</span></label>
              <div class="path-row" style="margin:0"><input class="path-input" id="pub-folder" value="${esc(p.folder)}" placeholder="${esc(S.settings.defaultFolder)}">${native ? `<button class="btn" id="pub-pick">Choose…</button>` : ""}</div></div>
            <div class="field wide"><label>Also upload to a web server <span class="hint">· HTTP PUT of index.html and status.json under this URL, optional</span></label><input id="pub-url" value="${esc(p.url)}" placeholder="https://status.example.com/lumpy/"></div>
            <div class="field"><label>Bearer token for the upload <span class="hint">· optional</span></label><input id="pub-token" type="password" value="${esc(p.token)}" autocomplete="off"></div>
            <div class="field"><label>File index <span class="hint">· lets the page say which drives hold a file</span></label><label class="switch" style="margin-top:7px">Include every file name <input type="checkbox" id="pub-files" ${p.files ? "checked" : ""}></label></div>
            <div class="field wide"><label>Or run a command after writing <span class="hint">· the folder is in $LUMPY_STATUS_DIR, e.g. <code>rsync -a "$LUMPY_STATUS_DIR"/ user@host:/var/www/status/</code></span></label><input id="pub-command" class="mono" value="${esc(p.command)}" placeholder="rsync -a &quot;$LUMPY_STATUS_DIR&quot;/ user@host:/var/www/status/" spellcheck="false"></div>
          </div>
          <div class="pub-last" id="pub-last">${last ? `<span class="badge ${last.ok ? "ok" : "bad"}">${last.ok ? ICON.check + "Published" : ICON.warn + "Failed"}</span> <span class="hint">${when(Math.floor(new Date(last.at).getTime() / 1000))} · ${esc(last.summary)}</span>` : `<span class="hint">Not published yet.</span>`}</div>
        </div></div>
    </div>
    <div class="sheet-foot"><button class="btn" id="settings-cancel">Cancel</button><button class="btn primary" id="settings-save">Save</button></div>`);
  $("#close").onclick = closeSheet;
  $("#settings-cancel").onclick = closeSheet;
  $("#upd-auto").onchange = async (e) => { try { await api("/api/update/settings", { method: "PUT", body: { auto: e.target.checked } }); } catch (err) { toast(err.message, true); } };
  $("#upd-token").onchange = async (e) => {
    if (e.target.value.includes("•")) return;
    try { await api("/api/update/settings", { method: "PUT", body: { token: e.target.value } }); toast(e.target.value ? "Token saved" : "Token removed"); } catch (err) { toast(err.message, true); }
  };
  $("#upd-check").onclick = async () => {
    const b = $("#upd-check"); b.disabled = true; b.textContent = "Checking…";
    try { const r = await api("/api/update/check", { method: "POST" }); loadState(); openSettings("updates"); if (r.error) toast(r.error, true); else toast(r.available ? `Lumpy ${r.latest.version} is available` : "Lumpy is up to date"); }
    catch (err) { toast(err.message, true); b.disabled = false; b.textContent = "Check now"; }
  };
  const ui = $("#upd-install");
  if (ui) ui.onclick = async () => {
    ui.disabled = true; ui.textContent = "Installing…";
    try { await api("/api/update/install", { method: "POST" }); toast("Installing the update…"); closeSheet(); await loadJobs(); toggleActivity(true); }
    catch (err) { toast(err.message, true); ui.disabled = false; ui.textContent = "Install"; }
  };
  const rl = $("#upd-relaunch");
  if (rl) rl.onclick = () => nativeCall("relaunch");
  if (focus === "updates") setTimeout(() => $("#sec-updates").scrollIntoView({ block: "start" }), 20);
  for (const b of document.querySelectorAll("[data-appearance]")) b.onclick = () => {
    localSet("appearance", b.dataset.appearance);
    document.querySelectorAll("[data-appearance]").forEach((x) => x.classList.toggle("on", x === b));
    if (applyAppearance() && S.state) { render(); openSettings(); }
  };
  $("#desk-shade").oninput = () => { localSet("desk", $("#desk-shade").value); applyAppearance(); };
  for (const b of document.querySelectorAll("[data-style]")) b.onclick = () => {
    localSet("style", b.dataset.style);
    document.querySelectorAll("[data-style]").forEach((x) => x.classList.toggle("on", x === b));
    if (applyAppearance() && S.state) { render(); openSettings(); }
  };
  // Preferences save as they change.
  for (const el of document.querySelectorAll("[data-pref]")) el.onchange = async () => {
    const body = {}; body[el.dataset.pref] = el.type === "checkbox" ? el.checked : el.tagName === "SELECT" ? +el.value : el.value;
    try { S.settings.prefs = await api("/api/prefs", { method: "PUT", body }); if (el.dataset.pref === "exclude") el.value = S.settings.prefs.exclude; loadState(); }
    catch (e) { toast(e.message, true); }
  };
  $("#webhook-test").onclick = async () => {
    const webhook = document.querySelector("[data-pref=webhook]").value.trim();
    if (!webhook) return toast("Paste a webhook URL first", true);
    try { await api("/api/notify/test", { method: "POST", body: { webhook } }); toast("Test posted · check the channel"); } catch (e) { toast(e.message, true); }
  };
  for (const b of document.querySelectorAll("[data-unhide]")) b.onclick = async () => {
    const d = driveOf(b.dataset.unhide); if (!d) return;
    try { await api(`/api/drives/${encodeURIComponent(d.id)}`, { method: "PATCH", body: { ...d.labels, neverLog: false } }); toast(`${d.title} will be logged again`); await loadState(); openSettings(); }
    catch (e) { toast(e.message, true); }
  };
  $("#lib-optimise").onclick = async () => {
    try { const r = await api("/api/library/optimise", { method: "POST" }); toast(`Library compacted: ${bytes(r.before)} → ${bytes(r.after)}`); openSettings(); } catch (e) { toast(e.message, true); }
  };
  const lr = $("#lib-reveal"); if (lr) lr.onclick = () => nativeCall("reveal", { path: lib.path });
  const lp = $("#lib-pick"); if (lp) lp.onclick = async () => { const f = await pickFolder($("#lib-folder").value || lib.default); if (f) $("#lib-folder").value = f; };
  $("#lib-move").onclick = async () => {
    const folder = $("#lib-folder").value.trim();
    const box = $("#lib-confirm");
    box.innerHTML = `<div class="confirm"><div><b>Move the library to ${esc(folder || lib.default)}?</b><br>Lumpy copies it there and relaunches on the copy. ${native ? "" : "Restart lumpy ui afterwards."}</div><div class="confirm-actions"><button class="btn" id="lm-no">Cancel</button><button class="btn primary" id="lm-yes">Move and relaunch</button></div></div>`;
    $("#lm-no").onclick = () => (box.innerHTML = "");
    $("#lm-yes").onclick = async () => {
      try { const r = await api("/api/library/move", { method: "POST", body: { folder } }); toast("Library copied to " + r.path + (native ? ". Relaunching…" : ". Restart lumpy ui to use it.")); if (native) setTimeout(() => nativeCall("relaunch"), 800); }
      catch (e) { toast(e.message, true); box.innerHTML = ""; }
    };
  };
  const clear = (what, text) => async () => {
    const box = $("#lib-confirm");
    box.innerHTML = `<div class="confirm"><div><b>${text}</b><br>This only changes Lumpy's library; nothing on any drive is touched.</div><div class="confirm-actions"><button class="btn" id="lc-no">Cancel</button><button class="btn primary danger-fill" id="lc-yes">${what === "drives" ? "Forget every drive" : "Clear history"}</button></div></div>`;
    $("#lc-no").onclick = () => (box.innerHTML = "");
    $("#lc-yes").onclick = async () => { try { await api("/api/library/clear", { method: "POST", body: { what } }); toast(what === "drives" ? "Library emptied" : "History cleared"); await loadState(); openSettings(); } catch (e) { toast(e.message, true); } };
  };
  $("#lib-clear-history").onclick = clear("history", "Clear the history of every drive?");
  $("#lib-clear-drives").onclick = clear("drives", "Forget every drive, its files, checksums, clip metadata and history? Plugged-in drives are logged again next time they mount (unless automatic logging is off).");
  const read = () => ({ enabled: $("#pub-enabled").checked, title: $("#pub-title").value, interval: +$("#pub-interval").value, folder: $("#pub-folder").value.trim(),
    url: $("#pub-url").value.trim(), token: $("#pub-token").value, files: $("#pub-files").checked, command: $("#pub-command").value });
  const save = async () => { S.settings = await api("/api/settings", { method: "PUT", body: read() }); renderCounts(); };
  $("#settings-save").onclick = async () => { try { await save(); closeSheet(); toast("Settings saved"); } catch (e) { toast(e.message, true); } };
  $("#pub-now").onclick = async () => {
    const b = $("#pub-now"); b.disabled = true; b.textContent = "Publishing…";
    try { await save(); const r = await api("/api/publish", { method: "POST" }); S.settings = r.settings; renderCounts(); toast("Published: " + r.result.summary); openSettings(); }
    catch (e) { toast(e.message, true); b.disabled = false; b.textContent = "Publish now"; try { S.settings = await api("/api/settings"); } catch {} }
  };
  const pk = $("#pub-pick");
  if (pk) pk.onclick = async () => { const f = await pickFolder($("#pub-folder").value || S.settings.defaultFolder); if (f) $("#pub-folder").value = f; };
  const op = $("#pub-open");
  if (op) op.onclick = () => nativeCall("reveal", { path: last.path });
}

// ---------- map ----------
// The home screen: this Mac, every project, and the drives that hold
// them, as nodes on a canvas. An edge says how much of a project sits
// on a drive and whether that part is backed up. Click a node and what
// it touches lights up; open it from the card, or double-click.
// What the map remembers: where each island was put (isl), the order
// of the nodes inside each (order), and where the Mac sits (pos).
const MAP = { k: 1, x: 0, y: 0, pos: {}, isl: {}, order: {}, sel: null, drag: null, fitted: false, dirty: false };
for (const k of ["pos", "isl", "order"]) { try { MAP[k] = JSON.parse(localGet("map." + k, "{}")) || {}; } catch { MAP[k] = {}; } }
function mapSave() { for (const k of ["pos", "isl", "order"]) localSet("map." + k, JSON.stringify(MAP[k])); }
// The grid everything snaps to: the dots in the background.
const GRID = 22;
const snap = (v) => Math.round(v / GRID) * GRID;
const NODE = { project: [300, 96], drive: [296, 72], slice: [296, 60], mac: [224, 92] };
// busName is how a drive's bus reads on the Mac's node.
function busName(bus) {
  const b = (bus || "").toLowerCase();
  if (!b) return "other";
  if (b.includes("thunderbolt")) return "Thunderbolt";
  if (b.includes("usb")) return "USB";
  if (b.includes("secure digital") || b.includes("sd")) return "SD";
  if (b.includes("pci")) return "PCIe";
  if (b.includes("sata")) return "SATA";
  if (b.includes("network") || b.includes("smb") || b.includes("afp") || b.includes("nfs")) return "network";
  return bus;
}
const ICON_TXT = { coffee: "☕ " };
const EDGE_LABEL = { full: "verified", none: "no backup", unknown: "unproven", likely: "probably backed up", empty: "empty" };

// mapModel groups every drive into the island of the first project that
// holds it (loose drives share a grey island). A drive in a second
// project keeps a wire to that project's header, so membership across
// islands stays visible without a wire for every drive.
function mapModel() {
  const projects = S.projects || [];
  const nodes = [], edges = [], islands = [];
  // What each project holds on each drive, and which projects each
  // drive holds.
  const hold = new Map(); // project id -> Map(drive id -> {bytes, files, backed, unknown, likely, backup, st, label})
  const onDrive = new Map(); // drive id -> project ids
  for (const p of projects) {
    const by = new Map();
    for (const sh of (p.health && p.health.sources) || []) {
      const a = by.get(sh.driveId) || { bytes: 0, files: 0, backed: 0, unknown: 0, likely: 0 };
      a.bytes += sh.bytes; a.files += sh.files; a.backed += sh.backedBytes; a.unknown += sh.unknownBytes; a.likely += sh.likelyBytes || 0;
      by.set(sh.driveId, a);
    }
    for (const src of p.sources || []) if (!by.has(src.driveId)) by.set(src.driveId, { bytes: 0, files: 0, backed: 0, unknown: 0, likely: 0 });
    const backup = new Set((p.sources || []).filter((x) => x.role === "backup").map((x) => x.driveId));
    for (const [id, a] of by) {
      if (!driveOf(id)) { by.delete(id); continue; }
      a.backup = backup.has(id);
      a.st = a.bytes ? backupState(a.backed, a.bytes, a.unknown, a.likely) : "empty";
      a.label = a.bytes ? `${bytes(a.bytes)} · ${EDGE_LABEL[a.st] || Math.round((a.backed / a.bytes) * 100) + "% backed up"}` : "nothing recorded";
      if (!onDrive.has(id)) onDrive.set(id, []);
      onDrive.get(id).push(p.id);
    }
    hold.set(p.id, by);
  }
  const byName = (x, y) => driveOf(x).title.localeCompare(driveOf(y).title, undefined, { numeric: true });
  // A drive that holds several projects is an island of its own, with a
  // node per project on it. A project's drives that hold nothing else
  // live in the project's island. A project with both gets a wire from
  // its header to each of its nodes in the drive islands.
  const shared = (id) => (onDrive.get(id) || []).length > 1;
  const heads = new Map(); // project id -> header node, when it has an island
  for (const p of projects) {
    const by = hold.get(p.id);
    const own = [...by.keys()].filter((id) => !shared(id)).sort((x, y) => (by.get(x).backup - by.get(y).backup) || byName(x, y));
    if (!own.length && by.size) continue; // lives only on shared drives
    const pn = { id: "p" + p.id, type: "project", p };
    const isl = { id: "p" + p.id, p, head: pn, drives: [], color: COLORS[p.color] || "var(--accent)" };
    pn.island = isl;
    nodes.push(pn); islands.push(isl); heads.set(p.id, pn);
    for (const id of own) {
      const a = by.get(id);
      const dn = { id: "d" + id, type: "drive", d: driveOf(id), st: a.st, label: a.label, backup: a.backup, island: isl };
      nodes.push(dn); isl.drives.push(dn);
    }
  }
  const sharedIds = [...onDrive.keys()].filter(shared).sort(byName);
  for (const id of sharedIds) {
    const d = driveOf(id), color = cardColor(d) || "var(--muted)";
    const dn = { id: "d" + id, type: "drive", d, st: "", label: `${onDrive.get(id).length} projects`, island: null, headOf: true };
    const isl = { id: "d" + id, d, head: dn, drives: [], color };
    dn.island = isl;
    nodes.push(dn); islands.push(isl);
    for (const pid of onDrive.get(id)) {
      const p = projects.find((x) => x.id === pid), a = hold.get(pid).get(id);
      const sn = { id: `s${pid}|${id}`, type: "slice", p, d, st: a.st, label: a.label, backup: a.backup, island: isl };
      nodes.push(sn); isl.drives.push(sn);
      if (heads.has(pid)) edges.push({ from: "p" + pid, to: sn.id, cls: a.st, label: a.label });
    }
  }
  const loose = { id: "loose", p: null, head: null, drives: [], color: "var(--muted)" };
  for (const d of [...S.state.drives].sort((x, y) => x.title.localeCompare(y.title, undefined, { numeric: true }))) if (!onDrive.has(d.id)) {
    const dn = { id: "d" + d.id, type: "drive", d, st: "", label: "", island: loose };
    nodes.push(dn); loose.drives.push(dn);
  }
  if (loose.drives.length) islands.push(loose);
  // Nodes keep the order they were put in; newcomers go last.
  for (const isl of islands) {
    const want = MAP.order[isl.id];
    if (!want) continue;
    const rank = new Map(want.map((id, i) => [id, i]));
    isl.drives.sort((a, b) => (rank.has(a.id) ? rank.get(a.id) : 1e9) - (rank.has(b.id) ? rank.get(b.id) : 1e9));
  }
  // This Mac; its wires to plugged-in drives are drawn by busPaths.
  nodes.push({ id: "mac", type: "mac" });
  return { nodes, edges, islands };
}

// mapLayout lays each island out as a header with its drives in a grid
// beneath, then packs the islands into lanes the way a mason lays
// stones: each goes to the lowest spot it fits, so the map is a block
// rather than a column. Dragged positions win; an island's frame
// always wraps its members.
const ISL = { pad: 18, gap: 14, row: 40, lane: 296 + 2 * 18, lanes: 6, aisle: 72 };
function mapLayout(m) {
  for (const n of m.nodes) [n.w, n.h] = NODE[n.type];
  const mac = m.nodes.find((n) => n.type === "mac");
  const laneW = ISL.lane + ISL.row, box = $("#map") || $("#main");
  // The Mac sits in the middle; islands pack into lanes either side of it.
  const lanes = Math.max(2, Math.min(ISL.lanes, Math.floor((((box ? box.clientWidth : 1400) / 0.72) - NODE.mac[0] - 2 * ISL.aisle) / laneW)));
  const sides = [{ lanes: Math.ceil(lanes / 2), sky: [] }, { lanes: Math.floor(lanes / 2), sky: [] }];
  for (const sd of sides) sd.sky = new Array(sd.lanes).fill(40);
  const leftEnd = 40 + sides[0].lanes * laneW - ISL.row; // right edge of the left side
  const rightStart = leftEnd + ISL.aisle + NODE.mac[0] + ISL.aisle;
  for (const isl of m.islands) {
    const n = isl.drives.length;
    let cols = isl.head ? Math.min(n <= 2 ? Math.max(1, n) : n <= 4 ? 2 : 3, Math.max(...sides.map((sd) => sd.lanes))) : 1;
    const rows = Math.ceil(n / cols);
    const gw = cols * NODE.drive[0] + (cols - 1) * ISL.gap;
    const w = ISL.pad * 2 + Math.max(isl.head ? NODE.project[0] : 0, gw);
    if (isl.head) isl.head.w = w - ISL.pad * 2;
    const hh = isl.head ? isl.head.h : 0, mh = isl.drives.length ? isl.drives[0].h : NODE.drive[1];
    const h = ISL.pad * 2 + (isl.head ? hh + ISL.gap : 26) + rows * mh + (rows - 1) * ISL.gap;
    isl.cols = cols; isl.mh = mh;
    // An island that was put somewhere stays there, out of the packing.
    const put = MAP.isl[isl.id];
    if (put) { islandPlace(isl, put.x, put.y); continue; }
    const span = cols;
    // The lowest place this many lanes wide on either side; nearest the
    // Mac among ties, and the left side before the right.
    let best = null, bestY = Infinity;
    sides.forEach((sd, si) => { for (let l = 0; l + span <= sd.lanes; l++) { const y = Math.max(...sd.sky.slice(l, l + span)); if (y < bestY) { bestY = y; best = { si, l }; } } });
    if (!best) best = { si: 0, l: 0 };
    const sd = sides[best.si], y = bestY;
    for (let l = best.l; l < best.l + span && l < sd.lanes; l++) sd.sky[l] = y + h + ISL.row;
    // Lane 0 is the one beside the Mac; islands hug it.
    const x = best.si === 0 ? leftEnd - best.l * laneW - w : rightStart + best.l * laneW;
    islandPlace(isl, x, y);
  }
  if (mac) { mac.x = leftEnd + ISL.aisle; mac.y = Math.max(40, (Math.max(...sides.flatMap((sd) => sd.sky)) - ISL.row - 40) / 2 + 40 - mac.h / 2); }
  if (mac && MAP.pos.mac) { mac.x = MAP.pos.mac.x; mac.y = MAP.pos.mac.y; }
  for (const isl of m.islands) islandFrame(isl);
  m.buses = busPaths(m);
}
// islandSlot is where the i-th member of an island sits when the
// island's top-left corner is at (x, y): a grid under the header.
function islandSlot(isl, i, x, y) {
  const top = y + ISL.pad + (isl.head ? isl.head.h + ISL.gap : 26);
  return { x: x + ISL.pad + (i % isl.cols) * (NODE.drive[0] + ISL.gap), y: top + Math.floor(i / isl.cols) * (isl.mh + ISL.gap) };
}
// islandPlace puts an island's corner at (x, y) and its members in
// their slots.
function islandPlace(isl, x, y) {
  if (isl.head) { isl.head.x = x + ISL.pad; isl.head.y = y + ISL.pad; }
  isl.drives.forEach((dn, i) => { const sl = islandSlot(isl, i, x, y); dn.x = sl.x; dn.y = sl.y; });
}
// slotAt is the slot index nearest a point inside an island, for the
// home-screen shuffle while a node is carried.
function slotAt(isl, wx, wy) {
  const top = isl.y + ISL.pad + (isl.head ? isl.head.h + ISL.gap : 26);
  const col = Math.max(0, Math.min(isl.cols - 1, Math.floor((wx - isl.x - ISL.pad + ISL.gap / 2) / (NODE.drive[0] + ISL.gap))));
  const row = Math.max(0, Math.floor((wy - top + ISL.gap / 2) / (isl.mh + ISL.gap)));
  return Math.max(0, Math.min(isl.drives.length - 1, row * isl.cols + col));
}
// busPaths runs a lead from the Mac to every island that has a drive
// plugged in, ending at the island's near side, and to each plugged-in
// drive that is in no project. Power shows on the drives themselves,
// as a lit ring, so nothing is drawn inside an island.
function busPaths(m) {
  const mac = m.nodes.find((n) => n.type === "mac");
  if (!mac) return [];
  const mcx = mac.x + mac.w / 2, r = (v) => Math.round(v * 10) / 10;
  const out = [];
  for (const isl of m.islands) {
    const members = isl.head ? [isl.head, ...isl.drives] : isl.drives;
    const live = members.filter((n) => n.type === "drive" && n.d.connected);
    if (!live.length) continue;
    const left = isl.x + isl.w / 2 < mcx;
    if (isl.head) {
      out.push({ left, port: [left ? isl.x + isl.w : isl.x, isl.y + isl.h / 2], ids: new Set(members.map((n) => n.id)), island: isl.id });
    } else {
      for (const n of live) out.push({ left, port: [left ? n.x + n.w : n.x, n.y + n.h / 2], ids: new Set([n.id]), island: isl.id });
    }
  }
  // Leads leave each side of the Mac in the order their ends sit, top to
  // bottom, so they fan out without crossing.
  for (const left of [true, false]) {
    const side = out.filter((b) => b.left === left).sort((a, b) => a.port[1] - b.port[1] || a.port[0] - b.port[0]);
    side.forEach((b, i) => {
      const sx = left ? mac.x : mac.x + mac.w, sy = mac.y + (mac.h * (i + 1)) / (side.length + 1);
      const k = Math.max(40, Math.abs(b.port[0] - sx) / 2), dir = left ? -1 : 1;
      b.start = [r(sx), r(sy)];
      b.lead = `M${r(sx)} ${r(sy)}C${r(sx + dir * k)} ${r(sy)},${r(b.port[0] - dir * k)} ${r(b.port[1])},${r(b.port[0])} ${r(b.port[1])}`;
    });
  }
  return out;
}
// islandFrame is the box around an island's members, from wherever they
// sit now.
function islandFrame(isl) {
  const ns = isl.head ? [isl.head, ...isl.drives] : isl.drives;
  const top = isl.head ? 0 : 26;
  isl.x = Math.min(...ns.map((n) => n.x)) - ISL.pad; isl.y = Math.min(...ns.map((n) => n.y)) - ISL.pad - top;
  isl.w = Math.max(...ns.map((n) => n.x + n.w)) + ISL.pad - isl.x; isl.h = Math.max(...ns.map((n) => n.y + n.h)) + ISL.pad - isl.y;
}

// edgeGeom picks a connector on each node (left or right, or top and
// bottom when they overlap sideways) so a wire leaves one node and
// enters the other on the facing sides, never under either.
function edgeGeom(a, b) {
  if (!a || !b) return null;
  let p0, p1, n0, n1;
  if (b.x >= a.x + a.w + 16) { p0 = [a.x + a.w, a.y + a.h / 2]; n0 = [1, 0]; p1 = [b.x, b.y + b.h / 2]; n1 = [-1, 0]; }
  else if (b.x + b.w <= a.x - 16) { p0 = [a.x, a.y + a.h / 2]; n0 = [-1, 0]; p1 = [b.x + b.w, b.y + b.h / 2]; n1 = [1, 0]; }
  else if (b.y >= a.y) { p0 = [a.x + a.w / 2, a.y + a.h]; n0 = [0, 1]; p1 = [b.x + b.w / 2, b.y]; n1 = [0, -1]; }
  else { p0 = [a.x + a.w / 2, a.y]; n0 = [0, -1]; p1 = [b.x + b.w / 2, b.y + b.h]; n1 = [0, 1]; }
  const k = Math.max(48, Math.hypot(p1[0] - p0[0], p1[1] - p0[1]) / 2);
  const c0 = [p0[0] + n0[0] * k, p0[1] + n0[1] * k], c1 = [p1[0] + n1[0] * k, p1[1] + n1[1] * k];
  const r = (v) => Math.round(v * 10) / 10;
  return {
    x0: r(p0[0]), y0: r(p0[1]), x1: r(p1[0]), y1: r(p1[1]),
    mx: r((p0[0] + 3 * c0[0] + 3 * c1[0] + p1[0]) / 8), my: r((p0[1] + 3 * c0[1] + 3 * c1[1] + p1[1]) / 8),
    d: `M${r(p0[0])} ${r(p0[1])} C${r(c0[0])} ${r(c0[1])}, ${r(c1[0])} ${r(c1[1])}, ${r(p1[0])} ${r(p1[1])}`,
  };
}

function renderMap() {
  const main = $("#main");
  if (!S.projects) { main.innerHTML = loadingGlyph("Reading the library…"); loadProjects(); return; }
  if (MAP.drag) { MAP.dirty = true; return; }
  const m = mapModel();
  mapLayout(m);
  const macN = m.nodes.find((n) => n.type === "mac");
  const W = Math.max(600, macN.x + macN.w, ...m.islands.map((i) => i.x + i.w)) + 80, H = Math.max(400, macN.y + macN.h, ...m.islands.map((i) => i.y + i.h)) + 80;
  const sel = MAP.sel, hot = new Set();
  if (sel) {
    hot.add(sel);
    for (const e of m.edges) { if (e.from === sel) hot.add(e.to); if (e.to === sel) hot.add(e.from); }
    if (sel === "mac") for (const n of m.nodes) if (n.type === "drive" && n.d.connected) hot.add(n.id);
    const isl = m.islands.find((i) => i.id === sel || i.drives.some((n) => n.id === sel));
    if (isl && isl.id === sel) for (const n of isl.drives) hot.add(n.id);
    else if (isl && isl.head) hot.add(isl.head.id);
    // A project lights up wherever it sits: its header, and its node in
    // every drive island.
    const pid = sel[0] === "p" ? +sel.slice(1) : sel[0] === "s" ? +sel.slice(1).split("|")[0] : null;
    if (pid !== null) for (const n of m.nodes) if (n.type === "slice" && n.p.id === pid || n.id === "p" + pid) { hot.add(n.id); if (n.island.head) hot.add(n.island.head.id); }
  }
  const at = (id) => m.nodes.find((n) => n.id === id);
  const edgeSVG = (e) => {
    const g = edgeGeom(at(e.from), at(e.to));
    if (!g) return "";
    return `<g class="edge-g ${e.cls} ${hot.has(e.from) && hot.has(e.to) ? "hot" : ""}"><path d="${g.d}"/><circle cx="${g.x0}" cy="${g.y0}" r="3.5"/><circle cx="${g.x1}" cy="${g.y1}" r="3.5"/></g>`;
  };
  const edgeLabel = (e) => {
    const g = e.label ? edgeGeom(at(e.from), at(e.to)) : null;
    if (!g) return "";
    return `<div class="e-lbl ${e.cls} ${hot.has(e.from) && hot.has(e.to) ? "hot" : ""}" style="left:${g.mx}px;top:${g.my}px">${esc(e.label)}</div>`;
  };
  const live = S.state.drives.filter((d) => d.connected);
  const busHot = (b) => sel && (sel === "mac" || b.ids.has(sel)) ? "hot" : "";
  const leadSVG = (b, cls = "") => `<g class="edge-g live lead ${cls}"><path class="halo" d="${b.lead}"/><path d="${b.lead}"/><circle cx="${b.start[0]}" cy="${b.start[1]}" r="4"/><circle cx="${b.port[0]}" cy="${b.port[1]}" r="4"/></g>`;
  MAP.leadSVG = leadSVG;
  const powered = new Set(m.buses.map((b) => b.island));
  // What the plugged-in drives hang off, for the Mac's node.
  const byBus = {};
  for (const d of live) { const k = busName(d.bus); byBus[k] = (byBus[k] || 0) + 1; }
  const busLine = Object.entries(byBus).filter(([k]) => k !== "other").sort((a, b) => b[1] - a[1]).map(([k, n]) => `${n} ${k}`).join(" · ");
  const node = (n) => {
    const cls = `node ${n.type} ${n.id === sel ? "sel" : ""} ${hot.has(n.id) ? "hot" : ""}`;
    const box = `left:${n.x}px;top:${n.y}px;width:${n.w}px;height:${n.h}px`;
    if (n.type === "project") {
      const p = n.p, h = p.health || {}, pct = h.bytes ? Math.round((h.backedBytes / h.bytes) * 100) : 0;
      const st = backupState(h.backedBytes || 0, h.bytes || 0, h.unknownBytes || 0, h.likelyBytes || 0);
      const nd = new Set(((h.sources) || []).map((x) => x.driveId)).size;
      const todo = (h.soleBytes || 0) + (h.shortBytes || 0);
      const line = !h.bytes ? "empty" : todo ? `<span class="warn">${bytes(todo)} still to copy</span>` : h.unknownBytes ? `<span class="hint">${bytes(h.unknownBytes)} unproven</span>` : `<span class="ok">${ICON.check}every file on ${p.target} drives</span>`;
      return `<div class="${cls}" data-node="${n.id}" style="${box};--pc:${COLORS[p.color] || "var(--accent)"}"><span class="n-tag">${ICON.film}</span><div class="n-body"><b>${esc(p.name)}<em>${p.target}× copies</em></b><small>${h.bytes ? `${bytes(h.bytes)} · ${(h.contents || 0).toLocaleString()} files · ` : ""}${nd} drive${nd === 1 ? "" : "s"}</small><div class="n-bar"><i class="${st}" style="width:${pct}%"></i></div><small class="n-foot">${h.bytes ? pct + "% backed up" : ""}${h.bytes ? " · " : ""}${line}</small></div></div>`;
    }
    if (n.type === "mac") return `<div class="${cls}" data-node="mac" style="${box};--pc:var(--accent)"><div class="n-pic"><svg class="pic on" viewBox="0 0 64 48">${PIC.mac}</svg></div><div class="n-body"><b>This Mac</b><small>${live.length} drive${live.length === 1 ? "" : "s"} plugged in</small>${busLine ? `<small class="n-bus">${esc(busLine)}</small>` : ""}</div></div>`;
    if (n.type === "slice") {
      const p = n.p;
      return `<div class="${cls}" data-node="${n.id}" style="${box};--pc:${COLORS[p.color] || "var(--accent)"}" title="${esc(p.name)} on ${esc(n.d.title)} · ${esc(n.label)}"><span class="n-tag">${ICON.film}</span><div class="n-body"><b>${esc(p.name)}</b><small>${n.backup ? '<span class="n-role">backup</span>' : ""}<span class="n-st ${n.st}">${esc(EDGE_LABEL[n.st] || n.label.split(" · ").pop())}</span> · ${esc(n.label.split(" · ")[0])} here</small></div></div>`;
    }
    const d = n.d, color = cardColor(d);
    const st = n.st ? `<span class="n-st ${n.st}">${esc(EDGE_LABEL[n.st] || n.label.split(" · ").pop())}</span>` : safeToFormat(d) === "safe" ? `<span class="n-st full">safe to format</span>` : safeToFormat(d) === "once" ? `<span class="n-st partial">backed up once</span>` : "";
    const sub = d.connected ? `${bytes(d.free)} free` : `seen ${when(d.lastSeen)}`;
    return `<div class="${cls} ${d.connected ? "live" : "away"} ${n.backup ? "backup" : ""} ${n.headOf ? "head" : ""}" data-node="${n.id}" style="${box};--pc:${color || "var(--muted)"}" title="${esc(d.title)}${n.label ? " · " + esc(n.label) : ""}"><div class="n-pic">${drivePic(driveKind(d), color || "var(--muted)", d.connected)}<i class="n-live" title="${d.connected ? "Plugged in" : "Not plugged in"}"></i></div><div class="n-body"><b>${esc(d.title)}</b><small>${n.backup ? '<span class="n-role">backup</span>' : ""}${n.headOf ? `<span class="n-st">${esc(n.label)}</span> · ` : ""}${st}${st ? " · " : ""}${sub}</small></div>${capRing(Math.max(0, d.size - d.free), d.size, "sm")}</div>`;
  };
  const island = (i) => `<div class="island ${i.p ? "" : i.d ? "shared" : "loose"} ${sel && (i.id === sel || i.drives.some((n) => n.id === sel)) ? "hot" : ""} ${powered.has(i.id) && i.head ? "powered" : ""}" data-island="${i.id}" style="left:${i.x}px;top:${i.y}px;width:${i.w}px;height:${i.h}px;--pc:${i.color}">${i.head ? "" : `<span class="island-lbl">Not in a project · ${i.drives.length}</span>`}</div>`;
  main.innerHTML = `<div class="map ${sel ? "has-sel" : ""}" id="map">
    <div class="map-world" id="map-world" style="width:${W}px;height:${H}px">
      <svg class="map-edges map-leads" width="${W}" height="${H}">${m.buses.map((b) => leadSVG(b, busHot(b))).join("")}</svg>
      ${m.islands.map(island).join("")}
      <svg class="map-edges" width="${W}" height="${H}">${m.edges.map(edgeSVG).join("")}</svg>
      ${m.edges.map(edgeLabel).join("")}
      ${m.nodes.map(node).join("")}
    </div>
    <div class="map-legend" title="Drag a drive within its island to reorder, onto a project to add it, out of its island to take it out · drag an island by its header or background · ⌘ + scroll zooms"><span><i class="dot"></i>${live.length} plugged in</span>${S.state.awake && S.state.awake.on ? `<span title="Keep drives awake is on">${ICON_TXT.coffee}${(S.state.awake.drives || []).length} kept awake</span>` : ""}</div>
    <div class="map-tools"><button class="icon-btn" data-map="out" title="Zoom out"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14"/></svg></button><button class="icon-btn" data-map="in" title="Zoom in">${ICON.plus}</button><button class="btn small" data-map="fit">Fit</button><button class="btn small" data-map="arrange" title="Put every node back where Lumpy lays it out">Arrange</button></div>
    ${sel ? mapCard(m, sel) : ""}
  </div>`;
  if (!MAP.fitted) { mapFit(m); MAP.fitted = true; }
  applyMap();
  wireMap(m);
}

// mapCard is the panel for the selected node: what it is, and the way in.
function mapCard(m, id) {
  const n = m.nodes.find((x) => x.id === id);
  if (!n) return "";
  if (n.type === "mac") {
    const live = S.state.drives.filter((d) => d.connected);
    return `<div class="map-card"><div class="mc-head"><b>This Mac</b><button class="btn small primary" data-open="connected">Connected</button></div>
      <div class="mc-list">${live.length ? live.map((d) => `<div data-node-go="d${esc(d.id)}"><span class="dot live" style="background:var(--ok);color:var(--ok)"></span>${esc(d.title)}<small>${esc(d.bus || "")}</small></div>`).join("") : '<span class="hint">Nothing plugged in.</span>'}</div></div>`;
  }
  if (n.type === "project" || n.type === "slice") {
    const p = n.p, h = p.health || {};
    const isl = m.islands.find((i) => i.id === "p" + p.id);
    const edges = [...(isl ? isl.drives.map((dn) => ({ to: dn.id, cls: dn.st, label: dn.label, backup: dn.backup })) : []), ...m.nodes.filter((x) => x.type === "slice" && x.p.id === p.id).map((x) => ({ to: x.id, dn: x, cls: x.st, label: x.label, backup: x.backup }))];
    return `<div class="map-card" style="--pc:${COLORS[p.color] || "var(--accent)"}"><div class="mc-head"><span class="n-tag">${ICON.film}</span><b>${esc(p.name)}</b><button class="btn small primary" data-open="project" data-id="${p.id}">Open project</button></div>
      ${h.bytes ? backupBar(h.backedBytes, h.bytes, h.unknownBytes, h.likelyBytes || 0, { compact: true }) : '<span class="hint">No files recorded yet.</span>'}
      <div class="mc-list">${edges.map((e) => { const d = e.dn ? e.dn.d : driveOf(e.to.slice(1)); return `<div data-node-go="${e.to}"><span class="dot ${d.connected ? "live" : ""}" style="background:${d.connected ? "var(--ok)" : "var(--muted)"};color:var(--ok)"></span>${esc(d.title)}${e.backup ? '<em class="role">backup</em>' : ""}<small class="${e.cls}">${esc(e.label)}</small></div>`; }).join("") || '<span class="hint">No drives yet. Drag a drive\'s edge onto this project, or open it to add folders.</span>'}</div></div>`;
  }
  const d = n.d;
  const ins = [...(n.island.p ? [{ from: n.island.id, cls: n.st, label: n.label }] : []), ...(n.headOf ? n.island.drives.map((x) => ({ from: "p" + x.p.id, go: x.id, cls: x.st, label: x.label })) : []), ...m.edges.filter((e) => e.to === id)];
  return `<div class="map-card" style="--pc:${cardColor(d) || "var(--accent)"}"><div class="mc-head"><b>${esc(d.title)}</b>${d.connected && native ? `<button class="btn small" data-finder="${esc(d.id)}" title="Open the drive in the Finder">${ICON.reveal}Finder</button>` : ""}${d.connected ? `<button class="btn small" data-eject="${esc(d.id)}" title="Unmount it so it can be unplugged">${ICON.eject}Eject</button>` : ""}<button class="btn small primary" data-open="drive" data-id="${esc(d.id)}">Details</button></div>
    <div class="cap"><b>${bytes(d.size)}</b><span class="sep">•</span><span class="${lowSpace(d) ? "low" : ""}">${bytes(d.free)} free</span><span class="sep">•</span>${d.files.toLocaleString()} files</div>
    <div class="badges" style="margin-top:6px">${backupBadge(d)}${checkBadge(d)}</div>
    <div class="mc-list">${ins.map((e) => { const p = (S.projects || []).find((x) => "p" + x.id === e.from); return p ? `<div data-node-go="${e.go || e.from}"><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span>${esc(p.name)}<small class="${e.cls}">${esc(e.label)}</small></div>` : ""; }).join("") || '<span class="hint">In no project. Drag its edge onto a project to add it.</span>'}</div></div>`;
}

function applyMap() {
  const w = $("#map-world");
  if (w) w.style.transform = `translate(${MAP.x}px, ${MAP.y}px) scale(${MAP.k})`;
}
function mapFit(m) {
  const box = $("#map");
  if (!box || !m.islands.length) return;
  const minX = Math.min(...m.islands.map((i) => i.x)) - 20, minY = Math.min(...m.islands.map((i) => i.y)) - 20;
  const maxX = Math.max(...m.islands.map((i) => i.x + i.w)) + 20, maxY = Math.max(...m.islands.map((i) => i.y + i.h)) + 20;
  const W = box.clientWidth, H = box.clientHeight;
  MAP.k = Math.max(0.35, Math.min(1.15, (W - 40) / (maxX - minX), (H - 110) / (maxY - minY)));
  MAP.x = (W - (maxX - minX) * MAP.k) / 2 - minX * MAP.k;
  MAP.y = (H - (maxY - minY) * MAP.k) / 2 - minY * MAP.k + 20;
}
function mapZoom(f, cx, cy) {
  const k = Math.max(0.35, Math.min(2.5, MAP.k * f));
  MAP.x = cx - ((cx - MAP.x) * k) / MAP.k;
  MAP.y = cy - ((cy - MAP.y) * k) / MAP.k;
  MAP.k = k;
  applyMap();
}
// mapChange carries out a drop: adding a drive to a project asks
// which folder; taking one out asks first, in case it was a slip.
function mapChange(act) {
  const { p, d } = act;
  const change = async (body) => {
    try { await api(`/api/projects/${p.id}`, { method: "PATCH", body }); await loadState(); S.projects = null; renderMap(); }
    catch (e) { toast(e.message, true); }
  };
  if (act.kind === "add") {
    addSourceDialog((src) => change({ add: [src] }), d.id, { lock: true, title: `Add ${d.title} to ${p.name}`, ok: "Add" });
    return;
  }
  const held = (p.sources || []).filter((s) => s.driveId === d.id);
  if (!held.length) return;
  openSheet(`<div class="sheet-head"><h2>Take ${esc(d.title)} out of ${esc(p.name)}?</h2><div class="actions"><button class="icon-btn" id="wd-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:12px"><p class="hint">${held.map((s) => esc(s.folder || "the whole drive")).join(", ")} would leave the project. Nothing on the drive is touched, and it can be added back any time.</p></div>
    <div class="sheet-foot"><button class="btn" id="wd-cancel">Keep it in</button><button class="btn danger-fill" id="wd-ok">Take it out</button></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  $("#wd-close").onclick = $("#wd-cancel").onclick = close;
  $("#wd-ok").onclick = () => { close(); change({ remove: held }); };
}
function openNode(id) {
  if (id === "mac") { S.tab = "connected"; localSet("tab", S.tab); render(); return; }
  if (id[0] === "p" || id[0] === "s") { S.tab = "projects"; S.project = +id.slice(1).split("|")[0]; localSet("tab", S.tab); render(); return; }
  openDetail(id.slice(1));
}
function wireMap(m) {
  const box = $("#map"), world = $("#map-world");
  box.addEventListener("wheel", (e) => {
    e.preventDefault();
    const r = box.getBoundingClientRect();
    if (e.ctrlKey || e.metaKey) mapZoom(Math.exp(-e.deltaY * 0.01), e.clientX - r.left, e.clientY - r.top);
    else { MAP.x -= e.deltaX; MAP.y -= e.deltaY; applyMap(); }
  }, { passive: false });
  // World coordinates of a pointer position.
  const toWorld = (e) => { const r = box.getBoundingClientRect(); return { x: (e.clientX - r.left - MAP.x) / MAP.k, y: (e.clientY - r.top - MAP.y) / MAP.k }; };
  box.onpointerdown = (e) => {
    if (e.button !== 0 || e.target.closest(".map-tools, .map-card, button")) return;
    e.preventDefault(); // no text selection while things are dragged
    const node = e.target.closest(".node");
    const n = node ? m.nodes.find((x) => x.id === node.dataset.node) : null;
    // A whole island moves by its header or its background; a node
    // inside it moves on its own.
    const islEl = node ? null : e.target.closest(".island");
    const isl = n ? (n.island && n.island.head === n ? n.island : null) : islEl ? m.islands.find((i) => i.id === islEl.dataset.island) : null;
    const group = isl ? (isl.head ? [isl.head, ...isl.drives] : isl.drives) : n ? [n] : [];
    MAP.drag = { sx: e.clientX, sy: e.clientY, x: MAP.x, y: MAP.y, node, n, isl, group: group.map((g) => ({ n: g, x: g.x, y: g.y, el: world.querySelector(`[data-node="${CSS.escape(g.id)}"]`) })), moved: false, slot: -1 };
    if (node) node.classList.add("dragging");
    box.setPointerCapture(e.pointerId);
    if (!group.length) box.classList.add("panning");
  };
  // The island under the pointer, other than the one the node came
  // from, while a drive or a project's node is being carried.
  const islandUnder = (e, d) => {
    const from = d.n.island;
    const els = document.elementsFromPoint(e.clientX, e.clientY);
    const fe = els.find((x) => x.classList && x.classList.contains("island") && (!from || x.dataset.island !== from.id));
    return fe ? m.islands.find((i) => i.id === fe.dataset.island) || null : null;
  };
  // What a drop would do: add to a project, take out of one, or nothing.
  const dropAction = (e, d) => {
    const n = d.n;
    if (!n || n.type === "mac" || n.type === "project") return null;
    const over = islandUnder(e, d);
    const inside = over || (n.island && document.elementsFromPoint(e.clientX, e.clientY).some((x) => x.dataset && x.dataset.island === n.island.id) ? n.island : null);
    if (n.type === "slice") return inside ? null : { kind: "remove", p: n.p, d: n.d };
    const d0 = n.d, held = (p) => (p.sources || []).some((src) => src.driveId === d0.id);
    if (over && over.p && !held(over.p)) return { kind: "add", p: over.p, d: d0 };
    if (!inside && n.island && n.island.p) return { kind: "remove", p: n.island.p, d: d0 };
    return null;
  };
  // Where a node's island's members sit for the order they are in now,
  // the carried one aside: the others slide over, home-screen style.
  const reflow = (d) => {
    const isl = d.n.island;
    isl.drives.forEach((dn, i) => {
      if (dn === d.n) return;
      const sl = islandSlot(isl, i, isl.x, isl.y);
      dn.x = sl.x; dn.y = sl.y;
      const el = world.querySelector(`[data-node="${CSS.escape(dn.id)}"]`);
      if (el) { el.classList.add("shift"); el.style.left = sl.x + "px"; el.style.top = sl.y + "px"; }
    });
  };
  box.onpointermove = (e) => {
    const d = MAP.drag;
    if (!d) return;
    const dx = e.clientX - d.sx, dy = e.clientY - d.sy;
    if (Math.abs(dx) + Math.abs(dy) > 3) d.moved = true;
    if (d.group.length) {
      const single = d.group.length === 1 && !d.isl;
      // A node carried inside its own island takes a new place in the
      // grid; carried out, say what a drop would do.
      if (d.moved && single && d.n.type !== "mac") {
        d.node.style.pointerEvents = "none";
        const isl = d.n.island, w = toWorld(e);
        const home = isl && w.x >= isl.x - 8 && w.x <= isl.x + isl.w + 8 && w.y >= isl.y - 8 && w.y <= isl.y + isl.h + 8;
        world.querySelectorAll(".island.target").forEach((x) => x.classList.remove("target"));
        if (home) {
          d.node.classList.remove("leaving");
          const want = slotAt(isl, w.x, w.y), at = isl.drives.indexOf(d.n);
          if (want !== at) { isl.drives.splice(at, 1); isl.drives.splice(want, 0, d.n); d.slot = want; reflow(d); }
        } else {
          const act = dropAction(e, d);
          d.node.classList.toggle("leaving", !!act && act.kind === "remove");
          if (act && act.kind === "add") { const fe = world.querySelector(`[data-island="${CSS.escape("p" + act.p.id)}"]`); if (fe) fe.classList.add("target"); }
        }
        box.classList.toggle("carrying", true);
      }
      for (const g of d.group) {
        g.n.x = g.x + dx / MAP.k; g.n.y = g.y + dy / MAP.k;
        if (g.el) { g.el.style.left = g.n.x + "px"; g.el.style.top = g.n.y + "px"; }
      }
      // An island's frame follows its members.
      if (d.isl) {
        islandFrame(d.isl);
        const fe = world.querySelector(`[data-island="${CSS.escape(d.isl.id)}"]`);
        if (fe) { fe.style.left = d.isl.x + "px"; fe.style.top = d.isl.y + "px"; }
      }
      const buses = busPaths(m);
      world.querySelector(".map-leads").innerHTML = buses.map((b) => MAP.leadSVG(b)).join("");
      world.querySelector(".map-edges:not(.map-leads)").innerHTML = m.edges.map((e2) => { const g = edgeGeom(m.nodes.find((x) => x.id === e2.from), m.nodes.find((x) => x.id === e2.to)); return g ? `<g class="edge-g ${e2.cls}"><path d="${g.d}"/><circle cx="${g.x0}" cy="${g.y0}" r="3.5"/><circle cx="${g.x1}" cy="${g.y1}" r="3.5"/></g>` : ""; }).join("");
      world.querySelectorAll(".e-lbl").forEach((el) => el.remove());
    } else { MAP.x = d.x + dx; MAP.y = d.y + dy; applyMap(); }
  };
  box.onpointerup = box.onpointercancel = (e) => {
    const d = MAP.drag;
    if (!d) return;
    MAP.drag = null;
    box.classList.remove("panning", "carrying");
    if (d.group.length) {
      if (d.node) { d.node.style.pointerEvents = ""; d.node.classList.remove("leaving", "dragging"); }
      world.querySelectorAll(".island.target").forEach((x) => x.classList.remove("target"));
      const single = d.group.length === 1 && !d.isl;
      if (d.moved && d.isl) {
        // The island lands on the grid, and stays there.
        MAP.isl[d.isl.id] = { x: snap(d.isl.x), y: snap(d.isl.y) };
        mapSave(); renderMap(); return;
      }
      if (d.moved && single && d.n.type === "mac") { MAP.pos.mac = { x: snap(d.n.x), y: snap(d.n.y) }; mapSave(); renderMap(); return; }
      const act = d.moved && single && e.type === "pointerup" ? dropAction(e, d) : null;
      if (act) {
        // The node goes back where it was; the change, if confirmed,
        // redraws the map.
        renderMap();
        mapChange(act);
        return;
      }
      if (d.moved && single && d.slot >= 0 && d.n.island) {
        // Dropped in its own island: the new order is kept.
        MAP.order[d.n.island.id] = d.n.island.drives.map((x) => x.id);
        mapSave(); renderMap(); return;
      }
      if (d.moved) renderMap(); // nowhere to land: back to its place
      else if (d.n && MAP.last && MAP.last.id === d.n.id && Date.now() - MAP.last.at < 400) { MAP.last = null; openNode(d.n.id); }
      else if (d.n) { MAP.last = { id: d.n.id, at: Date.now() }; MAP.sel = d.n.id; renderMap(); }
      else if (MAP.sel) { MAP.sel = null; renderMap(); }
    } else if (!d.moved && MAP.sel) { MAP.sel = null; renderMap(); }
    else if (MAP.dirty) { MAP.dirty = false; renderMap(); }
  };
  for (const b of box.querySelectorAll("[data-map]")) b.onclick = () => {
    const r = box.getBoundingClientRect();
    if (b.dataset.map === "in") mapZoom(1.25, r.width / 2, r.height / 2);
    else if (b.dataset.map === "out") mapZoom(0.8, r.width / 2, r.height / 2);
    else if (b.dataset.map === "fit") { mapFit(m); applyMap(); }
    else { MAP.pos = {}; MAP.isl = {}; MAP.order = {}; mapSave(); MAP.fitted = false; renderMap(); }
  };
  for (const b of box.querySelectorAll("[data-open]")) b.onclick = () => {
    if (b.dataset.open === "connected") { S.tab = "connected"; localSet("tab", S.tab); render(); }
    else if (b.dataset.open === "project") openNode("p" + b.dataset.id);
    else openNode("d" + b.dataset.id);
  };
  for (const el of box.querySelectorAll("[data-node-go]")) el.onclick = () => { MAP.sel = el.dataset.nodeGo; renderMap(); };
  for (const b of box.querySelectorAll("[data-eject]")) b.onclick = () => { b.disabled = true; ejectDrive(b.dataset.eject); };
  for (const b of box.querySelectorAll("[data-finder]")) b.onclick = (e) => { e.stopPropagation(); const d = driveOf(b.dataset.finder); if (d && d.mount) nativeCall("reveal", { path: d.mount }); };
}
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && S.tab === "map" && MAP.sel) { MAP.sel = null; renderMap(); } });

// ---------- Space: what takes the room ----------
// Like DaisyDisk: measure a disk, see it as rings, drill in, put the
// big things in the Trash. The measurement lives on the server; the
// page asks for the rings around the folder in view.
// Finder's tag colours, as the Finder draws them.
const TAG_COLOURS = { Red: "#ff5257", Orange: "#f7a23b", Yellow: "#f7ce46", Green: "#5bd36b", Blue: "#3f8cff", Purple: "#b465f0", Gray: "#9a9aa0" };
const tagDots = (tags) => (tags || []).map((t) => `<i class="tagdot" style="background:${TAG_COLOURS[t] || "var(--muted)"}" title="${esc(t)}"></i>`).join("");
// spaceAct is one organising change inside the measured place: the
// rings are measured again afterwards where things moved.
async function spaceAct(op, rel, to, tags) {
  const r = await api("/api/space/organise", { method: "POST", body: { path: SP.root, op, rel, to, tags } });
  if (r.job) { SP.data = null; SP.busy = true; await renderSpace(); await waitForJob(r.job); SP.busy = false; SP.data = null; }
  SP.tags = null;
  await spaceLoad();
}
// spaceTagMenu offers the Finder's colours for one item.
function spaceTagMenu(rel, current, anchor) {
  const names = Object.keys(TAG_COLOURS);
  const have = new Set(current || []);
  const pop = document.createElement("div");
  pop.className = "tagmenu";
  pop.innerHTML = names.map((n) => `<button data-tag="${n}" class="${have.has(n) ? "on" : ""}"><i class="tagdot" style="background:${TAG_COLOURS[n]}"></i>${n}</button>`).join("") + `<button data-tag="" class="${have.size ? "" : "on"}"><i class="tagdot none"></i>No tag</button>`;
  const r = anchor.getBoundingClientRect();
  pop.style.left = Math.min(window.innerWidth - 170, r.left) + "px"; pop.style.top = (r.bottom + 4) + "px";
  document.body.appendChild(pop);
  // Above the button when there is no room below it.
  if (r.bottom + 4 + pop.offsetHeight > window.innerHeight) pop.style.top = Math.max(8, r.top - 4 - pop.offsetHeight) + "px";
  const close = () => { pop.remove(); document.removeEventListener("pointerdown", away, true); };
  const away = (e) => { if (!pop.contains(e.target)) close(); };
  setTimeout(() => document.addEventListener("pointerdown", away, true), 0);
  for (const b of pop.querySelectorAll("[data-tag]")) b.onclick = async () => {
    const n = b.dataset.tag;
    let tags = [...have].filter((t) => !(t in TAG_COLOURS) || t === n);
    if (n && !have.has(n)) tags = [n, ...tags.filter((t) => !(t in TAG_COLOURS))];
    else if (n && have.has(n)) tags = tags.filter((t) => t !== n);
    else if (!n) tags = [];
    close();
    try { await spaceAct("tag", rel, "", tags); } catch (e) { toast(e.message, true); }
  };
}
// spaceMoveDialog picks a folder in the measured place to move into.
async function spaceMoveDialog(d, rel) {
  const name = rel.split("/").pop();
  const folders = [];
  // The whole place's folders, not just the ones in view.
  let tree = d.node, top = SP.at;
  try { tree = (await api(`/api/space?path=${encodeURIComponent(SP.root)}&at=&depth=8`)).node; top = ""; } catch {}
  const walk = (n, p) => { for (const c of n.children || []) { if (!c.dir || c.rest) continue; const cp = p ? p + "/" + c.name : c.name; if (cp === rel || cp.startsWith(rel + "/")) continue; folders.push(cp); walk(c, cp); } };
  walk(tree, top);
  folders.sort((a, b) => a.localeCompare(b));
  const parent = rel.includes("/") ? rel.slice(0, rel.lastIndexOf("/")) : "";
  openSheet(`<div class="sheet-head"><h2>Move ${esc(name)}</h2><div class="actions"><button class="icon-btn" id="mv-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:12px"><p class="hint">Into which folder of ${esc((SP.places.find((p) => p.path === SP.root) || {}).name || "this place")}? It moves on the same disk, so it is instant.</p>
      <div class="sp-list mv-list">${[""].concat(folders).filter((f) => f !== parent).map((f) => `<button class="sp-row mv-row" data-mv="${esc(f)}"><span class="pickcell">${ICON.folder}</span><span class="sp-name">${esc(f || "(top of the place)")}</span></button>`).join("") || `<p class="hint">No other folder here yet. Make one first.</p>`}</div>
    </div><div class="sheet-foot"><button class="btn" id="mv-cancel">Cancel</button></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  $("#mv-close").onclick = $("#mv-cancel").onclick = close;
  for (const b of document.querySelectorAll("[data-mv]")) b.onclick = async () => { close(); try { await spaceAct("move", rel, b.dataset.mv); toast(`${name} moved`); } catch (e) { toast(e.message, true); } };
}
const VD_LABEL = { safe: "safe to clear", check: "look first", critical: "critical", ok: "nothing against it" };
const SP = { mv: { kind: "video", q: "", thumbs: localGet("space.mvthumbs", "1") === "1", files: null, folders: null, open: new Set(), undo: null, mkdirIn: null, busy: false, total: 0, counts: {}, more: false }, places: null, root: localGet("space.root", ""), at: "", data: null, busy: false, hover: null, sel: new Set(), err: "", susOpen: true, view: "folders", dupes: null, old: null, dkind: "", dfolder: "", thumbs: localGet("space.thumbs", "0") === "1" };
async function renderSpace() {
  // A fault in this page must never take the tabs with it.
  try { await renderSpace1(); } catch (e) { console.error(e); $("#main").innerHTML = `<div class="empty"><h2>Space hit a snag</h2><p>${esc(e.message)}</p><p><button class="btn" onclick="SP.at='';SP.data=null;spaceLoad()">Start again</button></p></div>`; }
}
async function renderSpace1() {
  const main = $("#main");
  if (!SP.places) { main.innerHTML = loadingGlyph("Looking at the disks…"); try { SP.places = await api("/api/space/places"); } catch (e) { main.innerHTML = `<div class="empty"><h2>Can't look</h2><p>${esc(e.message)}</p></div>`; return; } }
  if (!SP.root || !SP.places.some((p) => p.path === SP.root)) SP.root = (SP.places[0] || {}).path || "";
  const place = SP.places.find((p) => p.path === SP.root) || {};
  const d = SP.data && SP.data.root === SP.root ? SP.data : null;
  const crumbs = SP.at ? SP.at.split("/") : [];
  main.innerHTML = `<div class="space">
    <div class="page-bar"><div class="toolbar-title">${ICON.pie}Space</div>
      <label class="select"><select id="sp-place">${SP.places.map((p) => `<option value="${esc(p.path)}" ${p.path === SP.root ? "selected" : ""}>${esc(p.name)}${p.size ? ` · ${bytes(p.size - p.free)} of ${bytes(p.size)} used` : ""}</option>`).join("")}</select></label>
      ${spaceOnly && native ? `<button class="btn" id="sp-add" title="Grant Lumpy a folder from the Files app: On My iPad, a plugged-in drive, iCloud Drive, or a folder another app shows there">${ICON.plus}Add folder…</button>` : ""}
      ${SP.places.length ? `<button class="btn ${d ? "" : "primary"}" id="sp-scan" ${SP.busy ? "disabled" : ""}>${SP.busy ? "Measuring…" : d ? ICON.refresh + "Measure again" : ICON.play + "Measure"}</button>` : ""}
      ${place.size ? `<span class="pills"><span class="pill">Used:<b>${bytes(place.size - place.free)}</b></span><span class="pill">Free:<b>${bytes(place.free)}</b></span>${d ? `<span class="pill" title="What the measurement found; the rest is hidden system files, snapshots and purgeable space">Measured:<b>${bytes(d.total)}</b></span>` : ""}</span>` : ""}
      <div class="right">${SP.sel.size ? `<button class="btn danger" id="sp-trash">${ICON.trash}${spaceOnly ? `Delete ${SP.sel.size}…` : `Move ${SP.sel.size} to the Trash…`}</button>` : ""}</div>
    </div>
    ${spaceOnly && !SP.places.length ? `<div class="empty">${ICON.pie}<h2>Add a folder to measure</h2><p>Lumpy for iPad measures the folders you grant it from the Files app: On My iPad, a drive on the USB-C port, iCloud Drive, or a folder another app shows there, such as DaVinci Resolve's. Tap <b>Add folder…</b>, choose one, and then <b>Measure</b>.</p></div>` : ""}
    ${SP.busy ? `<div class="empty">${loadingGlyph("")}<h2>Measuring ${esc(place.name || SP.root)}</h2><p id="sp-prog">${bytes(SP.data && SP.data.bytes || 0)} so far · see Activity for the folder it's reading</p></div>`
    : !d ? `<div class="empty">${ICON.pie}<h2>What's taking the room?</h2><p>Lumpy measures every folder on ${esc(place.name || "the disk")} and draws it as rings: the bigger the slice, the more it holds. Click a slice to look inside, hover for its size, tick the things to go and move them to the Trash in one go. Nothing is touched until you say so.</p><p><button class="btn primary" id="sp-scan2">${ICON.play}Measure ${esc(place.name || "")}</button></p></div>`
    : `<div class="sp-body ${SP.view === "move" ? "mv" : ""}">
        ${SP.view === "move" ? "" : `<div class="sp-chart" id="sp-chart">${sunburst(d.node)}</div>`}
        <div class="sp-side">
          <div class="seg sp-views" id="sp-views">${[["folders", "Folders"], ["dupes", "Duplicates"], ["old", "Not touched in a year"], ["move", "Move files"]].map(([v, l]) => `<button data-spv="${v}" class="${SP.view === v ? "on" : ""}">${l}</button>`).join("")}</div>
          ${SP.view === "dupes" ? spaceDupesHTML() : SP.view === "old" ? spaceOldHTML() : SP.view === "move" ? spaceMoveHTML() : `
          <div class="crumbs"><button class="crumb" data-sp-go="">${esc(place.name || SP.root)}</button>${crumbs.map((c, i) => `<span class="hint">›</span><button class="crumb" data-sp-go="${esc(crumbs.slice(0, i + 1).join("/"))}">${esc(c)}</button>`).join("")}</div>
          <div class="sp-head"><b>${bytes(d.node.bytes)}</b><span class="hint">· ${d.node.files.toLocaleString()} files · ${d.total ? Math.round((d.node.bytes / d.total) * 100) : 0}% of what was measured</span>
            <span class="sp-head-acts"><button class="btn small" id="sp-mkdir" title="A new folder here">${ICON.plus}New folder</button>${(d.node.children || []).some((c) => !c.dir && !c.rest && spPreviewable(c.name)) ? `<div class="seg sp-mode" id="sp-mode" title="Show the pictures and clips here as thumbnails"><button data-spm="list" class="${SP.thumbs ? "" : "on"}">List</button><button data-spm="thumbs" class="${SP.thumbs ? "on" : ""}">Thumbnails</button></div>` : ""}</span></div>
          ${SP.mkdir ? `<div class="sp-edit" style="margin:0 0 8px"><input class="path-input" id="sp-mkdir-name" placeholder="New folder name"><button class="btn small primary" id="sp-mkdir-go">Make folder</button><button class="btn small" id="sp-mkdir-cancel">Cancel</button></div>` : ""}
          ${!SP.at && place.size && !spaceOnly ? spaceCoverage(d, place) : ""}
          ${(d.suspects || []).length ? `<details class="sp-sus" ${SP.susOpen ? "open" : ""}><summary><b>Usual suspects</b><span class="hint">· ${bytes(d.suspects.reduce((t, x) => t + (x.risk === "keep" ? 0 : x.bytes), 0))} in caches, renders and leftovers that can go</span></summary>
            <div class="sp-list" style="margin-top:6px">${d.suspects.map((x) => `<div class="sp-row sus ${x.risk}" data-sp-row="${esc(x.rel)}" title="${esc(x.note)}">
              ${x.risk === "keep" ? '<span class="pickcell"></span>' : `<input type="checkbox" class="pick" data-sp-pick="${esc(x.rel)}" ${SP.sel.has(x.rel) ? "checked" : ""}>`}
              <span class="sp-name"><span>${esc(x.name)}</span><small class="hint">${esc(x.note)}</small></span>
              <span class="badge ${x.risk === "safe" ? "ok" : x.risk === "check" ? "warn" : ""}">${x.risk === "safe" ? "safe to clear" : x.risk === "check" ? "look first" : "keep"}</span>
              <span class="sp-size">${bytes(x.bytes)}</span>
              <span class="sp-acts"><button class="icon-btn" data-sp-open="${esc(x.rel)}" title="Look inside">${ICON.right}</button>${native ? `<button class="icon-btn" data-sp-reveal="${esc(x.rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`).join("")}</div>
            <p class="hint" style="margin:6px 0 2px">${spaceOnly ? "Safe to clear means the app makes it again. Close the app first." : "Safe to clear means the app makes it again. Quit the app first. The Trash itself can only be emptied from the Finder."}</p></details>` : ""}
          ${SP.thumbs ? spaceTilesHTML(d) : `<div class="sp-list">${(d.node.children || []).map((c, i) => { const rel = SP.at ? SP.at + "/" + c.name : c.name; const pct = d.node.bytes ? (c.bytes / d.node.bytes) * 100 : 0; return `<div class="sp-row ${c.rest ? "rest" : ""} ${SP.hover === rel ? "hot" : ""}" data-sp-row="${esc(rel)}" style="--sc:${c.rest ? "var(--muted)" : spColor(i, 0)}">
              ${c.rest ? '<span class="pickcell"></span>' : `<input type="checkbox" class="pick" data-sp-pick="${esc(rel)}" ${SP.sel.has(rel) ? "checked" : ""}>`}
              <span class="sp-name" title="${esc(c.name)}">${c.rest ? `${c.rest.toLocaleString()} smaller items` : (SP.editing === rel ? `<span class="sp-edit"><input class="path-input" data-sp-newname="${esc(rel)}" value="${esc(c.name)}"><button class="btn small primary" data-sp-save="${esc(rel)}">Save</button><button class="btn small" data-sp-cancel>Cancel</button></span>` : tagDots(SP.tags && SP.tags[c.name]) + esc(c.name))}${c.error ? ` <span class="badge warn" title="${esc(c.error)}">${ICON.warn}</span>` : ""}${d.copies && d.copies[c.name] ? ` <span class="badge ok" title="A file of this name and size is on ${d.copies[c.name]} logged drive${d.copies[c.name] === 1 ? "" : "s"}; check one before letting this go">${ICON.check}on ${d.copies[c.name]} drive${d.copies[c.name] === 1 ? "" : "s"}</span>` : ""}${d.verdicts && d.verdicts[c.name] ? ` <span class="vd ${d.verdicts[c.name].level}" title="${esc(d.verdicts[c.name].why)}">${VD_LABEL[d.verdicts[c.name].level]}</span>` : ""}</span>
              <span class="sp-bar"><i style="width:${pct.toFixed(1)}%"></i></span>
              <span class="sp-size">${bytes(c.bytes)}</span>
              <span class="sp-acts wide">${c.dir && !c.rest ? `<button class="icon-btn" data-sp-open="${esc(rel)}" title="Look inside">${ICON.right}</button>` : ""}${!c.dir && !c.rest && spPreviewable(c.name) ? `<button class="icon-btn" data-sp-peek="${esc(rel)}" title="Preview">${ICON.eye}</button>` : ""}${!c.rest ? `<button class="icon-btn" data-sp-tag="${esc(rel)}" title="Tag colour">${ICON.tag}</button><button class="icon-btn" data-sp-edit="${esc(rel)}" title="Rename">${ICON.edit}</button><button class="icon-btn" data-sp-move="${esc(rel)}" title="Move to another folder here">${ICON.folder}</button>` : ""}${native && !c.rest && !spaceOnly ? `<button class="icon-btn" data-sp-reveal="${esc(rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`; }).join("") || '<div class="hint">Empty.</div>'}</div>`}
          <p class="hint" style="margin-top:10px">Sizes are what files take on disk. Each folder keeps its ${60} biggest items apart and rolls the rest into "smaller items". ${spaceOnly ? "The rest of the iPad, other apps and the system, is out of any app's reach; Settings › General › iPad Storage shows it." : "The Mac's own disk also holds system files, snapshots and purgeable space that no app can measure; what Finder says is used can be more than this."}</p>`}
        </div></div>`}
  </div>`;
  $("#sp-place").onchange = (e) => { SP.root = e.target.value; localSet("space.root", SP.root); SP.at = ""; SP.sel.clear(); SP.data = null; SP.dupes = null; SP.old = null; spaceLoad(); };
  for (const b of document.querySelectorAll("[data-sp-root]")) b.onclick = () => { SP.root = b.dataset.spRoot; localSet("space.root", SP.root); SP.at = ""; SP.sel.clear(); SP.data = null; SP.dupes = null; SP.old = null; SP.places = null; renderSpace().then(() => { const pl = (SP.places || []).find((x) => x.path === SP.root); if (pl && !pl.done && !pl.busy) $("#sp-scan") && $("#sp-scan").click(); }); };
  for (const b of document.querySelectorAll("[data-sp-fda]")) b.onclick = () => nativeCall("open", { url: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" });
  const scan = async () => { try { await api("/api/space/scan", { method: "POST", body: { path: SP.root } }); SP.busy = true; renderSpace(); loadJobs(); } catch (e) { toast(e.message, true); } };
  if ($("#sp-scan")) $("#sp-scan").onclick = scan;
  if ($("#sp-scan2")) $("#sp-scan2").onclick = scan;
  if ($("#sp-add")) $("#sp-add").onclick = async () => {
    const path = await nativeCall("pickFolder", {});
    if (!path) return;
    SP.places = null; SP.root = path; localSet("space.root", path); SP.data = null;
    await renderSpace();
    if ($("#sp-scan")) $("#sp-scan").click();
  };
  for (const b of document.querySelectorAll("[data-sp-go]")) b.onclick = () => { SP.at = b.dataset.spGo; spaceLoad(); };
  for (const b of document.querySelectorAll("[data-sp-open]")) b.onclick = () => { SP.at = b.dataset.spOpen; spaceLoad(); };
  for (const b of document.querySelectorAll("[data-sp-reveal]")) b.onclick = () => nativeCall("reveal", { path: SP.root.replace(/\/$/, "") + "/" + b.dataset.spReveal });
  for (const b of document.querySelectorAll("[data-sp-tag]")) b.onclick = (e) => { e.stopPropagation(); spaceTagMenu(b.dataset.spTag, SP.tags && SP.tags[b.dataset.spTag.split("/").pop()], b); };
  for (const b of document.querySelectorAll("[data-sp-edit]")) b.onclick = () => { SP.editing = b.dataset.spEdit; renderSpace().then(() => { const i = document.querySelector("[data-sp-newname]"); if (i) { i.focus(); const dot = i.value.lastIndexOf("."); i.setSelectionRange(0, dot > 0 ? dot : i.value.length); } }); };
  for (const b of document.querySelectorAll("[data-sp-cancel]")) b.onclick = () => { SP.editing = null; renderSpace(); };
  for (const b of document.querySelectorAll("[data-sp-save]")) b.onclick = async () => { const to = (document.querySelector("[data-sp-newname]") || {}).value.trim(); const rel = b.dataset.spSave; SP.editing = null; if (!to || to === rel.split("/").pop()) return renderSpace(); try { await spaceAct("rename", rel, to); } catch (e) { toast(e.message, true); renderSpace(); } };
  const nn = document.querySelector("[data-sp-newname]"); if (nn) nn.onkeydown = (e) => { if (e.key === "Enter") document.querySelector("[data-sp-save]").click(); else if (e.key === "Escape") { SP.editing = null; renderSpace(); } };
  for (const b of document.querySelectorAll("[data-sp-move]")) b.onclick = () => spaceMoveDialog(d, b.dataset.spMove);
  if ($("#sp-mkdir")) $("#sp-mkdir").onclick = () => { SP.mkdir = true; renderSpace().then(() => { const i = $("#sp-mkdir-name"); if (i) i.focus(); }); };
  if ($("#sp-mkdir-cancel")) $("#sp-mkdir-cancel").onclick = () => { SP.mkdir = false; renderSpace(); };
  if ($("#sp-mkdir-go")) { const go = async () => { const to = $("#sp-mkdir-name").value.trim(); if (!to) return; SP.mkdir = false; try { await spaceAct("mkdir", SP.at, to); } catch (e) { toast(e.message, true); renderSpace(); } }; $("#sp-mkdir-go").onclick = go; $("#sp-mkdir-name").onkeydown = (e) => { if (e.key === "Enter") go(); else if (e.key === "Escape") { SP.mkdir = false; renderSpace(); } }; }
  // The tags of what is listed, read once per folder.
  if (d && SP.view === "folders" && (!SP.tags || SP.tagsAt !== SP.root + "\0" + SP.at)) { const at = SP.root + "\0" + SP.at; SP.tagsAt = at; api(`/api/space/tags?path=${encodeURIComponent(SP.root)}&rel=${encodeURIComponent(SP.at)}`).then((t) => { if (SP.tagsAt === at) { SP.tags = t || {}; if (Object.keys(SP.tags).length) renderSpace(); } }).catch(() => {}); }
  for (const b of document.querySelectorAll("[data-sp-peek]")) b.onclick = () => spacePeek(b.dataset.spPeek, d, SP.view === "move" && SP.mv.files ? SP.mv.files.filter((f) => spPreviewable(f.rel)) : null);
  fillNativeThumbs();
  for (const b of document.querySelectorAll("[data-spm]")) b.onclick = () => { SP.thumbs = b.dataset.spm === "thumbs"; localSet("space.thumbs", SP.thumbs ? "1" : "0"); renderSpace(); };
  // A tile's picture opens the preview; a folder tile opens the folder.
  for (const t of document.querySelectorAll(".sp-tile")) {
    const rel = t.dataset.spRow, c = (d.node.children || []).find((x) => (SP.at ? SP.at + "/" + x.name : x.name) === rel);
    const pic = t.querySelector(".sp-pic");
    if (pic && !pic.dataset.spPeek) pic.onclick = () => { if (c && c.dir) { SP.at = rel; spaceLoad(); } else if (c && spPreviewable(c.name)) spacePeek(rel, d); };
  }
  // A tick with shift held ticks (or clears) everything from the last
  // tick to this one, in the order shown.
  for (const el of document.querySelectorAll("[data-sp-pick]")) el.onchange = () => {
    const picks = [...document.querySelectorAll("[data-sp-pick]")].map((x) => x.dataset.spPick);
    const a = picks.indexOf(SP.lastPick), z = picks.indexOf(el.dataset.spPick);
    const range = SP.shift && a >= 0 && z >= 0 ? picks.slice(Math.min(a, z), Math.max(a, z) + 1) : [el.dataset.spPick];
    for (const rel of range) { if (el.checked) SP.sel.add(rel); else SP.sel.delete(rel); }
    SP.lastPick = el.dataset.spPick;
    renderSpace();
  };
  for (const row of document.querySelectorAll("[data-sp-row]")) { row.onmouseenter = () => spHover(row.dataset.spRow); row.onmouseleave = () => spHover(null); row.ondblclick = () => { const c = (d.node.children || []).find((x) => (SP.at ? SP.at + "/" + x.name : x.name) === row.dataset.spRow); if (c && c.dir) { SP.at = row.dataset.spRow; spaceLoad(); } }; }
  if ($("#sp-trash")) $("#sp-trash").onclick = () => spaceTrash(d);
  for (const b of document.querySelectorAll("[data-spv]")) b.onclick = () => { SP.view = b.dataset.spv; if (SP.view === "dupes" && !SP.dupes) spaceDupesLoad(); if (SP.view === "old" && !SP.old) spaceOldLoad(); if (SP.view === "move" && !SP.mv.files) spaceMoveLoad(); renderSpace(); };
  spaceMoveWire();
  if ($("#sp-dupes-go")) $("#sp-dupes-go").onclick = async () => { try { await api("/api/space/dupes?path=" + encodeURIComponent(SP.root), { method: "POST" }); SP.dupes = { busy: true }; renderSpace(); loadJobs(); } catch (e) { toast(e.message, true); } };
  for (const b of document.querySelectorAll("[data-sp-dkind]")) b.onclick = () => { SP.dkind = b.dataset.spDkind; renderSpace(); };
  if ($("#sp-dfolder")) $("#sp-dfolder").onchange = (e) => { SP.dfolder = e.target.value; renderSpace(); };
  if ($("#sp-dupes-all")) $("#sp-dupes-all").onclick = () => {
    const kindOf = (p) => SP_KIND[(p.split(".").pop() || "").toLowerCase()] || "other", topOf = (p) => p.includes("/") ? p.slice(0, p.indexOf("/")) : "";
    for (const g of SP.dupes.groups) if ((!SP.dkind || kindOf(g.paths[0]) === SP.dkind) && (!SP.dfolder || g.paths.some((p) => topOf(p) === SP.dfolder))) g.paths.forEach((p, i) => { if (i) SP.sel.add(p); else SP.sel.delete(p); });
    renderSpace();
  };
  for (const b of document.querySelectorAll("[data-sp-dupe-all]")) b.onclick = () => { const g = SP.dupes.groups[+b.dataset.spDupeAll]; g.paths.forEach((p, i) => { if (i) SP.sel.add(p); else SP.sel.delete(p); }); renderSpace(); };
  for (const el of document.querySelectorAll("[data-sp-keep]")) el.onchange = () => {
    // In a set of identical files, one is kept and the rest ticked.
    const g = SP.dupes.groups[+el.dataset.spKeep];
    for (const p of g.paths) SP.sel.delete(p);
    for (const p of g.paths) if (p !== el.value) SP.sel.add(p);
    renderSpace();
  };
  const sus = document.querySelector(".sp-sus");
  if (sus) sus.ontoggle = () => { SP.susOpen = sus.open; };
  wireSunburst(d);
  if (!d && !SP.busy && place.done) spaceLoad();
  if (place.busy && !SP.busy) { SP.busy = true; renderSpace(); }
}
// spaceTilesHTML is the folder as a grid of thumbnails: each picture or
// clip shows itself, so the ones to go can be ticked at a glance.
function spaceTilesHTML(d) {
  const kids = (d.node.children || []).filter((c) => !c.rest);
  const rest = (d.node.children || []).find((c) => c.rest);
  return `<div class="sp-tiles">${kids.map((c) => {
    const rel = SP.at ? SP.at + "/" + c.name : c.name;
    const pic = !c.dir && spPreviewable(c.name) ? spPicHTML(rel, 320) : `<span class="sp-glyph">${c.dir ? ICON.folder : ICON.file}</span>`;
    const vd = d.verdicts && d.verdicts[c.name];
    return `<div class="sp-tile ${SP.sel.has(rel) ? "picked" : ""} ${SP.hover === rel ? "hot" : ""}" data-sp-row="${esc(rel)}" title="${esc(c.name)}">
      <div class="sp-pic">${pic}</div>
      <label class="sp-tile-foot"><input type="checkbox" class="pick" data-sp-pick="${esc(rel)}" ${SP.sel.has(rel) ? "checked" : ""}><span class="sp-tile-name">${tagDots(SP.tags && SP.tags[c.name])}${esc(c.name)}</span><span class="sp-size">${bytes(c.bytes)}</span></label>
      ${vd && vd.level !== "check" ? `<span class="vd ${vd.level}" title="${esc(vd.why)}">${VD_LABEL[vd.level]}</span>` : ""}
      <span class="sp-acts">${c.dir ? `<button class="icon-btn" data-sp-open="${esc(rel)}" title="Look inside">${ICON.right}</button>` : ""}<button class="icon-btn" data-sp-tag="${esc(rel)}" title="Tag colour">${ICON.tag}</button><button class="icon-btn" data-sp-edit="${esc(rel)}" title="Rename">${ICON.edit}</button><button class="icon-btn" data-sp-move="${esc(rel)}" title="Move to another folder here">${ICON.folder}</button>${native && !spaceOnly ? `<button class="icon-btn" data-sp-reveal="${esc(rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`;
  }).join("")}</div>${rest ? `<div class="hint" style="margin-top:8px">${rest.rest.toLocaleString()} smaller items · ${bytes(rest.bytes)}</div>` : ""}`;
}
function spPreviewable(name) { const e = (name.split(".").pop() || "").toLowerCase(), k = SP_KIND[e]; return k === "image" || k === "video" || e === "pdf"; }
// Pictures of files: the engine renders them on the Mac; on the iPad
// the app does, through the same renderer the Files app uses, so
// HEIC, clips, PDFs and raws all show. Native ones are filled in
// after the page is drawn, a few at a time, and remembered.
const nativeThumbs = spaceOnly && !!native;
const NT = { have: new Map(), queue: [], running: 0 };
function spPicHTML(rel, w, alt) {
  const fallback = `this.replaceWith(Object.assign(document.createElement('span'), { className: 'sp-nopic', textContent: 'no preview' }))`;
  if (!nativeThumbs) return `<img loading="lazy" src="/api/space/thumb?path=${encodeURIComponent(SP.root)}&rel=${encodeURIComponent(rel)}&w=${w}&t=${encodeURIComponent(token)}" alt="${alt || ""}" onerror="${fallback}">`;
  const key = SP.root + "\0" + rel + "\0" + w, had = NT.have.get(key);
  if (had === null) return `<span class="sp-nopic">no preview</span>`;
  return `<img data-nthumb="${esc(rel)}" data-w="${w}" ${had ? `src="${had}"` : ""} alt="${alt || ""}">`;
}
function nativeThumb(rel, w) {
  const key = SP.root + "\0" + rel + "\0" + w;
  if (NT.have.has(key)) return Promise.resolve(NT.have.get(key));
  return nativeCall("thumb", { path: SP.root.replace(/\/$/, "") + "/" + rel, w }).then((u) => { if (NT.have.size > 400) NT.have.clear(); NT.have.set(key, u || null); return u || null; });
}
function fillNativeThumbs() {
  if (!nativeThumbs) return;
  for (const img of document.querySelectorAll("img[data-nthumb]:not([src]):not([data-asked])")) {
    img.dataset.asked = "1";
    NT.queue.push(img);
  }
  const next = () => {
    if (NT.running >= 3 || !NT.queue.length) return;
    const img = NT.queue.shift();
    if (!document.contains(img)) return next();
    NT.running++;
    nativeThumb(img.dataset.nthumb, +img.dataset.w).then((u) => { if (u) img.src = u; else img.replaceWith(Object.assign(document.createElement("span"), { className: "sp-nopic", textContent: "no preview" })); }).catch(() => {}).finally(() => { NT.running--; next(); });
  };
  next(); next(); next();
}
// spacePeek shows one file large, with the others in the same folder a
// key press away. A file Lumpy can't render says why.
function spacePeek(rel, d, files) {
  const here = SP.at ? SP.at + "/" : "";
  const list = files ? files.map((f) => f.rel) : (d.node.children || []).filter((c) => !c.dir && !c.rest && spPreviewable(c.name)).map((c) => here + c.name);
  let i = Math.max(0, list.indexOf(rel));
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; document.removeEventListener("keydown", keys); };
  const keys = (e) => { if (e.key === "Escape") close(); else if (e.key === "ArrowRight" && i < list.length - 1) { i++; draw(); } else if (e.key === "ArrowLeft" && i > 0) { i--; draw(); } };
  document.addEventListener("keydown", keys);
  let tx = null;
  const swipe = { start: (e) => { tx = e.touches[0].clientX; }, end: (e) => { if (tx === null) return; const dx = e.changedTouches[0].clientX - tx; tx = null; if (dx < -60 && i < list.length - 1) { i++; draw(); } else if (dx > 60 && i > 0) { i--; draw(); } } };
  const draw = async () => {
    const p = list[i], c = files ? (files.find((f) => f.rel === p) || {}) : ((d.node.children || []).find((x) => here + x.name === p) || {});
    if (c.size && !c.bytes) c.bytes = c.size;
    const src = `/api/space/thumb?path=${encodeURIComponent(SP.root)}&rel=${encodeURIComponent(p)}&w=1600&t=${token}`;
    openSheet(`<div class="sheet-head"><div><h2>${esc(p.split("/").pop())}</h2><div class="sub">${bytes(c.bytes || 0)} · ${i + 1} of ${list.length} ${files ? "listed" : "in this folder"} · ← → to step through</div></div><div class="actions">${native ? `<button class="btn small" data-sp-reveal="${esc(p)}">${ICON.reveal}Show in Finder</button>` : ""}<button class="icon-btn" id="pk-prev" ${i ? "" : "disabled"}>${ICON.back}</button><button class="icon-btn" id="pk-next" ${i < list.length - 1 ? "" : "disabled"}>${ICON.right}</button><button class="icon-btn" id="pk-close">${ICON.x}</button></div></div>
      <div class="sheet-body peek"><div class="peek-box">${loadingGlyph("Rendering…")}</div></div>`, true);
    $("#pk-close").onclick = close;
    const bx = $(".peek-box"); if (bx) { bx.ontouchstart = swipe.start; bx.ontouchend = swipe.end; }
    $("#pk-prev").onclick = () => { if (i > 0) { i--; draw(); } };
    $("#pk-next").onclick = () => { if (i < list.length - 1) { i++; draw(); } };
    for (const b of document.querySelectorAll(".sheet [data-sp-reveal]")) b.onclick = () => nativeCall("reveal", { path: SP.root.replace(/\/$/, "") + "/" + b.dataset.spReveal });
    try {
      let url;
      if (nativeThumbs) { url = await nativeThumb(p, 1600); if (!url) throw new Error("the iPad has no preview for this file"); }
      else {
        const res = await fetch(src, { headers: { "X-Lumpy-Token": token } });
        if (!res.ok) { let why = res.statusText; try { why = (await res.json()).error || why; } catch {} throw new Error(why); }
        url = URL.createObjectURL(await res.blob());
      }
      const box = $(".peek-box"); if (box) box.innerHTML = `<img src="${url}" alt="">`;
    } catch (e) { const box = $(".peek-box"); if (box) box.innerHTML = `<div class="empty">${ICON.warn}<h2>No preview</h2><p>${esc(e.message)}</p><p class="hint">If this is a DPX or DNG Lumpy should read, tell the developer which camera or app wrote it.</p></div>`; }
  };
  draw();
}
// spaceCoverage says what the measurement covers of the disk's used
// space, and what it can't: other places, and folders macOS keeps
// from apps without Full Disk Access.
function spaceCoverage(d, place) {
  const used = place.size - place.free, rest = Math.max(0, used - d.total);
  const isHome = SP.root !== "/";
  if (rest < used * 0.05 && !d.unreadable) return "";
  return `<div class="alert" style="margin:0 0 12px"><b>${bytes(d.total)} of the ${bytes(used)} used is here.</b> ${isHome ? `Home is one part of the disk: apps, macOS itself, other users and system data live outside it. <button class="btn small" data-sp-root="/">Measure Macintosh HD</button>` : "The rest is system data, local Time Machine snapshots and purgeable space that macOS doesn't let any app measure; Finder's Storage settings can clear snapshots. Your files are counted once, under Users, Applications and Library; the Data volume's second mount at System › Volumes is left out."}${d.unreadable ? ` <span style="display:block;margin-top:6px">${d.unreadable} folder${d.unreadable === 1 ? "" : "s"} couldn't be read: macOS keeps parts of Library (Mail, Messages, Safari, some app data) from apps without <b>Full Disk Access</b>. ${native ? `<button class="btn small" data-sp-fda>Open Privacy &amp; Security…</button>` : "System Settings → Privacy &amp; Security → Full Disk Access"}, add Lumpy, then Measure again.</span>` : ""}</div>`;
}
function spaceDupesHTML() {
  const dd = SP.dupes;
  if (!dd) return loadingGlyph("");
  if (dd.busy) return `<div class="empty">${loadingGlyph("")}<h2>Reading files that share a size</h2><p>${bytes(dd.hashed || 0)} read so far. Only files the same length as another are read, and each is checksummed in full, so a match is a real one. See Activity.</p></div>`;
  if (!dd.ready) return `<div class="empty">${ICON.copy}<h2>Find the same file twice</h2><p>Lumpy compares every file of 1 MB or more by length, then reads the ones that share a length and checksums them, so two files count as the same only when their contents are. Takes a while on a big disk; it runs in Activity.</p><p><button class="btn primary" id="sp-dupes-go">${ICON.play}Find duplicates</button></p></div>`;
  if (!dd.groups.length) return `<div class="alert ok" style="margin-top:8px"><b>No duplicates</b> among ${dd.candidates.toLocaleString()} files of 1 MB or more.</div>`;
  const kindOf = (p) => { const e = (p.split(".").pop() || "").toLowerCase(); return SP_KIND[e] || "other"; };
  const topOf = (p) => p.includes("/") ? p.slice(0, p.indexOf("/")) : "";
  const kinds = {}; const folders = {};
  for (const g of dd.groups) { const k = kindOf(g.paths[0]); kinds[k] = (kinds[k] || 0) + g.wasted; for (const p of g.paths) { const t = topOf(p); folders[t] = (folders[t] || 0) + 1; } }
  const shown = dd.groups.filter((g) => (!SP.dkind || kindOf(g.paths[0]) === SP.dkind) && (!SP.dfolder || g.paths.some((p) => topOf(p) === SP.dfolder)));
  const shownWaste = shown.reduce((t, g) => t + g.wasted, 0);
  const previewable = (p) => ["image", "video"].includes(kindOf(p));
  const allDone = shown.every((g) => g.paths.filter((p) => SP.sel.has(p)).length === g.paths.length - 1);
  return `<div class="sp-head"><b>${bytes(dd.wasted)}</b><span class="hint">· in extra copies, ${dd.groups.length} set${dd.groups.length === 1 ? "" : "s"} of identical files</span></div>
    <div class="sp-filters"><span class="seg">${[["", "All"], ...Object.entries(SP_KIND_LABEL).filter(([k]) => kinds[k])].map(([k, l]) => `<button data-sp-dkind="${k}" class="${SP.dkind === k ? "on" : ""}">${l}${k && kinds[k] ? ` <small>${bytes(kinds[k])}</small>` : ""}</button>`).join("")}</span>
      <label class="select"><select id="sp-dfolder"><option value="">Anywhere</option>${Object.keys(folders).sort().map((f) => `<option value="${esc(f)}" ${SP.dfolder === f ? "selected" : ""}>${esc(f || "(top level)")} · ${folders[f]}</option>`).join("")}</select></label>
      <button class="btn small" id="sp-dupes-all" ${allDone ? "disabled" : ""} title="In every set shown, keep the first copy and tick the rest">${ICON.check}Apply to all shown: keep the first, trash the rest</button>
      <span class="hint">${shown.length} set${shown.length === 1 ? "" : "s"} · ${bytes(shownWaste)} to free</span></div>
    <p class="hint" style="margin:0 0 8px">Pick which copy to keep in each set; the others are ticked for the Trash. Two copies on the same disk protect nothing: a backup lives on another drive. Pictures and clips show side by side so you can see they're the same.</p>
    <div class="sp-dupes">${shown.map((g) => { const gi = dd.groups.indexOf(g); return `<div class="sp-dgroup"><div class="sp-dhead"><b>${esc(g.paths[0].split("/").pop())}</b><span class="hint">· ${g.paths.length} copies · ${bytes(g.size)} each · ${bytes(g.wasted)} to free</span><span class="vd ok" style="margin-left:auto">${esc(SP_KIND_LABEL[kindOf(g.paths[0])] || "file")}</span></div>
      ${previewable(g.paths[0]) ? `<div class="sp-dpics">${g.paths.map((p) => `<figure class="${SP.sel.has(p) ? "go" : ""}" title="${esc(p)}"><img loading="lazy" src="/api/space/thumb?path=${encodeURIComponent(SP.root)}&rel=${encodeURIComponent(p)}&t=${token}" onerror="this.parentNode.classList.add('nopic')"><figcaption>${esc(topOf(p) || "top level")}</figcaption></figure>`).join("")}</div>` : ""}
      ${g.paths.map((p) => `<label class="sp-drow ${SP.sel.has(p) ? "go" : ""}"><input type="radio" name="keep${gi}" data-sp-keep="${gi}" value="${esc(p)}" ${!SP.sel.has(p) && g.paths.every((q) => q === p || SP.sel.has(q)) ? "checked" : ""} title="Keep this one"><span class="mono" title="${esc(p)}">${esc(p)}</span>${SP.sel.has(p) ? '<span class="vd check">to the Trash</span>' : '<span class="vd ok">keep</span>'}${native ? `<button class="icon-btn" data-sp-reveal="${esc(p)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</label>`).join("")}
      <div class="sp-dpick"><button class="btn small" data-sp-dupe-all="${gi}" ${g.paths.filter((p) => SP.sel.has(p)).length === g.paths.length - 1 ? "disabled" : ""}>Keep the first, trash the rest</button></div></div>`; }).join("")}</div>`;
}
const SP_KIND = { jpg: "image", jpeg: "image", png: "image", gif: "image", tif: "image", tiff: "image", dng: "image", dpx: "image", heic: "image", cr2: "image", cr3: "image", arw: "image", nef: "image", raf: "image", psd: "image", exr: "image", webp: "image",
  mov: "video", mp4: "video", m4v: "video", mxf: "video", avi: "video", mkv: "video", mts: "video", m2ts: "video", braw: "video", r3d: "video", ari: "video", crm: "video", webm: "video",
  wav: "audio", aif: "audio", aiff: "audio", mp3: "audio", flac: "audio", m4a: "audio", caf: "audio", bwf: "audio",
  pdf: "document", doc: "document", docx: "document", xls: "document", xlsx: "document", key: "document", pages: "document", numbers: "document", txt: "document", prproj: "document", drp: "document", fcpbundle: "document",
  zip: "archive", dmg: "archive", pkg: "archive", iso: "archive", tar: "archive", gz: "archive", "7z": "archive", rar: "archive", app: "app" };
const SP_KIND_LABEL = { image: "Pictures", video: "Clips", audio: "Audio", document: "Documents", archive: "Archives & installers", app: "Apps", other: "Other" };
function spaceOldHTML() {
  const od = SP.old;
  if (!od) return loadingGlyph("");
  const when2 = (t) => t ? new Date(t * 1000).toLocaleDateString(undefined, { month: "short", year: "numeric" }) : "never";
  return `<div class="sp-head"><b>${bytes(od.total)}</b><span class="hint">· in ${od.files.length}${od.files.length >= 400 ? "+" : ""} files of 1 MB or more neither changed nor opened in ${od.days} days</span></div>
    <p class="hint" style="margin:0 0 8px">Opened means read by anything, which macOS records for its own disk. Old isn't the same as unneeded: an archive is meant to sit. Tick what should go.</p>
    ${od.apps.length ? `<div class="sp-sus" style="margin-bottom:10px"><b>Apps not opened in ${od.days} days</b><div class="sp-list" style="margin-top:6px">${od.apps.map((a) => `<div class="sp-row" data-sp-row="${esc(a.rel)}"><input type="checkbox" class="pick" data-sp-pick="${esc(a.rel)}" ${SP.sel.has(a.rel) ? "checked" : ""}><span class="sp-name">${esc(a.name)}<small class="hint"> · last opened ${when2(a.lastUsed)}</small></span><span></span><span class="sp-size">${bytes(a.bytes)}</span><span class="sp-acts">${native ? `<button class="icon-btn" data-sp-reveal="${esc(a.rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`).join("")}</div><p class="hint" style="margin:6px 0 0">From Spotlight's last-used date. Apps it doesn't know about aren't listed.</p></div>` : ""}
    <div class="sp-list">${od.files.map((f) => `<div class="sp-row" data-sp-row="${esc(f.rel)}"><input type="checkbox" class="pick" data-sp-pick="${esc(f.rel)}" ${SP.sel.has(f.rel) ? "checked" : ""}><span class="sp-name" title="${esc(f.rel)}">${esc(f.rel.split("/").pop())}<small class="hint mono"> ${esc(f.rel.includes("/") ? f.rel.slice(0, f.rel.lastIndexOf("/")) : "")}</small></span><span class="hint" style="white-space:nowrap">${when2(Math.max(f.mtime, f.atime))}</span><span class="sp-size">${bytes(f.disk)}</span><span class="sp-acts">${native ? `<button class="icon-btn" data-sp-reveal="${esc(f.rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`).join("") || '<div class="hint">Nothing that old.</div>'}</div>`;
}
// Move files: two panes, like two Files windows but with the finding
// done. Left, every file of a kind anywhere in the place (all the
// clips, all the PDFs), to pick from; right, the place's folders:
// tap one and the picked files go there. One step back is kept.
const MV_KINDS = [["all", "Everything"], ["video", "Clips"], ["image", "Pictures"], ["audio", "Audio"], ["pdf", "PDFs"], ["document", "Documents"], ["project", "Project files"], ["archive", "Archives"], ["other", "Other"]];
async function spaceMoveLoad() {
  const m = SP.mv;
  const want = SP.root + "\0" + m.kind + "\0" + m.q;
  m.loading = want;
  try {
    const [f, t] = await Promise.all([api(`/api/space/files?path=${encodeURIComponent(SP.root)}&kind=${m.kind}&q=${encodeURIComponent(m.q)}`), api(`/api/space/folders?path=${encodeURIComponent(SP.root)}`)]);
    if (m.loading !== want) return;
    m.files = f.files; m.total = f.total; m.more = f.more; m.counts = f.counts || {}; m.folders = t;
    if (!m.open.size) for (const c of t.children || []) m.open.add(c.rel);
  } catch (e) { toast(e.message, true); m.files = m.files || []; m.folders = m.folders || { children: [] }; }
  renderSpace();
}
function spaceMoveHTML() {
  const m = SP.mv;
  if (!m.files || !m.folders) return loadingGlyph("Finding every file…");
  const when2 = (t) => t ? new Date(t * 1000).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" }) : "";
  const picked = m.files.filter((f) => SP.sel.has(f.rel));
  const pickedBytes = picked.reduce((t, f) => t + f.size, 0);
  const all = Object.values(m.counts).reduce((t, n) => t + n, 0);
  const dir = (rel) => rel.includes("/") ? rel.slice(0, rel.lastIndexOf("/")) : "";
  const rows = m.files.map((f) => `<div class="sp-row mv-file ${SP.sel.has(f.rel) ? "picked" : ""}" data-sp-row="${esc(f.rel)}"><input type="checkbox" class="pick" data-sp-pick="${esc(f.rel)}" ${SP.sel.has(f.rel) ? "checked" : ""}><span class="sp-name" title="${esc(f.rel)}">${tagDots(f.tags)}${esc(f.rel.split("/").pop())}<small class="hint mono"> <button class="linkish" data-mv-go="${esc(dir(f.rel))}" title="See it in Folders">${esc(dir(f.rel) || "top of the place")}</button></small></span><span class="hint" style="white-space:nowrap">${when2(f.mtime)}</span><span class="sp-size">${bytes(f.size)}</span><span class="sp-acts">${!spPreviewable(f.rel) ? "" : `<button class="icon-btn" data-sp-peek="${esc(f.rel)}" title="Preview">${ICON.eye}</button>`}${native && !spaceOnly ? `<button class="icon-btn" data-sp-reveal="${esc(f.rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`).join("");
  const tree = (n, depth) => (n.children || []).map((c) => {
    const open = m.open.has(c.rel), kids = (c.children || []).length;
    return `<div class="mv-row" style="padding-left:${6 + depth * 18}px" data-mv-row="${esc(c.rel)}">
      <button class="mv-disc ${kids ? "" : "none"} ${open ? "open" : ""}" data-mv-open="${esc(c.rel)}" ${kids ? "" : "disabled"}>${ICON.right}</button>
      <button class="mv-name" data-mv-to="${esc(c.rel)}" title="${SP.sel.size ? `Move ${SP.sel.size} here` : esc(c.rel)}">${ICON.folder}<span>${esc(c.name)}</span>${c.files ? `<small class="hint">${c.files}</small>` : ""}</button>
      <button class="icon-btn mv-mk" data-mv-mk="${esc(c.rel)}" title="New folder inside">${ICON.plus}</button></div>
      ${m.mkdirIn === c.rel ? mkrow(c.rel, depth + 1) : ""}${open ? tree(c, depth + 1) : ""}`;
  }).join("");
  const mkrow = (rel, depth) => `<div class="sp-edit mv-mkrow" style="padding-left:${6 + depth * 18}px"><input class="path-input" id="mv-mk-name" placeholder="New folder name"><button class="btn small primary" id="mv-mk-go">Make</button><button class="btn small" id="mv-mk-cancel">Cancel</button></div>`;
  return `<div class="mv-panes ${SP.sel.size ? "armed" : ""}">
    <div class="mv-files">
      <div class="chips mv-kinds">${MV_KINDS.map(([k, l]) => `<button class="chip ${m.kind === k ? "on" : ""}" data-mv-kind="${k}">${l}<small>${(k === "all" ? all : m.counts[k] || 0).toLocaleString()}</small></button>`).join("")}</div>
      <div class="mv-tools"><label class="search"><input class="path-input" id="mv-q" value="${esc(m.q)}" placeholder="Filter by name"></label><span class="hint">${m.files.length.toLocaleString()}${m.more ? "+" : ""} files · ${bytes(m.total)}</span><span class="right"><div class="seg sp-mode" title="Show the files as thumbnails"><button data-mvm="list" class="${m.thumbs ? "" : "on"}">List</button><button data-mvm="thumbs" class="${m.thumbs ? "on" : ""}">Thumbnails</button></div><button class="btn small" id="mv-all" ${m.files.length ? "" : "disabled"}>Pick all</button><button class="btn small" id="mv-none" ${SP.sel.size ? "" : "disabled"}>None</button></span></div>
      ${m.thumbs && m.files.length ? `<div class="mv-filelist"><div class="sp-tiles">${m.files.map((f) => `<div class="sp-tile ${SP.sel.has(f.rel) ? "picked" : ""}" data-sp-row="${esc(f.rel)}" title="${esc(f.rel)}">
        <div class="sp-pic" ${spPreviewable(f.rel) ? `data-sp-peek="${esc(f.rel)}"` : ""}>${spPreviewable(f.rel) ? spPicHTML(f.rel, 320) : `<span class="sp-glyph">${ICON.file}</span>`}</div>
        <label class="sp-tile-foot"><input type="checkbox" class="pick" data-sp-pick="${esc(f.rel)}" ${SP.sel.has(f.rel) ? "checked" : ""}><span class="sp-tile-name">${tagDots(f.tags)}${esc(f.rel.split("/").pop())}</span><span class="sp-size">${bytes(f.size)}</span></label>
        <small class="hint mono sp-tile-where"><button class="linkish" data-mv-go="${esc(dir(f.rel))}" title="See it in Folders">${esc(dir(f.rel) || "top")}</button></small>
        <span class="sp-acts">${native && !spaceOnly ? `<button class="icon-btn" data-sp-reveal="${esc(f.rel)}" title="Show in Finder">${ICON.reveal}</button>` : ""}</span></div>`).join("")}</div></div>` : `<div class="sp-list mv-filelist">${rows || `<div class="hint">No ${(MV_KINDS.find(([k]) => k === m.kind) || ["", "files"])[1].toLowerCase()} ${m.q ? "called that " : ""}in ${esc((SP.places.find((p) => p.path === SP.root) || {}).name || "this place")}.</div>`}</div>`}
    </div>
    <div class="mv-dest">
      <div class="sp-head"><b>${SP.sel.size ? `${SP.sel.size} picked · ${bytes(pickedBytes)}` : "Where things go"}</b></div>
      <p class="hint" style="margin:0 0 8px">${m.busy ? "Moving…" : SP.sel.size ? "Tap the folder they go to. Each goes on the same disk, so it is instant." : "Pick files on the left, then tap a folder here. The + makes a folder inside one."}</p>
      ${m.undo ? `<div class="mv-undo"><span>${m.undo.n} moved to <b>${esc(m.undo.to || "the top")}</b></span><button class="btn small" id="mv-undo">Undo</button></div>` : ""}
      <div class="mv-tree">
        <div class="mv-row" data-mv-row=""><span class="mv-disc none"></span><button class="mv-name" data-mv-to="" title="${SP.sel.size ? `Move ${SP.sel.size} to the top` : "The top of the place"}">${ICON.folder}<span>${esc((SP.places.find((p) => p.path === SP.root) || {}).name || "Top")}</span>${m.folders.files ? `<small class="hint">${m.folders.files}</small>` : ""}</button><button class="icon-btn mv-mk" data-mv-mk="" title="New folder at the top">${ICON.plus}</button></div>
        ${m.mkdirIn === "" ? mkrow("", 1) : ""}
        ${tree(m.folders, 1)}
      </div>
    </div>
  </div>`;
}
function spaceMoveWire() {
  const m = SP.mv;
  for (const b of document.querySelectorAll("[data-mvm]")) b.onclick = () => { m.thumbs = b.dataset.mvm === "thumbs"; localSet("space.mvthumbs", m.thumbs ? "1" : "0"); renderSpace(); };
  for (const b of document.querySelectorAll("[data-mv-kind]")) b.onclick = () => { m.kind = b.dataset.mvKind; m.files = null; renderSpace(); spaceMoveLoad(); };
  const q = $("#mv-q");
  if (q) { q.oninput = () => { m.q = q.value; clearTimeout(m.qt); m.qt = setTimeout(() => { m.focus = true; spaceMoveLoad(); }, 250); }; if (m.focus) { m.focus = false; q.focus(); q.setSelectionRange(q.value.length, q.value.length); } }
  if ($("#mv-all")) $("#mv-all").onclick = () => { for (const f of m.files) SP.sel.add(f.rel); renderSpace(); };
  if ($("#mv-none")) $("#mv-none").onclick = () => { SP.sel.clear(); renderSpace(); };
  for (const b of document.querySelectorAll("[data-mv-go]")) b.onclick = (e) => { e.preventDefault(); SP.view = "folders"; SP.at = b.dataset.mvGo; spaceLoad(); };
  for (const b of document.querySelectorAll("[data-mv-open]")) b.onclick = () => { const r = b.dataset.mvOpen; if (m.open.has(r)) m.open.delete(r); else m.open.add(r); renderSpace(); };
  for (const b of document.querySelectorAll("[data-mv-to]")) b.onclick = () => { const r = b.dataset.mvTo; if (!SP.sel.size) { if (m.open.has(r)) m.open.delete(r); else m.open.add(r); renderSpace(); return; } spaceMoveTo(r); };
  for (const b of document.querySelectorAll("[data-mv-mk]")) b.onclick = () => { m.mkdirIn = b.dataset.mvMk; m.open.add(b.dataset.mvMk); renderSpace().then(() => { const i = $("#mv-mk-name"); if (i) i.focus(); }); };
  if ($("#mv-mk-cancel")) $("#mv-mk-cancel").onclick = () => { m.mkdirIn = null; renderSpace(); };
  if ($("#mv-mk-go")) { const go = async () => { const to = $("#mv-mk-name").value.trim(); if (!to) return; const inside = m.mkdirIn; m.mkdirIn = null; try { await api("/api/space/organise", { method: "POST", body: { path: SP.root, op: "mkdir", rel: inside, to } }); m.open.add(inside ? inside + "/" + to : to); toast(`${to} made`); } catch (e) { toast(e.message, true); } spaceMoveLoad(); }; $("#mv-mk-go").onclick = go; $("#mv-mk-name").onkeydown = (e) => { if (e.key === "Enter") go(); else if (e.key === "Escape") { m.mkdirIn = null; renderSpace(); } }; }
  if ($("#mv-undo")) $("#mv-undo").onclick = spaceMoveUndo;
}
// spaceMoveTo moves what is picked into one folder of the place; the
// measurement is redone behind, and the two panes are read again.
async function spaceMoveTo(to) {
  const m = SP.mv;
  const rels = [...SP.sel];
  const base = (r) => r.split("/").pop();
  const dir = (r) => r.includes("/") ? r.slice(0, r.lastIndexOf("/")) : "";
  m.busy = true; renderSpace();
  try {
    const r = await api("/api/space/organise", { method: "POST", body: { path: SP.root, op: "move", rels, to } });
    m.undo = r.moved.length ? { n: r.moved.length, to, back: r.moved.map((f) => ({ now: (to ? to + "/" : "") + base(f), dir: dir(f) })) } : null;
    SP.sel.clear();
    if (r.failed.length) toast(r.failed.length === 1 ? r.failed[0] : `${r.failed.length} could not go: ${r.failed[0]}…`, true);
    else toast(`${r.moved.length} moved to ${to || "the top"}`);
  } catch (e) { toast(e.message, true); }
  m.busy = false;
  await spaceMoveLoad();
}
async function spaceMoveUndo() {
  const m = SP.mv, u = m.undo;
  if (!u) return;
  m.undo = null; m.busy = true; renderSpace();
  const byDir = {};
  for (const b of u.back) (byDir[b.dir] = byDir[b.dir] || []).push(b.now);
  try {
    for (const [dir, rels] of Object.entries(byDir)) await api("/api/space/organise", { method: "POST", body: { path: SP.root, op: "move", rels, to: dir } });
    toast(`${u.n} put back`);
  } catch (e) { toast(e.message, true); }
  m.busy = false;
  await spaceMoveLoad();
}
async function spaceDupesLoad() {
  try { SP.dupes = await api("/api/space/dupes?path=" + encodeURIComponent(SP.root)); } catch (e) { SP.dupes = { ready: false }; }
  renderSpace();
}
async function spaceOldLoad() {
  try { SP.old = await api("/api/space/old?path=" + encodeURIComponent(SP.root)); } catch (e) { toast(e.message, true); SP.old = { files: [], apps: [], total: 0, days: 365 }; }
  renderSpace();
}
async function spaceLoad() {
  try {
    const r = await api(`/api/space?path=${encodeURIComponent(SP.root)}&at=${encodeURIComponent(SP.at)}&depth=4`);
    if (r.busy) { SP.busy = true; SP.data = { root: SP.root, bytes: r.bytes }; }
    else { if (SP.busy || !SP.data || SP.data.root !== SP.root || (SP.data.measured && r.measured !== SP.data.measured)) { SP.dupes = null; SP.old = null; SP.mv.files = SP.mv.folders = null; } SP.busy = false; SP.data = r; }
  } catch (e) {
    SP.busy = false; SP.data = null;
    if (!/not measured/.test(e.message)) toast(e.message, true);
  }
  renderSpace();
}
// spaceTick runs with the state refresh: while a measurement runs, ask
// again; when it ends, show it.
async function spaceTick() {
  if (SP.dupes && SP.dupes.busy && SP.view === "dupes") { spaceDupesLoad(); }
  if (!SP.busy) return;
  try { SP.places = await api("/api/space/places"); } catch {}
  const place = (SP.places || []).find((p) => p.path === SP.root);
  if (place && !place.busy) { SP.busy = false; spaceLoad(); }
  else { const el = $("#sp-prog"); if (el && place) el.textContent = `${bytes(place.measured)} so far · see Activity for the folder it's reading`; }
}
// Whether shift was down for the click that is about to tick a box.
document.addEventListener("pointerdown", (e) => { SP.shift = e.shiftKey; }, true);
function spHover(rel) {
  SP.hover = rel;
  for (const el of document.querySelectorAll("[data-sp-row]")) el.classList.toggle("hot", el.dataset.spRow === rel);
  for (const el of document.querySelectorAll("[data-sp-seg]")) el.classList.toggle("hot", rel !== null && (el.dataset.spSeg === rel || el.dataset.spSeg.startsWith(rel + "/")));
  const tip = $("#sp-tip");
  if (!tip) return;
  const seg = rel !== null && document.querySelector(`[data-sp-seg="${CSS.escape(rel)}"]`);
  if (!seg) { tip.hidden = true; return; }
  tip.hidden = false;
  tip.innerHTML = `<b>${esc(seg.dataset.spName)}</b><span>${seg.dataset.spSize} · ${seg.dataset.spPct}%</span>`;
}
// spColor gives each top-level slice a hue of its own, lighter the
// deeper the ring, the way DaisyDisk does.
function spColor(i, depth) {
  const h = (i * 137.508) % 360;
  return `hsl(${h.toFixed(0)} ${Math.max(35, 72 - depth * 8)}% ${Math.min(74, 50 + depth * 7)}%)`;
}
function sunburst(node) {
  const R0 = 64, RING = 34, RINGS = 4, size = 2 * (R0 + RING * RINGS + 6), cx = size / 2, cy = size / 2;
  const total = node.bytes || 1;
  const segs = [];
  const arc = (r0, r1, a0, a1) => {
    const p = (r, a) => [cx + r * Math.cos(a), cy + r * Math.sin(a)];
    const big = a1 - a0 > Math.PI ? 1 : 0;
    const [x0, y0] = p(r1, a0), [x1, y1] = p(r1, a1), [x2, y2] = p(r0, a1), [x3, y3] = p(r0, a0);
    return `M${x0.toFixed(1)} ${y0.toFixed(1)}A${r1} ${r1} 0 ${big} 1 ${x1.toFixed(1)} ${y1.toFixed(1)}L${x2.toFixed(1)} ${y2.toFixed(1)}A${r0} ${r0} 0 ${big} 0 ${x3.toFixed(1)} ${y3.toFixed(1)}Z`;
  };
  const walk = (n, depth, a0, a1, rel, top) => {
    if (depth >= RINGS) return;
    let a = a0;
    (n.children || []).forEach((c, i) => {
      const span = (c.bytes / total) * 2 * Math.PI * ((a1 - a0) / (2 * Math.PI)) / ((n.bytes || 1) / total);
      const b = a + span;
      if (span > 0.004) {
        const t = depth === 0 ? i : top, crel = rel ? rel + "/" + c.name : c.name;
        const pct = ((c.bytes / total) * 100);
        segs.push(`<path class="sp-seg ${c.rest ? "rest" : ""}" d="${arc(R0 + depth * RING, R0 + (depth + 1) * RING - 1.5, a - Math.PI / 2, b - Math.PI / 2)}" fill="${c.rest ? "var(--line-2)" : spColor(t, depth)}" data-sp-seg="${esc(crel)}" data-sp-name="${esc(c.rest ? c.rest.toLocaleString() + " smaller items" : c.name)}" data-sp-size="${bytes(c.bytes)}" data-sp-pct="${pct < 1 ? pct.toFixed(1) : pct.toFixed(0)}" data-sp-dir="${c.dir && !c.rest ? 1 : 0}"/>`);
        if (c.dir) walk(c, depth + 1, a, b, crel, t);
      }
      a = b;
    });
  };
  walk(node, 0, 0, 2 * Math.PI, SP.at, 0);
  return `<svg viewBox="0 0 ${size} ${size}" class="sp-svg">${segs.join("")}<circle class="sp-center" cx="${cx}" cy="${cy}" r="${R0 - 4}" data-sp-up="1"/><text class="sp-ct" x="${cx}" y="${cy - 4}" text-anchor="middle">${esc(bytes(node.bytes))}</text><text class="sp-cs" x="${cx}" y="${cy + 13}" text-anchor="middle">${SP.at ? "click to go up" : esc((node.name || "").split("/").pop() || "disk")}</text></svg><div class="sp-tip" id="sp-tip" hidden></div>`;
}
function wireSunburst(d) {
  const chart = $("#sp-chart");
  if (!chart) return;
  chart.onmousemove = (e) => {
    const seg = e.target.closest("[data-sp-seg]");
    const rel = seg ? seg.dataset.spSeg : null;
    if (rel !== SP.hover) spHover(rel);
    const tip = $("#sp-tip");
    if (tip && !tip.hidden) { const r = chart.getBoundingClientRect(); tip.style.left = (e.clientX - r.left + 14) + "px"; tip.style.top = (e.clientY - r.top + 14) + "px"; }
  };
  chart.onmouseleave = () => spHover(null);
  chart.onclick = (e) => {
    const seg = e.target.closest("[data-sp-seg]");
    if (seg) { if (seg.dataset.spDir === "1") { SP.at = seg.dataset.spSeg; spaceLoad(); } return; }
    if (e.target.closest("[data-sp-up]") && SP.at) { SP.at = SP.at.split("/").slice(0, -1).join("/"); spaceLoad(); }
  };
}
async function spaceTrash(d) {
  const items = [...SP.sel];
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  openSheet(`<div class="sheet-head"><h2>Before anything goes</h2><div class="actions"><button class="icon-btn" id="st-close">${ICON.x}</button></div></div><div class="sheet-body">${loadingGlyph("Looking at each one…")}</div>`);
  $("#st-close").onclick = close;
  let checks;
  try { checks = await api("/api/space/check", { method: "POST", body: { path: SP.root, items } }); }
  catch (e) { toast(e.message, true); close(); return; }
  const on = new Set(checks.filter((c) => c.level !== "critical").map((c) => c.rel));
  const draw = () => {
    const chosen = checks.filter((c) => on.has(c.rel));
    const total = chosen.reduce((t, c) => t + c.bytes, 0);
    const crit = checks.filter((c) => c.level === "critical").length;
    openSheet(`<div class="sheet-head"><h2>Before anything goes</h2><div class="actions"><button class="icon-btn" id="st-close">${ICON.x}</button></div></div>
      <div class="sheet-body">
        <p class="hint" style="margin:0 0 10px">Lumpy looked at each item: where it sits, when it last changed, whether an app has it open, whether it belongs to a project, and whether the library holds a copy. ${crit ? `<b>${crit} critical item${crit === 1 ? " is" : "s are"} unticked</b>; tick ${crit === 1 ? "it" : "them"} only if you're sure.` : "Nothing here looks critical."}</p>
        <div class="checks vd-list">${checks.map((c) => `<label class="vd-row ${c.level}"><input type="checkbox" data-vd="${esc(c.rel)}" ${on.has(c.rel) ? "checked" : ""}><span class="vd-main"><span class="vd-name">${esc(c.rel.split("/").pop())}<small class="hint mono">${esc(c.rel.includes("/") ? c.rel.slice(0, c.rel.lastIndexOf("/")) : "")}</small></span><span class="vd-why">${c.reasons.map(esc).join(" · ")}</span></span><span class="vd ${c.level}">${VD_LABEL[c.level] || c.level}</span><span class="sp-size">${bytes(c.bytes)}</span></label>`).join("")}</div>
        <p class="hint" style="margin:10px 0 0">${spaceOnly ? `${bytes(total)} is deleted for good: there is no Trash to put it back from on the iPad.` : `${bytes(total)} goes to the Trash through the Finder, so it can be put back. The space is freed when you empty the Trash.`}</p>
      </div>
      <div class="sheet-foot"><button class="btn" id="st-cancel">Keep everything</button><button class="btn danger-fill" id="st-ok" ${chosen.length ? "" : "disabled"}>${ICON.trash}${spaceOnly ? `Delete ${chosen.length}` : `Move ${chosen.length} to the Trash`}</button></div>`);
    $("#st-close").onclick = $("#st-cancel").onclick = close;
    for (const el of document.querySelectorAll("[data-vd]")) el.onchange = () => { if (el.checked) on.add(el.dataset.vd); else on.delete(el.dataset.vd); draw(); };
    $("#st-ok").onclick = async () => {
      $("#st-ok").disabled = true;
      try {
        const r = await api("/api/space/trash", { method: "POST", body: { path: SP.root, items: [...on] } });
        close(); SP.sel.clear();
        toast(`${r.trashed.length} moved to the Trash · ${bytes(r.freed)} once it's emptied`);
        if (SP.dupes && SP.dupes.ready) spaceDupesLoad();
        if (SP.old) spaceOldLoad();
        // The folder in view may have nothing left in it: step up.
        const here = SP.at ? SP.at + "/" : "";
        const left = (d.node.children || []).filter((c) => !c.rest && !r.trashed.includes(here + c.name)).length;
        if (SP.at && !left) SP.at = SP.at.split("/").slice(0, -1).join("/");
        spaceLoad();
      } catch (e) { toast(e.message, true); $("#st-ok").disabled = false; }
    };
  };
  draw();
}

// ---------- events ----------
document.querySelector(".tabs").onclick = (e) => {
  const b = e.target.closest("button[data-tab]");
  if (!b) return;
  if (b.dataset.tab === "projects" && S.tab === "projects") S.project = null;
  // Coming from an open project, Clips shows that project.
  if (b.dataset.tab === "media" && S.project && S.media.project !== String(S.project)) { S.media.project = String(S.project); S.mediaData = null; }
  S.tab = b.dataset.tab;
  if (S.tab !== "projects") S.project = null;
  localSet("tab", S.tab);
  render();
};
$("#select-mode").onclick = () => { S.selecting = !S.selecting; if (!S.selecting) S.selected.clear(); render(); };
$("#view-seg").onclick = (e) => { const b = e.target.closest("button"); if (!b) return; S.view = b.dataset.view; localSet("view", S.view); render(); };
$("#sort").onchange = (e) => { S.sort = e.target.value; localSet("sort", S.sort); render(); };
$("#color-by").onchange = (e) => { S.colorBy = e.target.value; localSet("colorBy", S.colorBy); render(); };
$("#sort-dir").onclick = () => { S.desc = !S.desc; localSet("desc", S.desc ? "1" : "0"); render(); };
$("#refresh").innerHTML = ICON.refresh;
paintChrome();
$("#activity-btn").onclick = () => toggleActivity();
$("#pop-close").onclick = () => toggleActivity(false);
document.addEventListener("click", (e) => { if (!$("#activity-pop").hidden && !e.target.closest("#activity-pop, #activity-btn")) toggleActivity(false); });
$("#settings").onclick = () => openSettings();
$("#update-pill").onclick = () => openSettings("updates");
$("#refresh").onclick = async () => {
  loadState(); loadJobs();
  try {
    const r = await api("/api/drives/rescan", { method: "POST" });
    toast(r.queued ? `Rescanning ${r.queued} drive${r.queued === 1 ? "" : "s"}, one at a time; watch Activity` : "Nothing plugged in to rescan");
  } catch (e) { toast(e.message, true); }
};
$(".search-icon").innerHTML = ICON.search;
$("#export").innerHTML = ICON.export + "Export CSV…";
async function downloadCSV(url, name, body, type) {
  try {
    const res = await fetch(url, { method: body ? "POST" : "GET", headers: { "X-Lumpy-Token": token, ...(body ? { "Content-Type": "application/json" } : {}) }, body: body ? JSON.stringify(body) : undefined });
    if (!res.ok) throw new Error(res.statusText);
    const text = await res.text();
    if (native) { const saved = await nativeCall("saveFile", { name, content: text, type: type || "text/csv" }); if (saved) toast("Saved " + saved); return; }
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: type || "text/csv" }));
    a.download = name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  } catch (e) { toast(e.message, true); }
}
$("#export").onclick = () => downloadCSV("/api/export.csv", `lumpy-drives-${new Date().toISOString().slice(0, 10)}.csv`);
let searchTimer;
$("#search").oninput = (e) => {
  S.query = e.target.value;
  if (S.tab !== "logged" && S.tab !== "connected") { S.tab = "logged"; S.project = null; }
  render();
  clearTimeout(searchTimer);
  searchTimer = setTimeout(async () => {
    const q = S.query.trim();
    if (!q) { S.fileHits = null; render(); return; }
    try { const r = await api("/api/search?q=" + encodeURIComponent(q)); if (S.query.trim() === q) { S.fileHits = r.files; render(); } } catch {}
  }, 220);
};
$("#main").onclick = async (e) => {
  const fold = e.target.closest("[data-fold]");
  if (fold) { const k = fold.dataset.fold; if (S.folded.has(k)) S.folded.delete(k); else S.folded.add(k); localSet("folded", JSON.stringify([...S.folded])); render(); return; }
  const logBtn = e.target.closest("[data-log]");
  if (logBtn) {
    try { await api("/api/catalog", { method: "POST", body: { id: logBtn.dataset.log } }); toast("Logging the drive… its card fills in when the listing is done."); await loadJobs(); toggleActivity(true); } catch (err) { toast(err.message, true); }
    return;
  }
  const openP = e.target.closest("[data-open-project]");
  if (openP) { e.stopPropagation(); S.tab = "projects"; S.project = +openP.dataset.openProject; localSet("tab", S.tab); render(); return; }
  if (e.target.id === "sel-add") return addToProjectDialog([...S.selected].map((id) => ({ driveId: id, folder: "" })));
  if (e.target.id === "sel-new") return newProjectDialog([...S.selected].map((id) => ({ driveId: id, folder: "" })));
  if (e.target.id === "sel-all") { visibleDrives().filter(matchesQuery).forEach((d) => S.selected.add(d.id)); return render(); }
  if (e.target.id === "sel-none") { S.selected.clear(); return render(); }
  const fileRow = e.target.closest("[data-file]");
  if (fileRow) return openFile(fileRow.dataset.fileDrive, fileRow.dataset.file);
  const pick = e.target.closest("[data-pick]");
  if (pick) { pick.checked ? S.selected.add(pick.dataset.pick) : S.selected.delete(pick.dataset.pick); return render(); }
  const card = e.target.closest("[data-drive]");
  if (!card) return;
  if (S.selecting) { S.selected.has(card.dataset.drive) ? S.selected.delete(card.dataset.drive) : S.selected.add(card.dataset.drive); return render(); }
  const act = e.target.closest("[data-act]");
  if (act && act.dataset.act === "eject") return ejectDrive(card.dataset.drive);
  openDetail(card.dataset.drive, false, act && act.dataset.act === "edit");
};

// ejectDrive unmounts a plugged-in drive so it can be unplugged.
async function ejectDrive(id) {
  try {
    await api(`/api/drives/${encodeURIComponent(id)}/eject`, { method: "POST" });
    toast(`${driveTitle(id)} ejected. Safe to unplug.`);
    await loadState();
    if (S.detail === id) openDetail(id, true);
    if (S.tab === "map") renderMap();
  } catch (e) { toast(e.message, true); }
}
$("#scrim").onclick = () => { closeSheet(); };
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && !$("#activity-pop").hidden) return toggleActivity(false);
  if (e.key === "Escape" && !$("#sheet").hidden && S.file) return closeFile();
  if (e.key === "Escape" && !$("#sheet").hidden && S.detail) closeSheet();
  if ((e.metaKey || e.ctrlKey) && e.key === "f") { e.preventDefault(); $("#search").focus(); }
});

// The menu bar panel's Run again lands here: refill the offload form.
window.lumpyRunAgain = async (id) => {
  await loadJobs();
  const j = S.jobs.find((x) => x.id === id);
  if (!j || !j.params) return;
  S.offload = { source: j.params.source || "", dests: (j.params.dests || []).slice(), name: j.params.name || "", sourceCheck: j.params.sourceCheck !== false };
  S.tab = "offload"; S.project = null; localSet("tab", S.tab); render();
};

loadState();
loadJobs();
setInterval(() => { if (!document.hidden) loadState(); }, 3000);
setInterval(() => { if (!document.hidden) loadJobs(); }, 1000);
