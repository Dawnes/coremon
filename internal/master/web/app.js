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
  pulse: '<path d="M3 12h4l3-8 4 16 3-8h4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.01"/>',
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
// Windows stack: one opened from another window (over: true) lies on top of
// it, and closing it returns to the one below. `modal` is the top window.
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
// closeModal closes the top window only.
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

// ---------- job log modal ----------
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

// ---------- event delegation ----------
let view = null;
const globalActs = {
  'open-job': (t) => openJob(t.dataset.id),
  logout: async () => { await POST('logout').catch(() => {}); await boot().catch(() => showLogin()); },
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
let session = { version: '', min_password: 5, max_password: 40 };
// Password length when setting one (sign-in is not limited).
const pwMin = () => session.min_password || 5, pwMax = () => session.max_password || 40;
const pwRange = () => `${pwMin()}–${pwMax()} characters`;
// Read-only users see everything but get no controls that change things.
// (The master refuses such requests anyway.)
const canChange = () => !!session.can_change;
const ifChange = (x) => (canChange() ? x : '');
let refreshTimer = null;
let pendingCount = 0;

function showLogin() {
  clearInterval(refreshTimer);
  view = null;
  closeAllModals();
  setHTML($('#app'), html`
    <div class="login"><div class="panel">
      <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
      <div class="muted">Fleet update control</div>
      <form data-submit="login" id="login-form">
        <label class="field">User name<input type="text" name="username" autocomplete="username" autocapitalize="none" spellcheck="false" autofocus required></label>
        <label class="field">Password<input type="password" name="password" autocomplete="current-password" required></label>
        <div class="loginerr" id="login-err" role="alert"></div>
        <button class="btn primary" type="submit">Sign in</button>
      </form>
      ${session.initial_setup ? html`<div class="hint">First start? Sign in as <code>admin</code>; the initial password is in <code>/var/lib/coremon-master/initial-password.txt</code> on the master.</div>` : ''}
      ${session.version ? html`<div class="loginver">Coremon v${session.version}</div>` : ''}
    </div></div>`);
  view = {
    acts: {
      login: async (f) => {
        const err = $('#login-err');
        err.textContent = '';
        f.password.classList.remove('bad');
        try {
          await POST('login', { username: f.username.value, password: f.password.value });
          // After signing in always start at the dashboard, whatever
          // address was opened. (Reloading a page while signed in keeps it.)
          if (location.hash && location.hash !== '#/') history.replaceState(null, '', location.pathname + location.search + '#/');
          await boot();
        } catch (e) {
          const m = e.message || '';
          err.textContent = /wrong/i.test(m) ? 'Wrong user name or password' : /too many/i.test(m) ? 'Too many attempts, try again in a few minutes' : m;
          f.password.classList.add('bad');
          f.password.select();
        }
      },
    },
  };
  $('#login-form input[name=username]').focus();
}

const NAV = [
  ['dashboard', 'dash', 'Dashboard'],
  ['releases', 'box', 'Releases'],
  ['history', 'jobs', 'History'],
  ['settings', 'cog', 'Settings'],
  ['about', 'info', 'About'],
];
// Submenus, always shown below their menu item. The first entry opens when
// the menu item itself is clicked.
const SUBNAV = {
  dashboard: () => [['overview', 'Overview'], ['list', 'List']],
  about: () => [['about', 'About'], ['license', 'License']],
  history: () => [['jobs', 'Jobs'], ['audit', 'Audit log']],
  settings: () => [['general', 'General'], ['systems', 'Systems'], ...(canChange() ? [['users', 'Users']] : []), ['login', 'Login'], ['identity', 'Identity']],
};
const OLD_ROUTES = { systems: ['settings', 'systems'], jobs: ['history', 'jobs'], audit: ['history', 'audit'] };
function currentRoute() {
  const p = location.hash.replace(/^#\/?/, '').split('/');
  if (OLD_ROUTES[p[0]]) return OLD_ROUTES[p[0]];
  if (SUBNAV[p[0]]) {
    const items = SUBNAV[p[0]]();
    return [p[0], items.some(([k]) => k === p[1]) ? p[1] : items[0][0]];
  }
  // The start page is Overview; a system page belongs to the submenu it was
  // opened from.
  if (p[0] === '') return ['dashboard', 'overview'];
  if (p[0] === 'system') return ['dashboard', lastDashSub];
  return [p[0], ''];
}
const releasedLabel = () => (session.build_date ? 'Released: ' + session.build_date.split('-').reverse().join('-') : '');
// On narrow screens the submenu is shown as tabs at the top of the page.
const subTabs = (section, cur) => html`<div class="settabs">${SUBNAV[section]().map(([k, l]) => html`<a href="#/${section}/${k}" class="${cur === k ? 'on' : ''}">${l}</a>`)}</div>`;
const settingsTabs = (cur) => subTabs('settings', cur);
const roleLabel = (r) => (r === 'admin' ? 'Administrator' : r === 'viewer' ? 'Read-only' : r);

function renderShell() {
  const [active, sub] = currentRoute();
  setHTML($('#app'), html`
    <div class="shell">
      <aside class="side">
        <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
        <nav class="nav">${NAV.map(([h, ic, lbl]) => html`<a href="#/${h}" class="${active === h ? 'active' : ''}">${icon(ic)}<span class="lbl">${lbl}</span></a>
          ${SUBNAV[h] ? html`<div class="subnav">${SUBNAV[h]().map(([k, l]) => html`<a href="#/${h}/${k}" class="${active === h && sub === k ? 'active' : ''}">${l}${k === 'systems' ? html`<span id="nav-pending"></span>` : ''}</a>`)}</div>` : ''}`)}</nav>
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
  const el = $('#nav-pending');
  if (el) { el.className = n ? 'count' : ''; el.textContent = n ? String(n) : ''; }
}

function route() {
  closeAllModals();
  clearInterval(refreshTimer);
  if (view?.unmount) view.unmount();
  const parts = location.hash.replace(/^#\/?/, '').split('/');
  const [section, sub] = currentRoute();
  const subViews = {
    settings: { general: SettingsGeneral, systems: Systems, users: SettingsUsers, login: SettingsLogin, identity: SettingsIdentity },
    history: { jobs: Jobs, audit: Audit },
    dashboard: { overview: Dashboard, list: SystemList },
    about: { about: AboutPage, license: LicensePage },
  };
  const views = { system: AgentPage, releases: Releases };
  // A page of its own (e.g. a system) wins over the submenu it belongs to.
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
  if (!session.authenticated) return showLogin();
  route();
}

// ---------- shared bits ----------
function agentDot(a) {
  if (a.status === 'pending') return html`<span class="dot pend" title="pending approval"></span>`;
  if (a.status === 'report_only') return html`<span class="dot pause" title="report only${a.online ? '' : ' · offline'}"></span>`;
  return html`<span class="dot ${a.online ? 'on' : 'off'}" title="${a.online ? 'online' : 'offline since ' + fmtDate(a.last_seen)}"></span>`;
}
function osLabel(f) { return f.os_name || [f.os_id, f.os_version].filter(Boolean).join(' ') || 'unknown OS'; }
const hasUpdates = (a) => (a.features || []).includes('updates');
const canReboot = (a) => (a.features || []).includes('system');
// First IPv4 address; the others on hover.
// ---------- health and OS support ----------
const ymd = (s) => (s ? s.split('-').reverse().join('-') : ''); // YYYY-MM-DD -> DD-MM-YYYY
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
// Small badges on a dashboard card: failed services, OS support, automatic updates.
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
  if (a.status === 'report_only') out.push(html`<span class="badge info" title="Only reports: no checks, installs or reboots">${icon('pause')} report only</span>`);
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

// ---------- Dashboard: shared filter (Overview and List) ----------
const FILTER_CHIPS = [['all', 'All'], ['updates', 'Updates'], ['security', 'Security'], ['reboot', 'Reboot'], ['offline', 'Offline'], ['pending', 'Pending'], ['agent', 'Agent outdated']];
function agentMatches(a, f, q, groups) {
  const u = a.updates || {};
  if (f === 'updates' && !(u.total > 0)) return false;
  if (f === 'security' && !(u.security > 0)) return false;
  if (f === 'reboot' && !u.reboot_required) return false;
  if (f === 'offline' && (a.online || a.status === 'pending')) return false;
  if (f === 'pending' && a.status !== 'pending') return false;
  if (f === 'agent' && (a.core_is_latest || !a.core_version)) return false;
  if (q) {
    const g = groups.find((x) => x.id === a.group_id);
    const hay = `${a.name} ${osLabel(a.facts)} ${(a.facts.ips || []).join(' ')} ${g ? g.name : ''} ${a.core_version}`.toLowerCase();
    if (!hay.includes(q.toLowerCase())) return false;
  }
  return true;
}
function filterChips(agents, cur, test) {
  return FILTER_CHIPS.map(([k, l]) => html`<button class="chip ${cur === k ? 'on' : ''}" data-act="filter" data-f="${k}">${l}<span class="n">${agents.filter((a) => test(a, k)).length}</span></button>`);
}
// The dashboard submenu a system page was opened from (Overview or List).
let lastDashSub = 'overview';

// ---------- Dashboard: Overview ----------
function Dashboard() {
  lastDashSub = 'overview';
  const st = { filter: 'all', q: '', sel: new Set(), data: null, folded: new Set(), drag: null };
  const filtering = () => st.filter !== 'all' || st.q !== '';
  const matches = (a) => agentMatches(a, st.filter, st.q, st.data.groups);
  function card(a) {
    const u = a.updates;
    const f = a.facts;
    let accent = 'a-ok';
    if (a.status === 'pending') accent = 'a-pend';
    else if (a.below_min) accent = 'a-upd';
    else if (!a.online) accent = 'a-off';
    else if (u?.security) accent = 'a-sec';
    else if (u?.total) accent = 'a-upd';
    const selectable = a.status === 'active' && !a.below_min && canChange();
    let mid;
    if (a.status === 'pending') {
      mid = html`<div class="mid"><div><div class="biglbl">Waiting for approval</div><div class="dim small">enrolled ${when(a.enrolled_at)}</div></div>
        <div class="badges">${ifChange(html`<button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button>`)}</div></div>`;
    } else if (a.below_min) {
      mid = html`<div class="mid"><div class="big some">↑</div><div class="biglbl">agent update<br>required</div>
        <div class="badges">${ifChange(html`<button class="btn primary sm" data-act="agent-update" data-id="${a.id}" ${a.online ? '' : 'disabled'} title="${a.online ? `Update to agent ${a.core_latest}` : 'The system is offline'}">${icon('up')} Update agent</button>`)}</div></div>`;
    } else if (!hasUpdates(a) && a.online) {
      mid = html`<div class="mid"><div class="big dim">—</div><div class="biglbl">no update function</div><div class="badges"></div></div>`;
    } else if (!u) {
      mid = html`<div class="mid"><div class="big dim">—</div><div class="biglbl">${a.status === 'report_only' ? html`no checks<br>(report only)` : html`waiting for<br>first check`}</div><div class="badges">${updBadges(a)}</div></div>`;
    } else {
      const cls = u.security ? 'sec' : u.total ? 'some' : 'zero';
      mid = html`<div class="mid"><div class="big ${cls}">${u.total}</div><div class="biglbl">${u.total === 1 ? 'update' : 'updates'}<br>available</div><div class="badges">${updBadges(a)}</div></div>`;
    }
    return html`
      <div class="card ${accent} ${st.sel.has(a.id) ? 'sel' : ''}" data-act="open-agent" data-id="${a.id}" ${canChange() ? raw('data-drag="agent" draggable="true"') : ''}>
        <div class="top">${ifChange(html`<span class="grip" title="Drag to reorder or move to another group">${icon('grip')}</span>`)}${agentDot(a)}
          <div style="min-width:0"><div class="name">${a.name}</div>${a.below_min
            ? html`<div class="os">agent ${a.core_version} · lowest allowed ${a.min_agent}</div><div class="os">upgrade only — no data shown</div>`
            : html`<div class="os">${osLabel(f)} · ${f.arch || '?'} · ${f.virt || ''}</div><div class="os">${ipLabel(f)}</div>`}</div>
          ${selectable ? html`<input type="checkbox" class="selbox" data-act="sel" data-id="${a.id}" ${st.sel.has(a.id) ? 'checked' : ''} title="Select">` : ''}</div>
        ${mid}
        ${a.status !== 'pending' ? html`<div class="inforow"><span class="uptime" title="${a.below_min ? '' : lastReboot(a)}">${icon('clock')}${!a.online ? 'offline' : a.below_min ? 'online' : 'up ' + dur(f.uptime_sec)}</span>${healthBadges(a)}<span class="spacer"></span>${verPill(a)}</div>` : ''}
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
      <div class="ghead" data-gid="${g.id}" ${g.id && canChange() ? raw('data-drag="group" draggable="true"') : ''}>
        <button class="btn ghost sm iconbtn fold ${folded ? 'shut' : ''}" data-act="fold" data-gid="${g.id}" title="${folded ? 'Unfold' : 'Fold'}">${icon('chev')}</button>
        ${g.id && canChange() ? html`<span class="grip" title="Drag to reorder groups">${icon('grip')}</span>` : ''}
        <span class="gname">${g.name}</span>
        <span class="gsum">${agents.length} system${agents.length === 1 ? '' : 's'}${upd ? html` · <b class="c-amber">${upd} update${upd === 1 ? '' : 's'}</b>` : ''}${sec ? html` · <b class="c-red">${sec} security</b>` : ''}${off ? html` · <b class="c-red">${off} offline</b>` : ''}</span>
        <span class="spacer"></span>
        ${selectable.length && canChange() ? html`<label class="gsel" title="Select all systems in this group"><input type="checkbox" data-act="sel-group" data-gid="${g.id}" ${allSel ? 'checked' : ''}> select</label>` : ''}
      </div>` : '';
    const body = folded ? '' : list.length
      ? html`<div class="cards">${list.map(card)}</div>`
      : html`<div class="gempty">${g.id ? (canChange() ? 'Empty group — drag systems here' : 'Empty group') : 'No ungrouped systems'}</div>`;
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
    const reportOnly = A.filter((a) => a.status === 'report_only').length;
    setPending(pending);
    // Oldest information: per system the age of what the dashboard shows -
    // last contact when offline, last update check when online. Report-only and
    // pending systems are left out.
    // "Current": online and checked within the check interval + 15 min. An
    // offline system is never current.
    let oldest = null, anyOffline = false, anyUnchecked = false;
    for (const a of A) {
      if (a.status !== 'active' || a.below_min) continue; // upgrade-only systems have no data here
      const ts = a.online ? a.updates?.checked_at : a.last_seen;
      if (!a.online) anyOffline = true;
      if (a.online && !ts && hasUpdates(a)) anyUnchecked = true; // waiting for its first check
      if (!ts) continue;
      if (!oldest || ts < oldest.ts) oldest = { ts, a };
    }
    // An offline system is named even if an online one has older data.
    if (anyOffline && oldest?.a.online) {
      const off = A.filter((a) => a.status === 'active' && !a.below_min && !a.online && a.last_seen).sort((x, y) => x.last_seen - y.last_seen)[0];
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
      <div class="panel stat c-green"><div class="label">Systems online</div><div class="value">${online}<small> / ${active.length}</small></div>
        ${reportOnly ? html`<div class="statnote" title="Report-only systems only report; no checks, installs or reboots">${icon('pause')} ${reportOnly} report only</div>` : ''}</div>
      <div class="panel stat ${oc} ${oldest ? 'click' : ''}" ${oldest ? raw(`data-act="open-agent" data-id="${esc(oldest.a.id)}"`) : ''} title="${oldest ? 'Oldest information shown on this dashboard: ' + fmtDate(oldest.ts, true) : ''}">
        <div class="label">Oldest information</div>
        ${oldest ? html`<div class="value datev">${fmtDate(oldest.ts)}</div><div class="statnote">${ago(oldest.ts)} · ${oldest.a.name} (${oldest.a.online ? 'last update check' : 'offline'})</div>`
          : allCurrent ? html`<div class="value datev">All systems current</div><div class="statnote">all checked within ${hours} h</div>`
          : html`<div class="value">—</div><div class="statnote">nothing to report</div>`}</div>
      <div class="panel stat c-amber"><div class="label">Pending updates</div><div class="value">${totUpd}</div></div>
      <div class="panel stat c-red"><div class="label">Security updates</div><div class="value">${totSec}</div></div>
      <div class="panel stat c-violet"><div class="label">Reboot required</div><div class="value">${reboot}</div></div>
      <div class="panel stat c-cyan"><div class="label">Active jobs</div><div class="value">${d.active_jobs}</div></div>`);
    setHTML($('#d-chips'), filterChips(A, st.filter, (a, f) => agentMatches(a, f, st.q, d.groups)));
    if (!A.length) {
      setHTML($('#d-cards'), html`<div class="panel empty"><div class="big">🛰️</div><div>No systems yet.</div>${ifChange(html`<div style="margin-top:14px"><a class="btn primary" href="#/settings/systems">${icon('plus')} Add your first system</a></div>`)}</div>`);
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
      <button class="btn sm" data-act="bulk-check">${icon('refresh')} Check selected</button>
      <button class="btn warn sm" data-act="bulk-upgrade">${icon('zap')} Upgrade all packages</button>
      <button class="btn ghost sm" data-act="bulk-clear">Clear</button></div>` : '');
  }

  // ----- drag and drop (systems within/between groups, and groups) -----
  function clearMarks() { document.querySelectorAll('.drop-before,.drop-after,.drop-into').forEach((e) => e.classList.remove('drop-before', 'drop-after', 'drop-into')); }
  function dropTarget(e) {
    if (st.drag.type === 'agent') {
      const c = e.target.closest('.card[data-id]');
      if (c && c.dataset.id === st.drag.id) return null;
      if (c) {
        // Within the same group a drop takes the target's place (so two
        // neighbours swap wherever you drop); from another group, the half
        // of the card decides before or after.
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
        ${subTabs('dashboard', 'overview')}<div class="page-head"><div><h1>Overview</h1><div class="sub">Update status across your fleet</div></div><span class="spacer"></span>
          ${ifChange(html`<button class="btn" data-act="check-all">${icon('refresh')} Check now</button>`)}</div>
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
    // Systems too old to connect (before 0.3.0) are only listed under Settings > Systems.
    async refresh() { const d = await GET('overview'); d.groups ||= []; d.agents = d.agents.filter((a) => !a.legacy); if (!st.drag) { st.data = d; render(); } },
    acts: {
      filter: (t) => { st.filter = t.dataset.f; render(); },
      fold: (t) => { const g = t.dataset.gid; st.folded.has(g) ? st.folded.delete(g) : st.folded.add(g); render(); },
      'open-agent': (t) => { location.hash = '#/system/' + t.dataset.id; },
      sel: (t) => { t.checked ? st.sel.add(t.dataset.id) : st.sel.delete(t.dataset.id); render(); },
      'sel-group': (t) => {
        const gid = t.dataset.gid;
        const ids = st.data.agents.filter((a) => (a.group_id || '') === gid && a.status === 'active' && matches(a)).map((a) => a.id);
        ids.forEach((id) => (t.checked ? st.sel.add(id) : st.sel.delete(id)));
        render();
      },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.groups),
      'bulk-clear': () => { st.sel.clear(); render(); },
      'bulk-check': () => runAction([...st.sel].filter((id) => hasUpdates(st.data.agents.find((a) => a.id === id))), 'updates', 'check', {}, { open: false }),
      'bulk-upgrade': async () => {
        const names = st.data.agents.filter((a) => st.sel.has(a.id)).map((a) => a.name);
        if (await confirmBox('Upgrade all packages?', `This runs a full package upgrade on ${names.length} system(s): ${names.join(', ')}.`, { okText: 'Upgrade all', danger: true })) {
          await runAction([...st.sel], 'updates', 'upgrade_all', {}, { open: false });
          st.sel.clear();
        }
      },
      'check-all': () => {
        const ids = st.data.agents.filter((a) => a.status === 'active' && a.online && hasUpdates(a)).map((a) => a.id);
        if (!ids.length) return toast('No online systems with the update function', 'err');
        return runAction(ids, 'updates', 'check', {}, { open: false });
      },
      'agent-update': async (t) => {
        const a = st.data.agents.find((x) => x.id === t.dataset.id);
        if (!(await confirmBox(`Update the agent on ${a.name}?`, `Agent ${a.core_version} is older than the lowest allowed version ${a.min_agent}. It is updated to ${a.core_latest}; afterwards the system reports normally again.`, { okText: 'Update agent' }))) return;
        const res = await POST(`agents/${a.id}/core`, {});
        if (res.job) openJob(res.job); else toast('Update requested — it runs when the system is online', 'ok');
        view.refresh();
      },
    },
  };
}

// Last reboot: the agent reports its uptime with every heartbeat, so the
// time of that report (last seen) minus the uptime is the boot time.
function lastReboot(a) {
  const up = a.facts?.uptime_sec;
  if (!up || !a.last_seen) return '';
  return `Last reboot: ${fmtDate(a.last_seen - up)}${a.online ? '' : ' (as last reported)'}`;
}
// Cut text to n characters with an ellipsis.
const clipText = (s, n) => (s && s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s);
// List, Health column: ok or "N failed", plus auto; — when not known.
function healthCell(a) {
  const h = a.facts?.health;
  if (!h || h.failed_count < 0) return html`<span class="dim">—</span>`;
  const main = h.failed_count > 0
    ? html`<span class="badge sec xs" title="Failed services: ${(h.failed || []).join(', ')}${h.failed_count > (h.failed || []).length ? ', …' : ''}">${h.failed_count} failed</span>`
    : html`<span class="badge ok xs">ok</span>`;
  return html`${main}${h.auto_updates === 'on' ? html` <span class="badge info xs" title="Automatic updates (unattended-upgrades) are on">auto</span>` : ''}`;
}
// Short OS name for narrow places: "Debian 13", "Ubuntu 22.04".
const osShort = (f) => (f.os_id && f.os_version ? f.os_id.charAt(0).toUpperCase() + f.os_id.slice(1) + ' ' + f.os_version : osLabel(f));

// ---------- Dashboard: List ----------
// Every system on one line, view only. Grouped (groups A-Z, then Ungrouped),
// systems A-Z; a column header sorts within the groups, again reverses.
function SystemList() {
  lastDashSub = 'list';
  const st = { filter: 'all', q: '', data: null, sort: 'name', dir: 1 };
  const byText = (x, y) => String(x).localeCompare(String(y), undefined, { numeric: true, sensitivity: 'base' });
  const statusText = (a) => (a.status === 'pending' ? 'pending' : a.below_min ? 'update required' : !a.online ? 'offline' : a.status === 'report_only' ? 'report only' : 'online');
  const lastTs = (a) => (a.online ? a.updates?.checked_at || 0 : a.last_seen || 0);
  const healthN = (a) => (a.facts?.health?.failed_count > 0 ? 2 : 0) + (a.facts?.health?.auto_updates === 'on' ? 1 : 0);
  const COLS = [
    // key, title, sort value, class (opt = hidden on narrow screens)
    ['status', '●', (a) => statusText(a), 'dotcol'],
    ['name', 'Name', (a) => a.name, ''],
    ['group', 'Group', (a) => groupName(a.group_id), 'opt'],
    ['ip', 'IP', (a) => (a.facts.ips || [])[0] || '', 'opt'],
    ['os', 'OS', (a) => osShort(a.facts), 'opt'],
    ['updates', 'Updates', (a) => a.updates?.total ?? -1, 'num'],
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
    if (a.status === 'pending') return html`<span class="dim">enrolled ${when(a.enrolled_at)}</span>`;
    if (a.below_min) return html`<span class="dim">seen ${when(a.last_seen)}</span>`;
    if (!a.online) return html`<span class="c-red">offline</span><span class="dim">, seen ${when(a.last_seen)}</span>`;
    return a.updates?.checked_at ? when(a.updates.checked_at) : html`<span class="dim">waiting for first check</span>`;
  }
  function row(a) {
    const u = a.updates, o = a.os_support, f = a.facts;
    return html`<tr>
      <td class="nowrap" title="${statusText(a)}${a.below_min ? ` (agent ${a.core_version}, lowest allowed ${a.min_agent})` : ''}">${agentDot(a)}${statusText(a) === 'online' ? '' : html` <span class="small ${a.status === 'pending' || a.status === 'report_only' || a.below_min ? 'c-amber' : 'c-red'}">${statusText(a)}</span>`}</td>
      <td class="nowrap"><a href="#/system/${a.id}"><b>${a.name}</b></a></td>
      <td class="opt muted">${groupName(a.group_id)}</td>
      <td class="opt small nowrap">${(f.ips || [])[0] || ''}</td>
      <td class="opt nowrap" title="${a.below_min ? '' : osLabel(f)}">${a.below_min ? html`<span class="dim">—</span>` : clipText(osShort(f), 14)}${o?.status === 'expired' ? html` <span class="badge sec xs" title="${osSupportText(o, f.os_id)}">EOL</span>` : o?.status === 'soon' ? html` <span class="badge upd xs" title="${osSupportText(o, f.os_id)}">EOL soon</span>` : ''}</td>
      <td class="num"><b class="${u?.security ? 'c-red' : u?.total ? 'c-amber' : 'dim'}">${u ? u.total : '—'}</b></td>
      <td>${!u ? html`<span class="dim">—</span>` : u.reboot_required ? html`<span class="badge reboot xs">required</span>` : html`<span class="badge ok xs">no</span>`}</td>
      <td class="opt mono small"><span class="${!a.core_version ? 'dim' : a.core_is_latest ? 'c-green' : 'c-amber'}" title="${a.core_is_latest ? 'latest agent' : a.core_latest ? 'latest is ' + a.core_latest : ''}">${a.core_version || '—'}</span></td>
      <td class="opt nowrap">${healthCell(a)}</td>
      <td class="small nowrap">${last(a)}</td></tr>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const A = d.agents;
    setHTML($('#l-chips'), filterChips(A, st.filter, (a, f) => agentMatches(a, f, st.q, d.groups)));
    const shown = A.filter((a) => agentMatches(a, st.filter, st.q, d.groups));
    if (!A.length) return setHTML($('#l-table'), html`<div class="panel empty">No systems yet.</div>`);
    if (!shown.length) return setHTML($('#l-table'), html`<div class="panel empty">No systems match this filter.</div>`);
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
      el.innerHTML = r(html`${subTabs('dashboard', 'list')}<div class="page-head"><div><h1>List</h1><div class="sub">All systems at a glance</div></div></div>
        <div class="toolbar"><input type="search" class="search" id="l-q" placeholder="Search name, OS, IP, group…"><div class="chips" id="l-chips"></div></div>
        <div id="l-table"></div>`);
      $('#l-q').addEventListener('input', (e) => { st.q = e.target.value; render(); });
    },
    async refresh() { const d = await GET('overview'); d.groups ||= []; d.agents = d.agents.filter((a) => !a.legacy); st.data = d; render(); },
    acts: {
      filter: (t) => { st.filter = t.dataset.f; render(); },
      sort: (t) => { const k = t.dataset.k; st.dir = st.sort === k ? -st.dir : 1; st.sort = k; render(); },
    },
  };
}

// ---------- About ----------
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
        <div class="stack" style="max-width:860px">
          <div class="notice blue">Coremon is licensed under the <b>PolyForm Noncommercial License 1.0.0</b>: free for personal, hobby,
            educational and non-profit use. Commercial use requires permission from Dawnes, the Developer
            (<a href="https://github.com/Dawnes" target="_blank" rel="noopener noreferrer">github.com/Dawnes</a>).</div>
          <div class="panel panel-pad"><h2>Coremon</h2><div class="legal" id="lic-main">Loading…</div></div>
          <div class="panel panel-pad"><h2>Third-party software</h2><pre class="legal" id="lic-third">Loading…</pre></div>
          <div class="dim small">${COPYRIGHT}</div></div>`);
      const text = (p) => fetch(p, { cache: 'no-cache' }).then((x) => (x.ok ? x.text() : Promise.reject(new Error(x.status)))).catch(() => 'Could not load the license text.');
      const [main, third] = await Promise.all([text('legal/LICENSE.txt'), text('legal/THIRD-PARTY-NOTICES.txt')]);
      // The license is Markdown: headings become headings, link markup is
      // dropped. Built with textContent only, never as HTML.
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

// ---------- Ping ----------
const canPing = (a) => !!a.core_version && !versionLess(a.core_version, '0.3.1');
const shortKey = (p) => (p ? p.slice(0, 5) + '…' : ''); // the master already sends the short form
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
      <div class="small dim" style="margin-top:10px">Check that the system is running, can reach this master on the agent port, and that its clock is right.</div>`;
  }
  openModal(html`<h3>Ping ${a.name}</h3>${body}
    <div class="actions"><button class="btn ghost" data-act="job">Show job</button><button class="btn primary" data-act="close" autofocus>Close</button></div>`,
  { acts: { close: () => closeModal(), job: () => openJob(p.job_id) } });
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
        <div><div class="row"><a href="#/dashboard/${lastDashSub}" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(a)}<h1>${a.name}</h1>
          ${a.status === 'report_only' ? html`<span class="badge info">${icon('pause')} report only</span>` : ''}${a.status === 'pending' ? html`<span class="badge upd">pending approval</span>` : ''}
          <span class="tag">${icon('folder')} ${g ? g.name : 'Ungrouped'}</span>${verPill(a)}</div>
          <div class="sub">${a.below_min ? html`agent ${a.core_version} · upgrade only · ${a.online ? 'online' : raw('offline, last seen ' + r(when(a.last_seen)))}` : html`${osLabel(f)} · ${f.kernel || ''} · ${f.arch} · ${f.virt} · <span title="${lastReboot(a)}">${a.online ? 'online, up ' + dur(f.uptime_sec) : raw('offline, last seen ' + r(when(a.last_seen)))}</span>`}</div></div>
        <span class="spacer"></span>
        ${a.busy ? html`<span class="row muted"><span class="spin"></span>${a.busy} job(s) active</span>` : ''}
        ${!a.removing && !a.below_min ? html`<button class="btn sm" data-act="ping" ${canPing(a) ? '' : 'disabled'}
          title="${canPing(a) ? 'Check the connection and keys with the agent now; nothing is done on the system' : `Agent ${a.core_version || '?'} cannot answer a ping; update the agent first`}">${icon('pulse')} Ping</button>` : ''}
        ${a.status === 'active' && !a.below_min && canChange() ? html`<button class="btn sm" data-act="report-only">${icon('pause')} Report only</button>` : ''}
        ${a.status === 'report_only' && !a.below_min && canChange() ? html`<button class="btn sm" data-act="allow-changes">${icon('play')} Allow changes</button>` : ''}
        ${a.status === 'active' && !a.below_min && canChange() ? html`<button class="btn warn sm" data-act="reboot" ${a.online && canReboot(a) ? '' : 'disabled'}
          title="${canReboot(a) ? (a.online ? 'Reboot this system' : 'The system is offline') : `Agent ${a.core_version} cannot reboot; update the agent first`}">${icon('refresh')} Reboot</button>` : ''}
      </div>
      ${a.below_min ? html`<div class="notice row"><span><b>Agent update required.</b> Agent ${a.core_version} is older than the lowest allowed version ${a.min_agent}:
        the master stores and shows none of this system's data and runs nothing on it until the agent is updated.</span><span class="spacer"></span>
        ${ifChange(html`<button class="btn primary sm" data-act="agent-update" ${a.online ? '' : 'disabled'}>${icon('up')} Update agent to ${a.core_latest}</button>`)}</div>` : ''}
      ${a.status === 'report_only' && !a.below_min ? html`<div class="notice blue"><b>Report only:</b> this system only reports.
        <span class="small">${!a.mode_support ? html`Enforced by the master only — agent ${a.core_version} cannot enforce it itself; update the agent.`
          : a.agent_report_only ? html`${icon('check')} Confirmed by the agent.`
          : html`Waiting for the agent to confirm${a.online ? '…' : ' (the system is offline)'}.`}</span></div>` : ''}
      ${a.status === 'active' && a.mode_support && a.agent_report_only ? html`<div class="notice blue small">Changes allowed — waiting for the agent to confirm${a.online ? '…' : ' (the system is offline)'}.</div>` : ''}
      ${a.status === 'pending' ? html`<div class="notice blue row">This system enrolled ${when(a.enrolled_at)} and is waiting for approval. Until approved it receives no commands.<span class="spacer"></span>${ifChange(html`<button class="btn primary sm" data-act="approve">${icon('check')} Approve</button>`)}</div>` : ''}
      ${a.removing ? html`<div class="notice red">Uninstall requested — the agent will remove itself.</div>` : ''}`;
  }
  function updatesTab(a) {
    if (!hasUpdates(a)) {
      return html`<div class="panel empty"><div class="big">📦</div><div>Agent ${a.core_version || '?'} has no built-in update function.</div>
        <div style="margin-top:14px"><a href="#/releases">Update the agent in Releases</a></div></div>`;
    }
    const s = upd();
    const canAct = a.status === 'active' && a.online;
    // No update buttons for read-only users or report-only systems.
    const ed = canChange() && a.status === 'active';
    if (!s) {
      return html`<div class="panel empty">${a.status === 'report_only' ? html`<div class="big">⏸</div><div>Report only: no update checks are run on this system.</div>`
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
      ${a.status === 'report_only' ? html`<div class="notice blue">Report only: this list is from the last check before the system was set to report only, ${when(s.checked_at)}.</div>` : ''}
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
              <td class="w1">${p.security ? html`<span class="badge sec">security</span>` : ''}${p.new ? html`<span class="badge new" title="Pulled in as a dependency; installed together with its parent">new dependency</span>` : ''}</td>
            </tr>`)}</tbody></table></div>
      </div>` : html`<div class="panel empty"><div class="big">✨</div>Everything is up to date.</div>`}`;
  }
  function agentTab(a) {
    return html`
      <div class="panel panel-pad">
        <h2>Agent version</h2>
        <div class="row">${verPill(a)}${a.core_latest ? html`<span class="muted small">latest available: <span class="ver">${a.core_latest}</span></span>` : ''}</div>
        ${a.desired_core ? html`<div class="notice blue" style="margin:14px 0 0">Change to ${a.desired_core} requested${a.online ? ' — the agent downloads the signed binary and restarts.' : ' — applies as soon as the system is online.'}</div>` : ''}
        <div class="row" style="margin-top:14px"><span class="muted small">Functions:</span>${(a.features || []).map((n) => html`<span class="tag mono">${n}</span>`)}</div>
        <div style="margin-top:14px"><a href="#/releases">${canChange() ? 'Change the agent version in Releases' : 'Agent versions in Releases'}</a></div>
      </div>`;
  }
  function jobsTab() {
    const jobs = st.data.jobs;
    if (!jobs.length) return html`<div class="panel empty">No jobs yet.</div>`;
    return html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
      <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}</div></td><td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td><td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>`;
  }
  function systemTab(a) {
    const f = a.facts, h = f.health;
    const kv = [
      ['Hostname', f.hostname], ['Operating system', osLabel(f)], ['Kernel', f.kernel], ['Architecture', `${f.os}/${f.arch}`],
      ['Virtualization', f.virt], ['Uptime', a.online ? dur(f.uptime_sec) : 'offline'], ['CPUs', f.cpus], ['Memory (assigned)', bytes(f.mem_total)],
      ['IP addresses', (f.ips || []).join(', ')],
      ['IPv4 gateway', f.gateway ? f.gateway + (f.gateway_if ? ` (${f.gateway_if})` : '') : '—'],
      ['DNS servers', f.dns?.length ? f.dns.join(', ') + (f.dns_via ? ` (via ${f.dns_via})` : '') : '—'],
      ['OS support', a.os_support ? html`${a.os_support.status === 'expired' ? html`<span class="badge sec xs">EOL</span> ` : a.os_support.status === 'soon' ? html`<span class="badge upd xs">EOL soon</span> ` : ''}${osSupportText(a.os_support, f.os_id)}` : '—'],
      ['Failed services', !h || h.failed_count < 0 ? '—' : h.failed_count === 0 ? 'none'
        : html`<span class="badge sec xs">${h.failed_count} failed</span> ${(h.failed || []).join(', ')}${h.failed_count > (h.failed || []).length ? ', …' : ''}`],
      ['Automatic updates', ({ on: html`<span class="badge info xs">on</span> unattended-upgrades`, off: 'off', not_installed: 'not installed' })[h?.auto_updates] || '—'],
      ['Last upgrade', h?.last_upgrade ? `${fmtDate(h.last_upgrade.time)} (${ago(h.last_upgrade.time)}, ${h.last_upgrade.packages} package${h.last_upgrade.packages === 1 ? '' : 's'}) · by ${upgradedBy(h.last_upgrade.by)}` : h ? 'none recorded' : '—'],
      ['Agent ID', a.id], ['Agent version', a.core_version], ['Enrolled', fmtDate(a.enrolled_at)], ['Last seen', fmtDate(a.last_seen, true)],
    ];
    return html`<div class="panel panel-pad"><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v ?? '—'}</div></div>`)}</div></div>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const a = d.agent;
    setHTML($('#a-head'), header(a, d.groups));
    // Upgrade only: just the Agent tab (version and update), no data.
    const tabs = a.below_min ? [['agent', 'Agent']] : [['updates', 'Updates'], ['agent', 'Agent'], ['jobs', 'Jobs'], ['system', 'System']];
    if (!tabs.some(([k]) => k === st.tab)) st.tab = tabs[0][0];
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
      'agent-update': async () => {
        const res = await POST(`agents/${id}/core`, {});
        if (res.job) openJob(res.job); else toast('Update requested — it runs when the system is online', 'ok');
        refresh();
      },
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
      'report-only': async () => {
        if (!(await confirmBox(`Set ${a().name} to report only?`, 'The system keeps reporting, but gets no update checks, installs or reboots until you allow changes again. Queued jobs are cancelled; agent version changes stay possible.', { okText: 'Report only' }))) return;
        await POST('agents/' + id, { report_only: true }); toast('Report only', 'ok'); refresh();
      },
      'allow-changes': async () => {
        if (!(await confirmBox(`Allow changes on ${a().name}?`, 'Update checks, installs and reboots become possible again, and the automatic update check resumes.', { okText: 'Allow changes' }))) return;
        await POST('agents/' + id, { report_only: false }); toast('Changes allowed', 'ok'); refresh();
      },
      ping: () => pingModal(a()),
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
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button><button class="btn danger sm" data-act="reject" data-id="${a.id}">Reject</button></div></td>`)}</tr>`)}</tbody></table></div>` : '');
    setHTML($('#s-tokens'), st.tokens.length ? html`<table class="tbl" style="margin-top:14px"><thead><tr><th>Open token</th><th>Note</th><th>Expires</th><th class="w1"></th></tr></thead>
      <tbody>${st.tokens.map((t) => html`<tr><td class="mono">${t.prefix}…</td><td class="muted">${t.note || '—'}</td><td class="muted">${fmtDate(t.expires)}</td><td class="w1"><button class="btn ghost sm" data-act="revoke" data-hash="${t.hash}">Revoke</button></td></tr>`)}</tbody></table>` : '');
    const count = (gid) => d.agents.filter((a) => (a.group_id || '') === gid).length;
    setHTML($('#s-groups'), html`
      ${d.groups.length ? html`<table class="tbl" style="margin-top:12px"><tbody>${d.groups.map((g, i) => html`<tr>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap;gap:2px"><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="-1" ${i === 0 ? 'disabled' : ''} title="Move up">${icon('up')}</button><button class="btn ghost sm iconbtn" data-act="g-move" data-gid="${g.id}" data-dir="1" ${i === d.groups.length - 1 ? 'disabled' : ''} title="Move down">${icon('down')}</button></div></td>`)}
        <td><b>${g.name}</b></td><td class="muted">${count(g.id)} system(s)</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn ghost sm" data-act="g-rename" data-gid="${g.id}">${icon('edit')} Rename</button><button class="btn danger sm" data-act="g-delete" data-gid="${g.id}">${icon('trash')}</button></div></td>`)}</tr>`)}
        <tr>${ifChange(html`<td></td>`)}<td class="dim">Ungrouped</td><td class="muted">${count('')} system(s)</td>${ifChange(html`<td class="dim small w1">always last</td>`)}</tr></tbody></table>`
        : html`<div class="muted small" style="margin-top:10px">No groups yet. All systems are shown together.</div>`}`);
    const act = d.agents.filter((a) => a.status !== 'pending').sort(byName);
    setHTML($('#s-all'), act.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>System</th><th>IP address</th><th>Group</th><th>OS</th><th>Agent</th><th>Last seen</th><th class="w1"></th></tr></thead>
      <tbody>${act.map((a) => html`<tr><td>${agentDot(a)} ${a.legacy ? html`<b>${a.name}</b> <span class="badge sec xs" title="Agents before 0.3.0 can no longer connect and cannot be updated">cannot connect — run a new install command</span>`
        : html`<a href="#/system/${a.id}"><b>${a.name}</b></a>`} ${a.status === 'report_only' && !a.legacy ? html`<span class="badge info">report only</span>` : ''}${a.below_min ? html`<span class="badge upd xs">update required</span>` : ''}</td>
        <td class="mono small">${ipLabel(a.facts)}</td>
        <td>${canChange() ? html`<select class="sm" data-change="set-group" data-id="${a.id}" title="Group">${groupOptions(d.groups, a.group_id)}</select>` : html`<span class="muted">${d.groups.find((g) => g.id === a.group_id)?.name || 'Ungrouped'}</span>`}</td>
        <td class="muted">${osLabel(a.facts)}</td><td>${verPill(a)}</td>
        <td class="muted">${a.online ? 'online' : when(a.last_seen)}</td>
        ${ifChange(html`<td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn ghost sm" data-act="rename" data-id="${a.id}">${icon('edit')} Rename</button><button class="btn danger sm" data-act="remove" data-id="${a.id}" title="Remove">${icon('trash')}</button></div></td>`)}</tr>`)}</tbody></table></div>` : html`<div class="panel empty">No approved systems yet.</div>`);
  }
  const groupName = (gid) => st.data.groups.find((g) => g.id === gid)?.name || '';
  const agentBy = (id) => st.data.agents.find((a) => a.id === id);
  return {
    interval: 4000,
    mount(el) {
      el.innerHTML = r(html`${settingsTabs('systems')}
        <div class="page-head"><div><h1>Systems</h1><div class="sub">Enroll, approve, name, group and remove systems</div></div></div>
        <div class="stack">
          ${ifChange(html`<div class="panel panel-pad">
            <h2>Add a system</h2>
            <div class="muted small">Generates a single-use install command. Run it as <b>root</b> on the system (VM, container, bare metal — including this master container). The agent connects out to the master; no SSH or open ports needed.</div>
            <form class="row" style="margin-top:14px" data-submit="gen">
              <input type="text" name="note" placeholder="Note (optional), e.g. web-01" maxlength="64" style="width:240px">
              <select name="ttl"><option value="1" selected>valid 1 hour</option><option value="3">valid 3 hours</option></select>
              <button class="btn primary">${icon('plus')} Generate install command</button>
            </form>
            <div id="s-cmd"></div>
            <div id="s-tokens"></div>
          </div>`)}
          <div id="s-pending"></div>
          <div class="panel panel-pad">
            <h2>Groups</h2>
            <div class="muted small">Each system belongs to at most one group. On the dashboard, groups stay together and can be folded${canChange() ? '; drag systems and groups there to arrange them' : ''}.</div>
            ${ifChange(html`<form class="row" style="margin-top:12px" data-submit="g-create">
              <input type="text" name="name" placeholder="New group name, e.g. Production" maxlength="48" style="width:260px" required>
              <button class="btn">${icon('plus')} Create group</button>
            </form>`)}
            <div id="s-groups"></div>
          </div>
          <div><h2 style="font-size:15px;margin:6px 0 12px">All systems</h2><div id="s-all"></div></div>
        </div>`);
    },
    async refresh() {
      [st.data, st.tokens] = await Promise.all([GET('overview'), canChange() ? GET('tokens') : Promise.resolve([])]);
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
  const st = { active: false, agent: '', user: '', data: null };
  return {
    interval: 3000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('history', 'jobs')}<div class="page-head"><div><h1>Jobs</h1><div class="sub">Everything sent to agents</div></div><span class="spacer"></span>
        <div class="chips"><button class="chip on" data-act="f" data-v="0">All</button><button class="chip" data-act="f" data-v="1">Active</button></div>
        <label class="row small muted">User <select id="j-user"><option value="">All users</option></select></label>
        <label class="row small muted">System <select id="j-sys"><option value="">All systems</option></select></label></div><div id="j-list"></div>`);
      $('#j-user').addEventListener('change', (e) => { st.user = e.target.value; view.refresh(); });
      $('#j-sys').addEventListener('change', (e) => { st.agent = e.target.value; view.refresh(); });
    },
    async refresh() {
      const q = new URLSearchParams({ limit: '200' });
      if (st.active) q.set('active', '1');
      if (st.user) q.set('user', st.user);
      if (st.agent) q.set('agent', st.agent);
      st.data = await GET('jobs?' + q);
      const { jobs, names } = st.data;
      const actors = [...(st.data.users || []), 'scheduler'];
      setHTML($('#j-user'), html`<option value="">All users</option>${actors.map((n) => html`<option value="${n}" ${st.user === n ? 'selected' : ''}>${n === 'scheduler' ? 'scheduler (automatic)' : n}</option>`)}`);
      const sysOpts = Object.entries(names).sort((a, b) => a[1].localeCompare(b[1]));
      setHTML($('#j-sys'), html`<option value="">All systems</option>${sysOpts.map(([id, n]) => html`<option value="${id}" ${st.agent === id ? 'selected' : ''}>${n}</option>`)}`);
      setHTML($('#j-list'), jobs.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>System</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
        <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}${j.error ? html` <span style="color:var(--red)">${j.error}</span>` : ''}</div></td>
          <td>${names[j.agent_id] ? html`<a href="#/system/${j.agent_id}">${names[j.agent_id]}</a>` : html`<span class="dim">${j.agent_id}</span>`}</td>
          <td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted">${when(j.created)}</td>
          <td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">${st.agent || st.user || st.active ? 'No jobs for this filter.' : 'No jobs.'}</div>`);
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
    // Systems too old to connect cannot be updated, so they are not offered.
    const agents = d.agents.filter((a) => a.status !== 'pending' && !a.legacy);
    const minAgent = R.min_agent || '';
    setHTML($('#r-index'), html`<div class="panel panel-pad">
      <div class="row"><h2 style="margin:0">Version index</h2><span class="spacer"></span>
        ${ix.syncing || st.checking ? html`<span class="row muted small"><span class="spin"></span>checking…</span>` : html`<span class="muted small">${ix.checked_at ? when(ix.checked_at, 'checked ') : 'not checked yet'} · every 6 hours</span>`}
        ${ifChange(html`<button class="btn sm" data-act="check" ${ix.syncing || st.checking ? 'disabled' : ''}>${icon('refresh')} Check now</button>`)}</div>
      <div class="dim small mono" style="margin-top:6px;word-break:break-all">${ix.url || 'no GitHub repository configured — see Settings'}</div>
      ${ix.error ? html`<div class="notice red" style="margin:12px 0 0">${ix.error}</div>` : ''}
      ${repo.errors.length ? html`<div class="notice red" style="margin:12px 0 0"><b>Rejected files</b>${repo.errors.map((e) => html`<div class="mono small">${e}</div>`)}</div>` : ''}
      ${!repo.signing_key ? html`<div class="notice red" style="margin:12px 0 0">The master has no release signing key (/etc/coremon-master/signing.pub). Re-run the installer.</div>` : ''}
      <div class="dim small" style="margin-top:10px">The master downloads only this one signed file, verifies it, then downloads and verifies the listed binaries. Nothing is rolled out to systems until you choose to.</div>
    </div>`);
    setHTML($('#r-master'), html`<div class="panel panel-pad"><h2>Master (this website)</h2>
      <div class="row"><span class="muted">Installed</span><span class="ver">${m.version}</span>
        ${m.update ? html`<span class="badge upd">${m.latest} available</span>${ifChange(html`<button class="btn primary sm" data-act="master-update">${icon('up')} Update master to ${m.latest}</button>`)}` : html`<span class="badge ok">${icon('check')} latest</span>`}</div></div>`);
    const onVer = (v) => agents.filter((a) => a.core_version === v).length;
    setHTML($('#r-agents'), html`<div class="panel panel-pad"><h2>Agent versions <span class="muted small">— last ${repo.core.length || 0} kept on the master</span></h2>
      ${repo.core.length ? html`<table class="tbl"><thead><tr><th>Version</th><th>Published</th><th>Platforms</th><th>Systems</th></tr></thead>
        <tbody>${repo.core.map((v, i) => html`<tr><td><span class="verpill ${i === 0 ? 'ok' : 'old'}">${v.version}</span> ${i === 0 ? html`<span class="badge ok">latest</span>` : ''}</td>
          <td class="muted">${fmtDate(published.get(v.version))}</td><td>${v.platforms.map((p) => html`<span class="tag mono">${p}</span> `)}</td><td class="muted">${onVer(v.version)}</td></tr>`)}</tbody></table>`
        : html`<div class="muted">No agent versions downloaded yet.</div>`}</div>`);
    const outdated = agents.filter((a) => !a.core_is_latest && repo.core_latest);
    setHTML($('#r-rollout'), agents.length ? html`
      <div class="row" style="margin:6px 0 12px"><h2 style="font-size:15px;margin:0">Agents per system</h2><span class="spacer"></span>
        ${outdated.length && canChange() ? html`<button class="btn sm" data-act="all-latest">${icon('up')} Update ${outdated.length} outdated agent(s) to ${repo.core_latest}</button>` : ''}</div>
      <div class="panel"><table class="tbl"><thead><tr><th>System</th><th>Agent</th>${ifChange(html`<th>Change to</th>`)}</tr></thead>
      <tbody>${[...agents].sort(byName).map((a) => html`<tr><td>${agentDot(a)} <a href="#/system/${a.id}"><b>${a.name}</b></a></td><td>${verPill(a)}</td>
        ${ifChange(html`<td><form class="row" style="flex-wrap:nowrap" data-submit="set" data-id="${a.id}"><select name="v">${versionOptions({ core: repo.core.filter((v) => v.platforms.includes(`${a.facts.os}-${a.facts.arch}`) && !(a.sealed && versionLess(v.version, '0.3.0')) && !(minAgent && versionLess(v.version, minAgent))) }, a.core_version)}</select><button class="btn sm" ${a.status === 'active' ? '' : 'disabled'}>Apply</button></form></td>`)}</tr>`)}</tbody></table></div>` : '');
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
      set: async (f) => {
        const a = st.data.agents.find((x) => x.id === f.dataset.id);
        const v = f.v.value, cur = a.core_version;
        if (v === cur) return toast(`${a.name} already runs agent ${v}`);
        const down = versionLess(v, cur);
        if (!(await confirmBox(`${down ? 'Downgrade' : 'Upgrade'} the agent on ${a.name}?`, `${cur} → ${v}. The agent downloads the signed binary, verifies it and restarts.`, { okText: down ? 'Downgrade' : 'Upgrade' }))) return;
        await setAgentVersion(a, v);
      },
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
  const st = { agent: '', user: '', names: {}, agents: [] };
  return {
    interval: 10000,
    mount(el) {
      el.innerHTML = r(html`${subTabs('history', 'audit')}<div class="page-head"><div><h1>Audit log</h1><div class="sub">Who did what, and when</div></div><span class="spacer"></span>
        <label class="row small muted">User <select id="au-user"><option value="">All users</option></select></label>
        <label class="row small muted">System <select id="au-sys"><option value="">All systems</option></select></label></div><div id="au"></div>`);
      $('#au-sys').addEventListener('change', (e) => { st.agent = e.target.value; view.refresh(); });
      $('#au-user').addEventListener('change', (e) => { st.user = e.target.value; view.refresh(); });
    },
    async refresh() {
      const q = new URLSearchParams();
      if (st.agent) q.set('agent', st.agent);
      if (st.user) q.set('user', st.user);
      const d = await GET('audit' + (q.toString() ? '?' + q : ''));
      st.names = d.names;
      const opts = Object.entries(d.names).sort((a, b) => a[1].localeCompare(b[1]));
      setHTML($('#au-sys'), html`<option value="">All systems</option>${opts.map(([id, n]) => html`<option value="${id}" ${st.agent === id ? 'selected' : ''}>${n}</option>`)}`);
      const actors = [...(d.users || []), 'agent', 'scheduler'];
      setHTML($('#au-user'), html`<option value="">All users</option>${actors.map((n) => html`<option value="${n}" ${st.user === n ? 'selected' : ''}>${n === 'agent' || n === 'scheduler' ? n + ' (automatic)' : n}</option>`)}`);
      const rows = d.entries;
      const sys = (e) => e.agent ? (d.names[e.agent] || e.target || e.agent) : (d.names[e.target] || '');
      setHTML($('#au'), rows.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>System</th><th>Target</th><th>Detail</th><th>IP</th></tr></thead>
        <tbody>${rows.map((e) => html`<tr><td class="muted mono small" style="white-space:nowrap">${fmtDate(e.time, true)}</td><td>${e.actor}</td><td><span class="tag mono">${e.action}</span></td>
          <td>${sys(e)}</td><td class="mono small">${e.agent ? '' : (d.names[e.target] ? '' : e.target || '')}</td><td class="muted small">${e.detail || ''}${e.job ? html` <button class="btn ghost sm" data-act="open-job" data-id="${e.job}" title="Show the output of the job this action started">${icon('jobs')} Show output</button>` : ''}</td><td class="dim small">${e.ip || ''}</td></tr>`)}</tbody></table></div>`
        : html`<div class="panel empty">${st.agent || st.user ? 'No entries for this filter.' : 'No entries yet.'}</div>`);
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
  // OS support lookup: when last fetched, next check, last error.
  function osInfoBox(o) {
    if (!o) return;
    const host = (o.source || '').replace(/^https?:\/\//, '').split('/')[0];
    setHTML($('#osinfo-box'), html`<div class="dim small">End-of-support dates of Debian and Ubuntu, from Debian's distro-info-data (${host}).
      ${o.fetched_at ? html`Last fetched ${fmtDate(o.fetched_at)}` : 'Not fetched yet'}${o.next_at ? html` · next check ${fmtDate(o.next_at)}` : ''}.
      ${ifChange(html`<button type="button" class="btn ghost sm" data-act="osinfo-check" ${st.osChecking ? 'disabled' : ''}>${icon('refresh')} Check now</button>`)}</div>
      ${o.error ? html`<div class="notice red small" style="margin:6px 0 0">Last lookup failed (${fmtDate(o.last_try)}): ${o.error}. Retrying every 6 hours; the previous dates stay in use.</div>` : ''}`);
  }
  // Master URL: locked; administrators can Change it or go back to automatic.
  // Takes effect on Save, like the other fields.
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
      const d = await GET('settings');
      const s = d.settings, L = d.limits;
      st.saved = s.retention_days;
      el.innerHTML = r(html`${settingsHead('general', 'General', 'How the master checks, cleans up and is reached')}
        ${canChange() ? '' : html`<div class="notice blue">Your account is read-only: you can see these settings but not change them.</div>`}
        <div class="panel panel-pad" style="max-width:720px">
          <form class="form" data-submit="save" style="margin-top:0"><fieldset class="plain" ${canChange() ? '' : 'disabled'}>
            <label class="field">Automatic update check interval (hours)<input type="number" name="interval" min="1" max="168" value="${s.check_interval_hours}"></label>
            <label class="field">Heartbeat (seconds)<input type="number" name="hb" min="${L.heartbeat_min}" max="${L.heartbeat_max}" value="${s.heartbeat_sec}"></label>
            <div class="dim small" id="hb-hint"></div>
            <label class="field">Keep audit log and job history<span class="range"><input type="range" name="ret" min="${L.retention_min}" max="${L.retention_max}" step="1" value="${s.retention_days}"><b id="ret-val"></b></span></label>
            <div class="dim small" id="ret-info"></div>
            <label class="field">Website port<select name="port">
              <option value="8443" ${s.ui_port !== 443 ? 'selected' : ''}>8443 (default)</option>
              <option value="443" ${s.ui_port === 443 ? 'selected' : ''}>443 (https)</option></select></label>
            <div class="dim small">Agents always connect on port ${d.agent_port}; this only changes where the website is offered. With 443 the website is also still reachable on ${d.agent_port}.</div>
            ${d.ui_port_error ? html`<div class="notice red" style="margin:0">${d.ui_port_error}</div>` : ''}
            <div id="url-box"></div>
            <div class="dim small">Automatic: the address your browser uses, with the agent port. Change it only if agents reach the master under a different name.</div>
            <label class="field">Look up OS support dates<select name="osinfo">
              ${(d.os_info_choices || [3, 14, 30, 90]).map((n) => html`<option value="${n}" ${n === s.os_info_days ? 'selected' : ''}>every ${n} days${n === 14 ? ' (default)' : ''}</option>`)}</select></label>
            <div id="osinfo-box"></div>
            <label class="field">Lowest allowed agent version<select name="minagent">
              ${[...(d.agent_versions || [])].reverse().map((v, i) => html`<option value="${v}" ${v === (s.min_agent_version || d.min_agent) ? 'selected' : ''}>${v}${i === 0 ? ' (oldest stored)' : ''}</option>`)}</select></label>
            <div class="dim small">Older agents can still connect, but only to be updated: the master stores and shows none of their data and runs nothing else on them.
              Agents too old to connect at all (before 0.3.0) need a new install command.</div>
            <label class="field">GitHub repository for releases<input type="text" name="gh" placeholder="owner/name" value="${s.github_repo}"></label>
            <div class="dim small mono" style="word-break:break-all">${d.index_url ? 'Version index: ' + d.index_url : ''}</div>
            ${ifChange(html`<div><button class="btn primary">Save</button></div>`)}
          </fieldset></form>
        </div>`);
      st.d = d;
      st.url = s.master_url || '';
      st.editing = false;
      urlBox();
      osInfoBox(d.os_info);
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
      'osinfo-check': async () => {
        st.osChecking = true; osInfoBox(st.d.os_info);
        try {
          const res = await POST('osinfo/check');
          st.d.os_info = res.os_info;
          toast(res.os_info.error ? 'OS support lookup failed' : 'OS support dates updated', res.os_info.error ? 'err' : 'ok');
        } finally { st.osChecking = false; osInfoBox(st.d.os_info); }
      },
      'url-change': () => { st.editing = true; urlBox(); },
      'url-auto': () => { st.url = ''; st.editing = false; urlBox(); toast('Automatic address — press Save to apply'); },
      save: async (f) => {
        const url = st.editing ? f.url.value.trim() : st.url;
        const res = await POST('settings', { check_interval_hours: +f.interval.value, heartbeat_sec: +f.hb.value, retention_days: +f.ret.value,
          ui_port: +f.port.value, master_url: url, github_repo: f.gh.value, os_info_days: +f.osinfo.value, min_agent_version: f.minagent?.value || '' });
        st.url = res.settings.master_url || '';
        st.editing = false;
        urlBox();
        st.saved = res.settings.retention_days;
        if (res.ui_port_error) toast(res.ui_port_error, 'err');
        else toast('Saved', 'ok');
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
    setHTML($('#u-list'), html`<div class="panel"><table class="tbl"><thead><tr><th>User</th><th>Role</th><th>Created</th><th class="w1"></th></tr></thead>
      <tbody>${st.users.map((u) => html`<tr><td><b>${u.name}</b>${u.id === me ? html` <span class="dim small">(you)</span>` : ''}</td>
        <td>${u.id === me ? html`<span class="muted">${roleLabel(u.role)}</span>` : html`<select class="sm" data-change="role" data-id="${u.id}">${roleOpts(u.role)}</select>`}</td>
        <td class="muted">${fmtDate(u.created)}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap">${u.id === me ? html`<a class="btn ghost sm" href="#/settings/login">Change password</a>`
          : html`<button class="btn ghost sm" data-act="reset" data-id="${u.id}">Reset password</button><button class="btn danger sm" data-act="del" data-id="${u.id}" title="Remove">${icon('trash')}</button>`}</div></td></tr>`)}</tbody></table></div>`);
  }
  const userBy = (id) => st.users.find((u) => u.id === id);
  return {
    mount(el) {
      el.innerHTML = r(html`${settingsHead('users', 'Users', 'Who can sign in to this website')}
        <div class="stack" style="max-width:860px">
          <div class="panel panel-pad"><h2>Add a user</h2>
            <div class="muted small"><b>Administrator</b>: everything. <b>Read-only</b>: sees all systems, jobs, releases, the audit log and settings, changes nothing (except their own password) and cannot see enrollment tokens.</div>
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

function SettingsLogin() {
  return {
    mount(el) {
      const min = session.min_password || 5;
      el.innerHTML = r(html`${settingsHead('login', 'Login', `Signed in as ${session.user?.name} (${roleLabel(session.user?.role)})`)}
        ${session.initial_password ? html`<div class="notice">You are still using the generated initial password — please change it.</div>` : ''}
        <div class="panel panel-pad" style="max-width:480px"><h2>Change password</h2>
          <form class="form" data-submit="pw" style="margin-top:0">
            <label class="field">Current password<input type="password" name="cur" autocomplete="current-password" required></label>
            <label class="field">New password (${pwRange()})<input type="password" name="new" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
            <label class="field">Repeat new password<input type="password" name="new2" autocomplete="new-password" minlength="${pwMin()}" maxlength="${pwMax()}" required></label>
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
  const st = { d: null };
  // Slots arrive as a status (current | new | unknown | ""), never as keys.
  function slot(s) {
    if (s === 'current') return html`<span class="badge ok">${icon('check')} current</span>`;
    if (s === 'new') return html`<span class="badge info">${icon('check')} new</span>`;
    if (s) return html`<span class="badge sec">✗ unknown</span>`;
    return html`<span class="dim">—</span>`;
  }
  function status(a, d) {
    if (!a.sealed) return html`<span class="badge sec" title="Agents before 0.3.0 can no longer connect">agent ${a.core_version || '?'}: old method — run a new install command</span>`;
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
        : html`<div class="dim small" style="margin-top:12px">Key fingerprints are shown to administrators only.${d.rotating ? ` A new key was generated ${fmtDate(d.next_at)} and is being handed to the systems.` : ''}</div>`}
        ${canChange() ? html`<div class="row" style="margin-top:14px">
          ${d.rotating ? html`<span class="muted small">${ready} of ${d.agents.length} system(s) have the new key.</span><span class="spacer"></span>
              <button class="btn ghost" data-act="cancel-key">Cancel</button>
              <button class="btn primary" data-act="switch-key">${icon('refresh')} Switch to new key</button>`
            : html`<span class="muted small">Generate a new key if the current one may have leaked. Nothing changes until you switch.</span><span class="spacer"></span>
              <button class="btn" data-act="gen-key">${icon('plus')} Generate new key</button>`}
        </div>` : ''}
        <div class="dim small" style="margin-top:12px">If this key is ever lost (for example a new master without a backup), agents stop connecting. Run a new install command from Settings → Systems on each system; it updates the stored key.</div>
      </div>
      <div class="panel panel-pad" style="margin-top:18px"><h2>Website certificate (HTTPS)</h2>
        <div class="muted small">Carries the traffic; self-signed for now. Agents do not rely on it.</div>
        ${d.full ? html`<div class="keyrow"><span class="klbl">Fingerprint</span><code class="mono">sha256//${d.https}</code></div>` : ''}
        <div class="keyrow"><span class="klbl">Valid</span><span>${fmtDate(d.cert_from)} – ${fmtDate(d.cert_until)}
          ${d.cert_until - now() < d.cert_renew_days * 86400 ? html` <span class="badge upd xs">renewing soon</span>` : ''}</span></div>
        ${canChange() ? html`<div class="row" style="margin-top:14px"><span class="muted small">Renewed automatically ${d.cert_renew_days} days before it expires.</span><span class="spacer"></span>
          <button class="btn" data-act="new-cert">${icon('refresh')} Generate new certificate</button></div>` : ''}
      </div>`);
    // Systems too old to connect are only listed under Settings > Systems.
    d.agents = d.agents.filter((a) => !(a.core_version && versionLess(a.core_version, '0.3.0')));
    setHTML($('#id-agents'), d.agents.length ? html`<div class="panel"><table class="tbl">
      <thead><tr><th>System</th><th>Slot 1</th><th>Slot 2</th><th>Status</th><th>Reported</th></tr></thead>
      <tbody>${d.agents.map((a) => html`<tr><td>${agentDot(a)} <a href="#/system/${a.id}"><b>${a.name}</b></a></td>
        <td>${a.sealed ? slot(a.slots[0]) : html`<span class="dim">—</span>`}</td>
        <td>${a.sealed ? slot(a.slots[1]) : html`<span class="dim">—</span>`}</td>
        <td>${status(a, d)}</td>
        <td class="muted small">${a.reported_at ? when(a.reported_at) : '—'}</td></tr>`)}</tbody></table></div>`
      : html`<div class="panel empty">No systems yet.</div>`);
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
          <div><h2 style="font-size:15px;margin:6px 0 12px">Keys per system</h2><div id="id-agents"></div></div>
          <div class="dim small">Coremon v${session.version}${releasedLabel() ? ' · ' + releasedLabel() : ''}</div></div>`);
    },
    async refresh() { st.d = await GET('identity'); render(); },
    acts: {
      'gen-key': async () => {
        if (!(await confirmBox('Generate a new master key?', 'The master keeps using the current key. The new key is sent to every system over its current, verified connection; you switch when they all have it.', { okText: 'Generate' }))) return;
        await act('generate'); toast('New key generated — distributing to systems', 'ok');
      },
      'new-cert': async () => {
        if (!(await confirmBox('Generate a new HTTPS certificate?', html`The current certificate is replaced by a new self-signed one, valid for 1 year, and the old one is deleted. It is used immediately.
          <ul style="margin:10px 0 0 18px"><li>Every browser that accepted the old certificate shows the security warning once more (yours too, on the next page load).</li>
          <li>Agents and install commands are not affected.</li></ul>`, { okText: 'Generate new certificate' }))) return;
        await act('cert');
        toast('New certificate in use — reload the page and accept it', 'ok');
      },
      'cancel-key': async () => {
        if (!(await confirmBox('Cancel the new key?', 'The new key is discarded and systems remove it again. The current key stays in use.', { okText: 'Discard new key', danger: true }))) return;
        await act('cancel'); toast('New key discarded', 'ok');
      },
      'switch-key': async () => {
        const missing = st.d.agents.filter((a) => !a.has_next);
        if (!missing.length) {
          if (!(await confirmBox('Switch to the new key?', 'The master starts using the new key and the old key is deleted. Systems reconnect with the new key and remove the old one.', { okText: 'Switch' }))) return;
          await act('switch'); toast('Switched to the new key', 'ok'); return;
        }
        // Not everyone has the new key: the safe choice (Cancel) has focus.
        await new Promise((resolve) => {
          openModal(html`<h3>Not all systems have the new key</h3>
            <div class="notice red" style="margin:12px 0 0">These ${missing.length} system(s) have not reported the new key and will <b>lose contact</b> after the switch:
              <ul>${missing.map((a) => html`<li><b>${a.name}</b> — ${!a.sealed ? `agent ${a.core_version}: old method` : a.online ? 'waiting for the new key' : 'offline'}</li>`)}</ul>
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
