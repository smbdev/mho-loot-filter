'use strict';

const $ = (sel) => document.querySelector(sel);
const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};

const RARITIES = [
  ['Common', 'white'], ['Uncommon', 'green'], ['Rare', 'blue'], ['Epic', 'purple'], ['Cosmic', 'pink'], ['Unique', 'orange'],
];
// Name colours: the game's own text styles that set only a colour (keys of the item database's name styles).
const NAME_COLOURS = [['', 'Default'], ['white', 'White', '#FFFFFF'], ['grey', 'Grey', '#CCCCCC'], ['red', 'Red', '#EE2222'],
  ['orange', 'Orange', '#FF9900'], ['yellow', 'Yellow', '#FCDB1C'], ['green', 'Green', '#00FF00'], ['cyan', 'Cyan', '#00FFFF'],
  ['blue', 'Blue', '#1E90FF'], ['purple', 'Purple', '#CC00FF'], ['pink', 'Pink', '#FF34B3']];
const colourName = (key) => (NAME_COLOURS.find(([k]) => k === key) || SAFE_COLOURS.find(([v]) => v === key) || [key, key])[1];
const keepItem = (f) => f.hide || f.name || f.sound || !!f.color;

// Colour-blind safe colours (Okabe and Ito), stored as the hex colour they write.
const SAFE_COLOURS = [['#E69F00', 'Orange'], ['#56B4E9', 'Sky blue'], ['#009E73', 'Bluish green'], ['#F0E442', 'Yellow'],
  ['#0072B2', 'Blue'], ['#D55E00', 'Vermillion'], ['#CC79A7', 'Reddish purple']];
const colourHex = (c) => (NAME_COLOURS.find(([k]) => k === c) || [])[2] || (/^#[0-9a-f]{6}$/i.test(c || '') ? c.toUpperCase() : '');

// colourControl is a menu of name colours: the default, the game's colours, colour-blind safe ones, and a custom
// colour from the system colour picker. mixed shows a group whose items have different colours.
function colourControl(current, mixed, onPick, focus) {
  const option = (value, text, hex) => el('option', { value, textContent: text, selected: !mixed && value === current, style: hex ? `color:${hex}` : '' });
  const known = NAME_COLOURS.some(([k]) => k === current) || SAFE_COLOURS.some(([v]) => v === current);
  const select = el('select', { className: 'tweak-select colour-select', ariaLabel: 'Drop colour', title: 'Colour of the item\'s name when it drops' },
    ...(mixed ? [el('option', { value: '*', textContent: 'Mixed colours', selected: true, disabled: true })] : []),
    option('', 'Drop colour: Default'),
    el('optgroup', { label: 'Colours' }, ...NAME_COLOURS.slice(1).map(([k, label, hex]) => option(k, 'Drop colour: ' + label, hex))),
    el('optgroup', { label: 'Colour-blind safe' }, ...SAFE_COLOURS.map(([v, label]) => option(v, 'Drop colour: ' + label, v))),
    ...(!known && colourHex(current) ? [option(current, 'Drop colour: ' + colourHex(current), colourHex(current))] : []),
    el('option', { value: '+', textContent: 'Custom colour...' }));
  const hex = colourHex(current);
  if (hex && !mixed) select.style.color = hex;
  if (focus) select.dataset.focus = focus;
  select.addEventListener('change', guarded(async () => {
    if (select.value !== '+') return onPick(select.value);
    select.value = mixed ? '*' : current; // the menu shows the colour in use until one is picked
    openColourPicker(hex || '#FFFFFF', onPick);
  }));
  return select;
}

// The custom colour window: a colour square and a hue slider, a hex box and a preview. It can be dragged by its
// title, stays open while the rest of the app is used, and closes with its X or once a colour is used.
let colourPicker = null;

function hsvToHex(h, sv, v) {
  const f = (n) => { const k = (n + h / 60) % 6; return Math.round(255 * (v - v * sv * Math.max(0, Math.min(k, 4 - k, 1)))); };
  return '#' + [f(5), f(3), f(1)].map((c) => c.toString(16).padStart(2, '0')).join('').toUpperCase();
}

function hexToHsv(hex) {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const max = Math.max(r, g, b), d = max - Math.min(r, g, b);
  let h = 0;
  if (d) h = 60 * (max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4);
  return { h: (h + 360) % 360, s: max ? d / max : 0, v: max };
}

const hexToRgb = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
const rgbToHex = (rgb) => '#' + rgb.map((c) => Math.round(c).toString(16).padStart(2, '0')).join('').toUpperCase();

function rgbToHsl([r, g, b]) {
  [r, g, b] = [r / 255, g / 255, b / 255];
  const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min;
  const s = d ? d / (1 - Math.abs(2 * l - 1)) : 0;
  const { h } = hexToHsv(rgbToHex([r * 255, g * 255, b * 255]));
  return [h, s * 100, l * 100];
}

function hslToRgb([h, s, l]) {
  s /= 100;
  l /= 100;
  const a = s * Math.min(l, 1 - l);
  const f = (n) => { const k = (n + h / 30) % 12; return 255 * (l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))); };
  return [f(0), f(8), f(4)];
}

let colourMode = 'hex'; // the picker's value boxes: hex, rgb or hsl, kept while the app runs

function openColourPicker(start, onPick) {
  if (colourPicker) colourPicker.remove();
  let { h, s: sat, v } = hexToHsv(start);
  const square = el('canvas', { className: 'cp-square', width: 240, height: 150, ariaLabel: 'Colour' });
  const hue = el('input', { className: 'cp-hue', type: 'range', min: 0, max: 359, value: Math.round(h), ariaLabel: 'Hue' });
  const hexBox = el('input', { className: 'cp-hex', type: 'text', maxLength: 7, spellcheck: false, ariaLabel: 'Hex colour' });
  const number = (label, max) => el('input', { className: 'cp-num', type: 'number', min: 0, max, step: 1, ariaLabel: label, title: label });
  const rgbBoxes = [number('Red', 255), number('Green', 255), number('Blue', 255)];
  const hslBoxes = [number('Hue', 359), number('Saturation', 100), number('Lightness', 100)];
  const labelled = (boxes, letters) => el('div', { className: 'cp-fields' },
    ...boxes.map((b, i) => el('label', { className: 'cp-field' }, el('span', { textContent: letters[i] }), b)));
  const fields = { hex: el('div', { className: 'cp-fields' }, hexBox), rgb: labelled(rgbBoxes, 'RGB'), hsl: labelled(hslBoxes, 'HSL') };
  const modes = el('div', { className: 'cp-modes', role: 'radiogroup', ariaLabel: 'Colour format' });
  const showMode = () => {
    for (const [m, box] of Object.entries(fields)) box.hidden = m !== colourMode;
    for (const b of modes.children) b.setAttribute('aria-checked', String(b.dataset.mode === colourMode));
  };
  for (const [m, label] of [['hex', 'Hex'], ['rgb', 'RGB'], ['hsl', 'HSL']]) {
    const b = el('button', { type: 'button', role: 'radio', textContent: label });
    b.dataset.mode = m;
    b.addEventListener('click', () => { colourMode = m; showMode(); draw(); });
    modes.append(b);
  }
  const preview = el('span', { className: 'cp-preview' });
  const use = el('button', { className: 'primary cp-use', type: 'button', textContent: 'Use colour' });
  const close = el('button', { className: 'cp-close', type: 'button', ariaLabel: 'Close', title: 'Close', textContent: '\u00d7' });
  const title = el('div', { className: 'cp-title' }, el('span', { textContent: 'Custom colour' }), close);
  const win = el('div', { className: 'colour-picker', role: 'dialog', ariaLabel: 'Custom colour' },
    title, square, hue, modes, el('div', { className: 'cp-row' }, preview, fields.hex, fields.rgb, fields.hsl), use);
  const draw = () => {
    const ctx = square.getContext('2d');
    ctx.fillStyle = hsvToHex(h, 1, 1);
    ctx.fillRect(0, 0, square.width, square.height);
    const white = ctx.createLinearGradient(0, 0, square.width, 0);
    white.addColorStop(0, '#fff');
    white.addColorStop(1, 'rgba(255,255,255,0)');
    ctx.fillStyle = white;
    ctx.fillRect(0, 0, square.width, square.height);
    const black = ctx.createLinearGradient(0, 0, 0, square.height);
    black.addColorStop(0, 'rgba(0,0,0,0)');
    black.addColorStop(1, '#000');
    ctx.fillStyle = black;
    ctx.fillRect(0, 0, square.width, square.height);
    const x = sat * square.width, y = (1 - v) * square.height;
    ctx.lineWidth = 2;
    ctx.strokeStyle = v > 0.5 ? '#000' : '#fff';
    ctx.beginPath();
    ctx.arc(x, y, 6, 0, 2 * Math.PI);
    ctx.stroke();
    const colour = hsvToHex(h, sat, v);
    preview.style.background = colour;
    const typing = (box) => document.activeElement === box;
    if (!typing(hexBox)) hexBox.value = colour;
    const rgb = hexToRgb(colour), hsl = rgbToHsl(rgb);
    rgbBoxes.forEach((b, i) => { if (!typing(b)) b.value = rgb[i]; });
    hslBoxes.forEach((b, i) => { if (!typing(b)) b.value = Math.round(i === 0 ? h : hsl[i]); });
  };
  // typing a colour in any format moves the square and the slider to it
  const setColour = (hex) => { ({ h, s: sat, v } = hexToHsv(hex)); hue.value = Math.round(h); draw(); };
  const clamp = (b) => Math.min(Number(b.max), Math.max(0, Number(b.value) || 0));
  rgbBoxes.forEach((b) => b.addEventListener('input', () => setColour(rgbToHex(rgbBoxes.map(clamp)))));
  hslBoxes.forEach((b) => b.addEventListener('input', () => {
    const hsl = hslBoxes.map(clamp);
    setColour(rgbToHex(hslToRgb(hsl)));
    h = hsl[0]; // keep the typed hue, even for greys where the colour does not fix one
    hue.value = h;
    draw();
  }));
  const pickAt = (e) => {
    const r = square.getBoundingClientRect();
    sat = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    v = 1 - Math.min(1, Math.max(0, (e.clientY - r.top) / r.height));
    draw();
  };
  square.addEventListener('pointerdown', (e) => { square.setPointerCapture(e.pointerId); pickAt(e); });
  square.addEventListener('pointermove', (e) => { if (square.hasPointerCapture(e.pointerId)) pickAt(e); });
  hue.addEventListener('input', () => { h = Number(hue.value); draw(); });
  hexBox.addEventListener('input', () => {
    const t = hexBox.value.trim();
    const full = /^#?[0-9a-f]{6}$/i.test(t) ? '#' + t.replace('#', '') : '';
    if (full) setColour(full);
  });
  const shut = () => { win.remove(); colourPicker = null; };
  close.addEventListener('click', shut);
  use.addEventListener('click', guarded(async () => { const c = hsvToHex(h, sat, v); shut(); await onPick(c); }));
  // drag by the title bar, kept inside the window
  title.addEventListener('pointerdown', (e) => {
    if (e.target === close) return;
    title.setPointerCapture(e.pointerId);
    const r = win.getBoundingClientRect(), dx = e.clientX - r.left, dy = e.clientY - r.top;
    const move = (m) => {
      win.style.left = Math.max(0, Math.min(innerWidth - r.width, m.clientX - dx)) + 'px';
      win.style.top = Math.max(0, Math.min(innerHeight - r.height, m.clientY - dy)) + 'px';
    };
    const up = () => { title.removeEventListener('pointermove', move); title.removeEventListener('pointerup', up); };
    title.addEventListener('pointermove', move);
    title.addEventListener('pointerup', up);
  });
  showMode();
  document.body.append(win);
  colourPicker = win;
  win.style.left = Math.round((innerWidth - win.offsetWidth) / 2) + 'px';
  win.style.top = Math.round((innerHeight - win.offsetHeight) / 3) + 'px';
  draw();
}

const PAGES = ['search', 'groups', 'heroes', 'rarity', 'sound', 'filter', 'drops', 'achievements', 'settings', 'tweaks', 'mutes', 'backups'];
const OFF = { Glow: false, Model: false, Name: false };
const SOUND_RARITIES = ['Cosmic', 'Unique']; // the only rarities that set their own drop sound
const emptyFilter = () => ({ items: {}, looks: {}, groups: {}, rarities: {}, raritySounds: {}, rarityHide: {}, glowAll: false, glowShown: {} });
let rarityCategories = [];

let filter = emptyFilter();
let groupList = [];
let results = [];
let busy = false;
const itemCache = {};
const typeCache = {};
const openLooks = new Set();
const openGroups = new Set();
const groupMembers = {};
const groupOrder = {}; // item keys in the order shown while a group is open, so rows never move under the mouse

// In the desktop app, window.lfStart runs a request in the background and the answer arrives through
// window.lfDone; window.lfCall answers straight away and is used for the folder dialog.
const pending = new Map();
let nextCall = 0;
window.lfDone = (id, text) => {
  const resolve = pending.get(id);
  pending.delete(id);
  if (resolve) resolve(text);
};

function desktopCall(method, url, body) {
  const payload = body === undefined ? '' : JSON.stringify(body);
  if (url === '/api/pick-folder' || !window.lfStart) return window.lfCall(method, url, payload);
  return new Promise((resolve) => {
    const id = ++nextCall;
    pending.set(id, resolve);
    window.lfStart(id, method, url, payload);
  });
}

async function api(method, url, body) {
  if (window.lfCall) {
    const res = JSON.parse(await desktopCall(method, url, body));
    if (res.status >= 400) throw Object.assign(new Error((res.body && res.body.error) || `Request failed (${res.status})`), { status: res.status });
    return res.body;
  }
  let res;
  try {
    res = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new Error('The filter app is not running. Start MHOLootFilter.exe again.');
  }
  let data = {};
  try { data = await res.json(); } catch { /* error pages are not JSON */ }
  if (!res.ok) throw Object.assign(new Error(data.error || `Request failed (${res.status})`), { status: res.status });
  return data;
}

function showToast(lines, isError) {
  const toast = $('#toast');
  toast.replaceChildren(el('div', { className: 'lines' }, ...lines.map((line) => el('div', { textContent: line }))));
  toast.classList.toggle('error', isError);
  toast.hidden = false;
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => { toast.hidden = true; }, isError ? 10000 : 5000);
}

// guarded wraps an event handler so a failure is shown to the user instead of being lost.
const guarded = (fn) => async (...args) => {
  try {
    await fn(...args);
  } catch (err) {
    showToast([err.message], true);
    renderAll();
  }
};

// The page shown, as '#drops/bosses'. In a browser it is the address; in the desktop window, whose page is loaded
// from a string (about:blank), changing the address could leave every later change ignored, so it lives here.
let pageHash = location.hash;
const useAddress = () => !window.lfCall;
const currentHash = () => (useAddress() ? location.hash : pageHash);
const onNavigate = guarded(() => {
  $('#drops').scrollTop = 0; // a drop finder link opens its page at the top; the tier switch redraws without one
  $('#ach').scrollTop = 0;
  return showPage();
});

