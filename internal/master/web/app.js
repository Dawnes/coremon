// Coremon website: plain JavaScript, no build step, no third-party code.
class Raw { constructor(s) { this.s = s; } }
const raw = (s) => new Raw(s);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const r = (v) => (v == null || v === false ? '' : v instanceof Raw ? v.s : Array.isArray(v) ? v.map(r).join('') : esc(v));
function html(strings, ...vals) {
  let out = '';
  strings.forEach((s, i) => { out += s; if (i < vals.length) out += r(vals[i]); });
  return raw(out);
}
function setHTML(el, content) {
  if (!el) return;
  const s = r(content);
  if (el._last === s) return;
  const keep = [...el.querySelectorAll('[data-keep-scroll]')].map((e) => [e.dataset.keepScroll, e.scrollTop]);
  el.innerHTML = s;
  el._last = s;
  for (const [k, top] of keep) {
    const n = el.querySelector(`[data-keep-scroll="${k}"]`);
    if (n) n.scrollTop = top;
  }
}
const $ = (sel, root = document) => root.querySelector(sel);

const ICONS = {
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  unlock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 7.6-1.7"/>',
  dash: '<rect x="3" y="3" width="7" height="9" rx="2"/><rect x="14" y="3" width="7" height="5" rx="2"/><rect x="14" y="12" width="7" height="9" rx="2"/><rect x="3" y="16" width="7" height="5" rx="2"/>',
  server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  jobs: '<path d="M4 6h16M4 12h10M4 18h7"/><circle cx="18" cy="17" r="3"/>',
  box: '<path d="m21 8-9-5-9 5 9 5 9-5Z"/><path d="M3 8v8l9 5 9-5V8"/><path d="M12 13v8"/>',
  shield: '<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6l-8-3Z"/>',
  cog: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z"/>',
  refresh: '<path d="M21 12a9 9 0 0 1-15.5 6.2L3 16"/><path d="M3 12a9 9 0 0 1 15.5-6.2L21 8"/><path d="M21 3v5h-5M3 21v-5h5"/>',
  download: '<path d="M12 3v12m0 0 5-5m-5 5-5-5"/><path d="M4 21h16"/>',
  zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8Z"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  pause: '<rect x="6" y="5" width="4" height="14" rx="1"/><rect x="14" y="5" width="4" height="14" rx="1"/>',
  play: '<path d="m7 4 13 8-13 8V4Z"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16v4Z"/>',
  back: '<path d="M15 18 9 12l6-6"/>',
  logout: '<path d="M15 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4M10 17l5-5-5-5M15 12H3"/>',
  check: '<path d="m5 12 5 5L20 7"/>',
  up: '<path d="M12 19V5m0 0-6 6m6-6 6 6"/>',
  down: '<path d="M12 5v14m0 0 6-6m-6 6-6-6"/>',
  grip: '<circle cx="9" cy="6" r="1.2"/><circle cx="15" cy="6" r="1.2"/><circle cx="9" cy="12" r="1.2"/><circle cx="15" cy="12" r="1.2"/><circle cx="9" cy="18" r="1.2"/><circle cx="15" cy="18" r="1.2"/>',
  chev: '<path d="m6 9 6 6 6-6"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  pulse: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.01"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
};
const icon = (n) => raw(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[n] || ''}</svg>`);

const now = () => Date.now() / 1000;
const p2 = (n) => String(n).padStart(2, '0');
function fmtDate(ts, seconds = false) {
  if (!ts) return '—';
  const d = typeof ts === 'string' ? new Date(ts) : new Date(ts * 1000);
  if (isNaN(d)) return '—';
  const t = `${p2(d.getHours())}:${p2(d.getMinutes())}${seconds ? ':' + p2(d.getSeconds()) : ''}`;
  return `${p2(d.getDate())}-${p2(d.getMonth() + 1)}-${d.getFullYear()} ${t}`;
}
function ago(ts) {
  if (!ts) return 'never';
  const d = Math.max(0, now() - ts);
  if (d < 45) return 'just now';
  if (d < 3600) return `${Math.round(d / 60)}m ago`;
  if (d < 86400) return `${Math.round(d / 3600)}h ago`;
  return `${Math.round(d / 86400)}d ago`;
}
const when = (ts, prefix = '') => (ts ? html`<span class="when" title="${fmtDate(ts, true)}">${prefix}${ago(ts)}</span>` : html`<span>${prefix}never</span>`);
function dur(s) {
  s = Math.floor(s || 0);
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
}
function bytes(n) {
  if (!n) return '—';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(n < 10 && i ? 1 : 0)} ${u[i]}`;
}

async function api(method, path, body) {
  const res = await fetch('/api/ui/' + path, {
    method,
    headers: { 'X-Coremon': '1', ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (res.status === 401 && !['login', 'login/code', 'password'].includes(path) && !path.startsWith('account/2fa/')) {
    showLogin();
    throw new Error('Session expired');
  }
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return (res.headers.get('content-type') || '').includes('json') ? res.json() : null;
}
const GET = (p) => api('GET', p);
const POST = (p, b) => api('POST', p, b || {});

const TOAST = { gap: 14, visible: 3, max: 10, ms: { '': 4000, ok: 4000, err: 8000 } };
const TOAST_ICON = {
  ok: '<circle cx="10" cy="10" r="8" fill="currentColor"/><path d="m6.6 10.3 2.3 2.3 4.6-4.9" fill="none" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>',
  err: '<circle cx="10" cy="10" r="8" fill="currentColor"/><path d="M10 6v4.6M10 13.6v.1" fill="none" stroke-width="1.9" stroke-linecap="round"/>',
};
const toastState = { list: [], open: false, wired: false };

function layoutToasts() {
  const box = $('#toasts');
  const { list, open } = toastState;
  const frontH = list[0] ? list[0].h : 0;
  let y = 0;
  list.forEach((t, i) => {
    const base = open ? `translateY(${-y}px)` : `translateY(${-i * TOAST.gap}px) scale(${1 - i * 0.05})`;
    t.base = base;
    if (!t.el.classList.contains('swiping')) t.el.style.transform = base;
    t.el.style.height = (open || i === 0 ? t.h : frontH) + 'px';
    t.el.style.zIndex = String(TOAST.max - i);
    t.el.style.opacity = i < TOAST.visible ? '' : '0';
    t.el.style.pointerEvents = i < TOAST.visible ? '' : 'none';
    t.el.classList.toggle('behind', !open && i > 0);
    if (i < TOAST.visible) y += t.h + TOAST.gap;
  });
  box.style.height = (open ? Math.max(0, y - TOAST.gap) : frontH) + 'px';
  box.classList.toggle('open', open);
}

function toastTimer(t, run) {
  clearTimeout(t.timer);
  if (t.sticky) return; // stays until closed
  if (run) { t.started = Date.now(); t.timer = setTimeout(() => dropToast(t), t.left); }
  else if (t.started) { t.left = Math.max(800, t.left - (Date.now() - t.started)); t.started = 0; }
}
const toastsRun = (run) => toastState.list.forEach((t) => toastTimer(t, run));

function dropToast(t, swiped = false) {
  const i = toastState.list.indexOf(t);
  if (i < 0) return;
  toastState.list.splice(i, 1);
  clearTimeout(t.timer);
  if (swiped) t.el.style.transform = `${t.base} translateX(110%)`;
  t.el.classList.add('leaving');
  setTimeout(() => t.el.remove(), 400);
  if (!toastState.list.length) toastState.open = false;
  layoutToasts();
}

function wireToasts(box) {
  toastState.wired = true;
  box.setAttribute('aria-live', 'polite');
  box.setAttribute('aria-label', 'Notifications');
  const setOpen = (open) => { if (toastState.open === open) return; toastState.open = open; toastsRun(!open); layoutToasts(); };
  box.addEventListener('mouseenter', () => setOpen(true));
  box.addEventListener('mouseleave', () => { if (!box.contains(document.activeElement)) setOpen(false); });
  box.addEventListener('focusout', (e) => { if (!box.contains(e.relatedTarget) && !box.matches(':hover')) setOpen(false); });
  box.addEventListener('focusin', () => setOpen(true));
  document.addEventListener('visibilitychange', () => { if (!toastState.open) toastsRun(!document.hidden); });
}

function swipeToast(t) {
  let x0 = null, dx = 0, t0 = 0;
  t.el.addEventListener('pointerdown', (e) => {
    if (e.button !== 0 || e.target.closest('.tclose')) return;
    x0 = e.clientX; dx = 0; t0 = Date.now();
    t.el.setPointerCapture(e.pointerId);
  });
  t.el.addEventListener('pointermove', (e) => {
    if (x0 === null) return;
    const raw = e.clientX - x0;
    dx = raw > 0 ? raw : raw / 6;
    t.el.classList.add('swiping');
    t.el.style.transform = `${t.base} translateX(${dx}px)`;
    t.el.style.opacity = String(Math.max(0.2, 1 - Math.max(0, dx) / 220));
  });
  const end = () => {
    if (x0 === null) return;
    x0 = null;
    t.el.classList.remove('swiping');
    if (dx > 45 || dx / Math.max(1, Date.now() - t0) > 0.11) return dropToast(t, true);
    t.el.style.opacity = '';
    t.el.style.transform = t.base;
  };
  t.el.addEventListener('pointerup', end);
  t.el.addEventListener('pointercancel', end);
}

// toast shows a message; sticky ones stay until closed. Lines split on \n.
function toast(msg, kind = '', sticky = false) {
  const box = $('#toasts');
  if (!toastState.wired) wireToasts(box);
  const el = document.createElement('div');
  el.className = `toast ${kind} entering`;
  el.setAttribute('role', kind === 'err' ? 'alert' : 'status');
  const ic = TOAST_ICON[kind] ? `<svg class="ti" viewBox="0 0 20 20" aria-hidden="true">${TOAST_ICON[kind]}</svg>` : '';
  el.innerHTML = `<button class="tclose" type="button" aria-label="Close"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg></button>${ic}<div class="tbody"></div>`;
  el.querySelector('.tbody').textContent = msg;
  box.appendChild(el);
  const t = { el, h: el.getBoundingClientRect().height, left: TOAST.ms[kind] ?? 4000, started: 0, timer: 0, base: '', sticky };
  el.querySelector('.tclose').addEventListener('click', () => dropToast(t));
  swipeToast(t);
  toastState.list.unshift(t);
  while (toastState.list.length > TOAST.max) dropToast(toastState.list[toastState.list.length - 1]);
  layoutToasts();
  el.offsetHeight;
  el.classList.remove('entering');
  if (!toastState.open && !document.hidden) toastTimer(t, true);
}
const fail = (e) => toast(e.message || String(e), 'err');

const modals = [];
let modal = null;
function openModal(content, { wide = false, acts = {}, onClose, over = false } = {}) {
  if (!over) closeAllModals();
  const el = document.createElement('div');
  el.className = 'modal-back';
  el.innerHTML = `<div class="modal ${wide ? 'wide' : ''}">${r(content)}</div>`;
  $('#modal-root').appendChild(el);
  const m = { el, acts, onClose };
  modals.push(m);
  modal = m;
  el.addEventListener('mousedown', (e) => { if (e.target === el && modal === m) closeModal(); });
  const f = el.querySelector('[autofocus]');
  if (f) f.focus();
  return m;
}
function closeModal() {
  const m = modals.pop();
  modal = modals[modals.length - 1] || null;
  if (!m) return;
  m.el.remove();
  m.onClose && m.onClose();
}
function closeAllModals() {
  while (modals.length) closeModal();
}
function confirmBox(title, body, { okText = 'Confirm', danger = false } = {}) {
  return new Promise((resolve) => {
    let done = false;
    openModal(html`<h3>${title}</h3><div class="muted" style="margin-top:8px">${body}</div>
      <div class="actions"><button class="btn ghost" data-act="no">Cancel</button>
      <button class="btn ${danger ? 'danger' : 'primary'}" data-act="yes" autofocus>${okText}</button></div>`, {
      over: !!modal,
      acts: { yes: () => { done = true; closeModal(); resolve(true); }, no: () => closeModal() },
      onClose: () => { if (!done) resolve(false); },
    });
  });
}
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeModal(); });

// Searchable dropdown: a button with a hidden input (the form value); it
// opens a list with a search field on top that filters on name and IP,
// in any case. Arrow keys move, Enter picks, Esc closes.
// items: [{ v, label, sub, note, off, find }]; change: a data-change action.
function pick(name, items, cur, { sm = false, change = '', ph = 'Search name or IP…', style = '' } = {}) {
  const sel = items.find((x) => x.v === cur) || items[0];
  return html`<div class="pick ${sm ? 'sm' : ''}" data-pick ${style ? raw(`style="${esc(style)}"`) : ''}>
    <input type="hidden" name="${name}" value="${sel ? sel.v : ''}" ${change ? raw(`data-change="${esc(change)}"`) : ''}>
    <button type="button" class="pick-btn" data-pick-open aria-haspopup="listbox">${pickLabel(sel)}</button>
    <div class="pick-pop" hidden><input type="search" class="pick-q" placeholder="${ph}" autocomplete="off" spellcheck="false">
      <div class="pick-list" role="listbox">${items.map((x) => html`<div class="pick-opt ${x.off ? 'off' : ''} ${x === sel ? 'sel' : ''}" role="option" data-v="${x.v}"
        data-label="${x.label}" data-sub="${x.sub || ''}" data-s="${`${x.label} ${x.sub || ''} ${x.find || ''}`.toLowerCase()}">${x.label}${x.note ? html` <span class="dim small">(${x.note})</span>` : ''}${x.sub ? html`<span class="pick-sub">${x.sub}</span>` : ''}</div>`)}
      <div class="pick-none" hidden>No match</div></div></div></div>`;
}
const pickLabel = (x) => html`<span class="pick-lbl">${x?.label || ''}</span>${x?.sub ? html`<span class="pick-sub">${x.sub}</span>` : ''}${icon('chev')}`;
function pickClose(p, focus = true) {
  if (!p) return;
  p.classList.remove('open');
  $('.pick-pop', p).hidden = true;
  if (focus) $('.pick-btn', p).focus();
}
function pickMove(p, d, start = false) {
  const opts = [...p.querySelectorAll('.pick-opt:not([hidden]):not(.off)')];
  let i = opts.findIndex((o) => o.classList.contains(start ? 'sel' : 'act'));
  p.querySelectorAll('.pick-opt.act').forEach((o) => o.classList.remove('act'));
  if (!opts.length) return;
  i = start ? Math.max(i, 0) : i < 0 ? 0 : (i + d + opts.length) % opts.length;
  opts[i].classList.add('act');
  opts[i].scrollIntoView({ block: 'nearest' });
}
function pickFilter(p) {
  const q = $('.pick-q', p).value.trim().toLowerCase();
  let n = 0;
  p.querySelectorAll('.pick-opt').forEach((o) => { o.hidden = !!q && !o.dataset.s.includes(q); if (!o.hidden) n++; });
  $('.pick-none', p).hidden = n > 0;
  pickMove(p, 0, true);
}
function pickChoose(p, o) {
  if (!o || o.classList.contains('off')) return;
  const inp = $('input[type=hidden]', p), changed = inp.value !== o.dataset.v;
  p.querySelectorAll('.pick-opt.sel').forEach((x) => x.classList.remove('sel'));
  o.classList.add('sel');
  inp.value = o.dataset.v;
  setHTML($('.pick-btn', p), pickLabel({ label: o.dataset.label, sub: o.dataset.sub }));
  pickClose(p);
  if (changed) inp.dispatchEvent(new Event('change', { bubbles: true }));
}
document.addEventListener('click', (e) => {
  const open = document.querySelector('.pick.open');
  const o = e.target.closest('.pick-opt');
  if (o) { pickChoose(o.closest('.pick'), o); return; }
  const b = e.target.closest('[data-pick-open]');
  if (b) {
    const p = b.closest('.pick');
    if (p === open) { pickClose(p); return; }
    pickClose(open, false);
    p.classList.add('open');
    $('.pick-pop', p).hidden = false;
    const q = $('.pick-q', p);
    q.value = '';
    pickFilter(p);
    q.focus();
    return;
  }
  if (open && !open.contains(e.target)) pickClose(open, false);
});
document.addEventListener('input', (e) => { if (e.target.classList?.contains('pick-q')) pickFilter(e.target.closest('.pick')); });
window.addEventListener('keydown', (e) => {
  const p = e.target.closest?.('.pick.open');
  if (!p) return;
  if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); pickClose(p); }
  else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); pickMove(p, e.key === 'ArrowDown' ? 1 : -1); }
  else if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); pickChoose(p, $('.pick-opt.act', p)); }
  else if (e.key === 'Tab') pickClose(p, false);
}, true);

function statusBadge(st) {
  const m = { queued: ['mute', 'queued'], running: ['info', 'running'], success: ['ok', 'success'], failed: ['sec', 'failed'], timeout: ['sec', 'timeout'], cancelled: ['mute', 'cancelled'] };
  const [c, t] = m[st] || ['mute', st];
  return html`<span class="badge ${c}">${st === 'running' ? raw('<span class="spin" style="width:10px;height:10px"></span>') : ''}${t}</span>`;
}
function jobTitle(j) {
  switch (j.type) {
    case 'action': return `${j.feature} · ${j.action}`;
    case 'core.update': return `agent → ${j.artifact ? j.artifact.version : '?'}`;
    case 'agent.uninstall': return 'uninstall agent';
    case 'agent.mode': return /"report_only":true/.test(JSON.stringify(j.params)) ? 'report only' : 'allow changes';
    case 'agent.ping': return 'ping';
    case 'agent.pins': return 'master keys';
    case 'probe.list': return 'send check list';
    case 'probe.clear': return 'clear check list';
    default: return j.type + (j.action ? ' · ' + j.action : '');
  }
}
function jobParams(j) {
  if (j.type !== 'action' || !j.params) return '';
  try {
    const p = typeof j.params === 'string' ? JSON.parse(j.params) : j.params;
    if (p.packages) return `${p.packages.length} package(s): ${p.packages.join(', ')}`;
    const s = JSON.stringify(p);
    return s === '{}' ? '' : s;
  } catch { return ''; }
}
function openJob(id) {
  let offset = 0, timer = null, stopped = false;
  const m = openModal(html`
    <div class="row"><h3 id="jm-title">Job</h3><span id="jm-status"></span><span class="spacer"></span>
    <button class="btn ghost sm" data-act="close">Close</button></div>
    <div class="muted small" id="jm-sub" style="margin:4px 0 14px"></div>
    <div class="term" id="jm-term"></div>
    <div class="row small muted" style="margin-top:10px"><span id="jm-time"></span><span class="spacer"></span><span id="jm-err" class="err"></span></div>`,
  { wide: true, over: !!modal, acts: { close: () => closeModal() }, onClose: () => { stopped = true; clearTimeout(timer); } });
  const term = $('#jm-term', m.el);
  const tick = async () => {
    if (stopped) return;
    try {
      const d = await GET(`jobs/${id}?offset=${offset}`);
      const j = d.job;
      $('#jm-title', m.el).textContent = jobTitle(j);
      setHTML($('#jm-status', m.el), statusBadge(j.status));
      $('#jm-sub', m.el).textContent = `${d.agent_name || j.agent_id} · by ${j.actor} · created ${fmtDate(j.created, true)}${jobParams(j) ? ' · ' + jobParams(j) : ''}`;
      if (d.log) {
        const atBottom = term.scrollHeight - term.scrollTop - term.clientHeight < 40;
        if (term.querySelector('.dim')) term.innerHTML = '';
        for (const line of d.log.split('\n')) {
          if (line === '') continue;
          const span = document.createElement('div');
          if (line.startsWith('$ ')) span.className = 'cmd';
          else if (/^(ERROR|E:)/.test(line)) span.className = 'err';
          span.textContent = line;
          term.appendChild(span);
        }
        offset = d.offset;
        if (atBottom) term.scrollTop = term.scrollHeight;
      }
      const t = j.finished ? `finished ${fmtDate(j.finished, true)} · took ${dur(j.finished - (j.started || j.created))}` : j.started ? `started ${fmtDate(j.started, true)}` : `queued ${fmtDate(j.created, true)} — waiting for agent`;
      $('#jm-time', m.el).textContent = t;
      $('#jm-err', m.el).textContent = j.error || '';
      if (!term.childElementCount && j.status === 'queued') term.innerHTML = '<span class="dim">Waiting for the agent to pick up this job…</span>';
      if (j.status === 'queued' || j.status === 'running') timer = setTimeout(tick, 900);
      else { view?.refresh?.(); }
    } catch (e) { if (!stopped) timer = setTimeout(tick, 2500); }
  };
  tick();
}

let view = null;
const globalActs = {
  'open-job': (t) => openJob(t.dataset.id),
  logout: async () => { await POST('logout').catch(() => {}); navOpen.reset(); showLogin(); },
  'nav-fold': (t) => {
    const sec = t.dataset.sec;
    if (matchMedia('(max-width: 860px)').matches || !SUBNAV[sec]) { location.hash = '#/' + sec; return; }
    const open = navOpen.get();
    if (open.has(sec)) open.delete(sec); else open.add(sec);
    navOpen.set(open);
    t.classList.toggle('shut', !open.has(sec));
    t.setAttribute('aria-expanded', String(open.has(sec)));
    document.querySelector(`.subnav[data-sec="${sec}"]`)?.classList.toggle('folded', !open.has(sec));
  },
};
document.addEventListener('click', (e) => {
  const t = e.target.closest('[data-act]');
  if (!t) return;
  const name = t.dataset.act;
  const inModal = modal && modal.el.contains(t);
  const h = (inModal && modal.acts[name]) || (!inModal && view?.acts?.[name]) || globalActs[name];
  if (!h) return;
  if (t.tagName !== 'INPUT' && t.tagName !== 'SELECT') e.preventDefault();
  e.stopPropagation();
  Promise.resolve(h(t, e)).catch(fail);
});
document.addEventListener('change', (e) => {
  const t = e.target.closest('[data-change]');
  if (!t) return;
  const name = t.dataset.change;
  const inModal = modal && modal.el.contains(t);
  const h = (inModal && modal.acts[name]) || view?.acts?.[name];
  if (h) Promise.resolve(h(t, e)).catch(fail);
});
document.addEventListener('submit', (e) => {
  const f = e.target.closest('form[data-submit]');
  if (!f) return;
  e.preventDefault();
  const name = f.dataset.submit;
  const inModal = modal && modal.el.contains(f);
  const h = (inModal && modal.acts[name]) || view?.acts?.[name];
  if (h) Promise.resolve(h(f, e)).catch(fail);
});

let session = { version: '', min_password: 5, max_password: 40 };
const pwMin = () => session.min_password || 5, pwMax = () => session.max_password || 40;
const pwRange = () => `${pwMin()}–${pwMax()} characters`;
const canChange = () => !!session.can_change;
const ifChange = (x) => (canChange() ? x : '');
let refreshTimer = null;
let pendingCount = 0;

// Back to the sign-in page: a full reload, which also clears the application code from memory.
function showLogin() {
  clearInterval(refreshTimer);
  view = null;
  location.reload();
}

const NAV = [
  ['dashboard', 'dash', 'Dashboard'],
  ['activity', 'jobs', 'Activity'],
  ['settings', 'cog', 'Settings'],
  ['about', 'info', 'About'],
];
const SUBNAV = {
  dashboard: () => [['overview', 'Overview'], ['devices', 'Device List'], ['apps', 'Application List'], ['map', 'Map']],
  about: () => [['about', 'About'], ['license', 'License']],
  activity: () => [['jobs', 'Jobs'], ['probes', 'Remote Probes'], ['audit', 'Audit log']],
  settings: () => [['general', 'General'], ['devices', 'Devices'], ['upstream', 'Upstream'], ['releases', 'Releases'], ...(canChange() ? [['users', 'Users']] : []), ['account', 'My Account'], ['identity', 'Identity'], ...(canChange() ? [['backups', 'Backups']] : [])],
};
function currentRoute() {
  const p = location.hash.replace(/^#\/?/, '').split('/');
  if (SUBNAV[p[0]]) {
    const items = SUBNAV[p[0]]();
    return [p[0], items.some(([k]) => k === p[1]) ? p[1] : items[0][0]];
  }
  if (p[0] === '') return ['dashboard', 'overview'];
  if (p[0] === 'device') return ['dashboard', lastDashSub];
  return [p[0], ''];
}
const releasedLabel = () => (session.build_date ? 'Released: ' + session.build_date.split('-').reverse().join('-') : '');
const subTabs = (section, cur) => html`<div class="settabs">${SUBNAV[section]().map(([k, l]) => html`<a href="#/${section}/${k}" class="${cur === k ? 'on' : ''}">${l}</a>`)}</div>`;
const settingsTabs = (cur) => subTabs('settings', cur);
const roleLabel = (r) => (r === 'admin' ? 'Administrator' : r === 'viewer' ? 'Read-only' : r);

const NAV_KEY = 'coremon.nav';
let navMem = null;
const navOpen = {
  get() {
    try { const v = JSON.parse(localStorage.getItem(NAV_KEY)); if (Array.isArray(v)) return new Set(v); } catch {}
    return new Set(navMem || ['dashboard']);
  },
  set(s) { navMem = [...s]; try { localStorage.setItem(NAV_KEY, JSON.stringify(navMem)); } catch {} },
  reset() { this.set(new Set(['dashboard'])); },
};
function renderShell() {
  const [active, sub] = currentRoute();
  const open = navOpen.get();
  if (active && SUBNAV[active] && !open.has(active)) { open.add(active); navOpen.set(open); }
  setHTML($('#app'), html`
    <div class="shell">
      <aside class="side">
        <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
        <nav class="nav">${NAV.map(([h, ic, lbl]) => html`<a href="#/${h}" class="${active === h ? 'active' : ''} ${open.has(h) ? '' : 'shut'}" data-act="nav-fold" data-sec="${h}" aria-expanded="${open.has(h)}">${icon(ic)}<span class="lbl">${lbl}</span>${h === 'settings' ? html`<span id="nav-pending-main"></span>` : ''}${SUBNAV[h] ? html`<span class="chev">${icon('down')}</span>` : ''}</a>
          ${SUBNAV[h] ? html`<div class="subnav ${open.has(h) ? '' : 'folded'}" data-sec="${h}">${SUBNAV[h]().map(([k, l]) => html`<a href="#/${h}/${k}" class="${active === h && sub === k ? 'active' : ''}">${l}${h === 'settings' && k === 'devices' ? html`<span id="nav-pending"></span>` : ''}</a>`)}</div>` : ''}`)}</nav>
        <div class="foot"><div class="verinfo"><span class="who">Signed in as <b>${session.user?.name}</b>${canChange() ? '' : html` <span class="badge info">read-only</span>`}</span>
          <span title="Website and master version">Coremon v${session.version}</span>${releasedLabel() ? html`<span class="dim">${releasedLabel()}</span>` : ''}</div>
          <button class="btn ghost sm iconbtn" data-act="logout" title="Sign out">${icon('logout')}</button></div>
      </aside>
      <main class="main" id="view"></main>
    </div>`);
  setPending(pendingCount);
}
function setPending(n) {
  pendingCount = n;
  for (const [id, cls] of [['#nav-pending', 'count'], ['#nav-pending-main', 'count count-main']]) {
    const el = $(id);
    if (el) { el.className = n ? cls : ''; el.textContent = n ? String(n) : ''; el.title = n ? `${n} device${n === 1 ? '' : 's'} waiting for approval` : ''; }
  }
}

function route() {
  closeAllModals();
  clearInterval(refreshTimer);
  if (view?.unmount) view.unmount();
  const parts = location.hash.replace(/^#\/?/, '').split('/');
  const [section, sub] = currentRoute();
  const subViews = {
    settings: { general: SettingsGeneral, devices: Devices, upstream: SettingsUpstream, releases: Releases, users: SettingsUsers, backups: SettingsBackups, account: SettingsAccount, identity: SettingsIdentity },
    activity: { jobs: Jobs, probes: RemoteProbes, audit: Audit },
    dashboard: { overview: Dashboard, devices: DeviceList, apps: AppList, map: MapPage },
    about: { about: AboutPage, license: LicensePage },
  };
  const views = { device: (p) => (/^l_/.test(p[0]) ? LocalPage(p) : /^v_/.test(p[0]) ? VirtualPage(p) : AgentPage(p)) };
  if (parts[0] === 'device' && !/^[alv]_[0-9a-f]{16}$/.test(parts[1] || '')) {
    location.hash = '#/';
    return;
  }
  const V = views[parts[0]] || (subViews[section] && subViews[section][sub]) || Dashboard;
  renderShell();
  view = V(parts.slice(1));
  const el = $('#view');
  view.mount(el);
  if (view.refresh) {
    view.refresh().catch(fail);
    refreshTimer = setInterval(() => { if (!document.hidden) view.refresh().catch(() => {}); }, view.interval || 5000);
  }
}
window.addEventListener('hashchange', () => { if (view && $('#view')) route(); });

async function boot() {
  session = await fetch('/api/ui/session', { headers: { 'X-Coremon': '1' } }).then((r) => r.json());
  if (!session.authenticated || !session.app) return showLogin();
  // The master was updated while this page was open: load the new website.
  if (!import.meta.url.endsWith('/' + session.app)) return location.reload();
  if (session.must_change) return showMustChange();
  route();
}

function showMustChange() {
  clearInterval(refreshTimer);
  closeAllModals();
  setHTML($('#app'), html`
    <div class="login"><div class="panel">
      <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
      <div class="muted">Signed in as <b>${session.user?.name}</b> with a generated password. Choose your own password to continue.</div>
      <form data-submit="change" id="change-form">
        <label class="field">Current (generated) password<input type="password" name="cur" autocomplete="current-password" autofocus required></label>
        <label class="field">New password (${pwRange()})<input type="password" name="new" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
        <label class="field">Repeat new password<input type="password" name="new2" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
        <div class="loginerr" id="change-err" role="alert"></div>
        <button class="btn primary" type="submit">Change password and continue</button>
      </form>
      <div class="hint"><a href="#" data-act="logout">Sign out</a></div>
    </div></div>`);
  view = {
    acts: {
      change: async (f) => {
        const err = $('#change-err');
        err.textContent = '';
        if (f.new.value !== f.new2.value) { err.textContent = 'The new passwords do not match'; return; }
        try {
          await POST('password', { current: f.cur.value, new: f.new.value });
          toast('Password changed', 'ok');
          if (location.hash && location.hash !== '#/') history.replaceState(null, '', location.pathname + location.search + '#/');
          await boot();
        } catch (e) {
          err.textContent = /wrong/i.test(e.message || '') ? 'The current password is wrong' : e.message;
        }
      },
    },
  };
}

const isLocal = (a) => a.monitor === 'local';
// Down: an agent device that stopped reporting, or a local device that stopped answering.
// Unreachable (a.unreach): down, but cut off behind a device above it that
// is down or in maintenance; not a failure of its own.
const isDown = (a) => !isVirtual(a) && !a.maint && !a.unreach && (isLocal(a) ? a.local.checked && !a.local.up : !a.online && a.status !== 'pending');
const unrText = (a) => `unreachable · behind ${a.unreach.name}`;
// The address a device is known by: an agentless device's own, or an agent's first IPv4 (not loopback).
const devIP = (a) => {
  if (!a) return '';
  if (isLocal(a)) return a.local.address;
  const ips = a.facts?.ips || [];
  return ips.find((ip) => ip.includes('.') && !ip.startsWith('127.')) || ips.find((ip) => ip !== '::1') || '';
};
// "Connected via" choices (ov: the overview): Upstream first, then every
// approved and virtual device except the device itself and the devices
// below it (no loops). The device that is the default gateway of most
// agents is marked: probably the internet router.
const isVirtual = (a) => a?.monitor === 'virtual';
// The virtual devices (not Upstream) as list entries for the Overview and
// the Device List: no checks and no agent data.
// withUp: also Upstream, while it is on (the Device List shows it, the Overview does not).
const virtualDevs = (d, withUp = false) => (d.virtuals || []).filter((v) => !v.special || (withUp && d.upstream?.enabled))
  .map((v) => ({ ...v, facts: {}, status: 'active', ...(v.special ? { upstream: d.upstream } : {}) }));
// The last ping round of Upstream (for the Device List).
const upLast = (u) => Math.max(0, ...(u?.addrs || []).map((x) => x.at || 0));
function viaItems(ov, self) {
  const all = [...(ov?.agents || []), ...(ov?.virtuals || [])];
  const below = new Set([self]);
  for (let grew = true; grew;) {
    grew = false;
    for (const a of all) if (a.via && below.has(a.via.id) && !below.has(a.id)) { below.add(a.id); grew = true; }
  }
  const gw = ov?.gateway;
  const list = all.filter((a) => a.status !== 'pending' && !a.special && !below.has(a.id)).sort(byName);
  const up = (ov?.virtuals || []).find((v) => v.special === 'upstream');
  return [...(up && !below.has(up.id) ? [{ v: up.id, label: up.name, sub: 'the internet connection (Settings → Upstream)' }] : []),
    { v: '', label: 'None', sub: "directly on the master's network" },
    ...list.map((a) => ({ v: a.id, label: a.name,
      sub: isVirtual(a) ? 'virtual device' : `${devIP(a)} · ${isLocal(a) ? 'agentless' : 'agent'}${gw?.id === a.id ? ` · default gateway of ${gw.count} device${gw.count === 1 ? '' : 's'}` : ''}`,
      find: (a.facts?.ips || []).join(' ') }))];
}
// "Checked by" choices: the master, and the agents with Remote probe on.
function probeItems(pa, cur, curName, skip = '') {
  const ag = pa.agents.filter((p) => p.id !== skip);
  const items = [{ v: '', label: 'Master', sub: 'the master itself' },
    ...ag.map((p) => ({ v: p.id, label: p.name, sub: p.note ? '' : !p.online ? 'offline' : `${p.checks} of ${pa.max} checks`, note: p.note, off: !!p.note && cur !== p.id }))];
  if (cur && !ag.some((p) => p.id === cur)) items.push({ v: cur, label: curName || cur });
  return items;
}
const VIA_HINT = 'The device it sits behind on the network, such as a switch, a router or the host of a VM. When that device is down or in maintenance and this one fails too, it shows as unreachable instead of down. Its checks keep running.';
// "Connected via core › switch-2" (top down) on a device page, with a change link for admins.
// Disk, memory and CPU load of an agent device as reversed letters D M C:
// green within the limit, red down, grey unknown/paused (all grey in maintenance).
function statLetters(a) {
  const S = !isLocal(a) && a.status !== 'pending' && a.checks?.stats;
  if (!S || !S.length) return '';
  return html`<span class="stl">${S.map((x) => html`<span class="${a.maint ? 'unk' : x.s}">${x.k.toUpperCase()}</span>`)}</span>`;
}
function viaLine(a) {
  const ch = [...(a.via_chain || [])].reverse();
  return html`<div class="sub">Connected via ${ch.length ? ch.map((x, i) => html`${i ? ' › ' : ''}<a href="#/device/${x.id}">${x.name}</a>`) : html`<span class="dim">none</span>`}</div>`;
}
function unrNotice(a) {
  if (!a.unreach) return '';
  return html`<div class="notice unr"><b>Unreachable</b> — it does not answer, and <a href="#/device/${a.unreach.id}"><b>${a.unreach.name}</b></a>, which it is connected via, is down or in maintenance. It is not counted as a failure of its own; its checks keep running.</div>`;
}
// Maintenance: nothing checked or started automatically, no failures shown.
const hhmm = (ts) => new Date(ts * 1000).toTimeString().slice(0, 5);
const maintUntil = (m) => (m.until ? `until ${hhmm(m.until)}` : 'until ended');
const maintText = (m) => `in maintenance since ${hhmm(m.since)} by ${m.by}, ${maintUntil(m)}`;
// During maintenance agent actions need a connected agent (not queued).
const maintBlocked = (a) => !!a.maint && !a.online;
const MAINT_OFF = 'In maintenance and the agent is not connected';
const pctText = (x) => (x >= 1 ? '100%' : x >= 0.999 ? '99.9%' : (Math.floor(x * 1000) / 10).toFixed(1) + '%');
const rttText = (us) => (us < 1000 ? '<1 ms' : us < 10000 ? (us / 1000).toFixed(1) + ' ms' : Math.round(us / 1000) + ' ms');
// The remote probe of an agentless device, when it delivers no results
// (offline, cleared, …): the device is then unknown, not down.
const probeOut = (a) => (isLocal(a) && a.local.probe && a.local.probe.state !== 'ok' ? a.local.probe : null);
const unknownText = (p) => `unknown – ${p.note}`;
// The checks of a device (ping and services): an agentless device's own, or
// those on an agent device's Services tab (null while it has none).
const chk = (a) => (isLocal(a) ? a.local : a.checks || null);
// Ping and services counted for the overview: [total, ok, down].
function checkCounts(L) {
  const P = L.ping_info, out = L.probe && L.probe.state !== 'ok';
  let t = 0, ok = 0, down = 0;
  if (L.ping && P) {
    t++;
    if (!out && P.checked && !P.up) down++;
    else if (!out && P.checked && !P.fails) ok++;
  }
  return [t + L.svc_total, ok + (L.svc_ok || 0), down + L.svc_down];
}
// An agent device's own checks that are down (not in maintenance, with
// results coming in), or null. Its status itself still comes from its agent.
function agentChecksDown(a) {
  const L = a.checks;
  if (isLocal(a) || !L || a.maint || (L.probe && L.probe.state !== 'ok')) return null;
  const ping = !!(L.ping && L.ping_info?.checked && !L.ping_info.up);
  const n = L.svc_down + (ping ? 1 : 0);
  return n ? { n, total: L.svc_total + (L.ping ? 1 : 0), L, ping } : null;
}
const checksDownText = (x) => `${x.n} of ${x.total} ${x.L.ping ? 'check' : 'service'}${x.total === 1 ? '' : 's'} down`;
const checksDownShort = (x) => `${x.n} ${x.L.ping ? 'check' : 'service'}${x.n === 1 ? '' : 's'} down`;
// What failed: the first service that is down and its step, or the ping.
const checksFailText = (x) => (x.L.svc_fail ? `${x.L.svc_fail.name}${x.L.svc_fail.step && x.L.svc_fail.step !== 'connect' ? `: ${stepLabel(x.L.svc_fail.step).toLowerCase()}` : ''}` : x.ping ? 'ping: no answer' : '');
function agentDot(a) {
  if (a.special && a.upstream) {
    const s = a.upstream.state;
    return html`<span class="dot ${s === 'ok' ? 'on' : s === 'partial' ? 'pend' : s === 'down' ? 'off' : 'unk'}" title="${upWord[s] || s}"></span>`;
  }
  if (isVirtual(a)) return a.unreach ? html`<span class="dot unr" title="${unrText(a)}"></span>` : html`<span class="dot virt" title="virtual device"></span>`;
  if (a.maint) return html`<span class="dot maint" title="${maintText(a.maint)}"></span>`;
  if (a.unreach) return html`<span class="dot unr" title="${unrText(a)}"></span>`;
  if (isLocal(a)) {
    const L = a.local, p = probeOut(a);
    if (p) return html`<span class="dot unk" title="${unknownText(p)}"></span>`;
    if (!L.checked) return html`<span class="dot pend" title="${L.fails ? 'no answer yet' : 'waiting for the first ping'}"></span>`;
    return html`<span class="dot ${L.up ? 'on' : 'off'}" title="${L.up ? 'up: answers ping' : 'down since ' + fmtDate(L.since)}"></span>`;
  }
  if (a.status === 'pending') return html`<span class="dot pend" title="pending approval"></span>`;
  if (a.status === 'report_only') return html`<span class="dot pause" title="report only${a.online ? '' : ' · offline'}"></span>`;
  return html`<span class="dot ${a.online ? 'on' : 'off'}" title="${a.online ? 'online' : 'offline since ' + fmtDate(a.last_seen)}"></span>`;
}
function osLabel(f) { return f.os_name || [f.os_id, f.os_version].filter(Boolean).join(' ') || 'unknown OS'; }
// Platform: what the device is besides its OS (agent 0.3.16 and later), e.g. "Proxmox VE 9.0.11".
function platformLabel(f) { return f?.platform ? [f.platform.name, f.platform.version].filter(Boolean).join(' ') : '—'; }
const hasUpdates = (a) => !a.below_min && !isLocal(a);
const ymd = (s) => (s ? s.split('-').reverse().join('-') : '');
function osSupportText(o, osID) {
  if (!o) return '—';
  const esm = o.esm_until ? ` (ESM with Ubuntu Pro until ${ymd(o.esm_until)})` : '';
  switch (o.phase) {
    case 'security': return `security support until ${ymd(o.until)}${o.lts_until ? `, then LTS until ${ymd(o.lts_until)}` : ''}`;
    case 'lts': return `LTS until ${ymd(o.until)}`;
    case 'standard': return `standard support until ${ymd(o.until)}${esm}`;
    case 'ended': return osID === 'ubuntu' ? `standard support ended ${ymd(o.end)}${esm}` : `support ended ${ymd(o.end)}`;
    default: return 'supported, no end date yet';
  }
}
function upgradedBy(by) {
  if (by === 'coremon') return 'Coremon';
  if (by === 'automatic') return 'automatic updates';
  if (by === 'root') return 'root via apt';
  if (by?.startsWith('user:')) return `${by.slice(5)} via apt`;
  return 'apt';
}
function healthBadges(a, { eol = true } = {}) {
  const h = a.facts?.health, o = eol ? a.os_support : null, out = [];
  if (h?.failed_count > 0) out.push(html`<span class="badge sec xs" title="Failed services: ${(h.failed || []).join(', ')}${h.failed_count > (h.failed || []).length ? ', …' : ''}">${h.failed_count} failed</span>`);
  if (o?.status === 'expired') out.push(html`<span class="badge sec xs" title="${osLabel(a.facts)}: ${osSupportText(o, a.facts.os_id)}">EOL</span>`);
  else if (o?.status === 'soon') out.push(html`<span class="badge upd xs" title="${osLabel(a.facts)}: ${osSupportText(o, a.facts.os_id)}">EOL soon</span>`);
  if (h?.auto_updates === 'on') out.push(html`<span class="badge info xs" title="Automatic updates (unattended-upgrades) are on">auto</span>`);
  return out;
}
function ipLabel(f) {
  const ips = f.ips || [];
  if (!ips.length) return html`<span class="dim">—</span>`;
  const first = ips.find((ip) => ip.includes('.')) || ips[0];
  const more = ips.length - 1;
  return html`<span class="ip" title="${ips.join('\n')}">${first}${more ? html` <span class="dim">+${more}</span>` : ''}</span>`;
}
const byName = (a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true });

function verPill(a) {
  if (!a.core_version) return '';
  const latest = a.core_is_latest;
  const tip = a.core_latest ? (latest ? 'latest agent version' : `latest is ${a.core_latest}`) : 'no agent versions on the master yet';
  return html`<span class="verpill ${latest ? 'ok' : 'old'}" title="${tip}">agent ${a.core_version}</span>${a.desired_core ? html`<span class="verpill busy" title="change requested"><span class="spin" style="width:9px;height:9px"></span> → ${a.desired_core}</span>` : ''}`;
}
function updBadges(a) {
  const u = a.updates;
  const out = [];
  if (a.status === 'report_only') out.push(html`<span class="badge info" title="Only reports: no checks, installs or reboots">${icon('pause')} report only</span>`);
  if (u?.security) out.push(html`<span class="badge sec">${u.security} security</span>`);
  const cve = cveBadge(a);
  if (cve) out.push(cve);
  if (u?.reboot_required) out.push(html`<span class="badge reboot">reboot</span>`);
  return out;
}

const SEV = ['unknown', 'low', 'medium', 'high', 'critical'];
const CVE_RED = 'fixed by an update and rated High or Critical, or actively exploited';
function cveBadge(a) {
  const n = a.vulns?.badge;
  return n ? html`<span class="badge sec" title="${n} known vulnerabilit${n === 1 ? 'y' : 'ies'} ${CVE_RED}">${n} CVE</span>` : '';
}
function sevBadge(c) {
  const cls = ['mute', 'mute', 'upd', 'sec', 'sec'][c.level] || 'mute';
  const from = { osv: 'CVSS score from the OSV record', nvd: 'CVSS score from NVD', distro: "the distribution's own rating (no CVSS score yet)" }[c.src] || 'no rating yet';
  return html`<span class="badge ${cls} xs" title="${from}">${SEV[c.level] || 'unknown'}${c.scored ? ' ' + c.score.toFixed(1) : ''}</span>`;
}
function openCves(appName, v) {
  const pct = (x) => (x >= 0.1 ? Math.round(x * 100) + '%' : x >= 0.001 ? (x * 100).toFixed(1) + '%' : '<0.1%');
  const sum = [v.fixable ? `${v.fixable} fixed by an update` : '', v.open ? `${v.open} without a fix yet` : '', v.minor ? `${v.minor} minor` : '']
    .filter(Boolean).join(' · ');
  openModal(html`<h3>${appName}: known vulnerabilities</h3>
    <div class="muted small">${sum}${v.kev ? html` · <b class="c-red">${v.kev} actively exploited</b>` : ''}</div>
    <div class="tbl-wrap" style="margin-top:14px;max-height:60vh"><table class="tbl">
      <thead><tr><th>CVE</th><th>Severity</th><th>Fixed in</th><th title="EPSS: chance of exploitation in the next 30 days">Exploit chance</th></tr></thead>
      <tbody>${v.cves.map((c) => html`<tr class="${c.minor ? 'dimrow' : ''}">
        <td class="nowrap"><a href="${c.link}" target="_blank" rel="noopener noreferrer">${c.id}</a>${c.kev ? html` <span class="badge sec xs" title="On CISA's list of actively exploited vulnerabilities">actively exploited</span>` : ''}${c.minor ? html` <span class="badge mute xs" title="The distribution rates it ${c.word}: not worth fixing">minor</span>` : ''}</td>
        <td>${sevBadge(c)}</td>
        <td class="mono small">${c.fixed ? html`${c.fixed}${c.by_update ? html` <span class="badge ok xs" title="The waiting update installs this version or newer">in waiting update</span>` : ''}` : html`<span class="dim">no fix yet</span>`}</td>
        <td class="small">${c.epss ? html`<span title="Higher than ${Math.round(c.pct * 100)}% of all CVEs">${pct(c.epss)}</span>` : html`<span class="dim">—</span>`}</td></tr>`)}</tbody></table></div>
    <div class="dim small" style="margin-top:12px">Severity: the CVSS score from OSV or NVD, otherwise the distribution's own rating. Dimmed: the distribution considers it minor.
      ${/^Proxmox/.test(appName) ? 'Data: NVD (the CVEs listed for the product and the versions they affect), CISA (actively exploited), FIRST (EPSS). The CVE numbers link to NVD.'
        : 'Data: OSV.dev (Debian\'s and Ubuntu\'s advisories), NVD, CISA (actively exploited), FIRST (EPSS). The CVE numbers link to OSV.dev.'}</div>
    <div class="actions"><button class="btn" data-act="cancel">Close</button></div>`, { wide: true, acts: { cancel: () => closeModal() } });
}
function groupOptions(groups, sel) {
  return html`<option value="" ${!sel ? 'selected' : ''}>Ungrouped</option>${groups.map((g) => html`<option value="${g.id}" ${sel === g.id ? 'selected' : ''}>${g.name}</option>`)}`;
}
function versionOptions(repo, cur) {
  return (repo.core || []).map((v, i) => html`<option value="${v.version}" ${v.version === cur ? 'selected' : ''}>${v.version}${i === 0 ? ' (latest)' : ''}${v.version === cur ? ' — installed' : ''}</option>`);
}

async function runAction(ids, feature, action, params, { open = true } = {}) {
  const res = await POST('actions', { agent_ids: ids, feature, action, params });
  (res.errors || []).forEach((e) => toast(e, 'err'));
  if (res.jobs?.length) {
    toast(`${res.jobs.length} job(s) queued`, 'ok');
    if (open && res.jobs.length === 1) openJob(res.jobs[0]);
  }
  view?.refresh?.();
  return res;
}

async function setAgentVersion(a, version) {
  if (version === a.core_version) return toast(`${a.name} already runs ${version}`);
  const down = a.core_version && versionLess(version, a.core_version);
  let res;
  try { res = await POST(`agents/${a.id}/core`, { version }); }
  catch (e) { throw new Error(`${a.name}: agent update to ${version} not requested: ${e.message || e}`); }
  toast(`${a.name}: agent ${a.core_version} → ${version} requested${res.job ? '' : ' (applies when the device is online)'}`, 'ok');
  if (res.job && down) toast('Downgrade: the agent restarts with the older version');
  view?.refresh?.();
}

function approveModal(a, groups) {
  openModal(html`
    <h3>Approve ${a.name}</h3>
    <div class="muted small">${osLabel(a.facts)} · ${a.facts.arch} · ${(a.facts.ips || []).join(', ')} · agent ${a.core_version}</div>
    <form class="form" data-submit="approve">
      <label class="field">Display name<input type="text" name="name" value="${a.name}" maxlength="64" autofocus></label>
      <label class="field">Group<select name="group">${groupOptions(groups, a.group_id)}</select></label>
      <div class="dim small">The first update check starts right after approval.</div>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary" type="submit">${icon('check')} Approve</button></div>
    </form>`, {
    acts: {
      cancel: () => closeModal(),
      approve: async (f) => {
        await POST(`agents/${a.id}/approve`, { name: f.name.value, group_id: f.group.value });
        closeModal();
        toast(`${f.name.value} approved`, 'ok');
        view?.refresh?.();
      },
    },
  });
}

function layoutOf(data) {
  const lists = new Map(data.groups.map((g) => [g.id, []]));
  lists.set('', []);
  const all = [...data.agents, ...(data.vdev || virtualDevs(data))].sort((x, y) => (x.order ?? 0) - (y.order ?? 0));
  for (const a of all) (lists.get(a.group_id) || lists.get('')).push(a.id);
  return lists;
}
async function saveLayout(groupsOrder, lists) {
  const body = { groups: [...groupsOrder.map((id) => ({ id, agents: lists.get(id) || [] })), { id: '', agents: lists.get('') || [] }] };
  await POST('layout', body);
}

const FILTER_CHIPS = [['all', 'No filter'], ['updates', 'Updates'], ['reboot', 'Reboot'], ['offline', 'Offline'], ['agent', 'Agent outdated']];
const TYPE_TABS = [['all', 'All'], ['agent', 'Agents'], ['local', 'Agentless'], ['virtual', 'Virtual']];
// The device type switch next to the filter chips (never remembered).
const typeTabs = (cur) => TYPE_TABS.map(([k, l]) => html`<button class="${cur === k ? 'on' : ''}" data-act="type" data-t="${k}">${l}</button>`);
function agentMatches(a, f, q, groups, type) {
  const u = a.updates || {};
  // Virtual devices: only under All and Virtual, and never in a filter chip.
  if (isVirtual(a) ? (type !== 'all' && type !== 'virtual') || f !== 'all' : type === 'virtual') return false;
  if (type === 'agent' && (isLocal(a) || isVirtual(a))) return false;
  if (type === 'local' && !isLocal(a)) return false;
  if (f === 'updates' && !(u.total > 0)) return false;
  if (f === 'reboot' && !u.reboot_required) return false;
  if (f === 'offline' && !isDown(a) && !a.unreach) return false;
  if (f === 'agent' && (a.core_is_latest || !a.core_version)) return false;
  if (q) {
    const g = groups.find((x) => x.id === a.group_id);
    const hay = `${a.name} ${isVirtual(a) ? 'virtual' : isLocal(a) ? 'agentless ping' : osLabel(a.facts)} ${(a.facts.ips || []).join(' ')} ${g ? g.name : ''} ${a.core_version || ''}`.toLowerCase();
    if (!hay.includes(q.toLowerCase())) return false;
  }
  return true;
}
function filterChips(agents, cur, test) {
  return FILTER_CHIPS.map(([k, l]) => html`<button class="chip ${cur === k ? 'on' : ''}" data-act="filter" data-f="${k}">${l}<span class="n">${agents.filter((a) => test(a, k)).length}</span></button>`);
}
let lastDashSub = 'overview';

function Dashboard() {
  lastDashSub = 'overview';
  // arrange: the layout is unlocked (admins only). Never stored: every
  // page load starts locked.
  const st = { filter: 'all', type: 'all', q: '', sel: new Set(), data: null, folded: new Set(), drag: null, arrange: false };
  const arranging = () => canChange() && st.arrange;
  const filtering = () => st.filter !== 'all' || st.type !== 'all' || st.q !== '';
  const matches = (a) => agentMatches(a, st.filter, st.q, st.data.groups, st.type);
  function localCard(a) {
    const L = a.local, p = probeOut(a);
    const state = a.maint ? 'maint' : p ? 'unk' : !L.checked ? 'wait' : L.up ? 'up' : a.unreach ? 'unr' : 'down';
    const svcDown = state === 'up' && L.svc_down > 0;
    const accent = svcDown ? 'a-upd' : { maint: 'a-maint', unk: 'a-unk', wait: 'a-pend', up: 'a-ok', down: 'a-off', unr: 'a-unr' }[state];
    const big = { maint: html`<div class="big maint">${icon('pause')}</div><div class="biglbl">in maintenance<br>${a.maint ? maintUntil(a.maint) : ''}</div>`,
      unr: html`<div class="big unr">?</div><div class="biglbl">unreachable<br>behind ${a.unreach?.name || ''}</div>`,
      unk: html`<div class="big dim">?</div><div class="biglbl">unknown<br>${p ? p.note.replace(/:.*/, '') : ''}</div>`,
      wait: L.fails ? html`<div class="big some">?</div><div class="biglbl">no answer yet<br>${L.fails} of ${L.down_after} checks</div>`
      : L.last_error ? html`<div class="big sec" title="${L.last_error}">!</div><div class="biglbl">the master cannot<br>check it</div>`
      : html`<div class="big dim">—</div><div class="biglbl">waiting for<br>first check</div>`,
      up: svcDown ? html`<div class="big some">Up</div><div class="biglbl" title="${L.svc_fail ? `${L.svc_fail.name}: ${L.svc_fail.error || 'down'}` : ''}">${L.svc_down} of ${L.svc_total} service${L.svc_total === 1 ? '' : 's'} down${L.svc_fail ? html`<br>${L.svc_fail.name}${L.svc_fail.step && L.svc_fail.step !== 'connect' ? `: ${stepLabel(L.svc_fail.step).toLowerCase()}` : ''}` : ''}</div>` : html`<div class="big zero">Up</div>`,
      down: html`<div class="big sec">Down</div><div class="biglbl">not<br>reachable</div>` }[state];
    const uptime = L.uptime >= 0 ? html`<span class="badge ${L.uptime >= 0.999 ? 'ok' : L.uptime >= 0.9 ? 'upd' : 'sec'} xs" title="Share of successful checks in the last 24 hours">${pctText(L.uptime)} 24 h</span>` : '';
    return html`
      <div class="card ${accent}" data-act="open-agent" data-id="${a.id}" ${arranging() ? raw('data-drag="agent" draggable="true"') : ''}>
        <div class="top">${arranging() ? html`<span class="grip" title="Drag to reorder or move to another group">${icon('grip')}</span>` : ''}${agentDot(a)}
          <div style="min-width:0"><div class="name">${a.name}</div><div class="os">Agentless${L.probe ? html` · via ${L.probe.name}` : ''}</div><div class="os"><span class="ip">${L.address}</span></div></div></div>
        <div class="mid">${big}<div class="badges">${uptime}</div></div>
        <div class="inforow"><span class="uptime">${icon('clock')}${state === 'maint' ? 'no checks now' : state === 'unk' ? 'no results now' : state === 'wait' ? (L.fails ? 'not decided yet' : 'not checked yet') : `${L.up ? 'up' : state === 'unr' ? 'no answer' : 'down'} ${dur(now() - L.since)}`}</span><span class="spacer"></span>${state === 'up' && L.ping && L.rtt_us ? html`<span class="tag mono">${rttText(L.rtt_us)}</span>` : ''}</div>
        <div class="foot"><span>${L.last_check ? html`${when(L.last_check, 'Services checked ')}.` : 'waiting'}</span></div>
      </div>`;
  }
  // Footer of an online agent tile: when its updates and its services were
  // last checked. Services: the agent's last contact (it is a service itself
  // and brings the disk, memory and CPU readings), or a later outside check.
  function checkedLine(a) {
    const parts = [];
    if (a.updates?.checked_at) parts.push(when(a.updates.checked_at, 'Updates checked '));
    const svc = Math.max(a.last_seen || 0, a.checks?.last_check || 0);
    if (svc) parts.push(when(svc, 'Services checked '));
    return parts.length ? html`${parts.map((p, i) => html`${i ? '. ' : ''}${p}`)}.` : 'online';
  }
  // A virtual device: dashed and neutral, a dash in the middle; only its
  // unreachable state is shown (it is not checked itself).
  function virtualCard(a) {
    return html`
      <div class="card virt ${a.unreach ? 'a-unr' : ''}" data-act="open-agent" data-id="${a.id}" ${arranging() ? raw('data-drag="agent" draggable="true"') : ''}>
        <div class="top">${arranging() ? html`<span class="grip" title="Drag to reorder or move to another group">${icon('grip')}</span>` : ''}${agentDot(a)}
          <div style="min-width:0"><div class="name">${a.name}</div><div class="os">Virtual</div><div class="os">${a.via ? `Connected via ${a.via.name}` : ''}</div></div></div>
        <div class="mid">${a.unreach ? html`<div class="big unr">?</div><div class="biglbl">unreachable<br>behind ${a.unreach.name}</div>` : html`<div class="big dim">—</div>`}</div>
      </div>`;
  }
  function card(a) {
    if (isVirtual(a)) return virtualCard(a);
    if (isLocal(a)) return localCard(a);
    const u = a.updates;
    const f = a.facts;
    let accent = 'a-ok';
    if (a.status === 'pending') accent = 'a-pend';
    else if (a.maint) accent = 'a-maint';
    else if (a.below_min) accent = 'a-upd';
    else if (a.unreach) accent = 'a-unr';
    else if (!a.online) accent = 'a-off';
    else if (u?.security) accent = 'a-sec';
    else if (u?.total) accent = 'a-upd';
    const cdown = a.unreach ? null : agentChecksDown(a);
    if (cdown && accent === 'a-ok') accent = 'a-upd';
    const selectable = a.status === 'active' && !a.below_min && canChange();
    let mid;
    if (a.maint) {
      mid = html`<div class="mid"><div class="big maint">${icon('pause')}</div><div class="biglbl">in maintenance<br>${maintUntil(a.maint)}</div><div class="badges">${updBadges(a)}</div></div>`;
    } else if (a.status === 'pending') {
      mid = html`<div class="mid"><div><div class="biglbl">Waiting for approval</div><div class="dim small">enrolled ${when(a.enrolled_at)}</div></div>
        <div class="badges">${ifChange(html`<button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button>`)}</div></div>`;
    } else if (a.below_min) {
      mid = html`<div class="mid"><div class="big some">↑</div><div class="biglbl">agent update<br>required</div>
        <div class="badges">${ifChange(html`<button class="btn primary sm" data-act="agent-update" data-id="${a.id}" ${a.online ? '' : 'disabled'} title="${a.online ? `Update to agent ${a.core_latest}` : 'The device is offline'}">${icon('up')} Update agent</button>`)}</div></div>`;
    } else if (!u) {
      mid = html`<div class="mid"><div class="big dim">—</div><div class="biglbl">${a.status === 'report_only' ? html`no checks<br>(report only)` : html`waiting for<br>first check`}</div><div class="badges">${updBadges(a)}</div></div>`;
    } else {
      const cls = u.security ? 'sec' : u.total ? 'some' : 'zero';
      mid = html`<div class="mid"><div class="big ${cls}">${u.total}</div><div class="biglbl">${u.total === 1 ? 'update' : 'updates'}<br>available</div><div class="badges">${updBadges(a)}</div></div>`;
    }
    return html`
      <div class="card ${accent} ${st.sel.has(a.id) ? 'sel' : ''}" data-act="open-agent" data-id="${a.id}" ${arranging() ? raw('data-drag="agent" draggable="true"') : ''}>
        <div class="top">${arranging() ? html`<span class="grip" title="Drag to reorder or move to another group">${icon('grip')}</span>` : ''}${agentDot(a)}
          <div style="min-width:0"><div class="name">${a.name}</div>${a.below_min
            ? html`<div class="os">agent ${a.core_version} · lowest allowed ${a.min_agent}</div><div class="os">upgrade only — no data shown</div>`
            : html`<div class="os">${osLabel(f)} · ${f.arch || '?'} · ${f.virt || ''}</div><div class="os">${ipLabel(f)}</div>`}</div>
          ${statLetters(a) || selectable ? html`<div class="tcorner">${statLetters(a)}${selectable ? html`<input type="checkbox" class="selbox" data-act="sel" data-id="${a.id}" ${st.sel.has(a.id) ? 'checked' : ''} title="Select">` : ''}</div>` : ''}</div>
        ${mid}
        ${a.unreach ? html`<div class="svcline unr">${unrText(a)}</div>` : ''}
        ${cdown ? html`<div class="svcline" title="${cdown.L.svc_fail ? `${cdown.L.svc_fail.name}: ${cdown.L.svc_fail.error || 'down'}` : ''}">${checksDownText(cdown)}${checksFailText(cdown) ? ` · ${checksFailText(cdown)}` : ''}</div>` : ''}
        ${a.status !== 'pending' ? html`<div class="inforow"><span class="uptime" title="${a.below_min ? '' : lastReboot(a)}">${icon('clock')}${!a.online ? (a.maint ? 'not connected' : a.unreach ? 'unreachable' : 'offline') :a.below_min ? 'online' : 'up ' + dur(f.uptime_sec)}</span>${a.maint ? '' : healthBadges(a)}<span class="spacer"></span>${verPill(a)}</div>` : ''}
        <div class="foot">
          ${a.busy ? html`<span class="spin"></span><span>${a.busy} job${a.busy > 1 ? 's' : ''}</span>` : html`<span>${a.online ? checkedLine(a) : a.status === 'pending' ? '' : when(a.last_seen, 'seen ')}</span>`}
        </div>
      </div>`;
  }
  function section(g, agents, showHead, dragOnly = false) {
    const list = agents.filter(matches);
    if (dragOnly) {
      return html`<section class="gsec dragonly" data-gid=""><div class="ghead"><span class="gname">Ungrouped</span></div>
        <div class="gbody" data-gid=""><div class="gempty">Drop here to take a device out of its group</div></div></section>`;
    }
    if (filtering() && !list.length) return '';
    const folded = st.folded.has(g.id);
    const active = agents.filter((a) => a.status !== 'pending');
    const upd = active.reduce((s, a) => s + (a.updates?.total || 0), 0);
    const sec = active.reduce((s, a) => s + (a.updates?.security || 0), 0);
    const off = active.filter(isDown).length;
    const unr = active.filter((a) => a.unreach && !a.maint).length;
    const selectable = list.filter((a) => a.status === 'active' && !isLocal(a) && !isVirtual(a));
    const allSel = selectable.length && selectable.every((a) => st.sel.has(a.id));
    const head = showHead ? html`
      <div class="ghead" data-gid="${g.id}" ${g.id && arranging() ? raw('data-drag="group" draggable="true"') : ''}>
        <button class="btn ghost sm iconbtn fold ${folded ? 'shut' : ''}" data-act="fold" data-gid="${g.id}" title="${folded ? 'Unfold' : 'Fold'}">${icon('chev')}</button>
        ${g.id && arranging() ? html`<span class="grip" title="Drag to reorder groups">${icon('grip')}</span>` : ''}
        <span class="gname">${g.name}</span>
        <span class="gsum">${agents.length} device${agents.length === 1 ? '' : 's'}${upd ? html` · <b class="c-amber">${upd} update${upd === 1 ? '' : 's'}</b>` : ''}${sec ? html` · <b class="c-red">${sec} security</b>` : ''}${off ? html` · <b class="c-red">${off} offline</b>` : ''}${unr ? html` · <b class="c-unr">${unr} unreachable</b>` : ''}</span>
        <span class="spacer"></span>
        ${selectable.length && canChange() ? html`<label class="gsel" title="Select all devices in this group"><input type="checkbox" data-act="sel-group" data-gid="${g.id}" ${allSel ? 'checked' : ''}> select</label>` : ''}
      </div>` : '';
    const body = folded ? '' : list.length
      ? html`<div class="cards">${list.map(card)}</div>`
      : html`<div class="gempty">${g.id ? (arranging() ? 'Empty group — drag devices here' : 'Empty group') : 'No ungrouped devices'}</div>`;
    return html`<section class="gsec" data-gid="${g.id}">${head}<div class="gbody" data-gid="${g.id}">${body}</div></section>`;
  }
  function render() {
    const d = st.data;
    if (!d || st.drag) return;
    // Layout lock: locked by default; unlocked, cards and groups can be dragged.
    const ab = $('#d-arrange');
    if (ab) setHTML(ab, st.arrange
      ? html`<button class="btn primary" data-act="arrange" title="Lock the layout again">${icon('unlock')} Lock layout</button>`
      : html`<button class="btn" data-act="arrange" title="Unlock the layout to drag devices and groups into another order or group">${icon('lock')} Unlock layout</button>`);
    setHTML($('#d-arrange-note'), arranging() ? html`<div class="notice blue">Layout unlocked: drag a device card to change its order or group, or a group header to reorder the groups. Lock the layout when done; it is locked again after a page reload.</div>` : '');
    const A = d.agents;
    const active = A.filter((a) => a.status !== 'pending');
    // Services OK: per agent its connection; per agentless device the
    // running ping and services. Undecided (waiting, retrying) is not OK.
    let svcTotal = 0, svcOK = 0, svcDown = 0, devClean = 0;
    // Unreachable devices, per device they are cut off behind: the cause, once.
    const behind = new Map();
    for (const a of active) {
      let t = 0, ok = 0, down = 0;
      // An agent counts its connection, plus the checks on its Services tab.
      const L = chk(a), [ct, cok, cdown] = L ? checkCounts(L) : [0, 0, 0];
      if (a.maint || a.unreach) {
        // In maintenance or unreachable: counts, but not as OK and not as down.
        t = (isLocal(a) ? 0 : 1) + ct;
        if (a.unreach && !a.maint) {
          const c = behind.get(a.unreach.id) || { name: a.unreach.name, n: 0 };
          c.n++;
          behind.set(a.unreach.id, c);
        }
      } else {
        t = ct; ok = cok; down = cdown; // unknown (probe out): counts, but not as OK and not as down
        if (!isLocal(a)) {
          t++;
          if (a.online) ok++; else down++;
        }
      }
      svcTotal += t; svcOK += ok; svcDown += down;
      if (!down) devClean++;
    }
    const svcClass = svcDown ? 'c-red' : svcOK < svcTotal ? 'c-amber' : 'c-green';
    const totUpd = active.reduce((s, a) => s + (a.updates?.total || 0), 0);
    const totSec = active.reduce((s, a) => s + (a.updates?.security || 0), 0);
    const reboot = active.filter((a) => a.updates?.reboot_required).length;
    const pending = A.filter((a) => a.status === 'pending').length;
    const reportOnly = A.filter((a) => a.status === 'report_only').length;
    setPending(pending);
    let oldest = null, anyOffline = false, anyUnchecked = false;
    for (const a of A) {
      if (a.status !== 'active' || a.below_min || isLocal(a) || a.maint) continue;
      const ts = a.online ? a.updates?.checked_at : a.last_seen;
      if (!a.online) anyOffline = true;
      if (a.online && !ts && hasUpdates(a)) anyUnchecked = true;
      if (!ts) continue;
      if (!oldest || ts < oldest.ts) oldest = { ts, a };
    }
    if (anyOffline && oldest?.a.online) {
      const off = A.filter((a) => a.status === 'active' && !a.below_min && !isLocal(a) && !a.online && a.last_seen).sort((x, y) => x.last_seen - y.last_seen)[0];
      if (off) oldest = { ts: off.last_seen, a: off };
    }
    const age = oldest ? now() - oldest.ts : 0;
    const interval = (d.settings?.check_interval_hours || 6) * 3600;
    const fresh = age <= interval + 900;
    const allCurrent = !!oldest && fresh && !anyOffline && !anyUnchecked;
    const oc = !oldest || allCurrent ? 'c-green' : fresh || age <= Math.max(86400, interval * 2) ? 'c-amber' : 'c-red';
    if (allCurrent) oldest = null;
    const hours = d.settings?.check_interval_hours || 6;
    setHTML($('#d-stats'), html`
      <div class="panel stat ${svcClass}" title="Agents: the connection and their services. Agentless devices: the ping and their services. Paused checks are not counted."><div class="label">Services OK</div><div class="value">${svcOK}<small> / ${svcTotal}</small></div>
        <div class="statnote">${active.length} device${active.length === 1 ? '' : 's'} · ${devClean} without failures${reportOnly ? html` · <span title="Report-only devices only report; no checks, installs or reboots">${reportOnly} report only</span>` : ''}</div>
        ${[...behind.values()].map((c) => html`<div class="statnote"><span class="c-unr" title="Unreachable devices are not counted as failures">${c.n} unreachable behind ${c.name}</span></div>`)}</div>
      <div class="panel stat c-amber"><div class="label">Pending updates</div><div class="value">${totUpd}</div>
        ${totSec ? html`<div class="statnote"><span class="c-red">${totSec} security</span></div>` : ''}</div>
      <div class="panel stat c-violet"><div class="label">Reboot required</div><div class="value">${reboot}</div></div>
      ${upTile(d.upstream)}
      <div class="panel stat ${oc} ${oldest ? 'click' : ''}" ${oldest ? raw(`data-act="open-agent" data-id="${esc(oldest.a.id)}"`) : ''} title="${oldest ? 'Oldest information shown on this dashboard: ' + fmtDate(oldest.ts, true) : ''}">
        <div class="label">Oldest information</div>
        ${oldest ? html`<div class="value datev">${fmtDate(oldest.ts)}</div><div class="statnote">${ago(oldest.ts)} · ${oldest.a.name} (${oldest.a.online ? 'last update check' : 'offline'})</div>`
          : allCurrent ? html`<div class="value datev">All current</div><div class="statnote">all checked within ${hours} h</div>`
          : html`<div class="value">—</div><div class="statnote">nothing to report</div>`}</div>`);
    // The tiles below also show the virtual devices; the counts above do not.
    const D = [...A, ...d.vdev].sort((x, y) => (x.order ?? 0) - (y.order ?? 0));
    setHTML($('#d-chips'), filterChips(D, st.filter, (a, f) => agentMatches(a, f, st.q, d.groups, st.type)));
    setHTML($('#d-type'), typeTabs(st.type));
    if (!D.length) {
      setHTML($('#d-cards'), html`<div class="panel empty"><div class="big">🛰️</div><div>No devices yet.</div>${ifChange(html`<div style="margin-top:14px"><a class="btn primary" href="#/settings/devices">${icon('plus')} Add your first device</a></div>`)}</div>`);
    } else {
      const groups = d.groups;
      const showHead = groups.length > 0;
      const secs = [...groups, { id: '', name: 'Ungrouped' }].map((g) => {
        const members = D.filter((a) => (a.group_id || '') === g.id || (g.id === '' && !groups.some((x) => x.id === a.group_id)));
        if (!g.id && showHead && !members.length) return section(g, members, showHead, true);
        return section(g, members, showHead);
      });
      const any = D.some(matches);
      setHTML($('#d-cards'), any ? secs : html`<div class="panel empty">No devices match this filter.</div>`);
    }
    for (const id of [...st.sel]) if (!A.some((a) => a.id === id && a.status === 'active' && !isLocal(a))) st.sel.delete(id);
    const n = st.sel.size;
    setHTML($('#d-bulk'), n ? html`<div class="bulkbar"><b>${n}</b> selected<span class="spacer"></span>
      <button class="btn sm" data-act="bulk-check">${icon('refresh')} Check selected</button>
      <button class="btn warn sm" data-act="bulk-upgrade">${icon('zap')} Upgrade all packages</button>
      <button class="btn ghost sm" data-act="bulk-clear">Clear</button></div>` : '');
  }

  function clearMarks() { document.querySelectorAll('.drop-before,.drop-after,.drop-into').forEach((e) => e.classList.remove('drop-before', 'drop-after', 'drop-into')); }
  function dropTarget(e) {
    if (st.drag.type === 'agent') {
      const c = e.target.closest('.card[data-id]');
      if (c && c.dataset.id === st.drag.id) return null;
      if (c) {
        for (const l of layoutOf(st.data).values()) {
          const from = l.indexOf(st.drag.id), to = l.indexOf(c.dataset.id);
          if (from >= 0 && to >= 0) return { el: c, kind: 'card', id: c.dataset.id, after: from < to };
        }
        const b = c.getBoundingClientRect();
        return { el: c, kind: 'card', id: c.dataset.id, after: e.clientX > b.left + b.width / 2 };
      }
      const g = e.target.closest('.gsec');
      if (g) return { el: g, kind: 'group', gid: g.dataset.gid };
    } else {
      const g = e.target.closest('.gsec');
      if (g && g.dataset.gid && g.dataset.gid !== st.drag.id) {
        const b = g.getBoundingClientRect();
        return { el: g, kind: 'gsec', gid: g.dataset.gid, after: e.clientY > b.top + b.height / 2 };
      }
    }
    return null;
  }
  function onDragStart(e) {
    const src = e.target.closest('[data-drag]');
    if (!src) return;
    st.drag = src.dataset.drag === 'group' ? { type: 'group', id: src.dataset.gid } : { type: 'agent', id: src.dataset.id };
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', st.drag.id);
    (src.closest('.gsec') && st.drag.type === 'group' ? src.closest('.gsec') : src).classList.add('dragging');
    if (st.drag.type === 'agent') $('#d-cards').classList.add('dnd');
  }
  function onDragOver(e) {
    if (!st.drag) return;
    const t = dropTarget(e);
    clearMarks();
    if (!t) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    t.el.classList.add(t.kind === 'group' ? 'drop-into' : t.after ? 'drop-after' : 'drop-before');
  }
  async function onDrop(e) {
    if (!st.drag) return;
    const t = dropTarget(e);
    const drag = st.drag;
    e.preventDefault();
    clearMarks();
    $('#d-cards').classList.remove('dnd');
    if (!t) return;
    const d = st.data;
    const lists = layoutOf(d);
    let order = d.groups.map((g) => g.id);
    if (drag.type === 'agent') {
      for (const l of lists.values()) { const i = l.indexOf(drag.id); if (i >= 0) l.splice(i, 1); }
      if (t.kind === 'card') {
        const ta = [...d.agents, ...d.vdev].find((a) => a.id === t.id);
        const l = lists.get(ta.group_id) || lists.get('');
        const i = l.indexOf(t.id);
        l.splice(t.after ? i + 1 : i, 0, drag.id);
      } else {
        (lists.get(t.gid) || lists.get('')).push(drag.id);
      }
      for (const [gid, l] of lists) l.forEach((id, i) => { const a = [...d.agents, ...d.vdev].find((x) => x.id === id); a.group_id = gid; a.order = i; });
      const pos = new Map(); [...lists.values()].flat().forEach((id, i) => pos.set(id, i));
      d.agents.sort((a, b) => pos.get(a.id) - pos.get(b.id));
    } else {
      order = order.filter((id) => id !== drag.id);
      const i = order.indexOf(t.gid);
      order.splice(t.after ? i + 1 : i, 0, drag.id);
      d.groups.sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id));
    }
    st.drag = null;
    render();
    try { await saveLayout(order, lists); } catch (err) { fail(err); }
    view?.refresh?.();
  }
  function onDragEnd() { st.drag = null; clearMarks(); $('#d-cards').classList.remove('dnd'); document.querySelectorAll('.dragging').forEach((e) => e.classList.remove('dragging')); render(); }

  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`
        ${subTabs('dashboard', 'overview')}<div class="page-head"><div><h1>Overview</h1><div class="sub">What needs your attention, at a glance</div></div><span class="spacer"></span>
          ${ifChange(html`<span id="d-arrange"></span><button class="btn" data-act="check-all">${icon('refresh')} Check now</button>`)}</div>
        <div id="d-notice"></div><div id="d-arrange-note"></div>
        <div class="stats" id="d-stats"></div>
        <div class="toolbar"><input type="search" class="search" id="d-q" placeholder="Search name, OS, IP, group…"><div class="chips" id="d-chips"></div><div class="tabs typetabs" id="d-type"></div></div>
        <div id="d-cards"></div>
        <div id="d-bulk"></div>`);
      $('#d-q').addEventListener('input', (e) => { st.q = e.target.value; render(); });
      const c = $('#d-cards');
      c.addEventListener('dragstart', onDragStart);
      c.addEventListener('dragover', onDragOver);
      c.addEventListener('drop', onDrop);
      c.addEventListener('dragend', onDragEnd);
      if (session.initial_password) setHTML($('#d-notice'), html`<div class="notice">You are using the generated initial password. <a href="#/settings">Change it in Settings</a>.</div>`);
    },
    async refresh() { const d = await GET('overview'); d.groups ||= []; d.vdev = virtualDevs(d); if (!st.drag) { st.data = d; render(); } },
    acts: {
      arrange: () => { st.arrange = !st.arrange; render(); },
      filter: (t) => { st.filter = t.dataset.f; render(); },
      type: (t) => { st.type = t.dataset.t; render(); },
      fold: (t) => { const g = t.dataset.gid; st.folded.has(g) ? st.folded.delete(g) : st.folded.add(g); render(); },
      'open-agent': (t) => { location.hash = '#/device/' + t.dataset.id; },
      sel: (t) => { t.checked ? st.sel.add(t.dataset.id) : st.sel.delete(t.dataset.id); render(); },
      'sel-group': (t) => {
        const gid = t.dataset.gid;
        const ids = st.data.agents.filter((a) => (a.group_id || '') === gid && a.status === 'active' && !isLocal(a) && matches(a)).map((a) => a.id);
        ids.forEach((id) => (t.checked ? st.sel.add(id) : st.sel.delete(id)));
        render();
      },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.groups),
      'bulk-clear': () => { st.sel.clear(); render(); },
      'bulk-check': () => runAction([...st.sel].filter((id) => hasUpdates(st.data.agents.find((a) => a.id === id))), 'updates', 'check', {}, { open: false }),
      'bulk-upgrade': async () => {
        const names = st.data.agents.filter((a) => st.sel.has(a.id)).map((a) => a.name);
        if (await confirmBox('Upgrade all packages?', `This runs a full package upgrade on ${names.length} device(s): ${names.join(', ')}.`, { okText: 'Upgrade all', danger: true })) {
          await runAction([...st.sel], 'updates', 'upgrade_all', {}, { open: false });
          st.sel.clear();
        }
      },
      'check-all': () => {
        const ids = st.data.agents.filter((a) => a.status === 'active' && a.online && hasUpdates(a)).map((a) => a.id);
        if (!ids.length) return toast('No online devices with the update function', 'err');
        return runAction(ids, 'updates', 'check', {}, { open: false });
      },
      'agent-update': async (t) => {
        const a = st.data.agents.find((x) => x.id === t.dataset.id);
        if (!(await confirmBox(`Update the agent on ${a.name}?`, `Agent ${a.core_version} is older than the lowest allowed version ${a.min_agent}. It is updated to ${a.core_latest}; afterwards the device reports normally again.`, { okText: 'Update agent' }))) return;
        let res;
        try { res = await POST(`agents/${a.id}/core`, {}); }
        catch (e) { throw new Error(`${a.name}: agent update not requested: ${e.message || e}`); }
        if (res.job) openJob(res.job); else toast('Update requested — it runs when the device is online', 'ok');
        view.refresh();
      },
    },
  };
}

function lastReboot(a) {
  const up = a.facts?.uptime_sec;
  if (!up || !a.last_seen) return '';
  return `Last reboot: ${fmtDate(a.last_seen - up)}${a.online ? '' : ' (as last reported)'}`;
}
const clipText = (s, n) => (s && s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s);
function healthCell(a) {
  const h = a.facts?.health;
  if (!h || h.failed_count < 0) return html`<span class="dim">—</span>`;
  if (a.maint) return html`<span class="dim">maintenance</span>`;
  const main = h.failed_count > 0
    ? html`<span class="badge sec xs" title="Failed services: ${(h.failed || []).join(', ')}${h.failed_count > (h.failed || []).length ? ', …' : ''}">${h.failed_count} failed</span>`
    : html`<span class="badge ok xs">ok</span>`;
  return html`${main}${h.auto_updates === 'on' ? html` <span class="badge info xs" title="Automatic updates (unattended-upgrades) are on">auto</span>` : ''}`;
}
const osShort = (f) => (f.os_id && f.os_version ? f.os_id.charAt(0).toUpperCase() + f.os_id.slice(1) + ' ' + f.os_version : osLabel(f));

function DeviceList() {
  lastDashSub = 'devices';
  const st = { filter: 'all', type: 'all', q: '', data: null, sort: 'name', dir: 1 };
  const byText = (x, y) => String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' });
  const statusText = (a) => (a.upstream ? { ok: 'online', partial: 'partly online', down: 'offline' }[a.upstream.state] || 'waiting'
    : isVirtual(a) ? (a.unreach ? 'unreachable' : '') : a.maint ? 'maintenance' : a.unreach ? 'unreachable' : isLocal(a) ? (probeOut(a) ? 'unknown' : !a.local.checked ? 'waiting' : a.local.up ? (a.local.svc_down ? `${a.local.svc_down} service${a.local.svc_down === 1 ? '' : 's'} down` : 'online') : 'down')
    : a.status === 'pending' ? 'pending' : a.below_min ? 'update required' : !a.online ? 'offline' : agentChecksDown(a) ? checksDownShort(agentChecksDown(a)) : a.status === 'report_only' ? 'report only' : 'online');
  const lastTs = (a) => (a.upstream ? upLast(a.upstream) : isLocal(a) ? a.local.last_check || 0 : a.online ? a.updates?.checked_at || 0 : a.last_seen || 0);
  const healthN = (a) => (a.facts?.health?.failed_count > 0 ? 2 : 0) + (a.facts?.health?.auto_updates === 'on' ? 1 : 0);
  const COLS = [
    ['status', '●', (a) => statusText(a), 'dotcol'],
    ['name', 'Name', (a) => a.name, ''],
    ['group', 'Group', (a) => groupName(a.group_id), 'opt'],
    ['ip', 'IP', (a) => (a.facts.ips || [])[0] || '', 'opt'],
    ['os', 'OS', (a) => (isVirtual(a) ? '~virtual' : isLocal(a) ? '~local' : osShort(a.facts)), 'opt'],
    ['stats', 'Stats', (a) => (isLocal(a) || isVirtual(a) ? -1 : (a.checks?.stats || []).reduce((n, x) => n + (x.s === 'down' ? 10 : 1), 0)), 'opt'],
    ['updates', 'Updates', (a) => a.updates?.total ?? -1, 'num'],
    ['cve', 'CVE', (a) => a.vulns?.badge ?? -1, 'num'],
    ['reboot', 'Reboot', (a) => (a.updates?.reboot_required ? 1 : 0), ''],
    ['agent', 'Agent', (a) => a.core_version || '', 'opt'],
    ['health', 'Health', healthN, 'opt'],
    ['last', 'Checked', lastTs, ''],
  ];
  const groupName = (id) => st.data?.groups.find((g) => g.id === id)?.name || '';
  function cmp(x, y) {
    const col = COLS.find((c) => c[0] === st.sort);
    const vx = col[2](x), vy = col[2](y);
    const c = typeof vx === 'number' ? vx - vy : byText(vx, vy);
    return st.dir * (c || byText(x.name, y.name));
  }
  function last(a) {
    if (a.maint) return html`<span class="c-blue">maintenance</span><span class="dim">, ${maintUntil(a.maint)}</span>`;
    if (a.unreach) return html`<span class="c-unr">unreachable</span><span class="dim">, behind ${a.unreach.name}</span>`;
    if (a.upstream) return upLast(a.upstream) ? when(upLast(a.upstream)) : html`<span class="dim">waiting for the first ping</span>`;
    if (isVirtual(a)) return html`<span class="dim">—</span>`;
    if (isLocal(a)) {
      const L = a.local, p = probeOut(a);
      if (p) return html`<span class="dim">${unknownText(p)}</span>`;
      if (!L.last_check) return html`<span class="dim">waiting for first check</span>`;
      return L.checked && !L.up ? html`<span class="c-red">down</span><span class="dim">, checked ${when(L.last_check)}</span>` : when(L.last_check);
    }
    if (a.status === 'pending') return html`<span class="dim">enrolled ${when(a.enrolled_at)}</span>`;
    if (a.below_min) return html`<span class="dim">seen ${when(a.last_seen)}</span>`;
    if (!a.online) return html`<span class="c-red">offline</span><span class="dim">, seen ${when(a.last_seen)}</span>`;
    return a.updates?.checked_at ? when(a.updates.checked_at) : html`<span class="dim">waiting for first check</span>`;
  }
  function row(a) {
    const u = a.updates, o = a.os_support, f = a.facts;
    return html`<tr>
      <td class="nowrap" title="${statusText(a)}${a.below_min ? ` (agent ${a.core_version}, lowest allowed ${a.min_agent})` : ''}">${agentDot(a)}${statusText(a) === 'online' || statusText(a) === '' ? '' : html` <span class="small ${a.maint ? 'c-blue' : a.unreach ? 'c-unr' : statusText(a) === 'unknown' ? 'dim' : a.status === 'pending' || a.status === 'report_only' || a.below_min || statusText(a) === 'waiting' || statusText(a) === 'partly online' || / (service|check)s? down/.test(statusText(a)) ? 'c-amber' : 'c-red'}">${statusText(a)}</span>`}</td>
      <td class="nowrap"><a href="${a.upstream ? '#/settings/upstream' : `#/device/${a.id}`}"><b>${a.name}</b></a></td>
      <td class="opt muted">${groupName(a.group_id)}</td>
      <td class="opt small nowrap">${(f.ips || [])[0] || (isVirtual(a) ? html`<span class="dim">—</span>` : '')}</td>
      <td class="opt nowrap" title="${a.below_min || isLocal(a) || isVirtual(a) ? '' : osLabel(f)}">${a.upstream ? html`<span class="dim" title="The internet connection, pinged by the master (Settings → Upstream)">upstream</span>` : isVirtual(a) ? html`<span class="dim" title="Virtual device: not checked">virtual</span>` : isLocal(a) ? html`<span class="dim" title="Monitored by the master with a ping; no agent">agentless</span>` : a.below_min ? html`<span class="dim">—</span>` : clipText(osShort(f), 14)}${o?.status === 'expired' ? html` <span class="badge sec xs" title="${osSupportText(o, f.os_id)}">EOL</span>` : o?.status === 'soon' ? html` <span class="badge upd xs" title="${osSupportText(o, f.os_id)}">EOL soon</span>` : ''}</td>
      <td class="opt nowrap">${isVirtual(a) ? html`<span class="dim">—</span>` : statLetters(a)}</td>
      <td class="num"><b class="${u?.security ? 'c-red' : u?.total ? 'c-amber' : 'dim'}">${u ? u.total : '—'}</b></td>
      <td class="num">${!a.vulns ? html`<span class="dim">—</span>` : a.vulns.badge ? html`<b class="c-red" title="${CVE_RED}">${a.vulns.badge}</b>` : html`<span class="dim">0</span>`}</td>
      <td>${!u ? html`<span class="dim">—</span>` : u.reboot_required ? html`<span class="badge reboot xs">required</span>` : html`<span class="badge ok xs">no</span>`}</td>
      <td class="opt mono small"><span class="${!a.core_version ? 'dim' : a.core_is_latest ? 'c-green' : 'c-amber'}" title="${a.core_is_latest ? 'latest agent' : a.core_latest ? 'latest is ' + a.core_latest : ''}">${a.core_version || '—'}</span></td>
      <td class="opt nowrap">${healthCell(a)}</td>
      <td class="small nowrap">${last(a)}</td></tr>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const A = [...d.agents, ...d.vdev];
    setHTML($('#l-chips'), filterChips(A, st.filter, (a, f) => agentMatches(a, f, st.q, d.groups, st.type)));
    setHTML($('#l-type'), typeTabs(st.type));
    const shown = A.filter((a) => agentMatches(a, st.filter, st.q, d.groups, st.type));
    if (!A.length) return setHTML($('#l-table'), html`<div class="panel empty">No devices yet.</div>`);
    if (!shown.length) return setHTML($('#l-table'), html`<div class="panel empty">No devices match this filter.</div>`);
    const groups = [...d.groups].sort((x, y) => byText(x.name, y.name));
    const known = new Set(groups.map((g) => g.id));
    const secs = [...groups.map((g) => [g.name, shown.filter((a) => a.group_id === g.id)]), ['Ungrouped', shown.filter((a) => !known.has(a.group_id))]]
      .filter(([, list]) => list.length);
    const head = COLS.map(([k, t, , cls]) => html`<th class="${cls} sortable ${st.sort === k ? 'on' : ''}" data-act="sort" data-k="${k}" title="Sort by ${k === 'status' ? 'status' : t.toLowerCase()}">${t}${st.sort === k ? (st.dir > 0 ? ' ▲' : ' ▼') : ''}</th>`);
    setHTML($('#l-table'), html`<div class="panel scrollx"><table class="tbl syslist"><thead><tr>${head}</tr></thead>
      ${secs.map(([name, list]) => html`<tbody>${groups.length ? html`<tr class="grow"><td colspan="${COLS.length}">${name} <span class="dim small">${list.length}</span></td></tr>` : ''}
        ${[...list].sort(cmp).map(row)}</tbody>`)}</table></div>`);
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('dashboard', 'devices')}<div class="page-head"><div><h1>Device List</h1><div class="sub">All devices at a glance</div></div></div>
        <div class="toolbar"><input type="search" class="search" id="l-q" placeholder="Search name, OS, IP, group…"><div class="chips" id="l-chips"></div><div class="tabs typetabs" id="l-type"></div></div>
        <div id="l-table"></div>`);
      $('#l-q').addEventListener('input', (e) => { st.q = e.target.value; render(); });
    },
    async refresh() { const d = await GET('overview'); d.groups ||= []; d.vdev = virtualDevs(d, true); st.data = d; render(); },
    acts: {
      filter: (t) => { st.filter = t.dataset.f; render(); },
      type: (t) => { st.type = t.dataset.t; render(); },
      sort: (t) => { const k = t.dataset.k; st.dir = st.sort === k ? -st.dir : 1; st.sort = k; render(); },
    },
  };
}

// Dashboard > Map, in three views (always opening on Network):
// - Network: the tree of "Connected via": every device under the device it
//   sits behind, the others under the master. Where a device above is down,
//   the line breaks (✕) and the devices behind it are dimmed as unreachable.
// - Monitoring: who checks what: the master, and each remote probe with the
//   devices it checks.
// - Full map: the network tree, plus a dotted line from each remote probe to
//   every device it checks; hovering a box keeps only its own lines.
// Every view is a tree: leaves take the next row, a parent is centred on its
// children, so boxes never overlap and tree lines never cross. Colours
// refresh in place; the drawing is only rebuilt when the tree changes.
function MapPage() {
  lastDashSub = 'map';
  const W = 176, H = 40, ROW = 52, GAP = 112, PAD = 8;
  // view: zoom k and offset x/y of the drawing (fitted to the screen on every
  // visit and view change, never stored); size: the drawing's own size.
  const st = { key: '', data: null, mode: 'network', nodes: new Map(), view: null, size: null, hh: 0 };
  const byText = (x, y) => String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' });
  const VIEWS = [['network', 'Network'], ['monitor', 'Monitoring'], ['full', 'Full map']];
  // Monitoring colour: ok (green), bad (red), unr (unreachable) or unk
  // (grey), with a short status; a third value marks a device that is down
  // itself (its dot flashes).
  function health(a) {
    if (!a) return ['ok', 'the master'];
    if (isVirtual(a)) {
      if (a.special) {
        const u = st.data?.upstream || {}, txt = `${upWord[u.state] || 'waiting'}${u.total ? ` · ${u.answering} of ${u.total} answering` : ''}${u.rtt_us ? ` · ${fmtMs(u.rtt_us)}` : ''}`;
        return u.state === 'ok' ? ['ok', txt] : u.state === 'partial' ? ['part', txt] : u.state === 'down' ? ['bad', txt, true] : ['unk', 'waiting for the first ping'];
      }
      return a.unreach ? ['unr', unrText(a)] : ['virt', 'virtual device, not checked'];
    }
    if (a.maint) return ['unk', maintText(a.maint)];
    if (a.unreach) return ['unr', unrText(a)];
    if (isLocal(a)) {
      const L = a.local, p = probeOut(a);
      if (p) return ['unk', unknownText(p)];
      if (!L.checked) return ['unk', L.fails ? 'no answer yet' : 'waiting for the first check'];
      if (!L.up) return ['bad', `down for ${dur(now() - L.since)}`, true];
      if (L.svc_down) return ['bad', `${L.svc_down} of ${L.svc_total} service${L.svc_total === 1 ? '' : 's'} down`];
      return ['ok', `up for ${dur(now() - L.since)}`];
    }
    if (a.status === 'pending') return ['unk', 'pending approval'];
    if (!a.online) return ['bad', `offline since ${fmtDate(a.last_seen)}`, true];
    const x = agentChecksDown(a);
    if (x) return ['bad', `online · ${checksDownText(x)}${checksFailText(x) ? ` · ${checksFailText(x)}` : ''}`];
    return ['ok', a.status === 'report_only' ? 'online (report only)' : 'online'];
  }
  // The remote probe that checks a device (agentless, or an agent device's services), or null.
  const checker = (a) => (a ? (isLocal(a) ? a.local.probe : a.checks?.probe) || null : null);
  // The tree of a view. Each node: { id, a, depth, kids, remote }.
  // The network views also show the virtual devices and, with Upstream on,
  // start at the cloud: the master hangs under its own "Connected via" (or
  // the cloud), the devices without one under the master.
  function tree(d, mode) {
    const net = mode !== 'monitor';
    const cloud = net && d.upstream?.enabled ? (d.virtuals || []).find((v) => v.special === 'upstream') || null : null;
    const A = [...d.agents, ...(net ? (d.virtuals || []).filter((v) => !v.special) : [])], groups = d.groups || [];
    const gi = (a) => { const i = groups.findIndex((g) => g.id === a.group_id); return i < 0 ? groups.length : i; };
    const rootA = A.find((a) => a.is_master && a.status !== 'pending' && !isLocal(a)) || null;
    const masterID = rootA ? rootA.id : 'root';
    const byID = new Map(A.map((a) => [a.id, a]));
    // The devices above the master: they hang under the cloud, not under the master.
    const aboveMaster = new Set();
    if (cloud) for (let x = rootA?.via && byID.get(rootA.via.id); x && !aboveMaster.has(x.id); x = x.via && byID.get(x.via.id)) aboveMaster.add(x.id);
    const kids = new Map();
    const add = (pid, id, a, remote) => { if (!kids.has(pid)) kids.set(pid, []); kids.get(pid).push({ id, a, remote }); };
    for (const a of A) {
      if (a === rootA) continue;
      let pid = aboveMaster.has(a.id) ? cloud.id : masterID, remote = false;
      if (mode === 'monitor') {
        const p = isLocal(a) ? a.local.probe : null;
        if (p && byID.has(p.id) && p.id !== masterID && !isLocal(byID.get(p.id))) { pid = p.id; remote = true; }
      } else if (a.via && (byID.has(a.via.id) || a.via.id === cloud?.id)) pid = a.via.id;
      add(pid, a.id, a, remote);
    }
    if (cloud) add(rootA?.via && (byID.has(rootA.via.id) || rootA.via.id === cloud.id) ? rootA.via.id : cloud.id, masterID, rootA, false);
    const nm = (k) => (k.a ? k.a.name : '');
    for (const l of kids.values()) l.sort((x, y) => (x.id === masterID) - (y.id === masterID) || (x.a ? gi(x.a) : 0) - (y.a ? gi(y.a) : 0) || byText(nm(x), nm(y)));
    const seen = new Set();
    const node = (id, a, remote, depth) => {
      seen.add(id);
      return { id, a, remote, depth, kids: (kids.get(id) || []).filter((k) => !seen.has(k.id)).map((k) => node(k.id, k.a, k.remote, depth + 1)) };
    };
    const root = cloud ? node(cloud.id, cloud, false, 0) : node(masterID, rootA, false, 0);
    // Anything left (a loop in old data): under the top.
    if (cloud && !seen.has(masterID)) root.kids.push(node(masterID, rootA, false, 1));
    for (const a of A) if (a !== rootA && !seen.has(a.id)) root.kids.push(node(a.id, a, false, 1));
    return root;
  }
  // Positions: leaves take the next free row; a parent is centred on its children.
  function place(root) {
    let row = 0;
    // The cloud reaches above and below its row.
    const top = isVirtual(root.a) ? 20 : 0, bottom = isVirtual(root.a) ? 22 : 0;
    const all = [];
    (function walk(n, parent) {
      n.parent = parent;
      n.x = PAD + n.depth * (W + GAP);
      if (!n.kids.length) { n.y = PAD + top + row * ROW; row++; }
      else { n.kids.forEach((k) => walk(k, n)); n.y = (n.kids[0].y + n.kids[n.kids.length - 1].y) / 2; }
      all.push(n);
    })(root, null);
    const depth = Math.max(...all.map((n) => n.depth));
    if (top && root.y < PAD + top) { const dy = PAD + top - root.y; all.forEach((n) => { n.y += dy; }); row += dy / ROW; }
    return { all, w: PAD * 2 + (depth + 1) * W + depth * GAP, h: PAD * 2 + top + bottom + Math.max(1, row) * ROW - (ROW - H) };
  }
  // The state of the tree line to device k (under n): live (both ends answer:
  // a slow pulse runs toward the master; a service down does not stop it),
  // bad (k itself is down: red, still) or neither (unknown, maintenance, cut).
  function lineState(n, k, mode) {
    const cut = mode !== 'monitor' && !!k.a?.unreach;
    const kh = health(k.a), nh = health(n.a), alive = (h) => ['ok', 'bad', 'part', 'virt'].includes(h[0]) && !h[2];
    return { cut, live: !cut && alive(kh) && alive(nh), bad: !cut && !!kh[2] };
  }
  // Tree lines: an S-curve from the parent's right edge to each child. In the
  // network views a line to an unreachable device is dashed, and where the
  // break starts (the parent is not unreachable itself) it gets a ✕. Each
  // line has a pulse on top (shown while live). No ping times: they clutter.
  // The pulse runs from the child to the parent; alternating levels take
  // turns, so the wave flows on toward the master.
  function lines(all, mode) {
    const out = [];
    for (const n of all) {
      const x1 = n.x + W - (isVirtual(n.a) && n.a.special ? 20 : 0), y1 = n.y + H / 2;
      for (const k of n.kids) {
        const x2 = k.x, y2 = k.y + H / 2, mx = (x1 + x2) / 2;
        const s = lineState(n, k, mode), d = `M${x1} ${y1} C${mx} ${y1} ${mx} ${y2} ${x2} ${y2}`, tk = `t:${n.id}:${k.id}`;
        out.push(html`<path class="mapline ${k.remote ? 'remote' : ''} ${s.cut ? 'cut' : ''} ${s.bad ? 'bad' : ''}" data-k="${tk}" d="${d}"/>`);
        out.push(html`<path class="mappulse ${s.live ? 'live' : ''}" data-p="${tk}" d="${d}" pathLength="100" style="animation-delay:${(k.depth % 2) * 2.5}s"/>`);
        if (s.cut && !n.a?.unreach) {
          const cx = x1 + 16, cy = y1 + (y2 - y1) * 0.04;
          out.push(html`<path class="mapcut" data-k="${tk}" d="M${cx - 4} ${cy - 4} L${cx + 4} ${cy + 4} M${cx + 4} ${cy - 4} L${cx - 4} ${cy + 4}"/>`);
        }
      }
    }
    return out;
  }
  // Full map: a dotted line from each remote probe to the devices it checks.
  function checkLines(all) {
    const at = new Map(all.map((n) => [n.id, n]));
    const out = [];
    for (const n of all) {
      const p = checker(n.a), pn = p && at.get(p.id);
      if (!pn || pn === n) continue;
      const x1 = pn.x + W, y1 = pn.y + H / 2, x2 = n.x, y2 = n.y + H / 2, dx = Math.max(60, Math.abs(x2 - x1) / 2);
      out.push(html`<path class="mapline check" data-k="c:${pn.id}:${n.id}" d="M${x1} ${y1} C${x1 + dx} ${y1} ${x2 - dx} ${y2} ${x2} ${y2}"/>`);
    }
    return out;
  }
  function tip(n) {
    const a = n.a, p = checker(a);
    return `${a ? a.name : 'Master'}: ${health(a)[1]}${p ? ` · checked by ${p.name}` : ''}${a?.via ? ` · connected via ${a.via.name}` : ''}`;
  }
  // Upstream: a layered cloud in its state colour that breathes, with a
  // highlight drifting across and an edge that lights up as the pulses of
  // its lines arrive (every 2.5 s, in step with them).
  const CLOUD = 'M30 58 A20 20 0 0 1 26 18 A26 26 0 0 1 72 -2 A32 32 0 0 1 128 6 A22 22 0 0 1 156 30 A16 16 0 0 1 150 58 Z';
  function cloudSub(u) {
    if (!u || u.state === 'wait') return 'waiting';
    if (u.state === 'down') return 'offline';
    return `${u.rtt_us ? fmtMs(u.rtt_us) : ''}${u.state === 'partial' ? ` · ${u.answering} of ${u.total}` : ''}`;
  }
  function cloudBox(n) {
    const [cls] = health(n.a);
    return html`<g class="mapcloud ${cls} ${canChange() ? 'click' : ''}" id="map-${n.id}" data-id="${n.id}" ${canChange() ? raw('data-act="open-up"') : ''} transform="translate(${n.x} ${n.y})">
      <title>${tip(n)}</title>
      <defs><clipPath id="cloudclip"><path d="${CLOUD}"/></clipPath>
        <linearGradient id="cloudfill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" class="cl-s1"/><stop offset="1" class="cl-s2"/></linearGradient>
        <linearGradient id="cloudshine" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#fff" stop-opacity="0"/><stop offset="0.5" stop-color="#fff" stop-opacity="0.22"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient></defs>
      <path class="cl-glow" d="${CLOUD}"/>
      <path class="cl-back" d="${CLOUD}" transform="translate(14 -9) scale(0.9)"/>
      <path class="cl-base" d="${CLOUD}"/><path class="cl-tint" d="${CLOUD}" fill="url(#cloudfill)"/>
      <g clip-path="url(#cloudclip)"><rect class="cl-shine" x="-70" y="-30" width="56" height="100" fill="url(#cloudshine)"/></g>
      <path class="cl-flash" d="${CLOUD}"/>
      <text class="cl-name" x="90" y="27" text-anchor="middle">${clipText(n.a.name, 16)}</text>
      <text class="cl-sub" id="cloudsub" x="90" y="44" text-anchor="middle">${cloudSub(st.data?.upstream)}</text></g>`;
  }
  function box(n, mode) {
    if (isVirtual(n.a) && n.a.special) return cloudBox(n);
    const a = n.a, [cls, , down] = health(a), name = a ? a.name : 'Master';
    if (isVirtual(a)) {
      return html`<g class="mapbox virt click ${cls === 'unr' ? 'unr' : ''}" id="map-${n.id}" data-id="${n.id}" data-act="open" transform="translate(${n.x} ${n.y})">
        <title>${tip(n)}</title><rect width="${W}" height="${H}" rx="8"/><circle cx="16" cy="${H / 2}" r="4.5"/>
        <text x="30" y="${H / 2 - 2}">${clipText(name, 20)}</text><text class="ip" x="30" y="${H / 2 + 11}">virtual</text></g>`;
    }
    const ip = a && !isLocal(a) ? devIP(a) : '';
    const p = mode !== 'monitor' ? checker(a) : null;
    return html`<g class="mapbox ${cls} ${down ? 'down' : ''} ${a?.maint ? 'maint' : ''} ${a ? 'click' : ''}" id="map-${n.id}" data-id="${n.id}" ${a ? raw(`data-act="open"`) : ''} transform="translate(${n.x} ${n.y})">
      <title>${tip(n)}</title><rect width="${W}" height="${H}" rx="8"/><circle cx="16" cy="${H / 2}" r="5"/>
      ${ip || p ? html`<text x="30" y="${H / 2 - 2}">${clipText(name, p ? 16 : 20)}</text>${ip ? html`<text class="ip" x="30" y="${H / 2 + 11}">${ip}</text>` : ''}`
        : html`<text x="30" y="${H / 2 + 4.5}">${clipText(name, 20)}</text>`}
      ${p ? html`<text class="tag" x="${W - 8}" y="${H / 2 + 11}" text-anchor="end">by ${clipText(p.name, 12)}</text>` : ''}</g>`;
  }
  // Hover on the full map: the device, the devices above it (and their
  // lines) and its checker (and that line) stay; the rest fades.
  function focus(id) {
    const svg = $('#map svg');
    if (!svg) return;
    svg.querySelectorAll('.hl').forEach((e) => e.classList.remove('hl'));
    svg.classList.toggle('focus', !!id);
    if (!id) return;
    const n = st.nodes.get(id);
    if (!n) return;
    const keep = [`#map-${CSS.escape(id)}`];
    for (let x = n; x.parent; x = x.parent) keep.push(`#map-${CSS.escape(x.parent.id)}`, `[data-k="t:${x.parent.id}:${x.id}"]`);
    const p = checker(n.a);
    if (p) keep.push(`#map-${CSS.escape(p.id)}`, `[data-k="c:${p.id}:${id}"]`);
    // A probe: also the devices it checks.
    for (const m of st.nodes.values()) if (checker(m.a)?.id === id) keep.push(`#map-${CSS.escape(m.id)}`, `[data-k="c:${id}:${m.id}"]`);
    svg.querySelectorAll(keep.join(',')).forEach((e) => e.classList.add('hl'));
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const mode = st.mode;
    setHTML($('#map-tabs'), VIEWS.map(([k, l]) => html`<button class="${mode === k ? 'on' : ''}" data-act="mode" data-m="${k}">${l}</button>`));
    const root = tree(d, mode);
    const { all, w, h } = place(root);
    st.nodes = new Map(all.map((n) => [n.id, n]));
    const key = mode + '|' + all.map((n) => `${n.id}@${n.x},${n.y}${n.remote ? 'r' : ''}${n.a?.unreach ? 'u' : ''}/${checker(n.a)?.id || ''}`).join('|') + '|' + all.map((n) => (n.a ? n.a.name + '/' + devIP(n.a) : '')).join(',');
    if (key === st.key) {
      // Same tree: only colours, pulses, ping times and status texts change.
      for (const n of all) {
        const g = document.getElementById('map-' + n.id);
        if (!g) continue;
        const [cls, , down] = health(n.a);
        g.classList.remove('ok', 'bad', 'unk', 'unr', 'down', 'maint', 'part');
        if (cls !== 'virt') g.classList.add(cls);
        if (isVirtual(n.a) && n.a.special) { const cs = document.getElementById('cloudsub'); if (cs) cs.textContent = cloudSub(d.upstream); }
        if (down) g.classList.add('down');
        if (n.a?.maint) g.classList.add('maint');
        const t = g.querySelector('title');
        if (t) t.textContent = tip(n);
        if (!n.parent) continue;
        const tk = `t:${n.parent.id}:${n.id}`, s = lineState(n.parent, n, mode);
        document.querySelector(`#map path.mapline[data-k="${CSS.escape(tk)}"]`)?.classList.toggle('bad', s.bad);
        document.querySelector(`#map path.mappulse[data-p="${CSS.escape(tk)}"]`)?.classList.toggle('live', s.live);
      }
      return;
    }
    st.key = key;
    const keep = st.view && st.size && st.size.mode === mode ? st.view : null;
    st.size = { w, h, mode };
    const legend = mode === 'monitor'
      ? html`<span><svg width="28" height="8"><path class="mapline" d="M0 4 H28"/></svg> monitored by the master</span><span><svg width="28" height="8"><path class="mapline remote" d="M0 4 H28"/></svg> checked by a remote probe</span>`
      : html`<span><svg width="28" height="8"><path class="mapline" d="M0 4 H28"/></svg> connected via</span><span><svg width="28" height="8"><path class="mapline cut" d="M0 4 H28"/></svg> cut off</span>
        ${mode === 'full' ? html`<span><svg width="28" height="8"><path class="mapline check" d="M0 4 H28"/></svg> checked by a remote probe</span>` : html`<span><b style="color:var(--cyan);font-weight:500">by …</b> checked by a remote probe</span>`}
        ${(d.virtuals || []).some((v) => !v.special) ? html`<span><i class="lg virt"></i>virtual device</span>` : ''}${d.upstream?.enabled ? html`<span><i class="lg part"></i>${d.upstream.name}: partly answering</span>` : ''}`;
    const gw = d.gateway;
    const hint = mode !== 'monitor' && gw && !gw.done
      ? html`<div class="notice small" style="margin-top:10px">${icon('globe')} <b>${gw.name}</b> (${gw.ip}) is the default gateway of ${gw.count} device${gw.count === 1 ? '' : 's'}: probably your internet router.
          ${d.upstream?.enabled ? html`Set its Connected via to <b>${d.upstream.name}</b> to draw it under the cloud.` : html`Switch on ${canChange() ? html`<a href="#/settings/upstream">Settings → Upstream</a>` : 'Settings → Upstream'} and set its Connected via to it, to show your internet connection on the map.`}</div>`
      : '';
    setHTML($('#map'), html`<div class="panel mapwrap"><div class="mapctl"><button class="btn ghost sm iconbtn" data-act="zoom" data-f="1.25" title="Zoom in">+</button><button class="btn ghost sm iconbtn" data-act="zoom" data-f="0.8" title="Zoom out">−</button><button class="btn ghost sm" data-act="fit" title="Show the whole map">Fit</button></div>
      <svg class="mapsvg" width="100%" height="${st.hh || 320}" role="img" aria-label="${VIEWS.find((v) => v[0] === mode)[1]} map"><g id="map-vp">
      ${lines(all, mode)}${mode === 'full' ? checkLines(all) : ''}${all.map((n) => box(n, mode))}</g></svg></div>
      <div class="row dim small" style="margin-top:10px;gap:16px"><span><i class="lg ok"></i>up</span><span><i class="lg bad"></i>down or a service down</span>${mode === 'monitor' ? '' : html`<span><i class="lg unr"></i>unreachable</span>`}<span><i class="lg maint"></i>maintenance</span><span><i class="lg none"></i>unknown, waiting or pending</span>
        ${legend}</div>
      <div class="dim small" style="margin-top:6px">Scroll or pinch to zoom, drag to move.${mode === 'full' ? ' Point at a device to see only its own lines.' : ''}</div>${hint}`);
    const svg = $('#map svg');
    if (mode === 'full') {
      svg.addEventListener('mouseover', (e) => { const g = e.target.closest('.mapbox'); focus(g ? g.dataset.id : ''); });
      svg.addEventListener('mouseleave', () => focus(''));
    }
    panZoom(svg);
    if (keep) { st.view = keep; apply(); } else fit();
  }
  // Zoom and pan: the drawing sits in one group that is moved and scaled.
  function apply() {
    const v = st.view, g = $('#map-vp');
    if (v && g) g.setAttribute('transform', `translate(${v.x.toFixed(1)} ${v.y.toFixed(1)}) scale(${v.k.toFixed(4)})`);
  }
  // Fit: the whole map on one screen (never larger than its own size).
  function fit() {
    const svg = $('#map svg');
    if (!svg || !st.size) return;
    const { w, h } = st.size, cw = svg.clientWidth || w;
    const ch = Math.max(320, window.innerHeight - svg.getBoundingClientRect().top - 90);
    const k = Math.min(cw / w, ch / h, 1);
    st.hh = Math.max(260, Math.ceil(h * k));
    svg.setAttribute('height', st.hh);
    st.view = { k, x: (cw - w * k) / 2, y: (st.hh - h * k) / 2 };
    apply();
  }
  function zoomAt(px, py, f) {
    const v = st.view;
    if (!v) return;
    const k = Math.min(4, Math.max(0.15, v.k * f));
    v.x = px - (px - v.x) * (k / v.k);
    v.y = py - (py - v.y) * (k / v.k);
    v.k = k;
    apply();
  }
  function panZoom(svg) {
    const pts = new Map();
    let drag = null, moved = false, pinch = 0;
    const local = (e) => { const r = svg.getBoundingClientRect(); return [e.clientX - r.left, e.clientY - r.top]; };
    svg.addEventListener('wheel', (e) => { e.preventDefault(); const [x, y] = local(e); zoomAt(x, y, Math.exp(-e.deltaY * 0.0015)); }, { passive: false });
    svg.addEventListener('pointerdown', (e) => {
      if (e.pointerType === 'mouse' && e.button !== 0) return;
      pts.set(e.pointerId, local(e));
      moved = false;
      if (pts.size === 1) drag = { p: local(e), x: st.view.x, y: st.view.y };
      if (pts.size === 2) { const [a, b] = [...pts.values()]; pinch = Math.hypot(a[0] - b[0], a[1] - b[1]); drag = null; }
    });
    svg.addEventListener('pointermove', (e) => {
      if (!pts.has(e.pointerId)) return;
      const p = local(e);
      pts.set(e.pointerId, p);
      if (pts.size === 2 && pinch) {
        const [a, b] = [...pts.values()], dd = Math.hypot(a[0] - b[0], a[1] - b[1]);
        zoomAt((a[0] + b[0]) / 2, (a[1] + b[1]) / 2, dd / pinch);
        pinch = dd;
        moved = true;
      } else if (drag) {
        const dx = p[0] - drag.p[0], dy = p[1] - drag.p[1];
        if (!moved && Math.abs(dx) + Math.abs(dy) < 5) return;
        if (!moved) { moved = true; svg.setPointerCapture(e.pointerId); svg.classList.add('panning'); }
        st.view.x = drag.x + dx;
        st.view.y = drag.y + dy;
        apply();
      }
    });
    const up = (e) => { pts.delete(e.pointerId); if (pts.size < 2) pinch = 0; if (!pts.size) { drag = null; svg.classList.remove('panning'); } };
    svg.addEventListener('pointerup', up);
    svg.addEventListener('pointercancel', up);
    // A drag is not a click on the device under it.
    svg.addEventListener('click', (e) => { if (moved) { e.stopPropagation(); e.preventDefault(); moved = false; } }, true);
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('dashboard', 'map')}<div class="page-head"><div><h1>Map</h1></div></div><div class="tabs maptabs" id="map-tabs"></div><div id="map"></div>`);
    },
    async refresh() { const d = await GET('overview'); d.groups ||= []; st.data = d; render(); },
    acts: {
      open: (t) => { location.hash = '#/device/' + t.dataset.id; },
      'open-up': () => { location.hash = '#/settings/upstream'; },
      mode: (t) => { st.mode = t.dataset.m; st.key = ''; render(); },
      zoom: (t) => { const svg = $('#map svg'); if (svg) zoomAt(svg.clientWidth / 2, svg.clientHeight / 2, +t.dataset.f); },
      fit: () => fit(),
    },
  };
}

const COPYRIGHT = '© 2026 Dawnes, the Developer';
function AboutPage() {
  return {
    async mount(el) {
      el.innerHTML = r(html`${subTabs('about', 'about')}<div class="about"><div class="panel about-card"><div class="about-dim">Loading…</div></div></div>`);
      const [ov, set] = await Promise.all([GET('overview').catch(() => null), GET('settings').catch(() => null)]);
      const latest = ov?.repo?.core?.[0]?.version || '';
      const repo = set?.settings?.github_repo || 'Dawnes/coremon';
      setHTML($('.about-card', el), html`
        <img src="favicon.svg" alt="" class="about-logo">
        <div class="about-name">Coremon</div>
        <div class="about-ver">v${session.version}${session.build_date ? html` <span class="about-dim">· released ${ymd(session.build_date)}</span>` : ''}</div>
        <div class="about-rows">
          <div><span class="about-dim">Latest agent</span><span class="mono">${latest || '—'}</span></div>
          <div><span class="about-dim">Project</span><a href="https://github.com/${repo}" target="_blank" rel="noopener noreferrer">github.com/${repo}</a></div>
          <div><span class="about-dim">License</span><a href="#/about/license">PolyForm Noncommercial 1.0.0</a></div>
        </div>
        <div class="about-copy">${COPYRIGHT}</div>`);
    },
  };
}
function LicensePage() {
  return {
    async mount(el) {
      el.innerHTML = r(html`${subTabs('about', 'license')}<div class="page-head"><div><h1>License</h1><div class="sub">How Coremon may be used</div></div></div>
        <div class="stack licpage" style="max-width:860px">
          <div class="notice blue">Coremon is licensed under the <b>PolyForm Noncommercial License 1.0.0</b>: free for personal, hobby,
            educational and non-profit use. Commercial use requires permission from Dawnes, the Developer
            (<a href="https://github.com/Dawnes" target="_blank" rel="noopener noreferrer">github.com/Dawnes</a>).</div>
          <div class="panel panel-pad"><h2>Coremon</h2><div class="legal" id="lic-main">Loading…</div></div>
          <div class="panel panel-pad"><h2>Third-party software</h2><pre class="legal" id="lic-third">Loading…</pre></div>
          <div class="dim small">${COPYRIGHT}</div></div>`);
      const text = (p) => fetch(p, { cache: 'no-cache' }).then((x) => (x.ok ? x.text() : Promise.reject(new Error(x.status)))).catch(() => 'Could not load the license text.');
      const [main, third] = await Promise.all([text('legal/LICENSE.txt'), text('legal/THIRD-PARTY-NOTICES.txt')]);
      const box = $('#lic-main', el);
      box.textContent = '';
      for (const block of main.split(/\n\s*\n/)) {
        const s = block.trim();
        if (!s) continue;
        const m = s.match(/^(#{1,2})\s+(.*)$/);
        const n = document.createElement(m ? (m[1].length === 1 ? 'h3' : 'h4') : 'p');
        n.textContent = (m ? m[2] : s).replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').replace(/^<(https?:[^>]+)>$/, '$1').replace(/\s*\n\s*/g, ' ');
        box.appendChild(n);
      }
      $('#lic-third', el).textContent = third;
    },
  };
}

const shortKey = (p) => (p ? p.slice(0, 5) + '…' : '');
function clockText(ms) {
  const s = Math.abs(ms) / 1000;
  if (s < 2) return html`<span class="badge ok">${icon('check')} in sync</span>`;
  const t = s < 120 ? `${s.toFixed(0)} s` : `${(s / 60).toFixed(0)} min`;
  const cls = s >= 300 ? 'sec' : s >= 30 ? 'upd' : 'mute';
  return html`<span class="badge ${cls}">agent is ${t} ${ms > 0 ? 'ahead' : 'behind'}</span>${s >= 300 ? html`<div class="small dim">More than 10 minutes apart and the agent's messages are refused.</div>` : ''}`;
}
async function pingModal(a) {
  let open = true;
  openModal(html`<h3>Ping ${a.name}</h3><div class="row muted" style="margin-top:12px"><span class="spin"></span>Waiting for the agent (up to 15 s)…</div>
    <div class="actions"><button class="btn ghost" data-act="close">Close</button></div>`, { acts: { close: () => closeModal() }, onClose: () => { open = false; } });
  let p;
  try {
    p = await POST(`agents/${a.id}/ping`);
  } catch (e) {
    if (open) closeModal();
    fail(e);
    return;
  }
  if (!open) return;
  const row = (k, v) => html`<tr><td class="dim" style="padding:6px 16px 6px 0;white-space:nowrap">${k}</td><td style="padding:6px 0">${v}</td></tr>`;
  let body;
  if (p.answered && !p.error) {
    body = html`<div class="notice green" style="margin:12px 0">${icon('check')} <b>The agent answered in ${p.rtt_ms < 1 ? 'under 1' : p.rtt_ms} ms.</b> Network and keys work.</div>
      <table>${row('Agent', p.version)}
      ${row('Connection key', html`<span class="mono small">${shortKey(p.key_used)}</span> ${p.key_current ? html`<span class="badge ok">current</span>` : html`<span class="badge upd">not the current key</span>`}`)}
      ${row('Key slots', html`<span class="mono small">${shortKey(p.slots[0]) || 'empty'} · ${shortKey(p.slots[1]) || 'empty'}</span> ${p.slots_ok ? html`<span class="badge ok">as expected</span>` : html`<span class="badge upd">being updated</span>`}`)}
      ${row('Clock', clockText(p.clock_ms))}</table>`;
  } else if (p.answered) {
    body = html`<div class="notice red" style="margin:12px 0">The agent answered with an error: ${p.error}</div>`;
  } else {
    body = html`<div class="notice red" style="margin:12px 0"><b>No answer within 15 s.</b></div>
      <table>${row('Status', p.online ? 'online (it was heard from recently)' : 'offline')}${row('Last contact', when(p.last_seen))}</table>
      <div class="small dim" style="margin-top:10px">Check that the device is running, can reach this master on the agent port, and that its clock is right.</div>`;
  }
  openModal(html`<h3>Ping ${a.name}</h3>${body}
    <div class="actions"><button class="btn ghost" data-act="job">Show job</button><button class="btn primary" data-act="close" autofocus>Close</button></div>`,
  { acts: { close: () => closeModal(), job: () => openJob(p.job_id) } });
}

// Maintenance button and notice, on both device pages.
function maintButton(a) {
  return ifChange(a.maint
    ? html`<button class="btn primary sm" data-act="maint-end" title="Restore all checks now">${icon('play')} End Maintenance</button>`
    : html`<button class="btn sm" data-act="maint-start" title="Pause all checks for a while; no failures are reported">${icon('pause')} Maintenance</button>`);
}
function maintNotice(a) {
  if (!a.maint) return '';
  const m = a.maint;
  return html`<div class="notice blue"><b>In maintenance</b> since ${hhmm(m.since)} by ${m.by}, ${m.until ? `until ${hhmm(m.until)} (ends by itself)` : 'until ended by hand'}.
    ${isLocal(a) ? '' : html`<span class="small">No automatic jobs run${a.checks ? ', its services are not checked' : ''} and no failures are shown. Actions on the device work only while the agent is connected${a.online ? '' : html` — <b>it is not connected now</b>`}.</span>`}</div>`;
}
async function maintStart(a, done) {
  openModal(html`<h3>Maintenance for ${a.name}</h3>
    <form class="form" data-submit="go">
      <div class="muted small">${isLocal(a)
        ? `The ping and all services stop${a.local.probe ? `, also on ${a.local.probe.name}` : ''}, and no failures are shown.`
        : `No automatic jobs run (waiting ones are cancelled)${a.checks ? `, its services stop${a.checks.probe ? `, also on ${a.checks.probe.name}` : ''},` : ''} and no failures are shown. Your own actions stay possible while the agent is connected.`}</div>
      <label class="field">How long<select name="min">
        <option value="5">5 minutes</option><option value="30">30 minutes</option><option value="60" selected>1 hour</option>
        <option value="240">4 hours</option><option value="0">Manual (until ended)</option></select></label>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">${icon('pause')} Start maintenance</button></div>
    </form>`, {
    acts: {
      cancel: () => closeModal(),
      go: async (f) => {
        await POST(`devices/${a.id}/maintenance`, { minutes: +f.min.value });
        closeModal();
        toast(`${a.name}: maintenance started`, 'ok');
        done();
      },
    },
  });
}
async function maintEnd(a, done) {
  await POST(`devices/${a.id}/maintenance/end`);
  toast(`${a.name}: maintenance ended — checks run again`, 'ok');
  done();
}

function AgentPage([id, tab]) {
  const st = { tab: ['updates', 'jobs', 'system', 'apps', 'services', 'proxmox'].includes(tab) ? tab : 'system', sel: new Set(), data: null, appsAll: false, pinging: false, checking: new Set() };
  const upd = () => st.data?.agent?.state?.updates || null;
  function header(a, groups) {
    const f = a.facts, cdown = agentChecksDown(a);
    const g = groups.find((x) => x.id === a.group_id);
    return html`
      <div class="page-head syshead">
        <div class="headinfo"><div class="row"><a href="#/dashboard/${lastDashSub}" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(a)}<h1>${a.name}</h1>
          ${a.status === 'report_only' ? html`<span class="badge info">${icon('pause')} report only</span>` : ''}${a.status === 'pending' ? html`<span class="badge upd">pending approval</span>` : ''}
          <span class="tag">${icon('folder')} ${g ? g.name : 'Ungrouped'}</span>${verPill(a)}</div>
          <div class="sub" title="${a.below_min ? '' : `${osLabel(f)} · ${f.kernel || ''} · ${f.arch} · ${f.virt}`}">${a.below_min ? html`agent ${a.core_version} · upgrade only · ${a.online ? 'online' : raw('offline, last seen ' + r(when(a.last_seen)))}` : html`${osLabel(f)} · ${f.kernel || ''} · ${f.arch} · ${f.virt} · <span title="${lastReboot(a)}">${a.online ? 'online, up ' + dur(f.uptime_sec) : raw('offline, last seen ' + r(when(a.last_seen)))}</span>`}</div>
          ${a.status !== 'pending' ? viaLine(a) : ''}
          ${cdown ? html`<div class="sub"><a href="#" class="c-red" data-act="tab" data-t="services">${checksDownText(cdown)}</a>${cdown.L.svc_fail ? html` · <span class="c-red">${cdown.L.svc_fail.name}: ${cdown.L.svc_fail.error || 'down'}</span>` : cdown.ping ? html` · <span class="c-red">ping: ${cdown.L.ping_info.last_error || 'no answer'}</span>` : ''}</div>` : ''}</div>
        <span class="spacer"></span>
        ${a.busy ? html`<span class="row muted"><span class="spin"></span>${a.busy} job(s) active</span>` : ''}
        ${!a.removing && !a.below_min ? html`<button class="btn sm" data-act="ping" title="Check the connection and keys with the agent now; nothing is done on the device">${icon('pulse')} Ping</button>` : ''}
        ${a.status !== 'pending' && !a.removing && canChange() ? html`<button class="btn sm" data-act="agent-edit" title="Name, Connected via and group">${icon('edit')} Edit</button>` : ''}
        ${a.status === 'active' && !a.below_min && canChange() ? html`<button class="btn sm" data-act="report-only">${icon('pause')} Report only</button>` : ''}
        ${a.status === 'report_only' && !a.below_min && canChange() ? html`<button class="btn sm" data-act="allow-changes">${icon('play')} Allow changes</button>` : ''}
        ${a.status === 'active' && !a.below_min && canChange() ? html`<button class="btn warn sm" data-act="reboot" ${a.online ? '' : 'disabled'}
          title="${a.online ? 'Reboot this device' : maintBlocked(a) ? MAINT_OFF : 'The device is offline'}">${icon('refresh')} Reboot</button>` : ''}
        ${a.status !== 'pending' && (!a.is_master || a.maint) ? maintButton(a) : ''}
      </div>
      ${maintNotice(a)}${unrNotice(a)}
      ${a.below_min ? html`<div class="notice row"><span><b>Agent update required.</b> Agent ${a.core_version} is older than the lowest allowed version ${a.min_agent}:
        the master stores and shows none of this device's data and runs nothing on it until the agent is updated.</span><span class="spacer"></span>
        ${ifChange(html`<button class="btn primary sm" data-act="agent-update" ${a.online ? '' : 'disabled'}>${icon('up')} Update agent to ${a.core_latest}</button>`)}</div>` : ''}
      ${a.status === 'report_only' && !a.below_min ? html`<div class="notice blue"><b>Report only:</b> this device only reports.
        <span class="small">${a.agent_report_only ? html`${icon('check')} Confirmed by the agent.`
          : html`Waiting for the agent to confirm${a.online ? '…' : ' (the device is offline)'}.`}</span></div>` : ''}
      ${a.status === 'active' && a.agent_report_only ? html`<div class="notice blue small">Changes allowed — waiting for the agent to confirm${a.online ? '…' : ' (the device is offline)'}.</div>` : ''}
      ${a.status === 'pending' ? html`<div class="notice blue row">This device enrolled ${when(a.enrolled_at)} and is waiting for approval. Until approved it receives no commands.<span class="spacer"></span>${ifChange(html`<button class="btn primary sm" data-act="approve">${icon('check')} Approve</button>`)}</div>` : ''}
      ${a.removing ? html`<div class="notice red">Uninstall requested — the agent will remove itself.</div>` : ''}
      ${conflictNotice(a)}`;
  }
  function updatesTab(a) {
    const s = upd();
    const canAct = a.status === 'active' && a.online;
    const ed = canChange() && a.status === 'active';
    if (!s) {
      return html`<div class="panel empty">${a.status === 'report_only' ? html`<div class="big">⏸</div><div>Report only: no update checks are run on this device.</div>`
        : html`<div class="spin" style="width:26px;height:26px"></div><div style="margin-top:12px">Waiting for the first check…</div>
        ${ed ? html`<div style="margin-top:14px"><button class="btn" data-act="check" ${canAct ? '' : 'disabled'}>${icon('refresh')} Check now</button></div>` : ''}`}</div>`;
    }
    const pk = s.packages || [];
    const selectable = pk.filter((p) => !p.new);
    for (const n of [...st.sel]) if (!selectable.some((p) => p.name === n)) st.sel.delete(n);
    const nsel = st.sel.size;
    const allOn = selectable.length && selectable.every((p) => st.sel.has(p.name));
    return html`
      <div class="panel panel-pad" style="margin-bottom:16px">
        <div class="row">
          ${s.total ? html`<span class="badge upd">${s.total} update${s.total > 1 ? 's' : ''}</span>` : html`<span class="badge ok">${icon('check')} up to date</span>`}
          ${s.security ? html`<span class="badge sec">${s.security} security</span>` : ''}
          ${s.reboot_required ? html`<span class="badge reboot" title="${(s.reboot_pkgs || []).join(', ')}">reboot required${s.reboot_pkgs?.length ? ': ' + s.reboot_pkgs.slice(0, 3).join(', ') : ''}</span>` : ''}
          <span class="muted small">${when(s.checked_at, 'checked ')} · ${when(s.refreshed_at, 'package lists refreshed ')} · ${s.backend}</span>
          <span class="spacer"></span>
          ${ed ? html`<button class="btn sm" data-act="check" ${canAct ? '' : 'disabled'}>${icon('refresh')} Refresh &amp; check</button>
          <button class="btn primary sm" data-act="install" ${canAct && nsel ? '' : 'disabled'}>${icon('download')} Install selected${nsel ? ` (${nsel})` : ''}</button>
          <button class="btn warn sm" data-act="upgrade-all" ${canAct && s.total ? '' : 'disabled'}>${icon('zap')} Upgrade all</button>` : ''}
        </div>
      </div>
      ${s.removals?.length ? html`<div class="notice red">“Upgrade all” would remove: <b>${s.removals.join(', ')}</b></div>` : ''}
      ${a.status === 'report_only' ? html`<div class="notice blue">Report only: this list is from the last check before the device was set to report only, ${when(s.checked_at)}.</div>` : ''}
      ${pk.length ? html`
      <div class="panel">
        ${ed ? html`<div class="row panel-pad" style="padding-bottom:10px">
          <span class="muted small">Quick select:</span>
          <button class="btn ghost sm" data-act="sel-sec">Security only</button>
          <button class="btn ghost sm" data-act="sel-all">All</button>
          <button class="btn ghost sm" data-act="sel-none">None</button>
        </div>` : ''}
        <div class="tbl-wrap" data-keep-scroll="pk"><table class="tbl">
          <thead><tr>${ed ? html`<th class="w1"><input type="checkbox" data-act="sel-toggle-all" ${allOn ? 'checked' : ''}></th>` : ''}<th>Package</th><th>Version</th><th>Origin</th><th class="w1"></th></tr></thead>
          <tbody>${pk.map((p) => html`
            <tr class="${st.sel.has(p.name) ? 'picked' : ''}">
              ${ed ? html`<td class="w1">${p.new ? '' : html`<input type="checkbox" data-act="sel-pkg" data-name="${p.name}" ${st.sel.has(p.name) ? 'checked' : ''}>`}</td>` : ''}
              <td><b>${p.name}</b> <span class="dim small">${p.arch}</span></td>
              <td class="mono">${p.current ? html`<span class="ver">${p.current}</span><span class="arrow">→</span>` : ''}<span class="ver new">${p.available}</span></td>
              <td class="muted small">${p.origin}</td>
              <td class="w1"><div class="row" style="flex-wrap:nowrap;gap:6px">${p.security ? html`<span class="badge sec">security</span>` : ''}${p.new ? html`<span class="badge new" title="Pulled in as a dependency; installed together with its parent">new dependency</span>` : ''}${fixes(p.name)}</div></td>
            </tr>`)}</tbody></table></div>
      </div>` : noUpdates(a)}`;
  }
  function fixes(pkg) {
    const l = st.data?.agent?.vulns?.update_fixes?.[pkg];
    return l?.length ? html`<span class="badge sec" title="${l.join(', ')}">fixes ${l.length} CVE</span>` : '';
  }
  function noUpdates(a) {
    const o = a.os_support, os = osLabel(a.facts || {});
    if (o?.status === 'expired') {
      return html`<div class="notice red"><b>No updates, but ${os} is no longer supported</b> (support ended ${ymd(o.end)}).<br>
        It no longer receives security updates, so an empty list doesn't mean the device is safe. Upgrade to a supported release.</div>`;
    }
    return html`<div class="panel empty"><div class="big">✨</div>Everything is up to date.${o?.status === 'soon'
      ? html`<div class="c-amber small" style="margin-top:10px">Support for ${os} ends on ${ymd(o.end)}.</div>` : ''}</div>`;
  }
  function agentPanel(a) {
    const kv = [['Agent ID', a.id], ['Enrolled', fmtDate(a.enrolled_at)], ['Last seen', fmtDate(a.last_seen, true)]];
    return html`
      <div class="panel panel-pad">
        <h2>Agent</h2>
        <div class="row">${verPill(a)}${a.core_latest ? html`<span class="muted small">latest available: <span class="ver">${a.core_latest}</span></span>` : ''}</div>
        ${a.desired_core ? html`<div class="notice blue" style="margin:14px 0 0">Change to ${a.desired_core} requested${a.online ? ' — the agent downloads the signed binary and restarts.' : ' — applies as soon as the device is online.'}</div>` : ''}
        <div class="facts" style="margin-top:14px">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v || '—'}</div></div>`)}</div>
        <div class="row" style="margin-top:14px"><span class="muted small">Functions:</span>${[...(a.features || []), ...(a.probe ? ['probe'] : [])].map((n) => html`<span class="tag mono">${n}</span>`)}</div>
      </div>`;
  }
  function jobsTab() {
    const jobs = st.data.jobs;
    if (!jobs.length) return html`<div class="panel empty">No jobs yet.</div>`;
    return html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
      <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}</div></td><td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td><td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>`;
  }
  function appsTab(a) {
    let s = null;
    try { s = a.state?.apps ? (typeof a.state.apps === 'string' ? JSON.parse(a.state.apps) : a.state.apps) : null; } catch { s = null; }
    const busy = (st.data.jobs || []).some((j) => j.feature === 'apps' && (j.status === 'queued' || j.status === 'running'));
    const btn = ifChange(html`<button class="btn sm" data-act="apps-scan" ${a.online && !busy ? '' : 'disabled'} title="${a.online ? 'Scan the installed applications now' : 'The device is offline'}">
      ${busy ? html`<span class="spin"></span>` : icon('refresh')} Refresh now</button>`);
    const vul = a.vulns;
    const all = s?.apps || [], nInst = all.filter((x) => x.installed).length;
    const sw = all.length ? html`<div class="chips"><button class="chip ${st.appsAll ? '' : 'on'}" data-act="apps-view" data-v="0">Installed<span class="n">${nInst}</span></button><button class="chip ${st.appsAll ? 'on' : ''}" data-act="apps-view" data-v="1">All<span class="n">${all.length}</span></button></div>` : '';
    const head = html`<div class="row" style="margin-bottom:12px">${sw}<span class="muted small">${s?.scanned_at ? html`Scanned ${when(s.scanned_at)} · from the package manager` : 'Not scanned yet'}${vul?.checked_at ? html` · CVEs looked up ${when(vul.checked_at)}` : ''}</span><span class="spacer"></span>${btn}</div>
      ${vul?.status === 'old_agent' ? html`<div class="notice blue">The CVE check needs agent 0.3.6 or later on this device. <a href="#/settings/releases">Update the agent</a>.</div>` : ''}
      ${vul?.status === 'unsupported' ? html`<div class="notice">${a.os_support?.status === 'expired'
        ? `${osLabel(a.facts)} is no longer supported: no security data and no fixes are published, so it can't be checked. Upgrade to a supported release.`
        : `OSV.dev has no vulnerability data for ${osLabel(a.facts)}, so the applications cannot be checked for CVEs.`}</div>` : ''}`;
    const security = (x) => {
      const v = vul?.apps?.[x.name];
      if (!v || vul.status === 'unsupported') return html`<span class="dim">—</span>`;
      const dist = a.facts.os_id === 'ubuntu' ? 'Ubuntu' : 'Debian';
      switch (v.status) {
        case 'old_agent': return html`<span class="dim" title="Needs agent 0.3.6 or later">—</span>`;
        case 'manual': return html`<span class="dim" title="Not installed from a package source, so not checked">not checked</span>`;
        case 'not_distro': return html`<span class="dim" title="From ${v.origin}, not from ${dist}: ${dist}'s security data does not cover it, so not checked">not checked</span>`;
        case 'pending': return html`<span class="dim" title="Being looked up">…</span>`;
        case 'ok': return html`<span class="badge ok xs" title="No known vulnerabilities for this version${v.minor ? ` (${v.minor} minor)` : ''}">ok</span>`;
      }
      const tip = [v.fixable ? `${v.fixable} fixed by an update` : '', v.open ? `${v.open} without a fix yet` : '', v.minor ? `${v.minor} minor` : '', v.kev ? `${v.kev} actively exploited` : ''].filter(Boolean).join(', ');
      return html`<button type="button" class="linkbtn" data-act="cves" data-app="${x.name}" title="${tip} — click for the list">
        ${v.fixable ? html`<span class="badge ${v.red ? 'sec' : 'upd'} xs">${v.fixable} fixable</span>` : ''}
        ${v.open ? html`<span class="badge ${v.kev && !v.fixable ? 'sec' : 'mute'} xs">${v.open} without fix</span>` : ''}
        ${!v.fixable && !v.open ? html`<span class="badge sec xs">${v.kev} exploited</span>` : ''}</button>`;
    };
    if (!s?.apps?.length) {
      return html`${head}<div class="panel empty"><div>No scan yet — it runs with the next update check${a.status === 'report_only' ? ' (not on report-only devices: use Refresh now)' : ''}.</div></div>`;
    }
    const boot = (x) => (x.boot === 'enabled' ? 'starts at boot' : x.running === 'running' ? 'started manually (does not start at boot)' : 'does not start at boot');
    const running = (x) => (!x.running ? html`<span class="dim">—</span>`
      : x.running === 'running' ? html`<span class="badge ok xs" title="${boot(x)}">running</span>`
        : html`<span class="badge mute xs" title="${boot(x)}">stopped</span>`);
    const ports = (x) => (x.ports?.length
      ? html`<span title="${x.ports.map((p) => `${p.port}/${p.proto} on ${p.addr}`).join('\n')}">${x.ports.map((p, i) => html`${i ? ', ' : ''}${p.port}/${p.proto}${p.local ? html` <span class="dim">(local)</span>` : ''}`)}</span>`
      : html`<span class="dim">—</span>`);
    const rows = st.appsAll ? all : all.filter((x) => x.installed);
    if (!rows.length) {
      return html`${head}<div class="panel empty"><div>None of the ${all.length} monitored applications were found on this device.</div>
        <div style="margin-top:10px"><button type="button" class="linkbtn" data-act="apps-view" data-v="1">Show all</button></div></div>`;
    }
    return html`${head}<div class="panel scrollx"><table class="tbl"><thead><tr><th>Application</th><th>Installed</th><th>Version</th><th title="Known vulnerabilities (CVEs) of the installed version">Security</th><th>Running</th><th>Listening on</th></tr></thead>
      <tbody>${rows.map((x) => x.installed
        ? html`<tr><td><b>${x.name}</b></td><td><span class="badge ok xs">yes</span></td><td class="mono small" title="${x.full}${x.package ? ' (' + x.package + ')' : ''}${x.origin ? ' from ' + x.origin : ''}">${x.version || '—'}</td>
            <td class="nowrap">${security(x)}</td><td>${running(x)}</td><td class="mono small">${ports(x)}</td></tr>`
        : html`<tr class="dimrow"><td>${x.name}</td><td><span class="dim small">not detected</span></td><td></td><td></td><td></td><td></td></tr>`)}</tbody></table></div>`;
  }
  function systemTab(a) {
    const f = a.facts, h = f.health;
    const kv = [
      ['Hostname', f.hostname], ['Operating system', osLabel(f)], ['Kernel', f.kernel], ['Architecture', `${f.os}/${f.arch}`],
      ['Virtualization', f.virt], ['Platform', platformLabel(f)], ['Uptime', a.online ? dur(f.uptime_sec) : a.maint ? html`<span class="c-blue">not connected (maintenance)</span>` : html`<span class="flash-off">offline</span>`], ['CPUs', f.cpus], ['Memory (assigned)', bytes(f.mem_total)],
      ['OS support', a.os_support ? html`${a.os_support.status === 'expired' ? html`<span class="badge sec xs">EOL</span> ` : a.os_support.status === 'soon' ? html`<span class="badge upd xs">EOL soon</span> ` : ''}${osSupportText(a.os_support, f.os_id)}` : '—'],
      ['Failed services', !h || h.failed_count < 0 ? '—' : h.failed_count === 0 ? 'none'
        : html`<span class="badge sec xs">${h.failed_count} failed</span> ${(h.failed || []).join(', ')}${h.failed_count > (h.failed || []).length ? ', …' : ''}`],
      ['Automatic updates', ({ on: html`<span class="badge info xs">on</span> unattended-upgrades`, off: 'off', not_installed: 'not installed' })[h?.auto_updates] || '—'],
      ['Last upgrade', h?.last_upgrade ? `${fmtDate(h.last_upgrade.time)} (${ago(h.last_upgrade.time)}, ${h.last_upgrade.packages} package${h.last_upgrade.packages === 1 ? '' : 's'}) · by ${upgradedBy(h.last_upgrade.by)}` : h ? 'none recorded' : '—'],
    ];
    return html`<div class="panel panel-pad" style="margin-bottom:16px"><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v ?? '—'}</div></div>`)}</div></div>${networkPanel(f)}${metricGraphs(a)}${agentPanel(a)}${probePanel(a)}`;
  }
  // Network: the main card, every address and how it was set, gateway and DNS.
  function networkPanel(f) {
    const n = f.nic;
    const card = n ? html`<span class="mono">${n.iface}</span>${n.phys ? html` on <span class="mono">${n.phys}</span>` : ''}${n.virtual ? html` <span class="dim small">(virtual card)</span>` : ''}` : null;
    const origin = { static: 'static', dhcp: 'DHCP', slaac: 'SLAAC' };
    const addrs = f.addrs?.length
      ? html`${f.addrs.map((x) => html`<div><span class="mono">${x.ip}${x.prefix ? '/' + x.prefix : ''}</span> <span class="badge ${x.origin === 'static' ? 'mute' : 'info'} xs">${origin[x.origin] || 'unknown'}</span>${x.iface && (!n || x.iface !== n.iface) ? html` <span class="dim small">${x.iface}</span>` : ''}</div>`)}`
      : (f.ips || []).length ? html`${f.ips.map((ip) => html`<div class="mono">${ip}</div>`)}` : null;
    const kv = [
      ['Network card', card], ['MAC address', n ? html`<span class="mono">${n.mac}</span>` : null],
      ['IP addresses', addrs],
      ['IPv4 gateway', f.gateway ? f.gateway + (f.gateway_if ? ` (${f.gateway_if})` : '') : null],
      ['DNS servers', f.dns?.length ? f.dns.join(', ') + (f.dns_via ? ` (via ${f.dns_via})` : '') : null],
    ];
    return html`<div class="panel panel-pad" style="margin-bottom:16px"><h2 style="margin:0 0 12px">Network</h2><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v ?? '—'}</div></div>`)}</div>
      ${!n && !f.addrs ? html`<div class="dim small" style="margin-top:10px">The network card and how the addresses were set are shown from agent 0.3.16 on.</div>` : ''}</div>`;
  }
  function proxmoxTab(a) {
    return html`<div class="panel panel-pad"><div class="facts"><div><div class="k">Platform</div><div class="v">${platformLabel(a.facts)}</div></div></div>
      <div class="dim small" style="margin-top:14px">More Proxmox information will be shown here.</div></div>`;
  }
  // Remote probe: the agentless devices this agent checks for the master.
  function probePanel(a) {
    const p = a.probe;
    if (!p) return '';
    const busy = (st.data.jobs || []).find((j) => (j.type === 'probe.list' || j.type === 'probe.clear') && (j.status === 'queued' || j.status === 'running'));
    const state = p.cleared ? html`<span class="badge mute">cleared</span> <span class="muted small">no check list is sent until you resend it or the checks change</span>`
      : !p.want && !p.running ? html`<span class="muted small">no devices to check</span>`
        : p.running === p.want ? html`<span class="badge ok">${icon('check')} check list confirmed</span> <span class="muted small">${when(p.confirmed_at)}</span>`
          : html`<span class="badge upd">check list not confirmed yet</span>${a.online ? '' : html` <span class="muted small">(the device is offline)</span>`}`;
    const sw = html`<label class="checkrow" style="margin:0"><input type="checkbox" data-act="probe-on" ${p.on ? 'checked' : ''} ${canChange() ? '' : 'disabled'}> Act as remote probe</label>`;
    return html`<div class="panel panel-pad" style="margin-top:16px">
      <div class="row"><h2 style="margin:0">Remote probe</h2><span class="spacer"></span>
        ${ifChange(html`<button class="btn sm" data-act="probe-resend" ${busy ? 'disabled' : ''} title="Send the current check list again, even if the agent has it">${icon('refresh')} Resend check list</button>
        <button class="btn warn sm" data-act="probe-clear" ${busy ? 'disabled' : ''} title="The agent drops its whole check list and waits for a new one">${icon('trash')} Clear probe checks</button>`)}</div>
      <div class="row" style="margin-top:10px">${sw}</div>
      ${p.on ? html`<div class="muted small" style="margin:8px 0 12px">This agent checks devices for the master, for networks the master cannot reach: agentless devices, and the services of other agent devices. Choose it as “Checked by” in a device's Edit window or on a Services tab. It can be switched off once no device is checked by it.</div>
      <div class="row">${state}</div>` : html`<div class="muted small" style="margin-top:8px">Off: this agent is not offered as “Checked by” for agentless devices. Switch it on to let it check devices for networks the master cannot reach.</div>`}
      ${p.on ? html`
      <div class="facts" style="margin-top:14px"><div><div class="k">Devices</div><div class="v">${p.devices}</div></div><div><div class="k">Checks</div><div class="v">${p.checks} of at most ${p.max}</div></div>
        <div><div class="k">List version</div><div class="v mono small">${p.running || '—'}</div></div></div>` : ''}
      ${busy ? html`<div class="dim small" style="margin-top:10px"><span class="spin" style="width:10px;height:10px"></span> ${busy.type === 'probe.clear' ? 'Clear sent, not yet confirmed' : 'Check list sent, not yet confirmed'}…</div>` : ''}
    </div>`;
  }
  // Services checked from outside, like those of an agentless device.
  function servicesTab(a) {
    const L = a.checks, out = L?.probe && L.probe.state !== 'ok' ? L.probe : null;
    const head = html`<div class="row" style="margin-bottom:12px"><span class="muted small">${L && L.address
      ? html`Checked on <span class="mono">${L.address}</span> ${L.probe ? html`by the agent <a href="#/device/${L.probe.id}"><b>${L.probe.name}</b></a>, on its own schedule` : 'from the master'}.${(L.services || []).some((s) => s.metric) ? ' Disk, memory and load are measured by the agent itself.' : ''}`
      : L ? 'Disk, memory and load are measured by the agent itself, every minute. Other services are checked from outside, by the master or another agent.'
      : 'Services of this device checked from outside, by the master or another agent, as for an agentless device. The device\'s own status still comes from its agent.'}</span>
      <span class="spacer"></span>${ifChange(html`<button class="btn sm" data-act="checks-edit">${icon('edit')} Address and checker</button><button class="btn primary sm" data-act="svc-add">${icon('plus')} Add service</button>`)}</div>
      ${out && !a.maint ? html`<div class="notice grey"><b>Unknown – ${out.note}.</b> The shown results are the last ones received from <a href="#/device/${out.id}">${out.name}</a>.</div>` : ''}`;
    return html`${head}${servicesTable(st, L)}`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const a = d.agent;
    setHTML($('#a-head'), header(a, d.groups));
    const tabs = a.below_min ? [['system', 'System']] : [['system', 'System'], ['updates', 'Updates'], ['jobs', 'Jobs'], ['apps', 'Applications']];
    if (a.status !== 'pending') tabs.push(['services', 'Services']);
    if (!a.below_min && a.facts?.platform?.id === 'pve') tabs.push(['proxmox', 'Proxmox']);
    if (!tabs.some(([k]) => k === st.tab)) st.tab = tabs[0][0];
    const C = a.checks, cdown = agentChecksDown(a), ctotal = C ? (C.services || []).length + (C.ping_info ? 1 : 0) : 0;
    setHTML($('#a-tabs'), tabs.map(([k, l]) => html`<button class="${st.tab === k ? 'on' : ''}" data-act="tab" data-t="${k}">${l}${k === 'updates' && a.updates?.total ? html` <span class="badge upd">${a.updates.total}</span>` : ''}${k === 'system' && !a.core_is_latest && a.core_latest ? html` <span class="badge upd" title="A newer agent version is available">↑</span>` : ''}${k === 'services' ? (cdown ? html` <span class="badge sec">${cdown.n}</span>` : ctotal ? html` <span class="dim small">${ctotal}</span>` : '') : ''}</button>`));
    const body = st.tab === 'services' ? servicesTab(a) : a.below_min ? agentPanel(a) : { updates: updatesTab, jobs: jobsTab, system: systemTab, apps: appsTab, proxmox: proxmoxTab }[st.tab](a);
    setHTML($('#a-body'), body);
  }
  const a = () => st.data.agent;
  const refresh = () => view?.refresh?.();
  return {
    interval: 4000,
    mount(el) {
      el.innerHTML = '<div id="a-head"></div><div class="tabs" id="a-tabs"></div><div id="a-body"></div>';
    },
    async refresh() {
      try { st.data = await GET('agents/' + id); } catch (e) { if (/not found/.test(e.message)) { location.hash = '#/'; return; } throw e; }
      render();
    },
    acts: {
      ...serviceActs(st, id, render),
      'checks-edit': () => checksModal(a(), (v) => { st.data.agent = v; render(); }),
      tab: (t) => { st.tab = t.dataset.t; render(); },
      'apps-view': (t) => { st.appsAll = t.dataset.v === '1'; render(); },
      'agent-update': async () => {
        const res = await POST(`agents/${id}/core`, {});
        if (res.job) openJob(res.job); else toast('Update requested — it runs when the device is online', 'ok');
        refresh();
      },
      check: () => runAction([id], 'updates', 'check', {}),
      'apps-scan': async () => { await runAction([id], 'apps', 'scan', {}, { open: false }); refresh(); },
      cves: (t) => { const v = st.data?.agent?.vulns?.apps?.[t.dataset.app]; if (v?.cves?.length) openCves(t.dataset.app, v); },
      install: async () => {
        const pk = [...st.sel];
        if (!pk.length) return;
        if (await confirmBox(`Install ${pk.length} update(s) on ${a().name}?`, pk.join(', '), { okText: 'Install' })) {
          await runAction([id], 'updates', 'install', { packages: pk });
          st.sel.clear();
        }
      },
      'upgrade-all': async () => {
        const s = upd();
        const extra = s?.removals?.length ? ` It will also REMOVE: ${s.removals.join(', ')}.` : '';
        if (await confirmBox(`Upgrade all packages on ${a().name}?`, `${s?.total || 0} package(s) will be upgraded.${extra}`, { okText: 'Upgrade all', danger: true })) {
          await runAction([id], 'updates', 'upgrade_all', {});
        }
      },
      'sel-pkg': (t) => { t.checked ? st.sel.add(t.dataset.name) : st.sel.delete(t.dataset.name); render(); },
      'sel-toggle-all': (t) => { const pk = (upd()?.packages || []).filter((p) => !p.new); st.sel = new Set(t.checked ? pk.map((p) => p.name) : []); render(); },
      'sel-all': () => { st.sel = new Set((upd()?.packages || []).filter((p) => !p.new).map((p) => p.name)); render(); },
      'sel-sec': () => { st.sel = new Set((upd()?.packages || []).filter((p) => !p.new && p.security).map((p) => p.name)); render(); },
      'sel-none': () => { st.sel.clear(); render(); },
      approve: () => approveModal(a(), st.data.groups),
      'report-only': async () => {
        if (!(await confirmBox(`Set ${a().name} to report only?`, 'The device keeps reporting, but gets no update checks, installs or reboots until you allow changes again. Queued jobs are cancelled; agent version changes stay possible.', { okText: 'Report only' }))) return;
        await POST('agents/' + id, { report_only: true }); toast('Report only', 'ok'); refresh();
      },
      'allow-changes': async () => {
        if (!(await confirmBox(`Allow changes on ${a().name}?`, 'Update checks, installs and reboots become possible again, and the automatic update check resumes.', { okText: 'Allow changes' }))) return;
        await POST('agents/' + id, { report_only: false }); toast('Changes allowed', 'ok'); refresh();
      },
      ping: () => pingModal(a()),
      'agent-edit': async () => agentEditModal(await GET('overview'), a(), refresh),
      'probe-clear': async () => {
        const p = a().probe;
        if (!(await confirmBox(`Clear the probe checks of ${a().name}?`, html`The agent drops its whole check list and any results it has not sent yet, and then waits.
          ${p.devices ? html`Its ${p.devices} device${p.devices === 1 ? '' : 's'} show “unknown” until ` : 'Nothing is checked until '}you press “Resend check list” or the checks of its devices change.`, { okText: 'Clear probe checks', danger: true }))) return;
        await POST(`agents/${id}/probe/clear`); toast('Clear sent to the agent', 'ok'); refresh();
      },
      'maint-start': () => maintStart(a(), refresh),
      'maint-end': () => maintEnd(a(), refresh),
      'probe-on': async (t) => {
        const on = t.checked;
        try {
          await POST(`agents/${id}/probe/enabled`, { on });
          toast(`${a().name}: remote probe ${on ? 'on' : 'off'}`, 'ok');
        } catch (e) {
          t.checked = !on; // refused: back to how it was
          throw e;
        } finally { refresh(); }
      },
      'probe-resend': async () => {
        await POST(`agents/${id}/probe/resend`); toast('Check list sent to the agent', 'ok'); refresh();
      },
      reboot: async () => {
        const ag = a();
        const body = html`The device restarts within a few seconds and shows offline until it is back.
          ${ag.is_master ? html`<div class="notice red" style="margin:14px 0 0"><b>This is the device the Coremon master runs on.</b> The website is unavailable for a minute or two until it has restarted; this page reconnects by itself.</div>` : ''}`;
        if (!(await confirmBox(`Reboot ${ag.name} now?`, body, { okText: 'Reboot', danger: true }))) return;
        await runAction([id], 'system', 'reboot', {}, { open: false });
      },
    },
  };
}
// ---- local devices: monitored by the master with a ping ----

function conflictNotice(a) {
  const c = a.ip_conflicts || [];
  if (!c.length) return '';
  return html`<div class="notice red">${c.map((x, i) => html`${i ? html`<br>` : ''}IP ${x.ip} is also used by <a href="#/device/${x.id}"><b>${x.name}</b></a>.`)}
    A device should be monitored once, with one type of monitoring: remove one of them.</div>`;
}
async function localEditModal(a, groups, done) {
  const isNew = !a;
  let pa = { agents: [], max: 200 };
  try { pa = await GET('probe-agents'); } catch { /* only Master can be chosen */ }
  const ov = await GET('overview');
  const cur = a?.local.probe?.id || '';
  openModal(html`<h3>${isNew ? 'Add an agentless device' : `Edit ${a.name}`}</h3>
    <form class="form" data-submit="save">
      <label class="field">Name<input type="text" name="name" value="${a?.name || ''}" maxlength="64" placeholder="e.g. printer-hall" required ${isNew ? raw('autofocus') : ''}></label>
      <label class="field">IP address<input type="text" name="address" value="${a?.local.address || ''}" placeholder="e.g. 192.168.5.20" required spellcheck="false" autocomplete="off"></label>
      <div class="dim small">IPv4 or IPv6. No host names: Coremon does not rely on DNS. Each address can belong to one device only, per checking system.${isNew ? '' : ' A new address starts all checks over.'}</div>
      <div class="field">Checked by${pick('probe', probeItems(pa, cur, a?.local.probe?.name), cur)}</div>
      <div class="dim small">Master: the master checks the device itself. An agent: that agent checks it, for a network the master cannot reach; its results come in with the agent's contact. ${isNew ? '' : 'The history is kept when this changes.'}
        Only agents with Remote probe switched on are listed; switch it on on the agent's device page.</div>
      <div class="field">Connected via${pick('via', viaItems(ov, a?.id), a?.via?.id || '')}</div>
      <div class="dim small">${VIA_HINT}</div>
      <label class="field">Group<select name="group">${groupOptions(groups, a?.group_id || '')}</select></label>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">${isNew ? html`${icon('plus')} Add device` : 'Save'}</button></div>
    </form>`, {
    acts: {
      cancel: () => closeModal(),
      save: async (f) => {
        const body = { name: f.name.value, address: f.address.value.trim(), group_id: f.group.value, probe: f.probe.value, via: f.via.value };
        const res = await POST(isNew ? 'locals' : `locals/${a.id}`, body);
        closeModal();
        toast(isNew ? `${res.name} added` : 'Saved', 'ok');
        done?.(res);
      },
    },
  });
}
// Edit an agent device: display name, Connected via and group (ov: the overview).
function agentEditModal(ov, ag, done) {
  openModal(html`<h3>Edit ${ag.name}</h3><form class="form" data-submit="save">
    <label class="field">Display name<input type="text" name="name" value="${ag.name}" maxlength="64" autofocus required></label>
    <div class="dim small">Host name on the device: ${ag.facts?.hostname || '—'}. Only the name shown in Coremon changes.</div>
    <div class="field">Connected via${pick('via', viaItems(ov, ag.id), ag.via?.id || '')}</div>
    <div class="dim small">${VIA_HINT}</div>
    <label class="field">Group<select name="group">${groupOptions(ov.groups || [], ag.group_id || '')}</select></label>
    <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div></form>`, {
    acts: {
      cancel: () => closeModal(),
      save: async (f) => { await POST('agents/' + ag.id, { name: f.name.value, via: f.via.value, group_id: f.group.value }); closeModal(); toast('Saved', 'ok'); done?.(); },
    },
  });
}
function virtualEditModal(ov, v, done) {
  openModal(html`<h3>Edit ${v.name}</h3><form class="form" data-submit="save">
    <label class="field">Name<input type="text" name="name" value="${v.name}" maxlength="64" autofocus required></label>
    <div class="field">Connected via${pick('via', viaItems(ov, v.id), v.via?.id || '')}</div>
    <div class="dim small">A virtual device is not checked: it passes on the state of the devices above it.</div>
    <label class="field">Group<select name="group">${groupOptions(ov.groups, v.group_id || '')}</select></label>
    <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div></form>`, {
    acts: {
      cancel: () => closeModal(),
      save: async (f) => { await POST(`virtuals/${v.id}`, { name: f.name.value, via: f.via.value, group_id: f.group.value }); closeModal(); toast('Saved', 'ok'); done?.(); },
    },
  });
}
// Upstream state in words: "Online · 3 of 3 answering · 14 ms".
const upWord = { ok: 'Online', partial: 'Partly online', down: 'Offline', wait: 'Waiting for the first ping', off: 'Not set up' };
const upClass = { ok: 'c-green', partial: 'c-amber', down: 'c-red', wait: 'dim', off: 'dim' };
function upText(u) {
  if (!u) return '';
  const ms = u.rtt_us ? ` · ${fmtMs(u.rtt_us)}` : '';
  return html`<span class="${upClass[u.state] || 'dim'}">${upWord[u.state] || u.state}</span>${u.state === 'ok' || u.state === 'partial' || u.state === 'down' ? html`<span class="dim"> · ${u.answering} of ${u.total} answering${ms}</span>` : ''}`;
}
// The Upstream tile of the Overview (Settings → Upstream).
function upTile(u) {
  if (!u || !u.show_tile) return '';
  if (!u.enabled) {
    return html`<a class="panel stat c-cyan click" href="#/settings/upstream" style="text-decoration:none;color:inherit"><div class="label">${u.name}</div><div class="value dim" style="font-size:20px">Not set up</div>
      <div class="statnote">Monitor the internet connection: Settings → Upstream</div></a>`;
  }
  const cls = { ok: 'c-green', partial: 'c-amber', down: 'c-red' }[u.state] || 'c-cyan';
  const big = { ok: 'Online', partial: 'Partly online', down: 'Offline' }[u.state] || 'Waiting';
  const ms = u.rtt_us ? ` · ${fmtMs(u.rtt_us)}` : '';
  return html`<div class="panel stat ${cls}" title="${u.addrs.map((a) => `${a.addr}: ${!a.at ? 'waiting' : a.ok ? fmtMs(a.rtt_us) : 'no answer'}`).join('\n')}"><div class="label">${u.name}</div>
    <div class="value ${u.state === 'ok' ? 'upv-ok' : ''}">${big}</div>
    <div class="statnote">${u.state === 'wait' ? 'first ping within a minute' : `${u.answering} of ${u.total} answering${ms}`}</div>
    ${u.state === 'down' && u.since ? html`<div class="statnote"><span class="c-red">offline for ${dur(now() - u.since)}</span></div>` : ''}
    ${u.state === 'partial' && u.since ? html`<div class="statnote"><span class="c-amber">since ${dur(now() - u.since)}</span></div>` : ''}</div>`;
}
const fmtMs = (us) => (us >= 10000 ? `${Math.round(us / 1000)} ms` : `${(us / 1000).toFixed(1)} ms`);
async function localDelete(a, done) {
  if (!(await confirmBox(`Delete ${a.name}?`, 'It is no longer checked, and its history is removed. This cannot be undone.', { okText: 'Delete device', danger: true }))) return false;
  await POST(`locals/${a.id}/delete`);
  toast(`${a.name} deleted`, 'ok');
  done?.();
  return true;
}
// The last 24 hours in 96 quarters of an hour; word: 'ping' or 'check'.
function uptimeBar(hist, { word = 'ping', compact = false } = {}) {
  const slot = 900, end = Math.floor(now() / slot) * slot, by = new Map((hist || []).map((h) => [h.t, h]));
  const cells = [];
  for (let i = 95; i >= 0; i--) {
    const t = end - i * slot, h = by.get(t);
    // Unreachable: every failure of the slot happened while a device above it was down.
    const unr = h && h.ok < h.n && (h.u || 0) >= h.n - h.ok;
    const cls = !h ? 'none' : h.ok === h.n ? 'ok' : unr ? 'unr' : h.ok === 0 ? 'bad' : 'part';
    const range = `${fmtDate(t).split(' ')[1]}–${fmtDate(t + slot).split(' ')[1]}`;
    const why = (h && h.f && h.ok < h.n ? `, failed: ${stepLabel(h.f)}` : '') + (h?.u ? `, ${h.u} while unreachable behind a device that was down or in maintenance` : '');
    cells.push(html`<span class="${cls}" title="${range}: ${!h ? `no ${word}s` : `${h.ok} of ${h.n} ${word}${h.n === 1 ? '' : 's'} ${word === 'ping' ? 'answered' : word === 'reading' ? 'within the limit' : 'up'}${why}`}"></span>`);
  }
  return compact ? html`<div class="upbar mini">${cells}</div>` : html`<div class="upbar">${cells}</div><div class="upaxis"><span>24 hours ago</span><span>now</span></div>`;
}
// Round-trip times of the last 24 hours in 288 five-minute slots: the
// average as a line, red where no ping was answered, gaps where none ran.
function rttGraph(rtt) {
  const slot = 300, n = 288, end = Math.floor(now() / slot) * slot, by = new Map((rtt || []).map((r) => [r.t, r]));
  const slots = [];
  for (let i = 0; i < n; i++) { const t = end - (n - 1 - i) * slot; slots.push({ i, t, r: by.get(t) }); }
  const top0 = Math.max(0, ...slots.map((s) => (s.r && s.r.n ? s.r.avg / 1000 : 0)));
  if (!top0 && !slots.some((s) => s.r)) return html`<div class="dim small">No round-trip times yet: the graph fills as pings are answered.</div>`;
  const nice = (v) => { const p = 10 ** Math.floor(Math.log10(v)); for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p; return 10 * p; };
  const step = nice(Math.max(top0, 0.1) / 5), top = Math.max(step, Math.ceil(top0 / step) * step);
  const H = 100, y = (us) => (H - (us / 1000 / top) * H).toFixed(2);
  let line = '', area = '', run = [];
  const flush = () => {
    if (!run.length) return;
    const pts = run.map((s) => `${s.i + 0.5},${y(s.r.avg)}`);
    line += `M${pts.join('L')}${run.length === 1 ? 'h0.01' : ''}`;
    area += `M${run[0].i + 0.5},${H}L${pts.join('L')}L${run[run.length - 1].i + 0.5},${H}Z`;
    run = [];
  };
  for (const s of slots) { if (s.r && s.r.n) run.push(s); else flush(); }
  flush();
  const hhmm = (t) => fmtDate(t).split(' ')[1];
  let bands = '', hits = '', grid = '';
  for (const s of slots) {
    const r = s.r, range = `${hhmm(s.t)}–${hhmm(s.t + slot)}`;
    if (r && !r.n && r.fail) bands += `<rect x="${s.i}" y="0" width="1" height="${H}" class="bad"/>`;
    const tip = !r ? 'no pings' : !r.n ? `no answer (${r.fail} ping${r.fail === 1 ? '' : 's'})`
      : `average ${rttText(r.avg)}, lowest ${rttText(r.min)}, highest ${rttText(r.max)} · ${r.n} answered${r.fail ? `, ${r.fail} unanswered` : ''}`;
    hits += `<rect x="${s.i}" y="0" width="1" height="${H}" class="hit"><title>${esc(`${range}: ${tip}`)}</title></rect>`;
  }
  const ticks = [];
  for (let v = 0; v <= top + 1e-9; v += step) ticks.push(v);
  for (const v of ticks) grid += `<line x1="0" x2="${n}" y1="${y(v * 1000)}" y2="${y(v * 1000)}"/>`;
  const xl = slots.filter((s) => { const d = new Date(s.t * 1000); return d.getMinutes() === 0 && d.getHours() % 2 === 0; });
  for (const s of xl) grid += `<line x1="${s.i}" x2="${s.i}" y1="0" y2="${H}"/>`;
  const fmtV = (v) => (step < 1 ? v.toFixed(step < 0.5 ? 2 : 1).replace(/\.?0+$/, '') || '0' : String(+v.toFixed(1)));
  return html`<div class="rttg">
    <div class="rttg-y">${ticks.map((v) => html`<span style="top:${100 - (v / top) * 100}%">${fmtV(v)}</span>`)}</div>
    <div class="rttg-plot">${raw(`<svg viewBox="0 0 ${n} ${H}" preserveAspectRatio="none"><g class="grid">${grid}</g><g class="bands">${bands}</g><path class="area" d="${area}"/><path class="line" d="${line}"/><g>${hits}</g></svg>`)}
      <div class="rttg-x">${xl.map((s) => html`<span style="left:${(s.i / n) * 100}%">${hhmm(s.t)}</span>`)}</div></div>
    <div class="rttg-unit">ms</div></div>`;
}
// Sub-probes of a service check (the master's probe.Steps), in order.
const STEP_LABEL = { connect: 'Connect', greeting: 'Greeting', tls: 'TLS handshake', cert: 'Certificate valid', cert_time: 'Valid in time', cert_days: 'Certificate days', http: 'HTTP status',
  query: 'Query', rcode: 'Server status', records: 'Records', expect: 'Expected value', time: 'Response time' };
const stepLabel = (k) => STEP_LABEL[k] || k;
const svcProto = (s) => (s.metric ? { disk: `DISK ${s.mount}`, memory: 'MEMORY', load: 'CPU' }[s.method] || '' : `${(s.method || 'tcp').toUpperCase()} ${s.port}`);
// Disk, memory and load: the limit as text ("> 90% used", "< 10 GB free", "> 1.5 per core").
const metricLimit = (s) => (s.method === 'disk' && s.limit_kind === 'free' ? `< ${s.limit} GB free` : s.method === 'load' ? `> ${s.limit} per core` : `> ${s.limit}% used`);
// The disk size or total memory of the last reading: the master's text ends
// with "… of 100 GB" for disk and memory.
const metricTotal = (s) => (s.method === 'load' ? '' : (/ of (\S+ [KMGT]?B)$/.exec(s.banner || '') || [])[1] || '');
const metricVal = (s, v) => (s.method === 'load' ? v.toFixed(2) : s.limit_kind === 'free' ? `${v.toFixed(1)} GB` : `${Math.round(v)}%`);
// What a service checks beyond its port: the HTTPS host and path, or the DNS question.
const svcTarget = (s) => (s.method === 'https' ? ` · ${s.host}${s.path !== '/' ? s.path : ''}` : s.method === 'dns' ? ` · ${s.rtype} ${s.host}`
  : s.method === 'cert' ? ` · ${s.host}${s.target === 'dns' ? ' (DNS)' : ''}` : '');
// A certificate check's web address as typed: name, and the port when not 443.
const certAddr = (s) => (s.port && s.port !== 443 ? `${s.host.includes(':') ? `[${s.host}]` : s.host}:${s.port}` : s.host || '');
// How the last check went per sub-probe: ✓ passed, ✗ failed, – not reached.
function svcSteps(s) {
  if (!s.steps || s.steps.length < 2 || !s.last_check || s.paused || s.reverse) return '';
  const at = s.last_error ? s.steps.indexOf(s.failed) : -1;
  if (s.last_error && at < 0) return '';
  return html`<div class="steps">${s.steps.map((k, i) => {
    const st = at < 0 || i < at ? 'ok' : i === at ? 'bad' : 'skip';
    const label = s.method === 'cert' && k === 'cert' ? 'Valid chain' : s.method === 'cert' && k === 'cert_days' ? 'Not expiring soon' : stepLabel(k);
    return html`<span class="step ${st}" title="${st === 'bad' ? s.last_error : st === 'skip' ? 'Not reached: an earlier step failed' : 'Passed'}">${st === 'ok' ? '✓' : st === 'bad' ? '✗' : '–'} ${label}</span>`;
  })}</div>`;
}
function svcState(s) {
  if (s.paused) return html`<span class="badge mute xs">paused</span>`;
  if (s.stale) return html`<span class="badge mute xs" title="The agent is offline: no readings come in">unknown</span>`;
  // Measured: readings in a row past the limit, instead of retries.
  const tries = s.metric ? (n) => `${n} of ${s.readings} past the limit` : (n) => `retry ${n} of ${s.retries}`;
  if (!s.checked) return s.fails ? html`<span class="badge upd xs" title="${s.last_error || ''}">${tries(s.fails)}</span>` : html`<span class="badge mute xs">waiting</span>`;
  // Disk, memory and CPU load are not "up" but within their limit: "ok".
  const up = s.metric ? 'ok' : 'up';
  if (s.up) return s.fails ? html`<span class="badge upd xs" title="${s.last_error || ''}">${up} · ${tries(s.fails)}</span>` : html`<span class="badge ok xs">${up}</span>`;
  return html`<span class="badge sec xs" title="${s.last_error || ''}">down</span>`;
}
function pingState(P) {
  if (P.mode === 'paused') return html`<span class="badge mute xs">paused</span>`;
  if (!P.checked) return P.fails ? html`<span class="badge upd xs" title="${P.last_error || ''}">no answer ${P.fails} of ${P.down_after}</span>` : html`<span class="badge mute xs">waiting</span>`;
  if (P.up) return P.fails ? html`<span class="badge upd xs" title="${P.last_error || ''}">up · no answer ${P.fails} of ${P.down_after}</span>` : html`<span class="badge ok xs">up</span>`;
  return html`<span class="badge sec xs" title="${P.last_error || ''}">down</span>`;
}
// The settings of one check method, built from the method table the
// master sends (svc_methods). vals: the service's current values, or null
// for the method's defaults; fixed: settings a default service decides.
function methodFields(m, vals, fixed = {}) {
  const v = (fd) => (fixed[fd.key] !== undefined ? fixed[fd.key] : vals && vals[fd.key] !== undefined && vals[fd.key] !== '' ? vals[fd.key] : fd.default ?? (fd.type === 'bool' ? false : ''));
  const hint = (fd) => (fd.hint ? html`<div class="dim small">${fd.hint}</div>` : '');
  return m.fields.map((fd) => {
    const val = v(fd), dis = fixed[fd.key] !== undefined ? 'disabled' : '';
    if (fd.type === 'bool') return html`<label class="checkrow"><input type="checkbox" name="f_${fd.key}" ${val ? 'checked' : ''} ${dis}> ${fd.label}</label>${hint(fd)}`;
    if (fd.type === 'limit') {
      const on = vals ? +val > 0 : true, n = +val > 0 ? +val : fd.default;
      return html`<label class="checkrow"><input type="checkbox" name="f_${fd.key}_on" ${on ? 'checked' : ''}> ${fd.label}
        <input type="number" name="f_${fd.key}" min="1" max="${fd.max}" value="${n}" style="width:${fd.max > 9999 ? 90 : 76}px;margin:0 6px"> ${fd.unit || ''}</label>${hint(fd)}`;
    }
    if (fd.type === 'select') {
      return html`<label class="field">${fd.label}<select name="f_${fd.key}">${fd.options.map((o) => html`<option value="${o}" ${o === val ? 'selected' : ''}>${o}</option>`)}</select></label>${hint(fd)}`;
    }
    if (fd.type === 'switch') {
      return html`<div class="field">${fd.label}<div class="segsw">${fd.choices.map(([k, l]) => html`<label><input type="radio" name="f_${fd.key}" value="${k}" ${k === val ? 'checked' : ''}><span>${l}</span></label>`)}</div></div>${hint(fd)}`;
    }
    return html`<label class="field">${fd.label}<input type="text" name="f_${fd.key}" value="${val}" placeholder="${fd.placeholder || ''}" maxlength="253" ${fd.key === 'host' ? 'required' : ''}></label>${hint(fd)}`;
  });
}
function methodValues(m, f) {
  const out = {};
  for (const fd of m.fields) {
    const el = f[`f_${fd.key}`];
    if (fd.type === 'bool') out[fd.key] = el.checked;
    else if (fd.type === 'limit') out[fd.key] = f[`f_${fd.key}_on`].checked ? +el.value : 0;
    else out[fd.key] = el.value.trim();
  }
  return out;
}
function serviceModal(dev, svc, presets0, lim, methods, done) {
  const isNew = !svc;
  // Disk, memory and load need an agent on the device.
  const presets = presets0.filter((p) => !p.agent || !isLocal(dev));
  const canPing = isNew && (chk(dev)?.ping_mode ?? 'removed') === 'removed';
  // Agentless devices start with the ping; on an agent device it is optional.
  const pingFirst = canPing && isLocal(dev);
  const p0 = presets[0];
  const s = svc || (pingFirst ?{ preset: 'ping', name: 'Ping', port: '', method: 'tcp', greeting: false, interval: lim.interval, retries: lim.retries, timeout: lim.timeout, reverse: false }
    : { preset: p0.key, name: p0.name, port: p0.port || '', method: p0.agent ? 'tcp' : p0.method || 'tcp', greeting: p0.greeting, interval: lim.interval, retries: lim.retries, timeout: lim.timeout, reverse: false });
  const groups = [...new Set(presets.map((p) => p.group))];
  const method = (k) => methods.find((m) => m.key === (k || 'tcp')) || methods[0];
  let lastPresetName = s.preset === 'ping' ? '' : (presets.find((p) => p.key === s.preset) || {}).name || '';
  // Draws the method's settings: the service's own values while it keeps
  // its method, otherwise the defaults (a default service may bring some).
  const drawFields = (f, keep) => {
    const m = method(f.method.value), p = presets.find((x) => x.key === f.preset.value);
    let vals = keep ? s : null;
    // A certificate check shows its web address with the port in it.
    if (vals && m.key === 'cert') vals = { ...vals, host: certAddr(vals) };
    // A default service brings its own certificate choice (Proxmox: self-signed).
    if (!vals && p && m.key === 'https') vals = { cert_valid: !!p.cert_valid, cert_days: 14 };
    const fixed = p && m.key === 'tcp' ? { greeting: p.greeting } : {};
    setHTML($('#svc-fields'), html`${methodFields(m, vals, fixed)}`);
  };
  // Disk, memory and load: only the name, the filesystem (disk), the limit and the readings in a row.
  const metricDef = { disk: [90, 3], memory: [90, 5], load: [1.5, 5] };
  const drawMetric = (f, p) => {
    const keep = svc && svc.preset === p.key, m = p.method;
    const kind = keep ? svc.limit_kind || 'used' : 'used';
    const disks = dev.disks || [];
    const mount = keep ? svc.mount : '/';
    const mounts = disks.some((d) => d.mount === mount) ? disks : [{ mount, fs: '', size: 0 }, ...disks];
    setHTML($('#svc-metric'), html`
      <label class="field">Name<input type="text" name="mname" value="${keep ? svc.name : p.name}" maxlength="48" required></label>
      ${m === 'disk' ? html`<label class="field">Filesystem<select name="mount">${mounts.map((d) => html`<option value="${d.mount}" ${d.mount === mount ? 'selected' : ''}>${d.mount}${d.fs ? ` — ${d.fs}, ${bytes(d.size)}` : ''}</option>`)}</select></label>
        ${disks.length ? '' : html`<div class="dim small">The agent has not reported its filesystems yet (agent 0.3.12 or later). / is watched until then.</div>`}` : ''}
      <div class="row" style="gap:12px;align-items:flex-end">
        ${m === 'disk' ? html`<label class="field" style="flex:1">Limit on<select name="kind" data-change="mkind"><option value="used" ${kind === 'used' ? 'selected' : ''}>Percent used</option><option value="free" ${kind === 'free' ? 'selected' : ''}>GB free</option></select></label>` : ''}
        <label class="field" style="flex:1"><span id="m-lim-lbl"></span><input type="number" name="limit" step="any" min="0" value="${keep ? svc.limit : metricDef[m][0]}" required></label>
        <label class="field" style="flex:1">Readings in a row<input type="number" name="readings" min="1" max="30" value="${keep ? svc.readings : metricDef[m][1]}" required></label></div>
      <div class="dim small">${m === 'load'
        ? 'The 1-minute load average divided by the number of CPU cores: above 1.5 for 5 minutes is the usual alarm.'
        : m === 'memory' ? 'Used memory is the total minus what the system can still give out without swapping; swap is not looked at. Above 90% for 5 minutes is the usual alarm.'
          : 'As df: the space reserved for root counts as used. Percent used suits most disks; GB free suits large ones.'}
        Measured by the agent on the device every minute and sent with its regular contact: the service is down after this many readings in a row past the limit, and up again at the first reading within it.</div>`);
    metricLabel(f);
  };
  const metricLabel = (f) => {
    const m = presets.find((x) => x.key === f.preset.value)?.method, free = f.kind?.value === 'free';
    $('#m-lim-lbl').textContent = m === 'load' ? 'Down above (load per core)' : free ? 'Down below (GB free)' : 'Down above (% used)';
  };
  let lastPreset = s.preset || ''; // the service the Check list was last built for
  const fill = (f) => {
    const ping = f.preset.value === 'ping';
    const p = presets.find((x) => x.key === f.preset.value), met = !!p?.agent;
    for (const n of ['name', 'port', 'method', 'interval', 'retries', 'timeout']) f[n].disabled = ping || met;
    $('#svc-tcp').style.display = ping || met ? 'none' : '';
    $('#svc-ping').style.display = ping ? '' : 'none';
    $('#svc-metric').style.display = met ? '' : 'none';
    const cert = p?.method === 'cert';
    $('#svc-title').textContent = isNew ? `Add a service to ${dev.name}${addr && !met && !ping && !cert ? ` on ${addr}` : ''}` : `Edit ${s.name}`;
    // A certificate check: the port is part of its web address; it runs once an hour by default.
    $('#svc-port').style.display = cert ? 'none' : '';
    if (isNew && cert && +f.interval.value === lim.interval) f.interval.value = 3600;
    if (isNew && !cert && +f.interval.value === 3600) f.interval.value = lim.interval;
    if (!met) setHTML($('#svc-metric'), '');
    if (ping) return setHTML($('#svc-fields'), '');
    if (met) {
      lastPreset = f.preset.value;
      setHTML($('#svc-fields'), '');
      return drawMetric(f, p);
    }
    f.port.disabled = !!p;
    // Only the check methods that suit the service (a custom port: all of
    // them); another service starts with its own method.
    const allowed = p ? methods.filter((m) => (p.methods || ['tcp']).includes(m.key)) : methods.filter((m) => m.key !== 'cert');
    let cur = f.method.value;
    if (f.preset.value !== lastPreset) cur = p ? (p.methods || ['tcp'])[0] : 'tcp';
    if (!allowed.some((m) => m.key === cur)) cur = allowed[0].key;
    lastPreset = f.preset.value;
    setHTML(f.method, html`${allowed.map((m) => html`<option value="${m.key}">${m.name}</option>`)}`);
    f.method.value = cur;
    f.method.disabled = allowed.length < 2;
    f.method.title = allowed.length < 2 ? `The only check that suits ${p.name}` : '';
    if (p) {
      f.port.value = p.port;
      if (!f.name.value || f.name.value === lastPresetName || f.name.value === 'Ping') f.name.value = p.name;
      lastPresetName = p.name;
    } else if (lastPresetName || f.name.value === 'Ping') {
      // From a default service to Custom: the admin fills in everything.
      if (f.name.value === lastPresetName || f.name.value === 'Ping') f.name.value = '';
      f.port.value = ''; lastPresetName = '';
    }
    drawFields(f, f.preset.value === (s.preset || '') && f.method.value === (s.method || 'tcp'));
  };
  // The address the service is checked on (an agent device without checks yet: its first one).
  const addr = chk(dev)?.address || (dev.check_addrs || [])[0] || '';
  openModal(html`<h3 id="svc-title">${isNew ? `Add a service to ${dev.name}` : `Edit ${s.name}`}</h3>
    <form class="form" data-submit="save">
      <label class="field">Service<select name="preset" data-change="preset">
        ${presets.filter((p) => p.agent).map((p) => html`<option value="${p.key}" ${s.preset === p.key ? 'selected' : ''}>${p.name}</option>`)}
        ${canPing ? html`<option value="ping" ${pingFirst ? 'selected' : ''}>Ping (ICMP)</option>` : ''}
        ${presets.filter((p) => p.method === 'cert').map((p) => html`<option value="${p.key}" ${s.preset === p.key ? 'selected' : ''}>${p.name}</option>`)}
        ${groups.map((g) => presets.filter((p) => p.group === g && !p.agent && p.method !== 'cert').map((p) => html`<option value="${p.key}" ${s.preset === p.key ? 'selected' : ''}>${p.name} (${p.port})</option>`))}
        <option value="" ${!s.preset ? 'selected' : ''}>Custom port…</option></select></label>
      <div class="notice blue" id="svc-ping" style="margin:0;display:none">The ping uses the same settings for every device: every ${lim.ping_interval} s, down after ${lim.ping_down_after} unanswered pings (Settings → General → Ping).${isLocal(dev) ? '' : ' It shows whether the address answers; the device\'s own status still comes from its agent.'}</div>
      <div id="svc-metric" class="stack" style="gap:12px;display:none"></div>
      <div id="svc-tcp" class="stack" style="gap:12px">
      <div class="row" style="gap:12px;align-items:flex-end">
        <label class="field" style="flex:2">Name<input type="text" name="name" value="${s.name}" maxlength="48" required></label>
        <label class="field" style="flex:1" id="svc-port">Port<input type="number" name="port" min="1" max="65535" value="${s.port}" required></label>
        <label class="field" style="flex:1">Check<select name="method" data-change="method">${methods.map((m) => html`<option value="${m.key}" ${(s.method || 'tcp') === m.key ? 'selected' : ''}>${m.name}</option>`)}</select></label></div>
      <div id="svc-fields" class="stack" style="gap:10px"></div>
      <div class="row" style="gap:12px">
        <label class="field" style="flex:1">Heartbeat interval (s)<input type="number" name="interval" min="${lim.interval_min}" max="${lim.interval_max}" value="${s.interval}" required></label>
        <label class="field" style="flex:1">Retries<input type="number" name="retries" min="0" max="${lim.retries_max}" value="${s.retries}" required></label>
        <label class="field" style="flex:1">Request timeout (s)<input type="number" name="timeout" min="${lim.timeout_min}" max="${lim.timeout_max}" value="${s.timeout}" required></label></div>
      <div class="dim small">Checked every ${lim.interval_min}–${lim.interval_max} s (default ${lim.interval}). After a failed check up to ${lim.retries_max} retries follow, 30 s apart, before the service is marked down. The timeout limits each step: connecting, waiting for the greeting, the TLS handshake, the HTTP answer and the DNS answer.</div>
      </div>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">${isNew ? html`${icon('plus')} Add service` : 'Save'}</button></div>
    </form>`, {
    wide: true,
    acts: {
      cancel: () => closeModal(),
      preset: (t) => fill(t.form),
      method: (t) => drawFields(t.form, t.form.preset.value === (s.preset || '') && t.form.method.value === (s.method || 'tcp')),
      mkind: (t) => { t.form.limit.value = t.value === 'free' ? 10 : 90; metricLabel(t.form); },
      save: async (f) => {
        if (f.preset.value === 'ping') {
          const res = await POST(`locals/${dev.id}/pingstate`, { state: 'on' });
          closeModal();
          toast('Ping added — the first ping follows within seconds', 'ok');
          return done?.(res);
        }
        const p = presets.find((x) => x.key === f.preset.value);
        if (p?.agent) {
          const body = { preset: p.key, name: f.mname.value, limit: +f.limit.value, readings: +f.readings.value };
          if (p.method === 'disk') Object.assign(body, { mount: f.mount.value, limit_kind: f.kind.value });
          const res = await POST(isNew ? `locals/${dev.id}/services` : `locals/${dev.id}/services/${svc.id}`, body);
          closeModal();
          toast(isNew ? `${body.name} added — the agent sends its first reading within a minute` : 'Saved', 'ok');
          return done?.(res);
        }
        const m = method(f.method.value);
        const body = { preset: f.preset.value, name: f.name.value, method: m.key, interval: +f.interval.value, retries: +f.retries.value, timeout: +f.timeout.value,
          reverse: false, ...methodValues(m, f) };
        if (!f.preset.value) body.port = +f.port.value;
        const res = await POST(isNew ? `locals/${dev.id}/services` : `locals/${dev.id}/services/${svc.id}`, body);
        closeModal();
        toast(isNew ? `${body.name} added — the first check follows within seconds` : 'Saved', 'ok');
        done?.(res);
      },
    },
  });
  fill($('.modal form'));
}
// Certificate Validity "Show": the certificate the last check saw (also when
// it failed), with its chain, every name it is valid for and the connection.
function certModal(s) {
  const c = s?.cert;
  if (!c) return;
  const days = (t) => Math.floor((t - now()) / 86400);
  const until = (t) => {
    const d = days(t);
    return html`${fmtDate(t)} <span class="${d < 0 ? 'c-red' : d < (s.cert_days || 10) ? 'c-amber' : 'dim'}">(${d < 0 ? `expired ${-d} day${d === -1 ? '' : 's'} ago` : `${d} day${d === 1 ? '' : 's'} left`})</span>`;
  };
  const row = (k, v) => html`<tr><th>${k}</th><td>${v}</td></tr>`;
  const entry = (e, i) => html`<div class="panel panel-pad certbox">
    <div class="row" style="margin-bottom:8px"><b>${i === 0 ? 'Server certificate' : e.ca ? `Certificate authority ${i}` : `Intermediate ${i}`}</b></div>
    <table class="certtbl">
      ${row('Subject', html`<span class="mono small">${e.subject}</span>`)}
      ${e.names?.length ? row(`Valid names (${e.names.length})`, html`<div class="certnames">${e.names.map((n) => html`<span class="mono small">${n}</span>`)}</div>`) : ''}
      ${row('Issued by', html`<span class="mono small">${e.issuer}</span>`)}
      ${row('Valid from', fmtDate(e.not_before))}
      ${row('Valid until', until(e.not_after))}
      ${row('Key', e.key)}
      ${row('Signature', e.sigalg)}
      ${row('Serial number', html`<span class="mono small">${e.serial}</span>`)}
      ${row('SHA-256 fingerprint', html`<span class="mono small certfp">${e.sha256}</span>`)}
    </table></div>`;
  openModal(html`<h3>${s.name}: ${certAddr(s)}</h3>
    <div class="dim small" style="margin:-6px 0 12px">As seen by the last check${s.last_check ? html`, ${when(s.last_check)}` : ''}.</div>
    ${s.last_error ? html`<div class="notice red" style="margin-bottom:12px">${s.last_error}</div>` : ''}
    <table class="certtbl" style="margin-bottom:12px">
      ${row('Connected to', html`<span class="mono small">${c.addr}</span>${s.target === 'dns' ? html` <span class="dim small">(where ${s.host} points)</span>` : html` <span class="dim small">(this device's address)</span>`}`)}
      ${row('Protocol', `${c.tls}${c.cipher ? ` · ${c.cipher}` : ''}`)}
    </table>
    <div class="stack" style="gap:12px">${(c.chain || []).map(entry)}</div>
    <div class="actions"><button type="button" class="btn primary" data-act="close">Close</button></div>`, {
    wide: true,
    acts: { close: () => closeModal() },
  });
}
// Where an agent device's services are checked: the address and who checks them.
async function checksModal(a, done) {
  let pa = { agents: [], max: 200 };
  try { pa = await GET('probe-agents'); } catch { /* only Master can be chosen */ }
  const addrs = a.check_addrs || [];
  // No checks yet: the defaults, stored on Save for the services to come.
  const L = a.checks || { address: addrs[0] || '', probe: null }, cur = L.probe?.id || '';
  const other = !addrs.includes(L.address);
  openModal(html`<h3>Checks of ${a.name}</h3>
    <form class="form" data-submit="save">
      <label class="field">Address<select name="addr" data-change="addr">
        ${addrs.map((ip) => html`<option value="${ip}" ${ip === L.address ? 'selected' : ''}>${ip}</option>`)}
        <option value="" ${other ? 'selected' : ''}>Other address…</option></select></label>
      <label class="field" id="ck-other" style="${other ? '' : 'display:none'}">Other address<input type="text" name="other" value="${other ? L.address : ''}" placeholder="e.g. 192.168.5.20" spellcheck="false" autocomplete="off"></label>
      <div class="dim small">One of the addresses the agent reports, or another one such as a virtual IP. IPv4 or IPv6, no host names: Coremon does not rely on DNS (an HTTPS check has its own hostname). A new address starts all checks over.</div>
      <div class="field">Checked by${pick('probe', probeItems(pa, cur, L.probe?.name, a.id), cur)}</div>
      <div class="dim small">Master: the master checks the services itself. An agent: that agent checks them, for example from another network. Never the device's own agent: a check from outside also notices when the device is unreachable. The history is kept when this changes.</div>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div>
    </form>`, {
    acts: {
      cancel: () => closeModal(),
      addr: (t) => { $('#ck-other').style.display = t.value ? 'none' : ''; },
      save: async (f) => {
        const address = f.addr.value || f.other.value.trim();
        const res = await POST(`agents/${a.id}/checks`, { address, probe: f.probe.value });
        closeModal();
        toast('Saved', 'ok');
        done?.(res);
      },
    },
  });
}
// A virtual device: its place in the network and the devices connected via
// it. Not checked, so no tabs, history or maintenance.
function VirtualPage([id]) {
  const st = { d: null };
  function render() {
    const d = st.d, v = (d.virtuals || []).find((x) => x.id === id);
    if (!v) { location.hash = '#/dashboard/' + lastDashSub; return; }
    if (v.special) { location.hash = '#/settings/upstream'; return; }
    const g = d.groups.find((x) => x.id === v.group_id);
    const below = [...d.agents, ...virtualDevs(d)].filter((a) => a.via?.id === id).sort(byName);
    const kind = (a) => (isVirtual(a) ? 'virtual' : isLocal(a) ? 'agentless' : 'agent');
    setHTML($('#v-page'), html`
      <div class="page-head syshead">
        <div class="headinfo"><div class="row"><a href="#/dashboard/${lastDashSub}" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(v)}<h1>${v.name}</h1>
          <span class="tag">${icon('folder')} ${g ? g.name : 'Ungrouped'}</span><span class="verpill loc" title="Not checked: it passes on the state of the devices above it">virtual</span></div>
          ${viaLine(v)}</div>
        <span class="spacer"></span>
        ${ifChange(html`<button class="btn sm" data-act="edit">${icon('edit')} Edit</button><button class="btn danger sm" data-act="delete" title="Delete">${icon('trash')}</button>`)}
      </div>
      ${v.unreach ? html`<div class="notice unr"><b>Unreachable</b> — <a href="#/device/${v.unreach.id}"><b>${v.unreach.name}</b></a>, which it is connected via, is down or in maintenance. The devices connected via this one are cut off too.</div>` : ''}
      <div class="panel panel-pad"><h2 style="margin:0 0 8px">Connected via this device</h2>
        ${below.length ? html`<table class="tbl"><tbody>${below.map((a) => html`<tr><td class="nowrap">${agentDot(a)} <a href="#/device/${a.id}"><b>${a.name}</b></a></td><td class="muted">${kind(a)}</td><td class="mono small">${devIP(a) || html`<span class="dim">—</span>`}</td></tr>`)}</tbody></table>`
          : html`<div class="dim small">No devices are connected via it.</div>`}</div>`);
  }
  const refresh = () => view?.refresh?.();
  return {
    interval: 5000,
    mount(el) { el.innerHTML = '<div id="v-page"></div>'; },
    async refresh() { const d = await GET('overview'); d.groups ||= []; st.d = d; render(); },
    acts: {
      edit: () => virtualEditModal(st.d, st.d.virtuals.find((x) => x.id === id), refresh),
      delete: async () => {
        const v = st.d.virtuals.find((x) => x.id === id);
        if (!(await confirmBox(`Delete ${v.name}?`, 'Devices connected via it are then connected via nothing. This cannot be undone.', { okText: 'Delete device', danger: true }))) return;
        await POST(`virtuals/${id}/delete`);
        toast(`${v.name} deleted`, 'ok');
        location.hash = '#/dashboard/' + lastDashSub;
      },
    },
  };
}

function LocalPage([id, tab]) {
  const st = { data: null, pinging: false, tab: tab === 'services' ? 'services' : 'system', checking: new Set() };
  const pingNote = (L) => (L.ping_mode === 'paused' ? 'Ping is paused for this device.' : 'This device is not pinged.');
  function systemTab(a, L) {
    const P = L.ping_info, on = L.ping, out = probeOut(a);
    const status = a.maint ? html`<span class="badge info xs">maintenance</span>` : out ? html`<span class="badge mute xs" title="${out.note}">unknown</span>` : !L.checked ? html`<span class="badge mute xs">waiting</span>` : L.up ? html`<span class="badge ok xs">up</span>` : a.unreach ? html`<span class="badge unr xs" title="${unrText(a)}">unreachable</span>` : html`<span class="badge sec xs">down</span>`;
    const svcs = L.services || [];
    const kv = [
      ['Status', html`${status} ${L.checked ? `since ${fmtDate(L.since)}` : ''}`],
      ['IP address', html`<span class="mono">${L.address}</span>`],
      ['Ping', on ? 'on' : html`<span class="badge mute xs">${L.ping_mode === 'paused' ? 'paused' : 'not pinged'}</span>`],
      ['Round-trip time', on && L.up && L.rtt_us ? rttText(L.rtt_us) : '—'],
      ['Last ping', P?.last_check ? html`${fmtDate(P.last_check, true)}` : '—'],
      ['Last answer', !P ? '—' : P.last_up ? html`${fmtDate(P.last_up, true)}` : 'never'],
      ['Unanswered in a row', !on ? '—' : L.fails ? html`<span class="c-red">${L.fails}</span>` : '0'],
      ['Last error', on && L.last_error ? html`<span class="c-red">${L.last_error}</span>` : '—'],
      ['Services', svcs.length ? html`<a href="#" data-act="tab" data-t="services">${svcs.length} service${svcs.length === 1 ? '' : 's'}</a>${L.svc_down ? html` · <span class="c-red">${L.svc_down} down</span>` : ''}${L.svc_paused ? html` · <span class="dim">${L.svc_paused} paused</span>` : ''}` : html`<span class="dim">none</span>`],
      ['Monitoring', 'Agentless'],
      ['Checked by', L.probe ? html`<a href="#/device/${L.probe.id}">${L.probe.name}</a> <span class="dim small">(remote probe)</span>` : 'Master'],
    ];
    return html`
      ${on ? '' : html`<div class="notice blue">${pingNote(L)} Its status comes from its running services: up while at least one is up.${canChange() ? html` ${L.ping_mode === 'paused' ? 'Resume' : 'Add'} the ping on the <a href="#" data-act="tab" data-t="services">Services</a> tab.` : ''}</div>`}
      <div class="panel panel-pad" style="margin-bottom:16px"><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v}</div></div>`)}</div></div>
      <div class="panel panel-pad"><div class="row" style="margin-bottom:12px"><h2 style="margin:0">Ping, last 24 hours</h2><span class="spacer"></span>
        ${!P ? html`<span class="dim small">not pinged</span>` : P.uptime >= 0 ? html`<span class="muted small">${pctText(P.uptime)} of the pings answered${on ? '' : ' (paused)'}</span>` : html`<span class="dim small">${on ? 'no pings yet' : 'paused'}</span>`}</div>
        ${uptimeBar(P ? P.hist : [])}
        <div class="row dim small" style="margin-top:10px;gap:14px"><span><i class="lg ok"></i>all answered</span><span><i class="lg part"></i>some answered</span><span><i class="lg bad"></i>none answered</span><span><i class="lg unr"></i>unreachable</span><span><i class="lg none"></i>no pings</span></div>
        ${P ? html`<div class="row" style="margin:22px 0 10px"><h3 style="margin:0">Round-trip time</h3><span class="spacer"></span><span class="dim small">average per 5 minutes · point at the graph for lowest and highest</span></div>${rttGraph(P.rtt)}` : ''}</div>
      <div class="panel panel-pad" style="margin-top:16px"><div class="row" style="margin-bottom:12px"><h2 style="margin:0">Services, last 24 hours</h2><span class="spacer"></span>
        <button class="btn sm" data-act="tab" data-t="services">${canChange() ? 'Configure' : 'Details'}</button></div>
        ${svcs.length ? html`<div class="svcsum">${svcs.map((s) => html`
          <div class="svcsum-row ${s.paused ? 'paused' : ''}"><div class="svcsum-name">${svcState(s)} <b>${s.name}</b> <span class="dim small mono">${svcProto(s)}</span>${s.reverse ? html` <span class="badge info xs" title="Reverse mode: reachable counts as down">reverse</span>` : ''}
            ${s.last_error && !s.paused && !(s.up && !s.fails) ? html`<span class="svcsum-why c-red" title="${s.last_error}">${s.failed ? `${stepLabel(s.failed)}: ` : ''}${s.last_error}</span>` : ''}</div>
            <div class="svcsum-bar">${uptimeBar(s.hist, { word: 'check', compact: true })}</div>
            <div class="svcsum-pct dim small">${s.uptime >= 0 ? `${pctText(s.uptime)} up` : 'no checks yet'}</div></div>`)}</div>`
          : html`<div class="dim small">No services</div>`}</div>`;
  }
  function servicesTab(a, L) {
    const head = html`<div class="row" style="margin-bottom:12px"><span class="muted small">The ping and the services are checked ${L.probe ? html`by the agent <b>${L.probe.name}</b>, on its own schedule` : 'from the master'}.</span>
      <span class="spacer"></span>${ifChange(html`<button class="btn primary sm" data-act="svc-add">${icon('plus')} Add service</button>`)}</div>`;
    return html`${head}${servicesTable(st, L)}`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const a = d.agent, L = a.local;
    const g = d.groups.find((x) => x.id === a.group_id);
    const out = probeOut(a);
    const state = a.maint ? 'in maintenance' : out ? unknownText(out) : !L.checked ? (L.fails ? `no answer yet (${L.fails} of ${L.down_after} checks before down)` : 'waiting for the first check') : L.up ? `up for ${dur(now() - L.since)}` : a.unreach ? `${unrText(a)}, no answer for ${dur(now() - L.since)}` : `down for ${dur(now() - L.since)}`;
    const total = (L.services || []).length + (L.ping_info ? 1 : 0);
    setHTML($('#a-head'), html`
      <div class="page-head syshead">
        <div class="headinfo"><div class="row"><a href="#/dashboard/${lastDashSub}" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(a)}<h1>${a.name}</h1>
          <span class="tag">${icon('folder')} ${g ? g.name : 'Ungrouped'}</span><span class="verpill loc" title="${L.probe ? `Checked by the agent ${L.probe.name}` : 'Monitored by the master itself'}; no agent is installed on it">agentless${L.probe ? ` · via ${L.probe.name}` : ''}</span></div>
          <div class="sub">${L.address} · ${state}${!a.maint && !out && L.up && L.svc_down ? html` · ${L.svc_down} of ${L.svc_total} services down${L.svc_fail ? html` · <span class="c-red">${L.svc_fail.name}: ${L.svc_fail.error || 'down'}</span>` : ''}` : ''}</div>
          ${viaLine(a)}</div>
        <span class="spacer"></span>
        <button class="btn sm" data-act="ping" ${st.pinging || !L.ping || L.probe ? 'disabled' : ''} title="${L.probe ? `Checked by ${L.probe.name} on its own schedule` : L.ping ? 'Ping the device now' : pingNote(L)}">${st.pinging ? html`<span class="spin"></span>` : icon('pulse')} Ping now</button>
        ${ifChange(html`<button class="btn sm" data-act="edit">${icon('edit')} Edit</button><button class="btn danger sm" data-act="delete" title="Delete">${icon('trash')}</button>`)}
        ${maintButton(a)}
      </div>
      ${maintNotice(a)}${unrNotice(a)}
      ${out && !a.maint ? html`<div class="notice grey"><b>Unknown – ${out.note}.</b> ${{
        offline: html`The agent <a href="#/device/${out.id}">${out.name}</a> that checks this device has no contact with the master. The master does not take over: the shown results are the last ones received.`,
        cleared: html`An administrator cleared the check list of <a href="#/device/${out.id}">${out.name}</a>. Checks resume after “Resend check list” on that agent's page, or when the checks change.`,
        waiting: html`<a href="#/device/${out.id}">${out.name}</a> has not confirmed its check list yet.`,
      }[out.state] || html`Choose another agent, or the master, with Edit.`}</div>` : ''}
      ${conflictNotice(a)}`);
    setHTML($('#a-tabs'), html`<button class="${st.tab === 'system' ? 'on' : ''}" data-act="tab" data-t="system">System</button><button class="${st.tab === 'services' ? 'on' : ''}" data-act="tab" data-t="services">Services${L.svc_down ? html` <span class="badge sec">${L.svc_down}</span>` : total ? html` <span class="dim small">${total}</span>` : ''}</button>`);
    setHTML($('#a-body'), st.tab === 'services' ? servicesTab(a, L) : systemTab(a, L));
  }
  const refresh = () => view?.refresh?.();
  const sacts = serviceActs(st, id, render);
  return {
    interval: 5000,
    mount(el) { el.innerHTML = '<div id="a-head"></div><div class="tabs" id="a-tabs"></div><div id="a-body"></div>'; },
    async refresh() {
      try { st.data = await GET('agents/' + id); } catch (e) { if (/not found/.test(e.message)) { location.hash = '#/'; return; } throw e; }
      render();
    },
    acts: {
      ...sacts,
      ping: sacts['svc-ping'],
      tab: (t) => { st.tab = t.dataset.t; render(); },
      'maint-start': () => maintStart(st.data.agent, refresh),
      'maint-end': () => maintEnd(st.data.agent, refresh),
      edit: () => localEditModal(st.data.agent, st.data.groups, refresh),
      delete: () => localDelete(st.data.agent, () => { location.hash = '#/dashboard/' + lastDashSub; }),
    },
  };
}

// ---- the Services tab: ping and services, of agentless and agent devices ----

function pauseBtn(paused, act, extra) {
  return html`<button class="btn ghost sm iconbtn" data-act="${act}" ${raw(extra)} title="${paused ? 'Resume: check it again' : 'Pause: stop checking it for now'}">${icon(paused ? 'play' : 'pause')}</button>`;
}
// st: the page state (pinging, checking); L: the device's checks (may be null).
function servicesTable(st, L) {
  const svcs = L?.services || [], P = L?.ping_info;
  if (!svcs.length && !P) return html`<div class="panel empty">No services</div>`;
  const pingRow = P ? html`<tr class="${P.mode === 'paused' ? 'dimrow' : ''}">
        <td class="nowrap">${pingState(P)}${P.checked && P.mode !== 'paused' ? html`<div class="dim small">${dur(now() - P.since)}</div>` : ''}</td>
        <td class="nowrap"><b>Ping</b><div class="dim small mono">ICMP</div></td>
        <td class="small"><span class="dim">—</span></td>
        <td class="small nowrap">${P.up && P.rtt_us && P.mode !== 'paused' ? rttText(P.rtt_us) : html`<span class="dim">—</span>`}</td>
        <td class="small nowrap">${P.last_check ? when(P.last_check) : html`<span class="dim">waiting</span>`}${P.last_error && P.mode !== 'paused' && !(P.up && !P.fails) ? html`<div class="c-red small" style="font-weight:400">${P.last_error}</div>` : ''}</td>
        <td style="min-width:150px">${uptimeBar(P.hist, { compact: true })}<div class="dim small">${P.uptime >= 0 ? `${pctText(P.uptime)} answered` : 'no pings yet'}</div></td>
        <td class="dim small nowrap" title="The same for every device: Settings → General → Ping">every ${P.interval} s<br>down after ${P.down_after} · ${P.timeout} s</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap;gap:6px">
          <button class="btn ghost sm iconbtn" data-act="svc-ping"title="${L.probe ? 'Checked by a remote probe on its own schedule' : 'Ping now'}" ${st.pinging || P.mode === 'paused' || L.probe ? 'disabled' : ''}>${st.pinging ? html`<span class="spin"></span>` : icon('refresh')}</button>
          ${ifChange(html`${pauseBtn(P.mode === 'paused', 'ping-pause', '')}<button class="btn danger sm iconbtn" data-act="ping-remove" title="Delete">${icon('trash')}</button>`)}</div></td>
      </tr>` : '';
    return html`<div class="panel scrollx"><table class="tbl svctbl"><thead><tr><th>Status</th><th>Service</th><th>Answer</th><th>Response</th><th>Checked</th><th>Last 24 hours</th><th>Check</th><th class="w1"></th></tr></thead>
      <tbody>${pingRow}${svcs.map((s) => (s.metric ? metricRow(st, s) : html`<tr class="${s.paused ? 'dimrow' : ''}">
        <td class="nowrap">${svcState(s)}${s.checked && !s.paused ? html`<div class="dim small">${dur(now() - s.since)}</div>` : ''}</td>
        <td class="nowrap"><b>${s.name}</b> ${s.reverse ? html`<span class="badge info xs" title="Reverse mode: reachable counts as down">reverse</span>` : ''}<div class="dim small mono">${svcProto(s)}${svcTarget(s)}</div>${svcSteps(s)}</td>
        <td class="small mono banner">${s.banner ? html`<span title="${s.banner}">${s.banner}</span>` : html`<span class="dim">${s.greeting || s.method === 'https' || s.method === 'dns' ? '—' : 'not waited for'}</span>`}</td>
        <td class="small nowrap">${s.rtt_us && s.up && !s.reverse && !s.paused ? rttText(s.rtt_us) : html`<span class="dim">—</span>`}</td>
        <td class="small nowrap">${s.last_check ? when(s.last_check) : html`<span class="dim">waiting</span>`}${s.last_error && !s.paused && !(s.up && !s.fails) ? html`<div class="c-red small" style="font-weight:400">${s.last_error}</div>` : ''}</td>
        <td style="min-width:150px">${uptimeBar(s.hist, { word: 'check', compact: true })}<div class="dim small">${s.uptime >= 0 ? `${pctText(s.uptime)} up` : 'no checks yet'}</div></td>
        <td class="dim small nowrap">every ${s.interval} s<br>${s.retries} retr${s.retries === 1 ? 'y' : 'ies'} · ${s.timeout} s${s.greeting ? html`<br>greeting` : ''}${s.method === 'https' ? html`<br>status ${s.codes}${s.cert_days ? ` · cert ≥ ${s.cert_days} d` : ''}` : ''}${s.method === 'dns' && s.max_ms ? html`<br>answer ≤ ${s.max_ms} ms` : ''}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap;gap:6px">
          ${s.method === 'cert' ? html`<button class="btn ghost sm" data-act="svc-cert" data-sid="${s.id}" title="${s.cert ? 'The certificate the last check saw' : 'No certificate seen yet'}" ${s.cert ? '' : 'disabled'}>Show</button>` : ''}
          <button class="btn ghost sm iconbtn" data-act="svc-check" data-sid="${s.id}" title="${L.probe ? 'Checked by a remote probe on its own schedule' : 'Check now'}" ${st.checking.has(s.id) || s.paused || L.probe ? 'disabled' : ''}>${st.checking.has(s.id) ? html`<span class="spin"></span>` : icon('refresh')}</button>
          ${ifChange(html`${pauseBtn(s.paused, 'svc-pause', `data-sid="${esc(s.id)}"`)}<button class="btn ghost sm iconbtn" data-act="svc-edit" data-sid="${s.id}" title="Edit">${icon('edit')}</button><button class="btn danger sm iconbtn" data-act="svc-delete" data-sid="${s.id}" title="Delete">${icon('trash')}</button>`)}</div></td>
      </tr>`))}</tbody></table></div>`;
}
// Disk, memory or load: measured by the device's own agent every minute (no Check now).
function metricRow(st, s) {
  return html`<tr class="${s.paused ? 'dimrow' : ''}">
    <td class="nowrap">${svcState(s)}${s.checked && !s.paused ? html`<div class="dim small">${dur(now() - s.since)}</div>` : ''}</td>
    <td class="nowrap"><b>${s.name}</b><div class="dim small mono">${svcProto(s)}</div></td>
    <td class="small banner">${s.banner && !s.stale ? html`<span title="${s.banner}">${s.banner}</span>` : html`<span class="dim">${s.stale ? 'no readings now' : '—'}</span>`}</td>
    <td class="small nowrap"><span class="dim">—</span></td>
    <td class="small nowrap">${s.last_check ? when(s.last_check) : html`<span class="dim">waiting for the agent</span>`}${s.last_error && !s.paused && !s.stale && !(s.up && !s.fails) ? html`<div class="c-red small" style="font-weight:400">${s.last_error}</div>` : ''}</td>
    <td style="min-width:150px">${uptimeBar(s.hist, { word: 'reading', compact: true })}<div class="dim small">${s.uptime >= 0 ? `${pctText(s.uptime)} within the limit` : 'no readings yet'}</div></td>
    <td class="dim small nowrap" title="Measured by the agent on the device">every 60 s<br>down: ${metricLimit(s)}<br>${s.readings} reading${s.readings === 1 ? '' : 's'} in a row</td>
    <td class="w1"><div class="row" style="flex-wrap:nowrap;gap:6px">
      <button class="btn ghost sm iconbtn" disabled title="Measured by the agent every minute, sent with its regular contact">${icon('refresh')}</button>
      ${ifChange(html`${pauseBtn(s.paused, 'svc-pause', `data-sid="${esc(s.id)}"`)}<button class="btn ghost sm iconbtn" data-act="svc-edit" data-sid="${s.id}" title="Edit">${icon('edit')}</button><button class="btn danger sm iconbtn" data-act="svc-delete" data-sid="${s.id}" title="Delete">${icon('trash')}</button>`)}</div></td>
  </tr>`;
}
// The 24-hour graph of a disk, memory or load service: five-minute
// averages (lowest and highest on hover), with the limit as a dashed line.
function valGraph(s) {
  const slot = 300, n = 288, end = Math.floor(now() / slot) * slot, by = new Map((s.vals || []).map((r) => [r.t, r]));
  const slots = [];
  for (let i = 0; i < n; i++) { const t = end - (n - 1 - i) * slot; slots.push({ i, t, r: by.get(t) }); }
  if (!slots.some((x) => x.r)) return html`<div class="dim small">No readings yet: the graph fills as the agent sends them (every minute).</div>`;
  const pct = s.method !== 'load' && s.limit_kind !== 'free';
  const nice = (v) => { const p = 10 ** Math.floor(Math.log10(v)); for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p; return 10 * p; };
  const top0 = Math.max(s.limit, ...slots.map((x) => (x.r ? x.r.max : 0)));
  const step = pct ? 20 : nice(Math.max(top0, 0.1) / 5), top = pct ? 100 : Math.max(step, Math.ceil((top0 * 1.05) / step) * step);
  const H = 100, y = (v) => (H - (Math.min(v, top) / top) * H).toFixed(2);
  let line = '', area = '', run = [];
  const flush = () => {
    if (!run.length) return;
    const pts = run.map((x) => `${x.i + 0.5},${y(x.r.avg)}`);
    line += `M${pts.join('L')}${run.length === 1 ? 'h0.01' : ''}`;
    area += `M${run[0].i + 0.5},${H}L${pts.join('L')}L${run[run.length - 1].i + 0.5},${H}Z`;
    run = [];
  };
  for (const x of slots) { if (x.r) run.push(x); else flush(); }
  flush();
  const hh = (t) => fmtDate(t).split(' ')[1];
  // Past the limit: above it, or for free space below it.
  const past = (v) => (s.limit_kind === 'free' ? v < s.limit : v > s.limit);
  let bands = '', hits = '', grid = '';
  for (const x of slots) {
    const r = x.r, range = `${hh(x.t)}–${hh(x.t + slot)}`;
    if (r && past(r.avg)) bands += `<rect x="${x.i}" y="0" width="1" height="${H}" class="bad"/>`;
    const tip = !r ? 'no readings' : `average ${metricVal(s, r.avg)}, lowest ${metricVal(s, r.min)}, highest ${metricVal(s, r.max)} · ${r.n} reading${r.n === 1 ? '' : 's'}`;
    hits += `<rect x="${x.i}" y="0" width="1" height="${H}" class="hit"><title>${esc(`${range}: ${tip}`)}</title></rect>`;
  }
  const ticks = [];
  for (let v = 0; v <= top + 1e-9; v += step) ticks.push(v);
  for (const v of ticks) grid += `<line x1="0" x2="${n}" y1="${y(v)}" y2="${y(v)}"/>`;
  const xl = slots.filter((x) => { const d = new Date(x.t * 1000); return d.getMinutes() === 0 && d.getHours() % 2 === 0; });
  for (const x of xl) grid += `<line x1="${x.i}" x2="${x.i}" y1="0" y2="${H}"/>`;
  const lim = `<line class="limit" x1="0" x2="${n}" y1="${y(s.limit)}" y2="${y(s.limit)}"/>`;
  const fmtV = (v) => (step < 1 ? v.toFixed(step < 0.5 ? 2 : 1).replace(/\.?0+$/, '') || '0' : String(+v.toFixed(1)));
  return html`<div class="rttg">
    <div class="rttg-y">${ticks.map((v) => html`<span style="top:${100 - (v / top) * 100}%">${fmtV(v)}</span>`)}</div>
    <div class="rttg-plot">${raw(`<svg viewBox="0 0 ${n} ${H}" preserveAspectRatio="none"><g class="grid">${grid}</g><g class="bands">${bands}</g><path class="area" d="${area}"/><path class="line" d="${line}"/>${lim}<g>${hits}</g></svg>`)}
      <div class="rttg-x">${xl.map((x) => html`<span style="left:${(x.i / n) * 100}%">${hh(x.t)}</span>`)}</div></div>
    <div class="rttg-unit">${s.method === 'load' ? 'per core' : pct ? '%' : 'GB'}</div></div>`;
}
// The graphs of an agent device's active disk, memory and load services (System tab).
function metricGraphs(a) {
  const list = (a.checks?.services || []).filter((s) => s.metric && !s.paused);
  if (!list.length) return '';
  return html`<div class="setgrid" style="margin-bottom:16px">${list.map((s) => html`<div class="panel panel-pad">
    <div class="row" style="margin-bottom:14px"><h2 style="margin:0">${s.name}</h2><span class="dim small mono">${svcProto(s)}</span><span class="spacer"></span>
      ${svcState(s)}${s.last_check && !s.stale ? html` <span class="small"><b>${metricVal(s, s.value)}</b>${s.method === 'load' ? ' per core' : s.limit_kind === 'free' ? ' free' : ' used'}${metricTotal(s) ? html` <span class="dim">of ${metricTotal(s)}</span>` : ''}</span>` : ''}</div>
    ${valGraph(s)}
    <div class="dim small" style="margin-top:4px">Last 24 hours, five-minute averages; the dashed line is the limit (down ${metricLimit(s)} for ${s.readings} reading${s.readings === 1 ? '' : 's'} in a row).</div></div>`)}</div>`;
}
// The actions of the Services tab, on both device pages. The answers are
// the device's own view; its checks are chk(device).
function serviceActs(st, id, render) {
  const L = () => chk(st.data.agent) || {};
  const svc = (sid) => (L().services || []).find((s) => s.id === sid);
  const setAgent = (a) => { st.data.agent = a; render(); };
  return {
    'svc-ping': async () => {
      st.pinging = true; render();
      try {
        const a = await POST(`locals/${id}/ping`);
        st.data.agent = a;
        const P = chk(a).ping_info;
        toast(!P.last_error ? `${a.name} answered in ${rttText(P.rtt_us)}` : `${a.name}: ${P.last_error}`, !P.last_error ? 'ok' : 'err');
      } finally { st.pinging = false; render(); }
    },
    'ping-pause': async () => {
      const paused = L().ping_mode === 'paused';
      setAgent(await POST(`locals/${id}/pingstate`, { state: paused ? 'on' : 'paused' }));
      toast(paused ? 'Ping resumed' : 'Ping paused', 'ok');
    },
    'ping-remove': async () => {
      if (!(await confirmBox('Delete the ping?', 'The device is no longer pinged and its ping history is removed. You can add the ping again with Add service.', { okText: 'Delete ping', danger: true }))) return;
      setAgent(await POST(`locals/${id}/pingstate`, { state: 'removed' }));
      toast('Ping deleted', 'ok');
    },
    'svc-add': () => serviceModal(st.data.agent, null, st.data.presets, st.data.svc_limits, st.data.svc_methods, setAgent),
    'svc-edit': (t) => serviceModal(st.data.agent, svc(t.dataset.sid), st.data.presets, st.data.svc_limits, st.data.svc_methods, setAgent),
    'svc-cert': (t) => certModal(svc(t.dataset.sid)),
    'svc-pause': async (t) => {
      const s = svc(t.dataset.sid);
      setAgent(await POST(`locals/${id}/services/${s.id}/pause`, { paused: !s.paused }));
      toast(`${s.name} ${s.paused ? 'resumed' : 'paused'}`, 'ok');
    },
    'svc-delete': async (t) => {
      const s = svc(t.dataset.sid);
      if (!(await confirmBox(`Delete ${s.name}${s.metric ? '' : ` (port ${s.port})`}?`, 'It is no longer checked, and its history is removed.', { okText: 'Delete service', danger: true }))) return;
      setAgent(await POST(`locals/${id}/services/${s.id}/delete`));
      toast(`${s.name} deleted`, 'ok');
    },
    'svc-check': async (t) => {
      const sid = t.dataset.sid;
      st.checking.add(sid); render();
      try {
        st.data.agent = await POST(`locals/${id}/services/${sid}/check`);
        const s = svc(sid);
        if (s) toast(s.up && !s.fails ? `${s.name} is up` : `${s.name}: ${s.last_error || 'not up'}`, s.up && !s.fails ? 'ok' : 'err');
      } finally { st.checking.delete(sid); render(); }
    },
  };
}

function versionLess(a, b) {
  const pa = String(a).match(/\d+|\D+/g) || [], pb = String(b).match(/\d+|\D+/g) || [];
  for (let i = 0; i < Math.min(pa.length, pb.length); i++) {
    if (pa[i] === pb[i]) continue;
    const na = /^\d+$/.test(pa[i]), nb = /^\d+$/.test(pb[i]);
    return na && nb ? +pa[i] < +pb[i] : pa[i] < pb[i];
  }
  return pa.length < pb.length;
}

function Devices() {
  const st = { data: null, tokens: [], lastCmd: null, type: 'agent', addShown: false };
  function render() {
    const d = st.data;
    if (!d) return;
    const pend = d.agents.filter((a) => a.status === 'pending').sort(byName);
    setPending(pend.length);
    setHTML($('#s-pending'), pend.length ? html`<div class="panel panel-pad"><h2>Waiting for approval <span class="badge upd">${pend.length}</span></h2>
      <table class="tbl"><tbody>${pend.map((a) => html`<tr><td>${agentDot(a)} <b>${a.name}</b></td><td class="muted">${osLabel(a.facts)} · ${a.facts.arch}</td><td class="muted">${(a.facts.ips || []).join(', ')}</td><td class="muted">${when(a.enrolled_at)}</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button><button class="btn danger sm" data-act="reject" data-id="${a.id}">Reject</button></div></td>`)}</tr>`)}</tbody></table></div>` : '');
    renderTokens();
    const count = (gid) => d.agents.filter((a) => (a.group_id || '') === gid).length;
    setHTML($('#s-groups'), html`
      ${d.groups.length ? html`<table class="tbl" style="margin-top:12px"><tbody>${d.groups.map((g, i) => html`<tr>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap;gap:2px"><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="-1" ${i === 0 ? 'disabled' : ''} title="Move up">${icon('up')}</button><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="1" ${i === d.groups.length - 1 ? 'disabled' : ''} title="Move down">${icon('down')}</button></div></td>`)}
        <td><b>${g.name}</b></td><td class="muted">${count(g.id)} device(s)</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn ghost sm" data-act="g-rename" data-gid="${g.id}">${icon('edit')} Rename</button><button class="btn danger sm" data-act="g-delete" data-gid="${g.id}">${icon('trash')}</button></div></td>`)}</tr>`)}
        <tr>${ifChange(html`<td></td>`)}<td class="dim">Ungrouped</td><td class="muted">${count('')} device(s)</td>${ifChange(html`<td class="dim small w1">always last</td>`)}</tr></tbody></table>`
        : html`<div class="muted small" style="margin-top:10px">No groups yet. All devices are shown together.</div>`}`);
    const act = d.agents.filter((a) => a.status !== 'pending').sort(byName);
    const seen = (a) => (a.maint ? html`<span class="c-blue">maintenance</span>` : a.unreach ? html`<span class="c-unr">unreachable</span>, behind ${a.unreach.name}` : isLocal(a) ? (probeOut(a) ? html`<span class="dim">${unknownText(probeOut(a))}</span>` : !a.local.checked ? html`<span class="dim">waiting</span>` : a.local.up ? 'up' : html`<span class="c-red">down</span>, last answer ${when(a.local.last_up)}`)
      : a.online ? 'online' : when(a.last_seen));
    setHTML($('#s-all'), act.length ? html`<div class="panel scrollx"><table class="tbl"><thead><tr><th>Device</th><th>Type</th><th>IP address</th><th>Group</th><th>OS</th><th>Agent</th><th>Last seen</th><th class="w1"></th></tr></thead>
      <tbody>${act.map((a) => html`<tr><td class="nowrap">${agentDot(a)} <a href="#/device/${a.id}"><b>${a.name}</b></a> ${a.status === 'report_only' ? html`<span class="badge info">report only</span>` : ''}${a.below_min ? html`<span class="badge upd xs">update required</span>` : ''}${a.ip_conflicts?.length ? html` <span class="badge sec xs" title="${a.ip_conflicts.map((c) => `IP ${c.ip} is also used by ${c.name}`).join('\n')}">IP also used</span>` : ''}</td>
        <td class="muted nowrap">${isLocal(a) ? html`Agentless${a.local.probe ? html`<div class="dim small">via ${a.local.probe.name}</div>` : ''}` : 'Agent'}${a.via ? html`<div class="dim small" title="Connected via">behind ${a.via.name}</div>` : ''}</td>
        <td class="mono small">${ipLabel(a.facts)}</td>
        <td>${canChange() ? html`<select class="sm" data-change="set-group" data-id="${a.id}" title="Group">${groupOptions(d.groups, a.group_id)}</select>` : html`<span class="muted">${d.groups.find((g) => g.id === a.group_id)?.name || 'Ungrouped'}</span>`}</td>
        <td class="muted">${isLocal(a) ? html`<span class="dim">—</span>` : osLabel(a.facts)}</td><td>${isLocal(a) ? html`<span class="dim">—</span>` : verPill(a)}</td>
        <td class="muted nowrap">${seen(a)}</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap;justify-content:flex-start">${isLocal(a)
    ? html`<button class="btn ghost sm" style="min-width:104px;justify-content:flex-start" data-act="l-edit" data-id="${a.id}">${icon('edit')} Edit</button><button class="btn danger sm" data-act="l-delete" data-id="${a.id}" title="Delete">${icon('trash')}</button>`
    : html`<button class="btn ghost sm" style="min-width:104px;justify-content:flex-start" data-act="rename" data-id="${a.id}">${icon('edit')} Edit</button><button class="btn danger sm" data-act="remove" data-id="${a.id}" title="Remove">${icon('trash')}</button>`}</div></td>`)}</tr>`)}</tbody></table></div>` : html`<div class="panel empty">No devices yet.</div>`);
    const virt = (d.virtuals || []);
    setHTML($('#s-virt'), virt.length ? html`<h2 style="font-size:15px;margin:6px 0 12px">Virtual devices</h2><div class="panel scrollx"><table class="tbl"><thead><tr><th>Device</th><th>Connected via</th><th>Group</th><th>State</th><th class="w1"></th></tr></thead>
      <tbody>${virt.map((v) => html`<tr><td class="nowrap">${v.special ? icon('globe') : html`<span class="dot virt"></span>`} <b>${v.name}</b>${v.special ? html` <span class="badge info xs">upstream</span>` : ''}</td>
        <td class="muted">${v.via?.name || html`<span class="dim">—</span>`}</td>
        <td class="muted">${v.special ? html`<span class="dim">—</span>` : d.groups.find((g) => g.id === v.group_id)?.name || 'Ungrouped'}</td>
        <td class="muted nowrap">${v.special ? upText(d.upstream) : v.unreach ? html`<span class="c-unr">unreachable</span>, behind ${v.unreach.name}` : html`<span class="dim">not checked</span>`}</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap;justify-content:flex-start">${v.special
    ? html`<a class="btn ghost sm" style="min-width:104px;justify-content:flex-start" href="#/settings/upstream">${icon('cog')} Settings</a>`
    : html`<button class="btn ghost sm" style="min-width:104px;justify-content:flex-start" data-act="v-edit" data-id="${v.id}">${icon('edit')} Edit</button><button class="btn danger sm" data-act="v-delete" data-id="${v.id}" title="Delete">${icon('trash')}</button>`}</div></td>`)}</tr>`)}</tbody></table></div>` : '');
    if (!st.addShown) { st.addShown = true; addPanel(); }
  }
  function addPanel() {
    const box = $('#s-add');
    if (!box || !canChange()) return;
    const groups = st.data?.groups || [];
    setHTML(box, html`
      <div class="tabs" style="margin:12px 0 4px"><button type="button" class="${st.type === 'agent' ? 'on' : ''}" data-act="add-type" data-t="agent">Agent</button><button type="button" class="${st.type === 'local' ? 'on' : ''}" data-act="add-type" data-t="local">Agentless</button><button type="button" class="${st.type === 'virtual' ? 'on' : ''}" data-act="add-type" data-t="virtual">Virtual</button></div>
      ${st.type === 'virtual' ? html`
        <div class="muted small">A place in the network that Coremon cannot check, such as an unmanaged switch: it has no address and no services. Other devices can be connected via it, and it is shown on the network map. It passes on the state of the devices above it.</div>
        <form class="row" style="margin-top:14px" data-submit="v-add">
          <input type="text" name="name" placeholder="Name, e.g. switch-attic" maxlength="64" style="width:220px" required>
          <select name="group" title="Group">${groupOptions(groups, '')}</select>
          <span title="Connected via: the device it sits behind on the network">${pick('via', viaItems(st.data, '').map((x) => ({ ...x, label: `Connected via: ${x.label}`, find: `${x.find || ''} ${x.label}` })), '', { style: 'width:250px' })}</span>
          <button class="btn primary">${icon('plus')} Add device</button>
        </form>` : st.type === 'agent' ? html`
        <div class="muted small">An agent on the device does all checks: updates, applications, CVEs and system details. Generates a single-use install command. Run it as <b>root</b> on the device (VM, container, bare metal — including this master container). The agent connects out to the master; no SSH or open ports needed.</div>
        <form class="row" style="margin-top:14px" data-submit="gen">
          <input type="text" name="note" placeholder="Note (optional), e.g. web-01" maxlength="64" style="width:240px">
          <select name="ttl"><option value="1" selected>valid 1 hour</option><option value="3">valid 3 hours</option></select>
          <button class="btn primary">${icon('plus')} Generate install command</button>
        </form>
        <div id="s-cmd"></div>
        <div id="s-tokens"></div>` : html`
        <div class="muted small">The master checks the device itself; nothing is installed on it.</div>
        <form class="row" style="margin-top:14px" data-submit="l-add">
          <input type="text" name="name" placeholder="Name, e.g. printer-hall" maxlength="64" style="width:220px" required>
          <input type="text" name="address" placeholder="IP address, e.g. 192.168.5.20" style="width:220px" required spellcheck="false" autocomplete="off">
          <select name="group" title="Group">${groupOptions(groups, '')}</select>
          <span title="Checked by: the master itself, or an agent for a network the master cannot reach">${pick('probe', probeItems(st.probes || { agents: [], max: 200 }, '').map((x) => ({ ...x, label: `Checked by: ${x.label}` })), '', { style: 'width:230px' })}</span>
          <span title="Connected via: the device it sits behind on the network">${pick('via', viaItems(st.data, '').map((x) => ({ ...x, label: `Connected via: ${x.label}`, find: `${x.find || ''} ${x.label}` })), '', { style: 'width:250px' })}</span>
          <button class="btn primary">${icon('plus')} Add device</button>
        </form>
        <div class="dim small" style="margin-top:8px">IPv4 or IPv6 only, no host names. An address can belong to one device only, whether agent or agentless; devices checked by different agents may share an address (separate networks). Only agents with Remote probe switched on (on the agent's device page) can check devices.
          Connected via: the device it sits behind on the network, such as a switch; when that device is down, this one shows as unreachable instead of down.</div>`}`);
    if (st.type === 'agent') renderTokens();
  }
  function renderTokens() {
    const box = $('#s-tokens');
    if (!box) return;
    setHTML(box, st.tokens.length ? html`<table class="tbl" style="margin-top:14px"><thead><tr><th>Open token</th><th>Note</th><th>Expires</th><th class="w1"></th></tr></thead>
      <tbody>${st.tokens.map((t) => html`<tr><td class="mono">${t.prefix}…</td><td class="muted">${t.note || '—'}</td><td class="muted">${fmtDate(t.expires)}</td><td class="w1"><button class="btn ghost sm" data-act="revoke" data-hash="${t.hash}">Revoke</button></td></tr>`)}</tbody></table>` : '');
  }
  const groupName = (gid) => st.data.groups.find((g) => g.id === gid)?.name || '';
  const agentBy = (id) => st.data.agents.find((a) => a.id === id);
  return {
    interval: 4000,
    mount(el) {
      el.innerHTML = r(html`${settingsTabs('devices')}
        <div class="page-head"><div><h1>Devices</h1><div class="sub">Add, approve, name, group and remove devices</div></div></div>
        <div class="stack">
          ${ifChange(html`<div class="panel panel-pad">
            <h2>Add a device</h2>
            <div class="muted small">Choose how the device is monitored. A device has one type of monitoring.</div>
            <div id="s-add"></div>
          </div>`)}
          <div id="s-pending"></div>
          <div class="panel panel-pad">
            <h2>Groups</h2>
            <div class="muted small">Each device belongs to at most one group. On the dashboard, groups stay together and can be folded${canChange() ? '; drag devices and groups there to arrange them' : ''}.</div>
            ${ifChange(html`<form class="row" style="margin-top:12px" data-submit="g-create">
              <input type="text" name="name" placeholder="New group name, e.g. Production" maxlength="48" style="width:260px" required>
              <button class="btn">${icon('plus')} Create group</button>
            </form>`)}
            <div id="s-groups"></div>
          </div>
          <div><h2 style="font-size:15px;margin:6px 0 12px">All devices</h2><div id="s-all"></div></div>
          <div id="s-virt"></div>
        </div>`);
    },
    async refresh() {
      [st.data, st.tokens, st.probes] = await Promise.all([GET('overview'), canChange() ? GET('tokens') : Promise.resolve([]),
        canChange() ? GET('probe-agents').catch(() => null) : Promise.resolve(null)]);
      render();
    },
    acts: {
      gen: async (f) => {
        const res = await POST('tokens', { note: f.note.value, ttl_hours: +f.ttl.value });
        st.lastCmd = res.command;
        setHTML($('#s-cmd'), html`
          ${res.has_core ? '' : html`<div class="notice red" style="margin-top:14px">No agent version is on the master yet — the install will fail until the master has downloaded one (see Releases).</div>`}
          <div class="cmdbox"><code id="s-cmdtext">${res.command}</code><button class="btn" data-act="copy">${icon('copy')} Copy</button></div>
          <div class="dim small" style="margin-top:8px">Valid until ${fmtDate(res.expires)} · single use · requires curl and systemd · installs the latest agent version.</div>`);
        f.note.value = '';
        view.refresh();
      },
      copy: async () => {
        try { await navigator.clipboard.writeText(st.lastCmd); toast('Copied', 'ok'); }
        catch { const rng = document.createRange(); rng.selectNode($('#s-cmdtext')); getSelection().removeAllRanges(); getSelection().addRange(rng); toast('Press Ctrl+C to copy'); }
      },
      revoke: async (t) => { await POST(`tokens/${t.dataset.hash}/delete`); view.refresh(); },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.groups),
      reject: async (t) => {
        if (await confirmBox('Reject this device?', 'It is removed from the master. Its agent will be refused from now on.', { okText: 'Reject', danger: true })) {
          await POST(`agents/${t.dataset.id}/remove`, { uninstall: false }); view.refresh();
        }
      },
      'add-type': (t) => { st.type = t.dataset.t; addPanel(); },
      'l-add': async (f) => {
        const res = await POST('locals', { name: f.name.value, address: f.address.value.trim(), group_id: f.group.value, probe: f.probe.value, via: f.via.value });
        toast(`${res.name} added — the first ping follows within seconds`, 'ok');
        f.name.value = ''; f.address.value = '';
        st.addShown = false; // the new device can now be chosen as "Connected via"
        view.refresh();
      },
      'v-add': async (f) => {
        const res = await POST('virtuals', { name: f.name.value, group_id: f.group.value, via: f.via.value });
        toast(`${res.name} added`, 'ok');
        st.addShown = false;
        view.refresh();
      },
      'v-edit': (t) => virtualEditModal(st.data, st.data.virtuals.find((v) => v.id === t.dataset.id), () => view.refresh()),
      'v-delete': async (t) => {
        const v = st.data.virtuals.find((x) => x.id === t.dataset.id);
        if (!(await confirmBox(`Delete ${v.name}?`, 'Devices connected via it are then connected via nothing. This cannot be undone.', { okText: 'Delete device', danger: true }))) return;
        await POST(`virtuals/${v.id}/delete`); toast(`${v.name} deleted`, 'ok'); st.addShown = false; view.refresh();
      },
      'l-edit': (t) => localEditModal(agentBy(t.dataset.id), st.data.groups, () => view.refresh()),
      'l-delete': (t) => localDelete(agentBy(t.dataset.id), () => view.refresh()),
      'set-group': async (t) => {
        await POST((isLocal(agentBy(t.dataset.id)) ? 'locals/' : 'agents/') + t.dataset.id, { group_id: t.value });
        toast(`${agentBy(t.dataset.id).name} → ${t.value ? groupName(t.value) : 'Ungrouped'}`, 'ok');
        view.refresh();
      },
      rename: (t) => agentEditModal(st.data, agentBy(t.dataset.id), () => view.refresh()),
      remove: (t) => {
        const ag = agentBy(t.dataset.id);
        openModal(html`<h3>Remove ${ag.name}?</h3>
          <div class="muted" style="margin-top:8px">Choose how to remove this device from Coremon.</div>
          <div class="stack" style="margin-top:16px;gap:10px">
            <button class="btn danger" data-act="uninstall" ${ag.online ? '' : 'disabled'}>${icon('trash')} Uninstall the agent from the device and remove it</button>
            <button class="btn" data-act="forget">Only forget it here (the agent stays installed and is refused from now on)</button>
          </div>
          ${ag.online ? '' : html`<div class="dim small" style="margin-top:8px">Uninstalling needs the device to be online.</div>`}
          <div class="actions"><button class="btn ghost" data-act="cancel">Cancel</button></div>`, {
          acts: {
            cancel: () => closeModal(),
            uninstall: async () => { const res = await POST(`agents/${ag.id}/remove`, { uninstall: true }); closeModal(); if (res.job) openJob(res.job); toast('Uninstall queued', 'ok'); },
            forget: async () => { await POST(`agents/${ag.id}/remove`, { uninstall: false }); closeModal(); toast(`${ag.name} removed`, 'ok'); view.refresh(); },
          },
        });
      },
      'g-create': async (f) => { await POST('groups', { name: f.name.value }); toast(`Group ${f.name.value} created`, 'ok'); f.reset(); view.refresh(); },
      'g-rename': (t) => {
        const gid = t.dataset.gid;
        openModal(html`<h3>Rename group</h3><form class="form" data-submit="save">
          <label class="field">Name<input type="text" name="name" value="${groupName(gid)}" maxlength="48" autofocus required></label>
          <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div></form>`, {
          acts: { cancel: () => closeModal(), save: async (f) => { await POST(`groups/${gid}`, { name: f.name.value }); closeModal(); view.refresh(); } },
        });
      },
      'g-delete': async (t) => {
        const gid = t.dataset.gid;
        const n = st.data.agents.filter((a) => a.group_id === gid).length;
        if (!(await confirmBox(`Delete group ${groupName(gid)}?`, n ? `Its ${n} device(s) move to Ungrouped. Nothing else changes.` : 'The group is empty.', { okText: 'Delete group', danger: true }))) return;
        await POST(`groups/${gid}/delete`); view.refresh();
      },
      'g-move': async (t) => {
        const d = st.data;
        const order = d.groups.map((g) => g.id);
        const i = order.indexOf(t.dataset.gid), j = i + +t.dataset.dir;
        if (j < 0 || j >= order.length) return;
        [order[i], order[j]] = [order[j], order[i]];
        await saveLayout(order, layoutOf(d));
        view.refresh();
      },
    },
  };
}

function vulnBadges(v, missing) {
  const why = { old_agent: 'Needs agent 0.3.6 or later', unsupported: 'No security data for this OS release', pending: 'Being looked up' };
  if (!v) return html`<span class="dim" title="${why[missing] || 'Not checked'}">—</span>`;
  switch (v.status) {
    case 'ok': return html`<span class="badge ok xs">ok</span>`;
    case 'pending': return html`<span class="dim">…</span>`;
    case 'not_distro': return html`<span class="dim" title="From ${v.origin}: not covered by the distribution's security data">not checked</span>`;
    case 'manual': return html`<span class="dim" title="Not installed from a package source">not checked</span>`;
    case 'old_agent': return html`<span class="dim" title="Needs agent 0.3.6 or later">—</span>`;
  }
  return html`${v.fixable ? html`<span class="badge ${v.red ? 'sec' : 'upd'} xs">${v.fixable} fixable</span> ` : ''}${v.open ? html`<span class="badge ${v.kev && !v.fixable ? 'sec' : 'mute'} xs">${v.open} without fix</span>` : ''}${!v.fixable && !v.open && v.kev ? html`<span class="badge sec xs">${v.kev} exploited</span>` : ''}`;
}
// The worst of several systems running the same version.
function worstVulns(systems) {
  const found = systems.map((s) => s.vulns).filter((v) => v && v.status === 'found');
  if (found.length) {
    return { status: 'found', fixable: Math.max(...found.map((v) => v.fixable)), open: Math.max(...found.map((v) => v.open)),
      kev: Math.max(...found.map((v) => v.kev)), red: found.some((v) => v.red) };
  }
  return systems.map((s) => s.vulns).find((v) => v && v.status === 'ok') || systems.map((s) => s.vulns).find(Boolean) || null;
}
const hasCVE = (v) => v && v.status === 'found';

function AppList() {
  const st = { d: null, q: '', f: 'all', g: '', view: [] };
  const verKey = (app, full) => app + '|' + full;
  function groupMenu() {
    const groups = st.d.groups || [];
    const ungrouped = st.d.apps.some((a) => a.versions.some((v) => v.systems.some((s) => !s.group_id)));
    if (st.g && !(st.g === '-' ? ungrouped : groups.some((g) => g.id === st.g))) st.g = '';
    setHTML($('#al-g'), html`<option value="">All devices</option>${groups.map((g) => html`<option value="${g.id}" ${st.g === g.id ? raw('selected') : ''}>${g.name}</option>`)}${ungrouped ? html`<option value="-" ${st.g === '-' ? raw('selected') : ''}>Ungrouped</option>` : ''}`);
    $('#al-g').value = st.g;
  }
  function render() {
    if (!st.d) return;
    const q = st.q.trim().toLowerCase();
    const inGroup = (s) => !st.g || (st.g === '-' ? !s.group_id : s.group_id === st.g);
    st.view = st.d.apps.map((a) => ({ ...a, versions: a.versions.map((v) => ({ ...v, systems: v.systems.filter(inGroup) })).filter((v) => v.systems.length) }))
      .filter((a) => a.versions.length);
    const keep = (v) => st.f === 'all' || (st.f === 'upd' ? v.systems.some((s) => s.update) : hasCVE(worstVulns(v.systems)));
    const apps = st.view.filter((a) => !q || a.name.toLowerCase().includes(q))
      .map((a) => ({ ...a, versions: a.versions.filter(keep) })).filter((a) => a.versions.length);
    const count = (f) => st.view.reduce((n, a) => n + a.versions.filter(f).length, 0);
    setHTML($('#al-chips'), html`${[['all', 'All', () => true], ['upd', 'With updates', (v) => v.systems.some((s) => s.update)], ['cve', 'With CVEs', (v) => hasCVE(worstVulns(v.systems))]]
      .map(([k, l, f]) => html`<button class="chip ${st.f === k ? 'on' : ''}" data-act="f" data-v="${k}">${l}<span class="n">${count(f)}</span></button>`)}`);
    if (!st.d.apps.length) return setHTML($('#al-list'), html`<div class="panel empty">No application data yet. It arrives with the devices' next update check.</div>`);
    if (!st.view.length) return setHTML($('#al-list'), html`<div class="panel empty">No applications found on the devices in this group.</div>`);
    if (!apps.length) return setHTML($('#al-list'), html`<div class="panel empty">Nothing matches.</div>`);
    setHTML($('#al-list'), html`<div class="stack">${apps.map((a) => html`<div class="panel"><div class="panel-pad" style="padding-bottom:6px"><h2 style="margin:0">${a.name}</h2></div>
      <table class="tbl apptbl"><colgroup><col><col class="c-sys"><col class="c-upd"><col class="c-sec"></colgroup>
      <thead><tr><th>Version</th><th>Devices</th><th>Updates</th><th>Security</th></tr></thead>
      <tbody>${a.versions.map((v) => {
        const n = v.systems.filter((s) => s.update).length;
        return html`<tr class="click" data-act="ver" data-k="${verKey(a.name, v.full)}" title="Show the devices running this version">
          <td><b class="mono">${v.version || v.full}</b> <span class="dim small mono">${v.full}</span></td>
          <td class="num"><b>${v.systems.length}</b></td>
          <td>${n ? html`<span class="badge upd xs">update on ${n} of ${v.systems.length}</span>` : html`<span class="dim small">no update</span>`}</td>
          <td class="nowrap">${vulnBadges(worstVulns(v.systems))}</td></tr>`;
      })}</tbody></table></div>`)}</div>`);
  }
  function openVersion(app, v) {
    openModal(html`<h3>${app} ${v.version}</h3><div class="muted small mono">${v.full} · ${v.systems.length} device${v.systems.length === 1 ? '' : 's'}</div>
      <div class="tbl-wrap" style="margin-top:12px;max-height:60vh"><table class="tbl"><thead><tr><th>Device</th><th>OS</th><th>Update</th><th>Security</th></tr></thead>
      <tbody>${v.systems.map((s) => html`<tr class="click" data-act="go" data-id="${s.id}" title="Open ${s.name}">
        <td class="nowrap"><span class="dot ${s.online ? 'on' : 'off'}"></span> <b>${s.name}</b>${s.online ? '' : html` <span class="small c-red">offline</span>`}${s.status === 'report_only' ? html` <span class="badge info xs">report only</span>` : ''}</td>
        <td class="small muted">${s.os}</td>
        <td class="mono small">${s.update ? html`<span class="badge upd xs">${s.update}</span>` : html`<span class="dim">—</span>`}</td>
        <td class="nowrap">${vulnBadges(s.vulns, s.vuln_status)}</td></tr>`)}</tbody></table></div>
      <div class="actions"><button class="btn" data-act="cancel">Close</button></div>`, {
      wide: true,
      acts: { cancel: () => closeModal(), go: (t) => { closeModal(); location.hash = `#/device/${t.dataset.id}/apps`; } },
    });
  }
  return {
    interval: 15000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('dashboard', 'apps')}<div class="page-head"><div><h1>Application List</h1><div class="sub">Which applications run where, per version</div></div></div>
        <div class="toolbar"><input type="search" class="search" id="al-q" placeholder="Search applications…"><select id="al-g" class="grpsel" title="Show the applications of one group"><option value="">All devices</option></select><div class="chips" id="al-chips"></div></div>
        <div id="al-list"></div>`);
      $('#al-q').addEventListener('input', (e) => { st.q = e.target.value; render(); });
      $('#al-g').addEventListener('change', (e) => { st.g = e.target.value; render(); });
    },
    async refresh() { st.d = await GET('apps'); groupMenu(); render(); },
    acts: {
      f: (t) => { st.f = t.dataset.v; render(); },
      ver: (t) => {
        const i = t.dataset.k.indexOf('|'), app = t.dataset.k.slice(0, i), full = t.dataset.k.slice(i + 1);
        const v = st.view.find((a) => a.name === app)?.versions.find((x) => x.full === full);
        if (v) openVersion(app, v);
      },
    },
  };
}

function endlessList({ load, keyOf, size = 50 }) {
  const p = { items: [], pending: [], more: true, busy: false, gap: false, data: null, obs: null };
  const atTop = () => window.scrollY < 150;
  p.reset = () => { p.items = []; p.pending = []; p.more = true; p.gap = false; };
  p.next = async () => {
    if (p.busy || !p.more) return false;
    p.busy = true;
    try {
      const d = await load(p.items[p.items.length - 1], p.items, size);
      const seen = new Set(p.items.map(keyOf));
      p.items.push(...d.items.filter((x) => !seen.has(keyOf(x))));
      p.more = d.more;
      p.data = d;
    } finally { p.busy = false; }
    return true;
  };
  p.refresh = async () => {
    if (!p.items.length) { p.more = true; await p.next(); return; }
    const d = await load(null, [], size);
    p.data = d;
    const known = new Map([...p.pending, ...p.items].map((x) => [keyOf(x), x]));
    const fresh = [];
    for (const x of d.items) {
      const k = keyOf(x);
      if (known.has(k)) Object.assign(known.get(k), x); else fresh.push(x);
    }
    const full = fresh.length >= size;
    if (atTop()) {
      if (full || p.gap) { p.reset(); p.items = d.items; p.more = d.more; } else p.items.unshift(...fresh, ...p.pending.splice(0));
      p.pending = [];
    } else if (fresh.length) {
      p.pending.unshift(...fresh);
      p.gap ||= full;
    }
  };
  p.toTop = async () => {
    if (p.gap) { p.reset(); await p.next(); } else { p.items.unshift(...p.pending); p.pending = []; }
    window.scrollTo({ top: 0 });
  };
  p.watch = (el, rerender) => {
    p.obs?.disconnect();
    p.check = async () => {
      if (p.more && !p.busy && el.getBoundingClientRect().top < innerHeight + 400 && (await p.next())) { rerender(); p.check(); }
    };
    p.obs = new IntersectionObserver((es) => { if (es.some((e) => e.isIntersecting)) p.check(); }, { rootMargin: '400px' });
    p.obs.observe(el);
  };
  p.stop = () => p.obs?.disconnect();
  p.banner = () => (p.pending.length ? html`<div class="newbar"><button class="btn sm primary" data-act="to-top">${icon('up')} ${p.gap ? size + '+' : p.pending.length} new — back to top</button></div>` : '');
  p.footer = (days) => (p.busy ? html`<div class="endline"><span class="spin"></span> Loading…</div>`
    : !p.more && p.items.length ? html`<div class="endline"><span>End of history: older entries are removed after ${days} day${days === 1 ? '' : 's'} (<a href="#/settings/general">Settings → General</a>).</span></div>` : '');
  return p;
}

// "in 42 s", "in 5 min", "in 2h 14m", "in 3d 4h"; "now" when due.
function inText(ts) {
  const s = Math.floor(ts - now());
  if (s <= 0) return 'now';
  if (s < 60) return `in ${s} s`;
  if (s < 3600) return `in ${Math.round(s / 60)} min`;
  return 'in ' + dur(s);
}
function Jobs() {
  const st = { active: false, agent: '', user: '', tab: 'jobs', up: null };
  const list = endlessList({
    keyOf: (j) => j.id,
    load: async (last, _, size) => {
      const q = new URLSearchParams({ limit: String(size) });
      if (st.active) q.set('active', '1');
      if (st.user) q.set('user', st.user);
      if (st.agent) q.set('agent', st.agent);
      if (last) {
        const oldest = last.batch ? last.batch[last.batch.length - 1] : last;
        q.set('before', oldest.id); q.set('before_t', String(oldest.created));
      }
      const d = await GET('jobs?' + q);
      return { ...d, items: d.jobs };
    },
  });
  const open = new Set();
  const isActive = (j) => j.status === 'queued' || j.status === 'running';
  function batchStatus(b) {
    const n = (s) => b.filter((j) => j.status === s).length;
    const busy = b.filter(isActive).length, bad = n('failed') + n('timeout') + n('cancelled');
    if (st.active) return html`<span class="badge info"><span class="spin" style="width:10px;height:10px"></span>${busy} of ${b.length} still running</span>`;
    return html`${busy ? html`<span class="badge info"><span class="spin" style="width:10px;height:10px"></span>${busy} running</span> ` : ''}${n('success') ? html`<span class="badge ok">${n('success')} succeeded</span> ` : ''}${bad ? html`<span class="badge sec">${bad} failed</span>` : ''}`;
  }
  function render() {
    const d = list.data;
    if (!d) return;
    const names = d.names || {};
    const sysCell = (j) => (names[j.agent_id] ? html`<a href="#/device/${j.agent_id}">${names[j.agent_id]}</a>` : html`<span class="dim">${j.agent_id}</span>`);
    const jobRow = (j, sub) => html`<tr class="click ${sub ? 'subrow' : ''}" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}${j.error ? html` <span style="color:var(--red)">${j.error}</span>` : ''}</div></td>
        <td>${sysCell(j)}</td>
        <td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td>
        <td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`;
    const batchRow = (j) => {
      const b = j.batch, shut = !open.has(j.id);
      const end = b.every((x) => x.finished) ? Math.max(...b.map((x) => x.finished)) : 0;
      const start = Math.min(...b.map((x) => x.created));
      return html`<tr class="click batchrow" data-act="batch" data-id="${j.id}"><td><span class="fold ${shut ? 'shut' : ''}">${icon('down')}</span> <b>${j.actor === 'scheduler' ? 'Scheduled ' : ''}${jobTitle(j)}</b></td>
        <td><b>${b.length} devices</b></td><td class="nowrap">${batchStatus(b)}</td><td class="muted">${j.actor}</td><td class="muted">${when(start)}</td>
        <td class="muted">${end ? dur(end - start) : '—'}</td></tr>
        ${shut ? '' : b.filter((x) => !st.active || isActive(x)).map((x) => jobRow(x, true))}`;
    };
    const actors = [...(d.users || []), 'scheduler'];
    setHTML($('#j-user'), html`<option value="">All users</option>${actors.map((n) => html`<option value="${n}" ${st.user === n ? 'selected' : ''}>${n === 'scheduler' ? 'scheduler (automatic)' : n}</option>`)}`);
    const sysOpts = Object.entries(names).sort((a, b) => a[1].localeCompare(b[1]));
    setHTML($('#j-sys'), html`<option value="">All devices</option>${sysOpts.map(([id, n]) => html`<option value="${id}" ${st.agent === id ? 'selected' : ''}>${n}</option>`)}`);
    setHTML($('#j-list'), list.items.length ? html`${list.banner()}<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>Device</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
      <tbody>${list.items.map((j) => (j.batch ? batchRow(j) : jobRow(j)))}</tbody></table></div>${list.footer(d.retention_days)}`
      : html`<div class="panel empty">${st.agent || st.user || st.active ? 'No jobs for this filter.' : 'No jobs.'}</div>`);
  }
  function renderUpcoming() {
    const u = st.up;
    if (!u) return;
    const at = (ts) => html`<span class="when" title="${fmtDate(ts, true)}">${inText(ts)}</span>`;
    setHTML($('#j-up'), html`
      <div class="panel" style="margin-bottom:16px"><div class="panel-pad" style="padding-bottom:0"><h2 style="margin:0">Master tasks</h2></div>
        <table class="tbl"><thead><tr><th>Task</th><th>Next run</th><th>Time</th></tr></thead><tbody>
        ${u.tasks.map((t) => html`<tr><td><b>${t.title}</b>${t.detail ? html`<div class="dim small">${t.detail}</div>` : ''}</td>
          <td class="nowrap">${at(t.at)}</td><td class="muted nowrap">${fmtDate(t.at)}</td></tr>`)}</tbody></table></div>
      <div class="panel"><div class="panel-pad" style="padding-bottom:0"><h2 style="margin:0">Agentless checks</h2><div class="dim small" style="margin-top:4px">The next ${u.checks.length || ''} ping and service checks</div></div>
        ${u.checks.length ? html`<table class="tbl"><thead><tr><th>Device</th><th>Check</th><th>Next run</th></tr></thead><tbody>
        ${u.checks.map((c) => html`<tr><td><a href="#/device/${c.device_id}">${c.device}</a></td><td><b>${c.check}</b>${c.detail ? html` <span class="dim small">${c.detail}</span>` : ''}</td>
          <td class="nowrap">${c.running ? html`<span class="row small"><span class="spin" style="width:10px;height:10px"></span>running</span>` : at(c.at)}</td></tr>`)}</tbody></table>`
          : html`<div class="panel-pad dim small">No agentless checks: add an agentless device in Settings → Devices.</div>`}</div>`);
  }
  function showTab() {
    setHTML($('#j-tabs'), html`<button class="${st.tab === 'jobs' ? 'on' : ''}" data-act="jtab" data-t="jobs">Jobs</button><button class="${st.tab === 'upcoming' ? 'on' : ''}" data-act="jtab" data-t="upcoming">Upcoming tasks</button>`);
    $('#j-jobs').style.display = st.tab === 'jobs' ? '' : 'none';
    $('#j-up').style.display = st.tab === 'upcoming' ? '' : 'none';
  }
  const restart = () => { list.reset(); window.scrollTo({ top: 0 }); view.refresh(); };
  return {
    interval: 3000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('activity', 'jobs')}<div class="page-head"><div><h1>Jobs</h1><div class="sub">Everything sent to agents, and what is planned</div></div></div>
        <div class="tabs" id="j-tabs"></div>
        <div id="j-jobs"><div class="row" style="margin-bottom:14px;gap:14px;flex-wrap:wrap">
        <div class="chips"><button class="chip on" data-act="f" data-v="0">All</button><button class="chip" data-act="f" data-v="1">Active</button></div>
        <label class="row small muted">User <select id="j-user"><option value="">All users</option></select></label>
        <label class="row small muted">Device <select id="j-sys"><option value="">All devices</option></select></label></div><div id="j-list"></div><div id="j-end"></div></div>
        <div id="j-up"></div>`);
      $('#j-user').addEventListener('change', (e) => { st.user = e.target.value; restart(); });
      $('#j-sys').addEventListener('change', (e) => { st.agent = e.target.value; restart(); });
      showTab();
      list.watch($('#j-end'), render);
    },
    unmount() { list.stop(); },
    async refresh() {
      if (st.tab === 'upcoming') {
        st.up = await GET('upcoming');
        renderUpcoming();
        return;
      }
      if (st.active) list.reset();
      await list.refresh();
      render();
      list.check();
    },
    acts: {
      jtab: (t) => { st.tab = t.dataset.t; showTab(); view.refresh(); },
      f: (t) => { st.active = t.dataset.v === '1'; document.querySelectorAll('#j-jobs .chips .chip').forEach((c) => c.classList.toggle('on', c === t)); restart(); },
      'to-top': async () => { await list.toTop(); render(); },
      batch: (t) => { const id = t.dataset.id; if (open.has(id)) open.delete(id); else open.add(id); render(); },
    },
  };
}

// Activity > Remote Probes: what the master is waiting for from agents on
// other systems; answered requests disappear, ones that gave up stay 10 min.
function RemoteProbes() {
  const st = { d: null };
  const secs = (s) => (s < 60 ? `${s} s` : s < 3600 ? `${Math.floor(s / 60)} min ${s % 60} s` : dur(s));
  const stateBadge = (p) => ({
    waiting: html`<span class="badge mute">waiting</span>`,
    running: html`<span class="badge info"><span class="spin" style="width:10px;height:10px"></span>running</span>`,
    overdue: html`<span class="badge sec" title="${p.note || ''}">not answered in time</span>`,
    offline: html`<span class="badge upd" title="${p.note || ''}">agent offline</span>`,
    maintenance: html`<span class="badge info" title="${p.note || ''}">${icon('pause')} maintenance</span>`,
    gave_up: html`<span class="badge mute" title="${p.note || ''}">gave up</span>`,
    answered: html`<span class="badge ok">${icon('check')} answered</span>`,
    failed: html`<span class="badge sec" title="${p.note || ''}">failed</span>`,
    received: html`<span class="badge ok">${icon('check')} received</span>${p.failed ? html` <span class="badge sec">${p.failed} failed</span>` : ''}`,
  }[p.state] || p.state);
  const done = (p) => p.state === 'answered' || p.state === 'failed' || p.state === 'received';
  const itemsTip = (p) => (p.items || []).join('\n') + (p.checks > (p.items || []).length ? `\n… and ${p.checks - p.items.length} more` : '');
  function render() {
    const d = st.d;
    if (!d) return;
    const open = d.requests.filter((p) => p.state !== 'gave_up' && !done(p)).length;
    setHTML($('#p-list'), d.requests.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Request</th><th>Agent</th><th>Waiting for</th><th>Status</th></tr></thead><tbody>
      ${d.requests.map((p) => p.kind === 'results' ? html`<tr class="dimrow" title="${itemsTip(p)}">
        <td><b>results received</b> · ${p.checks} check${p.checks === 1 ? '' : 's'}<div class="dim small">hover to see the checks and their outcome</div></td>
        <td><a href="#/device/${p.agent_id}">${p.agent}</a></td>
        <td class="nowrap muted" title="received ${fmtDate(p.since, true)}">received ${secs(Math.max(0, d.now - p.since))} ago</td>
        <td class="nowrap">${stateBadge(p)}</td></tr>` : html`${p.kind === 'check' ? html`<tr>
        <td><a href="#/device/${p.device_id}"><b>${p.title}</b></a><div class="dim small">check result · <span class="c-red">${p.note}</span></div></td>` : html`<tr class="click ${p.state === 'gave_up' || done(p) ? 'dimrow' : ''}" data-act="open-job" data-id="${p.id}">
        <td><b>${jobTitle(p)}</b><div class="dim small">${jobParams(p)}${p.state === 'overdue' || p.state === 'gave_up' || p.state === 'failed' || ((p.type || '').startsWith('probe.') && p.note) ? html` <span class="${p.state === 'overdue' || p.state === 'failed' ? 'c-red' : ''}">${p.note}</span>` : ''}</div></td>`}
        <td><a href="#/device/${p.agent_id}">${p.agent}</a></td>
        <td class="nowrap ${p.state === 'overdue' ? 'c-red' : 'muted'}" title="requested ${fmtDate(p.since, true)}">${p.state === 'gave_up' ? html`gave up ${ago(p.finished)}` : done(p) ? `answered in ${secs(Math.max(0, p.finished - p.since))}` : secs(Math.max(0, d.now - p.since))}</td>
        <td class="nowrap">${stateBadge(p)}</td></tr>`)}</tbody></table></div>
      <div class="dim small" style="margin-top:10px">${open} open · answered requests${d.results_shown ? ' and received results' : ''} stay 20 seconds; requests that gave up stay 10 minutes.</div>`
      : html`<div class="panel empty">Empty queue</div>`);
  }
  return {
    interval: 2000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('activity', 'probes')}<div class="page-head"><div><h1>Remote Probes</h1><div class="sub">Requests to agents on other systems that are not answered yet, and those just answered</div></div></div><div id="p-list"></div>`);
    },
    async refresh() { st.d = await GET('probes'); render(); },
  };
}

const outdatedIn = (list) => list.filter((a) => a.status === 'active' && !a.core_is_latest);
function Releases() {
  const st = { rel: null, data: null, checking: false };
  function render() {
    const R = st.rel, d = st.data;
    if (!R || !d) return;
    const ix = R.index, m = R.master, repo = R.repo;
    const published = new Map((ix.agents || []).map((v) => [v.version, v.published]));
    const agents = d.agents.filter((a) => a.status !== 'pending' && !isLocal(a));
    const minAgent = R.min_agent || '';
    setHTML($('#r-index'), html`<div class="panel panel-pad">
      <div class="row"><h2 style="margin:0">Version index</h2><span class="spacer"></span>
        ${ix.syncing || st.checking ? html`<span class="row muted small"><span class="spin"></span>checking…</span>` : html`<span class="muted small">${ix.checked_at ? when(ix.checked_at, 'checked ') : 'not checked yet'} · every 6 hours</span>`}
        ${ifChange(html`<button class="btn sm" data-act="check" ${ix.syncing || st.checking ? 'disabled' : ''}>${icon('refresh')} Check now</button>`)}</div>
      <div class="dim small mono" style="margin-top:6px;word-break:break-all">${ix.url || 'no GitHub repository configured — see Settings'}</div>
      ${ix.error ? html`<div class="notice red" style="margin:12px 0 0">${ix.error}</div>` : ''}
      ${repo.errors.length ? html`<div class="notice red" style="margin:12px 0 0"><b>Rejected files</b>${repo.errors.map((e) => html`<div class="mono small">${e}</div>`)}</div>` : ''}
      ${!repo.signing_key ? html`<div class="notice red" style="margin:12px 0 0">The master has no release signing key (/etc/coremon-master/signing.pub). Re-run the installer.</div>` : ''}
      <div class="dim small" style="margin-top:10px">The master downloads only this one signed file, verifies it, then downloads and verifies the listed binaries. Nothing is rolled out to devices until you choose to.</div>
    </div>`);
    setHTML($('#r-master'), html`<div class="panel panel-pad"><h2>Master (this website)</h2>
      <div class="row"><span class="muted">Installed</span><span class="ver">${m.version}</span>
        ${m.update ? html`<span class="badge upd">${m.latest} available</span>${ifChange(html`<button class="btn primary sm" data-act="master-update">${icon('up')} Update master to ${m.latest}</button>`)}` : html`<span class="badge ok">${icon('check')} latest</span>`}</div></div>`);
    const onVer = (v) => agents.filter((a) => a.core_version === v).length;
    setHTML($('#r-agents'), html`<div class="panel panel-pad"><h2>Agent versions <span class="muted small">— last ${repo.core.length || 0} kept on the master</span></h2>
      ${repo.core.length ? html`<table class="tbl"><thead><tr><th>Version</th><th>Published</th><th>Platforms</th><th>Devices</th></tr></thead>
        <tbody>${repo.core.map((v, i) => html`<tr><td><span class="verpill ${i === 0 ? 'ok' : 'old'}">${v.version}</span> ${i === 0 ? html`<span class="badge ok">latest</span>` : ''}</td>
          <td class="muted">${fmtDate(published.get(v.version))}</td><td>${v.platforms.map((p) => html`<span class="tag mono">${p}</span> `)}</td><td class="muted">${onVer(v.version)}</td></tr>`)}</tbody></table>`
        : html`<div class="muted">No agent versions downloaded yet.</div>`}</div>`);
    const groups = d.groups || [];
    const known = new Set(groups.map((g) => g.id));
    const secs = [...groups.map((g) => [g.id, g.name, agents.filter((a) => a.group_id === g.id)]), ['', 'Ungrouped', agents.filter((a) => !known.has(a.group_id))]]
      .filter(([, , list]) => list.length);
    const row = (a) => html`<tr><td>${agentDot(a)} <a href="#/device/${a.id}"><b>${a.name}</b></a></td><td>${verPill(a)}</td>
        ${ifChange(html`<td><form class="row" style="flex-wrap:nowrap" data-submit="set" data-id="${a.id}"><select name="v">${versionOptions({ core: repo.core.filter((v) => v.platforms.includes(`${a.facts.os}-${a.facts.arch}`) && !(minAgent && versionLess(v.version, minAgent))) }, a.core_version)}</select><button class="btn sm" ${a.status === 'active' ? '' : 'disabled'}>Apply</button></form></td>`)}</tr>`;
    const groupBtn = (gid, name, list) => {
      const n = outdatedIn(list).length;
      return n && canChange() && repo.core_latest
        ? html`<button class="btn sm" data-act="group-latest" data-gid="${gid}">${icon('up')} Update ${n} agent${n === 1 ? '' : 's'} in ${name} to ${repo.core_latest}</button>`
        : html`<span class="dim small">${repo.core_latest && list.every((a) => a.core_is_latest) ? 'all on the latest version' : ''}</span>`;
    };
    setHTML($('#r-rollout'), agents.length ? html`
      <div class="panel"><div class="panel-pad" style="padding-bottom:8px"><h2 style="margin:0">Agents per device</h2></div>
      <table class="tbl syslist"><thead><tr><th>Device</th><th>Agent</th>${ifChange(html`<th>Change to</th>`)}</tr></thead>
      ${secs.map(([gid, name, list]) => html`<tbody><tr class="grow"><td colspan="${canChange() ? 3 : 2}"><div class="row" style="flex-wrap:nowrap">${name} <span class="dim small">${list.length}</span><span class="spacer"></span>${groupBtn(gid, name, list)}</div></td></tr>
        ${[...list].sort(byName).map(row)}</tbody>`)}</table></div>` : '');
    renderQueue();
  }
  function renderQueue() {
    const Q = st.queue;
    if (!Q) return;
    const today = (ts) => new Date(ts * 1000).toDateString() === new Date().toDateString();
    const at = (ts) => (today(ts) ? fmtDate(ts).split(' ')[1] : fmtDate(ts));
    const status = (i) => ({
      now: html`<span class="row" style="gap:6px;flex-wrap:nowrap"><span class="spin"></span>looking up now</span>`,
      ratelimit: 'waiting, NVD rate-limit reached',
      noscore: `waiting, NVD had no score yet; next try ${fmtDate(i.at)}`,
      nvd_error: `waiting, NVD answered with an error; retrying at ${at(i.at)}`,
      next: `waiting, next lookup at ${at(i.at)}`,
      failed: `waiting, the last check failed; retrying at ${at(i.at)}`,
    })[i.status] || i.status;
    setHTML($('#r-vulns'), html`
      <div class="panel panel-pad"><h2>Detected OS releases</h2>
        ${Q.releases.length ? html`<ul class="plainlist">${Q.releases.map((n) => html`<li>${n}</li>`)}</ul>` : html`<div class="muted">No devices yet.</div>`}</div>
      <div class="panel" style="margin-top:16px"><div class="panel-pad" style="padding-bottom:8px"><h2 style="margin:0">Vulnerability lookups</h2>
        <div class="dim small" style="margin-top:4px">CVE lookups at OSV.dev and severity lookups at NVD that run now or are waiting.</div></div>
        ${Q.items.length ? html`<div class="tbl-wrap" data-keep-scroll="vq"><table class="tbl"><thead><tr><th>Lookup</th><th>Where</th><th>Status</th></tr></thead>
          <tbody>${Q.items.map((i) => html`<tr><td class="mono small">${i.what}</td><td><span class="tag">${i.where}</span></td><td class="small ${i.status === 'now' ? '' : 'muted'}">${status(i)}</td></tr>`)}</tbody></table></div>
          ${Q.more ? html`<div class="dim small panel-pad" style="padding-top:8px">…and ${Q.more} more.</div>` : ''}`
        : html`<div class="muted panel-pad" style="padding-top:0">Nothing outstanding: all lookups are done.</div>`}</div>`);
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`${settingsTabs('releases')}<div class="page-head"><div><h1>Releases</h1><div class="sub">Versions and releases</div></div></div>
        <div class="stack"><div id="r-index"></div><div class="grid2"><div id="r-master"></div><div id="r-agents"></div></div><div id="r-rollout"></div><div id="r-vulns"></div></div>`);
    },
    async refresh() { [st.rel, st.data, st.queue] = await Promise.all([GET('releases'), GET('overview'), GET('vulns/queue')]); render(); },
    acts: {
      check: async () => {
        st.checking = true; render();
        try { st.rel = await POST('releases/check'); toast(st.rel.index.error ? 'Check finished with errors' : 'Version index checked', st.rel.index.error ? 'err' : 'ok'); }
        finally { st.checking = false; render(); }
      },
      'master-update': async () => {
        const m = st.rel.master;
        if (!(await confirmBox(`Update the master to ${m.latest}?`, 'The signed binary is verified by a root service, installed, and the master restarts. The page reconnects after a few seconds.', { okText: 'Update master' }))) return;
        await POST('master/update');
        toast('Master update requested — restarting shortly', 'ok');
      },
      set: async (f) => {
        const a = st.data.agents.find((x) => x.id === f.dataset.id);
        const v = f.v.value, cur = a.core_version;
        if (v === cur) return toast(`${a.name} already runs agent ${v}`);
        const down = versionLess(v, cur);
        if (!(await confirmBox(`${down ? 'Downgrade' : 'Upgrade'} the agent on ${a.name}?`, `${cur} → ${v}. The agent downloads the signed binary, verifies it and restarts.`, { okText: down ? 'Downgrade' : 'Upgrade' }))) return;
        await setAgentVersion(a, v);
      },
      'group-latest': async (t) => {
        const l = st.rel.repo.core_latest, gid = t.dataset.gid;
        const groups = st.data.groups || [];
        const known = new Set(groups.map((g) => g.id));
        const name = groups.find((g) => g.id === gid)?.name || 'Ungrouped';
        // Same devices as the table: no agentless or pending ones.
        const list = outdatedIn(st.data.agents.filter((a) => a.status !== 'pending' && !isLocal(a) &&
          (gid ? a.group_id === gid : !known.has(a.group_id)))).sort(byName);
        if (!list.length) return;
        if (!(await confirmBox(`Update ${list.length} agent${list.length === 1 ? '' : 's'} in ${name} to ${l}?`,
          html`${list.map((a) => html`<div>${a.name} <span class="dim">(${a.core_version}${a.online ? '' : ', offline: updates when it is online'})</span></div>`)}`,
          { okText: 'Update agents' }))) return;
        // One failing device does not stop the others; the failures are named at the end.
        const ok = [], bad = [];
        for (const a of list) {
          try { await POST(`agents/${a.id}/core`, { version: l }); ok.push(a.name); }
          catch (e) { bad.push(`${a.name}: ${e.message || e}`); }
        }
        const n = (k) => `${k} device${k === 1 ? '' : 's'}`;
        if (!bad.length) toast(`Update to ${l} requested for ${n(ok.length)}`, 'ok');
        else toast(`Update to ${l} requested for ${ok.length} of ${n(list.length)}${ok.length ? ` (${ok.join(', ')})` : ''}.\nFailed:\n${bad.map((b) => '• ' + b).join('\n')}`, 'err', true);
        view.refresh();
      },
    },
  };
}

function Audit() {
  const st = { agent: '', user: '' };
  const keyOf = (e) => [e.time, e.actor, e.action, e.agent, e.target, e.detail, e.job, e.ip].join('|');
  const list = endlessList({
    keyOf,
    load: async (last, items, size) => {
      const q = new URLSearchParams({ limit: String(size) });
      if (st.agent) q.set('agent', st.agent);
      if (st.user) q.set('user', st.user);
      if (last) { q.set('before', String(last.time)); q.set('skip', String(items.filter((e) => e.time === last.time).length)); }
      const d = await GET('audit?' + q);
      return { ...d, items: d.entries };
    },
  });
  function render() {
    const d = list.data;
    if (!d) return;
    const names = d.names || {};
    const opts = Object.entries(names).sort((a, b) => a[1].localeCompare(b[1]));
    setHTML($('#au-sys'), html`<option value="">All devices</option>${opts.map(([id, n]) => html`<option value="${id}" ${st.agent === id ? 'selected' : ''}>${n}</option>`)}`);
    const actors = [...(d.users || []), 'agent', 'scheduler'];
    setHTML($('#au-user'), html`<option value="">All users</option>${actors.map((n) => html`<option value="${n}" ${st.user === n ? 'selected' : ''}>${n === 'agent' || n === 'scheduler' ? n + ' (automatic)' : n}</option>`)}`);
    const sys = (e) => e.agent ? (names[e.agent] || e.target || e.agent) : (names[e.target] || '');
    setHTML($('#au'), list.items.length ? html`${list.banner()}<div class="panel"><table class="tbl"><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Device</th><th>Target</th><th>Detail</th><th>IP</th></tr></thead>
      <tbody>${list.items.map((e) => html`<tr><td class="muted mono small" style="white-space:nowrap">${fmtDate(e.time, true)}</td><td>${e.actor}</td><td><span class="tag mono">${e.action}</span></td>
        <td>${sys(e)}</td><td class="mono small">${e.agent ? '' : (names[e.target] ? '' : e.target || '')}</td><td class="muted small">${e.detail || ''}${e.job ? html` <button class="btn ghost sm" data-act="open-job" data-id="${e.job}" title="Show the output of the job this action started">${icon('jobs')} Show output</button>` : ''}</td><td class="dim small">${e.ip || ''}</td></tr>`)}</tbody></table></div>${list.footer(d.retention_days)}`
      : html`<div class="panel empty">${st.agent || st.user ? 'No entries for this filter.' : 'No entries yet.'}</div>`);
  }
  const restart = () => { list.reset(); window.scrollTo({ top: 0 }); view.refresh(); };
  return {
    interval: 10000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('activity', 'audit')}<div class="page-head"><div><h1>Audit log</h1><div class="sub">Who did what, and when</div></div><span class="spacer"></span>
        <label class="row small muted">User <select id="au-user"><option value="">All users</option></select></label>
        <label class="row small muted">Device <select id="au-sys"><option value="">All devices</option></select></label></div><div id="au"></div><div id="au-end"></div>`);
      $('#au-sys').addEventListener('change', (e) => { st.agent = e.target.value; restart(); });
      $('#au-user').addEventListener('change', (e) => { st.user = e.target.value; restart(); });
      list.watch($('#au-end'), render);
    },
    unmount() { list.stop(); },
    async refresh() { await list.refresh(); render(); list.check(); },
    acts: { 'to-top': async () => { await list.toTop(); render(); } },
  };
}

function settingsHead(sub, title, desc) {
  return html`${settingsTabs(sub)}<div class="page-head"><div><h1>${title}</h1><div class="sub">${desc}</div></div></div>
    ${session.initial_password && sub !== 'account' ? html`<div class="notice">You are still using the generated initial password — <a href="#/settings/account">change it under My Account</a>.</div>` : ''}`;
}

// Settings → Upstream: the internet connection as a virtual device, pinged
// by the master every minute.
function SettingsUpstream() {
  const st = { d: null, built: false };
  const DNS = { '9.9.9.9': 'Quad9, Switzerland', '86.54.11.100': 'DNS4EU unfiltered, EU', '1.1.1.1': 'Cloudflare' };
  function results() {
    const u = st.d?.upstream;
    const box = $('#up-live');
    if (!box || !u) return;
    if (!u.enabled) { setHTML(box, html`<div class="dim small">Switched off: nothing is pinged.</div>`); return; }
    setHTML(box, html`<div class="row" style="gap:10px;align-items:baseline"><b>${u.name}</b> ${upText(u)}${u.since && u.state !== 'wait' ? html`<span class="dim small">since ${fmtDate(u.since)}</span>` : ''}</div>
      <table class="tbl" style="margin-top:8px;max-width:680px"><tbody>${u.addrs.map((a) => html`<tr><td class="w1"><span class="dot ${!a.at ? 'unk' : a.ok ? 'on' : 'off'}"></span></td>
        <td class="mono">${a.addr}</td><td class="muted small">${DNS[a.addr] || ''}</td>
        <td class="nowrap">${!a.at ? html`<span class="dim">waiting</span>` : a.ok ? html`<span class="c-green">${fmtMs(a.rtt_us)}</span>` : html`<span class="c-red">no answer</span> <span class="dim small">${a.error || ''}</span>`}</td>
        <td class="dim small nowrap">${a.at ? when(a.at) : ''}</td></tr>`)}</tbody></table>`);
  }
  function build() {
    const d = st.d, u = d.upstream, addrs = d.addrs || [];
    setHTML($('#up-form'), html`<form class="form" data-submit="save"><fieldset class="plain" ${canChange() ? '' : 'disabled'}>
      <label class="checkrow"><input type="checkbox" name="enabled" ${u.enabled ? 'checked' : ''}> Monitor the internet connection as a virtual device</label>
      <div class="dim small">Adds a special virtual device that other devices can be connected via, usually your internet router. It is the top of the network map, drawn as a cloud.
        It is only created and removed here; switching it off also clears every "Connected via" to it. It is never the cause of "unreachable" for the devices below it.</div>
      <label class="field">Name<input type="text" name="name" value="${u.name}" maxlength="64" placeholder="${d.default_name}" style="max-width:320px"></label>
      <div class="dim small">Used for the cloud on the map, the Overview tile and the "Connected via" choice.</div>
      <div class="field">Addresses to ping<div class="stack" style="gap:6px;max-width:320px">${[0, 1, 2].map((i) => html`<input type="text" name="addr${i}" value="${addrs[i] || ''}" placeholder="${d.defaults[i]}" spellcheck="false" autocomplete="off">`)}</div></div>
      <div class="dim small">IP addresses only, pinged by the master.</div>
      <label class="field">Ping every<div class="row" style="gap:8px"><input type="number" name="interval" value="${u.interval_sec}" min="${d.interval.min}" max="${d.interval.max}" step="1" required style="width:110px"><span class="muted">seconds</span></div></label>
      <div class="dim small">${d.interval.min} to ${d.interval.max} seconds, default ${d.interval.default}.</div>
      <label class="checkrow"><input type="checkbox" name="tile" ${u.show_tile ? 'checked' : ''}> Show the ${u.name} tile on the Overview</label>
      ${ifChange(html`<div class="row"><button class="btn primary">Save</button><button type="button" class="btn ghost" data-act="defaults">Default addresses</button></div>`)}
    </fieldset></form>`);
    st.built = true;
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`${settingsTabs('upstream')}
        <div class="page-head"><div><h1>Upstream</h1><div class="sub">The internet connection, as a virtual device on the network map</div></div></div>
        <div class="stack">
          <div class="panel panel-pad"><h2 style="margin:0 0 8px">Settings</h2><div id="up-form"></div></div>
          <div class="panel panel-pad"><h2 style="margin:0 0 8px">Live result</h2><div id="up-live"></div></div>
        </div>`);
    },
    async refresh() {
      st.d = await GET('upstream');
      if (!st.built) build();
      results();
    },
    acts: {
      defaults: () => { const f = $('#up-form form'); st.d.defaults.forEach((a, i) => { f[`addr${i}`].value = a; }); },
      save: async (f) => {
        const body = { enabled: f.enabled.checked, name: f.name.value, addrs: [0, 1, 2].map((i) => f[`addr${i}`].value.trim()), show_tile: f.tile.checked, interval_sec: +f.interval.value };
        const res = await POST('upstream', body);
        toast(res.upstream.enabled ? `${res.upstream.name} saved — the first ping follows within seconds` : 'Saved — switched off', 'ok');
        st.built = false;
        setTimeout(() => view.refresh(), 2500);
        view.refresh();
      },
    },
  };
}

function SettingsGeneral() {
  const st = { saved: null, next: 0, timer: null };
  // One block: heading, description and a form with its own Save.
  const block = (name, title, sub, body) => html`<div class="panel panel-pad">
    <h2 style="margin:0">${title}</h2><div class="muted small" style="margin:4px 0 4px">${sub}</div>
    <form class="form" data-submit="save-${name}"><fieldset class="plain" ${canChange() ? '' : 'disabled'}>${body}
      ${ifChange(html`<div><button class="btn primary">Save</button></div>`)}</fieldset></form></div>`;
  const form = (name) => $(`form[data-submit="save-${name}"]`);
  async function preview(f) {
    const days = +f.ret.value;
    const info = $('#ret-info');
    if (!info) return;
    const p = await GET('retention?days=' + days).catch(() => null);
    if (!p || +f.ret.value !== days) return;
    const changed = days !== st.saved;
    const what = p.audit || p.jobs
      ? html`<b>${p.audit}</b> audit entr${p.audit === 1 ? 'y' : 'ies'} and <b>${p.jobs}</b> job${p.jobs === 1 ? '' : 's'} (with their output) from before ${fmtDate(p.cutoff)}`
      : html`nothing (no entries from before ${fmtDate(p.cutoff)})`;
    setHTML(info, changed
      ? html`<div class="notice" style="margin:0">After saving, the first cleanup with ${days} day${days === 1 ? '' : 's'} runs at <b>${fmtDate(p.at)}</b> and removes ${what}.</div>`
      : html`Cleanup runs every full hour. Next cleanup: <b>${fmtDate(p.at)}</b>, removing ${what}.`);
  }
  function osInfoBox(o) {
    if (!o) return;
    const host = (o.source || '').replace(/^https?:\/\//, '').split('/')[0];
    setHTML($('#osinfo-box'), html`<div class="dim small">End-of-support dates of Debian and Ubuntu, from Debian's distro-info-data (${host}).
      ${o.fetched_at ? html`Last fetched ${fmtDate(o.fetched_at)}` : 'Not fetched yet'}${o.next_at ? html` · next check ${fmtDate(o.next_at)}` : ''}.
      ${ifChange(html`<button type="button" class="btn ghost sm" data-act="osinfo-check" ${st.osChecking ? 'disabled' : ''}>${icon('refresh')} Check now</button>`)}</div>
      ${o.error ? html`<div class="notice red small" style="margin:6px 0 0">Last lookup failed (${fmtDate(o.last_try)}): ${o.error}. Retrying every 6 hours; the previous dates stay in use.</div>` : ''}`);
  }
  function vulnBox(v) {
    if (!v) return;
    const mins = Math.ceil((v.nvd_pending * (v.nvd_key_set ? 0.8 : 6.5)) / 60);
    setHTML($('#vuln-box'), html`<div class="dim small">The installed versions of the tracked applications are looked up at OSV.dev, which uses Debian's and Ubuntu's own security data.
      Sent: the OS release (e.g. Debian:12) and source package names with their versions — no host names or addresses.
      Severity comes from the OSV record, otherwise from NVD (only CVE numbers are sent there); CISA lists the actively exploited ones and FIRST (EPSS) the chance of exploitation.
      Proxmox VE and Proxmox Backup Server come from Proxmox's own package source: their CVEs come from NVD (only the product name is sent).</div>
      <div class="dim small" style="margin-top:6px">${v.last_run ? html`Last check ${fmtDate(v.last_run)}` : 'Not checked yet'}${v.next_at ? html` · next ${fmtDate(v.next_at)}` : ''} · ${v.cves} CVE${v.cves === 1 ? '' : 's'} known for ${v.systems} device${v.systems === 1 ? '' : 's'}${v.kev_count ? html` · CISA list: ${v.kev_count} exploited CVEs` : ''}.
        ${v.nvd_pending ? html`${v.nvd_pending} severity lookup${v.nvd_pending === 1 ? '' : 's'} at NVD waiting (about ${mins} min).` : ''}
        ${ifChange(html`<button type="button" class="btn ghost sm" data-act="vuln-check" ${st.vulnChecking ? 'disabled' : ''}>${st.vulnChecking ? html`<span class="spin"></span>` : icon('refresh')} Check now</button>`)}</div>
      ${v.error ? html`<div class="notice red small" style="margin:6px 0 0">Last check failed (${fmtDate(v.last_try)}): ${v.error}. Retrying in 6 hours; the previous results stay in use.</div>` : ''}
      ${v.nvd_error ? html`<div class="notice small" style="margin:6px 0 0">NVD: ${v.nvd_error}. Retrying in a few minutes.</div>` : ''}
      ${(v.unsupported || []).map((u) => html`<div class="notice small" style="margin:6px 0 0">${u.eol
        ? `${u.name} is no longer supported: no security data and no fixes are published, so it can't be checked. Upgrade to a supported release.`
        : `OSV.dev has no data for ${u.name}: devices on that release are not checked.`}</div>`)}
      <label class="field" style="margin-top:12px">NVD API key (optional)<div class="row">
        <input type="password" name="nvdkey" autocomplete="off" style="flex:1" placeholder="${v.nvd_key_set ? 'set — enter a new key to replace it' : 'none'}">
        ${ifChange(html`<button type="button" class="btn sm" data-act="nvd-save">Save key</button>${v.nvd_key_set ? html`<button type="button" class="btn ghost sm" data-act="nvd-remove">Remove</button>` : ''}`)}</div></label>
      <div class="dim small">${v.nvd_key_set ? html`<span class="badge ok xs">key set</span> ` : ''}Without a key NVD allows about 10 lookups a minute; a free key from nvd.nist.gov makes them about 8 times faster.</div>`);
  }
  function meBox() {
    const m = st.me || {};
    setHTML($('#tfa-me'), m.ip ? html`Your current address: <span class="mono">${m.ip}</span> – ${m.trusted ? html`<span class="badge ok xs">trusted</span>` : html`<span class="badge mute xs">not trusted${m.never ? ' (never trust)' : ''}</span>`}` : '');
    const u = m.unused || [];
    setHTML($('#tfa-never-hint'), u.length ? html`<span class="c-amber">${u.join(', ')}</span> <span class="dim">${u.length === 1 ? 'is' : 'are'} not in a trusted network: no effect.</span>` : '');
  }
  const meFrom = (x) => ({ ip: x.client_ip, trusted: x.client_trusted, never: x.client_never, unused: x.tfa_never_unused });
  function urlBox() {
    const d = st.d, auto = !st.url;
    const lbl = 'Master URL used in install commands';
    const autoBtn = html`<button type="button" class="btn ghost sm" data-act="url-auto">Use automatic</button>`;
    setHTML($('#url-box'), st.editing
      ? html`<label class="field">${lbl}<input type="text" name="url" value="${st.url || d.suggested_url}" placeholder="https://host:${d.agent_port}" autofocus></label>
          <div class="row">${autoBtn}<span class="dim small">Saved with the Save button.</span></div>`
      : html`<label class="field">${lbl}<div class="row">
          <input type="text" readonly tabindex="-1" style="flex:1;opacity:.75" value="${auto ? (d.suggested_url || 'not available: open this page by host name or IP, or set it by hand') : st.url}">
          ${auto ? html`<span class="badge mute">automatic</span>` : html`<span class="badge upd">set by hand</span>`}
          ${ifChange(html`<button type="button" class="btn sm" data-act="url-change">${icon('edit')} Change</button>`)}
          ${!auto ? ifChange(autoBtn) : ''}</div></label>`);
    if (st.editing) $('#url-box input[name=url]').focus();
  }
  return {
    async mount(el) {
      const [d, lp] = await Promise.all([GET('settings'), GET('local')]);
      const s = d.settings, L = d.limits;
      // The ping of every pinged device (agentless, and agent devices with a ping on their Services tab).
      const ps = lp.settings, PL = lp.limits, pg = lp.ping || {};
      st.saved = s.retention_days;
      el.innerHTML = r(html`${settingsHead('general', 'General', 'How the master checks, cleans up and is reached')}
        ${canChange() ? '' : html`<div class="notice blue">Your account is read-only: you can see these settings but not change them.</div>`}
        <div class="setgrid">
        ${block('agents', 'Agents', 'How the master works with its agents', html`
            <label class="field">Heartbeat (seconds)<input type="number" name="hb" min="${L.heartbeat_min}" max="${L.heartbeat_max}" value="${s.heartbeat_sec}"></label>
            <div class="dim small" id="hb-hint"></div>
            <label class="field">Automatic update check interval (hours)<input type="number" name="interval" min="1" max="168" value="${s.check_interval_hours}"></label>
            <div class="dim small">Every this many hours, all agents check for package updates at once.</div>
            <label class="field">Lowest allowed agent version<select name="minagent">
              ${[...(d.agent_versions || [])].reverse().map((v, i) => html`<option value="${v}" ${v === (s.min_agent_version || d.min_agent) ? 'selected' : ''}>${v}${i === 0 ? ' (oldest stored)' : ''}</option>`)}</select></label>
            <div class="dim small">Older agents can still connect, but only to be updated: the master stores and shows none of their data and runs nothing else on them.</div>`)}
        ${block('ping', 'Ping', 'How devices are pinged, by the master and by remote probes', html`
            ${pg.error ? html`<div class="notice red" style="margin:0"><b>The master cannot send pings.</b> ${pg.error}</div>` : ''}
            <div class="dim small">The same for every device with a ping: agentless devices, and agent devices with a ping on their Services tab. Services have their own settings, on each device's Services tab.</div>
            <label class="field">Ping every (seconds)<input type="number" name="interval" min="${PL.interval_min}" max="${PL.interval_max}" value="${ps.interval_sec}" required></label>
            <div class="dim small">${PL.interval_min}–${PL.interval_max} seconds. Default 60.</div>
            <label class="field">Wait for an answer (seconds)<input type="number" name="timeout" min="${PL.timeout_min}" max="${PL.timeout_max}" value="${ps.timeout_sec}" required></label>
            <div class="dim small">${PL.timeout_min}–${PL.timeout_max} seconds. Default 2.</div>
            <label class="field">Down after this many unanswered pings in a row<input type="number" name="down" min="${PL.down_min}" max="${PL.down_max}" value="${ps.down_after}" required></label>
            <div class="dim small" id="lc-hint"></div>
            <div class="dim small">${pg.method ? `The master sends pings through a ${pg.method}.` : 'The master has sent no ping yet.'} Only the IP address is used: no DNS lookups.</div>`)}
        ${block('access', 'Website & sign-in', 'How people and agents reach the master, and how users sign in', html`
            <label class="field">Website port<select name="port">
              <option value="8443" ${s.ui_port !== 443 ? 'selected' : ''}>8443 (default)</option>
              <option value="443" ${s.ui_port === 443 ? 'selected' : ''}>443 (https)</option></select></label>
            <div class="dim small">Agents always connect on port ${d.agent_port}; this only changes where the website is offered. With 443 the website is also still reachable on ${d.agent_port}.</div>
            ${d.ui_port_error ? html`<div class="notice red" style="margin:0">${d.ui_port_error}</div>` : ''}
            <div id="url-box"></div>
            <div class="dim small">Automatic: the address your browser uses, with the agent port. Change it only if agents reach the master under a different name.</div>
            <label class="field">Ask for the 2FA code<select name="tfamode">
              <option value="" ${s.tfa_mode !== 'untrusted' ? 'selected' : ''}>Always</option>
              <option value="untrusted" ${s.tfa_mode === 'untrusted' ? 'selected' : ''}>From untrusted networks</option></select></label>
            <label class="field">Trusted networks<input type="text" name="tfanets" value="${(s.tfa_trusted || []).join(', ')}" spellcheck="false" autocomplete="off" placeholder="none"></label>
            <div class="dim small">Comma separated, e.g. <span class="mono">192.168.5.0/24, 10.0.0.7/32, fd00::/64</span>. A single address is one host (/32). At most 20; none wider than /8 (IPv4) or /32 (IPv6).
              For users with 2FA on, a sign-in from a trusted network skips the code; the password is always needed. Turning 2FA off and downloading a backup still ask for the code.</div>
            <label class="field">Never trust<input type="text" name="tfanever" value="${(s.tfa_never || []).join(', ')}" spellcheck="false" autocomplete="off" placeholder="none"></label>
            <div class="dim small">Comma separated, e.g. <span class="mono">192.168.5.1, 10.8.0.0/24</span>. Never trusted, also when a trusted network above contains them: use it for a reverse proxy or VPN server, through which every sign-in would otherwise look trusted. At most 20.</div>
            <div class="small" id="tfa-never-hint"></div>
            <div class="small" id="tfa-me"></div>`)}
        ${block('lookups', 'Online lookups', 'What the master fetches from the internet, and how often', html`
            <label class="field">Look up OS support dates<select name="osinfo">
              ${(d.os_info_choices || [3, 14, 30, 90]).map((n) => html`<option value="${n}" ${n === s.os_info_days ? 'selected' : ''}>every ${n} days${n === 14 ? ' (default)' : ''}</option>`)}</select></label>
            <div id="osinfo-box"></div>
            <label class="field">Check applications for known vulnerabilities (CVEs)<select name="vulns">
              ${(d.vuln_choices || [1, 7, 30, 90]).map((n) => html`<option value="${n}" ${n === s.vuln_days ? 'selected' : ''}>every ${n === 1 ? 'day' : n + ' days'}${n === 1 ? ' (default)' : ''}</option>`)}</select></label>
            <div id="vuln-box"></div>
            <label class="field">GitHub repository for releases<input type="text" name="gh" placeholder="owner/name" value="${s.github_repo}"></label>
            <div class="dim small mono" style="word-break:break-all">${d.index_url ? 'Version index: ' + d.index_url : ''}</div>`)}
        ${block('history', 'History & display', 'What is kept, and what is shown in addition', html`
            <label class="field">Keep audit log and job history<span class="range"><input type="range" name="ret" min="${L.retention_min}" max="${L.retention_max}" step="1" value="${s.retention_days}"><b id="ret-val"></b></span></label>
            <div class="dim small" id="ret-info"></div>
            <label class="checkrow"><input type="checkbox" name="proberesults" ${s.probe_results_shown ? 'checked' : ''}> Show received probe results on Remote Probes</label>
            <div class="dim small">Activity › Remote Probes then also lists each batch of check results a remote probe sends, for 20 seconds, with the checks that failed. For all users.</div>`)}
        </div>`);
      st.d = d;
      st.me = meFrom(d);
      meBox();
      const fa = form('access');
      const empty = () => !fa.tfanets.value.trim();
      fa.tfanets.addEventListener('input', () => { if (empty() && fa.tfamode.value) fa.tfamode.value = ''; });
      fa.tfamode.addEventListener('change', () => { if (fa.tfamode.value && empty()) { fa.tfamode.value = ''; toast('Enter a trusted network first', 'err'); } });
      st.url = s.master_url || '';
      st.editing = false;
      urlBox();
      osInfoBox(d.os_info);
      vulnBox(d.vulns);
      const fg = form('agents'), fh = form('history');
      const hbHint = () => {
        const hb = +fg.hb.value || 0;
        $('#hb-hint').textContent = `Agents contact the master at least every ${hb}s. A device is shown offline after ${hb * 3}s without contact. Commands are delivered instantly regardless.`;
      };
      const retVal = () => { const days = +fh.ret.value; $('#ret-val').textContent = `${days} day${days === 1 ? '' : 's'}`; };
      fh.ret.addEventListener('input', () => { retVal(); clearTimeout(st.timer); st.timer = setTimeout(() => preview(fh), 250); });
      fg.hb.addEventListener('input', hbHint);
      const fp = form('ping');
      const pingHint = () => {
        const n = +fp.down.value || 1, i = +fp.interval.value || 0;
        $('#lc-hint').textContent = `${PL.down_min}–${PL.down_max}. Default 3. A device is shown down after about ${dur(n * i)} without an answer, and up again at the first answer.`;
      };
      fp.down.addEventListener('input', pingHint);
      fp.interval.addEventListener('input', pingHint);
      hbHint(); retVal(); pingHint();
      preview(fh);
    },
    unmount() { clearTimeout(st.timer); },
    acts: {
      'osinfo-check': async () => {
        st.osChecking = true; osInfoBox(st.d.os_info);
        try {
          const res = await POST('osinfo/check');
          st.d.os_info = res.os_info;
          toast(res.os_info.error ? 'OS support lookup failed' : 'OS support dates updated', res.os_info.error ? 'err' : 'ok');
        } finally { st.osChecking = false; osInfoBox(st.d.os_info); }
      },
      'vuln-check': async () => {
        st.vulnChecking = true; vulnBox(st.d.vulns);
        try {
          const res = await POST('vulns/check');
          st.d.vulns = res.vulns;
          toast(res.vulns.error ? 'Vulnerability check failed' : 'Vulnerability data updated', res.vulns.error ? 'err' : 'ok');
        } finally { st.vulnChecking = false; vulnBox(st.d.vulns); }
      },
      'nvd-save': async () => {
        const k = $('#vuln-box input[name=nvdkey]').value.trim();
        if (!k) return toast('Enter the key first', 'err');
        st.d.vulns = (await POST('vulns/nvdkey', { key: k })).vulns;
        vulnBox(st.d.vulns); toast('NVD API key saved', 'ok');
      },
      'nvd-remove': async () => {
        st.d.vulns = (await POST('vulns/nvdkey', { key: '' })).vulns;
        vulnBox(st.d.vulns); toast('NVD API key removed', 'ok');
      },
      'url-change': () => { st.editing = true; urlBox(); },
      'url-auto': () => { st.url = ''; st.editing = false; urlBox(); toast('Automatic address — press Save to apply'); },
      // Each block saves only its own settings.
      'save-agents': async (f) => {
        await POST('settings/agents', { heartbeat_sec: +f.hb.value, check_interval_hours: +f.interval.value, min_agent_version: f.minagent?.value || '' });
        toast('Agents settings saved', 'ok');
      },
      'save-ping': async (f) => {
        await POST('local', { interval_sec: +f.interval.value, timeout_sec: +f.timeout.value, down_after: +f.down.value });
        toast('Ping settings saved', 'ok');
      },
      'save-access': async (f) => {
        if (!f.tfanets.value.trim()) f.tfamode.value = '';
        const url = st.editing ? f.url.value.trim() : st.url;
        const res = await POST('settings/access', { ui_port: +f.port.value, master_url: url, tfa_mode: f.tfamode.value, tfa_trusted: f.tfanets.value, tfa_never: f.tfanever.value });
        const s = res.settings;
        st.url = s.master_url || '';
        st.editing = false;
        urlBox();
        f.tfamode.value = s.tfa_mode || '';
        f.tfanets.value = (s.tfa_trusted || []).join(', ');
        f.tfanever.value = (s.tfa_never || []).join(', ');
        st.me = meFrom(res);
        meBox();
        if (res.ui_port_error) toast(res.ui_port_error, 'err');
        else toast('Website & sign-in settings saved', 'ok');
      },
      'save-lookups': async (f) => {
        await POST('settings/lookups', { os_info_days: +f.osinfo.value, vuln_days: +f.vulns.value, github_repo: f.gh.value });
        if (f.nvdkey?.value.trim()) {
          st.d.vulns = (await POST('vulns/nvdkey', { key: f.nvdkey.value.trim() })).vulns;
          vulnBox(st.d.vulns);
        }
        toast('Online lookups saved', 'ok');
      },
      'save-history': async (f) => {
        const res = await POST('settings/history', { retention_days: +f.ret.value, probe_results_shown: f.proberesults.checked });
        st.saved = res.settings.retention_days;
        toast('History & display saved', 'ok');
        preview(f);
      },
    },
  };
}

function SettingsUsers() {
  const st = { users: [] };
  const roleOpts = (sel) => html`<option value="admin" ${sel === 'admin' ? 'selected' : ''}>Administrator</option><option value="viewer" ${sel === 'viewer' ? 'selected' : ''}>Read-only</option>`;
  function render() {
    const me = session.user?.id;
    setHTML($('#u-list'), html`<div class="panel"><table class="tbl"><thead><tr><th>User</th><th>Role</th><th>Last sign-in</th><th class="w1"></th></tr></thead>
      <tbody>${st.users.map((u) => html`<tr><td><b>${u.name}</b>${u.id === me ? html` <span class="dim small">(you)</span>` : ''}
          ${u.permanent ? html` <span class="badge mute xs" title="The admin account cannot be removed or demoted">permanent</span>` : ''}
          ${u.must_change ? html` <span class="badge upd xs" title="Signs in with a generated password that must be changed first">must change password</span>` : ''}
          ${u.locked ? html` <span class="badge sec xs" title="Cannot sign in until an administrator unlocks the account">locked</span>` : ''}
          ${u.two_fa ? html` <span class="badge ok xs" title="Signs in with a code from an authenticator app${u.two_fa_since ? ' (since ' + fmtDate(u.two_fa_since) + ')' : ''}">2FA</span>` : ''}</td>
        <td>${u.id === me || u.permanent ? html`<span class="muted">${roleLabel(u.role)}</span>` : html`<select class="sm" data-change="role" data-id="${u.id}">${roleOpts(u.role)}</select>`}</td>
        <td title="Created ${fmtDate(u.created)}">${u.last_login ? html`<div class="lastin">${fmtDate(u.last_login)}</div><div class="dim small mono">${u.last_ip || '—'}</div>` : html`<span class="dim">never</span>`}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap">${u.id === me ? html`<a class="btn ghost sm" href="#/settings/account">My Account</a>`
          : html`${u.locked ? html`<button class="btn primary sm" data-act="unlock" data-id="${u.id}">Unlock</button>` : html`<button class="btn warn sm" data-act="lock" data-id="${u.id}">Lock</button>`}
            ${u.two_fa ? html`<button class="btn warn sm" data-act="tfa-off" data-id="${u.id}">Turn off 2FA</button>` : ''}
            <button class="btn warn sm" data-act="reset" data-id="${u.id}">Reset password</button>${u.permanent ? '' : html`<button class="btn danger sm" data-act="del" data-id="${u.id}" title="Remove">${icon('trash')}</button>`}`}</div></td></tr>`)}</tbody></table></div>`);
  }
  const userBy = (id) => st.users.find((u) => u.id === id);
  return {
    mount(el) {
      el.innerHTML = r(html`${settingsHead('users', 'Users', 'Who can sign in to this website')}
        <div class="stack" style="max-width:860px">
          <div class="panel panel-pad"><h2>Add a user</h2>
            <div class="muted small"><b>Administrator</b>: everything. <b>Read-only</b>: sees all devices, jobs, releases, the audit log and settings, changes nothing (except their own password) and cannot see enrollment tokens.</div>
            <form class="row" style="margin-top:12px;align-items:flex-end" data-submit="create">
              <label class="field">User name<input type="text" name="name" maxlength="32" autocapitalize="none" spellcheck="false" required style="width:180px"></label>
              <label class="field">Role<select name="role">${roleOpts('viewer')}</select></label>
              <label class="field">Password (${pwRange()})<input type="password" name="pw" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required style="width:180px"></label>
              <button class="btn primary">${icon('plus')} Add user</button>
            </form>
          </div>
          <div id="u-list"></div>
        </div>`);
    },
    async refresh() { st.users = await GET('users'); render(); },
    interval: 15000,
    acts: {
      create: async (f) => {
        await POST('users', { name: f.name.value, role: f.role.value, password: f.pw.value });
        toast(`User ${f.name.value.toLowerCase()} added`, 'ok');
        f.reset();
        view.refresh();
      },
      role: async (t) => {
        const u = userBy(t.dataset.id);
        try {
          await POST('users/' + u.id, { role: t.value });
          toast(`${u.name} is now ${roleLabel(t.value)} (signed out)`, 'ok');
        } catch (e) { t.value = u.role; throw e; }
        view.refresh();
      },
      reset: (t) => {
        const u = userBy(t.dataset.id);
        openModal(html`<h3>New password for ${u.name}</h3><form class="form" data-submit="save">
          <label class="field">New password (${pwRange()})<input type="password" name="pw" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" autofocus required></label>
          <div class="dim small">${u.name} is signed out and signs in again with the new password.</div>
          <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Set password</button></div></form>`, {
          acts: { cancel: () => closeModal(), save: async (f) => { await POST('users/' + u.id, { password: f.pw.value }); closeModal(); toast(`Password of ${u.name} changed`, 'ok'); } },
        });
      },
      lock: async (t) => {
        const u = userBy(t.dataset.id);
        if (!(await confirmBox(`Lock ${u.name}?`, `${u.name} is signed out at once and cannot sign in until an administrator unlocks the account.`, { okText: 'Lock account', danger: true }))) return;
        await POST('users/' + u.id, { locked: true });
        toast(`${u.name} locked (signed out)`, 'ok');
        view.refresh();
      },
      'tfa-off': async (t) => {
        const u = userBy(t.dataset.id);
        if (!(await confirmBox(`Turn off 2FA for ${u.name}?`, `${u.name} then signs in with the password only, and can turn 2FA on again in My Account. Use this when the authenticator app is lost.`, { okText: 'Turn off 2FA', danger: true }))) return;
        await POST('users/' + u.id, { two_fa: false });
        toast(`2FA turned off for ${u.name}`, 'ok');
        view.refresh();
      },
      unlock: async (t) => {
        const u = userBy(t.dataset.id);
        await POST('users/' + u.id, { locked: false });
        toast(`${u.name} unlocked`, 'ok');
        view.refresh();
      },
      del: async (t) => {
        const u = userBy(t.dataset.id);
        if (!(await confirmBox(`Remove user ${u.name}?`, `${u.name} can no longer sign in. Their entries in the audit log stay.`, { okText: 'Remove user', danger: true }))) return;
        await POST(`users/${u.id}/delete`);
        toast(`${u.name} removed`, 'ok');
        view.refresh();
      },
    },
  };
}

function SettingsBackups() {
  const st = { d: null };
  const slotName = (n) => (n ? `Slot ${n}` : 'Before restore');
  const made = (b) => html`${fmtDate(b.created)} <span class="dim small">· master ${b.version} · ${bytes(b.size)}</span>`;
  function render() {
    const d = st.d;
    if (!d) return;
    const rows = d.slots.map((b, i) => [i + 1, b]);
    const sources = [...rows, ...(d.before ? [[0, d.before]] : [])].filter(([, b]) => b);
    setHTML($('#bk'), html`
      <div class="panel panel-pad"><h2>Backup</h2>
        <div class="muted small">Stored in <code>${d.dir}</code>.</div>
        <table class="tbl" style="margin-top:12px"><tbody>
          ${rows.map(([n, b]) => html`<tr><td class="w1"><b>${slotName(n)}</b></td><td>${b ? made(b) : html`<span class="dim">empty</span>`}</td>
            <td class="w1">${b ? html`<button class="btn danger sm" data-act="del" data-slot="${n}">${icon('trash')} Delete</button>` : ''}</td></tr>`)}
          ${d.before ? html`<tr><td class="w1"><b>Before restore</b></td><td>${made(d.before)} <span class="dim small">· made automatically before the last restore</span></td>
            <td class="w1"><button class="btn danger sm" data-act="del" data-slot="0">${icon('trash')} Delete</button></td></tr>` : ''}
        </tbody></table>
        <div class="row" style="margin-top:14px">
          <button class="btn primary" data-act="run" ${st.busy ? 'disabled' : ''}>${st.busy ? html`<span class="spin"></span>` : icon('download')} Backup now</button>
          <button class="btn" data-act="download" ${d.has_2fa ? '' : 'disabled'} title="${d.has_2fa ? 'Create an encrypted backup and download it' : 'Turn on 2FA in My Account first'}">${icon('download')} Download backup now</button>
          ${d.has_2fa ? '' : html`<span class="dim small">Downloading needs 2FA: <a href="#/settings/account">turn it on in My Account</a>.</span>`}
        </div>
        <label class="field" style="margin-top:16px;max-width:260px">Automatic backup<select data-change="schedule">
          ${d.schedules.map((x) => html`<option value="${x}" ${x === d.schedule ? 'selected' : ''}>${{ off: 'Off', daily: 'Daily', weekly: 'Weekly (default)', monthly: 'Monthly' }[x]}</option>`)}</select></label>
        <div class="notice blue small" style="margin:14px 0 0">For safety, keep backup files secured: store copies in a safe place and protect the encryption password.</div>
      </div>
      <div class="panel panel-pad"><h2>Restore</h2>
        <div class="muted small">Replaces all Coremon data with a backup. The backup is checked first; nothing changes until you confirm. The current data is kept as the before-restore backup.</div>
        ${sources.length ? html`<table class="tbl" style="margin-top:12px"><tbody>${sources.map(([n, b]) => html`<tr><td class="w1"><b>${slotName(n)}</b></td><td>${made(b)}</td>
          <td class="w1"><button class="btn warn sm" data-act="check" data-slot="${n}">Restore…</button></td></tr>`)}</tbody></table>` : html`<div class="dim small" style="margin-top:10px">No local backups yet.</div>`}
        <form class="row" data-submit="check-file" style="margin-top:16px;align-items:flex-end">
          <label class="field">Restore from a downloaded file<input type="file" name="file" accept=".cmbak" required></label>
          <label class="field">Encryption password<input type="password" name="pw" autocomplete="off" required style="width:200px"></label>
          <button class="btn warn">Check file…</button>
        </form>
      </div>`);
  }
  async function confirmRestore(info) {
    const warn = [];
    if (info.newer_key) warn.push(`${info.newer_key} device${info.newer_key === 1 ? '' : 's'} already trust${info.newer_key === 1 ? 's' : ''} a newer master key and will need a new install command.`);
    if (info.added_since) warn.push(`${info.added_since} device${info.added_since === 1 ? ' was' : 's were'} added after this backup and will no longer be known; ${info.added_since === 1 ? 'it needs' : 'they need'} a new install command.`);
    const body = html`<div>The backup checked out fine:</div>
      <div class="facts" style="margin:10px 0"><div><div class="k">From</div><div class="v">${info.source}</div></div><div><div class="k">Made</div><div class="v">${fmtDate(info.created)}</div></div>
        <div><div class="k">Master version</div><div class="v">${info.version}</div></div><div><div class="k">Users / devices</div><div class="v">${info.users} / ${info.systems}</div></div></div>
      ${warn.map((w) => html`<div class="notice small" style="margin:6px 0">${w}</div>`)}
      <div class="notice red small" style="margin:10px 0 0"><b>Are you really sure?</b> All current Coremon data is replaced by this backup. Everyone is signed out, the master restarts
        (the website is away for a few seconds), and you then sign in with the users and passwords <b>from the backup</b>.</div>`;
    if (!(await confirmBox('Restore this backup?', body, { okText: 'Restore', danger: true }))) return;
    await POST('backups/restore', { token: info.token });
    openModal(html`<h3>Restoring…</h3><div class="row muted" style="margin-top:12px"><span class="spin"></span>The master is restarting with the restored data. This page signs you out when it is back.</div>`);
    const t0 = Date.now();
    await new Promise((res) => setTimeout(res, 3000));
    while (Date.now() - t0 < 120000) {
      try { const r = await fetch('/api/ui/session', { headers: { 'X-Coremon': '1' } }); if (r.ok) break; } catch {}
      await new Promise((res) => setTimeout(res, 1500));
    }
    closeModal();
    location.hash = '#/';
    await boot().catch(() => showLogin());
  }
  return {
    async mount(el) {
      el.innerHTML = r(html`${settingsHead('backups', 'Backups', 'Backups of the Coremon data, to import into a fresh install or to go back to if something goes wrong')}<div class="stack" style="max-width:860px" id="bk"></div>`);
    },
    async refresh() { st.d = await GET('backups'); render(); },
    interval: 15000,
    acts: {
      run: async () => {
        st.busy = true; render();
        try { st.d = await POST('backups/run'); toast('Backup created in slot 1', 'ok'); } finally { st.busy = false; render(); }
      },
      del: async (t) => {
        const n = +t.dataset.slot;
        if (!(await confirmBox(`Delete ${slotName(n).toLowerCase()}?`, 'This backup file is removed for good.', { okText: 'Delete', danger: true }))) return;
        st.d = await POST('backups/delete', { slot: n });
        toast('Backup deleted', 'ok'); render();
      },
      schedule: async (t) => {
        st.d = await POST('backups/schedule', { schedule: t.value });
        toast(t.value === 'off' ? 'Automatic backup off' : `Automatic backup: ${t.value}`, 'ok'); render();
      },
      download: () => {
        const min = st.d.min_password, max = st.d.max_password;
        openModal(html`<h3>Download backup</h3>
          <div class="muted small" style="margin-top:6px">The file is encrypted with the encryption password you choose here. It is needed to restore the backup and cannot be recovered. Use a long one.</div>
          <form class="form" data-submit="go">
            <label class="field">Code from your authenticator app<input type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]{6,7}" maxlength="7" required style="max-width:160px"></label>
            <label class="field">Encryption password (${min}–${max} characters)<input type="password" name="pw" autocomplete="new-password" minlength="${min}" maxlength="${max}" required></label>
            <label class="field">Repeat the encryption password<input type="password" name="pw2" autocomplete="new-password" minlength="${min}" maxlength="${max}" required></label>
            <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">${icon('download')} Create and download</button></div>
          </form>`, {
          acts: {
            cancel: () => closeModal(),
            go: async (f) => {
              if (f.pw.value !== f.pw2.value) return toast('The encryption passwords do not match', 'err');
              const res = await fetch('/api/ui/backups/download', { method: 'POST', headers: { 'X-Coremon': '1', 'Content-Type': 'application/json' },
                body: JSON.stringify({ code: f.code.value.replace(/\s/g, ''), password: f.pw.value }) });
              if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
              const blob = await res.blob();
              const name = (res.headers.get('Content-Disposition') || '').match(/filename="([^"]+)"/)?.[1] || 'coremon-backup.cmbak';
              const a = document.createElement('a');
              a.href = URL.createObjectURL(blob);
              a.download = name;
              document.body.appendChild(a); a.click(); a.remove();
              setTimeout(() => URL.revokeObjectURL(a.href), 10000);
              closeModal();
              toast(`Downloaded ${name}`, 'ok');
            },
          },
        });
        $('.modal input[name=code]')?.focus();
      },
      check: async (t) => confirmRestore(await POST('backups/check', { slot: +t.dataset.slot })),
      'check-file': async (f) => {
        const fd = new FormData();
        fd.append('file', f.file.files[0]);
        fd.append('password', f.pw.value);
        const res = await fetch('/api/ui/backups/check-file', { method: 'POST', headers: { 'X-Coremon': '1' }, body: fd });
        if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
        await confirmRestore(await res.json());
      },
    },
  };
}

// A QR code from the master (rows of '1' and '0') as SVG, with a quiet zone.
function qrSVG(rows) {
  const n = rows.length, q = 4;
  let d = '';
  rows.forEach((row, y) => { for (let x = 0; x < n; x++) if (row[x] === '1') d += `M${x + q},${y + q}h1v1h-1z`; });
  return raw(`<svg class="qr" viewBox="0 0 ${n + 2 * q} ${n + 2 * q}" shape-rendering="crispEdges" role="img" aria-label="QR code"><rect width="100%" height="100%" fill="#fff"/><path d="${d}" fill="#000"/></svg>`);
}

function SettingsAccount() {
  const st = { enroll: null };
  function tfaBox() {
    const u = session.user || {};
    const codeField = html`<label class="field">Code from the app<input type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]{6,7}" maxlength="7" required style="max-width:160px"></label>`;
    let body;
    if (u.two_fa) {
      body = html`<div class="row"><span class="badge ok">${icon('check')} on</span><span class="muted small">${u.two_fa_since ? 'since ' + fmtDate(u.two_fa_since) : ''}</span></div>
        <div class="muted small" style="margin-top:8px">Every sign-in asks for a code from your authenticator app.</div>
        <form class="form" data-submit="tfa-disable">${codeField}<div style="margin-top:8px"><button class="btn danger">Turn off 2FA</button></div></form>`;
    } else if (st.enroll) {
      const key = st.enroll.secret.replace(/(.{4})/g, '$1 ').trim();
      body = html`<div class="muted small">1. Scan this QR code with your authenticator app (Google or Microsoft Authenticator, 2FAS Auth, …).</div>
        <div class="qrbox">${qrSVG(st.enroll.qr)}</div>
        <div class="muted small">Can't scan? Enter this key by hand (time-based, 6 digits):</div>
        <code class="tfakey">${key}</code>
        <form class="form" data-submit="tfa-enable"><div class="muted small">2. Enter the code the app now shows to confirm.</div>${codeField}
          <div class="row" style="margin-top:8px"><button class="btn primary">Turn on 2FA</button><button type="button" class="btn ghost" data-act="tfa-cancel">Cancel</button></div></form>`;
    } else {
      body = html`<div class="row"><span class="badge mute">off</span></div>
        <div class="muted small" style="margin-top:8px">With 2FA, signing in also asks for a 6-digit code from an authenticator app on your phone.</div>
        <div style="margin-top:12px"><button class="btn primary" data-act="tfa-start">${icon('shield')} Turn on 2FA</button></div>`;
    }
    setHTML($('#tfa'), html`<h2>Two-factor authentication (2FA)</h2>${body}`);
    $('#tfa input[name=code]')?.focus();
  }
  return {
    mount(el) {
      el.innerHTML = r(html`${settingsHead('account', 'My Account', `Signed in as ${session.user?.name} (${roleLabel(session.user?.role)})`)}
        ${session.initial_password ? html`<div class="notice">You are still using the generated initial password — please change it.</div>` : ''}
        <div class="stack" style="max-width:480px">
        <div class="panel panel-pad previn"><span class="previn-ic">${icon('clock')}</span>
          <div><div class="previn-lbl">Previous sign-in</div>
          ${session.prev_login ? html`<div class="previn-when">${fmtDate(session.prev_login)} <span class="previn-ago">${ago(session.prev_login)}</span></div>
            <div class="previn-ip">from <span class="mono">${session.prev_ip || 'an unknown address'}</span></div>`
    : html`<div class="previn-when">This is your first sign-in</div><div class="previn-ip">Next time, your previous sign-in shows here.</div>`}</div></div>
        <div class="panel panel-pad"><h2>Notification email</h2>
          <form class="form" data-submit="email" style="margin-top:0">
            <label class="field">Email address<input type="email" name="email" value="${session.notify_email || ''}" maxlength="254" autocomplete="email" placeholder="name@example.com"></label>
            <div style="margin-top:8px"><button class="btn primary">Save</button></div>
          </form>
        </div>
        <div class="panel panel-pad"><h2>Change password</h2>
          <form class="form" data-submit="pw" style="margin-top:0">
            <label class="field">Current password<input type="password" name="cur" autocomplete="current-password" required></label>
            <label class="field">New password (${pwRange()})<input type="password" name="new" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
            <label class="field">Repeat new password<input type="password" name="new2" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
            <div style="margin-top:8px"><button class="btn primary">Change password</button></div>
          </form>
        </div>
        <div class="panel panel-pad" id="tfa"></div></div>`);
      tfaBox();
    },
    acts: {
      email: async (f) => {
        const res = await POST('account/email', { email: f.email.value.trim() });
        const before = session.notify_email || '';
        session.notify_email = res.email;
        f.email.value = res.email;
        toast(res.email === before ? 'Saved' : res.email ? 'Notification email saved' : 'Notification email removed', 'ok');
      },
      pw: async (f) => {
        if (f.new.value !== f.new2.value) return toast('New passwords do not match', 'err');
        await POST('password', { current: f.cur.value, new: f.new.value });
        session.initial_password = false;
        f.reset();
        toast('Password changed', 'ok');
        route();
      },
      'tfa-start': async () => { st.enroll = await POST('account/2fa/start'); tfaBox(); },
      'tfa-cancel': () => { st.enroll = null; tfaBox(); },
      'tfa-enable': async (f) => {
        await POST('account/2fa/enable', { code: f.code.value.replace(/\s/g, '') });
        st.enroll = null;
        session.user = { ...session.user, two_fa: true, two_fa_since: Math.floor(Date.now() / 1000) };
        toast('2FA is on (other sessions signed out)', 'ok');
        tfaBox();
      },
      'tfa-disable': async (f) => {
        await POST('account/2fa/disable', { code: f.code.value.replace(/\s/g, '') });
        session.user = { ...session.user, two_fa: false, two_fa_since: 0 };
        toast('2FA is off', 'ok');
        tfaBox();
      },
    },
  };
}

function SettingsIdentity() {
  const st = { d: null };
  function slot(s) {
    if (s === 'current') return html`<span class="badge ok">${icon('check')} current</span>`;
    if (s === 'new') return html`<span class="badge info">${icon('check')} new</span>`;
    if (s) return html`<span class="badge sec">✗ unknown</span>`;
    return html`<span class="dim">—</span>`;
  }
  function status(a, d) {
    if (!a.reported_at) return html`<span class="dim">not reported yet</span>`;
    if (a.slots.includes('unknown')) return html`<span class="badge sec">trusts an unknown key</span>`;
    if (d.rotating) return a.has_next ? html`<span class="badge ok">ready to switch</span>` : html`<span class="badge upd">waiting for the new key</span>`;
    return a.slots.includes('current') && !a.slots.some((s) => s && s !== 'current') ? html`<span class="badge ok">${icon('check')} correct</span>` : html`<span class="badge upd">updating keys</span>`;
  }
  function render() {
    const d = st.d;
    if (!d) return;
    const ready = d.agents.filter((a) => a.has_next).length;
    setHTML($('#id-keys'), html`
      <div class="panel panel-pad"><h2>Master payload key</h2>
        <div class="muted small">All traffic between master and agents is encrypted and signed with this key, independent of the HTTPS certificate. Agents only accept messages signed by a key they trust. Each agent has two key slots, so a new key can be handed over in a message signed with the current one.</div>
        ${d.full ? html`<div class="keyrow"><span class="klbl">Current key</span><code class="mono">sha256//${d.current}</code></div>
          ${d.next ? html`<div class="keyrow"><span class="klbl">New key</span><code class="mono">sha256//${d.next}</code><span class="dim small">generated ${fmtDate(d.next_at)}</span></div>` : ''}`
        : html`<div class="dim small" style="margin-top:12px">Key fingerprints are shown to administrators only.${d.rotating ? ` A new key was generated ${fmtDate(d.next_at)} and is being handed to the devices.` : ''}</div>`}
        ${canChange() ? html`<div class="row" style="margin-top:14px">
          ${d.rotating ? html`<span class="muted small">${ready} of ${d.agents.length} device(s) have the new key.</span><span class="spacer"></span>
              <button class="btn ghost" data-act="cancel-key">Cancel</button>
              <button class="btn primary" data-act="switch-key">${icon('refresh')} Switch to new key</button>`
            : html`<span class="muted small">Generate a new key if the current one may have leaked. Nothing changes until you switch.</span><span class="spacer"></span>
              <button class="btn" data-act="gen-key">${icon('plus')} Generate new key</button>`}
        </div>` : ''}
        <div class="dim small" style="margin-top:12px">If this key is ever lost (for example a new master without a backup), agents stop connecting. Run a new install command from Settings → Devices on each device; it updates the stored key.</div>
      </div>
      <div class="panel panel-pad" style="margin-top:18px"><h2>Website certificate (HTTPS)</h2>
        <div class="muted small">Carries the traffic; self-signed for now. Agents do not rely on it.</div>
        ${d.full ? html`<div class="keyrow"><span class="klbl">Fingerprint</span><code class="mono">sha256//${d.https}</code></div>` : ''}
        <div class="keyrow"><span class="klbl">Valid</span><span>${fmtDate(d.cert_from)} – ${fmtDate(d.cert_until)}
          ${d.cert_until - now() < d.cert_renew_days * 86400 ? html` <span class="badge upd xs">renewing soon</span>` : ''}</span></div>
        ${canChange() ? html`<div class="row" style="margin-top:14px"><span class="muted small">Renewed automatically ${d.cert_renew_days} days before it expires.</span><span class="spacer"></span>
          <button class="btn" data-act="new-cert">${icon('refresh')} Generate new certificate</button></div>` : ''}
      </div>`);
    d.agents = d.agents.filter((a) => !(a.core_version && versionLess(a.core_version, '0.3.0')));
    setHTML($('#id-agents'), d.agents.length ? html`<div class="panel"><table class="tbl">
      <thead><tr><th>Device</th><th>Slot 1</th><th>Slot 2</th><th>Status</th><th>Reported</th></tr></thead>
      <tbody>${d.agents.map((a) => html`<tr><td>${agentDot(a)} <a href="#/device/${a.id}"><b>${a.name}</b></a></td>
        <td>${slot(a.slots[0])}</td>
        <td>${slot(a.slots[1])}</td>
        <td>${status(a, d)}</td>
        <td class="muted small">${a.reported_at ? when(a.reported_at) : '—'}</td></tr>`)}</tbody></table></div>`
      : html`<div class="panel empty">No devices yet.</div>`);
  }
  async function act(action, body = {}) {
    st.d = await POST('identity/' + action, body);
    render();
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`${settingsHead('identity', 'Identity', 'How agents recognise this master')}
        <div class="stack" style="max-width:1000px"><div id="id-keys"></div>
          <div><h2 style="font-size:15px;margin:6px 0 12px">Keys per device</h2><div id="id-agents"></div></div>
          <div class="dim small">Coremon v${session.version}${releasedLabel() ? ' · ' + releasedLabel() : ''}</div></div>`);
    },
    async refresh() { st.d = await GET('identity'); render(); },
    acts: {
      'gen-key': async () => {
        if (!(await confirmBox('Generate a new master key?', 'The master keeps using the current key. The new key is sent to every device over its current, verified connection; you switch when they all have it.', { okText: 'Generate' }))) return;
        await act('generate'); toast('New key generated — distributing to devices', 'ok');
      },
      'new-cert': async () => {
        if (!(await confirmBox('Generate a new HTTPS certificate?', html`The current certificate is replaced by a new self-signed one, valid for 1 year, and the old one is deleted. It is used immediately.
          <ul style="margin:10px 0 0 18px"><li>Every browser that accepted the old certificate shows the security warning once more (yours too, on the next page load).</li>
          <li>Agents and install commands are not affected.</li></ul>`, { okText: 'Generate new certificate' }))) return;
        await act('cert');
        toast('New certificate in use — reload the page and accept it', 'ok');
      },
      'cancel-key': async () => {
        if (!(await confirmBox('Cancel the new key?', 'The new key is discarded and devices remove it again. The current key stays in use.', { okText: 'Discard new key', danger: true }))) return;
        await act('cancel'); toast('New key discarded', 'ok');
      },
      'switch-key': async () => {
        const missing = st.d.agents.filter((a) => !a.has_next);
        if (!missing.length) {
          if (!(await confirmBox('Switch to the new key?', 'The master starts using the new key and the old key is deleted. Devices reconnect with the new key and remove the old one.', { okText: 'Switch' }))) return;
          await act('switch'); toast('Switched to the new key', 'ok'); return;
        }
        await new Promise((resolve) => {
          openModal(html`<h3>Not all devices have the new key</h3>
            <div class="notice red" style="margin:12px 0 0">These ${missing.length} device(s) have not reported the new key and will <b>lose contact</b> after the switch:
              <ul>${missing.map((a) => html`<li><b>${a.name}</b> — ${a.online ? 'waiting for the new key' : 'offline'}</li>`)}</ul>
              They can be reconnected later with a new install command.</div>
            <div class="actions"><button class="btn primary" data-act="no" autofocus>Cancel</button><button class="btn danger" data-act="yes">Switch anyway</button></div>`, {
            acts: {
              no: () => { closeModal(); },
              yes: async () => { closeModal(); try { await act('switch', { force: true }); toast('Switched to the new key', 'ok'); } catch (e) { fail(e); } },
            },
            onClose: resolve,
          });
        });
      },
    },
  };
}

boot().catch(() => showLogin());
