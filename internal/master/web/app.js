// coremon web UI - plain ES modules, no build step, no third-party code.

// ---------- tiny templating ----------
class Raw { constructor(s) { this.s = s; } }
const raw = (s) => new Raw(s);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const r = (v) => (v == null || v === false ? '' : v instanceof Raw ? v.s : Array.isArray(v) ? v.map(r).join('') : esc(v));
function html(strings, ...vals) {
  let out = '';
  strings.forEach((s, i) => { out += s; if (i < vals.length) out += r(vals[i]); });
  return raw(out);
}
// setHTML only touches the DOM when content changed, and keeps scroll
// positions of [data-keep-scroll] containers.
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

// ---------- icons ----------
const ICONS = {
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
};
const icon = (n) => raw(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[n] || ''}</svg>`);

// ---------- formatting: 24-hour clock, dd-mm-yyyy dates everywhere ----------
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
// Relative time with the full date and time on hover.
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

// ---------- api ----------
async function api(method, path, body) {
  const res = await fetch('/api/ui/' + path, {
    method,
    headers: { 'X-Coremon': '1', ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (res.status === 401 && path !== 'login' && path !== 'password') {
    showLogin();
    throw new Error('Session expired');
  }
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return (res.headers.get('content-type') || '').includes('json') ? res.json() : null;
}
const GET = (p) => api('GET', p);
const POST = (p, b) => api('POST', p, b || {});

// ---------- toasts ----------
function toast(msg, kind = '') {
  const el = document.createElement('div');
  el.className = 'toast ' + kind;
  el.textContent = msg;
  $('#toasts').appendChild(el);
  setTimeout(() => el.remove(), kind === 'err' ? 7000 : 3500);
}
const fail = (e) => toast(e.message || String(e), 'err');

// ---------- modals ----------
let modal = null;
function openModal(content, { wide = false, acts = {}, onClose } = {}) {
  closeModal();
  const root = $('#modal-root');
  root.innerHTML = `<div class="modal-back"><div class="modal ${wide ? 'wide' : ''}">${r(content)}</div></div>`;
  modal = { el: root.firstElementChild, acts, onClose };
  modal.el.addEventListener('mousedown', (e) => { if (e.target === modal?.el) closeModal(); });
  const f = modal.el.querySelector('[autofocus]');
  if (f) f.focus();
  return modal;
}
function closeModal() {
  if (!modal) return;
  const m = modal;
  modal = null;
  $('#modal-root').innerHTML = '';
  m.onClose && m.onClose();
}
function confirmBox(title, body, { okText = 'Confirm', danger = false } = {}) {
  return new Promise((resolve) => {
    let done = false;
    openModal(html`<h3>${title}</h3><div class="muted" style="margin-top:8px">${body}</div>
      <div class="actions"><button class="btn ghost" data-act="no">Cancel</button>
      <button class="btn ${danger ? 'danger' : 'primary'}" data-act="yes" autofocus>${okText}</button></div>`, {
      acts: { yes: () => { done = true; closeModal(); resolve(true); }, no: () => closeModal() },
      onClose: () => { if (!done) resolve(false); },
    });
  });
}
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeModal(); });

// ---------- job log modal ----------
function statusBadge(st) {
  const m = { queued: ['mute', 'queued'], running: ['info', 'running'], success: ['ok', 'success'], failed: ['sec', 'failed'], timeout: ['sec', 'timeout'] };
  const [c, t] = m[st] || ['mute', st];
  return html`<span class="badge ${c}">${st === 'running' ? raw('<span class="spin" style="width:10px;height:10px"></span>') : ''}${t}</span>`;
}
function jobTitle(j) {
  switch (j.type) {
    case 'action': return `${j.feature} · ${j.action}`;
    case 'core.update': return `agent → ${j.artifact ? j.artifact.version : '?'}`;
    case 'agent.uninstall': return 'uninstall agent';
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
  { wide: true, acts: { close: () => closeModal() }, onClose: () => { stopped = true; clearTimeout(timer); } });
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

// ---------- event delegation ----------
let view = null;
const globalActs = {
  'open-job': (t) => openJob(t.dataset.id),
  logout: async () => { await POST('logout').catch(() => {}); showLogin(); },
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
// <select data-change="name"> calls view.acts[name](select) when changed.
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

// ---------- shell & routing ----------
let session = { version: '', min_password: 5 };
let refreshTimer = null;
let pendingCount = 0;

function showLogin() {
  clearInterval(refreshTimer);
  view = null;
  closeModal();
  setHTML($('#app'), html`
    <div class="login"><div class="panel">
      <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
      <div class="muted">Fleet update control</div>
      <form data-submit="login" id="login-form">
        <label class="field">Admin password<input type="password" name="password" autocomplete="current-password" autofocus required></label>
        <div class="loginerr" id="login-err" role="alert"></div>
        <button class="btn primary" type="submit">Sign in</button>
      </form>
      <div class="hint">First start? The initial password is in <code>/var/lib/coremon-master/initial-password.txt</code> on the master.</div>
    </div></div>`);
  view = {
    acts: {
      login: async (f) => {
        const err = $('#login-err');
        err.textContent = '';
        f.password.classList.remove('bad');
        try {
          await POST('login', { password: f.password.value });
          await boot();
        } catch (e) {
          const m = e.message || '';
          err.textContent = /wrong password/i.test(m) ? 'Wrong password' : /too many/i.test(m) ? 'Too many attempts, try again in a few minutes' : m;
          f.password.classList.add('bad');
          f.password.select();
        }
      },
    },
  };
  $('#login-form input').focus();
}

const NAV = [
  ['', 'dash', 'Dashboard'],
  ['jobs', 'jobs', 'Jobs'],
  ['releases', 'box', 'Releases'],
  ['audit', 'shield', 'Audit log'],
  ['settings', 'cog', 'Settings'],
];
// Settings submenu, always shown below "Settings".
const SETTINGS_NAV = [['general', 'General'], ['systems', 'Systems'], ['login', 'Login'], ['identity', 'Identity']];
function currentRoute() {
  const p = location.hash.replace(/^#\/?/, '').split('/');
  if (p[0] === 'systems') return ['settings', 'systems']; // old address
  if (p[0] === 'settings') return ['settings', SETTINGS_NAV.some(([k]) => k === p[1]) ? p[1] : 'general'];
  return [p[0] === 'agent' ? '' : p[0], ''];
}
const releasedLabel = () => (session.build_date ? 'Released: ' + session.build_date.split('-').reverse().join('-') : '');
const settingsTabs = (cur) => html`<div class="settabs">${SETTINGS_NAV.map(([k, l]) => html`<a href="#/settings/${k}" class="${cur === k ? 'on' : ''}">${l}</a>`)}</div>`;

function renderShell() {
  const [active, sub] = currentRoute();
  setHTML($('#app'), html`
    <div class="shell">
      <aside class="side">
        <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
        <nav class="nav">${NAV.map(([h, ic, lbl]) => html`<a href="#/${h}" class="${active === h ? 'active' : ''}">${icon(ic)}<span class="lbl">${lbl}</span></a>
          ${h === 'settings' ? html`<div class="subnav">${SETTINGS_NAV.map(([k, l]) => html`<a href="#/settings/${k}" class="${sub === k ? 'active' : ''}">${l}${k === 'systems' ? html`<span id="nav-pending"></span>` : ''}</a>`)}</div>` : ''}`)}</nav>
        <div class="foot"><div class="verinfo" title="Website and master version"><span>Coremon v${session.version}</span>${releasedLabel() ? html`<span class="dim">${releasedLabel()}</span>` : ''}</div><button class="btn ghost sm iconbtn" data-act="logout" title="Sign out">${icon('logout')}</button></div>
      </aside>
      <main class="main" id="view"></main>
    </div>`);
  setPending(pendingCount);
}
function setPending(n) {
  pendingCount = n;
  const el = $('#nav-pending');
  if (el) { el.className = n ? 'count' : ''; el.textContent = n ? String(n) : ''; }
}

function route() {
  closeModal();
  clearInterval(refreshTimer);
  if (view?.unmount) view.unmount();
  const parts = location.hash.replace(/^#\/?/, '').split('/');
  const [, sub] = currentRoute();
  const settingsViews = { general: SettingsGeneral, systems: Systems, login: SettingsLogin, identity: SettingsIdentity };
  const views = { '': Dashboard, agent: AgentPage, systems: Systems, jobs: Jobs, releases: Releases, audit: Audit, settings: settingsViews[sub] };
  const V = views[parts[0]] || Dashboard;
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
  if (!session.authenticated) return showLogin();
  route();
}

// ---------- shared bits ----------
function agentDot(a) {
  if (a.status === 'pending') return html`<span class="dot pend" title="pending approval"></span>`;
  if (a.status === 'paused') return html`<span class="dot pause" title="paused"></span>`;
  return html`<span class="dot ${a.online ? 'on' : 'off'}" title="${a.online ? 'online' : 'offline since ' + fmtDate(a.last_seen)}"></span>`;
}
function osLabel(f) { return f.os_name || [f.os_id, f.os_version].filter(Boolean).join(' ') || 'unknown OS'; }
const hasUpdates = (a) => (a.features || []).includes('updates');
const canReboot = (a) => (a.features || []).includes('system');
// First IPv4 address; the others on hover.
function ipLabel(f) {
  const ips = f.ips || [];
  if (!ips.length) return html`<span class="dim">—</span>`;
  const first = ips.find((ip) => ip.includes('.')) || ips[0];
  const more = ips.length - 1;
  return html`<span class="ip" title="${ips.join('\n')}">${first}${more ? html` <span class="dim">+${more}</span>` : ''}</span>`;
}
const byName = (a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true });

// Agent version: green when it is the latest, orange otherwise.
function verPill(a) {
  if (!a.core_version) return '';
  const latest = a.core_is_latest;
  const tip = a.core_latest ? (latest ? 'latest agent version' : `latest is ${a.core_latest}`) : 'no agent versions on the master yet';
  return html`<span class="verpill ${latest ? 'ok' : 'old'}" title="${tip}">agent ${a.core_version}</span>${a.desired_core ? html`<span class="verpill busy" title="change requested"><span class="spin" style="width:9px;height:9px"></span> → ${a.desired_core}</span>` : ''}`;
}
function updBadges(a) {
  const u = a.updates;
  const out = [];
  if (u?.security) out.push(html`<span class="badge sec">${u.security} security</span>`);
  if (u?.reboot_required) out.push(html`<span class="badge reboot">reboot</span>`);
  return out;
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
  const res = await POST(`agents/${a.id}/core`, { version });
  toast(`${a.name}: agent ${a.core_version} → ${version} requested${res.job ? '' : ' (applies when the system is online)'}`, 'ok');
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

// Layout helpers: the complete arrangement as {gid: [agent ids]} in order.
function layoutOf(data) {
  const lists = new Map(data.groups.map((g) => [g.id, []]));
  lists.set('', []);
  for (const a of data.agents) (lists.get(a.group_id) || lists.get('')).push(a.id);
  return lists;
}
async function saveLayout(groupsOrder, lists) {
  const body = { groups: [...groupsOrder.map((id) => ({ id, agents: lists.get(id) || [] })), { id: '', agents: lists.get('') || [] }] };
  await POST('layout', body);
}

// ---------- Dashboard ----------
function Dashboard() {
  const st = { filter: 'all', q: '', sel: new Set(), data: null, folded: new Set(), drag: null };
  const filtering = () => st.filter !== 'all' || st.q !== '';
  const matches = (a) => {
    const u = a.updates || {};
    const f = st.filter;
    if (f === 'updates' && !(u.total > 0)) return false;
    if (f === 'security' && !(u.security > 0)) return false;
    if (f === 'reboot' && !u.reboot_required) return false;
    if (f === 'offline' && (a.online || a.status === 'pending')) return false;
    if (f === 'pending' && a.status !== 'pending') return false;
    if (f === 'agent' && (a.core_is_latest || !a.core_version)) return false;
    if (st.q) {
      const g = st.data.groups.find((x) => x.id === a.group_id);
      const hay = `${a.name} ${osLabel(a.facts)} ${(a.facts.ips || []).join(' ')} ${g ? g.name : ''} ${a.core_version}`.toLowerCase();
      if (!hay.includes(st.q.toLowerCase())) return false;
    }
    return true;
  };
  function card(a) {
    const u = a.updates;
    const f = a.facts;
    let accent = 'a-ok';
    if (a.status === 'pending') accent = 'a-pend';
    else if (!a.online) accent = 'a-off';
    else if (u?.security) accent = 'a-sec';
    else if (u?.total) accent = 'a-upd';
    const selectable = a.status === 'active';
    let mid;
    if (a.status === 'pending') {
      mid = html`<div class="mid"><div><div class="biglbl">Waiting for approval</div><div class="dim small">enrolled ${when(a.enrolled_at)}</div></div>
        <div class="badges"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button></div></div>`;
    } else if (!hasUpdates(a) && a.online) {
      mid = html`<div class="mid"><div class="big dim">—</div><div class="biglbl">${a.legacy ? html`agent too old for<br>built-in updates` : 'no update function'}</div>
        <div class="badges">${a.legacy && a.core_latest ? html`<button class="btn sm" data-act="agent-latest" data-id="${a.id}">${icon('up')} Update agent</button>` : ''}</div></div>`;
    } else if (!u) {
      mid = html`<div class="mid"><div class="big dim">—</div><div class="biglbl">waiting for<br>first check</div><div class="badges">${updBadges(a)}</div></div>`;
    } else {
      const cls = u.security ? 'sec' : u.total ? 'some' : 'zero';
      mid = html`<div class="mid"><div class="big ${cls}">${u.total}</div><div class="biglbl">${u.total === 1 ? 'update' : 'updates'}<br>available</div><div class="badges">${updBadges(a)}</div></div>`;
    }
    return html`
      <div class="card ${accent} ${st.sel.has(a.id) ? 'sel' : ''}" data-act="open-agent" data-id="${a.id}" data-drag="agent" draggable="true">
        <div class="top"><span class="grip" title="Drag to reorder or move to another group">${icon('grip')}</span>${agentDot(a)}
          <div style="min-width:0"><div class="name">${a.name}</div><div class="os">${osLabel(f)} · ${f.arch || '?'} · ${f.virt || ''}</div><div class="os">${ipLabel(f)}</div></div>
          ${selectable ? html`<input type="checkbox" class="selbox" data-act="sel" data-id="${a.id}" ${st.sel.has(a.id) ? 'checked' : ''} title="Select">` : ''}</div>
        ${mid}
        ${a.status !== 'pending' ? html`<div class="inforow"><span class="uptime" title="uptime">${icon('clock')}${a.online ? 'up ' + dur(f.uptime_sec) : 'offline'}</span><span class="spacer"></span>${verPill(a)}</div>` : ''}
        <div class="foot">
          ${a.busy ? html`<span class="spin"></span><span>${a.busy} job${a.busy > 1 ? 's' : ''}</span>` : html`<span>${a.online ? (u ? when(u.checked_at, 'checked ') : 'online') : a.status === 'pending' ? '' : when(a.last_seen, 'seen ')}</span>`}
        </div>
      </div>`;
  }
  function section(g, agents, showHead, dragOnly = false) {
    const list = agents.filter(matches);
    if (dragOnly) {
      // Shown only while dragging, so a system can always leave its group.
      return html`<section class="gsec dragonly" data-gid=""><div class="ghead"><span class="gname">Ungrouped</span></div>
        <div class="gbody" data-gid=""><div class="gempty">Drop here to take a system out of its group</div></div></section>`;
    }
    if (filtering() && !list.length) return '';
    const folded = st.folded.has(g.id);
    const active = agents.filter((a) => a.status !== 'pending');
    const upd = active.reduce((s, a) => s + (a.updates?.total || 0), 0);
    const sec = active.reduce((s, a) => s + (a.updates?.security || 0), 0);
    const off = active.filter((a) => !a.online).length;
    const selectable = list.filter((a) => a.status === 'active');
    const allSel = selectable.length && selectable.every((a) => st.sel.has(a.id));
    const head = showHead ? html`
      <div class="ghead" data-gid="${g.id}" ${g.id ? raw('data-drag="group" draggable="true"') : ''}>
        <button class="btn ghost sm iconbtn fold ${folded ? 'shut' : ''}" data-act="fold" data-gid="${g.id}" title="${folded ? 'Unfold' : 'Fold'}">${icon('chev')}</button>
        ${g.id ? html`<span class="grip" title="Drag to reorder groups">${icon('grip')}</span>` : ''}
        <span class="gname">${g.name}</span>
        <span class="gsum">${agents.length} system${agents.length === 1 ? '' : 's'}${upd ? html` · <b class="c-amber">${upd} update${upd === 1 ? '' : 's'}</b>` : ''}${sec ? html` · <b class="c-red">${sec} security</b>` : ''}${off ? html` · <b class="c-red">${off} offline</b>` : ''}</span>
        <span class="spacer"></span>
        ${selectable.length ? html`<label class="gsel" title="Select all systems in this group"><input type="checkbox" data-act="sel-group" data-gid="${g.id}" ${allSel ? 'checked' : ''}> select</label>` : ''}
      </div>` : '';
    const body = folded ? '' : list.length
      ? html`<div class="cards">${list.map(card)}</div>`
      : html`<div class="gempty">${g.id ? 'Empty group — drag systems here' : 'No ungrouped systems'}</div>`;
    return html`<section class="gsec" data-gid="${g.id}">${head}<div class="gbody" data-gid="${g.id}">${body}</div></section>`;
  }
  function render() {
    const d = st.data;
    if (!d || st.drag) return;
    const A = d.agents;
    const active = A.filter((a) => a.status !== 'pending');
    const online = active.filter((a) => a.online).length;
    const totUpd = active.reduce((s, a) => s + (a.updates?.total || 0), 0);
    const totSec = active.reduce((s, a) => s + (a.updates?.security || 0), 0);
    const reboot = active.filter((a) => a.updates?.reboot_required).length;
    const pending = A.filter((a) => a.status === 'pending').length;
    setPending(pending);
    setHTML($('#d-stats'), html`
      <div class="panel stat c-green"><div class="label">Systems online</div><div class="value">${online}<small> / ${active.length}</small></div></div>
      <div class="panel stat c-amber"><div class="label">Pending updates</div><div class="value">${totUpd}</div></div>
      <div class="panel stat c-red"><div class="label">Security updates</div><div class="value">${totSec}</div></div>
      <div class="panel stat c-violet"><div class="label">Reboot required</div><div class="value">${reboot}</div></div>
      <div class="panel stat c-cyan"><div class="label">Active jobs</div><div class="value">${d.active_jobs}</div></div>`);
    const count = (f) => { const save = st.filter; st.filter = f; const n = A.filter(matches).length; st.filter = save; return n; };
    const chips = [['all', 'All'], ['updates', 'Updates'], ['security', 'Security'], ['reboot', 'Reboot'], ['offline', 'Offline'], ['pending', 'Pending'], ['agent', 'Agent outdated']];
    setHTML($('#d-chips'), chips.map(([k, l]) => html`<button class="chip ${st.filter === k ? 'on' : ''}" data-act="filter" data-f="${k}">${l}<span class="n">${count(k)}</span></button>`));
    if (!A.length) {
      setHTML($('#d-cards'), html`<div class="panel empty"><div class="big">🛰️</div><div>No systems yet.</div><div style="margin-top:14px"><a class="btn primary" href="#/settings/systems">${icon('plus')} Add your first system</a></div></div>`);
    } else {
      const groups = d.groups;
      const showHead = groups.length > 0;
      const secs = [...groups, { id: '', name: 'Ungrouped' }].map((g) => {
        const members = A.filter((a) => (a.group_id || '') === g.id || (g.id === '' && !groups.some((x) => x.id === a.group_id)));
        if (!g.id && showHead && !members.length) return section(g, members, showHead, true);
        return section(g, members, showHead);
      });
      const any = A.some(matches);
      setHTML($('#d-cards'), any ? secs : html`<div class="panel empty">No systems match this filter.</div>`);
    }
    for (const id of [...st.sel]) if (!A.some((a) => a.id === id && a.status === 'active')) st.sel.delete(id);
    const n = st.sel.size;
    setHTML($('#d-bulk'), n ? html`<div class="bulkbar"><b>${n}</b> selected<span class="spacer"></span>
      <button class="btn sm" data-act="bulk-check">${icon('refresh')} Check now</button>
      <button class="btn warn sm" data-act="bulk-upgrade">${icon('zap')} Upgrade all packages</button>
      <button class="btn sm" data-act="bulk-agent">${icon('up')} Agent → latest</button>
      <button class="btn ghost sm" data-act="bulk-clear">Clear</button></div>` : '');
  }

  // ----- drag and drop (systems within/between groups, and groups) -----
  function clearMarks() { document.querySelectorAll('.drop-before,.drop-after,.drop-into').forEach((e) => e.classList.remove('drop-before', 'drop-after', 'drop-into')); }
  function dropTarget(e) {
    if (st.drag.type === 'agent') {
      const c = e.target.closest('.card[data-id]');
      if (c && c.dataset.id === st.drag.id) return null;
      if (c) {
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
        const ta = d.agents.find((a) => a.id === t.id);
        const l = lists.get(ta.group_id) || lists.get('');
        const i = l.indexOf(t.id);
        l.splice(t.after ? i + 1 : i, 0, drag.id);
      } else {
        (lists.get(t.gid) || lists.get('')).push(drag.id);
      }
      // Optimistic local update.
      for (const [gid, l] of lists) l.forEach((id, i) => { const a = d.agents.find((x) => x.id === id); a.group_id = gid; a.order = i; });
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
        <div class="page-head"><div><h1>Dashboard</h1><div class="sub">Update status across your fleet · drag systems and groups to arrange them</div></div><span class="spacer"></span>
          <button class="btn" data-act="check-all">${icon('refresh')} Check all</button></div>
        <div id="d-notice"></div>
        <div class="stats" id="d-stats"></div>
        <div class="toolbar"><input type="search" class="search" id="d-q" placeholder="Search name, OS, IP, group…"><div class="chips" id="d-chips"></div></div>
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
    async refresh() { const d = await GET('overview'); if (!st.drag) { st.data = d; render(); } },
    acts: {
      filter: (t) => { st.filter = t.dataset.f; render(); },
      fold: (t) => { const g = t.dataset.gid; st.folded.has(g) ? st.folded.delete(g) : st.folded.add(g); render(); },
      'open-agent': (t) => { location.hash = '#/agent/' + t.dataset.id; },
      sel: (t) => { t.checked ? st.sel.add(t.dataset.id) : st.sel.delete(t.dataset.id); render(); },
      'sel-group': (t) => {
        const gid = t.dataset.gid;
        const ids = st.data.agents.filter((a) => (a.group_id || '') === gid && a.status === 'active' && matches(a)).map((a) => a.id);
        ids.forEach((id) => (t.checked ? st.sel.add(id) : st.sel.delete(id)));
        render();
      },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.groups),
      'agent-latest': (t) => { const a = st.data.agents.find((x) => x.id === t.dataset.id); return setAgentVersion(a, a.core_latest); },
      'bulk-clear': () => { st.sel.clear(); render(); },
      'bulk-check': () => runAction([...st.sel].filter((id) => hasUpdates(st.data.agents.find((a) => a.id === id))), 'updates', 'check', {}, { open: false }),
      'bulk-upgrade': async () => {
        const names = st.data.agents.filter((a) => st.sel.has(a.id)).map((a) => a.name);
        if (await confirmBox('Upgrade all packages?', `This runs a full package upgrade on ${names.length} system(s): ${names.join(', ')}.`, { okText: 'Upgrade all', danger: true })) {
          await runAction([...st.sel], 'updates', 'upgrade_all', {}, { open: false });
          st.sel.clear();
        }
      },
      'bulk-agent': async () => {
        const list = st.data.agents.filter((a) => st.sel.has(a.id) && !a.core_is_latest && a.core_latest);
        if (!list.length) return toast('The selected systems already run the latest agent');
        if (!(await confirmBox(`Update the agent to ${list[0].core_latest}?`, `${list.map((a) => `${a.name} (${a.core_version})`).join(', ')}. Each agent downloads the signed binary and restarts.`, { okText: 'Update agents' }))) return;
        for (const a of list) await POST(`agents/${a.id}/core`, { version: a.core_latest });
        toast(`${list.length} agent update(s) requested`, 'ok');
        view.refresh();
      },
      'check-all': () => {
        const ids = st.data.agents.filter((a) => a.status === 'active' && a.online && hasUpdates(a)).map((a) => a.id);
        if (!ids.length) return toast('No online systems with the update function', 'err');
        return runAction(ids, 'updates', 'check', {}, { open: false });
      },
    },
  };
}

// ---------- Agent page ----------
function AgentPage([id]) {
  const st = { tab: 'updates', sel: new Set(), data: null };
  const upd = () => st.data?.agent?.state?.updates || null;
  function header(a, groups) {
    const f = a.facts;
    const g = groups.find((x) => x.id === a.group_id);
    return html`
      <div class="page-head">
        <div><div class="row"><a href="#/" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(a)}<h1>${a.name}</h1>
          ${a.status === 'paused' ? html`<span class="badge info">paused</span>` : ''}${a.status === 'pending' ? html`<span class="badge upd">pending approval</span>` : ''}
          <span class="tag">${icon('folder')} ${g ? g.name : 'Ungrouped'}</span>${verPill(a)}</div>
          <div class="sub">${osLabel(f)} · ${f.kernel || ''} · ${f.arch} · ${f.virt} · ${a.online ? 'online, up ' + dur(f.uptime_sec) : raw('offline, last seen ' + r(when(a.last_seen)))}</div></div>
        <span class="spacer"></span>
        ${a.busy ? html`<span class="row muted"><span class="spin"></span>${a.busy} job(s) active</span>` : ''}
        ${a.status !== 'pending' ? html`<button class="btn warn sm" data-act="reboot" ${a.status === 'active' && a.online && canReboot(a) ? '' : 'disabled'}
          title="${canReboot(a) ? (a.online ? 'Reboot this system' : 'The system is offline') : `Agent ${a.core_version} cannot reboot; update the agent first`}">${icon('refresh')} Reboot</button>` : ''}
        ${a.status === 'active' ? html`<button class="btn sm" data-act="pause">${icon('pause')} Pause</button>` : ''}
        ${a.status === 'paused' ? html`<button class="btn sm" data-act="resume">${icon('play')} Resume</button>` : ''}
      </div>
      ${a.status === 'pending' ? html`<div class="notice blue row">This system enrolled ${when(a.enrolled_at)} and is waiting for approval. Until approved it receives no commands.<span class="spacer"></span><button class="btn primary sm" data-act="approve">${icon('check')} Approve</button></div>` : ''}
      ${a.removing ? html`<div class="notice red">Uninstall requested — the agent will remove itself.</div>` : ''}`;
  }
  function updatesTab(a) {
    if (!hasUpdates(a)) {
      return html`<div class="panel empty"><div class="big">📦</div><div>Agent ${a.core_version || '?'} has no built-in update function.</div>
        ${a.core_latest ? html`<div style="margin-top:14px"><button class="btn primary" data-act="agent-version" data-v="${a.core_latest}" ${a.status === 'active' ? '' : 'disabled'}>${icon('up')} Update agent to ${a.core_latest}</button></div>` : ''}</div>`;
    }
    const s = upd();
    const canAct = a.status === 'active' && a.online;
    if (!s) {
      return html`<div class="panel empty"><div class="spin" style="width:26px;height:26px"></div><div style="margin-top:12px">Waiting for the first check…</div>
        <div style="margin-top:14px"><button class="btn" data-act="check" ${canAct ? '' : 'disabled'}>${icon('refresh')} Check now</button></div></div>`;
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
          <button class="btn sm" data-act="check" ${canAct ? '' : 'disabled'}>${icon('refresh')} Refresh &amp; check</button>
          <button class="btn primary sm" data-act="install" ${canAct && nsel ? '' : 'disabled'}>${icon('download')} Install selected${nsel ? ` (${nsel})` : ''}</button>
          <button class="btn warn sm" data-act="upgrade-all" ${canAct && s.total ? '' : 'disabled'}>${icon('zap')} Upgrade all</button>
        </div>
      </div>
      ${s.removals?.length ? html`<div class="notice red">“Upgrade all” would remove: <b>${s.removals.join(', ')}</b></div>` : ''}
      ${pk.length ? html`
      <div class="panel">
        <div class="row panel-pad" style="padding-bottom:10px">
          <span class="muted small">Quick select:</span>
          <button class="btn ghost sm" data-act="sel-sec">Security only</button>
          <button class="btn ghost sm" data-act="sel-all">All</button>
          <button class="btn ghost sm" data-act="sel-none">None</button>
        </div>
        <div class="tbl-wrap" data-keep-scroll="pk"><table class="tbl">
          <thead><tr><th class="w1"><input type="checkbox" data-act="sel-toggle-all" ${allOn ? 'checked' : ''}></th><th>Package</th><th>Version</th><th>Origin</th><th class="w1"></th></tr></thead>
          <tbody>${pk.map((p) => html`
            <tr class="${st.sel.has(p.name) ? 'picked' : ''}">
              <td class="w1">${p.new ? '' : html`<input type="checkbox" data-act="sel-pkg" data-name="${p.name}" ${st.sel.has(p.name) ? 'checked' : ''}>`}</td>
              <td><b>${p.name}</b> <span class="dim small">${p.arch}</span></td>
              <td class="mono">${p.current ? html`<span class="ver">${p.current}</span><span class="arrow">→</span>` : ''}<span class="ver new">${p.available}</span></td>
              <td class="muted small">${p.origin}</td>
              <td class="w1">${p.security ? html`<span class="badge sec">security</span>` : ''}${p.new ? html`<span class="badge new" title="Pulled in as a dependency; installed together with its parent">new dependency</span>` : ''}</td>
            </tr>`)}</tbody></table></div>
      </div>` : html`<div class="panel empty"><div class="big">✨</div>Everything is up to date.</div>`}`;
  }
  function agentTab(a) {
    const repo = st.data.repo;
    const active = a.status === 'active';
    const avail = (repo.core || []).filter((v) => v.platforms.includes(`${a.facts.os}-${a.facts.arch}`));
    return html`
      <div class="panel panel-pad">
        <h2>Agent version</h2>
        <div class="row">${verPill(a)}${a.core_latest ? html`<span class="muted small">latest available: <span class="ver">${a.core_latest}</span></span>` : ''}</div>
        ${a.desired_core ? html`<div class="notice blue" style="margin:14px 0 0">Change to ${a.desired_core} requested${a.online ? ' — the agent downloads the signed binary and restarts.' : ' — applies as soon as the system is online.'}</div>` : ''}
        ${avail.length ? html`<form class="row" style="margin-top:16px" data-submit="agent-set">
            <label class="field" style="min-width:240px">Install agent version<select name="v">${versionOptions({ core: avail }, a.core_version)}</select></label>
            <button class="btn primary" style="align-self:flex-end" ${active ? '' : 'disabled'}>${icon('up')} Apply</button>
          </form>
          <div class="dim small" style="margin-top:10px">Up- and downgrades both work. The master keeps the last ${avail.length === 1 ? 'version' : avail.length + ' versions'} ready; older ones are removed. Every binary is signed and verified by the agent before it replaces itself.</div>`
        : html`<div class="muted" style="margin-top:12px">No agent versions for ${a.facts.os}-${a.facts.arch} on the master yet — see <a href="#/releases">Releases</a>.</div>`}
      </div>`;
  }
  function jobsTab() {
    const jobs = st.data.jobs;
    if (!jobs.length) return html`<div class="panel empty">No jobs yet.</div>`;
    return html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
      <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}</div></td><td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td><td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>`;
  }
  function systemTab(a) {
    const f = a.facts;
    const kv = [
      ['Hostname', f.hostname], ['Operating system', osLabel(f)], ['Kernel', f.kernel], ['Architecture', `${f.os}/${f.arch}`],
      ['Virtualization', f.virt], ['Uptime', a.online ? dur(f.uptime_sec) : 'offline'], ['CPUs', f.cpus], ['Memory (assigned)', bytes(f.mem_total)],
      ['IP addresses', (f.ips || []).join(', ')], ['Agent ID', a.id], ['Agent version', a.core_version], ['Enrolled', fmtDate(a.enrolled_at)], ['Last seen', fmtDate(a.last_seen, true)],
    ];
    return html`<div class="panel panel-pad"><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v ?? '—'}</div></div>`)}</div></div>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const a = d.agent;
    setHTML($('#a-head'), header(a, d.groups));
    const tabs = [['updates', 'Updates'], ['agent', 'Agent'], ['jobs', 'Jobs'], ['system', 'System']];
    setHTML($('#a-tabs'), tabs.map(([k, l]) => html`<button class="${st.tab === k ? 'on' : ''}" data-act="tab" data-t="${k}">${l}${k === 'updates' && a.updates?.total ? html` <span class="badge upd">${a.updates.total}</span>` : ''}${k === 'agent' && !a.core_is_latest && a.core_latest ? html` <span class="badge upd">↑</span>` : ''}</button>`));
    const body = { updates: updatesTab, agent: agentTab, jobs: jobsTab, system: systemTab }[st.tab](a);
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
      tab: (t) => { st.tab = t.dataset.t; render(); },
      check: () => runAction([id], 'updates', 'check', {}),
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
      pause: async () => { await POST('agents/' + id, { paused: true }); toast('Paused — no jobs will be sent', 'ok'); refresh(); },
      resume: async () => { await POST('agents/' + id, { paused: false }); toast('Resumed', 'ok'); refresh(); },
      'agent-version': (t) => setAgentVersion(a(), t.dataset.v),
      'agent-set': async (f) => {
        const v = f.v.value, cur = a().core_version;
        if (v === cur) return toast(`Agent ${v} is already installed`);
        const down = versionLess(v, cur);
        if (!(await confirmBox(`${down ? 'Downgrade' : 'Upgrade'} the agent on ${a().name}?`, `${cur} → ${v}. The agent downloads the signed binary, verifies it and restarts.`, { okText: down ? 'Downgrade' : 'Upgrade' }))) return;
        await setAgentVersion(a(), v);
      },
      reboot: async () => {
        const ag = a();
        const body = html`The system restarts within a few seconds and shows offline until it is back.
          ${ag.is_master ? html`<div class="notice red" style="margin:14px 0 0"><b>This is the system the Coremon master runs on.</b> The website is unavailable for a minute or two until it has restarted; this page reconnects by itself.</div>` : ''}`;
        if (!(await confirmBox(`Reboot ${ag.name} now?`, body, { okText: 'Reboot', danger: true }))) return;
        await runAction([id], 'system', 'reboot', {}, { open: false });
      },
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

// ---------- Systems (enrollment, groups, management) ----------
function Systems() {
  const st = { data: null, tokens: [], lastCmd: null };
  function render() {
    const d = st.data;
    if (!d) return;
    const pend = d.agents.filter((a) => a.status === 'pending').sort(byName);
    setPending(pend.length);
    setHTML($('#s-pending'), pend.length ? html`<div class="panel panel-pad"><h2>Waiting for approval <span class="badge upd">${pend.length}</span></h2>
      <table class="tbl"><tbody>${pend.map((a) => html`<tr><td>${agentDot(a)} <b>${a.name}</b></td><td class="muted">${osLabel(a.facts)} · ${a.facts.arch}</td><td class="muted">${(a.facts.ips || []).join(', ')}</td><td class="muted">${when(a.enrolled_at)}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button><button class="btn danger sm" data-act="reject" data-id="${a.id}">Reject</button></div></td></tr>`)}</tbody></table></div>` : '');
    setHTML($('#s-tokens'), st.tokens.length ? html`<table class="tbl" style="margin-top:14px"><thead><tr><th>Open token</th><th>Note</th><th>Expires</th><th class="w1"></th></tr></thead>
      <tbody>${st.tokens.map((t) => html`<tr><td class="mono">${t.prefix}…</td><td class="muted">${t.note || '—'}</td><td class="muted">${fmtDate(t.expires)}</td><td class="w1"><button class="btn ghost sm" data-act="revoke" data-hash="${t.hash}">Revoke</button></td></tr>`)}</tbody></table>` : '');
    const count = (gid) => d.agents.filter((a) => (a.group_id || '') === gid).length;
    setHTML($('#s-groups'), html`
      ${d.groups.length ? html`<table class="tbl" style="margin-top:12px"><tbody>${d.groups.map((g, i) => html`<tr>
        <td class="w1"><div class="row" style="flex-wrap:nowrap;gap:2px"><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="-1" ${i === 0 ? 'disabled' : ''} title="Move up">${icon('up')}</button><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="1" ${i === d.groups.length - 1 ? 'disabled' : ''} title="Move down">${icon('down')}</button></div></td>
        <td><b>${g.name}</b></td><td class="muted">${count(g.id)} system(s)</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn ghost sm" data-act="g-rename" data-gid="${g.id}">${icon('edit')} Rename</button><button class="btn danger sm" data-act="g-delete" data-gid="${g.id}">${icon('trash')}</button></div></td></tr>`)}
        <tr><td></td><td class="dim">Ungrouped</td><td class="muted">${count('')} system(s)</td><td class="dim small w1">always last</td></tr></tbody></table>`
        : html`<div class="muted small" style="margin-top:10px">No groups yet. All systems are shown together.</div>`}`);
    const act = d.agents.filter((a) => a.status !== 'pending').sort(byName);
    setHTML($('#s-all'), act.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>System</th><th>IP address</th><th>Group</th><th>OS</th><th>Agent</th><th>Last seen</th><th class="w1"></th></tr></thead>
      <tbody>${act.map((a) => html`<tr><td>${agentDot(a)} <a href="#/agent/${a.id}"><b>${a.name}</b></a> ${a.status === 'paused' ? html`<span class="badge info">paused</span>` : ''}</td>
        <td class="mono small">${ipLabel(a.facts)}</td>
        <td><select class="sm" data-change="set-group" data-id="${a.id}" title="Group">${groupOptions(d.groups, a.group_id)}</select></td>
        <td class="muted">${osLabel(a.facts)}</td><td>${verPill(a)}</td>
        <td class="muted">${a.online ? 'online' : when(a.last_seen)}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn ghost sm" data-act="rename" data-id="${a.id}">${icon('edit')} Rename</button><button class="btn danger sm" data-act="remove" data-id="${a.id}" title="Remove">${icon('trash')}</button></div></td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">No approved systems yet.</div>`);
  }
  const groupName = (gid) => st.data.groups.find((g) => g.id === gid)?.name || '';
  const agentBy = (id) => st.data.agents.find((a) => a.id === id);
  return {
    interval: 4000,
    mount(el) {
      el.innerHTML = r(html`${settingsTabs('systems')}
        <div class="page-head"><div><h1>Systems</h1><div class="sub">Enroll, approve, name, group and remove systems</div></div></div>
        <div class="stack">
          <div class="panel panel-pad">
            <h2>Add a system</h2>
            <div class="muted small">Generates a single-use install command. Run it as <b>root</b> on the system (VM, container, bare metal — including this master container). The agent connects out to the master; no SSH or open ports needed.</div>
            <form class="row" style="margin-top:14px" data-submit="gen">
              <input type="text" name="note" placeholder="Note (optional), e.g. web-01" maxlength="64" style="width:240px">
              <select name="ttl"><option value="1">valid 1 hour</option><option value="24" selected>valid 24 hours</option><option value="168">valid 7 days</option></select>
              <button class="btn primary">${icon('plus')} Generate install command</button>
            </form>
            <div id="s-cmd"></div>
            <div id="s-tokens"></div>
          </div>
          <div id="s-pending"></div>
          <div class="panel panel-pad">
            <h2>Groups</h2>
            <div class="muted small">Each system belongs to at most one group. On the dashboard, groups stay together and can be folded; drag systems and groups there to arrange them.</div>
            <form class="row" style="margin-top:12px" data-submit="g-create">
              <input type="text" name="name" placeholder="New group name, e.g. Production" maxlength="48" style="width:260px" required>
              <button class="btn">${icon('plus')} Create group</button>
            </form>
            <div id="s-groups"></div>
          </div>
          <div><h2 style="font-size:15px;margin:6px 0 12px">All systems</h2><div id="s-all"></div></div>
        </div>`);
    },
    async refresh() {
      [st.data, st.tokens] = await Promise.all([GET('overview'), GET('tokens')]);
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
        if (await confirmBox('Reject this system?', 'It is removed from the master. Its agent will be refused from now on.', { okText: 'Reject', danger: true })) {
          await POST(`agents/${t.dataset.id}/remove`, { uninstall: false }); view.refresh();
        }
      },
      'set-group': async (t) => {
        await POST('agents/' + t.dataset.id, { group_id: t.value });
        toast(`${agentBy(t.dataset.id).name} → ${t.value ? groupName(t.value) : 'Ungrouped'}`, 'ok');
        view.refresh();
      },
      rename: (t) => {
        const ag = agentBy(t.dataset.id);
        openModal(html`<h3>Rename ${ag.name}</h3><form class="form" data-submit="save">
          <label class="field">Display name<input type="text" name="name" value="${ag.name}" maxlength="64" autofocus required></label>
          <div class="dim small">Host name on the system: ${ag.facts.hostname || '—'}. Only the name shown in Coremon changes.</div>
          <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div></form>`, {
          acts: {
            cancel: () => closeModal(),
            save: async (f) => { await POST('agents/' + ag.id, { name: f.name.value }); closeModal(); toast('Renamed', 'ok'); view.refresh(); },
          },
        });
      },
      remove: (t) => {
        const ag = agentBy(t.dataset.id);
        openModal(html`<h3>Remove ${ag.name}?</h3>
          <div class="muted" style="margin-top:8px">Choose how to remove this system from Coremon.</div>
          <div class="stack" style="margin-top:16px;gap:10px">
            <button class="btn danger" data-act="uninstall" ${ag.online ? '' : 'disabled'}>${icon('trash')} Uninstall the agent from the system and remove it</button>
            <button class="btn" data-act="forget">Only forget it here (the agent stays installed and is refused from now on)</button>
          </div>
          ${ag.online ? '' : html`<div class="dim small" style="margin-top:8px">Uninstalling needs the system to be online.</div>`}
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
        if (!(await confirmBox(`Delete group ${groupName(gid)}?`, n ? `Its ${n} system(s) move to Ungrouped. Nothing else changes.` : 'The group is empty.', { okText: 'Delete group', danger: true }))) return;
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

// ---------- Jobs ----------
function Jobs() {
  const st = { active: false, data: null };
  return {
    interval: 3000,
    mount(el) {
      el.innerHTML = r(html`<div class="page-head"><div><h1>Jobs</h1><div class="sub">Everything sent to agents</div></div><span class="spacer"></span>
        <div class="chips"><button class="chip on" data-act="f" data-v="0">All</button><button class="chip" data-act="f" data-v="1">Active</button></div></div><div id="j-list"></div>`);
    },
    async refresh() {
      st.data = await GET('jobs?limit=200' + (st.active ? '&active=1' : ''));
      const { jobs, names } = st.data;
      setHTML($('#j-list'), jobs.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>System</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
        <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}${j.error ? html` <span style="color:var(--red)">${j.error}</span>` : ''}</div></td>
          <td>${names[j.agent_id] ? html`<a href="#/agent/${j.agent_id}">${names[j.agent_id]}</a>` : html`<span class="dim">${j.agent_id}</span>`}</td>
          <td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td>
          <td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">No jobs.</div>`);
    },
    acts: {
      f: (t) => { st.active = t.dataset.v === '1'; document.querySelectorAll('.chips .chip').forEach((c) => c.classList.toggle('on', c === t)); view.refresh(); },
    },
  };
}