function navigate(hash) {
  const h = '#' + hash.replace(/^#/, '');
  if (useAddress()) {
    location.hash = h;
  } else if (h !== pageHash) {
    pageHash = h;
    onNavigate();
  }
}
window.addEventListener('hashchange', () => { if (useAddress()) onNavigate(); });

function showPage() {
  const [first, ...route] = currentHash().slice(1).split('/');
  const page = PAGES.includes(first) ? first : 'search';
  for (const p of PAGES) $('#page-' + p).hidden = p !== page;
  for (const link of document.querySelectorAll('#sidebar a')) {
    if (link.dataset.page === page) link.setAttribute('aria-current', 'page');
    else link.removeAttribute('aria-current');
  }
  if (page === 'search') $('#q').focus();
  if (page === 'filter') return renderFilter();
  if (page === 'settings') return loadSettings();
  if (page === 'sound') return loadSound();
  if (page === 'tweaks') return loadTweaks();
  if (page !== 'mutes' && previewing) {
    preview.pause();
    previewing = '';
  }
  if (page === 'mutes') return loadMutes();
  if (page === 'drops') return renderDrops(route.map(decodeURIComponent));
  if (page === 'achievements') return renderAchievements(route.map(decodeURIComponent));
  return undefined;
}

async function saveFilter() {
  await api('PUT', '/api/filter', filter);
  $('#dirty').hidden = false;
  renderCount();
}

function filterSize() {
  const on = (o) => Object.values(o).filter((v) => v === true || (v && (v.hide || v.name || v.sound || v.color || v.Glow || v.Model || v.Name))).length;
  return on(filter.items) + on(filter.looks) + on(filter.rarities) + on(filter.raritySounds)
    + Object.values(filter.rarityHide).filter((r) => r.length).length + (filter.glowAll ? 1 : 0);
}

function renderCount() {
  const n = filterSize();
  $('#count').textContent = n;
  $('#count').hidden = n === 0;
}

// focus names a control so it keeps the keyboard focus when its list is redrawn (see keepView).
const UNIQUE_NOTE = 'This Unique has an item class of its own that uses the standard Unique glow and drop sound, '
  + 'so they can only be changed for all such uniques at once, with the Unique row on the Rarity page.';

// pageLink opens another page of the app. It is a button, not an <a href>: the app window shows a link's address
// in a status bar at the bottom while the mouse is over it.
function pageLink(text, page, title) {
  const button = el('button', { className: 'link', type: 'button', textContent: text, title });
  button.addEventListener('click', () => { navigate(page); });
  return button;
}

function switchControl(label, checked, disabled, title, onChange, focus) {
  const input = el('input', { type: 'checkbox', checked, disabled });
  if (focus) input.dataset.focus = focus;
  input.addEventListener('change', guarded(() => onChange(input.checked)));
  return el('label', { className: 'switch' + (disabled ? ' disabled' : ''), title }, input, el('span', { className: 'track' }), label);
}

async function setItem(info, change) {
  const next = { hide: false, name: false, sound: false, ...(filter.items[info.key] || {}), ...change };
  if (info.sharedWith > 0 && change.hide === false) next.name = false; // a visible item shows its look's name
  if (next.hide) next.sound = false; // a hidden item cannot be heard either
  if (!next.color) delete next.color;
  if (keepItem(next)) filter.items[info.key] = next;
  else delete filter.items[info.key];
  itemCache[info.key] = info;
  await saveFilter();
  renderAll();
}

async function setLook(type, change) {
  const next = { ...OFF, ...(filter.looks[type] || {}), ...change };
  if (next.Glow || next.Model || next.Name) filter.looks[type] = next;
  else delete filter.looks[type];
  await saveFilter();
  renderAll();
}

// glowHidden reports whether a look's own glow is off: by itself, or by Every glow unless it was switched back.
const glowHidden = (type) => (filter.looks[type] || OFF).Glow || (filter.glowAll && !filter.glowShown[type]);

// setGlow switches a look's glow; name is the item it was switched from, to list it under Every glow.
async function setGlow(type, hide, name) {
  if (filter.glowAll) {
    if (hide) delete filter.glowShown[type];
    else filter.glowShown[type] = name;
  }
  await setLook(type, { Glow: hide && !filter.glowAll });
}

// setGlowAll switches every glow off, rarity colours included, or every one back on.
async function setGlowAll(hide) {
  filter.glowAll = hide;
  filter.glowShown = {};
  for (const [r] of RARITIES) {
    if (hide) filter.rarities[r] = true;
    else delete filter.rarities[r];
  }
  for (const [t, look] of Object.entries(filter.looks)) {
    look.Glow = false; // covered by Every glow, or switched back on with it
    if (!look.Model && !look.Name) delete filter.looks[t];
  }
  await saveFilter();
  renderAll();
}

async function typeInfo(type) {
  if (!typeCache[type]) typeCache[type] = await api('GET', '/api/type?key=' + encodeURIComponent(type));
  return typeCache[type];
}

function lookPanel(info) {
  const panel = el('div', { className: 'look', hidden: !openLooks.has(info.type) });
  const fill = async () => {
    const t = await typeInfo(info.type);
    const look = filter.looks[info.type] || OFF;
    const n = t.items.length;
    panel.replaceChildren(
      el('div', { textContent: `These ${n} items are drawn the same way in game: ${t.items.join(', ')}.` }),
      el('div', { className: 'flags' },
        info.rarityGlow
          ? pageLink('Their glow is set by rarity', 'rarity', 'Open Rarity')
          : switchControl(`Hide glow for all ${n}`, glowHidden(info.type), !info.canGlow, '', (v) => setGlow(info.type, v, info.name)),
        switchControl(`Hide name for all ${n}`, look.Name, !info.canName, '', (v) => setLook(info.type, { Name: v })),
        switchControl(`Hide all ${n}`, look.Model, false, '', (v) => setLook(info.type, { Model: v }))));
  };
  if (!panel.hidden) fill();
  return { panel, fill };
}

function itemRow(info, extra = []) {
  const f = filter.items[info.key] || { hide: false, name: false, sound: false };
  const shared = info.sharedWith > 0;
  const lookHidden = (filter.looks[info.type] || OFF).Model;

  const title = el('div', { className: 'item' },
    el('span', { className: 'name', textContent: info.name }),
    el('span', { className: 'tag', textContent: info.category }));
  if (info.detail) title.append(el('div', { className: 'sub', textContent: info.detail }));
  if (lookHidden) title.append(el('div', { className: 'sub by-group', textContent: 'Hidden with all items that look the same' }));

  const switches = [switchControl('Hide item', f.hide, false, '', (v) => setItem(info, { hide: v }), 'hide|' + info.key)];
  const nameBlocked = shared && !f.hide && !f.name;
  switches.push(switchControl('Hide name', f.name, nameBlocked || !info.canName,
    nameBlocked ? `Looks the same as ${info.sharedWith} other items: hide the item to hide its name, or use "Looks the same" below` : '',
    (v) => setItem(info, { name: v }), 'name|' + info.key));
  switches.push(info.rarityGlow
    ? pageLink(info.soundByRarity ? 'Unique glow' : 'Glow set by rarity', 'rarity', info.soundByRarity ? UNIQUE_NOTE : 'Open Rarity')
    : switchControl('Hide glow', glowHidden(info.type), !info.canGlow,
      shared ? `Also hides the glow of the ${info.sharedWith} other items that look the same` : '',
      (v) => setGlow(info.type, v, info.name), 'glow|' + info.key));
  const hidden = f.hide || lookHidden;
  switches.push(info.soundByRarity
    ? pageLink('Unique sound', 'rarity', UNIQUE_NOTE)
    : switchControl('Play sound', f.sound && !hidden, hidden, hidden ? 'Hidden items play no sound' : 'Play an alert when this item drops',
      (v) => setItem(info, { sound: v }), 'sound|' + info.key));
  switches.push(colourControl(f.color || '', false, (c) => setItem(info, { color: c }), 'colour|' + info.key));

  const active = keepItem(f) || lookHidden;
  const row = el('div', { className: 'row' + (active ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item-cell' }, iconImg(info.key), title),
    el('div', { className: 'flags item-flags' }, ...switches, ...extra));
  if (shared) {
    const { panel, fill } = lookPanel(info);
    const link = el('button', {
      className: 'link', type: 'button', textContent: `Looks the same as ${info.sharedWith} other item${info.sharedWith === 1 ? '' : 's'}`,
      title: 'These items use the same model on the ground. Hide item and Play sound still change only this item.',
    });
    link.addEventListener('click', guarded(async () => {
      if (panel.hidden) await fill();
      panel.hidden = !panel.hidden;
      if (panel.hidden) openLooks.delete(info.type);
      else openLooks.add(info.type);
    }));
    title.append(el('div', {}, link));
    row.append(panel);
  }
  return row;
}

// Item icons come from the game's own files, through /api/icons. Any list that adds item rows gets their icons
// in one request, whichever page drew them.
const iconCache = {};
const iconAsked = new Set();
let iconTimer;

function iconImg(key) {
  const img = el('img', { className: 'icon', alt: '', width: 36, height: 36 });
  img.dataset.key = key;
  if (iconCache[key]) img.src = iconCache[key];
  return img;
}

async function loadIcons() {
  const want = [...new Set([...document.querySelectorAll('img.icon:not([src])')].map((i) => i.dataset.key))]
    .filter((k) => !iconAsked.has(k));
  for (let i = 0; i < want.length; i += 500) {
    const keys = want.slice(i, i + 500);
    keys.forEach((k) => iconAsked.add(k));
    Object.assign(iconCache, await api('POST', '/api/icons', { keys }));
  }
  for (const img of document.querySelectorAll('img.icon:not([src])')) {
    if (iconCache[img.dataset.key]) img.src = iconCache[img.dataset.key];
  }
}

new MutationObserver(() => {
  clearTimeout(iconTimer);
  iconTimer = setTimeout(() => loadIcons().catch(() => {}), 30); // no icons is not worth an error message
}).observe(document.body, { childList: true, subtree: true });

function renderResults() {
  const box = $('#results');
  const query = $('#q').value.trim();
  if (query.length < 2) {
    box.replaceChildren(el('p', { className: 'empty', textContent: 'Type at least 2 letters of an item name.' }));
    return;
  }
  if (!results.length) {
    box.replaceChildren(el('p', { className: 'empty', textContent: `No items match "${query}".` }));
    return;
  }
  box.replaceChildren(...results.map((r) => itemRow(r)));
}

let searchTimer;
$('#q').addEventListener('input', () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(guarded(async () => {
    const query = $('#q').value.trim();
    results = query.length >= 2 ? await api('GET', '/api/search?q=' + encodeURIComponent(query)) : [];
    for (const r of results) itemCache[r.key] = r;
    renderResults();
  }), 150);
});

async function loadMembers(id) {
  if (!groupMembers[id]) groupMembers[id] = await api('GET', '/api/group?id=' + encodeURIComponent(id));
  for (const m of groupMembers[id]) itemCache[m.key] = m;
  return groupMembers[id];
}

// setGroup switches one setting on or off for every item of a group, so single items can then be switched back.
async function setGroup(g, flag, value) {
  for (const m of await loadMembers(g.id)) {
    const next = { hide: false, name: false, sound: false, ...(filter.items[m.key] || {}) };
    next[flag] = value;
    if (next.hide) next.sound = false;
    if (flag === 'hide' && !value && !g.namesAlone && m.sharedWith > 0) next.name = false; // a visible look keeps its name
    if (flag === 'sound' && value && next.hide) continue;
    if (!next.color) delete next.color;
    if (keepItem(next)) filter.items[m.key] = next;
    else delete filter.items[m.key];
  }
}

// groupHas reports whether every item of a group has a setting on.
const groupHas = (g, flag) => g.keys.length > 0 && g.keys.every((k) => filter.items[k] && filter.items[k][flag]);

function memberList(g) {
  const box = el('div', { className: 'members' });
  // draw shows the members once loaded; fill loads them first. Redraws use draw, so the list keeps its height and
  // its controls exist straight away (see keepView).
  const draw = () => {
    const members = groupMembers[g.id];
    const mine = (m) => !!filter.items[m.key];
    if (!groupOrder[g.id]) {
      groupOrder[g.id] = [...members].sort((a, b) => (mine(b) - mine(a)) || a.name.localeCompare(b.name)).map((m) => m.key);
    }
    const byKey = Object.fromEntries(members.map((m) => [m.key, m]));
    const sorted = groupOrder[g.id].map((k) => byKey[k]);
    const hiddenHere = members.filter((m) => filter.items[m.key] && filter.items[m.key].hide).length;
    box.replaceChildren(
      el('div', { className: 'sub', textContent: hiddenHere
        ? `${hiddenHere} of these ${members.length} items are hidden.`
        : `None of these ${members.length} items are hidden yet.` }),
      ...sorted.map((m) => itemRow(m)));
  };
  const fill = async () => {
    await loadMembers(g.id);
    draw();
  };
  return { box, fill, draw };
}

function renderGroups() {
  $('#groups').replaceChildren(...groupList.map(groupRow));
  renderHeroes();
}

// groupRow is one group with switches that act on all of its items, and a list of those items when opened.
function groupRow(g) {
  const f = { hide: groupHas(g, 'hide'), name: groupHas(g, 'name'), sound: groupHas(g, 'sound') };
  const set = async (flag, value) => {
    await setGroup(g, flag, value);
    await saveFilter();
    renderAll();
  };
  const open = openGroups.has(g.id);
  const toggle = el('button', { className: 'expand', type: 'button' },
    el('span', { className: 'chevron', textContent: open ? '\u25be' : '\u25b8' }),
    el('span', { className: 'name', textContent: g.label }),
    el('span', { className: 'count', textContent: `${g.count} items` }));
  toggle.setAttribute('aria-expanded', String(open));
  toggle.dataset.focus = 'open|' + g.id;
  toggle.addEventListener('click', guarded(async () => {
    if (openGroups.has(g.id)) {
      openGroups.delete(g.id);
      delete groupOrder[g.id]; // re-sort with hidden items first next time it opens
    } else openGroups.add(g.id);
    keepView(renderGroups);
  }));
  const some = g.keys.some((k) => filter.items[k] && keepItem(filter.items[k]));
  const colours = new Set(g.keys.map((k) => (filter.items[k] && filter.items[k].color) || ''));
  const row = el('div', { className: 'row' + (some ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item' }, toggle, el('div', { className: 'sub', textContent: g.note })),
    el('div', { className: 'flags' },
      switchControl('Hide items', f.hide, false, 'Switch on Hide item for every item here; switch single items back afterwards', (v) => set('hide', v), 'hide|' + g.id),
      switchControl('Hide names', f.name, !g.namesAlone && !f.hide,
        !g.namesAlone && !f.hide ? 'Some of these items look the same as items outside this group: hide the items to hide their names' : '',
        (v) => set('name', v), 'name|' + g.id),
      switchControl('Play sound', f.sound, f.hide, f.hide ? 'Hidden items play no sound' : 'Switch on Play sound for every item here',
        (v) => set('sound', v), 'sound|' + g.id),
      colourControl(colours.size === 1 ? [...colours][0] : '', colours.size > 1, (c) => set('color', c), 'colour|' + g.id)));
  if (!open) return row;
  const { box, fill, draw } = memberList(g);
  row.append(box);
  if (groupMembers[g.id]) draw();
  else fill().catch((err) => showToast([err.message], true));
  return row;
}

let heroList = [];
let heroPicked = '';

// renderHeroes shows the hero picker and, once a hero is picked, that hero's uniques as a group.
function renderHeroes() {
  const select = $('#hero');
  const heroes = heroList.filter((h) => !h.all);
  if (select.options.length <= 1 && heroes.length) {
    select.append(...heroes.map((h) => el('option', { value: h.id, textContent: `${h.label} (${h.count})` })));
  }
  select.value = heroPicked;
  const all = heroList.find((h) => h.all);
  $('#hero-all').replaceChildren(...(all ? [groupRow(all)] : []));
  const hero = heroes.find((h) => h.id === heroPicked);
  $('#hero-group').replaceChildren(...(hero ? [groupRow(hero)] : []));
}

$('#hero').addEventListener('change', () => {
  heroPicked = $('#hero').value;
  if (heroPicked) openGroups.add(heroPicked); // show the hero's uniques straight away, to switch single ones
  renderHeroes();
});

async function toggle(map, key, value) {
  if (value) map[key] = true;
  else delete map[key];
  await saveFilter();
  renderAll();
}

function renderGlowAll() {
  const kept = Object.keys(filter.glowShown).length;
  $('#glow-all').replaceChildren(el('div', { className: 'row' + (filter.glowAll ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item' },
      el('span', { className: 'name', textContent: 'Every glow' }),
      el('div', { className: 'sub', textContent: filter.glowAll
        ? `Every item's glow is hidden${kept ? `, except ${kept} you switched back on` : ''}. To bring back one item's glow, find it in Item search and turn off its Hide glow switch. To bring back a rarity's glow, turn off its Hide glow below.`
        : 'Hide the glow of every item and rarity, then switch back only the ones you want to see in the Item search tab.' })),
    el('div', { className: 'flags' },
      switchControl('Hide all glows', filter.glowAll, false, '', setGlowAll, 'glow|all'))));
}

function renderRarities() {
  $('#rarities').replaceChildren(...RARITIES.map(([rarity, colour]) => el('div', {
    className: 'row' + (filter.rarities[rarity] || filter.raritySounds[rarity] ? ' active' : ''), role: 'listitem',
  },
  el('div', { className: 'item' },
    el('span', { className: 'name rarity-' + rarity.toLowerCase(), textContent: rarity }),
    el('span', { className: 'count', textContent: colour + ' glow' })),
  el('div', { className: 'flags' },
    switchControl('Hide glow', !!filter.rarities[rarity], false, '', (v) => toggle(filter.rarities, rarity, v)),
    SOUND_RARITIES.includes(rarity)
      ? switchControl('Play sound', !!filter.raritySounds[rarity], false, `Play an alert when any ${rarity} item drops`,
        (v) => toggle(filter.raritySounds, rarity, v))
      : el('span', { className: 'switch spacer', textContent: 'Play sound' }))))); // keeps Hide glow in one column
}

function renderRarityGrid() {
  const head = el('tr', {}, el('th', { scope: 'col', textContent: 'Hide when it drops as' }),
    ...RARITIES.map(([r]) => el('th', { scope: 'col', className: 'rarity-' + r.toLowerCase(), textContent: r })));
  const rows = rarityCategories.map((cat) => el('tr', {}, el('th', { scope: 'row', textContent: cat }),
    ...RARITIES.map(([r]) => {
      const box = el('input', { type: 'checkbox', checked: (filter.rarityHide[cat] || []).includes(r), title: `Hide ${r} ${cat}` });
      box.setAttribute('aria-label', `Hide ${r} ${cat}`);
      box.addEventListener('change', guarded(async () => {
        const set = new Set(filter.rarityHide[cat] || []);
        if (box.checked) set.add(r);
        else set.delete(r);
        if (set.size) filter.rarityHide[cat] = RARITIES.map(([x]) => x).filter((x) => set.has(x));
        else delete filter.rarityHide[cat];
        await saveFilter();
        renderAll();
      }));
      return el('td', {}, box);
    })));
  $('#rarity-grid').replaceChildren(el('thead', {}, head), el('tbody', {}, ...rows));
}

function removeButton(label, onRemove) {
  const button = el('button', { className: 'remove', type: 'button', title: 'Remove from filter', textContent: '×' });
  button.setAttribute('aria-label', 'Remove ' + label);
  button.addEventListener('click', guarded(async () => {
    onRemove();
    await saveFilter();
    renderAll();
  }));
  return button;
}

async function renderFilter() {
  const box = $('#filter');
  if (filterSize() === 0) {
    box.replaceChildren(el('p', { className: 'empty', textContent: 'Your filter is empty. Use Item search or Item groups to choose what to hide or hear.' }));
    return;
  }
  const sections = [];
  const itemKeys = Object.keys(filter.items);
  if (itemKeys.length) {
    const settled = await Promise.allSettled(itemKeys.map(async (key) => itemCache[key] || (itemCache[key] = await api('GET', '/api/item?key=' + encodeURIComponent(key)))));
    const rows = settled.map((s, i) => {
      const key = itemKeys[i];
      const remove = removeButton(key, () => delete filter.items[key]);
      if (s.status === 'fulfilled') return itemRow(s.value, [remove]);
      return el('div', { className: 'row' }, el('div', { className: 'item' },
        el('span', { className: 'name', textContent: key.split('|').pop() }),
        el('div', { className: 'sub', textContent: 'No longer in the item database. Remove it from the filter.' })), remove);
    });
    sections.push(el('h3', { className: 'section-title', textContent: 'Items' }), ...rows);
  }
  const looks = Object.keys(filter.looks);
  if (looks.length) {
    const infos = await Promise.allSettled(looks.map(typeInfo));
    sections.push(el('h3', { className: 'section-title', textContent: 'Items that look the same' }), ...looks.map((type, i) => {
      const t = infos[i].status === 'fulfilled' ? infos[i].value : { items: [type] };
      const f = filter.looks[type];
      const what = [f.Model && 'hidden', f.Glow && 'glow hidden', f.Name && 'names hidden'].filter(Boolean).join(', ');
      const names = t.items.length > 3 ? `${t.items.slice(0, 3).join(', ')} and ${t.items.length - 3} more` : t.items.join(', ');
      return el('div', { className: 'row' }, el('div', { className: 'item' },
        el('span', { className: 'name', textContent: names }), el('div', { className: 'sub', textContent: what })),
      removeButton(names, () => delete filter.looks[type]));
    }));
  }
  const hiddenBy = Object.entries(filter.rarityHide).filter(([, r]) => r.length);
  if (hiddenBy.length) {
    sections.push(el('h3', { className: 'section-title', textContent: 'Hidden by rarity' }), ...hiddenBy.map(([cat, r]) =>
      el('div', { className: 'row' }, el('div', { className: 'item' },
        el('span', { className: 'name', textContent: cat }), el('div', { className: 'sub', textContent: r.join(', ') + ' hidden' })),
      removeButton(cat, () => delete filter.rarityHide[cat]))));
  }
  if (filter.glowAll) {
    const kept = Object.keys(filter.glowShown);
    const infos = await Promise.allSettled(kept.map(typeInfo));
    const names = kept.map((type, i) => {
      const n = infos[i].status === 'fulfilled' ? infos[i].value.items.length - 1 : 0;
      return filter.glowShown[type] + (n > 0 ? ` (and ${n} that look the same)` : '');
    });
    sections.push(el('h3', { className: 'section-title', textContent: 'Glow' }),
      el('div', { className: 'row' }, el('div', { className: 'item' },
        el('span', { className: 'name', textContent: 'Every glow hidden' }),
        el('div', { className: 'sub', textContent: names.length ? 'Except ' + names.join(', ') : 'No exceptions' })),
      removeButton('Every glow', () => { filter.glowAll = false; filter.glowShown = {}; })));
  }
  const rarities = RARITIES.filter(([r]) => filter.rarities[r] || filter.raritySounds[r]);
  if (rarities.length) {
    sections.push(el('h3', { className: 'section-title', textContent: 'Rarity' }), ...rarities.map(([r, colour]) => {
      const what = [filter.rarities[r] && colour + ' glow hidden', filter.raritySounds[r] && 'plays a sound'].filter(Boolean).join(', ');
      return el('div', { className: 'row' }, el('div', { className: 'item' },
        el('span', { className: 'name rarity-' + r.toLowerCase(), textContent: r }), el('div', { className: 'sub', textContent: what })),
      removeButton(r, () => { delete filter.rarities[r]; delete filter.raritySounds[r]; }));
    }));
  }
  box.replaceChildren(...sections);
  searchFilter();
}

// searchFilter shows only the My filter rows whose name or description has the search text, and the headings
// above them. Menu entries are left out of the match, so "red" finds red items, not every colour menu.
function searchFilter() {
  const q = $('#fq').value.trim().toLowerCase();
  const box = $('#filter');
  box.querySelector('.no-match')?.remove();
  let heading = null, any = false;
  for (const node of box.children) {
    if (node.matches('h3')) {
      heading = node;
      heading.hidden = true;
      continue;
    }
    if (!node.matches('.row')) continue;
    const text = [...node.querySelectorAll(':scope > .item-cell .name, :scope > .item-cell .sub, :scope > .item .name, :scope > .item .sub, .tag')]
      .map((e) => e.textContent).join(' ') + ' ' + [...node.querySelectorAll('select.colour-select')].filter((e) => e.value && e.value !== '*').map((e) => colourName(e.value)).join(' ');
    const match = !q || text.toLowerCase().includes(q);
    node.hidden = !match;
    if (match && heading) heading.hidden = false;
    any = any || match;
  }
  if (!any && q) box.append(el('p', { className: 'empty no-match', textContent: `Nothing in your filter matches "${$('#fq').value.trim()}".` }));
}
$('#fq').addEventListener('input', () => searchFilter());

function renderAll() {
  keepView(() => {
    renderCount();
    renderResults();
    renderGroups();
    renderGlowAll();
    renderRarities();
    renderRarityGrid();
    if (!$('#page-filter').hidden) renderFilter();
  });
}

// keepView runs a redraw without moving the page. Lists are rebuilt from scratch, which drops the keyboard focus
// with the old control (some browsers then scroll to the top), so it puts back the scroll positions and the focus.
function keepView(draw) {
  const scrolls = [document.scrollingElement, ...document.querySelectorAll('.content')].map((e) => [e, e.scrollTop]);
  const focus = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.focus;
  draw();
  for (const [e, top] of scrolls) e.scrollTop = top;
  const again = focus && [...document.querySelectorAll(`[data-focus="${CSS.escape(focus)}"]`)].find((e) => e.offsetParent !== null);
  if (again) again.focus({ preventScroll: true });
}

async function loadSettings() {
  const s = await api('GET', '/api/settings');
  if (document.activeElement !== $('#folder')) $('#folder').value = s.gameDir;
  const status = $('#folder-status');
  status.className = 'lead ' + (s.gameFound ? 'ok-text' : s.searching ? 'muted' : 'bad-text');
  status.textContent = s.gameFound ? 'Marvel Heroes Omega found in this folder.'
    : s.searching ? 'Searching this PC for Marvel Heroes Omega...' : 'Marvel Heroes Omega was not found in this folder.';
  clearTimeout(loadSettings.timer);
  if (s.searching) loadSettings.timer = setTimeout(guarded(() => !$('#page-settings').hidden && loadSettings()), 1500);
  const others = s.detected.filter((d) => d.toLowerCase() !== s.gameDir.toLowerCase());
  $('#detected').replaceChildren(...(others.length ? [el('p', { className: 'lead muted', textContent: 'Also found on this PC:' })] : []),
    ...others.map((dir) => {
      const b = el('button', { className: 'secondary', type: 'button', textContent: dir });
      b.addEventListener('click', guarded(() => useFolder(dir)));
      return b;
    }));
}

async function useFolder(dir) {
  try {
    await api('PUT', '/api/settings', { gameDir: dir });
  } catch (err) {
    $('#folder-status').className = 'lead bad-text';
    $('#folder-status').textContent = err.message;
    throw err;
  }
  iconAsked.clear(); // the icons come from the game folder
  showToast(['Using ' + dir], false);
  await loadSettings();
  refreshStatus();
}

$('#use-folder').addEventListener('click', guarded(() => useFolder($('#folder').value.trim())));
$('#browse').addEventListener('click', guarded(async () => {
  try {
    const r = await api('POST', '/api/pick-folder');
    if (r.path) {
      $('#folder').value = r.path;
      await useFolder(r.path);
    }
  } catch (err) {
    if (err.status === 501) showToast(['Type or paste the folder path, then click Use this folder.'], false);
    else throw err;
  }
}));

// checkUpdates asks GitHub for the latest release and offers it when it is newer. The check at launch is quiet:
// it says nothing when the app is up to date or GitHub cannot be reached.
async function checkUpdates(quiet) {
  const button = $('#check-update');
  button.disabled = true;
  button.textContent = 'Checking...';
  try {
    let u;
    try {
      u = await api('GET', '/api/update');
    } catch (err) {
      if (!quiet) throw err;
      return;
    }
    if (!u.newer) {
      if (!quiet) showToast([`You have the latest version (${u.current}).`], false);
      return;
    }
    const download = el('button', { className: 'primary', type: 'button', textContent: 'Download' });
    download.addEventListener('click', guarded(async () => {
      try {
        await api('POST', '/api/open-release', { url: u.url });
      } catch (err) {
        if (err.status === 501) showToast(['Open this page in your browser:', u.url], false);
        else throw err;
      }
    }));
    $('#update').replaceChildren(el('span', { textContent: `Version ${u.latest} is available` }), download);
    $('#update').hidden = false;
  } finally {
    button.disabled = false;
    button.textContent = 'Check for updates';
  }
}

$('#check-update').addEventListener('click', guarded(() => checkUpdates(false)));

// A shared filter file: the filter itself plus a marker, so a wrong file is refused instead of wiping the filter.
const SHARE_APP = 'MHO Loot Filter';

$('#export-filter').addEventListener('click', guarded(() => {
  const blob = new Blob([JSON.stringify({ app: SHARE_APP, version: 1, filter }, null, 1)], { type: 'application/json' });
  const link = el('a', { href: URL.createObjectURL(blob), download: 'mho-loot-filter.json' });
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(link.href), 10000);
  showToast(['Filter exported as mho-loot-filter.json in your Downloads folder.'], false);
}));

$('#import-filter').addEventListener('click', () => $('#import-file').click());
$('#import-file').addEventListener('change', guarded(async () => {
  const file = $('#import-file').files[0];
  $('#import-file').value = '';
  if (!file) return;
  let shared;
  try {
    shared = JSON.parse(await file.text());
  } catch {
    throw new Error(`${file.name} is not a filter file.`);
  }
  if (!shared || shared.app !== SHARE_APP || typeof shared.filter !== 'object' || shared.filter === null) {
    throw new Error(`${file.name} is not a filter exported from MHO Loot Filter.`);
  }
  const empty = emptyFilter();
  const next = { ...empty, ...Object.fromEntries(Object.entries(shared.filter).filter(([k, v]) => k in empty && v !== null && typeof v === typeof empty[k])) };
  next.groups = {};
  if (!confirm(`Replace your filter with the one in ${file.name}?`)) return;
  await api('PUT', '/api/filter', next);
  filter = next;
  $('#dirty').hidden = false;
  renderAll();
  showToast([`Imported ${file.name}.`, 'Click Apply to game to use it.'], false);
}));

let sound = null;

async function loadSound() {
  sound = await api('GET', '/api/sound');
  $('#sound-name').textContent = sound.custom ? sound.name : 'Built-in alert';
  $('#sound-note').textContent = sound.custom ? 'Your own sound' : 'Comes with the filter';
  $('#sound-reset').hidden = !sound.custom;
  $('#sound-max').textContent = sound.maxSeconds;
  Object.assign($('#sound-volume'), { min: sound.minVolume, max: sound.maxVolume, value: sound.volume });
  showVolume();
}

function showVolume() {
  const v = Number($('#sound-volume').value);
  $('#sound-volume-value').textContent = `${v > 0 ? '+' : ''}${v} dB`;
  $('#sound-volume-reset').hidden = !sound || v === sound.defaultVolume;
}

let audio;
// playSound plays the alert at the chosen volume: the default plays it as stored, and other settings louder or
// quieter by the same amount as in game, distortion included.
async function playSound() {
  if (!sound) await loadSound();
  audio ||= new AudioContext();
  const bytes = Uint8Array.from(atob(sound.wav), (c) => c.charCodeAt(0));
  const source = audio.createBufferSource();
  source.buffer = await audio.decodeAudioData(bytes.buffer);
  const gain = audio.createGain();
  gain.gain.value = 10 ** ((Number($('#sound-volume').value) - sound.defaultVolume) / 20);
  source.connect(gain).connect(audio.destination);
  source.start();
}

async function setVolume(v) {
  await api('PUT', '/api/sound/volume', { volume: v });
  sound.volume = v;
  $('#sound-volume').value = v;
  showVolume();
  $('#dirty').hidden = false;
}

$('#sound-volume').addEventListener('input', showVolume);
$('#sound-volume').addEventListener('change', guarded(() => setVolume(Number($('#sound-volume').value))));
$('#sound-volume-reset').addEventListener('click', guarded(() => setVolume(sound.defaultVolume)));

// decodeSound turns any audio file the browser can play into mono 16-bit samples at 44.1 kHz.
async function decodeSound(file, maxSeconds) {
  const ctx = new AudioContext();
  let decoded;
  try {
    decoded = await ctx.decodeAudioData(await file.arrayBuffer());
  } catch {
    throw new Error(`${file.name} could not be read as a sound.`);
  } finally {
    ctx.close();
  }
  const seconds = Math.min(decoded.duration, maxSeconds);
  const offline = new OfflineAudioContext(1, Math.max(1, Math.ceil(seconds * 44100)), 44100);
  const source = offline.createBufferSource();
  source.buffer = decoded;
  source.connect(offline.destination);
  source.start();
  const mono = (await offline.startRendering()).getChannelData(0);
  const pcm = new Int16Array(mono.length);
  for (let i = 0; i < mono.length; i++) pcm[i] = Math.max(-1, Math.min(1, mono[i])) * 32767;
  const bytes = new Uint8Array(pcm.buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return { samples: btoa(binary), cut: decoded.duration > maxSeconds };
}

$('#sound-play').addEventListener('click', guarded(playSound));
$('#sound-choose').addEventListener('click', () => $('#sound-file').click());
$('#sound-file').addEventListener('change', guarded(async () => {
  const file = $('#sound-file').files[0];
  $('#sound-file').value = '';
  if (!file) return;
  const { samples, cut } = await decodeSound(file, sound ? sound.maxSeconds : 10);
  await api('PUT', '/api/sound', { name: file.name, samples });
  await loadSound();
  $('#dirty').hidden = false;
  showToast([`Alert sound set to ${file.name}.`, ...(cut ? [`Only the first ${sound.maxSeconds} seconds are used.`] : []), 'Click Apply to game to use it.'], false);
}));
$('#sound-reset').addEventListener('click', guarded(async () => {
  await api('DELETE', '/api/sound');
  await loadSound();
  $('#dirty').hidden = false;
  showToast(['Back to the built-in alert. Click Apply to game to use it.'], false);
}));

let profiles = { active: 0, list: [] };

function renderProfiles() {
  $('#profile').replaceChildren(...profiles.list.map((p) => el('option', { value: p.id, textContent: p.name })));
  $('#profile').value = profiles.active;
  const active = profiles.list.find((p) => p.id === profiles.active);
  $('#profile-name').value = active ? active.name : '';
  $('#profile-delete').disabled = profiles.list.length < 2;
}

async function loadFilter() {
  const f = await api('GET', '/api/filter');
  filter = { ...emptyFilter(), ...Object.fromEntries(Object.entries(f).filter(([, v]) => v)) };
}

// useProfiles shows a new list of profiles and, when another one is in use, its filter and sound.
async function useProfiles(p) {
  const switched = p.active !== profiles.active;
  profiles = p;
  if (switched) {
    await loadFilter();
    for (const id of Object.keys(groupOrder)) delete groupOrder[id];
    sound = null;
    if (!$('#page-sound').hidden) await loadSound();
    $('#dirty').hidden = false;
  }
  renderProfiles();
  renderAll();
}

const activeProfile = () => profiles.list.find((p) => p.id === profiles.active);

$('#profile').addEventListener('change', guarded(async () => {
  await useProfiles(await api('POST', `/api/profiles/${$('#profile').value}/use`));
  showToast([`Using ${activeProfile().name}.`, 'Click Apply to game to use it in game.'], false);
}));

async function addProfile(copy) {
  await useProfiles(await api('POST', '/api/profiles', { copy }));
  navigate('filter');
  $('#profile-name').focus();
  $('#profile-name').select();
  showToast([`Made ${activeProfile().name}.`, 'Type a name for it, then press Enter.'], false);
}

$('#profile-new').addEventListener('click', guarded(() => addProfile(false)));
$('#profile-copy').addEventListener('click', guarded(() => addProfile(true)));
$('#profile-delete').addEventListener('click', guarded(async () => {
  const name = activeProfile().name;
  if (!confirm(`Delete the profile ${name}? Its filter and alert sound are lost.`)) return;
  await useProfiles(await api('DELETE', `/api/profiles/${profiles.active}`));
  showToast([`Deleted ${name}. Now using ${activeProfile().name}.`], false);
}));
$('#profile-name').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') $('#profile-name').blur();
  if (e.key === 'Escape') {
    renderProfiles();
    $('#profile-name').blur();
  }
});
$('#profile-name').addEventListener('change', guarded(async () => {
  try {
    await useProfiles(await api('PUT', `/api/profiles/${profiles.active}`, { name: $('#profile-name').value }));
  } catch (err) {
    renderProfiles();
    throw err;
  }
}));

let tweaks = {};

const TWEAKS = [
  { key: 'graphics', name: 'Graphics preset', sub: 'Sets shadows, bloom, depth of field, particles, lens flares and draw distance in one go. '
      + 'Potato is fastest, Balanced drops only the costliest effects, Pretty turns everything up. '
      + 'The game\'s Options show the preset and can still change it, and Game default puts your earlier Options back.',
    options: [['', 'Game default'], ['potato', 'Potato'], ['balanced', 'Balanced'], ['pretty', 'Pretty']] },
  { key: 'fps', name: 'Frame rate limit', sub: 'The game stops at 200 frames per second, and its Options only have VSync. '
      + 'A lower limit keeps your PC cooler and quieter, a higher one suits fast screens.',
    options: [[0, 'Game default (200)'], [30, '30'], [60, '60'], [120, '120'], [144, '144'], [165, '165'], [240, '240'], [360, '360'], [-1, 'No limit']] },
  { key: 'skipIntro', name: 'Startup videos', label: 'Skip videos', sub: 'Goes straight to the login screen instead of playing the logo videos first.' },
  { key: 'textureMB', name: 'Texture memory', sub: 'The game keeps 160 MB of textures loaded, little for today\'s graphics cards. '
      + 'More means fewer blurry textures sharpening as you play. Pick at most half of your graphics card\'s memory.',
    options: [[0, 'Game default (160 MB)'], [512, '512 MB'], [1024, '1 GB'], [2048, '2 GB'], [4096, '4 GB']] },
  { key: 'lowLag', name: 'Input lag', label: 'Lower', sub: 'The game shows each frame one frame late to gain speed. '
      + 'Lower makes clicks and powers feel more direct, for a few frames per second less.' },
  { key: 'nameSize', name: 'Item names on the ground', sub: 'Bigger names are easier to read in busy fights, smaller ones cover less of the screen.',
    options: [[11, 'Small'], [0, 'Game size'], [18, 'Large'], [22, 'Extra large']] },
  { key: 'mapOverlay', name: 'Map overlay', label: 'Show automatically', sub: 'Shows the map overlay, the big map Tab opens in the middle '
      + 'of the screen, every time you enter an area or terminal. Hubs such as Avengers Tower keep the small minimap. Tab still switches between them.' },
];

// Pointer pictures come from the game's own files; null until loaded, or when the game folder has none.
let pointerPics = null;
const POINTER_COLORS = [['', 'Game blue'], ['yellow', 'Yellow'], ['green', 'Green'], ['pink', 'Pink'], ['purple', 'Purple'], ['white', 'White']];
const POINTER_SIZES = [[0, 'Game size', 1], [150, 'Large', 1.5], [200, 'Extra large', 2]];

async function loadPointerPics() {
  try {
    pointerPics = await api('POST', '/api/pointers', { color: tweaks.pointer.color });
  } catch {
    pointerPics = null;
  }
}

async function loadTweaks() {
  tweaks = await api('GET', '/api/tweaks');
  await loadPointerPics();
  renderTweaks();
}

async function setTweak(key, value) {
  const colorChanged = key === 'pointer' && value.color !== tweaks.pointer.color;
  tweaks = await api('PUT', '/api/tweaks', { ...tweaks, [key]: value });
  $('#dirty').hidden = false;
  if (colorChanged) await loadPointerPics();
  renderTweaks();
}

// swatches is a row of picture choices. Each choice is [value, label, picture URL, picture box width, height].
function swatches(name, choices, current, onPick) {
  const group = el('div', { className: 'swatches', role: 'radiogroup', ariaLabel: name });
  for (const [value, label, src, w, h] of choices) {
    const on = value === current;
    const b = el('button', { type: 'button', className: 'swatch', role: 'radio', ariaChecked: String(on), tabIndex: on ? 0 : -1 },
      el('span', { className: 'pic', style: `width:${w}px;height:${h}px` }, src ? el('img', { src, alt: '' }) : ''),
      el('span', { textContent: label }));
    b.dataset.focus = 'swatch|' + name + '|' + value;
    b.addEventListener('click', guarded(() => onPick(value)));
    group.append(b);
  }
  // arrow keys move between choices, like other radio groups
  group.addEventListener('keydown', guarded(async (e) => {
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
    if (!step) return;
    e.preventDefault();
    const next = choices[(choices.findIndex(([v]) => v === current) + step + choices.length) % choices.length][0];
    await onPick(next);
    const b = document.querySelector(`[data-focus="${CSS.escape('swatch|' + name + '|' + next)}"]`);
    if (b) b.focus();
  }));
  return group;
}

function pointerRows() {
  const p = tweaks.pointer;
  const pics = pointerPics || { colors: {}, sizes: {} };
  const row = (name, sub, on, control) => el('div', { className: 'row' + (on ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item' }, el('span', { className: 'name', textContent: name }), el('div', { className: 'sub', textContent: sub })),
    control);
  return [
    row('Pointer colour', 'Paints the blue arrow. The red attack arrow and the badges for pick up, talk and the rest keep their colours.',
      p.color !== '', swatches('Pointer colour', POINTER_COLORS.map(([v, label]) => [v, label, pics.colors[v], 44, 40]), p.color,
        (color) => setTweak('pointer', { ...p, color }))),
    row('Pointer size', 'A larger pointer is easier to follow in busy fights. The arrow\'s tip still marks where you click.',
      p.size !== 0, swatches('Pointer size', POINTER_SIZES.map(([v, label, k]) => [v, label, pics.sizes[v], Math.round(44 * k), Math.round(40 * k)]),
        p.size, (size) => setTweak('pointer', { ...p, size }))),
  ];
}

function renderTweaks() {
  keepView(() => $('#tweaks').replaceChildren(...pointerRows(), ...TWEAKS.map((t) => {
    let control;
    if (t.options) {
      control = el('select', { className: 'tweak-select', ariaLabel: t.name },
        ...t.options.map(([v, label]) => el('option', { value: v, textContent: label, selected: tweaks[t.key] === v })));
      control.dataset.focus = 'tweak|' + t.key;
      const typed = typeof t.options[0][0] === 'number' ? Number : String;
      control.addEventListener('change', guarded(() => setTweak(t.key, typed(control.value))));
    } else {
      control = switchControl(t.label, tweaks[t.key], false, '', (on) => setTweak(t.key, on), 'tweak|' + t.key);
    }
    const on = Boolean(tweaks[t.key]); // 0, '' and false are the game's own setting
    return el('div', { className: 'row' + (on ? ' active' : ''), role: 'listitem' },
      el('div', { className: 'item' }, el('span', { className: 'name', textContent: t.name }), el('div', { className: 'sub', textContent: t.sub })),
      el('div', { className: 'flags' }, control));
  })));
}

let playWays = [], playDir = null;

// loadPlay lists the ways to start the game found in the game folder (Bifrost, Steam) and picks the last one used.
async function loadPlay() {
  playWays = await api('GET', '/api/play');
  const select = $('#play-way');
  let last = '';
  try { last = localStorage.getItem('playWay') || ''; } catch { /* storage can be off */ }
  select.replaceChildren(...playWays.map((w) => el('option', { value: w.id, textContent: w.name })));
  if (playWays.some((w) => w.id === last)) select.value = last;
  select.hidden = playWays.length < 2;
  $('#play-box').hidden = playWays.length === 0;
  $('#play').title = playWays.length === 1 ? `Starts the game with ${playWays[0].name}` : '';
}

async function play() {
  const id = $('#play-way').value;
  if (!$('#dirty').hidden && !await run('/api/apply', 'Applied')) return;
  try {
    await api('POST', '/api/play', { id });
    showToast(['Starting Marvel Heroes Omega.'], false);
  } catch (err) {
    showToast([err.message], true);
  }
  refreshStatus();
}

// Mute sounds page. soundList comes once from /api/sounds; muted is the set of muted event names.
let soundList = null;
let muted = new Set();
let onlyMuted = false;
const MAX_SOUNDS = 200;

async function loadMutes() {
  if (!soundList) soundList = await api('GET', '/api/sounds');
  muted = new Set(await api('GET', '/api/mutes'));
  renderMutes();
}

async function setMutes(names, on) {
  const next = new Set(muted);
  for (const n of names) {
    if (on) next.add(n);
    else next.delete(n);
  }
  muted = new Set(await api('PUT', '/api/mutes', [...next]));
  $('#dirty').hidden = false;
  renderMutes();
}

const openSoundGroups = new Set();
const preview = new Audio();
let previewing = '';
preview.addEventListener('ended', () => { previewing = ''; keepView(renderMutes); });

// previewSound plays a game sound, or stops it when it is the one playing.
async function previewSound(name) {
  preview.pause();
  if (previewing === name) {
    previewing = '';
    renderMutes();
    return;
  }
  previewing = name;
  renderMutes();
  try {
    const { url } = await api('POST', '/api/sound-preview', { name });
    if (previewing !== name) return; // another sound was picked meanwhile
    preview.src = url;
    await preview.play();
  } catch (err) {
    if (previewing === name) previewing = '';
    renderMutes();
    throw err.status ? err : new Error('This sound could not be played.');
  }
}

function soundRow(s, showGroup) {
  const play = el('button', { className: 'secondary small', type: 'button', disabled: !s.preview,
    textContent: previewing === s.name ? 'Stop' : 'Play', title: s.preview ? '' : 'This sound cannot be previewed' });
  play.dataset.focus = 'play|' + s.name;
  play.addEventListener('click', guarded(() => previewSound(s.name)));
  return el('div', { className: 'row' + (muted.has(s.name) ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item' }, el('span', { className: 'name', textContent: s.label.charAt(0).toUpperCase() + s.label.slice(1) }),
      showGroup ? el('div', { className: 'sub', textContent: s.group }) : ''),
    el('div', { className: 'flags' }, play,
      switchControl('Mute', muted.has(s.name), false, '', (on) => setMutes([s.name], on), 'mute|' + s.name)));
}

function renderMutes() {
  keepView(() => {
    $('#mute-groups').replaceChildren(...soundList.groups.map((g) => {
      const sounds = soundList.sounds.filter((s) => s.group === g);
      const names = sounds.map((s) => s.name);
      const n = names.filter((x) => muted.has(x)).length;
      if (!names.length) return '';
      const open = openSoundGroups.has(g);
      const toggle = el('button', { className: 'expand', type: 'button' },
        el('span', { className: 'chevron', textContent: open ? '\u25be' : '\u25b8' }),
        el('span', { className: 'name', textContent: g }),
        el('span', { className: 'count', textContent: `${names.length} sound${names.length === 1 ? '' : 's'}` }));
      toggle.setAttribute('aria-expanded', String(open));
      toggle.dataset.focus = 'opensounds|' + g;
      toggle.addEventListener('click', () => {
        if (open) openSoundGroups.delete(g);
        else openSoundGroups.add(g);
        renderMutes();
      });
      const row = el('div', { className: 'row' + (n ? ' active' : ''), role: 'listitem' },
        el('div', { className: 'item' }, toggle, el('div', { className: 'sub', textContent: n ? `${n} muted` : 'None muted' })),
        el('div', { className: 'flags' }, switchControl('Mute all', n === names.length, false, '', (on) => setMutes(names, on), 'mutegroup|' + g)));
      if (open) row.append(el('div', { className: 'members', role: 'list' }, ...sounds.map((s) => soundRow(s, false))));
      return row;
    }));
    $('#mute-only').replaceChildren(switchControl('Only muted', onlyMuted, false, '', (on) => { onlyMuted = on; renderMutes(); }, 'muteonly'));
    renderMuteResults();
  });
}

function renderMuteResults() {
  const box = $('#mute-results');
  const words = $('#mq').value.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length && !onlyMuted) {
    box.replaceChildren(el('p', { className: 'empty', textContent: 'Type part of a sound name, a hero or a zone.' }));
    return;
  }
  const found = soundList.sounds.filter((s) => (!onlyMuted || muted.has(s.name))
    && words.every((w) => s.label.includes(w) || s.name.includes(w) || s.group.toLowerCase().includes(w)));
  if (!found.length) {
    box.replaceChildren(el('p', { className: 'empty', textContent: onlyMuted && !words.length ? 'No sounds are muted.' : 'No sounds match.' }));
    return;
  }
  const rows = found.slice(0, MAX_SOUNDS).map((s) => soundRow(s, true));
  if (found.length > MAX_SOUNDS) {
    rows.push(el('p', { className: 'empty', textContent: `Showing ${MAX_SOUNDS} of ${found.length} sounds. Type more to narrow it down.` }));
  }
  box.replaceChildren(...rows);
}

$('#mq').addEventListener('input', () => { if (soundList) keepView(renderMuteResults); });

async function refreshStatus() {
  try {
    const s = await api('GET', '/api/status');
    if (s.gameDir !== playDir) {
      playDir = s.gameDir;
      loadPlay().catch(() => { $('#play-box').hidden = true; });
    }
    if (s.version) $('#version').textContent = 'Version ' + s.version;
    $('#status .dot').className = 'dot ' + (!s.gameFound ? 'bad' : s.gameRunning ? 'warn' : 'ok');
    $('#status-text').textContent = !s.gameFound ? 'Game folder not found' : s.gameRunning ? 'Close the game to apply' : 'Game found';
    $('#status').title = s.gameDir;
    if (!busy) $('#apply').disabled = $('#restore').disabled = $('#play').disabled = !s.gameFound || s.gameRunning;
  } catch {
    $('#status .dot').className = 'dot bad';
    $('#status-text').textContent = 'Filter app stopped';
    $('#apply').disabled = $('#restore').disabled = $('#play').disabled = true;
  }
}

// run applies or restores and reports whether it worked.
async function run(url, doneWord) {
  busy = true;
  $('#apply').disabled = $('#restore').disabled = $('#play').disabled = true;
  try {
    const r = await api('POST', url);
    const warnings = r.warnings || [];
    if (url === '/api/restore') {
      filter = emptyFilter();
      tweaks = {};
      renderAll();
      if (currentHash() === '#tweaks') await loadTweaks();
      if (currentHash() === '#mutes') await loadMutes();
    }
    $('#dirty').hidden = url === '/api/apply' || filterSize() === 0;
    showToast([`${doneWord}: ${r.changed} game file${r.changed === 1 ? '' : 's'} changed.`, ...warnings], warnings.length > 0);
    return true;
  } catch (err) {
    showToast([err.status === 409 ? 'Close Marvel Heroes Omega, then try again.' : err.message], true);
    return false;
  } finally {
    busy = false;
    refreshStatus();
  }
}

$('#apply').addEventListener('click', () => run('/api/apply', 'Applied'));
$('#play').addEventListener('click', play);
$('#play-way').addEventListener('change', () => {
  try { localStorage.setItem('playWay', $('#play-way').value); } catch { /* storage can be off */ }
});
$('#restore').addEventListener('click', () => {
  if (confirm(`Put every game file back to its original, and clear the filter of ${activeProfile().name}, the game tweaks and the muted sounds?`)) run('/api/restore', 'Restored');
});
// an error outside a guarded call would otherwise go unseen in the desktop window
window.addEventListener('error', (e) => showToast([e.message || 'Something went wrong'], true));
window.addEventListener('unhandledrejection', (e) => showToast([(e.reason && e.reason.message) || String(e.reason)], true));

// Drop finder: Items and Bosses tabs, item and boss pages. Every chance comes from the built drop data.
const TIER_NAMES = ['Normal', 'Heroic', 'Cosmic']; // the game's names; green, red and yellow on the waypoint
let dropsState = { tab: 'items', tier: 2 };
try { dropsState = { ...dropsState, ...JSON.parse(localStorage.getItem('dropsState') || '{}') }; } catch { /* storage can be off */ }
const saveDropsState = () => { try { localStorage.setItem('dropsState', JSON.stringify(dropsState)); } catch { /* storage can be off */ } };
let dropsQuery = '';
let dropsRender = 0; // each drawing of the page; an answer for an older one is dropped (the user moved on)

function pct(p) {
  if (p >= 0.995) return '100%';
  const v = p * 100;
  return (v >= 10 ? v.toFixed(0) : v >= 1 ? +v.toFixed(1) : +v.toPrecision(2)) + '%';
}

function oneIn(p) {
  if (p <= 0 || p >= 0.02) return '';
  const n = 1 / p;
  const digits = Math.max(0, Math.floor(Math.log10(n)) - 1);
  return '1 in ' + (Math.round(n / 10 ** digits) * 10 ** digits).toLocaleString('en-US');
}

const dropIconCache = {};
function dropIcon(path) {
  const img = el('img', { className: 'icon drop-icon', alt: '', width: 36, height: 36 });
  img.dataset.path = path;
  if (dropIconCache[path]) img.src = dropIconCache[path];
  return img;
}

async function loadDropIcons() {
  const want = [...new Set([...document.querySelectorAll('img.drop-icon:not([src])')].map((i) => i.dataset.path))]
    .filter((p) => !(p in dropIconCache));
  if (!want.length) return;
  want.forEach((p) => { dropIconCache[p] = ''; });
  for (let i = 0; i < want.length; i += 1000) { // the server answers at most 1,000 at a time
    Object.assign(dropIconCache, await api('POST', '/api/drops/icons', { paths: want.slice(i, i + 1000) }));
  }
  for (const img of document.querySelectorAll('img.drop-icon:not([src])')) {
    if (dropIconCache[img.dataset.path]) img.src = dropIconCache[img.dataset.path];
  }
}

function tierSwitch(onChange) {
  const box = el('div', { className: 'tier-switch', role: 'radiogroup', ariaLabel: 'Difficulty' });
  TIER_NAMES.forEach((name, i) => {
    const b = el('button', { type: 'button', className: 'tier-' + i, textContent: name, role: 'radio' });
    b.setAttribute('aria-checked', String(dropsState.tier === i));
    b.addEventListener('click', () => { dropsState.tier = i; saveDropsState(); onChange(); });
    box.append(b);
  });
  return box;
}

// the drop finder pages visited, for its Back button: the browser's own history may start outside the finder
const dropsTrail = [];

// Mouse button 4 goes back too, as in browsers. The browser's own back on that button is cancelled: see crumbs.
window.addEventListener('mouseup', (e) => {
  if (e.button !== 3 && e.button !== 4) return;
  e.preventDefault();
  const back = e.button === 3 && [...document.querySelectorAll('.crumbs .back')].find((b) => b.offsetParent);
  if (back) back.click();
});

function crumbs(parts, trail = dropsTrail, home = 'drops') {
  const back = el('button', { className: 'back', type: 'button', textContent: '‹ Back', title: 'Back (mouse button 4)' });
  // our own trail, not history.back(): in the Windows app (a page loaded from a string) going back through the
  // browser's history left the page ignoring every later link
  back.addEventListener('click', () => {
    trail.pop();
    navigate(trail.at(-1) || home);
  });
  const box = el('div', { className: 'crumbs' }, back);
  parts.forEach(([text, hash], i) => {
    if (i) box.append(' › ');
    if (hash === undefined) box.append(text);
    else {
      const b = el('button', { className: 'link', type: 'button', textContent: text });
      b.addEventListener('click', () => { navigate(hash); });
      box.append(b);
    }
  });
  return box;
}

const goDrops = (...parts) => { navigate(['drops', ...parts.map(encodeURIComponent)].join('/')); };

function itemName(it, onClick) {
  const name = el('span', { className: 'name' + (it.rarity ? ' rarity-' + it.rarity : '') + (onClick ? ' link-like' : ''), textContent: it.name });
  if (onClick) {
    name.tabIndex = 0;
    name.setAttribute('role', 'link');
    name.addEventListener('click', onClick);
    name.addEventListener('keydown', (e) => { if (e.key === 'Enter') onClick(); });
  }
  const wrap = el('span', {}, name);
  if (it.label) wrap.append(el('span', { className: 'tag', textContent: it.label }));
  return wrap;
}

function chanceCell(p, n) {
  const cell = el('div', { className: 'pct' }, el('b', { textContent: pct(p) }));
  const sub = n > 1.05 ? Math.round(n * 10) / 10 + ' each' : oneIn(p);
  if (sub) cell.append(sub);
  return cell;
}

const DROP_TABS = ['items', 'bosses', 'places', 'vendors'];

// Advanced search: stat words that must all be on an item's tooltip, and ticks that narrow by kind. Inside one
// group a tick means "any of", across groups "all of". Kept while the app runs, so Back finds it as it was.
const ADV_ITEM_GROUPS = [
  { key: 'slot', label: 'Gear slot', options: [1, 2, 3, 4, 5].map((n) => [String(n), 'Gear slot ' + n]) },
  { key: 'type', label: 'Type', options: [['rings', 'Rings'], ['artifacts', 'Artifacts'], ['uniques', 'Uniques'],
    ['medallions', 'Medallions'], ['legendaries', 'Legendaries'], ['relics', 'Relics'], ['insignias', 'Insignias'],
    ['teamup', 'Team-up gear']] },
];
const dropsAdv = { open: false, terms: [], ticks: {}, sort: 'stat' }; // sort: by the first stat, biggest first, or by name
let advWanted = ''; // the newest advanced search asked for
const advActive = (st) => st.terms.length > 0 || Object.values(st.ticks).some((v) => v.length);

function advancedToggle(st, onChange) {
  const b = el('button', { type: 'button', className: 'adv-toggle', textContent: 'Advanced search' });
  b.setAttribute('aria-expanded', String(st.open));
  b.addEventListener('click', () => { st.open = !st.open; onChange(); });
  return b;
}

// the panel under a search box: an optional stat word box with removable words, and a row of ticks per group
function advancedPanel({ groups, terms, state, onChange }) {
  const box = el('div', { className: 'adv-panel' });
  if (terms) {
    const words = el('div', { className: 'adv-terms' });
    const input = el('input', { type: 'search', autocomplete: 'off', placeholder: 'Type a stat, e.g. Energy, and pick it from the list' });
    input.setAttribute('aria-label', 'Add a stat');
    // the picker: stat names on the tooltips as you type, so Energy can be told from Energy Damage Rating. A word
    // that is no stat's name (a Legendary's rank, a word in an effect) can still be added as it is typed.
    const menu = el('div', { className: 'adv-suggest', role: 'listbox', id: 'adv-suggest', hidden: true });
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', menu.id);
    input.setAttribute('aria-expanded', 'false');
    let stats = [], shown = [], at = -1;
    api('GET', '/api/drops/stats').then((s) => { stats = s; }).catch(() => {}); // without it, typed words still work
    const show = () => {
      menu.replaceChildren(...shown.map((s, i) => {
        const o = el('div', { className: 'adv-option', role: 'option', id: 'adv-opt-' + i },
          el('span', { textContent: s.name }), el('span', { className: 'adv-count', textContent: s.items === 1 ? '1 item' : s.items + ' items' }));
        o.setAttribute('aria-selected', String(i === at));
        o.addEventListener('mousedown', (e) => { e.preventDefault(); add(s.name); }); // before the input loses focus
        return o;
      }));
      menu.hidden = !shown.length;
      input.setAttribute('aria-expanded', String(!menu.hidden));
      if (at >= 0 && shown.length) input.setAttribute('aria-activedescendant', 'adv-opt-' + at); else input.removeAttribute('aria-activedescendant');
      menu.children[at]?.scrollIntoView({ block: 'nearest' });
    };
    const suggest = () => { // the name itself first, then names starting with it, then a word starting with it, then the rest
      const q = input.value.trim().toLowerCase();
      const rank = (n) => (n === q ? 0 : n.startsWith(q) ? 1 : n.split(' ').some((w) => w.startsWith(q)) ? 2 : 3);
      // the list comes most items first, and the sort keeps that order inside a rank
      shown = !q ? [] : stats.filter((s) => s.name.toLowerCase().includes(q))
        .map((s) => [rank(s.name.toLowerCase()), s]).sort((a, b) => a[0] - b[0]).map((x) => x[1]).slice(0, 10);
      at = shown.length ? 0 : -1;
      show();
    };
    const add = (picked) => {
      const t = picked || input.value.trim();
      if (t && !state.terms.some((x) => x.toLowerCase() === t.toLowerCase())) state.terms.push(t);
      input.value = '';
      shown = [];
      show();
      draw();
      onChange();
    };
    input.addEventListener('input', suggest);
    input.addEventListener('blur', () => { shown = []; show(); });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        if (!shown.length) { suggest(); return; }
        at = (at + (e.key === 'ArrowDown' ? 1 : shown.length - 1)) % shown.length;
        show();
      } else if (e.key === 'Escape' && shown.length) {
        e.preventDefault();
        shown = [];
        show();
      } else if (e.key === 'Enter') {
        e.preventDefault();
        add(shown[at]?.name);
      }
    });
    const draw = () => words.replaceChildren(...state.terms.map((t) => {
      const chip = el('button', { type: 'button', className: 'adv-chip', title: 'Remove ' + t }, t, el('span', { className: 'adv-x', ariaHidden: 'true', textContent: '\u00d7' }));
      chip.setAttribute('aria-label', 'Remove ' + t);
      chip.addEventListener('click', () => { state.terms = state.terms.filter((x) => x !== t); draw(); onChange(); });
      return chip;
    }));
    const addBtn = el('button', { type: 'button', className: 'adv-add', textContent: 'Add' });
    addBtn.addEventListener('mousedown', (e) => { e.preventDefault(); }); // keep the list, so Add takes the highlighted stat as Enter does
    addBtn.addEventListener('click', () => { add(shown[at]?.name); });
    draw();
    box.append(el('div', { className: 'adv-group' }, el('div', { className: 'adv-label', textContent: 'Stats' }),
      el('div', { className: 'adv-term-row' }, el('div', { className: 'adv-combo' }, input, menu), addBtn), words));
  }
  for (const g of groups) {
    const picked = (state.ticks[g.key] ||= []);
    const row = el('div', { className: 'adv-pills', role: 'group', ariaLabel: g.label });
    for (const [value, text] of g.options) {
      const b = el('button', { type: 'button', className: 'adv-pill', textContent: text });
      b.setAttribute('aria-pressed', String(picked.includes(value)));
      b.addEventListener('click', () => {
        const i = picked.indexOf(value);
        if (i < 0) picked.push(value); else picked.splice(i, 1);
        b.setAttribute('aria-pressed', String(i < 0));
        onChange();
      });
      row.append(b);
    }
    box.append(el('div', { className: 'adv-group' }, el('div', { className: 'adv-label', textContent: g.label }), row));
  }
  const clear = el('button', { type: 'button', className: 'adv-clear', textContent: 'Clear all' });
  clear.addEventListener('click', () => { state.terms = []; state.ticks = {}; onChange(true); });
  box.append(clear);
  return box;
}
// what an item comes from when nothing drops it
const ONLY_FROM = { vendor: 'Sold by a vendor', service: 'Crafter service', recipe: 'Crafted', achievement: 'Achievement reward', store: 'In the store' };

async function renderDrops(route) {
  const box = $('#drops');
  const n = ++dropsRender;
  const here = currentHash();
  if (dropsTrail.at(-1) !== here) dropsTrail.push(here);
  if (DROP_TABS.includes(route[0]) && dropsState.tab !== route[0]) { // a tab's own link, such as #drops/bosses
    dropsState.tab = route[0];
    saveDropsState();
    dropsQuery = '';
  }
  if (route[0] === 'item') return renderDropItem(box, route[1], n);
  if (route[0] === 'boss') return renderDropBoss(box, route[1], route[2], n);
  if (route[0] === 'place') return renderDropPlace(box, route[1], n);
  if (route[0] === 'vendor') return renderDropVendor(box, route[1], n);
  const tabs = el('div', { className: 'tabs', role: 'tablist' });
  for (const [id, text] of [['items', 'Items'], ['bosses', 'Bosses'], ['places', 'Places'], ['vendors', 'Vendors']]) {
    const b = el('button', { type: 'button', role: 'tab', textContent: text });
    b.setAttribute('aria-selected', String(dropsState.tab === id));
    b.addEventListener('click', () => { navigate('drops/' + id); });
    tabs.append(b);
  }
  const items = dropsState.tab === 'items';
  const places = dropsState.tab === 'places';
  const vendors = dropsState.tab === 'vendors';
  const input = el('input', { type: 'search', value: dropsQuery, autocomplete: 'off',
    placeholder: items && dropsAdv.open ? 'Narrow by item name (optional)' : items ? 'Search items, e.g. Doctor Doom Medallion' : places ? 'Filter places, e.g. Castle Doom'
      : vendors ? 'Filter vendors, e.g. Eternity Splinter' : 'Filter bosses, e.g. Doom' });
  input.setAttribute('aria-label', items ? 'Search items' : places ? 'Filter places' : vendors ? 'Filter vendors' : 'Filter bosses');
  const list = el('div', { role: 'list' });
  // place and vendor lists have no chances that follow the difficulty, so no switch
  const bar = el('div', { className: 'drops-bar' }, input, ...(items ? [advancedToggle(dropsAdv, () => renderDrops([]))] : []),
    ...(places || vendors ? [] : [tierSwitch(() => renderDrops([]))]));
  const adv = items && dropsAdv.open;
  box.replaceChildren(tabs, bar, ...(adv ? [advancedPanel({ groups: ADV_ITEM_GROUPS, terms: true, state: dropsAdv,
    onChange: (redraw) => (redraw ? renderDrops([]) : fill()) })] : []), list);
  const fill = guarded(async () => {
    dropsQuery = input.value;
    const q = input.value.trim();
    if (adv) {
      if (!advActive(dropsAdv)) {
        list.replaceChildren(el('p', { className: 'empty', textContent: 'Type a stat or tick a filter.' }));
        return;
      }
      const byStat = dropsAdv.terms.length > 0 && dropsAdv.sort === 'stat';
      const params = new URLSearchParams({ q, sort: byStat ? 'stat' : 'name' });
      dropsAdv.terms.forEach((t) => params.append('stat', t));
      for (const [k, vs] of Object.entries(dropsAdv.ticks)) vs.forEach((v) => params.append(k, v));
      const want = params.toString();
      advWanted = want;
      const found = await api('GET', '/api/drops/advanced?' + want);
      if (n !== dropsRender || input.value.trim() !== q || advWanted !== want) return; // a newer search is on its way
      if (!found.length) {
        list.replaceChildren(el('p', { className: 'empty', textContent: 'No items match these stats and filters.' }));
        return;
      }
      const head = el('div', { className: 'adv-head' }, el('p', { className: 'drop-note', textContent: found.length === 1 ? '1 item' : `${found.length} items` }));
      if (dropsAdv.terms.length) { // sort by how big the first stat is, or by name
        const sorter = el('div', { className: 'tier-switch', role: 'radiogroup', ariaLabel: 'Sort' });
        for (const [id, text] of [['stat', `Highest ${dropsAdv.terms[0]}`], ['name', 'Name']]) {
          const b = el('button', { type: 'button', role: 'radio', textContent: text });
          b.setAttribute('aria-checked', String(dropsAdv.sort === id));
          b.addEventListener('click', () => { dropsAdv.sort = id; fill(); });
          sorter.append(b);
        }
        head.append(el('div', { className: 'adv-sort' }, el('span', { textContent: 'Sort' }), sorter));
      }
      list.replaceChildren(head,
        ...found.map((it) => el('div', { className: 'drop-row', role: 'listitem' },
          dropIcon(it.path), el('div', {}, itemName(it, () => goDrops('item', it.path)),
            ...(it.match || []).map((m) => el('div', { className: 'adv-match', textContent: m }))))));
    } else if (items) {
      if (q.length < 2) {
        list.replaceChildren(el('p', { className: 'empty', textContent: 'Type at least 2 letters of an item name.' }));
        return;
      }
      const found = await api('GET', '/api/drops/items?q=' + encodeURIComponent(q));
      if (input.value.trim() !== q || n !== dropsRender) return;
      if (!found.length) {
        list.replaceChildren(el('p', { className: 'empty', textContent: `No dropped items match "${q}".` }));
        return;
      }
      list.replaceChildren(el('p', { className: 'drop-note', textContent: `Best chance from any boss or box on ${TIER_NAMES[dropsState.tier]}.` }),
        ...found.map((it) => el('div', { className: 'drop-row', role: 'listitem' },
        dropIcon(it.path), el('div', {}, itemName(it, () => goDrops('item', it.path))),
        it.only ? el('div', { className: 'pct', textContent: ONLY_FROM[it.only] })
          : it.conditional ? el('div', { className: 'pct' }, el('b', { textContent: pct(it.best[dropsState.tier]) }), 'with conditions')
            : chanceCell(it.best[dropsState.tier], 0))));
    } else if (places) {
      const found = await api('GET', '/api/drops/places?q=' + encodeURIComponent(q));
      if (input.value.trim() !== q || n !== dropsRender) return;
      if (!found.length) {
        list.replaceChildren(el('p', { className: 'empty', textContent: `No places match "${q}".` }));
        return;
      }
      const parts = [];
      let group = '';
      for (const p of found) {
        if (p.group !== group) parts.push(el('h3', { className: 'section-title', textContent: (group = p.group) }));
        parts.push(el('div', { className: 'drop-row no-icon', role: 'listitem' },
          el('div', {}, itemName({ name: p.name }, () => goDrops('place', p.id))),
          el('div', { className: 'pct', textContent: p.bosses === 1 ? '1 boss' : p.bosses + ' bosses' })));
      }
      list.replaceChildren(...parts);
    } else if (vendors) {
      const found = await api('GET', '/api/drops/vendors?q=' + encodeURIComponent(q));
      if (input.value.trim() !== q || n !== dropsRender) return;
      list.replaceChildren(...(found.length ? found.map((v) => el('div', { className: 'drop-row no-icon', role: 'listitem' },
        el('div', {}, itemName({ name: v.name }, () => goDrops('vendor', v.name)))))
        : [el('p', { className: 'empty', textContent: `No vendors match "${q}".` })]));
    } else {
      const bosses = await api('GET', '/api/drops/bosses?q=' + encodeURIComponent(q));
      if (input.value.trim() !== q || n !== dropsRender) return;
      list.replaceChildren(...(bosses.length ? bosses.map((b) => el('div', { className: 'drop-row no-icon', role: 'listitem' },
        el('div', {}, itemName({ name: b.name }, () => goDrops('boss', b.name))),
        el('div', { className: 'pct', textContent: b.variants > 1 ? b.variants + ' versions' : '' })))
        : [el('p', { className: 'empty', textContent: `No bosses match "${q}".` })]));
    }
    loadDropIcons().catch(() => {}); // no icons is not worth an error message
  });
  let timer;
  input.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(fill, 150); });
  input.focus();
  await fill();
}

