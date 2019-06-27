const port = chrome.runtime.connect({
	name: "popup"
});

port.onMessage.addListener((m) => {
	const active = m.active;
	const div = document.getElementById('active');
	const img = div.querySelector('img');
        const enabled = m.enabled
	img.src = `icons/status-${enabled ? "on" : "off"}.png`;
	const ps = div.querySelectorAll('p');
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
});

document.addEventListener('change', (event) => {
    port.postMessage({enabled: event.target.checked});
})
