// Spotter UI - vanilla ES modules. Talks to Go backend via Wails runtime.

const rowsEl = document.getElementById('rows');
const detailBody = document.getElementById('detail-body');
const statusEl = document.getElementById('status');

let devices = [];

function renderList() {
  rowsEl.innerHTML = '';
  devices.forEach((d) => {
    const tr = document.createElement('tr');
    tr.dataset.id = d.DeviceID;
    tr.addEventListener('click', () => showDetail(d));
    tr.innerHTML = `
      <td>${d.IP}</td>
      <td>${d.Port}</td>
      <td>${d.Username}</td>
      <td>${d.LastInfo?.Basic?.Hostname || ''}</td>
      <td class="${d.Online ? 'online' : 'offline'}">${d.Online ? 'online' : 'offline'}</td>
    `;
    rowsEl.appendChild(tr);
  });
}

function showDetail(d) {
  detailBody.textContent = JSON.stringify(d.LastInfo || {}, null, 2);
}

async function refresh() {
  devices = await window.go.main.App.ListDevices();
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

window.runtime.EventsOn('info-updated', refresh);
window.runtime.EventsOn('offline', refresh);
window.runtime.EventsOn('unknown-device', refresh);

refresh();