function rowTags(r) {
  const tags = r.labels.map((l) => el('span', { className: 'tag', textContent: l }));
  if (r.server) tags.push(el('span', { className: 'tag server', textContent: r.server === 1 ? 'added by the server' : 'changed by the server' }));
  return tags;
}

// the item as its in-game tooltip, laid out by the game's own template (tools/tooltip.py builds it)
function gameCard(card, iconKey) {
  const hex = (c) => '#' + (c || 'FFFFFF');
  // a range never breaks across lines: "+221-442" stays together
  const unbroken = (t) => t.split(/(\d[\d,.]*-\d[\d,.]*)/).map((part, i) => (i % 2 ? el('span', { className: 'nowrap', textContent: part }) : part));
  const text = (segs) => segs.map(([t, c]) => (c ? el('span', { style: `color:${hex(c)}` }, ...unbroken(t)) : el('span', {}, ...unbroken(t))));
  const box = el('article', { className: 'game-card' });
  box.setAttribute('aria-label', card.title);
  const head = el('header', { className: 'gc-head' }, dropIcon(iconKey),
    el('h2', { className: 'gc-title', style: `color:${hex(card.color)}`, textContent: card.title }),
    el('div', { className: 'gc-cat', style: `color:${hex(card.color)}`, textContent: card.category }));
  box.append(head);
  const lastLines = card.sections.map((s) => s.kind).lastIndexOf('lines');
  card.sections.forEach((sec, i) => {
    box.append(el('hr', { className: 'gc-rule' }));
    const colour = `color:${hex(sec.color)}`;
    if (sec.kind === 'text') {
      box.append(el('p', { className: 'gc-text' + (i === card.sections.length - 1 && i > lastLines ? ' gc-flavor' : ''), style: colour }, ...text(sec.text)));
    } else if (sec.kind === 'lines') {
      const list = el('ul', { className: 'gc-lines', style: colour }, ...sec.lines.map((l) => el('li', {}, ...text(l))));
      box.append(list);
      if (i === lastLines && card.hidden) {
        list.append(el('li', { className: 'gc-hidden', textContent: card.hidden === 1 ? '1 more effect not shown' : `${card.hidden} more effects not shown` }));
      }
    } else if (sec.kind === 'options') {
      box.append(el('div', { className: 'gc-options', style: colour }, ...(sec.title ? [el('div', { textContent: sec.title })] : []),
        el('div', { className: 'gc-note', textContent: sec.note || '1 random bonus of these:' }),
        el('ul', { className: 'gc-lines' }, ...sec.options.map((o) => el('li', {}, ...text(o))),
          ...(sec.hidden ? [el('li', { className: 'gc-hidden', textContent: sec.hidden === 1 ? '1 more bonus not shown' : `${sec.hidden} more bonuses not shown` })] : []))));
    }
  });
  if (lastLines < 0 && card.hidden) box.append(el('p', { className: 'gc-hidden', textContent: `${card.hidden} effects not shown` }));
  if (card.level || card.grade || !card.tradeable) box.append(el('hr', { className: 'gc-rule' }), el('footer', { className: 'gc-foot' },
    el('span', { textContent: card.level ? `Level Required: ${card.level}` : '' }), el('span', { textContent: card.tradeable ? '' : 'Not tradeable' }),
    el('span', { textContent: card.grade ? `Item Grade: ${card.grade}` : '' })));
  return box;
}

