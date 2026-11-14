// Coremon sign-in page. The application code is loaded after signing in.
const $ = (sel) => document.querySelector(sel);

async function call(path, body) {
  const res = await fetch('/api/ui/' + path, {
    method: body ? 'POST' : 'GET',
    headers: { 'X-Coremon': '1', ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return res.json();
}

const message = (m) => (/wrong code/i.test(m) ? 'Wrong code' : /wrong/i.test(m) ? 'Wrong user name or password'
  : /too many wrong codes/i.test(m) ? 'Too many wrong codes for this account, try again in 15 minutes'
    : /too many/i.test(m) ? 'Too many attempts, try again in a few minutes' : m);

async function start() {
  let s;
  try { s = await call('session'); } catch { s = {}; }
  if (s.authenticated && s.app) {
    await import('/' + s.app);
    return;
  }
  showForm(s);
}

function showForm(s) {
  $('#app').innerHTML = `
    <div class="login"><div class="panel">
      <div class="brand"><img src="favicon.svg" alt=""><span>Coremon</span></div>
      <div class="muted">Fleet update control</div>
      <form id="login-form">
        <label class="field">User name<input type="text" name="username" autocomplete="username" autocapitalize="none" spellcheck="false" autofocus required></label>
        <label class="field">Password<input type="password" name="password" autocomplete="current-password" required></label>
        <div class="loginerr" id="login-err" role="alert"></div>
        <button class="btn primary" type="submit">Sign in</button>
      </form>
      ${s.initial_setup ? '<div class="hint">First start? Sign in as <code>admin</code>; the initial password is in <code>/var/lib/coremon-master/initial-password.txt</code> on the master.</div>' : ''}
    </div></div>`;
  const form = $('#login-form');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const err = $('#login-err');
    err.textContent = '';
    form.password.classList.remove('bad');
    try {
      const res = await call('login', { username: form.username.value, password: form.password.value });
      if (res?.code_required) return codeStep(res.ticket);
      await signedIn();
    } catch (x) {
      err.textContent = message(x.message || '');
      form.password.classList.add('bad');
      form.password.select();
    }
  });
  form.username.focus();
}

// Second step: the code from the authenticator app.
function codeStep(ticket) {
  const old = $('#login-form');
  const form = old.cloneNode(false);
  old.replaceWith(form);
  form.innerHTML = `
    <div class="muted small">Enter the 6-digit code from your authenticator app.</div>
    <label class="field">Code<input type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9 ]{6,7}" maxlength="7" required></label>
    <div class="loginerr" id="login-err" role="alert"></div>
    <button class="btn primary" type="submit">Sign in</button>
    <button class="btn ghost sm" type="button" id="login-back">Back</button>`;
  $('#login-back').addEventListener('click', () => start());
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const err = $('#login-err');
    err.textContent = '';
    form.code.classList.remove('bad');
    try {
      await call('login/code', { ticket, code: form.code.value.replace(/\s/g, '') });
      await signedIn();
    } catch (x) {
      const m = x.message || '';
      if (/expired/i.test(m)) return start();
      err.textContent = message(m);
      form.code.classList.add('bad');
      form.code.select();
    }
  });
  form.code.focus();
}

async function signedIn() {
  // Every sign-in starts on the dashboard with only its menu open.
  try { localStorage.setItem('coremon.nav', JSON.stringify(['dashboard'])); } catch {}
  if (location.hash && location.hash !== '#/') history.replaceState(null, '', location.pathname + location.search + '#/');
  await start();
}

start();
