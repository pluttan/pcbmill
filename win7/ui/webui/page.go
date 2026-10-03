package webui

// ==========================
// ===  The page          ===
// ==========================

// pageHTML is the whole client: a canvas that draws frames from the
// /frame long-poll and posts input to /ev. {{TITLE}} is filled per request.
// Add ?stats to the URL to see frames per second and latency.
const pageHTML = `<!doctype html>
<html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{TITLE}}</title>
<style>
html,body{margin:0;padding:0;height:100%;overflow:hidden;background:#1e1e2e}
canvas{display:block;position:fixed;left:0;top:0;outline:none;touch-action:none;user-select:none}
#note{position:fixed;left:0;right:0;top:0;padding:6px 10px;font:13px sans-serif;
  background:#f9e2af;color:#1e1e2e;display:none;cursor:pointer;text-align:center}
#stats{position:fixed;right:4px;bottom:4px;font:12px monospace;color:#a6e3a1;
  background:rgba(0,0,0,.6);padding:2px 6px;display:none;pointer-events:none}
</style></head>
<body><canvas id="c" tabindex="0"></canvas><div id="note"></div><div id="stats"></div>
<script>
'use strict';
const cv = document.getElementById('c');
const ctx = cv.getContext('2d', {alpha: false});
const note = document.getElementById('note');
const statsEl = document.getElementById('stats');
const showStats = location.search.indexOf('stats') >= 0;
const id = Math.random().toString(36).slice(2) + Date.now().toString(36);
let dpr = window.devicePixelRatio || 1;
let seq = 0, st = -1, gen = 0, ctrl = null, passive = false;

// ---- size: the backing store is in device pixels, so frames are crisp
function backing() {
  dpr = window.devicePixelRatio || 1;
  return [Math.max(1, Math.round(innerWidth * dpr)), Math.max(1, Math.round(innerHeight * dpr))];
}
function setSize(w, h) {
  let keep = null;
  if (cv.width > 1 && cv.height > 1) {
    // a resize wipes the canvas; show the old picture until the new frame
    keep = document.createElement('canvas');
    keep.width = cv.width; keep.height = cv.height;
    keep.getContext('2d').drawImage(cv, 0, 0);
  }
  cv.width = w; cv.height = h;
  cv.style.width = innerWidth + 'px'; cv.style.height = innerHeight + 'px';
  ctx.fillStyle = '#1e1e2e'; ctx.fillRect(0, 0, w, h);
  if (keep) ctx.drawImage(keep, 0, 0);
}
function onResize() {
  const [w, h] = backing();
  if (w === cv.width && h === cv.height) return;
  setSize(w, h);
  gen++; seq = 0;            // partial frames of the old size are useless
  if (ctrl) ctrl.abort();
  push({t: 'resize', w: w, h: h, s: dpr});
}

// ---- input: one POST at a time keeps the order; moves and wheel are
// merged per animation frame
let queue = [], sending = false, move = null, wheel = null, rafOn = false;
function flushMerged() {
  if (move) { queue.push(move); move = null; }
  if (wheel) { queue.push(wheel); wheel = null; }
}
function push(e) { flushMerged(); queue.push(e); send(); }
function later() {
  if (rafOn) return;
  rafOn = true;
  requestAnimationFrame(() => { rafOn = false; flushMerged(); send(); });
}
async function send() {
  if (sending || !queue.length) return;
  if (passive) { queue = queue.filter(e => e.t === 'hello'); if (!queue.length) return; }
  sending = true;
  const batch = queue; queue = [];
  try {
    const r = await fetch('/ev', {method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: id, ev: batch})});
    if (r.status === 409) setPassive(true);
  } catch (e) {}
  sending = false;
  if (queue.length) send();
}
function hello() {
  passive = false; note.style.display = 'none';
  const [w, h] = backing();
  if (w !== cv.width || h !== cv.height) setSize(w, h);
  push({t: 'hello', w: w, h: h, s: dpr, f: document.hasFocus()});
}
function setPassive(p) {
  if (p === passive) return;
  passive = p;
  note.textContent = 'The window is controlled from another tab. Click here to take it over.';
  note.style.display = p ? 'block' : 'none';
}
note.onclick = () => hello();

function pos(e) {
  const r = cv.getBoundingClientRect();
  return [(e.clientX - r.left) * dpr, (e.clientY - r.top) * dpr];
}
function mods(e) {
  const altgr = e.getModifierState && e.getModifierState('AltGraph');
  const ctl = (e.ctrlKey || e.metaKey) && !altgr;
  return (e.shiftKey ? 1 : 0) | (ctl ? 2 : 0) | (e.altKey && !altgr ? 4 : 0);
}
window.addEventListener('keydown', e => {
  if (e.isComposing) return;
  const m = mods(e);
  // browser shortcuts (Ctrl/Cmd) stay with the browser; plain keys are ours
  if (!(m & 2)) e.preventDefault();
  push({t: 'key', k: e.key, c: e.code, m: m});
}, true);
cv.addEventListener('mousedown', e => {
  cv.focus();
  e.preventDefault();
  const [x, y] = pos(e);
  push({t: 'down', b: e.button, x: x, y: y, m: mods(e)});
});
// on window: a drag that leaves the page still reports moves and the release
window.addEventListener('mouseup', e => {
  const [x, y] = pos(e);
  push({t: 'up', b: e.button, x: x, y: y});
});
window.addEventListener('mousemove', e => {
  const [x, y] = pos(e);
  move = {t: 'move', x: x, y: y};
  later();
});
document.documentElement.addEventListener('mouseleave', () => push({t: 'leave'}));
cv.addEventListener('wheel', e => {
  e.preventDefault();
  let d = -e.deltaY;
  if (e.deltaMode === 1) d /= 3;            // lines: 3 per notch
  else if (e.deltaMode === 2) d *= 1;       // pages
  else d /= (Math.abs(e.deltaY) % 120 === 0 ? 120 : 100);
  const [x, y] = pos(e);
  if (wheel) { wheel.d += d; wheel.x = x; wheel.y = y; }
  else wheel = {t: 'wheel', d: d, x: x, y: y};
  later();
}, {passive: false});
cv.addEventListener('contextmenu', e => e.preventDefault());
cv.addEventListener('dragstart', e => e.preventDefault());
window.addEventListener('focus', () => push({t: 'focus', f: true}));
window.addEventListener('blur', () => push({t: 'focus', f: false}));
window.addEventListener('resize', onResize);

// ---- frames: the next long-poll is parked while this frame decodes
function applyState(r) {
  const s = r.headers.get('X-State');
  if (s !== null) st = +s;
  const c = r.headers.get('X-Cursor');
  if (c && cv.style.cursor !== c) cv.style.cursor = c;
  const t = r.headers.get('X-Title');
  if (t !== null) { const d = decodeURIComponent(t); if (document.title !== d) document.title = d; }
  const a = r.headers.get('X-Active');
  if (a === '0') setPassive(true);
}
async function getFrame() {
  const myGen = gen;
  ctrl = new AbortController();
  const t0 = performance.now();
  const r = await fetch('/frame?seq=' + seq + '&st=' + st + '&id=' + id,
    {cache: 'no-store', signal: ctrl.signal});
  const tH = performance.now();
  applyState(r);
  if (r.status !== 200 || myGen !== gen) return null;
  seq = +r.headers.get('X-Seq');
  const rect = r.headers.get('X-Rect').split(',').map(Number);
  const full = r.headers.get('X-Full') === '1';
  const blob = await r.blob();
  return {gen: myGen, rect: rect, full: full, blob: blob, t0: t0, tH: tH,
    tB: performance.now(), enc: +r.headers.get('X-Encode-Ms'), bytes: blob.size};
}
// stats: time to headers (server wait + encode), body, decode+draw
let nFrames = 0, statT = performance.now(), sum = [0, 0, 0, 0, 0];
async function draw(f) {
  const tD = performance.now();
  const bm = await createImageBitmap(f.blob);
  if (f.gen === gen || f.full) ctx.drawImage(bm, f.rect[0], f.rect[1]);
  bm.close();
  if (!showStats) return;
  const now = performance.now();
  nFrames++;
  const add = [f.tH - f.t0, f.tB - f.tH, now - tD, f.enc, f.bytes / 1024];
  for (let i = 0; i < 5; i++) sum[i] += add[i];
  if (now - statT >= 1000) {
    const a = sum.map(v => (v / nFrames).toFixed(1));
    const s = (nFrames * 1000 / (now - statT)).toFixed(1) + ' fps | headers ' + a[0] +
      ' ms (encode ' + a[3] + ') | body ' + a[1] + ' ms | decode+draw ' + a[2] + ' ms | ' + a[4] + ' KB';
    statsEl.textContent = s; statsEl.style.display = 'block';
    console.log('webui stats: ' + s);
    nFrames = 0; sum = [0, 0, 0, 0, 0]; statT = now;
  }
}
const sleep = ms => new Promise(r => setTimeout(r, ms));
// helloNow registers this page before polling, so the first answer
// already counts it as the active one.
async function helloNow() {
  passive = false; note.style.display = 'none';
  const [w, h] = backing();
  if (w !== cv.width || h !== cv.height) setSize(w, h);
  const r = await fetch('/ev', {method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({id: id, ev: [{t: 'hello', w: w, h: h, s: dpr, f: document.hasFocus()}]})});
  if (!r.ok) throw new Error('hello ' + r.status);
}
async function loop() {
  let lost = true, next = null;
  for (;;) {
    let f = null;
    try {
      if (lost) { await helloNow(); lost = false; }
      f = await (next || getFrame());
    } catch (e) {
      next = null;
      if (e.name !== 'AbortError') {
        // the program is gone or restarting: retry, then start from scratch
        if (showStats) console.log('webui frame error: ' + e);
        lost = true; seq = 0;
        await sleep(1000);
      }
      continue;
    }
    next = null;
    if (!f) continue;
    next = getFrame();     // parked on the server while this one decodes
    next.catch(() => {});
    try { await draw(f); } catch (e) {}
  }
}

loop();
</script></body></html>
`
