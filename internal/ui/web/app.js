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
  tab: localGet("tab", "logged"),
  view: localGet("view", "grid"),
  sort: localGet("sort", "seen"),
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
};

// ---------- data refresh ----------
async function loadState() {
  try {
    const first = !S.state;
    S.state = await api("/api/state");
    // The offload form keeps what's being typed; only the counts refresh.
    if (!first && (S.tab === "offload" || S.tab === "activity")) renderCounts();
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
      }
      lastJobSig = sig;
    }
    if (S.tab === "offload") $(".offload")?.classList.toggle("busy", running > 0);
    if (S.tab === "activity" || S.tab === "offload") renderJobsInto();
  } catch {}
}

// ---------- rendering ----------
// ratioBar draws bytes by backup state: green has enough copies, yellow
// has some but not enough, red is a sole copy, grey has no checksum.
function ratioBar(parts, total, withKey) {
  total = total || 1;
  const order = [["backed", "Backed up"], ["short", "Not enough copies"], ["sole", "Sole copy"], ["likely", "Probably backed up"], ["unknown", "No checksum"], ["damaged", "Damaged"]];
  const bar = `<div class="hbar ${withKey ? "" : "thin"}">${order.map(([k]) => parts[k] ? `<i class="${k}" style="width:${((parts[k] / total) * 100).toFixed(2)}%" title="${k}: ${bytes(parts[k])}"></i>` : "").join("")}</div>`;
  if (!withKey) return bar;
  return bar + `<div class="hkey">${order.filter(([k]) => parts[k]).map(([k, label]) => `<span><i class="${k}" style="background:${{ backed: "var(--ok)", short: "var(--warn)", sole: "var(--bad)", likely: "var(--blue)", unknown: "#4a4a52", damaged: "#8a1c1c" }[k]}"></i>${label} <b>${bytes(parts[k])}</b> (${Math.round((parts[k] / total) * 100)}%)</span>`).join("")}</div>`;
}

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

