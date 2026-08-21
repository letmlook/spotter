// Spotter UI - vanilla ES modules. Talks to Go backend via Wails runtime.

const rowsEl = document.getElementById('rows');
const detailBody = document.getElementById('detail-body');
const statusEl = document.getElementById('status');
const uninstallBtn = document.getElementById('uninstall');

let devices = [];
let selectedId = '';

function renderList() {
  rowsEl.innerHTML = '';
  devices.forEach((d) => {
    const tr = document.createElement('tr');
    tr.dataset.id = d.device_id;
    if (d.device_id === selectedId) tr.classList.add('selected');
    tr.addEventListener('click', () => select(d));
    tr.innerHTML = `
      <td>${d.ip}</td>
      <td>${d.port}</td>
      <td>${d.username}</td>
      <td>${d.last_info?.basic?.hostname || ''}</td>
      <td class="${d.online ? 'online' : 'offline'}">${d.online ? 'online' : 'offline'}</td>
    `;
    rowsEl.appendChild(tr);
  });
  uninstallBtn.disabled = !selectedId;
}

function select(d) {
  selectedId = d.device_id;
  showDetail(d);
  renderList();
}

function showDetail(d) {
  detailBody.replaceChildren(buildDeviceDetail(d));
}

function showPlaceholder(text) {
  detailBody.replaceChildren(el('div', {class: 'detail-empty'}, text));
}

function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') node.className = v;
    else node.setAttribute(k, v);
  }
  for (const c of children) {
    if (c == null || c === '') continue;
    node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return node;
}

function section(title, body) {
  return el('section', {class: 'detail-section'},
    el('h3', {}, title),
    el('div', {class: 'body'}, body),
  );
}

function kvGrid(pairs) {
  const dl = el('dl', {class: 'detail-grid'});
  for (const [k, v] of pairs) {
    if (v == null || v === '') continue;
    dl.appendChild(el('dt', {}, k));
    dl.appendChild(el('dd', {}, String(v)));
  }
  return dl;
}

function networkTable(ifaces, primaryIP) {
  const root = el('div');
  if (primaryIP) {
    root.appendChild(kvGrid([['Primary IP', primaryIP]]));
  }
  if (!ifaces || ifaces.length === 0) {
    root.appendChild(el('div', {class: 'detail-empty'}, 'No network interfaces reported'));
    return root;
  }
  const tbl = el('table');
  tbl.appendChild(el('thead', {}, el('tr', {},
    el('th', {}, 'Interface'),
    el('th', {}, 'MAC'),
    el('th', {}, 'Addresses'),
  )));
  const tbody = el('tbody');
  for (const i of ifaces) {
    tbody.appendChild(el('tr', {},
      el('td', {}, i.name || '—'),
      el('td', {}, i.mac || '—'),
      el('td', {}, (i.addrs || []).join(', ') || '—'),
    ));
  }
  tbl.appendChild(tbody);
  root.appendChild(tbl);
  return root;
}

function jetsonBlock(j) {
  if (!j) {
    return el('div', {class: 'detail-empty'}, 'Not a Jetson device or probe failed');
  }
  const fields = [
    ['Model', j.model],
    ['JetPack', j.jetpack],
    ['L4T', j.l4t],
    ['CUDA', j.cuda],
    ['cuDNN', j.cudnn],
    ['TensorRT', j.tensorrt],
    ['Python', j.python],
    ['Serial', j.serial],
  ];
  const grid = kvGrid(fields);
  if (grid.childElementCount === 0) {
    return el('div', {class: 'detail-empty'}, 'No Jetson probes succeeded');
  }
  return grid;
}

function buildDeviceDetail(d) {
  const info = d.last_info;
  if (!info) {
    const reason = d.online ? 'Polling…' : 'No info yet (device offline or not yet polled)';
    return el('div', {class: 'detail-empty'}, reason);
  }

  const b = info.basic || {};
  const os = b.os || {};
  const net = info.network || {};

  const header = el('h2', {},
    b.hostname || '(no hostname)',
    ' ',
    el('span', {class: d.online ? 'online' : 'offline'}, d.online ? 'online' : 'offline'),
  );

  const basicKv = [
    ['Hostname', b.hostname],
    ['Username', b.username],
    ['OS', os.pretty_name],
    ['Distribution', os.id ? `${os.id} ${os.version_id || ''}`.trim() : ''],
    ['Kernel', b.kernel],
    ['Arch', b.arch],
    ['Uptime', formatUptime(b.uptime_seconds)],
    ['Collected at', info.collected_at],
    ['Agent version', info.agent_version],
    ['Device ID', info.device_id],
  ];

  return el('div', {},
    header,
    section('Basic', kvGrid(basicKv)),
    section('Network', networkTable(net.interfaces, net.primary_ip)),
    section('Jetson', jetsonBlock(info.jetson)),
  );
}