// a round boss portrait; the initials show until the game's icon loads, and stay when it has none
function portrait(key, name, big) {
  const initials = name.split(/\s+/).filter((w) => /^[A-Za-z0-9]/.test(w)).slice(0, 2).map((w) => w[0]).join('').toUpperCase();
  const img = dropIcon(key);
  img.classList.add('portrait-img');
  const ring = el('span', { className: 'portrait' + (big ? ' big' : '') }, el('span', { className: 'initials', textContent: initials }), img);
  ring.setAttribute('aria-hidden', 'true');
  return ring;
}

// places as the game's M.A.P. names them, one line per map version, with the difficulties it drops on
// (entries: {place, placeName, version, p}; the regular Hightown Patrol is Normal and Heroic, its Cosmic map Cosmic)
// a place's versions as the game orders them: Green, Red, then Cosmic terminals; the standard map
// before its Cosmic one
const VERSION_ORDER = ['', 'Standard', 'Green terminal', 'Red terminal', 'Cosmic terminal', 'Cosmic', 'random boss', 'random boss, Cosmic'];
const byVersion = (a, b) => (VERSION_ORDER.indexOf(a) + 1 || 99) - (VERSION_ORDER.indexOf(b) + 1 || 99);

function placesPanel(title, entries, current = new Set(), note = 'Open a place to see every boss there and what it drops.') {
  // one row per place, as every version of a place opens the same place page; its versions on a line below
  const lines = new Map();
  for (const e of entries.filter((x) => x.place && x.p.some((p) => p > 0))) {
    const line = lines.get(e.place) || { place: e.place, label: e.placeName, keys: [], versions: [], tiers: [false, false, false] };
    line.keys.push(e.place + '|' + e.version);
    const version = e.version || 'Standard';
    if (!line.versions.includes(version)) line.versions.push(version);
    e.p.forEach((x, i) => { if (x > 0) line.tiers[i] = true; });
    lines.set(e.place, line);
  }
  const shown = [...lines.values()];
  if (!shown.length) return null;
  for (const l of shown) l.here = l.keys.some((k) => current.has(k));
  return el('section', { className: 'drop-panel places-panel' }, el('h3', { className: 'section-title', textContent: title }),
    el('p', { className: 'drop-note', textContent: note + (shown.some((l) => l.here) ? ' The highlighted one is shown here.' : '') }),
    el('ul', { className: 'place-list' }, ...shown.map((l) => {
      const here = l.here;
      const tiers = TIER_NAMES.flatMap((name, i) => (l.tiers[i] ? [el('span', { className: 'tier-pill tier-' + i, textContent: name })] : []));
      const versions = l.versions.length > 1 || l.versions[0] !== 'Standard' ? [...l.versions].sort(byVersion).join(', ') : '';
      const b = el('button', { type: 'button', className: 'place-row', title: `Open ${l.label}` },
        el('span', { className: 'place-main' }, el('span', { className: 'place-name', textContent: l.label }),
          ...(versions ? [el('span', { className: 'place-versions', textContent: versions })] : [])), el('span', { className: 'place-tiers' }, ...tiers),
        el('span', { className: 'place-go', 'aria-hidden': 'true', textContent: '›' }));
      b.addEventListener('click', () => goDrops('place', l.place));
      const li = el('li', { className: here ? 'current' : '' }, b);
      if (here) li.setAttribute('aria-current', 'true');
      return li;
    })));
}

