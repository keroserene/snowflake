const port = chrome.runtime.connect({
  name: "popup"
});

port.onMessage.addListener((m) => {
  const div = document.getElementById('active');
  const ps = div.querySelectorAll('p');
  if (m.missingFeature) {
    div.querySelector('img').src = "icons/status-off.png";
    ps[0].innerText = "Snowflake is off";
    ps[1].innerText = "WebRTC feature is not detected.";
    ps[1].style.color = 'firebrick';
    document.querySelector('.toggle').style.display = 'none';
    return;
  }
  const active = m.active;
  const img = div.querySelector('img');
  const enabled = m.enabled;
  const clients = active ? 1 : 0;
  const enabledText = document.getElementById('toggle');
  if (enabled) {
    document.getElementById('enabled').checked = true;
    enabledText.innerText = 'Turn Off';
    ps[0].innerText = `${clients} client${(clients !== 1) ? 's' : ''} connected.`;
    ps[1].innerText = `Your snowflake has helped ${m.total} user${(m.total !== 1) ? 's' : ''} circumvent censorship in the last 24 hours.`;
  } else {
    ps[0].innerText = "Snowflake is off";
    ps[1].innerText = "";
    document.getElementById('enabled').checked = false;
    enabledText.innerText = 'Turn On';
  }
  img.src = `icons/status-${active? "running" : enabled? "on" : "off"}.png`;
});

document.addEventListener('change', (event) => {
  port.postMessage({enabled: event.target.checked});
})
