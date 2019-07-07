/* global chrome */

const port = chrome.runtime.connect({
  name: "popup"
});

class Popup {
  constructor() {
    this.div = document.getElementById('active');
    this.ps = this.div.querySelectorAll('p');
    this.img = this.div.querySelector('img');
  }
  setImgSrc(src) {
    this.img.src = `icons/status-${src}.png`;
  }
  setStatusText(txt) {
    this.ps[0].innerText = txt;
  }
  setStatusDesc(desc, color) {
    this.ps[1].innerText = desc;
    this.ps[1].style.color = color || 'black';
  }
  hideButton() {
    document.querySelector('.button').style.display = 'none';
  }
  setChecked(checked) {
    document.getElementById('enabled').checked = checked;
  }
  setToggleText(txt) {
    document.getElementById('toggle').innerText = txt;
  }
}

port.onMessage.addListener((m) => {
  const { active, enabled, total, missingFeature } = m;
  const popup = new Popup();

  if (missingFeature) {
    popup.setImgSrc('off');
    popup.setStatusText("Snowflake is off");
    popup.setStatusDesc("WebRTC feature is not detected.", 'firebrick');
    popup.hideButton();
    return;
  }

  const clients = active ? 1 : 0;

  if (enabled) {
    popup.setChecked(true);
    popup.setToggleText('Turn Off');
    popup.setStatusText(`${clients} client${(clients !== 1) ? 's' : ''} connected.`);
    popup.setStatusDesc(`Your snowflake has helped ${total} user${(total !== 1) ? 's' : ''} circumvent censorship in the last 24 hours.`);
  } else {
    popup.setChecked(false);
    popup.setToggleText('Turn On');
    popup.setStatusText("Snowflake is off");
    popup.setStatusDesc("");
  }

  popup.setImgSrc(active ? "running" : enabled ? "on" : "off");
});

document.addEventListener('change', (event) => {
  port.postMessage({ enabled: event.target.checked });
})