// one tile per boss; versions with their own chance open from the tile
function bossTile(name, versions, t) {
  const best = versions[0];
  const open = () => goDrops('boss', name, best.id);
  const face = best.iconKey || versions.find((v) => v.icon)?.id || best.id; // any version's portrait
  const link = el('button', { type: 'button', className: 'boss-tile-link' }, portrait(face, name), el('span', { className: 'boss-tile-name', textContent: name }));
  link.addEventListener('click', open);
  const tile = el('div', { className: 'boss-tile', role: 'listitem' }, link);
  if (t !== undefined) tile.append(el('div', { className: 'boss-tile-pct' }, el('b', { textContent: pct(best.p[t]) }), oneIn(best.p[t])));
  if (versions.length > 1) {
    const list = el('div', { className: 'boss-versions' }, ...versions.map((v) => {
      // short labels: the place and its version, else the version alone (a place page), else the long hint
      const label = v.placeName ? v.placeName + (v.version ? ` (${v.version})` : '') : v.version || v.where || 'Version';
      const b = el('button', { type: 'button', className: 'link', textContent: label });
      if (v.where && v.where !== label) b.title = v.where;
      b.addEventListener('click', () => goDrops('boss', name, v.id));
      return el('div', { className: 'boss-version' }, b, ...(t !== undefined ? [el('span', { textContent: pct(v.p[t]) })] : []));
    }));
    tile.append(el('details', {}, el('summary', { textContent: `${versions.length} versions` }), list));
  }
  return tile;
}

