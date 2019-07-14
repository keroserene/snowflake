/* exported Popup */

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
