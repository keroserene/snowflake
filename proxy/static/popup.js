/* exported Popup */

// Add or remove a class from elem.classList, depending on cond.
function setClass(elem, className, cond) {
  if (cond) {
    elem.classList.add(className);
  } else {
    elem.classList.remove(className);
  }
}

class Popup {
  constructor() {
    this.div = document.getElementById('active');
    this.statustext = document.getElementById('statustext');
    this.statusdesc = document.getElementById('statusdesc');
    this.img = document.getElementById('statusimg');
  }
  setEnabled(enabled) {
    setClass(this.img, 'on', enabled);
  }
  setActive(active) {
    setClass(this.img, 'running', active);
  }
  setStatusText(txt) {
    this.statustext.innerText = txt;
  }
  setStatusDesc(desc, error) {
    this.statusdesc.innerText = desc;
    setClass(this.statusdesc, 'error', error);
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