// ---------- Releases ----------
function Releases() {
  const st = { rel: null, data: null, checking: false };
  function render() {
    const R = st.rel, d = st.data;
    if (!R || !d) return;
    const ix = R.index, m = R.master, repo = R.repo;
    const published = new Map((ix.agents || []).map((v) => [v.version, v.published]));
    const agents = d.agents.filter((a) => a.status !== 'pending');
    setHTML($('#r-index'), html`<div class="panel panel-pad">
      <div class="row"><h2 style="margin:0">Version index</h2><span class="spacer"></span>
        ${ix.syncing || st.checking ? html`<span class="row muted small"><span class="spin"></span>checking…</span>` : html`<span class="muted small">${ix.checked_at ? when(ix.checked_at, 'checked ') : 'not checked yet'} · every 6 hours</span>`}
        <button class="btn sm" data-act="check" ${ix.syncing || st.checking ? 'disabled' : ''}>${icon('refresh')} Check now</button></div>
      <div class="dim small mono" style="margin-top:6px;word-break:break-all">${ix.url || 'no GitHub repository configured — see Settings'}</div>
      ${ix.error ? html`<div class="notice red" style="margin:12px 0 0">${ix.error}</div>` : ''}
      ${repo.errors.length ? html`<div class="notice red" style="margin:12px 0 0"><b>Rejected files</b>${repo.errors.map((e) => html`<div class="mono small">${e}</div>`)}</div>` : ''}
      ${!repo.signing_key ? html`<div class="notice red" style="margin:12px 0 0">The master has no release signing key (/etc/coremon-master/signing.pub). Re-run the installer.</div>` : ''}
      <div class="dim small" style="margin-top:10px">The master downloads only this one signed file, verifies it, then downloads and verifies the listed binaries. Nothing is rolled out to systems until you choose to.</div>
    </div>`);
    setHTML($('#r-master'), html`<div class="panel panel-pad"><h2>Master (this website)</h2>
      <div class="row"><span class="muted">Installed</span><span class="ver">${m.version}</span>
        ${m.update ? html`<span class="badge upd">${m.latest} available</span><button class="btn primary sm" data-act="master-update">${icon('up')} Update master to ${m.latest}</button>` : html`<span class="badge ok">${icon('check')} latest</span>`}</div>
      <div class="dim small" style="margin-top:10px">The master only ever moves forward; it is never downgraded.</div></div>`);
    const onVer = (v) => agents.filter((a) => a.core_version === v).length;
    setHTML($('#r-agents'), html`<div class="panel panel-pad"><h2>Agent versions <span class="muted small">— last ${repo.core.length || 0} kept on the master</span></h2>
      ${repo.core.length ? html`<table class="tbl"><thead><tr><th>Version</th><th>Published</th><th>Platforms</th><th>Systems</th></tr></thead>
        <tbody>${repo.core.map((v, i) => html`<tr><td><span class="verpill ${i === 0 ? 'ok' : 'old'}">${v.version}</span> ${i === 0 ? html`<span class="badge ok">latest</span>` : ''}</td>
          <td class="muted">${fmtDate(published.get(v.version))}</td><td>${v.platforms.map((p) => html`<span class="tag mono">${p}</span> `)}</td><td class="muted">${onVer(v.version)}</td></tr>`)}</tbody></table>`
        : html`<div class="muted">No agent versions downloaded yet.</div>`}</div>`);
    const outdated = agents.filter((a) => !a.core_is_latest && repo.core_latest);
    setHTML($('#r-rollout'), agents.length ? html`
      <div class="row" style="margin:6px 0 12px"><h2 style="font-size:15px;margin:0">Agents per system</h2><span class="spacer"></span>
        ${outdated.length ? html`<button class="btn sm" data-act="all-latest">${icon('up')} Update ${outdated.length} outdated agent(s) to ${repo.core_latest}</button>` : ''}</div>
      <div class="panel"><table class="tbl"><thead><tr><th>System</th><th>Agent</th><th>Change to</th></tr></thead>
      <tbody>${agents.map((a) => html`<tr><td>${agentDot(a)} <a href="#/agent/${a.id}"><b>${a.name}</b></a></td><td>${verPill(a)}</td>
        <td><form class="row" style="flex-wrap:nowrap" data-submit="set" data-id="${a.id}"><select name="v">${versionOptions({ core: repo.core.filter((v) => v.platforms.includes(`${a.facts.os}-${a.facts.arch}`)) }, a.core_version)}</select><button class="btn sm" ${a.status === 'active' ? '' : 'disabled'}>Apply</button></form></td></tr>`)}</tbody></table></div>` : '');
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`<div class="page-head"><div><h1>Releases</h1><div class="sub">Master and agent versions, each on their own release track</div></div></div>
        <div class="stack"><div id="r-index"></div><div class="grid2"><div id="r-master"></div><div id="r-agents"></div></div><div id="r-rollout"></div></div>`);
    },
    async refresh() { [st.rel, st.data] = await Promise.all([GET('releases'), GET('overview')]); render(); },
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
      set: async (f) => { const a = st.data.agents.find((x) => x.id === f.dataset.id); await setAgentVersion(a, f.v.value); },
      'all-latest': async () => {
        const l = st.rel.repo.core_latest;
        const list = st.data.agents.filter((a) => a.status === 'active' && !a.core_is_latest);
        if (!(await confirmBox(`Update ${list.length} agent(s) to ${l}?`, list.map((a) => `${a.name} (${a.core_version})`).join(', '), { okText: 'Update agents' }))) return;
        for (const a of list) await POST(`agents/${a.id}/core`, { version: l });
        toast('Requested', 'ok'); view.refresh();
      },
    },
  };
}

