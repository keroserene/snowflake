/* global chrome, Popup */

// Fill i18n in HTML
window.onload = () => {
  Popup.fill(document.body, (m) => {
    return chrome.i18n.getMessage(m);
  });
};

const port = chrome.runtime.connect({
  name: "popup"
});

port.onMessage.addListener((m) => {
  const { active, enabled, total, missingFeature } = m;
  const popup = new Popup();

  if (missingFeature) {
    popup.setEnabled(false);
    popup.setActive(false);
    popup.setStatusText(chrome.i18n.getMessage('popupStatusOff'));
    popup.setStatusDesc(chrome.i18n.getMessage(missingFeature), true);
    popup.hideButton();
    return;
  }

  const clients = active ? 1 : 0;

  if (enabled) {
    popup.setChecked(true);
    if (clients > 0) {
      popup.setStatusText(chrome.i18n.getMessage('popupStatusOn', String(clients)));
    } else {
      popup.setStatusText(chrome.i18n.getMessage('popupStatusReady'));
    }
    popup.setStatusDesc((total > 0) ? chrome.i18n.getMessage('popupDescOn', String(total)) : '');
  } else {
    popup.setChecked(false);
    popup.setStatusText(chrome.i18n.getMessage('popupStatusOff'));
    popup.setStatusDesc("");
  }
  popup.setEnabled(enabled);
  popup.setActive(active);
});

document.addEventListener('change', (event) => {
  port.postMessage({ enabled: event.target.checked });
})
