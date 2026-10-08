'use strict';

const session = new URLSearchParams(location.hash.slice(1)).get('session') || '';
history.replaceState(null, '', '/');
const byId = (id) => document.getElementById(id);
let busy = false;
let next = 0;
let timer = null;

function buttons() {
  document.querySelectorAll('button').forEach((button) => {
    button.disabled = busy || Date.now() < next || !session;
  });
  const seconds = Math.max(0, Math.ceil((next - Date.now()) / 1000));
  byId('retry').textContent = seconds ? `Retry available in ${seconds} seconds.` : '';
}

function render(result) {
  byId('key-protection').textContent = result.key_protection;
  byId('audience').textContent = result.audience;
  byId('message').textContent = result.message;
  byId('prepare').hidden = !!result.metadata;
  byId('identity').hidden = !result.metadata;
  byId('claim').hidden = !result.can_claim;
  byId('recovery').hidden = !result.confirm_recovery;
  byId('confirm-recovery').required = !!result.confirm_recovery;
  byId('confirm-recovery').checked = false;
  if (result.metadata) {
    byId('metadata').textContent = `${result.metadata.organization} / ${result.metadata.invitation_id}`;
    byId('fingerprint').textContent = result.fingerprint;
  }
  const status = result.status;
  byId('state').textContent = status
    ? `Last verified state: ${status.state}${result.uncertain ? ' (current outcome uncertain)' : ''}. Credentials not issued.${status.approval_expires_at ? ` Approval deadline: ${status.approval_expires_at}.` : ''}`
    : 'No server status verified in this session.';
  next = Date.now() + (result.retry_after_seconds || 0) * 1000;
}

async function request(path, body) {
  if (busy) return;
  clearTimeout(timer);
  busy = true;
  buttons();
  try {
    const response = await fetch(path, {
      method: body === undefined ? 'GET' : 'POST',
      headers: { 'X-ZPR-Setup-Session': session, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: 'no-store', credentials: 'omit', redirect: 'error'
    });
    const result = await response.json();
    if (result.audience) {
      render(result);
    } else {
      byId('message').textContent = result.error || 'Local setup request failed.';
      next = Date.now() + Number(response.headers.get('Retry-After') || 2) * 1000;
    }
    if (response.ok && result.status?.state === 'pending_approval') {
      timer = setTimeout(() => request('/api/status', {}), Math.max(10000, next - Date.now()));
    }
  } catch {
    byId('message').textContent = 'Local setup connection failed. Outcome is uncertain; check fresh status or restart the command. Do not resubmit the code or replace the key.';
    byId('claim').hidden = true;
    next = Date.now() + 10000;
  } finally {
    busy = false;
    buttons();
  }
}

byId('prepare').addEventListener('submit', (event) => {
  event.preventDefault();
  request('/api/prepare', { organization: byId('organization').value, invitation_id: byId('invitation').value });
});
byId('claim').addEventListener('submit', (event) => {
  event.preventDefault();
  const code = byId('code').value;
  byId('code').value = '';
  request('/api/claim', { code, confirm_recovery: byId('confirm-recovery').checked });
});
byId('status').addEventListener('click', () => request('/api/status', {}));
setInterval(buttons, 1000);
if (session) {
  request('/api/session');
} else {
  byId('message').textContent = 'Missing setup session. Open the complete setup URL printed by the command.';
  buttons();
}
