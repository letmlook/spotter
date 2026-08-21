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
    tr.dataset.id = d.DeviceID;
    if (d.DeviceID === selectedId) tr.classList.add('selected');
    tr.addEventListener('click', () => select(d));
    tr.innerHTML = `
      <td>${d.IP}</td>
      <td>${d.Port}</td>
      <td>${d.Username}</td>
      <td>${d.LastInfo?.Basic?.Hostname || ''}</td>
      <td class="${d.Online ? 'online' : 'offline'}">${d.Online ? 'online' : 'offline'}</td>
    `;
    rowsEl.appendChild(tr);
  });
  uninstallBtn.disabled = !selectedId;
}

function select(d) {
  selectedId = d.DeviceID;
  showDetail(d);
  renderList();
}

function showDetail(d) {
  detailBody.textContent = JSON.stringify(d.LastInfo || {}, null, 2);
}

async function refresh() {
  devices = await window.go.main.App.ListDevices();
  // If selected device was removed, drop the selection.
  if (selectedId && !devices.some((x) => x.DeviceID === selectedId)) {
    selectedId = '';
    detailBody.textContent = '';
  }
  renderList();
  statusEl.textContent = `${devices.length} device(s)`;
}

document.getElementById('refresh').addEventListener('click', refresh);
document.getElementById('scan-subnet').addEventListener('click', async () => {
  const cidr = prompt('Enter CIDR (e.g. 192.168.1.0/24):');
  if (!cidr) return;
  await window.go.main.App.ScanSubnet(cidr);
  setTimeout(refresh, 2000);
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
  const dev = devices.find((x) => x.DeviceID === selectedId);
  if (!dev) return;
  if (!confirm(`Uninstall spotterd from ${dev.IP}? You will need to supply the SSH password.`)) {
    return;
  }
  const password = prompt('SSH password (not persisted):');
  if (!password) return;
  statusEl.textContent = `Uninstalling ${dev.IP}…`;
  try {
    await window.go.main.App.UninstallDevice(selectedId, password);
    statusEl.textContent = `Uninstalled ${dev.IP}`;
    selectedId = '';
    await refresh();
  } catch (err) {
    statusEl.textContent = `Uninstall failed: ${err}`;
  }
});

window.runtime.EventsOn('info-updated', refresh);
window.runtime.EventsOn('offline', refresh);
window.runtime.EventsOn('unknown-device', refresh);

refresh();
