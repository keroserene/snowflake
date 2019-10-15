/* global availableLangs */

class Messages {
  constructor(json) {
    this.json = json;
  }
  getMessage(m, ...rest) {
    if (Object.prototype.hasOwnProperty.call(this.json, m)) {
      let message = this.json[m].message;
      return message.replace(/\$(\d+)/g, (...args) => {
        return rest[Number(args[1]) - 1];
      });
    }
  }
}


var defaultLang = "en_US";

var getLang = function() {
  let lang = navigator.language || defaultLang;
  lang = lang.replace(/-/g, '_');

  //prioritize override language
  var url_string = window.location.href; //window.location.href
  var url = new URL(url_string);
  var override_lang = url.searchParams.get("lang");
  if (override_lang != null) {
    lang = override_lang;
  }

  if (Object.prototype.hasOwnProperty.call(availableLangs, lang)) {
    return lang;
  }
  lang = lang.split('_')[0];
  if (Object.prototype.hasOwnProperty.call(availableLangs, lang)) {
    return lang;
  }
  return defaultLang;
}

var fill = function(n, func) {
  switch(n.nodeType) {
    case 1:  // Node.ELEMENT_NODE
    {
      const m = /^__MSG_([^_]*)__$/.exec(n.dataset.msgid);
      if (m) {
        var val = func(m[1]);
        if (val != undefined) {
          n.innerHTML = val
        }
      }
      n.childNodes.forEach(c => fill(c, func));
      break;
    }
  }
}


fetch(`./_locales/${getLang()}/messages.json`)
.then((res) => {
  if (!res.ok) { return; }
  return res.json();
})
.then((json) => {
  var language = document.getElementById('language-switcher');
  var lang = `${getLang()}`
  language.innerText = availableLangs[lang].name + ' (' + lang + ')';
  var messages = new Messages(json);
  fill(document.body, (m) => {
    return messages.getMessage(m);
  });
});

// Populate language switcher list
for (var lang in availableLangs) {
  var languageList = document.getElementById('supported-languages');
  var link = document.createElement('a');
  link.setAttribute('href', '?lang='+lang);
  link.setAttribute('class', "dropdown-item");
  link.innerText = availableLangs[lang].name + ' (' + lang + ')';
  languageList.lastChild.after(link);
}