function driveCard(d) {
  const l = d.labels;
  const tint = COLORS[l.color] || "transparent";
  const dot = d.connected ? "var(--ok)" : COLORS[l.color] || "var(--muted)";
  const used = Math.max(0, d.size - d.free);
  const pct = d.size ? Math.min(100, (used / d.size) * 100) : 0;
  const who = [l.client, l.project].filter(Boolean);
  const sum = summaryText(d);
  return `<article class="card ${S.selecting && S.selected.has(d.id) ? "picked" : ""}" style="--tint:${tint}" data-drive="${esc(d.id)}">
    <div class="card-top">
      <span class="dot" style="background:${dot}" title="${d.connected ? "Plugged in" : "Not plugged in"}"></span>
      <span class="drive-ico">${ICON.drive}</span>
      ${d.connected ? `<span class="badge live">Plugged in</span>` : ""}
      ${S.selecting ? `<input type="checkbox" class="pick" data-pick="${esc(d.id)}" ${S.selected.has(d.id) ? "checked" : ""}>` : `<button class="icon-btn card-edit" data-act="edit" title="Edit labels">${ICON.edit}</button>`}
    </div>
    <div class="card-title"><h3>${hl(d.title)}</h3>${l.nickname ? `<span class="vol">(${hl(d.name)})</span>` : ""}</div>
    ${who.length ? `<div class="meta-line">${ICON.person}<span>${who.map(hl).join(" &nbsp;·&nbsp; ")}</span></div>` : ""}
    ${l.contact ? `<div class="meta-line">${ICON.mail}<span>${hl(l.contact)}</span></div>` : ""}
    <div class="path" title="${esc(d.mount || d.lastMount)}">${esc(d.mount || d.lastMount)}</div>
    <div class="bar ${capClass(used, d.size)}"><i style="width:${pct.toFixed(1)}%"></i></div>
    <div class="cap">${bytes(d.size)}<span class="sep">•</span>Free ${bytes(d.free)}<span class="sep">•</span>${d.files.toLocaleString()} files</div>
    ${sum ? `<div class="summary">${esc(sum)}</div><button class="more" data-act="open">More</button>` : ""}
    <div class="badges">${backupBadge(d)}${checkBadge(d)}${projectBadges(d)}</div>
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
}

function render() {
  if (!S.state) return;
  renderCounts();
  renderToolbar();
  const main = $("#main");
  const keepScroll = main.scrollTop;
  if (S.tab === "offload") return renderOffload();
  if (S.tab === "activity") return renderActivity();
  if (S.tab === "projects") return renderProjects();
  main.innerHTML = (S.tab === "connected" ? renderConnected() : renderLogged()) + selectionBar();
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
        ${ratioBar({ backed: b.enoughBytes, short: b.shortBytes, sole: b.noneBytes, likely: b.likelyBytes, unknown: b.unknownBytes, damaged: b.damagedBytes }, b.bytes, true)}
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

      <div class="sec"><div class="sec-title">${ICON.pie}Contents by folder <span class="hint" style="font-weight:400">backed up elsewhere vs sole copy</span>
          <div class="right">${d.connected ? `<button class="btn small" id="rescan" ${d.busy ? "disabled" : ""}>${ICON.refresh}Rescan</button>` : ""}</div></div>
        ${folders.length ? `<table class="list compact"><thead><tr><th>Folder</th><th class="num">Size</th><th>Backed up</th><th class="num">Sole copy</th><th class="num">No checksum</th></tr></thead><tbody>
          ${folders.map((f) => `<tr><td><div class="name" style="font-weight:500">${f.folder ? ICON.folder.replace("<svg", '<svg style="width:14px;height:14px;color:var(--muted)"') : ICON.file.replace("<svg", '<svg style="width:14px;height:14px;color:var(--muted)"')}${esc(f.name)}</div></td>
            <td class="num">${bytes(f.bytes)}</td><td>${ratioBar({ backed: f.backedBytes, sole: f.soleBytes, unknown: f.unknownBytes }, f.bytes, false)}</td>
            <td class="num" style="color:${f.soleBytes ? "var(--bad)" : "var(--muted)"}">${f.soleBytes ? bytes(f.soleBytes) : "—"}</td>
            <td class="num" style="color:${f.unknownBytes ? "var(--warn)" : "var(--muted)"}">${f.unknownBytes ? bytes(f.unknownBytes) : "—"}</td></tr>`).join("")}</tbody></table>`
          : `<span class="hint">No files recorded on this drive.</span>`}
      </div>

      <div class="sec"><div class="sec-title">${ICON.film}Projects</div>
        <div class="box">${(S.state.projects || []).length ? `<div class="checks">${S.state.projects.map((p) => `<label><input type="checkbox" data-project="${p.id}" ${myProjects.has(p.id) ? "checked" : ""}><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span>${esc(p.name)}<span class="hint">· needs ${p.target} copies of everything</span></label>`).join("")}</div>`
          : `<span class="hint">No projects yet. Make one on the Projects tab to see how this drive and others back each other up.</span>`}</div></div>

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
  for (const el of document.querySelectorAll("[data-project]")) el.onchange = async () => {
    try { await api(`/api/projects/${el.dataset.project}`, { method: "PATCH", body: el.checked ? { add: [d.id] } : { remove: [d.id] } }); loadState(); }
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


// ---------- projects ----------
function healthParts(h) {
  return { backed: h.backedBytes, short: h.shortBytes, sole: h.soleBytes, unknown: h.unknownBytes };
}
function driveTitle(id) {
  const d = (S.state.drives || []).find((x) => x.id === id);
  return d ? d.title : id;
}
function driveOf(id) { return (S.state.drives || []).find((x) => x.id === id); }

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
  if (!S.projects) { main.innerHTML = `<div class="empty"><p>Loading…</p></div>`; loadProjects(); return; }
  const ps = S.projects;
  if (!ps.length) {
    main.innerHTML = `<div class="empty">${ICON.film}<h2>No projects yet</h2>
      <p>A project is a group of drives that together hold one body of work. Lumpy shows how well they back each other up and plans the copies that get every file onto two drives.</p>
      <p><button class="btn primary" id="new-project">${ICON.plus}New project</button></p></div>`;
  } else {
    main.innerHTML = `<div class="section-head"><h2>Projects</h2><span>${ps.length}</span><div class="right"><button class="btn primary" id="new-project">${ICON.plus}New project</button></div></div>
      <div class="grid">${ps.map((p) => {
        const h = p.health;
        const bad = h.soleBytes + h.shortBytes;
        return `<article class="card pcard" style="--tint:${COLORS[p.color] || "transparent"}" data-open-project="${p.id}">
          <div class="card-top"><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span><span class="drive-ico">${ICON.film}</span></div>
          <div class="card-title"><h3>${esc(p.name)}</h3><span class="vol">${p.driveIds.length} drive${p.driveIds.length === 1 ? "" : "s"} · ${p.target} copies wanted</span></div>
          ${ratioBar(healthParts(h), h.bytes, false)}
          <div class="cap">${bytes(h.bytes)} of content<span class="sep">•</span>${h.contents.toLocaleString()} files</div>
          <div class="badges">${bad ? `<span class="badge bad">${ICON.warn}${bytes(bad)} not backed up enough</span>` : h.bytes ? `<span class="badge ok">${ICON.check}Everything has ${p.target} copies</span>` : `<span class="badge">Empty</span>`}${h.unknownBytes ? `<span class="badge warn">${bytes(h.unknownBytes)} unproven</span>` : ""}${h.damaged ? `<span class="badge bad">${h.damaged} damaged</span>` : ""}</div>
          <div class="card-foot"><span>${p.driveIds.map(driveTitle).join(", ")}</span><button class="btn small">Open</button></div></article>`;
      }).join("")}</div>`;
  }
  $("#new-project").onclick = () => newProjectDialog([]);
}

async function loadProject(id, refresh) {
  try { S.projectData = await api(`/api/projects/${id}`); }
  catch (e) { toast(e.message, true); S.project = null; S.projectData = null; return render(); }
  if (S.project === id) renderProjectPage();
}

function renderProjectPage() {
  const main = $("#main");
  const data = S.projectData;
  if (!data || data.project.id !== S.project) {
    main.innerHTML = `<div class="empty"><p>Loading…</p></div>`;
    loadProject(S.project);
    return;
  }
  const keepScroll = main.scrollTop;
  const p = data.project, h = data.health;
  const bad = h.soleBytes + h.shortBytes;
  const inProject = new Set(p.driveIds);
  const others = (S.state.drives || []).filter((d) => !inProject.has(d.id));
  const byId = Object.fromEntries(h.drives.map((d) => [d.id, d]));
  const connected = (id) => { const d = driveOf(id); return d && d.connected; };

  main.innerHTML = `
    <div class="phead">
      <button class="icon-btn" id="proj-back" title="All projects">${ICON.back}</button>
      <span class="dot" style="width:12px;height:12px;background:${COLORS[p.color] || "var(--accent)"}"></span>
      <input class="inline-input" id="proj-name" value="${esc(p.name)}" title="Rename">
      <label class="hint">Copies wanted <select class="plain" id="proj-target">${[1, 2, 3, 4].map((n) => `<option value="${n}" ${p.target === n ? "selected" : ""}>${n}</option>`).join("")}</select></label>
      <div class="swatches">${Object.entries(COLORS).map(([k, c]) => `<button class="swatch ${p.color === k ? "on" : ""}" data-pcolor="${k}" style="background:${c}" title="${k}"></button>`).join("")}</div>
      <div class="right"><button class="btn danger" id="proj-delete">Delete project</button></div>
    </div>

    <div class="pgrid">
    <div>
      <section class="panel">
        <h2>Health</h2>
        <p class="hint">Every distinct file should be on at least ${p.target} different drive${p.target === 1 ? "" : "s"}. Files are matched by checksum, so the same clip in different folders counts as one.</p>
        ${h.bytes ? ratioBar(healthParts(h), h.bytes, true) : `<p class="hint">No files recorded on these drives yet.</p>`}
        <div class="stat-row" style="margin-top:14px">
          <div class="stat ${h.bytes && !bad ? "ok" : ""}"><div class="v">${h.bytes ? Math.round((h.backedBytes / h.bytes) * 100) : 0}%</div><div class="l">backed up (${bytes(h.backedBytes)}, ${h.backed.toLocaleString()} files)</div></div>
          <div class="stat ${h.soleBytes ? "bad" : "dim"}"><div class="v">${bytes(h.soleBytes)}</div><div class="l">sole copies (${h.sole.toLocaleString()} files)</div></div>
          ${p.target > 2 ? `<div class="stat ${h.shortBytes ? "warn" : "dim"}"><div class="v">${bytes(h.shortBytes)}</div><div class="l">fewer than ${p.target} copies (${h.short.toLocaleString()} files)</div></div>` : ""}
          <div class="stat ${h.unknownBytes ? "warn" : "dim"}"><div class="v">${bytes(h.unknownBytes)}</div><div class="l">no checksum yet (${h.unknown.toLocaleString()} files)</div></div>
          <div class="stat ${h.damaged ? "bad" : "dim"}"><div class="v">${h.damaged}</div><div class="l">damaged copies</div></div>
        </div>
      </section>

      <section class="panel">
        <h2>Plan to reach ${p.target} copies</h2>
        <p class="hint">Each step copies only the files that need another copy, from one drive to another. Plug both in and run it. Files keep their paths inside a folder named after the source drive. ${bytes(data.reserve)} is kept free on every drive.</p>
        ${!h.needs && !bad ? "" : ""}
        ${data.transfers.length === 0 && !data.unplacedBytes ? `<div class="alert ok">${h.bytes ? `<b>Nothing to do.</b> Every checksummed file is on ${p.target} drives.` : "<b>Nothing to plan yet.</b> Add drives with files on them."}</div>` : ""}
        ${data.transfers.map((t) => {
          const ready = t.ready && !t.running && !t.busy;
          const need = [t.sourceId, t.destId].filter((id) => !connected(id)).map(driveTitle);
          return `<div class="step-card" data-transfer="${esc(t.sourceId)}|${esc(t.destId)}">
            <div class="step-head"><span class="dot ${connected(t.sourceId) ? "live" : ""}" style="background:${connected(t.sourceId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.sourceId))}</b><span class="arrow">${ICON.arrow}</span>
              <span class="dot ${connected(t.destId) ? "live" : ""}" style="background:${connected(t.destId) ? "var(--ok)" : "var(--muted)"}"></span><b>${esc(driveTitle(t.destId))}</b>
              <span class="hint">${bytes(t.bytes)} · ${t.fileCount.toLocaleString()} file${t.fileCount === 1 ? "" : "s"}</span>
              <div class="right">${t.running ? `<span class="badge info">${ICON.clock}Running</span>` : t.busy ? `<span class="badge">${ICON.clock}Drive busy</span>` : ready ? `<button class="btn go small" data-run="${esc(t.sourceId)}|${esc(t.destId)}">${ICON.play}Run this step</button>` : `<span class="badge">${ICON.plug}Plug in ${esc(need.join(" and "))}</span>`}</div></div>
            <div class="chips">${t.folderList.slice(0, 8).map((f) => `<span class="chip">${esc(f.name)}<small>${bytes(f.bytes)}</small></span>`).join("")}${t.folderList.length > 8 ? `<span class="chip">+${t.folderList.length - 8} more</span>` : ""}</div>
            ${ready ? `<div class="step-run"><span class="hint">Copy into folder</span><input class="path-input" data-folder="${esc(t.sourceId)}|${esc(t.destId)}" value="${esc(driveTitle(t.sourceId))}" spellcheck="false"><span class="hint">on ${esc(driveTitle(t.destId))}</span></div>` : ""}
          </div>`;
        }).join("")}
        ${data.unplacedBytes ? `<div class="alert bad" style="margin-top:10px"><b>${bytes(data.unplacedBytes)} has nowhere to go.</b> No drive in the project has room for these files (each needs a drive that doesn't already hold it). Add a drive with at least ${bytes(data.unplacedBytes + data.reserve)} free to the project, or free up space.
          <div class="chips">${data.unplaced.map((g) => `<span class="chip">${esc(driveTitle(g.driveId))} / ${esc(g.folder)}<small>${bytes(g.bytes)}</small></span>`).join("")}</div></div>` : ""}
      </section>

      ${data.sole.length ? `<section class="panel"><h2>Files with too few copies</h2><p class="hint">Where the ${bytes(bad)} that isn't backed up enough lives, by drive and top-level folder.</p>
        <table class="list compact"><thead><tr><th>Drive</th><th>Folder</th><th class="num">Files</th><th class="num">Size</th></tr></thead><tbody>
        ${data.sole.map((g) => `<tr><td><div class="name" style="font-weight:500"><span class="dot" style="background:${connected(g.driveId) ? "var(--ok)" : "var(--muted)"}"></span>${esc(driveTitle(g.driveId))}</div></td><td class="mono">${esc(g.folder)}</td><td class="num">${g.files.toLocaleString()}</td><td class="num" style="color:var(--bad)">${bytes(g.bytes)}</td></tr>`).join("")}</tbody></table></section>` : ""}

      ${data.unknown.length ? `<section class="panel"><h2>Files with no checksum</h2><p class="hint">These can't be counted as copies of anything until they're checksummed. Plug the drive in and checksum its new files.</p>
        <table class="list compact"><thead><tr><th>Drive</th><th>Folder</th><th class="num">Files</th><th class="num">Size</th><th></th></tr></thead><tbody>
        ${data.unknown.map((g) => `<tr><td><div class="name" style="font-weight:500"><span class="dot" style="background:${connected(g.driveId) ? "var(--ok)" : "var(--muted)"}"></span>${esc(driveTitle(g.driveId))}</div></td><td class="mono">${esc(g.folder)}</td><td class="num">${g.files.toLocaleString()}</td><td class="num" style="color:var(--warn)">${bytes(g.bytes)}</td>
          <td class="num">${connected(g.driveId) ? `<button class="btn small" data-hash="${esc(g.driveId)}" ${driveOf(g.driveId).busy ? "disabled" : ""}>Checksum now</button>` : `<span class="hint">not plugged in</span>`}</td></tr>`).join("")}</tbody></table></section>` : ""}
    </div>

    <div>
      <section class="panel">
        <h2>Drives in this project</h2>
        <p class="hint">${p.driveIds.length} drive${p.driveIds.length === 1 ? "" : "s"}. Each bar shows what the drive holds: backed up elsewhere in the project, sole copies, or unproven.</p>
        ${p.driveIds.map((id) => {
          const d = driveOf(id), dh = byId[id] || {};
          if (!d) return "";
          const used = Math.max(0, d.size - d.free);
          return `<div class="step-card" style="margin-bottom:10px" data-drive="${esc(id)}">
            <div class="step-head"><span class="dot ${d.connected ? "live" : ""}" style="background:${d.connected ? "var(--ok)" : COLORS[d.labels.color] || "var(--muted)"}"></span><b>${esc(d.title)}</b><span class="hint">${d.connected ? "plugged in" : "last seen " + new Date(d.lastSeen * 1000).toLocaleDateString()}</span>
              <div class="right"><button class="btn small" data-act="open">Details</button><button class="icon-btn" data-remove-drive="${esc(id)}" title="Remove from project">${ICON.x}</button></div></div>
            ${ratioBar({ backed: dh.backedBytes || 0, short: dh.shortBytes || 0, sole: dh.soleBytes || 0, unknown: dh.unknownBytes || 0 }, dh.bytes || 1, false)}
            <div class="cap">${dh.soleBytes ? `<span style="color:var(--bad)">${bytes(dh.soleBytes)} sole copies</span>` : `<span style="color:var(--ok)">no sole copies</span>`}<span class="sep">•</span>${bytes(dh.bytes || 0)} in project${dh.extraBytes ? `<span class="sep">•</span><span class="hint">${bytes(dh.extraBytes)} on more drives than needed</span>` : ""}</div>
            <div class="bar ${capClass(used, d.size)}"><i style="width:${d.size ? ((used / d.size) * 100).toFixed(1) : 0}%"></i></div>
            <div class="cap">${bytes(d.size)}<span class="sep">•</span>Free ${bytes(d.free)}</div></div>`;
        }).join("")}
        ${others.length ? `<div class="step-run" style="margin-top:6px"><select class="plain" id="proj-add"><option value="">Add a drive…</option>${others.map((d) => `<option value="${esc(d.id)}">${esc(d.title)}${d.connected ? " (plugged in)" : ""}</option>`).join("")}</select></div>` : ""}
      </section>
      <section class="panel"><h2>Notes</h2><textarea class="path-input" id="proj-notes" rows="4" placeholder="Where the drives live, who has what, what's still to come…">${esc(p.notes)}</textarea></section>
    </div>
    </div>`;
  main.scrollTop = keepScroll;
  wireProjectPage(p);
}

function wireProjectPage(p) {
  const patch = async (body, then) => {
    try { await api(`/api/projects/${p.id}`, { method: "PATCH", body }); await loadState(); await loadProject(p.id); if (then) then(); }
    catch (e) { toast(e.message, true); }
  };
  $("#proj-back").onclick = () => { S.project = null; render(); };
  $("#proj-name").onchange = (e) => patch({ name: e.target.value.trim() || p.name });
  $("#proj-target").onchange = (e) => patch({ target: +e.target.value });
  $("#proj-notes").onchange = (e) => patch({ notes: e.target.value });
  for (const el of document.querySelectorAll("[data-pcolor]")) el.onclick = () => patch({ color: el.dataset.pcolor });
  const add = $("#proj-add");
  if (add) add.onchange = () => { if (add.value) patch({ add: [add.value] }); };
  for (const el of document.querySelectorAll("[data-remove-drive]")) el.onclick = (e) => { e.stopPropagation(); patch({ remove: [el.dataset.removeDrive] }); };
  $("#proj-delete").onclick = () => {
    const box = document.createElement("div");
    box.className = "confirm";
    box.innerHTML = `<div><b>Delete ${esc(p.name)}?</b><br>The drives and everything recorded about them stay in the library.</div><div class="confirm-actions"><button class="btn" id="pd-no">Cancel</button><button class="btn danger-fill" id="pd-yes">Delete</button></div>`;
    $(".phead").after(box);
    $("#pd-no").onclick = () => box.remove();
    $("#pd-yes").onclick = async () => { try { await api(`/api/projects/${p.id}`, { method: "DELETE" }); S.project = null; await loadState(); toast("Project deleted"); } catch (e) { toast(e.message, true); } };
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
}

function newProjectDialog(driveIds) {
  const drives = S.state.drives || [];
  openSheet(`<div class="sheet-head"><h2>New project</h2><div class="actions"><button class="icon-btn" id="np-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:14px">
      <div class="fields"><div class="field"><label>Name</label><input id="np-name" placeholder="Feature film, Season 2, …" autofocus></div>
        <div class="field"><label>Copies wanted of every file</label><select class="plain" id="np-target"><option value="2" selected>2 (RAID 1-style)</option><option value="3">3</option><option value="1">1 (just track it)</option></select></div></div>
      <div class="sec"><div class="sec-title">Drives</div><div class="box checks">${drives.length ? drives.map((d) => `<label><input type="checkbox" data-np-drive="${esc(d.id)}" ${driveIds.includes(d.id) ? "checked" : ""}><span class="dot" style="background:${d.connected ? "var(--ok)" : COLORS[d.labels.color] || "var(--muted)"}"></span>${esc(d.title)}<span class="hint">${bytes(d.size)}</span></label>`).join("") : '<span class="hint">No drives logged yet.</span>'}</div></div>
    </div>
    <div class="sheet-foot"><button class="btn" id="np-cancel">Cancel</button><button class="btn primary" id="np-ok">Create</button></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  $("#np-close").onclick = $("#np-cancel").onclick = close;
  $("#np-ok").onclick = async () => {
    const ids = [...document.querySelectorAll("[data-np-drive]:checked")].map((el) => el.dataset.npDrive);
    try {
      const p = await api("/api/projects", { method: "POST", body: { name: $("#np-name").value, target: +$("#np-target").value, driveIds: ids } });
      close();
      S.selecting = false; S.selected.clear();
      S.tab = "projects"; S.project = p.id; localSet("tab", S.tab);
      await loadState();
    } catch (e) { toast(e.message, true); }
  };
  setTimeout(() => $("#np-name").focus(), 30);
}

