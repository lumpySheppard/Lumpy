"use strict";
// The menu bar panel: what is running, what is plugged in, a few
// numbers, and a way into the app. Lumpy.app shows it in a popover.

const token = new URLSearchParams(location.hash.slice(1)).get("t") || "";
const native = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.lumpy;
const $ = (s) => document.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const pending = new Map();
let nextReq = 1;
window.lumpyNative = { resolve(id, value) { const p = pending.get(id); if (p) { pending.delete(id); p(value); } } };
function nativeCall(action, args = {}) {
  if (!native) return Promise.resolve(null);
  return new Promise((resolve) => { const id = nextReq++; pending.set(id, resolve); native.postMessage({ action, id, ...args }); });
}

async function api(path, opts = {}) {
  const res = await fetch(path, { ...opts, headers: { "X-Lumpy-Token": token, ...(opts.body ? { "Content-Type": "application/json" } : {}) }, body: opts.body ? JSON.stringify(opts.body) : undefined });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}
function bytes(n) { if (!n) return "0 B"; const u = ["B", "KB", "MB", "GB", "TB", "PB"]; let i = 0; while (n >= 1000 && i < u.length - 1) { n /= 1000; i++; } return (i < 2 ? Math.round(n) : n.toFixed(n < 10 ? 2 : 1)) + " " + u[i]; }
function duration(s) { s = Math.max(0, Math.round(s)); if (s < 60) return s + "s"; if (s < 3600) return Math.floor(s / 60) + "m " + (s % 60) + "s"; return Math.floor(s / 3600) + "h " + Math.floor((s % 3600) / 60) + "m"; }
function clock(ts) { return new Date(ts * 1000).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" }); }
const STATE = { running: ["info", "Running"], queued: ["", "Queued"], ok: ["ok", "Done"], cancelled: ["warn", "Cancelled"], error: ["bad", "Error"], failed: ["bad", "Problems"] };

const GLYPH = '<svg class="brand-glyph" viewBox="0 0 32 32"><defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#4a4a52"/><stop offset="1" stop-color="#2a2a30"/></linearGradient></defs><rect x="2" y="2" width="28" height="28" rx="7" fill="url(#bg)"/><rect x="2.5" y="2.5" width="27" height="27" rx="6.5" fill="none" stroke="rgba(255,255,255,.16)"/><g fill="rgba(255,255,255,.34)"><rect x="6" y="7.5" width="9" height="2.6" rx="1.3"/><rect x="6" y="12.3" width="9" height="2.6" rx="1.3"/><rect x="6" y="17.1" width="9" height="2.6" rx="1.3"/><rect x="6" y="21.9" width="9" height="2.6" rx="1.3"/></g><g fill="#3ddc84"><circle cx="17.2" cy="8.8" r=".9"/><circle cx="17.2" cy="13.6" r=".9"/><circle cx="17.2" cy="18.4" r=".9"/><circle cx="17.2" cy="23.2" r=".9"/></g><path d="M21 9v14h5.5" fill="none" stroke="#d4d4dc" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/></svg>';

let state = null, jobs = [], lib = null, settings = null;

// Camera cards (by picture, "no backups here", or camera folders at the
// root) say when they are safe to format: every file checksummed with
// two verified copies elsewhere.
function isCard(d) {
  return (d.labels && (d.labels.icon === "card" || d.labels.noBackups)) || (d.topFolders || []).some((x) => ["DCIM", "PRIVATE", "CLIPS", "XDROOT", "BPAV", "M4ROOT", "AVCHD", "CONTENTS"].includes(x.toUpperCase()));
}
function backupOf(b, d) {
  if (!b || !b.files) return ["", ""];
  const backed = b.enough + b.short, likely = b.likely || 0;
  if (d && isCard(d) && !b.unknown && !likely && !b.none && !d.damaged) {
    if (b.minCopies >= 2) return ["full", "safe to format"];
    if (b.minCopies >= 1) return ["partial", "backed up once"];
  }
  if (backed >= b.files) return ["full", "verified"];
  if (likely && backed + likely >= b.files) return ["likely", "probably"];
  if (!backed && !likely) return ["none", "no backup"];
  return ["partial", `${Math.round((backed / b.files) * 100)}% backed up`];
}

function render() {
  const el = $("#mb");
  if (!state) return;
  const running = jobs.filter((j) => j.state === "running" || j.state === "queued");
  const recent = jobs.filter((j) => j.state !== "running").slice(0, 1);
  const vols = state.drives.filter((d) => d.connected);
  const low = (d) => d.size > 0 && d.free < d.size * 0.1;
  el.innerHTML = `
    <div class="mb-head">${GLYPH}<b>Lumpy</b><span class="hint">${esc(state.version || "")}</span></div>
    <div class="mb-sec mb-jobs">
      ${running.length ? running.map((j) => { const p = j.progress || {}; const pct = p.total ? Math.min(100, (p.done / p.total) * 100) : 0; const eta = p.rate > 0 && p.total > p.done ? duration((p.total - p.done) / p.rate) + " left" : "";
        if (j.state === "queued") { const names = (j.needs || []).map((id) => { const d = state.drives.find((x) => x.id === id); return d ? d.title : id; });
          return `<div class="mb-job"><div class="t"><span>${esc(j.title)}</span><span class="badge">Queued</span></div><div class="s"><span>waits for ${esc(names.join(" and ") || "another job")} to be free</span><button class="btn small" data-cancel="${j.id}">Cancel</button></div></div>`; }
        return `<div class="mb-job"><div class="t"><span>${esc(j.title)}</span><span class="badge">${p.label ? esc(p.label) : "Running"}</span></div>
          <div class="bar"><i style="width:${pct.toFixed(1)}%"></i></div>
          <div class="s"><span><b>${pct.toFixed(0)}%</b></span><span>${bytes(p.done || 0)} of ${bytes(p.total || 0)}</span><span>${bytes(p.rate || 0)}/s</span><span>${eta}</span><button class="btn small" data-cancel="${j.id}">Cancel</button></div>
          ${p.current ? `<div class="cur">${esc(p.current)}</div>` : ""}</div>`; }).join("")
      : recent.length ? recent.map((j) => { const [cls, label] = STATE[j.state] || ["", j.state]; return `<div class="mb-job"><div class="t"><span>${esc(j.title)}</span><span class="badge ${cls}">${label}${j.failures ? " · " + j.failures : ""}</span></div><div class="s"><span>${clock(j.ended || j.started)}</span>${j.params ? `<button class="btn small" data-again="${j.id}">Run again</button>` : ""}</div></div>`; }).join("")
      : `<div class="mb-empty">Nothing running</div>`}
    </div>
    <div class="mb-sec mb-stats"><span><b>${vols.length}</b> plugged in</span><i>·</i><span><b>${state.drives.length}</b> in the library</span><i>·</i><span><b>${lib ? lib.media.toLocaleString() : "…"}</b> clips</span></div>
    ${vols.length ? `<div class="mb-sec mb-drives">${vols.slice(0, 8).map((d) => { const [st, label] = backupOf(d.backups, d); return `<div class="mb-drive"><span class="sdot ${st || "none"}" title="${esc(label)}"></span><span class="n">${esc(d.title)}<small>${esc(label)}</small></span><span class="f ${low(d) ? "low" : ""}">${bytes(d.free)} free</span></div>`; }).join("")}${vols.length > 8 ? `<div class="mb-empty">and ${vols.length - 8} more</div>` : ""}</div>` : ""}
    <div class="mb-foot">
      <span class="hint">${settings && settings.last && settings.last.ok ? "Status page · " + clock(Math.floor(new Date(settings.last.at).getTime() / 1000)) : state.agent && state.agent.startsWith("installed and running") ? "Auto-logging on" : "Logging while open"}</span>
      <button class="btn small" id="quit">Quit</button>
      <button class="btn small primary" id="open">Open Lumpy</button>
    </div>`;
  $("#open").onclick = () => nativeCall("openApp");
  $("#quit").onclick = () => nativeCall("quit");
  for (const b of el.querySelectorAll("[data-cancel]")) b.onclick = async () => { try { await api(`/api/jobs/${b.dataset.cancel}/cancel`, { method: "POST" }); } catch {} };
  for (const b of el.querySelectorAll("[data-again]")) b.onclick = () => nativeCall("openApp", { again: +b.dataset.again });
  nativeCall("resize", { height: Math.ceil(el.getBoundingClientRect().height) });
}

async function refresh() {
  try {
    [state, jobs] = await Promise.all([api("/api/state"), api("/api/jobs")]);
    render();
  } catch (e) {
    $("#mb").innerHTML = `<div class="mb-empty">Can't reach Lumpy: ${esc(e.message)}</div>`;
  }
}
async function refreshSlow() {
  try { [lib, settings] = await Promise.all([api("/api/library"), api("/api/settings")]); render(); } catch {}
}
refresh();
refreshSlow();
setInterval(refresh, 2000);
setInterval(refreshSlow, 15000);
