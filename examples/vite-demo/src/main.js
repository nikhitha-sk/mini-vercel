import './style.css'

document.getElementById('dpl').textContent = location.hostname.split('.')[0]
document.getElementById('built').textContent = new Date(__BUILD_TIME__).toLocaleString()
document.getElementById('route').textContent = location.pathname