// ---------- Audit ----------
function Audit() {
  const st = { agent: '', names: {}, agents: [] };
  return {
    interval: 10000,
    mount(el) {
      el.innerHTML = r(html`<div class="page-head"><div><h1>Audit log</h1><div class="sub">Who did what, and when</div></div><span class="spacer"></span>
        <label class="row small muted">System <select id="au-sys"><option value="">All systems</option></select></label></div><div id="au"></div>`);
      $('#au-sys').addEventListener('change', (e) => { st.agent = e.target.value; view.refresh(); });
    },
    async refresh() {
      const d = await GET('audit' + (st.agent ? '?agent=' + encodeURIComponent(st.agent) : ''));
      st.names = d.names;
      const opts = Object.entries(d.names).sort((a, b) => a[1].localeCompare(b[1]));
      setHTML($('#au-sys'), html`<option value="">All systems</option>${opts.map(([id, n]) => html`<option value="${id}" ${st.agent === id ? 'selected' : ''}>${n}</option>`)}`);
      const rows = d.entries;
      const sys = (e) => e.agent ? (d.names[e.agent] || e.target || e.agent) : (d.names[e.target] || '');
      setHTML($('#au'), rows.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>System</th><th>Target</th><th>Detail</th><th>IP</th></tr></thead>
        <tbody>${rows.map((e) => html`<tr><td class="muted mono small" style="white-space:nowrap">${fmtDate(e.time, true)}</td><td>${e.actor}</td><td><span class="tag mono">${e.action}</span></td>
          <td>${sys(e)}</td><td class="mono small">${e.agent ? '' : (d.names[e.target] ? '' : e.target || '')}</td><td class="muted small">${e.detail || ''}</td><td class="dim small">${e.ip || ''}</td></tr>`)}</tbody></table></div>`
        : html`<div class="panel empty">${st.agent ? 'No entries for this system.' : 'No entries yet.'}</div>`);
    },
  };
}

// ---------- Settings: General, (Systems,) Login, Identity ----------
function settingsHead(sub, title, desc) {
  return html`${settingsTabs(sub)}<div class="page-head"><div><h1>${title}</h1><div class="sub">${desc}</div></div></div>
    ${session.initial_password && sub !== 'login' ? html`<div class="notice">You are still using the generated initial password — <a href="#/settings/login">change it under Login</a>.</div>` : ''}`;
}

function SettingsGeneral() {
  const st = { saved: null, next: 0, timer: null };
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
  return {
    async mount(el) {
      const d = await GET('settings');
      const s = d.settings, L = d.limits;
      st.saved = s.retention_days;
      el.innerHTML = r(html`${settingsHead('general', 'General', 'How the master checks, cleans up and is reached')}
        <div class="panel panel-pad" style="max-width:720px">
          <form class="form" data-submit="save" style="margin-top:0">
            <label class="field">Automatic update check interval (hours)<input type="number" name="interval" min="1" max="168" value="${s.check_interval_hours}"></label>
            <label class="field">Heartbeat (seconds)<input type="number" name="hb" min="${L.heartbeat_min}" max="${L.heartbeat_max}" value="${s.heartbeat_sec}"></label>
            <div class="dim small" id="hb-hint"></div>
            <label class="field">Keep audit log and job history<span class="range"><input type="range" name="ret" min="${L.retention_min}" max="${L.retention_max}" step="1" value="${s.retention_days}"><b id="ret-val"></b></span></label>
            <div class="dim small" id="ret-info"></div>
            <label class="field">Website port<select name="port">
              <option value="8443" ${s.ui_port !== 443 ? 'selected' : ''}>8443 (default)</option>
              <option value="443" ${s.ui_port === 443 ? 'selected' : ''}>443 (https://host without a port number)</option></select></label>
            <div class="dim small">Agents always connect on port ${d.agent_port}; this only changes where the website is offered. With 443 the website is also still reachable on ${d.agent_port}.</div>
            ${d.ui_port_error ? html`<div class="notice red" style="margin:0">${d.ui_port_error}</div>` : ''}
            <label class="field">Master URL used in install commands<input type="text" name="url" placeholder="${d.suggested_url} (auto)" value="${s.master_url}"></label>
            <div class="dim small">Set this if agents reach the master under a different name than your browser does.</div>
            <label class="field">GitHub repository for releases<input type="text" name="gh" placeholder="owner/name" value="${s.github_repo}"></label>
            <div class="dim small mono" style="word-break:break-all">${d.index_url ? 'Version index: ' + d.index_url : ''}</div>
            <div><button class="btn primary">Save</button></div>
          </form>
        </div>`);
      const f = $('form[data-submit=save]', el);
      const upd = () => {
        const days = +f.ret.value;
        $('#ret-val').textContent = `${days} day${days === 1 ? '' : 's'}`;
        const hb = +f.hb.value || 0;
        $('#hb-hint').textContent = `Agents contact the master at least every ${hb}s. A system is shown offline after ${hb * 3}s without contact. Commands are delivered instantly regardless.`;
      };
      f.ret.addEventListener('input', () => { upd(); clearTimeout(st.timer); st.timer = setTimeout(() => preview(f), 250); });
      f.hb.addEventListener('input', upd);
      upd();
      preview(f);
    },
    unmount() { clearTimeout(st.timer); },
    acts: {
      save: async (f) => {
        const res = await POST('settings', { check_interval_hours: +f.interval.value, heartbeat_sec: +f.hb.value, retention_days: +f.ret.value,
          ui_port: +f.port.value, master_url: f.url.value, github_repo: f.gh.value });
        st.saved = res.settings.retention_days;
        if (res.ui_port_error) toast(res.ui_port_error, 'err');
        else toast('Saved', 'ok');
        preview(f);
      },
    },
  };
}

function SettingsLogin() {
  return {
    mount(el) {
      const min = session.min_password || 5;
      el.innerHTML = r(html`${settingsHead('login', 'Login', 'The admin password for this website')}
        ${session.initial_password ? html`<div class="notice">You are still using the generated initial password — please change it.</div>` : ''}
        <div class="panel panel-pad" style="max-width:480px"><h2>Change password</h2>
          <form class="form" data-submit="pw" style="margin-top:0">
            <label class="field">Current password<input type="password" name="cur" autocomplete="current-password" required></label>
            <label class="field">New password (min. ${min} characters)<input type="password" name="new" autocomplete="new-password" minlength="${min}" required></label>
            <label class="field">Repeat new password<input type="password" name="new2" autocomplete="new-password" minlength="${min}" required></label>
            <div><button class="btn primary">Change password</button></div>
          </form>
        </div>`);
    },
    acts: {
      pw: async (f) => {
        if (f.new.value !== f.new2.value) return toast('New passwords do not match', 'err');
        await POST('password', { current: f.cur.value, new: f.new.value });
        session.initial_password = false;
        f.reset();
        toast('Password changed', 'ok');
        route();
      },
    },
  };
}

function SettingsIdentity() {
  return {
    async mount(el) {
      const d = await GET('settings');
      el.innerHTML = r(html`${settingsHead('identity', 'Identity', 'How agents recognise this master')}
        <div class="panel panel-pad" style="max-width:720px"><h2>Master identity</h2>
          <div class="muted small">Agents pin this public-key fingerprint. If the master certificate is ever regenerated, agents must be reinstalled.</div>
          <div class="cmdbox"><code>sha256//${d.pin}</code></div>
          <div class="dim small" style="margin-top:10px">Coremon v${session.version}${releasedLabel() ? ' · ' + releasedLabel() : ''}</div>
        </div>`);
    },
  };
}

boot().catch(() => showLogin());