function formatUptime(seconds) {
  if (seconds == null) return null;
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

async function refresh() {
  devices = await window.go.main.App.ListDevices();
  if (selectedId && !devices.some((x) => x.device_id === selectedId)) {
    selectedId = '';
    showPlaceholder('Select a device from the list');
  }
  if (!selectedId) {
    showPlaceholder(devices.length === 0
      ? 'No devices yet. Use the toolbar to deploy, scan a subnet, or add by IP.'
      : 'Select a device from the list');
  }
  renderList();
  statusEl.textContent = `${devices.length} device(s)`;
}

document.getElementById('refresh').addEventListener('click', refresh);
document.getElementById('clear').addEventListener('click', async () => {
  const n = devices.length;
  if (n === 0) {
    statusEl.textContent = 'Registry is already empty';
    return;
  }
  if (!confirm(`Remove all ${n} device(s) from the local registry?\n\nThis does NOT touch the remote devices — use Uninstall to also remove spotterd.`)) {
    return;
  }
  try {
    const removed = await window.go.main.App.ClearRegistry();
    statusEl.textContent = `Cleared ${removed} device(s) from registry`;
    selectedId = '';
    await refresh();
  } catch (err) {
    statusEl.textContent = `Clear failed: ${err}`;
  }
});
document.getElementById('scan-subnet').addEventListener('click', async () => {
  const cidr = prompt('Enter CIDR (e.g. 192.168.1.0/24):');
  if (!cidr) return;
  await window.go.main.App.ScanSubnet(cidr);
  setTimeout(refresh, 2000);
});
document.getElementById('add-known').addEventListener('click', async () => {
  const ip = prompt('Device IP (e.g. 10.10.9.165):');
  if (!ip) return;
  const portStr = prompt('HTTP port (default 9999):', '9999');
  if (portStr === null) return;
  const port = parseInt(portStr, 10) || 9999;
  const username = prompt('SSH username for this device:', 'fitow') || '';
  statusEl.textContent = `Probing ${ip}:${port}…`;
  try {
    const entry = await window.go.main.App.ProbeByIP(ip, port, username);
    statusEl.textContent = `Added ${entry.device_id} (${entry.last_info?.basic?.hostname || ip})`;
    await refresh();
  } catch (err) {
    statusEl.textContent = `Probe failed: ${err}`;
  }
});

document.getElementById('deploy').addEventListener('click', async () => {
  const ip = prompt('Device IP (e.g. 10.0.5.23):');
  if (!ip) return;
  const portStr = prompt('SSH port (default 22):', '22');
  if (portStr === null) return;
  const port = parseInt(portStr, 10) || 22;
  const password = prompt('SSH password (not persisted):');
  if (!password) return;
  statusEl.textContent = `Deploying to ${ip}…`;
  try {
    const id = await window.go.main.App.DeployDevice(ip, port, password);
    statusEl.textContent = `Deployed ${id} to ${ip}`;
    await refresh();
  } catch (err) {
    statusEl.textContent = `Deploy failed: ${err}`;
  }
});

uninstallBtn.addEventListener('click', async () => {
  if (!selectedId) return;
  const dev = devices.find((x) => x.device_id === selectedId);
  if (!dev) return;
  if (!confirm(`Uninstall spotterd from ${dev.ip}? You will need to supply the SSH password.`)) {
    return;
  }
  let username = dev.username;
  if (!username) {
    username = prompt(`SSH username for ${dev.ip}:`, 'fitow') || '';
    if (!username) return;
  }
  const password = prompt('SSH password (not persisted):');
  if (!password) return;
  statusEl.textContent = `Uninstalling ${dev.ip}…`;
  try {
    await window.go.main.App.UninstallDevice(selectedId, username, password);
    statusEl.textContent = `Uninstalled ${dev.ip}`;
    selectedId = '';
    await refresh();
  } catch (err) {
    statusEl.textContent = `Uninstall failed: ${err}`;
  }
});

window.runtime.EventsOn('info-updated', refresh);
window.runtime.EventsOn('offline', refresh);
window.runtime.EventsOn('unknown-device', (event) => {
  // Wails serialises Go structs to JSON using the struct's json tags
  // (snake_case), so payload fields are lowercase.
  const data = typeof event === 'string' ? JSON.parse(event) : event;
  const info = data?.Info || data?.info || {};
  const ip = data?.IP || data?.ip || info?.network?.primary_ip || '';
  const port = data?.Port || data?.port || 9999;
  const deviceID = info?.device_id || '';
  if (!deviceID) return; // nothing to track
  // Silently add to registry — the next 30s poll cycle will populate
  // last_info and flip online=true. No user prompt: the device
  // announced itself, so the user wants it tracked.
  window.go.main.App.AcceptUnknownDevice(deviceID, ip, port, '')
    .then(() => { statusEl.textContent = `Discovered ${ip}:${port} (tracking)`; refresh(); })
    .catch((err) => { statusEl.textContent = `Accept failed: ${err}`; });
});

refresh();