async function renderDropItem(box, path, n) {
  let data;
  try {
    data = await api('GET', '/api/drops/item?path=' + encodeURIComponent(path));
    if (n !== dropsRender) return;
  } catch (err) {
    if (n !== dropsRender) return;
    if (err.status !== 404) throw err;
    box.replaceChildren(crumbs([['Drop finder', 'drops'], ['Not found']]),
      el('p', { className: 'empty', textContent: 'This item is not in the drop finder. It may come only from a source the finder does not cover yet.' }));
    return;
  }
  const t = dropsState.tier;
  const it = data.item;
  const note = el('p', { className: 'drop-note', textContent: `Chance per kill or per box for a level 60 hero on ${TIER_NAMES[t]}, before item find bonuses.` });
  // with a card the card names the item, so the note shares the row with the difficulty switch
  // an item no enemy or box drops (sold, crafted, a crafter's service) has no chance per kill and no difficulty
  const drops = data.sources.some((s) => s.kind === 'boss' || s.kind === 'box') || data.pools.length > 0;
  const tiers = drops ? [tierSwitch(() => renderDrops(['item', path]))] : [];
  const head = it.card ? el('div', { className: 'drop-title' }, ...(drops ? [note] : []), ...tiers)
    : el('div', { className: 'drop-title' }, dropIcon(it.path),
      el('h2', { className: it.rarity ? 'rarity-' + it.rarity : '', textContent: it.name }),
      ...(it.label ? [el('span', { className: 'tag', textContent: it.label })] : []), ...tiers);
  const parts = [crumbs([['Drop finder', 'drops'], ['Items', 'drops/items'], [it.name + (it.card && it.label ? ` (${it.label})` : '')]]),
    ...(it.card && !drops ? [] : [head]), ...(it.card || !drops ? [] : [note])];
  const panels = [];
  const seen = new Set(); // several entities can share a version's table: show each row once
  const shown = data.sources.filter((s) => s.p[t] > 0).sort((a, b) => b.p[t] - a.p[t]).filter((s) => {
    const key = [s.kind, s.name, s.where, s.p[t], s.labels.join(','), s.server].join('|');
    return !seen.has(key) && seen.add(key);
  });
  const bosses = shown.filter((s) => s.kind === 'boss');
  const boxes = shown.filter((s) => s.kind === 'box');
  const pools = data.pools.filter((p) => p.best[t] > 0);
  if (bosses.length) {
    const byName = new Map(); // versions in chance order, best first
    for (const s of bosses) byName.set(s.name, [...(byName.get(s.name) || []), s]);
    panels.push(el('section', { className: 'drop-panel' }, el('h3', { className: 'section-title', textContent: 'Bosses' }),
      el('div', { className: 'boss-tiles', role: 'list' }, ...[...byName].map(([name, versions]) => bossTile(name, versions, t)))));
    // every place, with the difficulties each drops on, not only the one picked above
    const lootInfo = placesPanel('Loot info', data.sources.filter((x) => x.kind === 'boss'), new Set(),
      'Where it drops, on each difficulty. Open a place to see every boss there.');
    if (lootInfo) panels.push(lootInfo);
  }
  const boxRow = (s) => el('div', { className: 'drop-row', role: 'listitem' }, dropIcon(s.id),
    el('div', {}, itemName({ name: s.name }), el('div', { className: 'sub' }, ...rowTags(s))), chanceCell(s.p[t], s.n[t]));
  if (boxes.length) panels.push(el('section', { className: 'drop-panel' }, el('h3', { className: 'section-title', textContent: 'Comes in' }),
    el('div', { role: 'list' }, ...boxes.map(boxRow))));
  panels.push(...shopPanels(data));
  parts.push(...(it.card ? [el('div', { className: 'item-layout' }, gameCard(it.card, it.path), el('div', { className: 'item-panels' }, ...panels))] : panels));
  if (pools.length) parts.push(el('h3', { className: 'section-title', textContent: 'Shared pools' }), ...pools.map((p) =>
    el('div', { className: 'pool-row' }, el('div', {}, el('b', { textContent: p.name }),
      el('div', { className: 'sub', textContent: `Shared by ${p.count} boss versions, the best one shown` })), chanceCell(p.best[t], 0))));
  if (!bosses.length && !boxes.length && !pools.length && !data.sources.some((s) => ONLY_FROM[s.kind])) {
    const others = TIER_NAMES.filter((_, i) => data.sources.some((s) => s.p[i] > 0) || data.pools.some((p) => p.best[i] > 0));
    parts.push(el('p', { className: 'empty', textContent: `No drops on ${TIER_NAMES[t]}.` + (others.length ? ` It drops on ${others.join(' and ')}.` : '') }));
  }
  box.replaceChildren(...parts);
  loadDropIcons().catch(() => {});
}

const priceText = (price) => (price || []).map((x) => `${x.amount.toLocaleString('en-US')} ${x.currency}`).join(' + ');
// the vendor ranks a row is stocked at; nothing for stock every rank has
const rankText = (r) => (r.to === r.from ? `Rank ${r.from}` : r.to ? `Ranks ${r.from}-${r.to}` : r.from > 1 ? `Rank ${r.from} and up` : '');
const serviceCost = (service) => (service && service.credits ? `${service.credits.toLocaleString('en-US')} Credits` : '');
const joinSub = (...parts) => parts.filter(Boolean).join(', ');

// Sold by, Made by and Also from: vendors with their chance per stock refresh, recipes with their ingredients,
// achievements and store entries. None of these follow the difficulty.
function shopPanels(data) {
  const of = (kind) => {
    const seen = new Set(); // two vendor types or entries with one name and one result show once
    const key = (s) => [s.name, s.from, s.to, s.p[0]].join('|');
    return data.sources.filter((s) => s.kind === kind && !seen.has(key(s)) && seen.add(key(s)));
  };
  const panel = (title, rows) => (rows.length ? [el('section', { className: 'drop-panel' }, el('h3', { className: 'section-title', textContent: title }),
    el('div', { role: 'list' }, ...rows))] : []);
  const price = priceText(data.item.price);
  // a crafter's service is always on offer once unlocked: no chance per refresh, its credit cost instead
  const service = data.item.service;
  const sold = of('vendor').sort((a, b) => a.name.localeCompare(b.name) || a.from - b.from).map((s) => el('div', { className: 'drop-row no-icon', role: 'listitem' },
    el('div', {}, itemName({ name: s.name }, () => goDrops('vendor', s.name)),
      el('div', { className: 'sub' }, joinSub(rankText(s), service ? serviceCost(service) : price),
        ...(s.event ? [el('span', { className: 'tag', textContent: 'during ' + s.event })] : []), ...rowTags(s))),
    ...(service ? [] : [el('div', { className: 'pct' }, el('b', { textContent: pct(s.p[0]) }), 'per refresh')])));
  const made = of('recipe').map((s) => el('div', { className: 'drop-row no-icon recipe-row', role: 'listitem' },
    el('div', {}, el('span', { className: 'name', textContent: s.name }), el('div', { className: 'sub', textContent: 'At the ' + s.crafter }),
      el('ul', { className: 'recipe-inputs' }, ...(s.inputs || []).map((i) => el('li', { textContent: (i.count > 1 ? i.count + ' x ' : '') + i.text })))),
    ...(s.p[0] < 0.995 ? [chanceCell(s.p[0], 0)] : [])));
  const also = [...of('achievement').map((s) => ['Achievement', s]), ...of('store').map((s) => ['In the store', s])].map(([what, s]) =>
    el('div', { className: 'drop-row no-icon', role: 'listitem' }, el('div', {}, el('span', { className: 'name', textContent: s.name }),
      el('div', { className: 'sub', textContent: what })), ...(s.p[0] < 0.995 ? [chanceCell(s.p[0], 0)] : [])));
  return [...panel(service ? 'Offered by' : 'Sold by', sold), ...panel('Made by', made), ...panel('Also from', also)];
}

