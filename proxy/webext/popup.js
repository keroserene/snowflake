const port = chrome.runtime.connect({
	name: "popup"
});

port.onMessage.addListener((m) => {
	const active = m.active;
	const div = document.getElementById('active');
	const img = div.querySelector('img');
	img.src = `icons/status-${active ? "on" : "off"}.png`;
	const ps = div.querySelectorAll('p');
	const clients = active ? 1 : 0;
	ps[0].innerText = `${clients} client${(clients !== 1) ? 's' : ''} connected.`;
	ps[1].innerText = `Your snowflake has helped ${m.total} user${(m.total !== 1) ? 's' : ''} circumvent censorship in the last 24 hours.`;
});
