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
};
const icon = (n) => raw(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[n] || ''}</svg>`);

// ---------- formatting ----------
const now = () => Date.now() / 1000;
function ago(ts) {
  if (!ts) return 'never';
  const d = Math.max(0, now() - ts);
  if (d < 45) return 'just now';
  if (d < 3600) return `${Math.round(d / 60)}m ago`;
  if (d < 86400) return `${Math.round(d / 3600)}h ago`;
  return `${Math.round(d / 86400)}d ago`;
}
function dur(s) {
  s = Math.floor(s || 0);
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
}
function bytes(n) {
  if (!n) return '0';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(n < 10 && i ? 1 : 0)} ${u[i]}`;
}
const dt = (ts) => (ts ? new Date(ts * 1000).toLocaleString() : '—');
const pct = (used, total) => (total ? Math.round((used / total) * 100) : 0);

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
    case 'module.call': return `${j.module} · ${j.action}`;
    case 'module.install': return `install module ${j.module}${j.artifact ? ' ' + j.artifact.version : ''}`;
    case 'module.remove': return `remove module ${j.module}`;
    case 'core.update': return `update agent-core${j.artifact ? ' → ' + j.artifact.version : ''}`;
    case 'agent.uninstall': return 'uninstall agent';
    default: return j.type;
  }
}
function jobParams(j) {
  if (j.type !== 'module.call' || !j.params) return '';
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
      $('#jm-sub', m.el).textContent = `${d.agent_name || j.agent_id} · by ${j.actor}${jobParams(j) ? ' · ' + jobParams(j) : ''}`;
      if (d.log) {
        const atBottom = term.scrollHeight - term.scrollTop - term.clientHeight < 40;
        for (const line of d.log.split('\n')) {
          if (line === '' ) continue;
          const span = document.createElement('div');
          if (line.startsWith('$ ')) span.className = 'cmd';
          else if (/^(ERROR|E:)/.test(line)) span.className = 'err';
          span.textContent = line;
          term.appendChild(span);
        }
        offset = d.offset;
        if (atBottom) term.scrollTop = term.scrollHeight;
      }
      const t = j.finished ? `finished ${dt(j.finished)} · ${dur(j.finished - (j.started || j.created))}` : j.started ? `started ${ago(j.started)}` : `queued ${ago(j.created)} — waiting for agent`;
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
  if (t.tagName !== 'INPUT') e.preventDefault();
  Promise.resolve(h(t, e)).catch(fail);
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
let session = { version: '' };
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
        <button class="btn primary" type="submit">Sign in</button>
      </form>
      <div class="hint">First start? The initial password is in <code>/var/lib/coremon-master/initial-password.txt</code> on the master.</div>
    </div></div>`);
  view = {
    acts: {
      login: async (f) => {
        try {
          await POST('login', { password: f.password.value });
          await boot();
        } catch (e) { fail(e); f.password.select(); }
      },
    },
  };
  $('#login-form input').focus();
}

const NAV = [
  ['', 'dash', 'Dashboard'],
  ['systems', 'server', 'Systems'],
  ['jobs', 'jobs', 'Jobs'],
  ['modules', 'box', 'Modules'],
  ['audit', 'shield', 'Audit log'],
  ['settings', 'cog', 'Settings'],
];

function renderShell() {
  const cur = location.hash.replace(/^#\/?/, '').split('/')[0];
  const active = cur === 'agent' ? '' : cur;
  setHTML($('#app'), html`
    <div class="shell">
      <aside class="side">
        <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
        <nav class="nav">${NAV.map(([h, ic, lbl]) => html`<a href="#/${h}" class="${active === h ? 'active' : ''}">${icon(ic)}<span class="lbl">${lbl}</span>${h === 'systems' ? html`<span id="nav-pending"></span>` : ''}</a>`)}</nav>
        <div class="foot"><span>${session.version}</span><button class="btn ghost sm iconbtn" data-act="logout" title="Sign out">${icon('logout')}</button></div>
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
  const views = { '': Dashboard, agent: AgentPage, systems: Systems, jobs: Jobs, modules: Modules, audit: Audit, settings: Settings };
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
  return html`<span class="dot ${a.online ? 'on' : 'off'}" title="${a.online ? 'online' : 'offline since ' + ago(a.last_seen)}"></span>`;
}
function osLabel(f) { return f.os_name || [f.os_id, f.os_version].filter(Boolean).join(' ') || 'unknown OS'; }
function updBadges(a) {
  const u = a.updates;
  const out = [];
  if (u?.security) out.push(html`<span class="badge sec">${u.security} security</span>`);
  if (u?.reboot_required) out.push(html`<span class="badge reboot">reboot</span>`);
  if (a.core_upgrade) out.push(html`<span class="badge info" title="agent-core ${a.core_latest} available">agent ↑</span>`);
  if (a.modules?.some((m) => m.upgrade)) out.push(html`<span class="badge info">module ↑</span>`);
  return out;
}

async function runAction(ids, module, action, params, { open = true } = {}) {
  const res = await POST('actions', { agent_ids: ids, module, action, params });
  (res.errors || []).forEach((e) => toast(e, 'err'));
  if (res.jobs?.length) {
    toast(`${res.jobs.length} job(s) queued`, 'ok');
    if (open && res.jobs.length === 1) openJob(res.jobs[0]);
  }
  view?.refresh?.();
  return res;
}

