/* exported Popup */

class Popup {
  constructor() {
    this.div = document.getElementById('active');
    this.statustext = document.getElementById('statustext');
    this.statusdesc = document.getElementById('statusdesc');
    this.img = document.getElementById('statusimg');
  }
  setImgSrc(src) {
    this.img.src = `assets/status-${src}.png`;
  }
  setStatusText(txt) {
    this.statustext.innerText = txt;
  }
  setStatusDesc(desc, color) {
    this.statusdesc.innerText = desc;
    this.statusdesc.style.color = color || 'black';
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