async function renderDropVendor(box, name, n) {
  let data;
  try {
    data = await api('GET', '/api/drops/vendor?name=' + encodeURIComponent(name));
    if (n !== dropsRender) return;
  } catch (err) {
    if (n !== dropsRender) return;
    if (err.status !== 404) throw err;
    box.replaceChildren(crumbs([['Drop finder', 'drops'], ['Not found']]),
      el('p', { className: 'empty', textContent: 'This vendor is not in the drop finder.' }));
    return;
  }
  const events = [...new Set(data.tabs.map((x) => x.event).filter(Boolean))];
  const seen = new Set(); // two vendor types with one name stock the same rows once
  const all = data.tabs.flatMap((x) => x.rows).filter((r) => {
    const key = [r.path, r.from, r.to, r.p[0], r.labels.join(',')].join('|');
    return !seen.has(key) && seen.add(key);
  });
  const services = all.filter((r) => r.service);
  const stock = all.filter((r) => !r.service);
  const where = events.length ? ` Only in Avengers Tower during ${events.join(', ')}.` : '';
  const parts = [crumbs([['Drop finder', 'drops'], ['Vendors', 'drops/vendors'], [data.name]]),
    el('div', { className: 'drop-title' }, el('h2', { textContent: data.name })),
    el('p', { className: 'drop-note', textContent: (stock.length ? 'Chance an item is in stock after a refresh, for a level 60 hero.'
      : 'Services on offer once the vendor reaches the rank shown. Credit costs are before crafting cost bonuses.') + where })];
  const isItem = (r) => !r.random && r.path.includes('/');
  const row = (r, right) => el('div', { className: 'drop-row' + (isItem(r) ? '' : ' no-icon'), role: 'listitem' },
    ...(isItem(r) ? [dropIcon(r.path)] : []),
    el('div', {}, itemName(r, isItem(r) ? () => goDrops('item', r.path) : null),
      el('div', { className: 'sub' }, joinSub(rankText(r), r.service ? serviceCost(r.service) : priceText(r.price)), ...rowTags(r))),
    ...right);
  if (stock.length) {
    const rows = [...stock].sort((a, b) => a.from - b.from || b.p[0] - a.p[0] || a.name.localeCompare(b.name));
    parts.push(el('h3', { className: 'section-title', textContent: 'Stock' }),
      el('div', { role: 'list' }, ...rows.map((r) => row(r, [el('div', { className: 'pct' }, el('b', { textContent: pct(r.p[0]) }), r.p[0] < 0.995 ? 'per refresh' : 'always')]))));
  }
  // services under the crafting panel's tab names
  const order = data.tabs.flatMap((x) => x.tabs || []);
  const tabs = [...new Set(services.map((r) => r.service.tab || 'Services'))]
    .sort((a, b) => (order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99));
  for (const tab of tabs) {
    const rows = services.filter((r) => (r.service.tab || 'Services') === tab).sort((a, b) => a.from - b.from || a.name.localeCompare(b.name));
    parts.push(el('h3', { className: 'section-title', textContent: tabs.length > 1 || stock.length ? `${tab} services` : 'Services' }),
      el('div', { role: 'list' }, ...rows.map((r) => row(r, []))));
  }
  box.replaceChildren(...parts);
  loadDropIcons().catch(() => {});
}

// The version a boss page opens on: the linked one, unless it drops nothing on this difficulty (a regular patrol
// on Cosmic), so a boss page is never blank. Otherwise one where the boss drops its own items (a terminal's
// mid-fight Bullseye drops only gear, Kingpin is that terminal's boss), then one with a known place, then where
// people farm first: the Cosmic terminal, a Cosmic patrol, the other terminals, the rest.
function bossVersion(variants, t, id) {
  const drops = (x) => x.rows.some((r) => r.p[t] > 0);
  const own = (x) => (x.rows.some((r) => r.chase && r.p[t] > 0) ? 0 : 1);
  const farm = (x) => Math.min(...(x.places || []).map((pl) => (pl.version === 'Cosmic terminal' ? 0
    : pl.version === 'Cosmic' && pl.place.endsWith('-patrol') ? 1 : /terminal$/i.test(pl.version) ? 2 : 3)), 3);
  const linked = variants.findIndex((x) => x.ids.includes(id));
  const placed = (x) => ((x.places || []).some((pl) => pl.placeName) ? 0 : 1);
  const ranked = variants.map((x, i) => i).filter((i) => drops(variants[i]))
    .sort((a, b) => own(variants[a]) - own(variants[b]) || placed(variants[a]) - placed(variants[b])
      || farm(variants[a]) - farm(variants[b]) || a - b);
  return linked >= 0 && (drops(variants[linked]) || !ranked.length) ? linked : ranked[0] ?? 0;
}

async function renderDropBoss(box, name, id, n) {
  let data;
  try {
    data = await api('GET', '/api/drops/boss?name=' + encodeURIComponent(name));
    if (n !== dropsRender) return;
  } catch (err) {
    if (n !== dropsRender) return;
    if (err.status !== 404) throw err;
    box.replaceChildren(crumbs([['Drop finder', 'drops'], ['Not found']]),
      el('p', { className: 'empty', textContent: 'This boss is not in the drop finder.' }));
    return;
  }
  const drops = (x, i) => x.rows.some((r) => r.p[i] > 0);
  // a boss with nothing on this difficulty (the Heroic-only Danger Room bosses on Cosmic) shows one it drops on
  let offTier = '';
  if (!data.variants.some((x) => drops(x, dropsState.tier))) {
    const other = [0, 1, 2].find((i) => data.variants.some((x) => drops(x, i)));
    if (other !== undefined) {
      offTier = `${data.name} drops nothing on ${TIER_NAMES[dropsState.tier]}, so this shows ${TIER_NAMES[other]}.`;
      dropsState.tier = other; // for this visit only: not saved
    }
  }
  const t = dropsState.tier;
  const picked = bossVersion(data.variants, t, id);
  const v = data.variants[picked];
  const parts = [crumbs([['Drop finder', 'drops'], ['Bosses', 'drops/bosses'], [data.name]]),
    el('div', { className: 'drop-title' }, portrait(v.icon ? v.id : data.variants.find((x) => x.icon)?.id || v.id, data.name, true),
      el('h2', { textContent: data.name }), tierSwitch(() => renderDrops(['boss', name, v.id])))];
  // One button per place that drops anything on this difficulty (Location lists every place); a place with
  // several versions of its own (Green, Red and Cosmic terminals with different tables) gets a second row while
  // it is picked, in the game's order. Versions no place could be worked out for get no button.
  const placed = (x) => (x.places || []).some((pl) => pl.placeName);
  const shownVariants = data.variants.map((x, i) => i).filter((i) => placed(data.variants[i]) && (drops(data.variants[i], t) || i === picked));
  if (shownVariants.length > 1) {
    const placeOf = (x) => (x.places || []).find((pl) => pl.placeName);
    const groups = new Map();
    for (const i of shownVariants) {
      const pl = placeOf(data.variants[i]);
      const key = pl.place;
      if (!groups.has(key)) groups.set(key, { label: pl.placeName, items: [] });
      groups.get(key).items.push(i);
    }
    // a version within its place: its versions there ("Red terminal, Cosmic terminal"), else its hint
    const versionLabel = (x, key) => {
      const versions = [...new Set((x.places || []).filter((pl) => pl.place === key && pl.version).map((pl) => pl.version))];
      return versions.length ? versions.sort(byVersion).join(', ') : 'Standard';
    };
    const first = (x, key) => Math.min(...(x.places || []).filter((pl) => pl.place === key).map((pl) => VERSION_ORDER.indexOf(pl.version) + 1 || 99));
    const go = (i) => goDrops('boss', name, data.variants[i].id);
    const button = (text, pressed, onClick) => {
      const b = el('button', { type: 'button', textContent: text });
      b.setAttribute('aria-pressed', String(pressed));
      b.addEventListener('click', onClick);
      return b;
    };
    const current = [...groups].find(([, g]) => g.items.includes(picked));
    const chips = el('div', { className: 'variant-chips', role: 'group' });
    chips.setAttribute('aria-label', `Places ${data.name} drops on ${TIER_NAMES[t]}`);
    for (const [key, g] of groups) {
      const best = g.items.includes(picked) ? picked : g.items[bossVersion(g.items.map((i) => data.variants[i]), t)];
      chips.append(button(g.label + (g.items.length > 1 ? ` (${g.items.length})` : ''), g.items.includes(picked), () => go(best)));
    }
    const head = [el('h3', { className: 'section-title', textContent: 'Drops by place' }),
      el('p', { className: 'drop-note', textContent: `Where ${data.name} drops something on ${TIER_NAMES[t]}. Pick a place to see its drops below.` }), chips];
    if (current && current[1].items.length > 1) {
      const [key, g] = current;
      g.items.sort((a, b) => first(data.variants[a], key) - first(data.variants[b], key));
      const labels = g.items.map((i) => versionLabel(data.variants[i], key));
      const sub = el('div', { className: 'variant-chips variant-sub', role: 'group' });
      sub.setAttribute('aria-label', `Versions at ${g.label}`);
      g.items.forEach((i, k) => {
        const same = labels.filter((l) => l === labels[k]).length > 1; // number versions that share a label
        sub.append(button(same ? `${labels[k]} ${labels.slice(0, k + 1).filter((l) => l === labels[k]).length}` : labels[k], i === picked, () => go(i)));
      });
      head.push(el('p', { className: 'drop-note variant-sub-note', textContent: `${g.label} has ${g.items.length} versions of ${data.name} with different drops:` }), sub);
    }
    parts.push(el('section', { className: 'variant-head' }, ...head));
  }
  if (offTier) parts.push(el('p', { className: 'drop-note', textContent: offTier }));
  if (v.fight) parts.push(el('p', { className: 'drop-note', textContent: `Rolled once each time ${v.where.includes('shared with') ? 'the group is beaten' : 'the fight is won'}: ${v.where}.` }));
  else if (data.variants.length === 1 && v.where) parts.push(el('p', { className: 'drop-note', textContent: v.where }));
  const rows = v.rows.filter((r) => r.p[t] > 0).sort((a, b) => b.p[t] - a.p[t]);
  const isItem = (r) => !r.random && r.path.includes('/'); // Credits and the like are not items with an icon
  const line = (r) => el('div', { className: 'drop-row' + (isItem(r) ? '' : ' no-icon'), role: 'listitem' },
    ...(isItem(r) ? [dropIcon(r.path)] : []),
    el('div', {}, itemName(r, isItem(r) ? () => goDrops('item', r.path) : null), el('div', { className: 'sub' }, ...rowTags(r))),
    chanceCell(r.p[t], r.n[t]));
  const section = (title, list) => (list.length ? [el('h3', { className: 'section-title', textContent: title }), ...list] : []);
  const pools = {};
  for (const r of rows.filter((x) => x.pool)) {
    pools[r.pool] = pools[r.pool] || { count: 0, miss: 1 };
    pools[r.pool].count++;
    pools[r.pool].miss *= 1 - r.p[t];
  }
  const poolLines = Object.entries(pools).map(([pool, x]) => el('div', { className: 'pool-row' },
    el('div', {}, el('b', { textContent: pool }), el('div', { className: 'sub', textContent: `1 of ${x.count} items` })),
    chanceCell(1 - x.miss, 0)));
  const plain = rows.filter((r) => !r.pool);
  // gear others drop too, such as runes, rings and Uru-forged items, is left out: it drops everywhere
  const left = [...section(v.fight ? 'Every win' : 'Every kill', plain.filter((r) => r.chase && r.p[t] >= 0.5).map(line)),
    ...section('Chase drops', plain.filter((r) => r.chase && r.p[t] < 0.5).map(line))];
  const rest = plain.filter((r) => r.random || r.section === 'currency').length + poolLines.length;
  // where every version of the boss is, with the difficulties each one drops anything on
  const location = placesPanel('Location', data.variants.flatMap((x) => (x.places || []).map((pl) => ({
    ...pl, p: [0, 1, 2].map((i) => Math.max(0, ...x.rows.map((r) => r.p[i]))) }))), new Set((v.places || []).map((pl) => pl.place + '|' + pl.version)));
  const other = TIER_NAMES.filter((_, i) => drops(v, i));
  if (!left.length && !rest) parts.push(el('p', { className: 'empty', textContent: other.length
    ? `This version drops nothing on ${TIER_NAMES[t]}. It drops on ${other.join(' and ')}.` : `${data.name} drops nothing here.` }));
  if (left.length || location) parts.push(el('div', { className: 'boss-layout' }, el('div', {}, ...left), ...(location ? [location] : [])));
  const below = [section('Random gear and pools', [...plain.filter((r) => r.random).map(line), ...poolLines]),
    section('Currencies and crafting', plain.filter((r) => !r.random && r.section === 'currency').map(line))].filter((x) => x.length);
  if (below.length) parts.push(el('div', { className: 'drop-cols boss-below' }, ...below.map((x) => el('div', {}, ...x))));
  box.replaceChildren(...parts);
  loadDropIcons().catch(() => {});
}

async function renderDropPlace(box, id, n) {
  let data;
  try {
    data = await api('GET', '/api/drops/place?id=' + encodeURIComponent(id));
    if (n !== dropsRender) return;
  } catch (err) {
    if (n !== dropsRender) return;
    if (err.status !== 404) throw err;
    box.replaceChildren(crumbs([['Drop finder', 'drops'], ['Not found']]),
      el('p', { className: 'empty', textContent: 'This place is not in the drop finder.' }));
    return;
  }
  const rank = (v) => ['', 'Green terminal', 'Red terminal', 'Cosmic terminal'].indexOf(v.version) + 1 || 9; // others last
  for (const b of data.bosses) b.versions.sort((x, y) => rank(x) - rank(y));
  const tiles = data.bosses.map((b) => bossTile(b.name, b.versions.map((v) => ({ ...v, iconKey: b.versions.find((x) => x.icon)?.id }))));
  box.replaceChildren(crumbs([['Drop finder', 'drops'], ['Places', 'drops/places'], [data.name]]),
    el('div', { className: 'drop-title' }, el('h2', { textContent: data.name })),
    el('p', { className: 'drop-note', textContent: `${data.group}. Every boss found here; open one for what it drops.` }),
    el('div', { className: 'boss-tiles', role: 'list' }, ...tiles));
  loadDropIcons().catch(() => {});
}

// ---- Achievements: the hunter's ticklist and the rewards they give ----

let achData = null; // {categories, achievements, done}, loaded once
const achTrail = []; // the Achievements pages visited, for Back
// where the hunter is: a category and subcategory (-1 = all), or the Hidden achievements section
const achState = { cat: -1, sub: -1, openCat: -1, hidden: false, show: 'all', q: '', rq: '', expanded: new Set(), open: new Set(), adv: { open: false, terms: [], ticks: {} } };
const ACH_REWARD_TYPES = [['gear', 'Unique or gear item'], ['costume', 'Costume'], ['coffer', 'Coffer of Achievement'],
  ['orb', 'Infinity Orb'], ['commendation', 'Commendation'], ['title', 'Title'], ['credits', 'Credits'],
  ['boost', 'Boosts and tokens'], ['none', 'No reward']];

const achDone = (a) => achData.done.has(a.id);
// secret achievements (shown in game once earned) and the ones the window never lists go in Hidden achievements;
// a ticked secret one shows in its own category too, as in game
const inCategories = (a) => a.vis === 1 || a.vis === 3 || (a.vis === 4 && achDone(a));
const inHidden = (a) => a.vis === 2 || a.vis === 4;
const achWhere = (a) => `${achData.categories[a.cat].name} › ${achData.categories[a.cat].subs[a.sub]}`;
const achText = (a) => [a.name, a.desc.map(([t]) => t).join(''), a.reward, ...a.items.map((i) => i.name), ...a.objectives.map((o) => o.name)];
// an objective that is another achievement shares that achievement's tick
const stepId = (o) => o.dep || o.id;

// the same rule as the server's matchWords: every word in some field
const matchAll = (q, fields) => {
  const all = fields.join('\n').toLowerCase();
  return q.toLowerCase().split(/\s+/).filter(Boolean).every((w) => all.includes(w));
};

function achPercent(list) {
  const total = list.reduce((s, a) => s + a.points, 0);
  if (!list.length) return '';
  const got = total ? list.filter(achDone).reduce((s, a) => s + a.points, 0) / total : list.filter(achDone).length / list.length;
  return Math.floor(got * 100) + '%';
}

async function loadAchievements() {
  if (!achData) {
    const d = await api('GET', '/api/achievements');
    achData = { ...d, done: new Set(d.done) };
  }
}

async function renderAchievements(route) {
  const box = $('#ach');
  const here = currentHash();
  if (achTrail.at(-1) !== here) achTrail.push(here);
  await loadAchievements();
  const rewards = route[0] === 'rewards';
  const tabs = el('div', { className: 'tabs', role: 'tablist' });
  for (const [id, text, hash] of [['hunter', 'Achievement hunter', 'achievements'], ['rewards', 'Rewards', 'achievements/rewards']]) {
    const b = el('button', { type: 'button', role: 'tab', textContent: text });
    b.setAttribute('aria-selected', String(rewards === (id === 'rewards')));
    b.addEventListener('click', () => { navigate(hash); });
    tabs.append(b);
  }
  let jump = null;
  if (route[0] === 'a') { // one achievement, from a Rewards row: open its place in the hunter
    jump = achData.achievements.find((a) => a.id === Number(route[1]));
    if (jump) {
      achState.q = '';
      achState.show = 'all';
      achState.adv.ticks = {};
      Object.assign(achState, inCategories(jump) ? { hidden: false, cat: jump.cat, sub: jump.sub, openCat: jump.cat } : { hidden: true, cat: -1, sub: -1 });
    }
  }
  const head = jump ? [crumbs([['Achievements', 'achievements'], ['Rewards', 'achievements/rewards'], [jump.name]], achTrail, 'achievements')] : [];
  box.replaceChildren(...head, tabs, rewards ? achRewardsTab() : achHunterTab(jump));
  fitHunter();
  loadDropIcons().catch(() => {});
  if (jump) box.querySelector(`[data-ach="${jump.id}"]`)?.scrollIntoView({ block: 'center' });
}