function approveModal(a, repo) {
  const mods = repo.modules || [];
  openModal(html`
    <h3>Approve ${a.name}</h3>
    <div class="muted small">${osLabel(a.facts)} · ${a.facts.arch} · ${(a.facts.ips || []).join(', ')}</div>
    <form class="form" data-submit="approve">
      <label class="field">Display name<input type="text" name="name" value="${a.name}" maxlength="64"></label>
      <label class="field">Tags (comma separated)<input type="text" name="tags" placeholder="e.g. lxc, prod, proxmox" value="${(a.tags || []).join(', ')}"></label>
      <div class="field"><span class="muted small">Modules to deploy</span>
        ${mods.length ? mods.map((m) => html`<label class="row" style="color:var(--text)"><input type="checkbox" name="mod" value="${m.name}" ${m.name === 'updates' ? 'checked' : ''}> ${m.name} <span class="dim mono">${m.latest}</span></label>`)
        : html`<div class="notice">No modules in the repository yet — copy a signed release to the master first.</div>`}
      </div>
      <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary" type="submit">${icon('check')} Approve</button></div>
    </form>`, {
    acts: {
      cancel: () => closeModal(),
      approve: async (f) => {
        const modsSel = [...f.querySelectorAll('input[name=mod]:checked')].map((i) => i.value);
        await POST(`agents/${a.id}/approve`, { name: f.name.value, tags: f.tags.value.split(','), modules: modsSel });
        closeModal();
        toast(`${f.name.value} approved`, 'ok');
        view?.refresh?.();
      },
    },
  });
}