function addToProjectDialog(driveIds) {
  const ps = S.state.projects || [];
  if (!ps.length) return newProjectDialog(driveIds);
  openSheet(`<div class="sheet-head"><h2>Add ${driveIds.length} drive${driveIds.length === 1 ? "" : "s"} to a project</h2><div class="actions"><button class="icon-btn" id="ap-close">${ICON.x}</button></div></div>
    <div class="sheet-body" style="padding-top:14px"><div class="picker-list">${ps.map((p) => `<button data-ap="${p.id}"><span class="dot" style="background:${COLORS[p.color] || "var(--accent)"}"></span>${esc(p.name)}<span class="hint">${p.driveIds.length} drives</span></button>`).join("")}
      <button id="ap-new">${ICON.plus}New project…</button></div></div>`, true);
  const close = () => { $("#sheet").hidden = true; $("#scrim").hidden = true; };
  $("#ap-close").onclick = close;
  $("#ap-new").onclick = () => { close(); newProjectDialog(driveIds); };
  for (const el of document.querySelectorAll("[data-ap]")) el.onclick = async () => {
    try { await api(`/api/projects/${el.dataset.ap}`, { method: "PATCH", body: { add: driveIds } }); close(); S.selecting = false; S.selected.clear(); toast("Added"); await loadState(); }
    catch (e) { toast(e.message, true); }
  };
}

