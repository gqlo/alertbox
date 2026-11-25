package server

import "html/template"

const headerIntroHTML = `<p class="subtitle">OpenShift Virtualization Performance and Scale</p>
<p class="about"><strong>What it is:</strong> Alertbox is an alert triage dashboard with <strong>persistent alert history</strong> for OpenShift and Kubernetes clusters — not just what Alertmanager shows right now.</p>
<p class="does"><strong>What it does:</strong> Ingests firing and resolved alerts from Alertmanager (poll or webhook) and <strong>stores them in Redis</strong> so incidents survive restarts and stay searchable for future debugging. Filter past fires by severity, namespace, status, and time; cluster version info in the sidebar.</p>
<p class="repo">Source: <a href="https://github.com/gqlo/alertbox" target="_blank" rel="noopener noreferrer">github.com/gqlo/alertbox</a></p>`

const utcClockHTML = `<div class="utc-clock" title="Current UTC time" aria-label="Current UTC time">
  <span class="utc-clock-label">UTC</span>
  <time id="utc-clock-time" class="utc-clock-time" datetime=""></time>
</div>`

func headerIntro() template.HTML {
	return template.HTML(headerIntroHTML)
}

func utcClock() template.HTML {
	return template.HTML(utcClockHTML)
}

// UTCClockRefreshJS updates the header UTC clock every second (browser UTC).
const UTCClockRefreshJS = `
(function () {
  function formatUTCNow() {
    var now = new Date();
    var y = now.getUTCFullYear();
    var m = String(now.getUTCMonth() + 1).padStart(2, '0');
    var d = String(now.getUTCDate()).padStart(2, '0');
    var h = String(now.getUTCHours()).padStart(2, '0');
    var min = String(now.getUTCMinutes()).padStart(2, '0');
    var sec = String(now.getUTCSeconds()).padStart(2, '0');
    return y + '-' + m + '-' + d + ' ' + h + ':' + min + ':' + sec;
  }
  function refreshUTCClock() {
    var el = document.getElementById('utc-clock-time');
    if (!el) return;
    el.textContent = formatUTCNow();
    el.setAttribute('datetime', new Date().toISOString());
  }
  refreshUTCClock();
  setInterval(refreshUTCClock, 1000);
})();
`

// RelativeTimeRefreshJS updates <time class="relative-time"> labels in the browser.
const RelativeTimeRefreshJS = `
(function () {
  function pluralAgo(n, unit) {
    if (n < 1) n = 1;
    return n === 1 ? '1 ' + unit + ' ago' : n + ' ' + unit + 's ago';
  }
  function formatRelative(ms) {
    if (ms < 0) ms = 0;
    var sec = Math.floor(ms / 1000);
    var min = Math.floor(ms / 60000);
    var hr = Math.floor(ms / 3600000);
    var day = Math.floor(ms / 86400000);
    if (sec < 60) return pluralAgo(sec, 'second');
    if (min < 120) return pluralAgo(min, 'minute');
    if (hr < 24) return pluralAgo(hr, 'hour');
    if (day < 7) return pluralAgo(day, 'day');
    if (day < 30) return pluralAgo(Math.floor(day / 7), 'week');
    if (day < 365) return pluralAgo(Math.floor(day / 30), 'month');
    return pluralAgo(Math.floor(day / 365), 'year');
  }
  function refreshRelativeTimes() {
    document.querySelectorAll('time.relative-time[datetime]').forEach(function (el) {
      var t = new Date(el.getAttribute('datetime'));
      if (isNaN(t.getTime())) return;
      el.textContent = formatRelative(Date.now() - t.getTime());
    });
  }
  refreshRelativeTimes();
  setInterval(refreshRelativeTimes, 30000);
})();
`
