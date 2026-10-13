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
  reveal: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/></svg>',
};
const COLORS = { purple: "#a855f7", blue: "#3b82f6", green: "#34c759", yellow: "#f5b400", orange: "#f08a24", red: "#ff453a", pink: "#ec4899", teal: "#14b8a6", gray: "#8e8e93" };
const SLICE_COLORS = ["#f08a24", "#3b82f6", "#c026d3", "#e5484d", "#34c759", "#2dd4bf", "#6366f1", "#facc15", "#b08968", "#94a3b8"];

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
  tab: localGet("tab", "logged"),
  view: localGet("view", "grid"),
  sort: localGet("sort", "seen"),
  desc: localGet("desc", "1") === "1",
  query: "",
  state: null,
  jobs: [],
  fileHits: null,
  detail: null, // id of open drive
  offload: { source: "", dests: [""], name: "" },
};

// ---------- data refresh ----------
async function loadState() {
  try {
    const first = !S.state;
    S.state = await api("/api/state");
    // The offload form keeps what's being typed; only the counts refresh.
    if (!first && (S.tab === "offload" || S.tab === "activity")) renderCounts();
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
      }
      lastJobSig = sig;
    }
    if (S.tab === "activity" || S.tab === "offload") renderJobsInto();
  } catch {}
}

// ---------- rendering ----------
function capClass(used, size) {
  const p = size ? used / size : 0;
  return p > 0.95 ? "full" : p > 0.85 ? "hi" : p > 0.6 ? "mid" : "lo";
}