// ---------- offload ----------
function renderOffload() {
  const o = S.offload;
  const vols = S.state.volumes;
  const chips = (target) => vols.map((v) => `<button class="chip" data-chip="${target}" data-path="${esc(v.mount)}">${esc(v.name)}<small>${bytes(v.free)} free</small></button>`).join("");
  const name = o.name || (o.source ? o.source.replace(/\/+$/, "").split("/").pop() : "");
  const busy = S.jobs.some((j) => j.kind === "offload" && j.state === "running");
  $("#main").innerHTML = `<div class="offload split ${busy ? "busy" : ""}">
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
        <label class="check-row"><input type="checkbox" id="source-check" ${o.sourceCheck ? "checked" : ""}><span>Read the card a second time while verifying<span class="note">Catches a faulty card or reader. Recommended; it runs alongside the copy checks.</span></span></label>
        <div class="go-row"><button class="btn primary" id="start-offload">${ICON.copy}Start offload</button>
          <span class="note" id="offload-note">${o.source && o.dests.some(Boolean) ? `Copies to ${o.dests.filter(Boolean).map((d) => esc(d.replace(/\/+$/, "") + "/" + (name || ""))).join(", ")}` : ""}</span></div></div></div>
    </section>
    <div class="jobs-col"><div class="section-head"><h2>Offloads</h2></div><div class="jobs" id="jobs"></div></div>
  </div>`;
  wireOffload();
  renderJobsInto();
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
      S.offload = { source: "", dests: [""], name: "", sourceCheck: o.sourceCheck };
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
      <div class="term"><div class="term-bar"><i></i><i></i><i></i><span class="term-title"></span><button class="term-copy" title="Copy the log">${ICON.copy}</button></div>
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
  const wantCancel = running ? "1" : "";
  if (actions.dataset.cancel !== wantCancel) {
    actions.dataset.cancel = wantCancel;
    actions.innerHTML = running ? `<button class="btn small" data-cancel="${j.id}">${ICON.x}Cancel</button>` : "";
    const b = actions.querySelector("button");
    if (b) b.onclick = async () => { try { await api(`/api/jobs/${j.id}/cancel`, { method: "POST" }); } catch (e) { toast(e.message, true); } };
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

function renderJobsInto() {
  const box = $("#jobs");
  if (!box) return;
  const list = S.tab === "offload" ? S.jobs.filter((j) => j.kind === "offload") : S.jobs;
  if (!list.length) {
    box.innerHTML = `<div class="empty" style="margin:30px auto">${ICON.activity}<p>${S.tab === "offload" ? "No offloads yet." : "Nothing has run yet. Offloads, drive checks and rescans show their progress here."}</p></div>`;
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
    try { await api("/api/catalog", { method: "POST", body: { id: logBtn.dataset.log } }); toast("Logging drive…"); loadJobs(); } catch (err) { toast(err.message, true); }
    return;
  }
  const openP = e.target.closest("[data-open-project]");
  if (openP) { e.stopPropagation(); S.tab = "projects"; S.project = +openP.dataset.openProject; localSet("tab", S.tab); render(); return; }
  if (e.target.id === "sel-add") return addToProjectDialog([...S.selected]);
  if (e.target.id === "sel-new") return newProjectDialog([...S.selected]);
  if (e.target.id === "sel-all") { visibleDrives().filter(matchesQuery).forEach((d) => S.selected.add(d.id)); return render(); }
  if (e.target.id === "sel-none") { S.selected.clear(); return render(); }
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
  if (e.key === "Escape" && !$("#sheet").hidden && S.detail) closeSheet();
  if ((e.metaKey || e.ctrlKey) && e.key === "f") { e.preventDefault(); $("#search").focus(); }
});

loadState();
loadJobs();
setInterval(() => { if (!document.hidden) loadState(); }, 3000);
setInterval(() => { if (!document.hidden) loadJobs(); }, 1000);
