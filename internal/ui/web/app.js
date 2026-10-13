"use strict";

// ---------- basics ----------
const token = new URLSearchParams(location.hash.slice(1)).get("t") || sessionStorageGet("lumpy-token") || "";
sessionStorageSet("lumpy-token", token);
const native = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.lumpy;
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
  history: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12a9 9 0 1 0 3-6.7M3 4v5h5M12 7v5l3 3"/></svg>',
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
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/></svg>',
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M12 5v14M5 12h14"/></svg>',
  copy: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/></svg>',
  activity: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h4l3-8 4 16 3-8h4"/></svg>',
  film: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4"/></svg>',
  back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M15 5l-7 7 7 7"/></svg>',
  arrow: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 12h16M13 5l7 7-7 7"/></svg>',
  play: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M7 4l13 8-13 8z"/></svg>',
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
  tab: localGet("tab", "logged") === "activity" ? "logged" : localGet("tab", "logged"),
  view: localGet("view", "grid"),
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
  offload: { source: "", dests: [""], name: "", sourceCheck: true },
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
    S.state = await api("/api/state");
    if (first || !S.settings) S.settings = await api("/api/settings").catch(() => null);
    // The offload form keeps what's being typed; only the counts refresh.
    if (!first && (S.tab === "offload" || S.tab === "media")) renderCounts();
    else if (!first && S.tab === "projects") { renderCounts(); if (S.project) loadProject(S.project, true); else loadProjects(); }
    else render();
  } catch (e) {
    if (!S.state) $("#main").innerHTML = `<div class="empty"><h2>Can't reach Lumpy</h2><p>${esc(e.message)}</p></div>`;
  }
}
let lastJobSig = "";
async function loadJobs() {
  try {
    S.jobs = await api("/api/jobs");
    const running = S.jobs.filter((j) => j.state === "running").length;
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


function backupBadge(d) {
  const b = d.backups;
  if (d.damaged) return `<span class="badge bad">${ICON.warn}${d.damaged} damaged</span>`;
  if (!b || !d.files) return "";
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
  raid: `<rect x="12" y="3" width="40" height="42" rx="5" fill="#34343d" stroke="#5c5c68"/><rect x="12" y="3" width="40" height="7" rx="3" fill="var(--pc, #8a5cf6)"/>
    <g fill="#1c1c22" stroke="#62626e" stroke-width=".8"><rect x="17" y="14" width="24" height="5.5" rx="1.2"/><rect x="17" y="21.5" width="24" height="5.5" rx="1.2"/><rect x="17" y="29" width="24" height="5.5" rx="1.2"/><rect x="17" y="36.5" width="24" height="5.5" rx="1.2"/></g>
    <g class="leds"><circle class="led" cx="46" cy="16.75" r="1.5"/><circle class="led" cx="46" cy="24.25" r="1.5"/><circle class="led" cx="46" cy="31.75" r="1.5"/><circle class="led" cx="46" cy="39.25" r="1.5"/></g>`,
};
function drivePic(kind, color, on) {
  return `<svg class="pic ${on ? "on" : ""}" viewBox="0 0 64 48" style="--pc:${color || "var(--muted)"}" aria-hidden="true">${PIC[kind] || PIC.hdd}</svg>`;
}
// capRing is how full a drive is, as a ring: pale until 70%, then
// yellow, then red in the last 10%.
function capRing(used, size) {
  const pct = size ? Math.max(0, Math.min(100, (used / size) * 100)) : 0;
  const r = 24, c = 2 * Math.PI * r;
  const col = pct >= 90 ? "var(--bad)" : pct >= 85 ? "var(--orange)" : pct >= 70 ? "var(--warn)" : "var(--ring)";
  return `<div class="ring" title="${pct.toFixed(0)}% full"><svg viewBox="0 0 60 60"><circle cx="30" cy="30" r="${r}" class="track"/><circle cx="30" cy="30" r="${r}" class="fill" style="stroke:${col};stroke-dasharray:${c.toFixed(2)};stroke-dashoffset:${(c * (1 - pct / 100)).toFixed(2)}"/></svg><span class="pct" style="color:${pct >= 70 ? col : "var(--text)"}">${Math.round(pct)}<small>%</small></span></div>`;
}
// loadingGlyph is a hard drive being read, for every "loading" state.
function loadingGlyph(text, small) {
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
    <div class="badges">${backupBadge(d)}${checkBadge(d)}${projectBadges(d)}</div>
    <div class="card-foot"><span><i class="dot ${d.connected ? "live" : ""}" style="background:${dot};color:${dot}"></i>${d.connected ? "Plugged in now" : "Last seen • " + when(d.lastSeen)}</span>
      <span class="card-acts">${S.selecting ? `<input type="checkbox" class="pick" data-pick="${esc(d.id)}" ${S.selected.has(d.id) ? "checked" : ""}>` : `<button class="icon-btn card-edit" data-act="edit" title="Edit labels">${ICON.edit}</button><button class="btn small" data-act="open">Details</button>`}</span></div>
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
  renderCounts();
  renderToolbar();
  const main = $("#main");
  const keepScroll = main.scrollTop;
  if (S.tab === "offload") return renderOffload();
  if (S.tab === "projects") return renderProjects();
  if (S.tab === "media") return renderMedia();
  main.innerHTML = (S.tab === "connected" ? renderConnected() : renderLogged()) + selectionBar();
  main.scrollTop = keepScroll;
}

function drivesHTML(ds) {
  if (S.view === "list") {
    return `<table class="list"><thead><tr><th>Name</th><th>Client · Project</th><th class="num">Capacity</th><th>Used</th><th class="num">Free</th><th class="num">Files</th><th>Backups</th><th>Last check</th><th>Last seen</th></tr></thead><tbody>
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
      <div class="card-foot"><span><i class="dot live" style="background:var(--ok);color:var(--ok)"></i>${never ? "Set to never log" : "Being logged…"}</span>
      <button class="btn small primary" data-log="${esc(v.id)}">Log now</button></div></article>`).join("");
    html += "</div>";
  }
  return html;
}

// ---------- details ----------
function openSheet(html, small) {
  const sh = $("#sheet");
  sh.className = "sheet" + (small ? " small" : "");
  sh.innerHTML = html;
  sh.hidden = false;
  $("#scrim").hidden = false;
}
function closeSheet() {
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
      <div><h2>${esc(d.title)}</h2><div class="sub">${esc(d.name)}${d.connected ? " · plugged in" : ""}</div></div>
      <div class="actions">
        <label class="switch" title="Project plans won't copy backups onto this drive: camera cards that get wiped, loaners, the edit SSD">No backups here <input type="checkbox" id="no-backups" ${l.noBackups ? "checked" : ""}></label>
        <label class="switch" title="Lumpy will not log or check this drive when it's plugged in">Never log <input type="checkbox" id="never-log" ${l.neverLog ? "checked" : ""}></label>
        <button class="btn danger" id="forget">Remove</button>
        <button class="icon-btn" id="close" title="Close">${ICON.x}</button>
      </div>
    </div>
    <div class="sheet-body">
      <div class="sec"><div class="sec-title">${ICON.info}Identity</div>
        <div class="box"><dl class="kv">
          <dt>Name</dt><dd>${esc(d.name)}</dd>
          <dt>Path</dt><dd>${esc(d.mount || d.lastMount)} ${d.connected && native ? `<button class="btn small" id="reveal">${ICON.reveal}Show in Finder</button>` : ""}</dd>
          <dt>Volume UUID</dt><dd class="mono">${esc(d.id)}</dd>
          <dt>Format</dt><dd>${esc(d.fstype || "—")}</dd>
          <dt>First seen</dt><dd>${when(d.firstSeen)}</dd>
          <dt>Last seen</dt><dd>${d.connected ? "Plugged in now" : when(d.lastSeen)}</dd>
        </dl></div></div>

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

      <div class="sec"><div class="sec-title">${ICON.shield}Backups &amp; checks
          <div class="right">
            ${d.verifyRequested ? `<button class="btn small" id="cancel-check">Cancel queued check</button>` : ""}
            ${d.connected ? `<button class="btn small" id="hash" ${d.busy ? "disabled" : ""}>Checksum new files</button>` : ""}
            <button class="btn small primary" id="verify" ${d.busy || (d.verifyRequested && !d.connected) ? "disabled" : ""}>${d.connected ? "Check drive now" : d.verifyRequested ? "Check queued" : "Queue check"}</button>
          </div></div>
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
  container.innerHTML = entries.map((e) => { const [dot, bk] = entryDot(e); return `<div class="tree-row ${e.folder ? "folder" : ""}" data-path="${esc(e.path)}" style="padding-left:${10 + depth * 18}px">
      <div class="nm">${e.folder ? ICON.chev : '<span style="width:12px"></span>'}${dot}${e.folder ? ICON.folder : ICON.file}<span title="${esc(e.path)}">${esc(e.name)}</span></div>
      ${bk}<span class="sz">${bytes(e.bytes)}${e.folder ? ` · ${e.files.toLocaleString()}` : ""}</span><span class="st">${e.folder ? `<button class="icon-btn mini" data-addsrc="${esc(e.path)}" title="Add this folder to a project">${ICON.plus}</button>` : statusIcon(e)}</span></div>
      ${e.folder ? `<div class="kids" hidden></div>` : ""}`; }).join("") + (entries.length >= 2000 ? `<div class="tree-note">Showing the first 2,000.</div>` : "");
  for (const b of container.querySelectorAll(":scope > .tree-row.folder [data-addsrc]")) b.onclick = (e) => {
    e.stopPropagation();
    addToProjectDialog([{ driveId: id, folder: b.dataset.addsrc }], () => openDetail(id, false));
  };
  for (const row of container.querySelectorAll(":scope > .tree-row:not(.folder)")) row.onclick = () => openFile(id, row.dataset.path, id);
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
  if (S.tab === "projects" && !S.project) renderProjects();
}

function renderProjects() {
  if (S.project) return renderProjectPage();
  const main = $("#main");
  if (!S.projects) { main.innerHTML = loadingGlyph("Reading the library…"); loadProjects(); return; }
  const ps = S.projects;
  if (!ps.length) {
    main.innerHTML = `<div class="empty">${ICON.film}<h2>No projects yet</h2>
      <p>A project is the folders that hold one piece of work: a feature across five drives, or one music video that shares a shuttle with three other jobs. Lumpy shows how well it's backed up, wherever the copies are, and plans the copies that get every file onto two drives.</p>
      <p><button class="btn primary" id="new-project">${ICON.plus}New project</button></p></div>`;
  } else {
    main.innerHTML = `<div class="section-head"><h2>Projects</h2><span>${ps.length}</span><div class="right"><button class="btn primary" id="new-project">${ICON.plus}New project</button></div></div>
      <div class="grid">${ps.map((p) => {
        const h = p.health;
        const bad = h.soleBytes + h.shortBytes;
        return `<article class="card pcard" style="--tint:${COLORS[p.color] || "transparent"}" data-open-project="${p.id}">
          <div class="card-top"><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span><span class="drive-ico">${ICON.film}</span></div>
          <div class="card-title"><h3>${esc(p.name)}</h3><span class="vol">${p.sources.length} folder${p.sources.length === 1 ? "" : "s"} · ${p.target} copies wanted</span></div>
          ${backupBar(h.backedBytes, h.bytes, h.unknownBytes, h.likelyBytes || 0, { compact: true })}
          <div class="cap">${bytes(h.bytes)}<span class="sep">•</span>${h.contents.toLocaleString()} files${bad ? `<span class="sep">•</span><span style="color:var(--warn)">${bytes(bad)} still to copy</span>` : ""}</div>
          <div class="badges">${h.damaged ? `<span class="badge bad">${h.damaged} damaged</span>` : ""}</div>
          <div class="card-foot"><span>${p.sources.map((src) => driveTitle(src.driveId) + (src.folder ? "/" + src.folder : "")).join(", ") || "No folders yet"}</span><button class="btn small">Open</button></div></article>`;
      }).join("")}</div>`;
  }
  $("#new-project").onclick = () => newProjectDialog([]);
}

async function loadProject(id, refresh) {
  try { [S.projectData, S.projectProxies] = await Promise.all([api(`/api/projects/${id}`), api(`/api/projects/${id}/proxies`).catch(() => [])]); }
  catch (e) { toast(e.message, true); S.project = null; S.projectData = null; return render(); }
  if (S.project === id) renderProjectPage();
}

function renderProjectPage() {
  const main = $("#main");
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
    <div class="phead">
      <button class="icon-btn" id="proj-back" title="All projects">${ICON.back}</button>
      <span class="dot" style="width:12px;height:12px;background:${COLORS[p.color] || "var(--accent)"}"></span>
      <input class="inline-input" id="proj-name" value="${esc(p.name)}" title="Rename">
      <label class="hint">Copies wanted <select class="plain" id="proj-target">${[1, 2, 3, 4].map((n) => `<option value="${n}" ${p.target === n ? "selected" : ""}>${n}</option>`).join("")}</select></label>
      <div class="swatches">${Object.entries(COLORS).map(([k, c]) => `<button class="swatch ${p.color === k ? "on" : ""}" data-pcolor="${k}" style="background:${c}" title="${k}"></button>`).join("")}</div>
      <div class="right"><button class="btn" id="proj-media" title="Every clip in this project with its codec, frame rate, timecode and log notes">${ICON.clapper}Clips</button><button class="btn" id="proj-media-csv" title="Clip list with metadata, for post">${ICON.export}Clips CSV…</button><button class="btn danger" id="proj-delete">Delete project</button></div>
    </div>

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
        <p class="hint">Each step copies only the files that need another copy, from one drive to another. Plug both in and run it. Copies go into a folder named after the project on the destination${p.sources.length > 1 ? ", with a folder per source inside it" : ""}, keeping their paths. ${bytes(data.reserve)} is kept free on every drive.</p>
        ${data.transfers.length === 0 && !data.unplacedBytes ? `<div class="alert ok">${h.bytes ? `<b>Nothing to do.</b> Every checksummed file is on ${p.target} drives.` : "<b>Nothing to plan yet.</b>"}</div>` : ""}
        ${data.transfers.map((t) => {
          const ready = t.ready && !t.running && !t.busy;
          const need = [t.sourceId, t.destId].filter((id) => !connected(id)).map(driveTitle);
          return `<div class="step-card" data-transfer="${esc(t.sourceId)}|${esc(t.destId)}">
            <div class="step-head"><span class="dot ${connected(t.sourceId) ? "live" : ""}" style="background:${connected(t.sourceId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.sourceId))}</b><span class="arrow">${ICON.arrow}</span>
              <span class="dot ${connected(t.destId) ? "live" : ""}" style="background:${connected(t.destId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.destId))}</b>
              <span class="hint">${bytes(t.bytes)} · ${t.fileCount.toLocaleString()} file${t.fileCount === 1 ? "" : "s"}</span>
              <div class="right">${t.running ? `<span class="badge info">${ICON.clock}Running</span>` : t.busy ? `<span class="badge">${ICON.clock}Drive busy</span>` : ready ? `<button class="btn go small" data-run="${esc(t.sourceId)}|${esc(t.destId)}">${ICON.play}Run this step</button>` : `<span class="badge">${ICON.plug}Plug in ${esc(need.join(" and "))}</span>`}</div></div>
            <div class="chips">${t.folderList.slice(0, 8).map((f) => `<span class="chip">${esc(f.name === "." ? "(top level)" : f.name)}<small>${bytes(f.bytes)}</small></span>`).join("")}${t.folderList.length > 8 ? `<span class="chip">+${t.folderList.length - 8} more</span>` : ""}</div>
            ${ready ? `<div class="step-run"><span class="hint">Copy into folder</span><input class="path-input" data-folder="${esc(t.sourceId)}|${esc(t.destId)}" value="${esc(p.name)}" spellcheck="false"><span class="hint">on ${esc(driveTitle(t.destId))}</span></div>` : ""}
          </div>`;
        }).join("")}
        ${data.unplacedBytes ? `<div class="alert bad" style="margin-top:10px"><b>${bytes(data.unplacedBytes)} has nowhere to go.</b> No ${anyDest ? "drive in the library" : "chosen destination"} has room for these files (each needs a drive that doesn't already hold it). ${anyDest ? "Log a drive" : "Allow a drive"} with at least ${bytes(data.unplacedBytes + data.reserve)} free, or free up space.
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
          return `<div class="step-card" style="margin-bottom:10px">
            <div class="step-head"><span class="dot ${d && d.connected ? "live" : ""}" style="background:${d && d.connected ? "var(--ok)" : (d && COLORS[d.labels.color]) || "var(--muted)"}"></span><b style="min-width:0;overflow:hidden;text-overflow:ellipsis">${sourceLabel(sh)}</b>
              <div class="right"><button class="btn small" data-drive-open="${esc(sh.driveId)}">Drive</button><button class="icon-btn" data-remove-src="${i}" title="Remove from project">${ICON.x}</button></div></div>
            ${backupBar(sh.backedBytes, sh.bytes, sh.unknownBytes, sh.likelyBytes || 0, { compact: true })}
            <div class="cap">${bytes(sh.bytes)} · ${sh.files.toLocaleString()} files${sh.soleBytes + sh.shortBytes ? `<span class="sep">•</span><span style="color:var(--warn)">${bytes(sh.soleBytes + sh.shortBytes)} still to copy</span>` : ""}${sh.extraBytes ? `<span class="sep">•</span><span class="hint">${bytes(sh.extraBytes)} on more drives than needed</span>` : ""}${d && !d.connected ? `<span class="sep">•</span><span class="hint">last seen ${new Date(d.lastSeen * 1000).toLocaleDateString()}</span>` : ""}</div></div>`;
        }).join("")}
        <button class="btn" id="proj-add-src">${ICON.plus}Add a folder or drive…</button>
      </section>

      <section class="panel">
        <h2>Back up onto</h2>
        <p class="hint">Where the plan may put copies. Lumpy never copies a file onto a drive that already holds it.</p>
        <label class="check-row"><input type="radio" name="dest-mode" value="any" ${anyDest ? "checked" : ""}><span>Any drive in the library with room</span></label>
        <label class="check-row"><input type="radio" name="dest-mode" value="only" ${anyDest ? "" : "checked"}><span>Only these drives</span></label>
        <div class="checks" id="dest-list" style="margin-top:8px;${anyDest ? "opacity:.55" : ""}">${(S.state.drives || []).map((d) => `<label><input type="checkbox" data-dest="${esc(d.id)}" ${dest.has(d.id) ? "checked" : ""} ${anyDest ? "disabled" : ""}><span class="dot" style="background:${d.connected ? "var(--ok)" : COLORS[d.labels.color] || "var(--muted)"}"></span>${esc(d.title)}<span class="hint">${bytes(d.free)} free${d.connected ? " · plugged in" : ""}${d.labels.noBackups ? " · marked “no backups here”" : ""}</span></label>`).join("")}</div>
        <p class="hint" style="margin:8px 0 0">Drives marked “no backups here” in their details (camera cards, loaners) are skipped unless ticked above.</p>
      </section>
      <section class="panel"><h2>Notes</h2><textarea class="path-input" id="proj-notes" rows="4" placeholder="Where the drives live, who has what, what's still to come…">${esc(p.notes)}</textarea></section>
    </div>
    </div>`;
  main.scrollTop = keepScroll;
  wireProjectPage(p);
}

function wireProjectPage(p) {
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
    el.disabled = true;
    try {
      await api(`/api/projects/${p.id}/transfer`, { method: "POST", body: { sourceId, destId, folder } });
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
    <p class="hint">When the cut is locked, hand Lumpy the timeline export and it says which originals the edit uses and which it doesn't. FCPXML (Final Cut Pro, Resolve), Premiere Pro or FCP 7 XML, an EDL, or a plain list of clip names; several at once is fine (the cut, the trailer, the socials).</p>
    <div class="step-run" style="margin-top:0;flex-wrap:wrap"><button class="btn ${a ? "" : "primary"}" id="arc-pick">${ICON.film}Choose timeline export…</button>${a ? a.files.map((f) => `<span class="chip">${esc(f)}</span>`).join("") : ""}</div>
    <input type="file" id="arc-file" multiple hidden accept=".fcpxml,.xml,.edl,.txt,.csv">
    ${a ? archiveResult(p, a) : ""}</section>`;
}
function archiveResult(p, a) {
  const n = (v) => v.toLocaleString();
  const unplugged = a.drives.filter((d) => d.unusedFiles && !d.connected).map((d) => driveTitle(d.id));
  const rows = a.folders.map((f) => { const d = driveOf(f.driveId); return `<tr><td><div class="name" style="font-weight:500"><span class="dot" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(driveTitle(f.driveId))} <span class="hint">›</span> <span class="mono">${esc(f.source ? f.source + "/" : "")}${f.folder === "." ? "" : esc(f.folder)}</span></div></td><td class="num">${n(f.files)}</td><td class="num" style="color:${f.unusedFiles ? "var(--warn)" : "var(--muted)"};font-weight:${f.unusedFiles ? 600 : 400}">${n(f.unusedFiles)}</td><td class="num">${f.unusedBytes ? bytes(f.unusedBytes) : "—"}</td></tr>`; }).join("");
  return `
    <div class="stat-row" style="margin-top:14px">
      <div class="stat ok"><div class="v">${n(a.usedFiles)}</div><div class="l">clips the edit uses (${bytes(a.usedBytes)})</div></div>
      <div class="stat ${a.unusedFiles ? "warn" : "dim"}"><div class="v">${n(a.unusedFiles)}</div><div class="l">not used (${bytes(a.unusedBytes)})</div></div>
      <div class="stat ${a.notFoundTotal ? "bad" : "dim"}"><div class="v">${n(a.notFoundTotal)}</div><div class="l">in the edit, not in this project (of ${n(a.refs)} referenced)</div></div>
    </div>
    ${a.notFoundTotal ? `<details class="notes" style="margin-top:10px"><summary>${n(a.notFoundTotal)} referenced clip${a.notFoundTotal === 1 ? "" : "s"} not in this project${a.notFound.length < a.notFoundTotal ? ` (first ${a.notFound.length})` : ""}</summary><div class="chips" style="margin-top:6px">${a.notFound.map((x) => `<span class="chip">${esc(x)}</span>`).join("")}</div><p class="hint" style="margin:6px 0 0">Stock, graphics, music, or footage in a folder outside this project. Nothing is done with these.</p></details>` : ""}
    ${a.folders.length ? `<table class="list compact" style="margin-top:12px"><thead><tr><th>Drive · folder</th><th class="num">Clips</th><th class="num">Unused</th><th class="num">Unused size</th></tr></thead><tbody>${rows}</tbody></table>` : ""}
    ${a.unusedFiles ? `<details class="notes" style="margin-top:10px"><summary>${n(a.unusedFiles)} unused clip${a.unusedFiles === 1 ? "" : "s"}${a.unused.length < a.unusedFiles ? ` (first ${a.unused.length})` : ""}</summary><div class="chips" style="margin-top:6px">${a.unused.map((c) => `<span class="chip" title="${esc(driveTitle(c.driveId) + ": " + c.path)}">${esc(c.name)}<small>${bytes(c.bytes)}${c.files > 1 ? ` · ${c.files} parts` : ""}</small></span>`).join("")}</div></details>
      <div class="step-run" style="margin-top:12px;flex-wrap:wrap"><span class="hint">Move the unused clips into a folder named</span><input class="path-input" id="arc-folder" value="_Unused" spellcheck="false" style="max-width:180px"><span class="hint">inside each project folder, keeping their paths, or to the Trash.</span></div>
      <div class="confirm-actions" style="margin-top:10px;justify-content:flex-start">${unplugged.length ? `<span class="badge">${ICON.plug}Plug in ${esc(unplugged.join(" and "))}</span>` : `<button class="btn primary" id="arc-move">${ICON.folder}Move ${n(a.unusedFiles)} unused into the folder</button><button class="btn danger" id="arc-trash">${ICON.trash}Move ${n(a.unusedFiles)} unused to the Trash…</button>`}</div>
      <p class="hint" style="margin:8px 0 0">Matching is by clip name, so a name the edit uses anywhere keeps every clip with that name. Clips in the Trash stay there until you empty it. The library follows the files either way.</p>`
    : `<div class="alert ok" style="margin-top:12px"><b>Everything in this project is used by the edit.</b> Nothing to move.</div>`}`;
}
function wireArchive(p) {
  const run = async (body) => {
    const btn = $("#arc-pick");
    btn.disabled = true;
    try { const a = await api(`/api/projects/${p.id}/archive`, { method: "POST", body }); S.archive = { ...a, project: p.id }; renderProjectPage(); $("#archive-panel").scrollIntoView({ block: "nearest" }); }
    catch (e) { toast(e.message, true); btn.disabled = false; }
  };
  $("#arc-pick").onclick = async () => {
    if (native) { const paths = await nativeCall("pickFiles", {}); if (paths && paths.length) run({ paths }); }
    else $("#arc-file").click();
  };
  $("#arc-file").onchange = async (e) => {
    const files = [...e.target.files];
    if (!files.length) return;
    const uploads = await Promise.all(files.map((f) => f.text().then((text) => ({ name: f.name, text }))));
    run({ uploads });
  };
  const a = S.archive && S.archive.project === p.id ? S.archive : null;
  if (!a) return;
  const apply = async (action) => {
    try {
      await api(`/api/projects/${p.id}/archive/apply`, { method: "POST", body: { token: a.token, action, folder: ($("#arc-folder") || {}).value || "_Unused" } });
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
    box.innerHTML = `<div><b>Move ${a.unusedFiles.toLocaleString()} clips (${bytes(a.unusedBytes)}) to the Trash?</b><br>They stay in the Trash until you empty it, and can be put back from there. Lumpy's library forgets them now.</div><div class="confirm-actions"><button class="btn" id="arc-no">Cancel</button><button class="btn danger-fill" id="arc-yes">Move to Trash</button></div>`;
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
    <div class="step-head"><span class="dot ${d && d.connected ? "live" : ""}" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(x.driveId))}</b><span class="px-path" title="${esc(x.online)}">${esc(x.online)}</span>
      <div class="right">${done ? `<span class="badge ok">${ICON.check}All proxied</span>` : x.total ? `<button class="btn small" data-copy-missing="${i}" title="Copy the names of the originals without a proxy, to hand to whoever makes them">${ICON.copy}Copy list</button>` : ""}</div></div>
    <div class="bbar ${done ? "s-full" : "s-partial"}"><div class="track">${x.proxied ? `<i class="${done ? "fill-full" : "fill-prog"}" style="width:${pct.toFixed(2)}%"></i>` : ""}</div><span class="lbl">${done ? ICON.check : ""}${x.total ? `${x.proxied.toLocaleString()} of ${x.total.toLocaleString()} originals have a proxy` : "No video in the Online folder"}</span></div>
    <div class="cap">${missing ? `<span style="color:var(--warn)">${missing.toLocaleString()} still to make · ${bytes(x.missingBytes)}</span>` : ""}${x.extra ? `<span class="sep">•</span><span class="hint">${x.extra.toLocaleString()} prox${x.extra === 1 ? "y" : "ies"} with no original</span>` : ""}<span class="sep">•</span><span class="hint">Offline: <span class="mono">${esc(x.offline)}</span></span></div>
    ${missing ? `<details class="notes"><summary>${missing.toLocaleString()} without a proxy${x.truncated ? ` (first ${x.missing.length} shown)` : ""}</summary><div class="chips" style="margin-top:6px">${x.missing.map((c) => `<span class="chip" title="${esc(c.path)}">${esc(c.name)}<small>${bytes(c.bytes)}${c.files > 1 ? ` · ${c.files} parts` : ""}</small></span>`).join("")}</div></details>` : ""}
  </div>`;
}
function wireProxies(pairs) {
  for (const el of document.querySelectorAll("[data-copy-missing]")) el.onclick = async () => {
    const x = pairs[+el.dataset.copyMissing];
    const text = x.missing.map((c) => c.name).join("\n") + "\n";
    try { await navigator.clipboard.writeText(text); toast(`Copied ${x.missing.length} clip name${x.missing.length === 1 ? "" : "s"}`); }
    catch { const ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta); ta.select(); document.execCommand("copy"); ta.remove(); toast(`Copied ${x.missing.length} clip name${x.missing.length === 1 ? "" : "s"}`); }
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

// addSourceDialog lets the user pick a drive and, inside it, a folder
// from the library's index (or the whole drive), then calls done(source).
function addSourceDialog(done, startDrive) {
  const drives = S.state.drives || [];
  if (!drives.length) return toast("No drives logged yet", true);
  let driveId = startDrive || drives[0].id, dir = "";
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  const show = async () => {
    let entries = [];
    try { entries = await api(`/api/drives/${encodeURIComponent(driveId)}/list?dir=${encodeURIComponent(dir)}`); } catch (e) { toast(e.message, true); }
    const folders = entries.filter((e) => e.folder);
    const crumbs = dir ? dir.split("/") : [];
    openSheet(`<div class="sheet-head"><h2>Add to project</h2><div class="actions"><button class="icon-btn" id="as-close">${ICON.x}</button></div></div>
      <div class="sheet-body" style="padding-top:14px">
        <div class="field"><label>Drive</label><select class="plain" id="as-drive">${drives.map((d) => `<option value="${esc(d.id)}" ${d.id === driveId ? "selected" : ""}>${esc(d.title)}${d.connected ? " (plugged in)" : ""}</option>`).join("")}</select></div>
        <div class="picker-path" style="padding-top:12px"><button class="chip" data-crumb="">${esc(driveTitle(driveId))}</button>${crumbs.map((c, i) => ` <span class="hint">›</span> <button class="chip" data-crumb="${esc(crumbs.slice(0, i + 1).join("/"))}">${esc(c)}</button>`).join("")}</div>
        <div class="picker-list">${folders.length ? folders.map((f) => `<button data-go="${esc(f.path)}">${ICON.folder}${esc(f.name)}<span class="hint">${bytes(f.bytes)} · ${f.files.toLocaleString()} files</span></button>`).join("") : `<div class="tree-note">${dir ? "No folders inside this one." : "Nothing indexed on this drive yet."}</div>`}</div>
      </div>
      <div class="sheet-foot"><button class="btn" id="as-cancel">Cancel</button><button class="btn primary" id="as-ok">${dir ? `Add “${esc(crumbs[crumbs.length - 1])}”` : "Add the whole drive"}</button></div>`, true);
    $("#as-close").onclick = $("#as-cancel").onclick = close;
    $("#as-drive").onchange = (e) => { driveId = e.target.value; dir = ""; show(); };
    for (const b of document.querySelectorAll("[data-go]")) b.onclick = () => { dir = b.dataset.go; show(); };
    for (const b of document.querySelectorAll("[data-crumb]")) b.onclick = () => { dir = b.dataset.crumb; show(); };
    $("#as-ok").onclick = () => { close(); done({ driveId, folder: dir }); };
  };
  show();
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
  $("#main").innerHTML = `<div class="offload split ${busy ? "busy" : ""}">
    <section class="panel">
      <h2>New offload</h2>
      <p class="hint">Lumpy reads the source once, writes every destination at the same time, reads each copy back from its drive and compares xxh64 checksums. Each copy gets an ASC MHL manifest and is recorded in the library.</p>
      <div class="step"><span class="step-n">1</span><div><h3>Source card or folder</h3>
        <div class="path-row"><input class="path-input" id="src" value="${esc(o.source)}" placeholder="/Volumes/A001">${recentSel("sources")}<button class="btn" data-pick="src">Choose…</button></div>
        <div class="chips">${chips("src")}</div></div></div>
      <div class="step"><span class="step-n">2</span><div><h3>Destinations <span style="color:var(--muted);font-weight:400">(at least two recommended)</span></h3>
        ${o.dests.map((p, i) => `<div class="path-row"><input class="path-input" data-dest="${i}" value="${esc(p)}" placeholder="/Volumes/SHUTTLE_1/Day_01">${recentSel("dests", i)}<button class="btn" data-pick="dest" data-i="${i}">Choose…</button>${o.dests.length > 1 ? `<button class="icon-btn" data-rmdest="${i}" title="Remove">${ICON.x}</button>` : ""}</div>`).join("")}
        <div class="chips"><button class="chip" id="add-dest">${"+ Add destination"}</button>${chips("dest")}</div></div></div>
      <div class="step"><span class="step-n">3</span><div><h3>Folder name in each destination</h3>
        <div class="path-row"><input class="path-input" id="oname" value="${esc(o.name)}" placeholder="${esc(name || "Same as the source")}"></div>
        <label class="check-row"><input type="checkbox" id="source-check" ${o.sourceCheck ? "checked" : ""}><span>Read the card a second time while verifying<span class="note">Catches a faulty card or reader. Recommended; it runs alongside the copy checks.</span></span></label>
        <div class="go-row"><button class="btn primary" id="start-offload">${ICON.copy}Start offload</button>
          <span class="note" id="offload-note">${o.source && o.dests.some(Boolean) ? `Copies to ${o.dests.filter(Boolean).map((d) => esc(d.replace(/\/+$/, "") + "/" + (name || ""))).join(", ")}` : ""}</span></div></div></div>
    </section>
    <div class="jobs-col"><div class="section-head"><h2>Offloads</h2></div><div class="jobs" id="offload-jobs"></div></div>
  </div>`;
  wireOffload();
  renderJobsInto($("#offload-jobs"), S.jobs.filter((j) => j.kind === "offload"), "No offloads yet.");
}

function wireOffload() {
  const o = S.offload;
  const rerender = () => renderOffload();
  $("#src").onchange = (e) => { o.source = e.target.value.trim(); rerender(); };
  $("#oname").onchange = (e) => { o.name = e.target.value.trim(); rerender(); };
  $("#source-check").onchange = (e) => { o.sourceCheck = e.target.checked; };
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
      await api("/api/offload", { method: "POST", body: { source: o.source, dests, name: o.name, sourceCheck: o.sourceCheck } });
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

const STATE_BADGE = { running: ["info", "Running"], ok: ["ok", "Done"], cancelled: ["warn", "Cancelled"], error: ["bad", "Error"] };
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
  const running = j.state === "running";
  const p = j.progress;
  el.querySelector(".job-head h3").textContent = j.title;
  const [cls, label] = j.state === "failed" ? ["bad", `${j.failures} problem(s)`] : STATE_BADGE[j.state] || ["", j.state];
  const badge = el.querySelector(".job-head .badge");
  badge.className = "badge " + cls;
  badge.textContent = label;
  el.querySelector(".job-head .when").textContent = short(j.started) + (j.ended ? " · " + duration(j.ended - j.started || 1) : "");
  const actions = el.querySelector(".job-head .actions");
  const wantCancel = running ? "1" : j.params ? "again" : "";
  if (actions.dataset.cancel !== wantCancel) {
    actions.dataset.cancel = wantCancel;
    actions.innerHTML = running ? `<button class="btn small" data-cancel="${j.id}">${ICON.x}Cancel</button>`
      : j.params ? `<button class="btn small" title="Fill the offload form with this job's source and destinations">${ICON.refresh}Run again</button>` : "";
    const b = actions.querySelector("button");
    if (b && running) b.onclick = async () => { try { await api(`/api/jobs/${j.id}/cancel`, { method: "POST" }); } catch (e) { toast(e.message, true); } };
    else if (b) b.onclick = () => {
      // Same source, same destinations: a disconnected card or a
      // cancelled run is picked up where it stopped.
      S.offload = { source: j.params.source || "", dests: (j.params.dests || []).slice(), name: j.params.name || "", sourceCheck: j.params.sourceCheck !== false };
      toggleActivity(false);
      S.tab = "offload"; S.project = null; localSet("tab", S.tab); render();
      toast("Offload form filled in from that job. Check it and press Start offload.");
    };
  }

  const prog = el.querySelector(".job-progress");
  prog.hidden = !running;
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
  const results = running ? [] : j.lines.filter((l) => /^(OK|BAD) /.test(l));
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
      <label class="select"><select id="m-project">${opt("", "All projects", m.project)}${(st.projects || []).map((p) => opt(p.id, p.name, m.project)).join("")}</select></label>
      <label class="select"><select id="m-drive">${opt("", "All drives", m.drive)}${st.drives.map((x) => opt(x.id, x.title, m.drive)).join("")}</select></label>
      <label class="select"><select id="m-kind">${opt("", "Any kind", m.kind)}${Object.entries(KIND_LABEL).map(([k, v]) => opt(k, v, m.kind)).join("")}</select></label>
      <label class="select"><select id="m-format">${catalogue(d.formats || [], FORMATS, m.format, "Any format")}</select></label>
      <label class="select"><select id="m-codec">${catalogue(d.codecs || [], CODECS, m.codec, "Any codec")}</select></label>
      <label class="select"><select id="m-rate">${opt("", "Any frame rate", m.rate)}${(d.rates || []).map((r) => opt(r, fps(r) + " fps", m.rate)).join("")}</select></label>
      <label class="select"><select id="m-qc">${opt("", "Any QC", m.qc)}${opt("none", "Not checked", m.qc)}${opt("ok", "OK", m.qc)}${opt("flag", "Flagged", m.qc)}${opt("reject", "Rejected", m.qc)}</select></label>
      <div class="search"><span class="search-icon">${ICON.search}</span><input id="m-q" type="search" placeholder="Clip, scene, take, camera, reel, notes, tag…" value="${esc(m.q)}" autocomplete="off" spellcheck="false"></div>
      <span class="sortgrp"><label class="select" title="Sort"><select id="m-sort">${SORTS.map(([k, v]) => opt(k, "Sort: " + v, S.mediaSort)).join("")}</select></label>
      <button class="icon-btn" id="m-dir" title="Reverse order">${S.mediaDesc ? ICON.down : ICON.up}</button></span>
      <button class="btn" id="m-export" title="This list with every column, for post">${ICON.export}Export CSV…</button>
    </div>
    <div id="media-body"></div></div>`;
  const reload = () => { for (const k of ["project", "drive", "kind", "format", "codec", "rate", "qc"]) m[k] = $("#m-" + k).value; loadMedia(); };
  for (const k of ["project", "drive", "kind", "format", "codec", "rate", "qc"]) $("#m-" + k).onchange = reload;
  $("#m-sort").onchange = (e) => { S.mediaSort = e.target.value; localSet("mediaSort", S.mediaSort); renderMediaTable(); };
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
  body.innerHTML = `<div class="pills media-sum">
      <span class="pill">Clips:<b>${files.length.toLocaleString()}${files.length >= d.limit ? "+" : ""}</b></span>
      <span class="pill">Running time:<b>${dur(total) || "—"}</b></span>
      <span class="pill">Size:<b>${bytes(bytesTotal)}</b></span>
      ${flagged ? `<span class="pill">Flagged:<b style="color:var(--warn)">${flagged}</b></span>` : ""}
      ${rejected ? `<span class="pill">Rejected:<b style="color:var(--bad)">${rejected}</b></span>` : ""}
    </div>
    <table class="list media-table"><thead><tr><th></th><th>Clip</th><th>Drive</th><th>Format</th><th class="num">fps</th><th class="num">Length</th><th>Start TC</th><th>Audio</th><th>Scene / take</th><th>QC</th><th>Camera</th></tr></thead><tbody>
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
    }).join("")}</tbody></table>
    ${files.length >= d.limit ? `<p class="hint" style="margin:10px 0 0">Showing the first ${d.limit.toLocaleString()}. Filter to see the rest, or export the CSV.</p>` : ""}`;
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
  const preview = data.playable
    ? (kind === "video" ? `<video controls preload="metadata" src="${stream}"></video>` : kind === "audio" ? `<div class="audio-box">${ICON.clapper}<audio controls preload="metadata" src="${stream}"></audio></div>` : `<img src="${stream}" alt="">`)
    : `<div class="no-preview">${ICON.film}<span>${dr.connected ? "No preview for this file type" : `Plug in ${esc(dr.title)} to preview`}</span></div>`;
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
  wireFile(driveId, path, m);
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
  const pref = localGet("appearance", "dark");
  const light = pref === "light" || (pref === "system" && window.matchMedia("(prefers-color-scheme: light)").matches);
  document.documentElement.dataset.theme = light ? "light" : "dark";
  if (native) nativeCall("appearance", { theme: light ? "light" : "dark" });
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
      <div class="sec"><div class="sec-title">${ICON.gear}General</div>
        <div class="box"><div class="fields"><div class="field"><label>Appearance</label>
          <div class="seg" id="appearance">${[["system", "System"], ["light", "Light"], ["dark", "Dark"]].map(([v, l]) => `<button data-appearance="${v}" class="${localGet("appearance", "dark") === v ? "on" : ""}">${l}</button>`).join("")}</div></div></div></div></div>

      <div class="sec"><div class="sec-title">${ICON.drive}Drives</div>
        <div class="box">
          <div class="fields">
            <div class="field"><label>Which volumes count as drives</label>
              <label class="switch" style="margin-top:7px">Hide the Mac's internal volumes <input type="checkbox" data-pref="hideInternal" ${pf.hideInternal ? "checked" : ""}></label>
              <label class="switch" style="margin-top:8px">Hide mounted disk images (.dmg, .iso) <input type="checkbox" data-pref="hideImages" ${pf.hideImages ? "checked" : ""}></label></div>
            <div class="field"><label>Logging</label>
              <label class="switch" style="margin-top:7px" title="Off: only drives already in the library are kept up to date when plugged in">Log new drives when plugged in <input type="checkbox" data-pref="autoLog" ${pf.autoLog ? "checked" : ""}></label>
              <label class="switch" style="margin-top:8px" title="Codec, frame rate, timecode and camera, read from clip headers during scans">Read clip metadata during scans <input type="checkbox" data-pref="autoMetadata" ${pf.autoMetadata ? "checked" : ""}></label></div>
            <div class="field"><label>Rescan plugged-in drives while Lumpy is open <span class="hint">· off is recommended: a rescan reads every folder, which can get in the way of editing</span></label>
              <select class="plain" data-pref="rescanMinutes">${[[0, "Only when plugged in, or by hand"], [15, "Every 15 minutes"], [60, "Every hour"], [240, "Every 4 hours"]].map(([v, l]) => `<option value="${v}" ${+pf.rescanMinutes === v ? "selected" : ""}>${l}</option>`).join("")}</select></div>
            <div class="field"><label>Folder exclusions <span class="hint">· names left out of library scans at any depth; a trailing * matches a prefix</span></label><input data-pref="exclude" value="${esc(pf.exclude)}" placeholder=".git, node_modules, Private, Render*" spellcheck="false"></div>
          </div>
          <p class="hint" style="margin:10px 0 0">Exclusions apply to the next scan of each drive (Details → Rescan). Offloads always copy everything on the card except macOS housekeeping files.</p>
          ${hiddenDrives.length ? `<p class="hint" style="margin:12px 0 6px;color:var(--text-2);font-weight:550">Never logged</p><div class="checks">${hiddenDrives.map((d) => `<label><span class="dot" style="background:${d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(d.title)}<span class="hint">${esc(d.name !== d.title ? d.name : "")}</span><button class="btn small" data-unhide="${esc(d.id)}" style="margin-left:auto">Log again</button></label>`).join("")}</div>` : ""}
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
    applyAppearance();
  };
  // Preferences save as they change.
  for (const el of document.querySelectorAll("[data-pref]")) el.onchange = async () => {
    const body = {}; body[el.dataset.pref] = el.type === "checkbox" ? el.checked : el.tagName === "SELECT" ? +el.value : el.value;
    try { S.settings.prefs = await api("/api/prefs", { method: "PUT", body }); if (el.dataset.pref === "exclude") el.value = S.settings.prefs.exclude; loadState(); }
    catch (e) { toast(e.message, true); }
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

// ---------- events ----------
document.querySelector(".tabs").onclick = (e) => {
  const b = e.target.closest("button[data-tab]");
  if (!b) return;
  if (b.dataset.tab === "projects" && S.tab === "projects") S.project = null;
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
$("#settings").innerHTML = ICON.gear;
$("#activity-btn").insertAdjacentHTML("afterbegin", ICON.activity);
$("#activity-btn").onclick = () => toggleActivity();
document.addEventListener("click", (e) => { if (!$("#activity-pop").hidden && !e.target.closest("#activity-pop, #activity-btn")) toggleActivity(false); });
$("#settings").onclick = () => openSettings();
$("#update-pill").onclick = () => openSettings("updates");
$("#refresh").onclick = () => { loadState(); loadJobs(); };
$(".search-icon").innerHTML = ICON.search;
$("#export").innerHTML = ICON.export + "Export CSV…";
async function downloadCSV(url, name) {
  try {
    const res = await fetch(url, { headers: { "X-Lumpy-Token": token } });
    if (!res.ok) throw new Error(res.statusText);
    const text = await res.text();
    if (native) { const saved = await nativeCall("saveFile", { name, content: text }); if (saved) toast("Saved " + saved); return; }
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/csv" }));
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
  openDetail(card.dataset.drive, false, act && act.dataset.act === "edit");
};
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