// ---------- Dashboard ----------
function Dashboard() {
  const st = { filter: 'all', q: '', tag: '', sel: new Set(), data: null };
  const matches = (a) => {
    const u = a.updates || {};
    const f = st.filter;
    if (f === 'updates' && !(u.total > 0)) return false;
    if (f === 'security' && !(u.security > 0)) return false;
    if (f === 'reboot' && !u.reboot_required) return false;
    if (f === 'offline' && (a.online || a.status === 'pending')) return false;
    if (f === 'pending' && a.status !== 'pending') return false;
    if (st.tag && !a.tags.includes(st.tag)) return false;
    if (st.q) {
      const hay = `${a.name} ${osLabel(a.facts)} ${(a.facts.ips || []).join(' ')} ${a.tags.join(' ')}`.toLowerCase();
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
    const memUsed = f.mem_total - f.mem_avail, diskUsed = f.disk_total - f.disk_free;
    const mp = pct(memUsed, f.mem_total), dp = pct(diskUsed, f.disk_total);
    let mid;
    if (a.status === 'pending') {
      mid = html`<div class="mid"><div><div class="biglbl">Waiting for approval</div><div class="dim small">enrolled ${ago(a.enrolled_at)}</div></div>
        <div class="badges"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button></div></div>`;
    } else if (!u) {
      const hasUpd = a.modules.some((m) => m.name === 'updates');
      mid = html`<div class="mid"><div><div class="big dim">—</div></div><div class="biglbl">${hasUpd ? 'waiting for first check' : 'updates module not deployed'}</div><div class="badges">${updBadges(a)}</div></div>`;
    } else {
      const cls = u.security ? 'sec' : u.total ? 'some' : 'zero';
      mid = html`<div class="mid"><div class="big ${cls}">${u.total}</div><div class="biglbl">${u.total === 1 ? 'update' : 'updates'}<br>available</div><div class="badges">${updBadges(a)}</div></div>`;
    }
    return html`
      <div class="card ${accent} ${st.sel.has(a.id) ? 'sel' : ''}" data-act="open-agent" data-id="${a.id}">
        <div class="top">${agentDot(a)}<div style="min-width:0"><div class="name">${a.name}</div><div class="os">${osLabel(f)} · ${f.arch || '?'} · ${f.virt || ''}</div></div>
          ${selectable ? html`<input type="checkbox" class="selbox" data-act="sel" data-id="${a.id}" ${st.sel.has(a.id) ? 'checked' : ''} title="Select">` : ''}</div>
        ${mid}
        ${a.status !== 'pending' && f.mem_total ? html`<div class="meters">
          <div class="meter"><div class="lbl"><span>Memory</span><span>${mp}%</span></div><div class="bar"><i class="${mp > 85 ? 'hot' : ''}" style="width:${mp}%"></i></div></div>
          <div class="meter"><div class="lbl"><span>Disk</span><span>${dp}%</span></div><div class="bar"><i class="${dp > 85 ? 'hot' : ''}" style="width:${dp}%"></i></div></div></div>` : ''}
        <div class="foot">
          ${a.busy ? html`<span class="spin"></span><span>${a.busy} job${a.busy > 1 ? 's' : ''}</span>` : html`<span>${a.online ? (u ? 'checked ' + ago(u.checked_at) : 'online') : a.status === 'pending' ? '' : 'seen ' + ago(a.last_seen)}</span>`}
          <span class="spacer"></span><div class="tags">${a.tags.map((t) => html`<span class="tag">${t}</span>`)}</div>
        </div>
      </div>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
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
    const chips = [['all', 'All'], ['updates', 'Updates'], ['security', 'Security'], ['reboot', 'Reboot'], ['offline', 'Offline'], ['pending', 'Pending']];
    setHTML($('#d-chips'), chips.map(([k, l]) => html`<button class="chip ${st.filter === k ? 'on' : ''}" data-act="filter" data-f="${k}">${l}<span class="n">${count(k)}</span></button>`));
    const tags = [...new Set(A.flatMap((a) => a.tags))].sort();
    const tagSel = $('#d-tag');
    const tagHTML = html`<option value="">All tags</option>${tags.map((t) => html`<option value="${t}" ${st.tag === t ? 'selected' : ''}>${t}</option>`)}`;
    setHTML(tagSel, tagHTML);
    tagSel.style.display = tags.length ? '' : 'none';
    const list = A.filter(matches);
    if (!A.length) {
      setHTML($('#d-cards'), html`<div class="panel empty"><div class="big">🛰️</div><div>No systems yet.</div><div style="margin-top:14px"><a class="btn primary" href="#/systems">${icon('plus')} Add your first system</a></div></div>`);
    } else if (!list.length) {
      setHTML($('#d-cards'), html`<div class="panel empty">No systems match this filter.</div>`);
    } else {
      setHTML($('#d-cards'), html`<div class="cards">${list.map(card)}</div>`);
    }
    for (const id of [...st.sel]) if (!A.some((a) => a.id === id && a.status === 'active')) st.sel.delete(id);
    const n = st.sel.size;
    setHTML($('#d-bulk'), n ? html`<div class="bulkbar"><b>${n}</b> selected<span class="spacer"></span>
      <button class="btn sm" data-act="bulk-check">${icon('refresh')} Check now</button>
      <button class="btn warn sm" data-act="bulk-upgrade">${icon('zap')} Upgrade all packages</button>
      <button class="btn ghost sm" data-act="bulk-clear">Clear</button></div>` : '');
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`
        <div class="page-head"><div><h1>Dashboard</h1><div class="sub">Update status across your fleet</div></div><span class="spacer"></span>
          <button class="btn" data-act="check-all">${icon('refresh')} Check all</button></div>
        <div id="d-notice"></div>
        <div class="stats" id="d-stats"></div>
        <div class="toolbar"><input type="search" class="search" id="d-q" placeholder="Search name, OS, IP, tag…"><select id="d-tag"></select><div class="chips" id="d-chips"></div></div>
        <div id="d-cards"></div>
        <div id="d-bulk"></div>`);
      $('#d-q').addEventListener('input', (e) => { st.q = e.target.value; render(); });
      $('#d-tag').addEventListener('change', (e) => { st.tag = e.target.value; render(); });
      if (session.initial_password) setHTML($('#d-notice'), html`<div class="notice">You are using the generated initial password. <a href="#/settings">Change it in Settings</a>.</div>`);
    },
    async refresh() { st.data = await GET('overview'); render(); },
    acts: {
      filter: (t) => { st.filter = t.dataset.f; render(); },
      'open-agent': (t) => { location.hash = '#/agent/' + t.dataset.id; },
      sel: (t) => { t.checked ? st.sel.add(t.dataset.id) : st.sel.delete(t.dataset.id); render(); },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.repo),
      'bulk-clear': () => { st.sel.clear(); render(); },
      'bulk-check': () => runAction([...st.sel], 'updates', 'check', {}, { open: false }),
      'bulk-upgrade': async () => {
        const names = st.data.agents.filter((a) => st.sel.has(a.id)).map((a) => a.name);
        if (await confirmBox('Upgrade all packages?', `This runs a full package upgrade on ${names.length} system(s): ${names.join(', ')}.`, { okText: 'Upgrade all', danger: true })) {
          await runAction([...st.sel], 'updates', 'upgrade_all', {}, { open: false });
          st.sel.clear();
        }
      },
      'check-all': () => {
        const ids = st.data.agents.filter((a) => a.status === 'active' && a.online && a.modules.some((m) => m.name === 'updates' && m.manifest)).map((a) => a.id);
        if (!ids.length) return toast('No online systems with the updates module', 'err');
        return runAction(ids, 'updates', 'check', {}, { open: false });
      },
    },
  };
}

// ---------- Agent page ----------
function AgentPage([id]) {
  const st = { tab: 'updates', sel: new Set(), data: null };
  const upd = () => {
    const raw = st.data?.agent?.state?.updates;
    return raw || null;
  };
  function header(a) {
    const f = a.facts;
    return html`
      <div class="page-head">
        <div><div class="row"><a href="#/" class="btn ghost sm iconbtn" title="Back">${icon('back')}</a>${agentDot(a)}<h1>${a.name}</h1>
          ${a.status === 'paused' ? html`<span class="badge info">paused</span>` : ''}${a.status === 'pending' ? html`<span class="badge upd">pending approval</span>` : ''}
          ${a.tags.map((t) => html`<span class="tag">${t}</span>`)}</div>
          <div class="sub">${osLabel(f)} · ${f.kernel || ''} · ${f.arch} · ${f.virt} · ${a.online ? 'online, up ' + dur(f.uptime_sec) : 'offline, last seen ' + ago(a.last_seen)}</div></div>
        <span class="spacer"></span>
        ${a.busy ? html`<span class="row muted"><span class="spin"></span>${a.busy} job(s) active</span>` : ''}
        <button class="btn sm" data-act="edit">${icon('edit')} Edit</button>
        ${a.status === 'active' ? html`<button class="btn sm" data-act="pause">${icon('pause')} Pause</button>` : ''}
        ${a.status === 'paused' ? html`<button class="btn sm" data-act="resume">${icon('play')} Resume</button>` : ''}
        <button class="btn danger sm" data-act="remove">${icon('trash')} Remove</button>
      </div>
      ${a.status === 'pending' ? html`<div class="notice blue row">This system enrolled ${ago(a.enrolled_at)} and is waiting for approval. Until approved it receives no commands.<span class="spacer"></span><button class="btn primary sm" data-act="approve">${icon('check')} Approve</button></div>` : ''}
      ${a.removing ? html`<div class="notice red">Uninstall requested — the agent will remove itself.</div>` : ''}`;
  }
  function updatesTab(a) {
    const mod = a.modules.find((m) => m.name === 'updates');
    const repoHas = (st.data.repo.modules || []).some((m) => m.name === 'updates');
    if (!mod) {
      return html`<div class="panel empty"><div class="big">📦</div><div>The <b>updates</b> module is not deployed on this system.</div>
        ${repoHas && a.status === 'active' ? html`<div style="margin-top:14px"><button class="btn primary" data-act="mod-op" data-name="updates" data-op="deploy">${icon('download')} Deploy updates module</button></div>` : ''}</div>`;
    }
    if (!mod.deployed || !mod.manifest) {
      return html`<div class="panel empty"><div class="spin" style="width:26px;height:26px"></div><div style="margin-top:12px">Installing the updates module…</div></div>`;
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
          <span class="muted small">checked ${ago(s.checked_at)} · package lists refreshed ${ago(s.refreshed_at)} · ${s.backend}</span>
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
  function modulesTab(a) {
    const repo = st.data.repo;
    const deployed = new Set(a.modules.map((m) => m.name));
    const avail = (repo.modules || []).filter((m) => !deployed.has(m.name));
    const active = a.status === 'active';
    return html`
      <div class="panel" style="margin-bottom:16px"><table class="tbl">
        <thead><tr><th>Component</th><th>Installed</th><th>Available</th><th>Status</th><th class="w1"></th></tr></thead>
        <tbody>
          <tr><td><b>agent-core</b><div class="dim small">installed by you; updates itself on request</div></td><td class="ver">${a.core_version}</td><td class="ver">${a.core_latest || '—'}</td>
            <td>${a.desired_core && a.desired_core !== a.core_version ? html`<span class="badge info"><span class="spin" style="width:10px;height:10px"></span> updating to ${a.desired_core}</span>` : a.core_upgrade ? html`<span class="badge upd">update available</span>` : html`<span class="badge ok">current</span>`}</td>
            <td class="w1">${a.core_upgrade && active ? html`<button class="btn sm" data-act="core-update">${icon('up')} Update</button>` : ''}</td></tr>
          ${a.modules.map((m) => html`
          <tr><td><b>${m.manifest?.title || m.name}</b> <span class="dim mono small">${m.name}</span><div class="dim small">${m.manifest?.description || ''}</div></td>
            <td class="ver">${m.version || '—'}</td><td class="ver">${m.latest || '—'}</td>
            <td>${m.error ? html`<span class="badge sec">${m.error}</span>` : !m.deployed ? html`<span class="badge info"><span class="spin" style="width:10px;height:10px"></span> installing</span>` : m.desired && m.desired !== m.version ? html`<span class="badge info">updating to ${m.desired}</span>` : m.upgrade ? html`<span class="badge upd">update available</span>` : html`<span class="badge ok">installed</span>`}</td>
            <td class="w1"><div class="row" style="flex-wrap:nowrap">${m.upgrade && active ? html`<button class="btn sm" data-act="mod-op" data-name="${m.name}" data-op="upgrade">${icon('up')} Upgrade</button>` : ''}
              ${active ? html`<button class="btn danger sm" data-act="mod-op" data-name="${m.name}" data-op="remove">${icon('trash')}</button>` : ''}</div></td></tr>`)}
        </tbody></table></div>
      ${avail.length ? html`<div class="panel panel-pad"><h2>Available modules</h2><div class="row">${avail.map((m) => html`<button class="btn" data-act="mod-op" data-name="${m.name}" data-op="deploy" ${active ? '' : 'disabled'}>${icon('download')} Deploy ${m.name} <span class="dim mono">${m.latest}</span></button>`)}</div></div>` : ''}`;
  }
  function jobsTab() {
    const jobs = st.data.jobs;
    if (!jobs.length) return html`<div class="panel empty">No jobs yet.</div>`;
    return html`<div class="panel"><table class="tbl"><thead><tr><th>Job</th><th>Status</th><th>By</th><th>Created</th><th>Duration</th></tr></thead>
      <tbody>${jobs.map((j) => html`<tr class="click" data-act="open-job" data-id="${j.id}"><td><b>${jobTitle(j)}</b><div class="dim small">${jobParams(j)}</div></td><td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted" title="${dt(j.created)}">${ago(j.created)}</td><td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>`;
  }
  function systemTab(a) {
    const f = a.facts;
    const kv = [
      ['Hostname', f.hostname], ['Operating system', osLabel(f)], ['Kernel', f.kernel], ['Architecture', `${f.os}/${f.arch}`],
      ['Virtualization', f.virt], ['Uptime', dur(f.uptime_sec)], ['CPUs', f.cpus], ['Load (1m)', f.load1?.toFixed(2)],
      ['Memory', `${bytes(f.mem_total - f.mem_avail)} / ${bytes(f.mem_total)}`], ['Disk /', `${bytes(f.disk_total - f.disk_free)} / ${bytes(f.disk_total)}`],
      ['IP addresses', (f.ips || []).join(', ')], ['Agent ID', a.id], ['agent-core', a.core_version], ['Enrolled', dt(a.enrolled_at)], ['Last seen', dt(a.last_seen)],
    ];
    return html`<div class="panel panel-pad"><div class="facts">${kv.map(([k, v]) => html`<div><div class="k">${k}</div><div class="v">${v ?? '—'}</div></div>`)}</div></div>`;
  }
  function render() {
    const d = st.data;
    if (!d) return;
    const a = d.agent;
    setHTML($('#a-head'), header(a));
    const tabs = [['updates', 'Updates'], ['modules', 'Modules'], ['jobs', 'Jobs'], ['system', 'System']];
    setHTML($('#a-tabs'), tabs.map(([k, l]) => html`<button class="${st.tab === k ? 'on' : ''}" data-act="tab" data-t="${k}">${l}${k === 'updates' && a.updates?.total ? html` <span class="badge upd">${a.updates.total}</span>` : ''}</button>`));
    const body = { updates: updatesTab, modules: modulesTab, jobs: jobsTab, system: systemTab }[st.tab](a);
    setHTML($('#a-body'), body);
  }
  const a = () => st.data.agent;
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
      approve: () => approveModal(a(), st.data.repo),
      pause: async () => { await POST('agents/' + id, { paused: true }); toast('Paused — no jobs will be sent', 'ok'); this_refresh(); },
      resume: async () => { await POST('agents/' + id, { paused: false }); toast('Resumed', 'ok'); this_refresh(); },
      'core-update': async () => { await POST(`agents/${id}/core`); toast('agent-core update scheduled', 'ok'); this_refresh(); },
      'mod-op': async (t) => {
        const { name, op } = t.dataset;
        if (op === 'remove' && !(await confirmBox(`Remove module ${name}?`, 'The module is deleted from the system. Its data in the master is kept.', { okText: 'Remove', danger: true }))) return;
        await POST(`agents/${id}/modules`, { name, op });
        toast(`${name}: ${op} scheduled`, 'ok');
        this_refresh();
      },
      edit: () => {
        const ag = a();
        openModal(html`<h3>Edit ${ag.name}</h3><form class="form" data-submit="save">
          <label class="field">Display name<input type="text" name="name" value="${ag.name}" maxlength="64" autofocus></label>
          <label class="field">Tags (comma separated)<input type="text" name="tags" value="${ag.tags.join(', ')}"></label>
          <div class="actions"><button type="button" class="btn ghost" data-act="cancel">Cancel</button><button class="btn primary">Save</button></div></form>`, {
          acts: {
            cancel: () => closeModal(),
            save: async (f) => { await POST('agents/' + id, { name: f.name.value, tags: f.tags.value.split(',') }); closeModal(); this_refresh(); },
          },
        });
      },
      remove: () => {
        const ag = a();
        openModal(html`<h3>Remove ${ag.name}?</h3>
          <div class="muted" style="margin-top:8px">Choose how to remove this system from coremon.</div>
          <div class="stack" style="margin-top:16px;gap:10px">
            <button class="btn danger" data-act="uninstall" ${ag.online ? '' : 'disabled'}>${icon('trash')} Uninstall agent from the system and remove</button>
            <button class="btn" data-act="forget">Only forget it here (agent stays installed, will be rejected)</button>
          </div>
          <div class="actions"><button class="btn ghost" data-act="cancel">Cancel</button></div>`, {
          acts: {
            cancel: () => closeModal(),
            uninstall: async () => { const res = await POST(`agents/${id}/remove`, { uninstall: true }); closeModal(); if (res.job) openJob(res.job); toast('Uninstall queued', 'ok'); },
            forget: async () => { await POST(`agents/${id}/remove`, { uninstall: false }); closeModal(); location.hash = '#/'; toast('System removed', 'ok'); },
          },
        });
      },
    },
  };
  function this_refresh() { view?.refresh?.(); }
}

// ---------- Systems (enrollment & management) ----------
function Systems() {
  const st = { data: null, tokens: [], lastCmd: null };
  function render() {
    const d = st.data;
    if (!d) return;
    const pend = d.agents.filter((a) => a.status === 'pending');
    setPending(pend.length);
    setHTML($('#s-pending'), pend.length ? html`<div class="panel panel-pad"><h2>Waiting for approval <span class="badge upd">${pend.length}</span></h2>
      <table class="tbl"><tbody>${pend.map((a) => html`<tr><td>${agentDot(a)} <b>${a.name}</b></td><td class="muted">${osLabel(a.facts)} · ${a.facts.arch}</td><td class="muted">${(a.facts.ips || []).join(', ')}</td><td class="muted">${ago(a.enrolled_at)}</td>
        <td class="w1"><div class="row" style="flex-wrap:nowrap"><button class="btn primary sm" data-act="approve" data-id="${a.id}">${icon('check')} Approve</button><button class="btn danger sm" data-act="reject" data-id="${a.id}">Reject</button></div></td></tr>`)}</tbody></table></div>` : '');
    setHTML($('#s-tokens'), st.tokens.length ? html`<table class="tbl" style="margin-top:14px"><thead><tr><th>Open token</th><th>Note</th><th>Expires</th><th class="w1"></th></tr></thead>
      <tbody>${st.tokens.map((t) => html`<tr><td class="mono">${t.prefix}…</td><td class="muted">${t.note || '—'}</td><td class="muted">${dt(t.expires)}</td><td class="w1"><button class="btn ghost sm" data-act="revoke" data-hash="${t.hash}">Revoke</button></td></tr>`)}</tbody></table>` : '');
    const act = d.agents.filter((a) => a.status !== 'pending');
    setHTML($('#s-all'), act.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>System</th><th>OS</th><th>Tags</th><th>agent-core</th><th>Modules</th><th>Last seen</th><th class="w1"></th></tr></thead>
      <tbody>${act.map((a) => html`<tr><td>${agentDot(a)} <a href="#/agent/${a.id}"><b>${a.name}</b></a> ${a.status === 'paused' ? html`<span class="badge info">paused</span>` : ''}</td>
        <td class="muted">${osLabel(a.facts)}</td><td>${a.tags.map((t) => html`<span class="tag">${t}</span> `)}</td>
        <td class="ver">${a.core_version}${a.core_upgrade ? html` <span class="badge info">↑ ${a.core_latest}</span>` : ''}</td>
        <td>${a.modules.map((m) => html`<span class="tag mono">${m.name} ${m.version || '…'}</span> `)}</td>
        <td class="muted">${a.online ? 'online' : ago(a.last_seen)}</td>
        <td class="w1">${a.status === 'active' ? html`<button class="btn ghost sm" data-act="pause" data-id="${a.id}" title="Pause">${icon('pause')}</button>` : html`<button class="btn ghost sm" data-act="resume" data-id="${a.id}" title="Resume">${icon('play')}</button>`}</td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">No approved systems yet.</div>`);
  }
  return {
    interval: 4000,
    mount(el) {
      el.innerHTML = r(html`
        <div class="page-head"><div><h1>Systems</h1><div class="sub">Enroll, approve and manage agents</div></div></div>
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
          ${res.has_core ? '' : html`<div class="notice red" style="margin-top:14px">No agent-core binary is in the release repository yet — the install will fail until you copy a signed release to the master.</div>`}
          <div class="cmdbox"><code id="s-cmdtext">${res.command}</code><button class="btn" data-act="copy">${icon('copy')} Copy</button></div>
          <div class="dim small" style="margin-top:8px">Valid until ${dt(res.expires)} · single use · requires curl and systemd · options: <code>--allow-modules updates</code> to restrict which modules this system accepts, <code>--allow-modules none</code> for none.</div>`);
        f.note.value = '';
        view.refresh();
      },
      copy: async () => {
        try { await navigator.clipboard.writeText(st.lastCmd); toast('Copied', 'ok'); }
        catch { const rng = document.createRange(); rng.selectNode($('#s-cmdtext')); getSelection().removeAllRanges(); getSelection().addRange(rng); toast('Press Ctrl+C to copy'); }
      },
      revoke: async (t) => { await POST(`tokens/${t.dataset.hash}/delete`); view.refresh(); },
      approve: (t) => approveModal(st.data.agents.find((a) => a.id === t.dataset.id), st.data.repo),
      reject: async (t) => {
        if (await confirmBox('Reject this system?', 'It is removed from the master. Its agent will be refused from now on.', { okText: 'Reject', danger: true })) {
          await POST(`agents/${t.dataset.id}/remove`, { uninstall: false }); view.refresh();
        }
      },
      pause: async (t) => { await POST('agents/' + t.dataset.id, { paused: true }); view.refresh(); },
      resume: async (t) => { await POST('agents/' + t.dataset.id, { paused: false }); view.refresh(); },
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
          <td>${statusBadge(j.status)}</td><td class="muted">${j.actor}</td><td class="muted" title="${dt(j.created)}">${ago(j.created)}</td>
          <td class="muted">${j.finished ? dur(j.finished - (j.started || j.created)) : '—'}</td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">No jobs.</div>`);
    },
    acts: {
      f: (t) => { st.active = t.dataset.v === '1'; document.querySelectorAll('.chips .chip').forEach((c) => c.classList.toggle('on', c === t)); view.refresh(); },
    },
  };
}

// ---------- Modules ----------
function Modules() {
  const st = { data: null, gh: null };
  function releases() {
    const g = st.gh;
    if (!g) return '';
    const s = g.status, m = g.master;
    if (!g.repo) {
      return html`<div class="panel panel-pad" style="margin-bottom:18px"><h2>Releases</h2><div class="muted">No GitHub repository configured. Set one under <a href="#/settings">Settings</a> to receive signed releases automatically.</div></div>`;
    }
    return html`<div class="panel panel-pad" style="margin-bottom:18px">
      <div class="row"><h2 style="margin:0">Releases from <a href="https://github.com/${g.repo}" target="_blank" rel="noopener noreferrer">${g.repo}</a></h2><span class="spacer"></span>
        ${s.importing ? html`<span class="row muted small"><span class="spin"></span>importing ${s.importing}…</span>` : ''}
        <span class="muted small">${s.checked_at ? 'checked ' + ago(s.checked_at) : 'not checked yet'} · auto-import ${g.auto_import ? 'on' : 'off'}</span>
        <button class="btn sm" data-act="gh-check">${icon('refresh')} Check GitHub</button></div>
      ${s.error ? html`<div class="notice red" style="margin:12px 0 0">${s.error}</div>` : ''}
      ${s.last_import ? html`<div class="dim small" style="margin-top:8px">Last import: ${s.last_import}</div>` : ''}
      <div class="row" style="margin-top:14px">
        <span class="muted">Master</span><span class="ver">${m.version}</span>
        ${m.update ? html`<span class="badge upd">${m.latest} available</span><button class="btn primary sm" data-act="master-update">${icon('up')} Update master to ${m.latest}</button>` : html`<span class="badge ok">current</span>`}
      </div>
      ${s.releases.length ? html`<table class="tbl" style="margin-top:12px"><thead><tr><th>Version</th><th>Published</th><th>Status</th><th class="w1"></th></tr></thead>
        <tbody>${s.releases.slice(0, 8).map((r) => html`<tr><td><a href="${r.url}" target="_blank" rel="noopener noreferrer" class="mono">v${r.version}</a></td><td class="muted">${r.published ? new Date(r.published).toLocaleString() : ''}</td>
          <td>${r.imported ? html`<span class="badge ok">${icon('check')} imported</span>` : r.has_asset ? html`<span class="badge upd">available</span>` : html`<span class="badge mute">no release asset</span>`}</td>
          <td class="w1">${!r.imported && r.has_asset ? html`<button class="btn sm" data-act="gh-import" data-v="${r.version}" ${s.importing ? 'disabled' : ''}>${icon('download')} Import</button>` : ''}</td></tr>`)}</tbody></table>` : html`<div class="muted" style="margin-top:12px">No published releases found.</div>`}
      <div class="dim small" style="margin-top:10px">Importing verifies every signature and makes the versions available below. Nothing is rolled out to systems until you choose to.</div>
    </div>`;
  }
  function render() {
    const d = st.data;
    const repo = d.repo;
    const agents = d.agents.filter((a) => a.status === 'active');
    setHTML($('#m-gh'), releases());
    setHTML($('#m-repo'), html`
      ${repo.errors.length ? html`<div class="notice red"><b>Rejected artifacts</b><br>${repo.errors.map((e) => html`<div class="mono small">${e}</div>`)}</div>` : ''}
      ${!repo.signing_key ? html`<div class="notice red">The master has no release signing key (/etc/coremon-master/signing.pub). Re-run the installer.</div>` : ''}
      <div class="grid2">
        <div class="panel panel-pad"><h2>agent-core</h2>
          ${repo.core_latest ? html`<div class="row"><span class="badge ok">latest ${repo.core_latest}</span>${(repo.core_platforms || []).map((p) => html`<span class="tag mono">${p}</span>`)}</div>
          <div class="dim small" style="margin-top:8px">All versions: ${repo.core_versions.join(', ')}</div>` : html`<div class="muted">No agent-core in the repository.</div>`}</div>
        ${repo.modules.map((m) => html`<div class="panel panel-pad"><h2>${m.name}</h2><div class="row"><span class="badge ok">latest ${m.latest}</span>${m.platforms.map((p) => html`<span class="tag mono">${p}</span>`)}</div>
          <div class="dim small" style="margin-top:8px">All versions: ${m.versions.join(', ')}</div></div>`)}
      </div>`);
    if (!agents.length) return setHTML($('#m-matrix'), html`<div class="panel empty">No approved systems.</div>`);
    const coreBehind = agents.filter((a) => a.core_upgrade).length;
    setHTML($('#m-matrix'), html`
      <div class="row" style="margin:6px 0 12px"><h2 style="font-size:15px;margin:0">Rollout</h2><span class="spacer"></span>
        ${coreBehind ? html`<button class="btn sm" data-act="core-all">${icon('up')} Update agent-core on ${coreBehind} system(s)</button>` : ''}
        ${repo.modules.map((m) => { const n = agents.filter((a) => { const am = a.modules.find((x) => x.name === m.name); return !am || am.upgrade; }).length; return n ? html`<button class="btn sm" data-act="mod-all" data-name="${m.name}">${icon('download')} ${m.name} → ${n} system(s)</button>` : ''; })}
      </div>
      <div class="panel"><table class="tbl"><thead><tr><th>System</th><th>agent-core</th>${repo.modules.map((m) => html`<th>${m.name}</th>`)}</tr></thead>
      <tbody>${agents.map((a) => html`<tr><td>${agentDot(a)} <a href="#/agent/${a.id}"><b>${a.name}</b></a></td>
        <td><span class="ver">${a.core_version}</span> ${a.core_upgrade ? html`<button class="btn ghost sm" data-act="core-one" data-id="${a.id}">${icon('up')} ${a.core_latest}</button>` : ''}</td>
        ${repo.modules.map((m) => { const am = a.modules.find((x) => x.name === m.name);
          if (!am) return html`<td><button class="btn ghost sm" data-act="mod-one" data-id="${a.id}" data-name="${m.name}" data-op="deploy">${icon('download')} deploy</button></td>`;
          if (!am.deployed) return html`<td><span class="badge info">installing…</span></td>`;
          return html`<td><span class="ver">${am.version}</span> ${am.upgrade ? html`<button class="btn ghost sm" data-act="mod-one" data-id="${a.id}" data-name="${m.name}" data-op="upgrade">${icon('up')} ${m.latest}</button>` : ''}</td>`; })}
      </tr>`)}</tbody></table></div>`);
  }
  return {
    interval: 5000,
    mount(el) {
      el.innerHTML = r(html`<div class="page-head"><div><h1>Modules</h1><div class="sub">Signed release repository and rollout</div></div><span class="spacer"></span>
        <button class="btn" data-act="rescan">${icon('refresh')} Rescan repository</button></div><div id="m-gh"></div><div id="m-repo"></div><div style="margin-top:22px" id="m-matrix"></div>`);
    },
    async refresh() { [st.data, st.gh] = await Promise.all([GET('overview'), GET('github')]); render(); },
    acts: {
      rescan: async () => { await POST('repo/rescan'); toast('Repository rescanned', 'ok'); view.refresh(); },
      'gh-check': async () => { st.gh = await POST('github/check'); render(); toast(st.gh.status.error ? 'GitHub check failed' : 'GitHub checked', st.gh.status.error ? 'err' : 'ok'); },
      'gh-import': async (t) => {
        toast(`Importing v${t.dataset.v}…`);
        st.gh = await POST('github/import', { version: t.dataset.v });
        toast(`v${t.dataset.v} imported and verified`, 'ok');
        view.refresh();
      },
      'master-update': async () => {
        const m = st.gh.master;
        if (!(await confirmBox(`Update the master to ${m.latest}?`, 'The signed binary is verified by a root service, installed, and the master restarts. The page reconnects after a few seconds.', { okText: 'Update master' }))) return;
        await POST('master/update');
        toast('Master update requested — restarting shortly', 'ok');
      },
      'core-one': async (t) => { await POST(`agents/${t.dataset.id}/core`); view.refresh(); },
      'core-all': async () => {
        const list = st.data.agents.filter((a) => a.status === 'active' && a.core_upgrade);
        if (!(await confirmBox('Update agent-core?', `${list.length} agent(s) will download the new signed binary and restart.`, { okText: 'Update' }))) return;
        for (const a of list) await POST(`agents/${a.id}/core`);
        toast('Scheduled', 'ok'); view.refresh();
      },
      'mod-one': async (t) => { await POST(`agents/${t.dataset.id}/modules`, { name: t.dataset.name, op: t.dataset.op }); view.refresh(); },
      'mod-all': async (t) => {
        const name = t.dataset.name;
        const list = st.data.agents.filter((a) => a.status === 'active' && (() => { const am = a.modules.find((x) => x.name === name); return !am || am.upgrade; })());
        if (!(await confirmBox(`Roll out ${name}?`, `Deploy or upgrade ${name} on: ${list.map((a) => a.name).join(', ')}.`, { okText: 'Roll out' }))) return;
        for (const a of list) await POST(`agents/${a.id}/modules`, { name, op: a.modules.some((x) => x.name === name) ? 'upgrade' : 'deploy' });
        toast('Scheduled', 'ok'); view.refresh();
      },
    },
  };
}

// ---------- Audit ----------
function Audit() {
  return {
    interval: 10000,
    mount(el) { el.innerHTML = r(html`<div class="page-head"><div><h1>Audit log</h1><div class="sub">Who did what, and when</div></div></div><div id="au"></div>`); },
    async refresh() {
      const rows = await GET('audit');
      setHTML($('#au'), rows.length ? html`<div class="panel"><table class="tbl"><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Target</th><th>Detail</th><th>IP</th></tr></thead>
        <tbody>${rows.map((e) => html`<tr><td class="muted" title="${dt(e.time)}">${dt(e.time)}</td><td>${e.actor}</td><td><span class="tag mono">${e.action}</span></td><td class="mono small">${e.target || ''}</td><td class="muted small">${e.detail || ''}</td><td class="dim small">${e.ip || ''}</td></tr>`)}</tbody></table></div>` : html`<div class="panel empty">No entries yet.</div>`);
    },
  };
}

// ---------- Settings ----------
function Settings() {
  return {
    async mount(el) {
      const d = await GET('settings');
      el.innerHTML = r(html`<div class="page-head"><div><h1>Settings</h1></div></div>
        ${session.initial_password ? html`<div class="notice">You are still using the generated initial password — please change it.</div>` : ''}
        <div class="grid2">
          <div class="panel panel-pad"><h2>General</h2>
            <form class="form" data-submit="save" style="margin-top:0">
              <label class="field">Automatic check interval (hours)<input type="number" name="interval" min="1" max="168" value="${d.settings.check_interval_hours}"></label>
              <label class="field">Master URL used in install commands<input type="text" name="url" placeholder="${d.suggested_url} (auto)" value="${d.settings.master_url}"></label>
              <div class="dim small">Set this if agents reach the master under a different name than your browser does.</div>
              <label class="field">GitHub repository for releases<input type="text" name="gh" placeholder="owner/name" value="${d.settings.github_repo}"></label>
              <label class="row" style="color:var(--text)"><input type="checkbox" name="auto" ${d.settings.github_auto_import ? 'checked' : ''}> Import new releases automatically <span class="dim small">(verified; never rolled out without you)</span></label>
              <div><button class="btn primary">Save</button></div>
            </form>
          </div>
          <div class="panel panel-pad"><h2>Change password</h2>
            <form class="form" data-submit="pw" style="margin-top:0">
              <label class="field">Current password<input type="password" name="cur" autocomplete="current-password" required></label>
              <label class="field">New password (min. 10 characters)<input type="password" name="new" autocomplete="new-password" minlength="10" required></label>
              <label class="field">Repeat new password<input type="password" name="new2" autocomplete="new-password" minlength="10" required></label>
              <div><button class="btn primary">Change password</button></div>
            </form>
          </div>
          <div class="panel panel-pad"><h2>Master identity</h2>
            <div class="muted small">Agents pin this public-key fingerprint. If the master certificate is ever regenerated, agents must be reinstalled.</div>
            <div class="cmdbox"><code>sha256//${d.pin}</code></div>
          </div>
        </div>`);
    },
    acts: {
      save: async (f) => {
        await POST('settings', { check_interval_hours: +f.interval.value, master_url: f.url.value, github_repo: f.gh.value, github_auto_import: f.auto.checked });
        toast('Saved', 'ok');
      },
      pw: async (f) => {
        if (f.new.value !== f.new2.value) return toast('New passwords do not match', 'err');
        await POST('password', { current: f.cur.value, new: f.new.value });
        session.initial_password = false;
        f.reset();
        toast('Password changed', 'ok');
      },
    },
  };
}

boot().catch(() => showLogin());
