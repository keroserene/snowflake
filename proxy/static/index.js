class Messages {
  constructor(json) {
    this.json = json;
  }
  getMessage(m, ...rest) {
    if (this.json.hasOwnProperty(m)) {
    let message = this.json[m].message;
    return message.replace(/\$(\d+)/g, (...args) => {
      return rest[Number(args[1]) - 1];
    });
    }
  }
}


defaultLang = "en_US";

var getLang = function() {
  let lang = navigator.language || defaultLang;
  lang = lang.replace(/-/g, '_');
  if (availableLangs.has(lang)) {
    return lang;
  }
  lang = lang.split('_')[0];
  if (availableLangs.has(lang)) {
    return lang;
  }
  return defaultLang;
}

var fill = function(n, func) {
  switch(n.nodeType) {
    case 1:  // Node.ELEMENT_NODE
      const m = /^__MSG_([^_]*)__$/.exec(n.dataset.msgid);
      if (m) {
          val = func(m[1]);
          if (val != undefined) {
            n.innerHTML = val
          }
      }
      n.childNodes.forEach(c => fill(c, func));
      break;
  }
}

console.log("Fetching", `./_locales/${getLang()}/messages.json`);

fetch(`./_locales/${getLang()}/messages.json`)
  .then((res) => {
    if (!res.ok) { return; }
    return res.json();
  })
  .then((json) => {
    messages = new Messages(json);
        console.log("Filling document body");
    fill(document.body, (m) => {
        console.log("Filling ", m);
      return messages.getMessage(m);
    });
  });