function backupBadge(d) {
  const b = d.backups;
  if (d.damaged) return `<span class="badge bad">${ICON.warn}${d.damaged} damaged</span>`;
  if (!b || !d.files) return "";
  const missing = b.short + b.none + b.unknown;
  if (b.minCopies < 0) return `<span class="badge">${ICON.dash}No checksums yet</span>`;
  if (missing > 0) return `<span class="badge bad">${ICON.warn}${missing.toLocaleString()} not backed up</span>`;
  if (b.likely > 0) return `<span class="badge warn">${ICON.warn}${b.likely.toLocaleString()} unproven</span>`;
  return `<span class="badge ok">${ICON.check}${b.minCopies}× backed up</span>`;
}
function checkBadge(d) {
  if (d.busy) return `<span class="badge info">${ICON.clock}Working…</span>`;
  if (d.verifyRequested) return `<span class="badge info">${ICON.clock}Check queued</span>`;
  if (!d.lastVerify) return "";
  return d.lastVerifyOk
    ? `<span class="badge ok">${ICON.shield}Checked ${new Date(d.lastVerify * 1000).toLocaleDateString(undefined, { month: "short", day: "numeric" })}</span>`
    : `<span class="badge bad">${ICON.warn}Check failed</span>`;
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

function driveCard(d) {
  const l = d.labels;
  const tint = COLORS[l.color] || "transparent";
  const dot = d.connected ? "var(--ok)" : COLORS[l.color] || "var(--muted)";
  const used = Math.max(0, d.size - d.free);
  const pct = d.size ? Math.min(100, (used / d.size) * 100) : 0;
  const who = [l.client, l.project].filter(Boolean);
  const sum = summaryText(d);
  return `<article class="card" style="--tint:${tint}" data-drive="${esc(d.id)}">
    <div class="card-top">
      <span class="dot" style="background:${dot}" title="${d.connected ? "Plugged in" : "Not plugged in"}"></span>
      <span class="drive-ico">${ICON.drive}</span>
      ${d.connected ? `<span class="badge live">Plugged in</span>` : ""}
      <button class="icon-btn card-edit" data-act="edit" title="Edit labels">${ICON.edit}</button>
    </div>
    <div class="card-title"><h3>${hl(d.title)}</h3>${l.nickname ? `<span class="vol">(${hl(d.name)})</span>` : ""}</div>
    ${who.length ? `<div class="meta-line">${ICON.person}<span>${who.map(hl).join(" &nbsp;·&nbsp; ")}</span></div>` : ""}
    ${l.contact ? `<div class="meta-line">${ICON.mail}<span>${hl(l.contact)}</span></div>` : ""}
    <div class="path" title="${esc(d.mount || d.lastMount)}">${esc(d.mount || d.lastMount)}</div>
    <div class="bar ${capClass(used, d.size)}"><i style="width:${pct.toFixed(1)}%"></i></div>
    <div class="cap">${bytes(d.size)}<span class="sep">•</span>Free ${bytes(d.free)}<span class="sep">•</span>${d.files.toLocaleString()} files</div>
    ${sum ? `<div class="summary">${esc(sum)}</div><button class="more" data-act="open">More</button>` : ""}
    <div class="badges">${backupBadge(d)}${checkBadge(d)}</div>
    <div class="card-foot"><span>${d.connected ? "Plugged in now" : "Last seen • " + when(d.lastSeen)}</span>
      <button class="btn small" data-act="open">Details</button></div>
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
  const pill = $("#agent-pill");
  const agentOn = st.agent.startsWith("installed and running");
  pill.className = "agent-pill" + (agentOn ? " on" : "");
  pill.textContent = agentOn ? "Auto-logging on" : "Logging while open";
  pill.title = "Background agent: " + st.agent;
}

function render() {
  if (!S.state) return;
  renderCounts();
  renderToolbar();
  const main = $("#main");
  const keepScroll = main.scrollTop;
  if (S.tab === "offload") return renderOffload();
  if (S.tab === "activity") return renderActivity();
  main.innerHTML = S.tab === "connected" ? renderConnected() : renderLogged();
  main.scrollTop = keepScroll;
}

function drivesHTML(ds) {
  if (S.view === "list") {
    return `<table class="list"><thead><tr><th>Name</th><th>Client · Project</th><th class="num">Capacity</th><th>Used</th><th class="num">Free</th><th class="num">Files</th><th>Backups</th><th>Last check</th><th>Last seen</th></tr></thead><tbody>
      ${ds.map((d) => {
        const used = Math.max(0, d.size - d.free);
        return `<tr class="row" data-drive="${esc(d.id)}"><td><div class="name"><span class="dot" style="background:${d.connected ? "var(--ok)" : COLORS[d.labels.color] || "var(--muted)"}"></span>${hl(d.title)}${d.labels.nickname ? ` <small>${esc(d.name)}</small>` : ""}</div></td>
          <td>${esc([d.labels.client, d.labels.project].filter(Boolean).join(" · "))}</td>
          <td class="num">${bytes(d.size)}</td><td><div class="bar ${capClass(used, d.size)}"><i style="width:${d.size ? ((used / d.size) * 100).toFixed(1) : 0}%"></i></div></td>
          <td class="num">${bytes(d.free)}</td><td class="num">${d.files.toLocaleString()}</td>
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
      return `<tr class="row" data-drive="${esc(m.DriveID)}"><td><div class="name"><span class="dot" style="background:${d && d.connected ? "var(--ok)" : "var(--muted)"}"></span>${esc(d ? d.title : m.DriveName)}</div></td>
        <td class="mono">${hl(m.Path)}</td><td class="num">${bytes(m.Size)}</td><td class="mono">${esc(m.XXH64 || "—")}</td></tr>`;
    }).join("")}</tbody></table>`;
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
    html += extra.map(({ v, never }) => `<article class="card"><div class="card-top"><span class="dot" style="background:var(--ok)"></span><span class="drive-ico">${ICON.drive}</span></div>
      <div class="card-title"><h3>${esc(v.name)}</h3></div><div class="path">${esc(v.mount)}</div>
      <div class="bar ${capClass(v.size - v.free, v.size)}"><i style="width:${v.size ? (((v.size - v.free) / v.size) * 100).toFixed(1) : 0}%"></i></div>
      <div class="cap">${bytes(v.size)}<span class="sep">•</span>Free ${bytes(v.free)}</div>
      <div class="card-foot"><span>${never ? "Set to never log" : "Being logged…"}</span>
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
}

async function openDetail(id, refresh, focusLabels) {
  let data;
  try { data = await api(`/api/drives/${encodeURIComponent(id)}`); }
  catch (e) { toast(e.message, true); return; }
  if (refresh && S.detail !== id) return;
  S.detail = id;
  const d = data.drive, l = d.labels, b = data.backups;
  const used = Math.max(0, d.size - d.free);
  const total = data.breakdown.reduce((a, s) => a + s.bytes, 0) || 1;
  const slices = data.breakdown.slice(0, 9);
  const rest = data.breakdown.slice(9).reduce((a, s) => a + s.bytes, 0);
  if (rest) slices.push({ name: "Other", bytes: rest, folder: true });
  const missing = b.short + b.none + b.unknown;
  const holders = Object.entries(b.holders).sort((a, c) => c[1] - a[1]);
  const scroll = refresh ? ($(".sheet-body") || {}).scrollTop : 0;

  openSheet(`
    <div class="sheet-head">
      <span class="drive-ico">${ICON.drive}</span>
      <div><h2>${esc(d.title)}</h2><div class="sub">${esc(d.name)}${d.connected ? " · plugged in" : ""}</div></div>
      <div class="actions">
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
          <div class="field wide"><label>Notes</label><textarea data-label="notes" placeholder="Where it lives, who has it, what's on it…">${esc(l.notes)}</textarea></div>
          <div class="field wide"><label>Color</label><div class="swatches">
            <button class="swatch none ${l.color ? "" : "on"}" data-color="" title="None"></button>
            ${Object.entries(COLORS).map(([k, c]) => `<button class="swatch ${l.color === k ? "on" : ""}" data-color="${k}" title="${k}" style="background:${c}"></button>`).join("")}
          </div></div>
        </div></div></div>

      <div class="sec"><div class="sec-title">${ICON.gauge}Capacity</div>
        <div class="box"><dl class="kv"><dt>Total</dt><dd>${bytes(d.size)}</dd><dt>Used</dt><dd>${bytes(used)}</dd><dt>Free</dt><dd>${bytes(d.free)}</dd></dl>
        <div class="bar ${capClass(used, d.size)}" style="height:7px;margin-top:10px"><i style="width:${d.size ? ((used / d.size) * 100).toFixed(1) : 0}%;background:var(--accent)"></i></div></div></div>

      <div class="sec"><div class="sec-title">${ICON.shield}Backups &amp; checks
          <div class="right">
            ${d.verifyRequested ? `<button class="btn small" id="cancel-check">Cancel queued check</button>` : ""}
            ${d.connected ? `<button class="btn small" id="hash" ${d.busy ? "disabled" : ""}>Checksum new files</button>` : ""}
            <button class="btn small primary" id="verify" ${d.busy || (d.verifyRequested && !d.connected) ? "disabled" : ""}>${d.connected ? "Check drive now" : d.verifyRequested ? "Check queued" : "Queue check"}</button>
          </div></div>
        <div class="stat-row">
          <div class="stat ${missing ? "bad" : b.minCopies > 0 ? "ok" : ""}"><div class="v">${b.minCopies < 0 ? "—" : missing ? missing.toLocaleString() : b.minCopies + "×"}</div><div class="l">${b.minCopies < 0 ? "no checksums yet" : missing ? "files not backed up" : "fewest copies elsewhere"}</div></div>
          <div class="stat"><div class="v">${d.files ? Math.round((d.hashed / d.files) * 100) : 0}%</div><div class="l">${d.hashed.toLocaleString()} of ${d.files.toLocaleString()} checksummed</div></div>
          <div class="stat ${d.damaged ? "bad" : ""}"><div class="v">${d.damaged}</div><div class="l">damaged files</div></div>
          <div class="stat ${d.lastVerify ? (d.lastVerifyOk ? "ok" : "bad") : ""}"><div class="v">${d.lastVerify ? (d.lastVerifyOk ? "OK" : "Failed") : "Never"}</div><div class="l">${d.lastVerify ? "last full check " + new Date(d.lastVerify * 1000).toLocaleDateString() : d.verifyRequested ? "check queued for next mount" : "full check"}</div></div>
        </div>
        ${holders.length ? `<p class="hint" style="margin:10px 0 0;color:var(--text-2)">Copies on ${holders.map(([n, c]) => `<b>${esc(n)}</b> (${c.toLocaleString()} files)`).join(", ")}</p>` : ""}
        ${b.likely ? `<p class="hint" style="margin:6px 0 0;color:var(--muted)">${b.likely.toLocaleString()} files without checksums match a same-name, same-size file elsewhere (probable copies). ${d.connected ? "Checksum new files to prove them." : "Plug the drive in and checksum them to prove it."}</p>` : ""}
        ${b.problems && b.problems.length ? `<p class="hint" style="margin:12px 0 0;color:var(--text-2);font-weight:550">Needs attention${b.problems.length >= 200 ? " (first 200)" : ""}: not backed up${b.damaged ? ", or damaged on this drive" : ""}</p><div class="problems">${b.problems.map((p) => `<div>${esc(p)}</div>`).join("")}</div>` : ""}
      </div>

      <div class="sec"><div class="sec-title">${ICON.pie}Storage breakdown
          <div class="right">${d.connected ? `<button class="btn small" id="rescan" ${d.busy ? "disabled" : ""}>${ICON.refresh}Rescan</button>` : ""}</div></div>
        <div class="box">
          ${data.breakdown.length ? `<div class="stack">${slices.map((s, i) => `<i style="width:${((s.bytes / total) * 100).toFixed(2)}%;background:${SLICE_COLORS[i % SLICE_COLORS.length]}" title="${esc(s.name)}"></i>`).join("")}</div>
          <div class="legend">${slices.map((s, i) => `<span class="sw" style="background:${SLICE_COLORS[i % SLICE_COLORS.length]}"></span><span>${esc(s.name)}${s.folder ? "" : ' <small style="color:var(--muted)">(file)</small>'}</span><span class="num">${bytes(s.bytes)}</span><span class="num">${Math.round((s.bytes / total) * 100)}%</span>`).join("")}</div>`
          : `<span style="color:var(--muted)">No files recorded on this drive.</span>`}
        </div></div>

      ${summaryText(d) ? `<div class="sec"><div class="sec-title">${ICON.search}Top-level summary</div><div style="color:var(--text-2)">${esc(summaryText(d))}</div></div>` : ""}

      <div class="sec"><div class="sec-title">${ICON.folder}Browse drive <span style="color:var(--muted);font-weight:400;font-size:12px">Indexed • ${d.files.toLocaleString()} files${d.lastScan ? " • " + when(d.lastScan) : ""}</span></div>
        <div class="search"><span class="search-icon">${ICON.search}</span><input id="tree-filter" type="search" placeholder="Filter by name…"></div>
        <div class="tree" id="tree"><div class="tree-note">Loading…</div></div></div>

      <div class="sec"><div class="sec-title">${ICON.history}History</div>
        <div class="history">${data.events.map((e) => `<div><span class="when">${short(e.at)}</span><span class="kind">${esc(e.kind)}</span><span class="${e.failures ? "fail" : ""}">${esc(e.summary)}</span></div>`).join("") || '<span style="color:var(--muted)">Nothing yet.</span>'}</div></div>
    </div>`);
  if (scroll) $(".sheet-body").scrollTop = scroll;
  wireDetail(d);
  loadTree(d.id, "", $("#tree"), 0);
  if (focusLabels) setTimeout(() => { const el = $('[data-label="nickname"]'); el.scrollIntoView({ block: "center" }); el.focus(); }, 30);
}

function wireDetail(d) {
  const save = async (patch) => {
    try { await api(`/api/drives/${encodeURIComponent(d.id)}`, { method: "PATCH", body: { ...d.labels, ...patch } }); Object.assign(d.labels, patch); loadState(); }
    catch (e) { toast(e.message, true); }
  };
  $("#close").onclick = closeSheet;
  $("#never-log").onchange = (e) => save({ neverLog: e.target.checked });
  for (const el of document.querySelectorAll("[data-label]")) el.onchange = () => save({ [el.dataset.label]: el.value });
  for (const el of document.querySelectorAll("[data-color]")) el.onclick = () => {
    document.querySelectorAll("[data-color]").forEach((x) => x.classList.toggle("on", x === el));
    save({ color: el.dataset.color });
  };
  $("#forget").onclick = async () => {
    if (!confirm(`Remove ${d.title} from the library? Its file list, checksums and history are deleted. The drive itself is not touched.`)) return;
    try { await api(`/api/drives/${encodeURIComponent(d.id)}`, { method: "DELETE" }); closeSheet(); toast(`${d.title} removed`); loadState(); }
    catch (e) { toast(e.message, true); }
  };
  const act = (sel, path, msg) => { const el = $(sel); if (el) el.onclick = async () => {
    try { const r = await api(`/api/drives/${encodeURIComponent(d.id)}/${path}`, { method: "POST" });
      toast(r.queued ? `${d.title} isn't plugged in. The check will run next time it's mounted.` : msg); loadJobs(); loadState(); openDetail(d.id, true); }
    catch (e) { toast(e.message, true); } }; };
  act("#verify", "verify", `Checking ${d.title}… (see Activity)`);
  act("#hash", "hash", `Checksumming new files on ${d.title}…`);
  act("#rescan", "rescan", `Rescanning ${d.title}…`);
  act("#cancel-check", "cancel", "Queued check cancelled");
  const rv = $("#reveal");
  if (rv) rv.onclick = () => nativeCall("reveal", { path: d.mount });
  let t;
  $("#tree-filter").oninput = (e) => { clearTimeout(t); t = setTimeout(() => loadTree(d.id, "", $("#tree"), 0, e.target.value.trim()), 200); };
}

function statusIcon(e) {
  if (e.damaged) return `<span class="bad" title="Failed its last check">${ICON.warn}</span>`;
  if (e.folder) return "";
  if (e.verified) return `<span class="ok" title="Read back and matched">${ICON.check}</span>`;
  if (e.xxh64) return `<span class="none" title="xxh64 ${esc(e.xxh64)}">${ICON.check}</span>`;
  return `<span class="none" title="No checksum yet">${ICON.dash}</span>`;
}

async function loadTree(id, dir, container, depth, filter) {
  let entries;
  try { entries = await api(`/api/drives/${encodeURIComponent(id)}/list?dir=${encodeURIComponent(dir)}&filter=${encodeURIComponent(filter || "")}`); }
  catch (e) { container.innerHTML = `<div class="tree-note">${esc(e.message)}</div>`; return; }
  if (!entries.length) { container.innerHTML = `<div class="tree-note">${filter ? "No files match." : "Empty."}</div>`; return; }
  container.innerHTML = entries.map((e) => `<div class="tree-row ${e.folder ? "folder" : ""}" data-path="${esc(e.path)}" style="padding-left:${10 + depth * 18}px">
      <div class="nm">${e.folder ? ICON.chev + ICON.folder : '<span style="width:12px"></span>' + ICON.file}<span title="${esc(e.path)}">${esc(e.name)}</span></div>
      <span class="sz">${bytes(e.bytes)}${e.folder ? ` · ${e.files.toLocaleString()}` : ""}</span><span class="st">${statusIcon(e)}</span></div>
      ${e.folder ? `<div class="kids" hidden></div>` : ""}`).join("") + (entries.length >= 2000 ? `<div class="tree-note">Showing the first 2,000.</div>` : "");
  for (const row of container.querySelectorAll(":scope > .tree-row.folder")) {
    row.onclick = () => {
      const kids = row.nextElementSibling;
      const open = row.classList.toggle("open");
      kids.hidden = !open;
      if (open && !kids.dataset.loaded) { kids.dataset.loaded = "1"; kids.innerHTML = `<div class="tree-note" style="padding-left:${28 + depth * 18}px">Loading…</div>`; loadTree(id, row.dataset.path, kids, depth + 1); }
    };
  }
}

// ---------- offload ----------
function renderOffload() {
  const o = S.offload;
  const vols = S.state.volumes;
  const chips = (target) => vols.map((v) => `<button class="chip" data-chip="${target}" data-path="${esc(v.mount)}">${esc(v.name)}<small>${bytes(v.free)} free</small></button>`).join("");
  const name = o.name || (o.source ? o.source.replace(/\/+$/, "").split("/").pop() : "");
  $("#main").innerHTML = `<div class="offload">
    <section class="panel">
      <h2>New offload</h2>
      <p class="hint">Lumpy reads the source once, writes every destination at the same time, reads each copy back from its drive and compares xxh64 checksums. Each copy gets an ASC MHL manifest and is recorded in the library.</p>
      <div class="step"><span class="step-n">1</span><div><h3>Source card or folder</h3>
        <div class="path-row"><input class="path-input" id="src" value="${esc(o.source)}" placeholder="/Volumes/A001"><button class="btn" data-pick="src">Choose…</button></div>
        <div class="chips">${chips("src")}</div></div></div>
      <div class="step"><span class="step-n">2</span><div><h3>Destinations <span style="color:var(--muted);font-weight:400">(at least two recommended)</span></h3>
        ${o.dests.map((p, i) => `<div class="path-row"><input class="path-input" data-dest="${i}" value="${esc(p)}" placeholder="/Volumes/SHUTTLE_1/Day_01"><button class="btn" data-pick="dest" data-i="${i}">Choose…</button>${o.dests.length > 1 ? `<button class="icon-btn" data-rmdest="${i}" title="Remove">${ICON.x}</button>` : ""}</div>`).join("")}
        <div class="chips"><button class="chip" id="add-dest">${"+ Add destination"}</button>${chips("dest")}</div></div></div>
      <div class="step"><span class="step-n">3</span><div><h3>Folder name in each destination</h3>
        <div class="path-row"><input class="path-input" id="oname" value="${esc(o.name)}" placeholder="${esc(name || "Same as the source")}"></div>
        <div class="go-row"><button class="btn primary" id="start-offload">${ICON.copy}Start offload</button>
          <span class="note" id="offload-note">${o.source && o.dests.some(Boolean) ? `Copies to ${o.dests.filter(Boolean).map((d) => esc(d.replace(/\/+$/, "") + "/" + (name || ""))).join(", ")}` : ""}</span></div></div></div>
    </section>
    <div><div class="section-head"><h2>Offloads</h2></div><div class="jobs" id="jobs"></div></div>
  </div>`;
  wireOffload();
  renderJobsInto();
}

function wireOffload() {
  const o = S.offload;
  const rerender = () => renderOffload();
  $("#src").onchange = (e) => { o.source = e.target.value.trim(); rerender(); };
  $("#oname").onchange = (e) => { o.name = e.target.value.trim(); rerender(); };
  for (const el of document.querySelectorAll("[data-dest]")) el.onchange = () => { o.dests[+el.dataset.dest] = el.value.trim(); rerender(); };
  for (const el of document.querySelectorAll("[data-rmdest]")) el.onclick = () => { o.dests.splice(+el.dataset.rmdest, 1); rerender(); };
  $("#add-dest").onclick = () => { o.dests.push(""); rerender(); };
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
      await api("/api/offload", { method: "POST", body: { source: o.source, dests, name: o.name } });
      toast("Offload started");
      S.offload = { source: "", dests: [""], name: "" };
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
function renderActivity() {
  $("#main").innerHTML = `<div class="offload"><div class="section-head"><h2>Activity</h2><span>Offloads, checks and scans started from this window</span></div><div class="jobs" id="jobs"></div></div>`;
  renderJobsInto();
}

const openLogs = new Set();
function renderJobsInto() {
  const box = $("#jobs");
  if (!box) return;
  const list = S.tab === "offload" ? S.jobs.filter((j) => j.kind === "offload") : S.jobs;
  if (!list.length) {
    box.innerHTML = `<div class="empty" style="margin:30px auto">${ICON.activity}<p>${S.tab === "offload" ? "No offloads yet." : "Nothing has run yet. Offloads, drive checks and rescans show their progress here."}</p></div>`;
    return;
  }
  const scrolls = {};
  box.querySelectorAll(".log").forEach((el) => { scrolls[el.dataset.id] = el.scrollHeight - el.scrollTop - el.clientHeight < 20 ? -1 : el.scrollTop; });
  box.innerHTML = list.map((j) => {
    const p = j.progress;
    const pct = p && p.total ? Math.min(100, (p.done / p.total) * 100) : j.state === "running" ? 0 : 100;
    const state = { running: ["info", "Running"], ok: ["ok", "Done"], failed: ["bad", `${j.failures} problem(s)`], cancelled: ["warn", "Cancelled"], error: ["bad", "Error"] }[j.state];
    const eta = p && p.rate > 0 && p.total > p.done ? duration((p.total - p.done) / p.rate) + " left" : "";
    const lines = j.lines.map((l) => {
      const c = l.startsWith("FAIL") || l.startsWith("BAD") ? "f" : l.startsWith("WARN") ? "w" : l.startsWith("OK") ? "k" : "";
      return c ? `<span class="${c}">${esc(l)}</span>` : esc(l);
    }).join("\n");
    const outcome = j.lines.filter((l) => /^(OK|BAD) /.test(l)).map((l) => esc(l.replace(/\s{2,}/g, "  "))).join("<br>");
    return `<div class="job"><div class="job-head"><h3>${esc(j.title)}</h3><span class="badge ${state[0]}">${state[1]}</span>
        <span class="when">${short(j.started)}${j.ended ? " · " + duration(j.ended - j.started || 1) : ""}</span>
        <div class="actions">${j.state === "running" ? `<button class="btn small" data-cancel="${j.id}">${ICON.x}Cancel</button>` : ""}</div></div>
      ${j.state === "running" ? `<div class="bar"><i style="width:${pct.toFixed(1)}%"></i></div>
        <div class="job-stats">${p ? `<span><b>${esc(p.label)}</b></span><span>${bytes(p.done)} of ${bytes(p.total)}</span><span>${bytes(p.rate)}/s</span><span>${eta}</span>` : "<span>Preparing…</span>"}${j.failures ? `<span style="color:var(--bad)">${j.failures} problem(s)</span>` : ""}</div>` : ""}
      ${j.error ? `<div class="job-stats" style="color:var(--bad);margin-top:8px">${esc(j.error)}</div>` : ""}
      ${outcome && j.state !== "running" ? `<div class="job-stats" style="margin-top:8px;display:block;font-family:var(--mono);font-size:11.5px">${outcome}</div>` : ""}
      <details class="logwrap" data-id="${j.id}" ${openLogs.has(j.id) || (j.state === "running" && j.failures) ? "open" : ""}><summary>Log (${j.lines.length} lines)</summary><div class="log" data-id="${j.id}">${lines}</div></details></div>`;
  }).join("");
  box.querySelectorAll(".log").forEach((el) => { const s = scrolls[el.dataset.id]; el.scrollTop = s === undefined || s === -1 ? el.scrollHeight : s; });
  box.querySelectorAll("details.logwrap").forEach((el) => el.ontoggle = () => { el.open ? openLogs.add(+el.dataset.id) : openLogs.delete(+el.dataset.id); });
  box.querySelectorAll("[data-cancel]").forEach((b) => b.onclick = async () => { try { await api(`/api/jobs/${b.dataset.cancel}/cancel`, { method: "POST" }); } catch (e) { toast(e.message, true); } });
}

// ---------- events ----------
document.querySelector(".tabs").onclick = (e) => {
  const b = e.target.closest("button[data-tab]");
  if (!b) return;
  S.tab = b.dataset.tab;
  localSet("tab", S.tab);
  render();
};
$("#view-seg").onclick = (e) => { const b = e.target.closest("button"); if (!b) return; S.view = b.dataset.view; localSet("view", S.view); render(); };
$("#sort").onchange = (e) => { S.sort = e.target.value; localSet("sort", S.sort); render(); };
$("#sort-dir").onclick = () => { S.desc = !S.desc; localSet("desc", S.desc ? "1" : "0"); render(); };
$("#refresh").innerHTML = ICON.refresh;
$("#refresh").onclick = () => { loadState(); loadJobs(); };
$(".search-icon").innerHTML = ICON.search;
$("#export").innerHTML = ICON.export + "Export CSV…";
$("#export").onclick = async () => {
  try {
    const res = await fetch("/api/export.csv", { headers: { "X-Lumpy-Token": token } });
    if (!res.ok) throw new Error(res.statusText);
    const text = await res.text();
    const name = `lumpy-drives-${new Date().toISOString().slice(0, 10)}.csv`;
    if (native) { const saved = await nativeCall("saveFile", { name, content: text }); if (saved) toast("Saved " + saved); return; }
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/csv" }));
    a.download = name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  } catch (e) { toast(e.message, true); }
};
let searchTimer;
$("#search").oninput = (e) => {
  S.query = e.target.value;
  if (S.tab !== "logged" && S.tab !== "connected") { S.tab = "logged"; }
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
    try { await api("/api/catalog", { method: "POST", body: { id: logBtn.dataset.log } }); toast("Logging drive…"); loadJobs(); } catch (err) { toast(err.message, true); }
    return;
  }
  const card = e.target.closest("[data-drive]");
  if (!card) return;
  const act = e.target.closest("[data-act]");
  openDetail(card.dataset.drive, false, act && act.dataset.act === "edit");
};
$("#scrim").onclick = () => { if (S.detail) closeSheet(); };
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && !$("#sheet").hidden && S.detail) closeSheet();
  if ((e.metaKey || e.ctrlKey) && e.key === "f") { e.preventDefault(); $("#search").focus(); }
});

loadState();
loadJobs();
setInterval(() => { if (!document.hidden) loadState(); }, 3000);
setInterval(() => { if (!document.hidden) loadJobs(); }, 1000);