// The hunter's two columns each scroll on their own (the one under the mouse): the layout fills the page below the tabs
function fitHunter() {
  const hunter = document.querySelector('#ach .ach-hunter');
  if (!hunter) return;
  const page = $('#ach');
  const below = page.getBoundingClientRect().bottom - hunter.getBoundingClientRect().top - parseFloat(getComputedStyle(page).paddingBottom);
  hunter.style.setProperty('--ach-h', Math.max(320, Math.floor(below + page.scrollTop)) + 'px');
}
window.addEventListener('resize', () => { fitHunter(); });

function achHunterTab(jump) {
  const wrap = el('div', { className: 'ach-hunter' });
  const side = el('nav', { className: 'ach-side', ariaLabel: 'Achievement categories' });
  const main = el('div', { className: 'ach-main' });
  wrap.append(side, main);
  const all = achData.achievements;
  const sideButton = (text, list, on, pick, sub) => {
    const b = el('button', { type: 'button', className: 'ach-cat' + (sub ? ' sub' : '') },
      el('span', { textContent: text }), el('small', { textContent: achPercent(list) }));
    if (on) b.setAttribute('aria-current', 'true');
    b.dataset.pick = JSON.stringify(pick);
    b.addEventListener('click', () => {
      // a category's own button opens its subcategories, and closes them again when they are open
      const close = !sub && pick.cat >= 0 && achState.openCat === pick.cat && achState.cat === pick.cat;
      Object.assign(achState, pick, sub ? {} : { openCat: close ? -1 : pick.cat });
      drawAll();
      // the list is drawn again: keep focus on the same button, or the Windows app jumps to the top
      [...side.querySelectorAll('.ach-cat')].find((x) => x.dataset.pick === b.dataset.pick)?.focus({ preventScroll: true });
    });
    return b;
  };
  const drawSide = () => {
    const top = side.scrollTop;
    const shown = all.filter(inCategories);
    side.replaceChildren(sideButton('All achievements', shown, !achState.hidden && achState.cat < 0, { hidden: false, cat: -1, sub: -1 }));
    achData.categories.forEach((c, ci) => {
      const inCat = shown.filter((a) => a.cat === ci);
      if (!inCat.length) return;
      const open = !achState.hidden && achState.openCat === ci;
      const picked = !achState.hidden && achState.cat === ci;
      const head = sideButton(c.name, inCat, picked && (achState.sub < 0 || !open), { hidden: false, cat: ci, sub: -1 });
      head.setAttribute('aria-expanded', String(open));
      side.append(head);
      if (open) {
        side.append(el('div', { className: 'ach-break' })); // narrow windows: the subcategories start their own row
        c.subs.forEach((s, si) => {
          const inSub = inCat.filter((a) => a.sub === si);
          if (inSub.length) side.append(sideButton(s, inSub, achState.sub === si, { hidden: false, cat: ci, sub: si }, true));
        });
      }
    });
    side.append(sideButton('Hidden achievements', all.filter(inHidden), achState.hidden, { hidden: true, cat: -1, sub: -1 }));
    side.scrollTop = top;
  };

  const input = el('input', { type: 'search', value: achState.q, autocomplete: 'off', placeholder: 'Search achievements, bosses, rewards...' });
  input.setAttribute('aria-label', 'Search achievements');
  const score = el('div', { className: 'ach-score' });
  const bar = el('div', { className: 'ach-bar' }, el('i'));
  const show = el('div', { className: 'tier-switch', role: 'radiogroup', ariaLabel: 'Show' });
  for (const [id, text] of [['all', 'All'], ['todo', 'Not done'], ['done', 'Done']]) {
    const b = el('button', { type: 'button', role: 'radio', textContent: text });
    b.setAttribute('aria-checked', String(achState.show === id));
    b.addEventListener('click', () => {
      achState.show = id;
      show.querySelectorAll('button').forEach((x) => x.setAttribute('aria-checked', String(x === b)));
      drawList();
    });
    show.append(b);
  }
  const points = [...new Set(all.map((a) => a.points))].sort((x, y) => x - y);
  const groups = [
    { key: 'rtype', label: 'Reward type', options: ACH_REWARD_TYPES },
    { key: 'points', label: 'Points', options: points.map((p) => [String(p), String(p)]) },
    { key: 'status', label: 'Status', options: [['done', 'Done'], ['todo', 'Not done']] },
    { key: 'kind', label: 'Kind', options: [['tiered', 'Tiered'], ['secret', 'Secret'], ['party', 'Party']] },
  ];
  const tiered = new Set(all.filter((a) => a.parent).flatMap((a) => [a.id, a.parent]));
  const panelSlot = el('div');
  const drawPanel = () => panelSlot.replaceChildren(...(achState.adv.open ? [advancedPanel({ groups, terms: false, state: achState.adv,
    onChange: (redraw) => { if (redraw) drawPanel(); drawList(); } })] : []));
  const toggle = advancedToggle(achState.adv, () => { toggle.setAttribute('aria-expanded', String(achState.adv.open)); drawPanel(); drawList(); });
  const list = el('div', { className: 'ach-list', role: 'list' });

  const passes = (a) => {
    const t = achState.adv.ticks;
    const any = (key, test) => !(t[key] || []).length || t[key].some(test);
    return (achState.show === 'all' || (achState.show === 'done') === achDone(a))
      && any('rtype', (v) => a.rtypes.includes(v))
      && any('points', (v) => Number(v) === a.points)
      && any('status', (v) => (v === 'done') === achDone(a))
      && any('kind', (v) => (v === 'tiered' ? tiered.has(a.id) : v === 'secret' ? a.vis === 4 : a.party));
  };
  const drawScore = () => {
    const counted = all.filter((a) => inCategories(a) || a.vis === 4);
    const total = counted.reduce((s, a) => s + a.points, 0);
    const got = counted.filter(achDone).reduce((s, a) => s + a.points, 0);
    score.replaceChildren(el('b', { textContent: got.toLocaleString('en-US') }), ` / ${total.toLocaleString('en-US')} points`);
    bar.firstChild.style.width = (total ? (got / total) * 100 : 0) + '%';
  };
  const drawList = () => {
    const q = input.value.trim();
    achState.q = input.value;
    const pool = q ? all.filter((a) => inCategories(a) || inHidden(a)).filter((a) => matchAll(q, achText(a)))
      : achState.hidden ? all.filter(inHidden)
        : all.filter((a) => inCategories(a) && (achState.cat < 0 || a.cat === achState.cat) && (achState.sub < 0 || a.sub === achState.sub));
    const rows = pool.filter(passes);
    where = Boolean(q) || achState.hidden || achState.cat < 0;
    list.replaceChildren(...(rows.length ? rows.map((a) => achRow(a, where, refresh))
      : [el('p', { className: 'empty', textContent: q ? `No achievements match "${q}".` : 'No achievements match these filters.' })]));
    if (jump) list.querySelector(`[data-ach="${jump.id}"]`)?.classList.add('jumped');
    loadDropIcons().catch(() => {});
  };
  // after a tick: redraw only the rows it touches (the achievement, and any whose objective it is), keeping the
  // scroll and the focus where they were; rebuilding the list loses the clicked box and WebView2 jumps to the top
  let where = false;
  const refresh = (id, rowId) => {
    drawSide();
    drawScore();
    const scrollers = [$('#ach'), main];
    const tops = scrollers.map((x) => x.scrollTop);
    for (const old of list.querySelectorAll('.ach')) {
      const a = all.find((x) => x.id === Number(old.dataset.ach));
      if (!a || (a.id !== id && !a.objectives.some((o) => stepId(o) === id))) continue;
      if (passes(a)) old.replaceWith(achRow(a, where, refresh)); else old.remove();
    }
    list.querySelector(`[data-ach="${rowId}"] [data-tick="${id}"]`)?.focus({ preventScroll: true });
    scrollers.forEach((x, i) => { x.scrollTop = tops[i]; });
    loadDropIcons().catch(() => {});
  };
  const drawAll = () => { input.value = ''; drawSide(); drawList(); };
  let timer;
  input.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(drawList, 150); });
  main.append(el('div', { className: 'drops-bar' }, input, toggle, show), panelSlot,
    el('div', { className: 'ach-top' }, score, bar), list);
  drawSide();
  drawPanel();
  drawScore();
  drawList();
  return wrap;
}

function achRow(a, where, changed) {
  const done = achDone(a);
  const tick = el('input', { type: 'checkbox', className: 'ach-tick', checked: done });
  tick.dataset.tick = String(a.id);
  tick.setAttribute('aria-label', (done ? 'Untick ' : 'Tick ') + a.name);
  tick.addEventListener('change', guarded(async () => {
    const list = await api('PUT', '/api/achievements/done', { id: a.id, done: tick.checked });
    achData.done = new Set(list);
    changed(a.id, a.id);
  }));
  const desc = el('div', { className: 'ach-desc' }, ...a.desc.map(([t, c]) => (c ? el('em', { style: `color:#${c}`, textContent: t }) : t)));
  const parent = a.parent && achData.achievements.find((x) => x.id === a.parent);
  const reward = a.reward || a.items.length ? el('div', { className: 'ach-reward' }, 'Reward: ',
    ...(a.items.length === 1 ? [achItemLink(a.items[0], a.reward || a.items[0].name)]
      : a.items.length ? a.items.flatMap((it, i) => [...(i ? [', '] : []), achItemLink(it, it.name)]) : [a.reward])) : '';
  const steps = a.objectives || [];
  const open = steps.length > 0 && achState.expanded.has(a.id);
  const right = el('div', { className: 'ach-right' }, el('span', { className: 'ach-points', textContent: `${a.points} pts` }), tick);
  let body;
  if (steps.length) { // the game's + / - : the row opens to list what the achievement asks for
    const got = steps.filter((o) => achData.done.has(stepId(o))).length;
    const more = el('button', { type: 'button', className: 'ach-more', textContent: open ? '\u2212' : '+', title: open ? 'Hide the objectives' : 'Show the objectives' });
    more.setAttribute('aria-expanded', String(open));
    more.setAttribute('aria-label', (open ? 'Hide' : 'Show') + ` the ${steps.length} objectives of ${a.name}`);
    more.addEventListener('click', () => {
      if (achState.expanded.has(a.id)) achState.expanded.delete(a.id); else achState.expanded.add(a.id);
      row.replaceWith(achRow(a, where, changed));
    });
    right.prepend(more);
    body = el('div', { className: 'ach-steps-wrap' }, el('div', { className: 'ach-steps-count', textContent: `${got} / ${steps.length} objectives` }));
    if (open) {
      body.append(el('div', { className: 'ach-steps' }, ...steps.map((o) => {
        const box = el('input', { type: 'checkbox', className: 'ach-tick small', checked: achData.done.has(stepId(o)) });
        box.dataset.tick = String(stepId(o));
        box.setAttribute('aria-label', (box.checked ? 'Untick ' : 'Tick ') + o.name);
        box.addEventListener('change', guarded(async () => {
          achData.done = new Set(await api('PUT', '/api/achievements/done', { id: stepId(o), done: box.checked }));
          changed(stepId(o), a.id);
        }));
        let label = el('span', { textContent: o.name });
        if (o.dep) { // another achievement: open it in the hunter
          label = el('button', { type: 'button', className: 'link', textContent: o.name, title: 'Open this achievement' });
          label.addEventListener('click', () => { navigate('achievements/a/' + o.dep); });
        } else if (o.item) { // an item to loot: open it in the drop finder
          label = el('button', { type: 'button', className: 'link', textContent: o.name, title: 'Where it drops' });
          label.addEventListener('click', () => { achGoItem(o.item.path); });
        }
        return el('label', { className: 'ach-step' + (box.checked ? ' done' : '') }, box, label);
      })));
    }
  }
  const row = el('div', { className: 'ach' + (done ? ' done' : '') + (open ? ' open' : ''), role: 'listitem' },
    dropIcon('ach:' + a.id),
    el('div', { className: 'ach-body' },
      el('div', { className: 'ach-name', textContent: a.name }),
      ...(where ? [el('div', { className: 'ach-where', textContent: achWhere(a) + (a.vis === 4 ? ' (secret)' : '') })] : []),
      desc,
      ...(parent ? [el('div', { className: 'ach-tier', textContent: 'Next step after ' + parent.name })] : []),
      reward, ...(body ? [body] : [])),
    right);
  row.dataset.ach = String(a.id);
  if (steps.length) { // the name opens the row too
    const name = row.querySelector('.ach-name');
    name.classList.add('opens');
    name.addEventListener('click', () => { right.querySelector('.ach-more').click(); });
  }
  return row;
}

// an item page opened from here: the drop finder's Back returns to this page
const achGoItem = (path) => { dropsTrail.push(currentHash()); goDrops('item', path); };

function achItemLink(it, text) {
  const b = el('button', { type: 'button', className: 'link', textContent: text });
  b.addEventListener('click', () => { achGoItem(it.path); });
  return b;
}

function achRewardsTab() {
  const wrap = el('div', { className: 'ach-rewards' });
  const by = new Map(); // reward item path -> {item, achievements}
  for (const a of achData.achievements) {
    if (!(inCategories(a) || inHidden(a))) continue;
    for (const it of a.items) {
      if (!by.has(it.path)) by.set(it.path, { item: it, list: [] });
      by.get(it.path).list.push(a);
    }
  }
  const rows = [...by.values()].sort((x, y) => x.list.length - y.list.length || x.item.name.localeCompare(y.item.name));
  const input = el('input', { type: 'search', value: achState.rq, autocomplete: 'off', placeholder: 'Search rewards and achievements...' });
  input.setAttribute('aria-label', 'Search rewards');
  const list = el('div', { role: 'list' });
  const draw = () => {
    const q = input.value.trim();
    achState.rq = input.value;
    const shown = rows.filter((r) => !q || matchAll(q, [r.item.name, ...r.list.map((a) => a.name)]));
    list.replaceChildren(...(shown.length ? shown.map((r) => {
      const open = achState.open.has(r.item.path);
      const more = el('button', { type: 'button', className: 'ach-count', textContent: r.list.length === 1 ? 'Given by 1 achievement' : `Given by ${r.list.length} achievements` });
      more.setAttribute('aria-expanded', String(open));
      const kids = el('div', { className: 'ach-givers', hidden: !open }, ...r.list.map((a) => {
        const b = el('button', { type: 'button', className: 'link', textContent: a.name });
        b.addEventListener('click', () => { navigate('achievements/a/' + a.id); });
        return el('div', {}, b, el('span', { className: 'ach-giver-where', textContent: ` ${achWhere(a)}, ${a.points} pts` + (achDone(a) ? ', done' : '') }));
      }));
      more.addEventListener('click', () => {
        if (achState.open.has(r.item.path)) achState.open.delete(r.item.path); else achState.open.add(r.item.path);
        more.setAttribute('aria-expanded', String(!open));
        draw();
      });
      return el('div', { className: 'ach-reward-row', role: 'listitem' },
        el('div', { className: 'drop-row' }, dropIcon(r.item.path), el('div', {}, itemName(r.item, () => achGoItem(r.item.path))), more), kids);
    }) : [el('p', { className: 'empty', textContent: `No rewards match "${q}".` })]));
    loadDropIcons().catch(() => {});
  };
  let timer;
  input.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(draw, 150); });
  wrap.append(el('div', { className: 'drops-bar' }, input),
    el('p', { className: 'drop-note', textContent: 'What achievements give, the rarest rewards first. Open a reward to see the achievements that give it.' }), list);
  draw();
  return wrap;
}

for (const link of document.querySelectorAll('#sidebar a[data-page]')) {
  const go = () => { navigate(link.dataset.page); };
  link.addEventListener('click', go);
  link.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); } });
}

guarded(async () => {
  refreshStatus();
  const [p, groups, cats, heroes] = await Promise.all([api('GET', '/api/profiles'), api('GET', '/api/groups'),
    api('GET', '/api/rarity-categories'), api('GET', '/api/heroes'), loadFilter()]);
  profiles = p;
  renderProfiles();
  heroList = heroes;
  rarityCategories = cats;
  groupList = groups;
  // Earlier versions kept one setting per group; spread them over the group's items, which can now be changed one by one.
  const old = Object.entries(filter.groups);
  if (old.length) {
    for (const [id, gf] of old) {
      const g = groupList.find((x) => x.id === id);
      for (const flag of ['hide', 'name', 'sound']) if (g && gf[flag]) await setGroup(g, flag, true);
    }
    filter.groups = {};
    await api('PUT', '/api/filter', filter);
  }
  renderAll();
  await showPage();
  setInterval(refreshStatus, 5000);
  checkUpdates(true);
})();
