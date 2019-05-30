const port = chrome.runtime.connect({
	name: "popup"
});

port.onMessage.addListener((m) => {
	const active = m.active;
	const div = document.getElementById('active');
	const img = div.querySelector('img');
	img.src = `icons/status-${active ? "on" : "off"}.png`;
	const p = div.querySelector('p');
	p.innerText = active ? "Connected" : "Offline";
});
