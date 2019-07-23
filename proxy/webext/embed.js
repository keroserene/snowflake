/* global chrome, Popup */

const port = chrome.runtime.connect({
  name: "popup"
});

port.onMessage.addListener((m) => {
  const { active, enabled, total, missingFeature } = m;
  const popup = new Popup();

  if (missingFeature) {
    popup.setEnabled(false);
    popup.setActive(false);
    popup.setStatusText("Snowflake is off");
    popup.setStatusDesc("WebRTC feature is not detected.", true);
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
  popup.setEnabled(enabled);
  popup.setActive(active);
});

document.addEventListener('change', (event) => {
  port.postMessage({ enabled: event.target.checked });
})
