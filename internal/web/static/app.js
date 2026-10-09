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
const PAGES = ['search', 'groups', 'rarity', 'sound', 'filter', 'settings', 'backups'];
const OFF = { Glow: false, Model: false, Name: false };
const SOUND_RARITIES = ['Cosmic', 'Unique']; // the only rarities that set their own drop sound
const emptyFilter = () => ({ items: {}, looks: {}, groups: {}, rarities: {}, raritySounds: {}, rarityHide: {} });
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

function showPage() {
  const page = PAGES.includes(location.hash.slice(1)) ? location.hash.slice(1) : 'search';
  for (const p of PAGES) $('#page-' + p).hidden = p !== page;
  for (const link of document.querySelectorAll('#sidebar a')) {
    if (link.dataset.page === page) link.setAttribute('aria-current', 'page');
    else link.removeAttribute('aria-current');
  }
  if (page === 'search') $('#q').focus();
  if (page === 'filter') return renderFilter();
  if (page === 'settings') return loadSettings();
  if (page === 'sound') return loadSound();
  return undefined;
}

async function saveFilter() {
  await api('PUT', '/api/filter', filter);
  $('#dirty').hidden = false;
  renderCount();
}

function filterSize() {
  const on = (o) => Object.values(o).filter((v) => v === true || (v && (v.hide || v.name || v.sound || v.Glow || v.Model || v.Name))).length;
  return on(filter.items) + on(filter.looks) + on(filter.rarities) + on(filter.raritySounds)
    + Object.values(filter.rarityHide).filter((r) => r.length).length;
}

function renderCount() {
  const n = filterSize();
  $('#count').textContent = n;
  $('#count').hidden = n === 0;
}

function switchControl(label, checked, disabled, title, onChange) {
  const input = el('input', { type: 'checkbox', checked, disabled });
  input.addEventListener('change', guarded(() => onChange(input.checked)));
  return el('label', { className: 'switch' + (disabled ? ' disabled' : ''), title }, input, el('span', { className: 'track' }), label);
}

async function setItem(info, change) {
  const next = { hide: false, name: false, sound: false, ...(filter.items[info.key] || {}), ...change };
  if (info.sharedWith > 0 && change.hide === false) next.name = false; // a visible item shows its look's name
  if (next.hide) next.sound = false; // a hidden item cannot be heard either
  if (next.hide || next.name || next.sound) filter.items[info.key] = next;
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
          ? el('a', { className: 'link', href: '#rarity', textContent: 'Their glow is set by rarity' })
          : switchControl(`Hide glow for all ${n}`, look.Glow, !info.canGlow, '', (v) => setLook(info.type, { Glow: v })),
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

  const switches = [switchControl('Hide item', f.hide, false, '', (v) => setItem(info, { hide: v }))];
  const nameBlocked = shared && !f.hide && !f.name;
  switches.push(switchControl('Hide name', f.name, nameBlocked || !info.canName,
    nameBlocked ? `Looks the same as ${info.sharedWith} other items: hide the item to hide its name, or use "Looks the same" below` : '',
    (v) => setItem(info, { name: v })));
  if (shared) switches.push(el('span')); // keeps every row's switches in the same columns
  else {
    switches.push(info.rarityGlow
      ? el('a', { className: 'link', href: '#rarity', title: 'Open Rarity', textContent: 'Glow set by rarity' })
      : switchControl('Hide glow', (filter.looks[info.type] || OFF).Glow, !info.canGlow, '', (v) => setLook(info.type, { Glow: v })));
  }
  const hidden = f.hide || lookHidden;
  switches.push(info.soundByRarity
    ? el('a', { className: 'link', href: '#rarity', title: 'Open Rarity', textContent: 'Sound set by rarity' })
    : switchControl('Play sound', f.sound && !hidden, hidden, hidden ? 'Hidden items play no sound' : 'Play an alert when this item drops',
      (v) => setItem(info, { sound: v })));

  const active = f.hide || f.name || f.sound || lookHidden;
  const row = el('div', { className: 'row' + (active ? ' active' : ''), role: 'listitem' }, title,
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
    if (next.hide || next.name || next.sound) filter.items[m.key] = next;
    else delete filter.items[m.key];
  }
}

// groupHas reports whether every item of a group has a setting on.
const groupHas = (g, flag) => g.keys.length > 0 && g.keys.every((k) => filter.items[k] && filter.items[k][flag]);

function memberList(g) {
  const box = el('div', { className: 'members' });
  const fill = async () => {
    const members = await loadMembers(g.id);
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
  return { box, fill };
}

function renderGroups() {
  $('#groups').replaceChildren(...groupList.map((g) => {
    const f = { hide: groupHas(g, 'hide'), name: groupHas(g, 'name'), sound: groupHas(g, 'sound') };
    const set = async (flag, value) => {
      await setGroup(g, flag, value);
      await saveFilter();
      renderAll();
    };
    const open = openGroups.has(g.id);
    const toggle = el('button', { className: 'expand', type: 'button', 'aria-expanded': String(open) },
      el('span', { className: 'chevron', textContent: open ? '\u25be' : '\u25b8' }),
      el('span', { className: 'name', textContent: g.label }),
      el('span', { className: 'count', textContent: `${g.count} items` }));
    toggle.setAttribute('aria-expanded', String(open));
    toggle.addEventListener('click', guarded(async () => {
      if (openGroups.has(g.id)) {
        openGroups.delete(g.id);
        delete groupOrder[g.id]; // re-sort with hidden items first next time it opens
      } else openGroups.add(g.id);
      renderGroups();
    }));
    const some = g.keys.some((k) => filter.items[k] && (filter.items[k].hide || filter.items[k].name || filter.items[k].sound));
    const row = el('div', { className: 'row' + (some ? ' active' : ''), role: 'listitem' },
      el('div', { className: 'item' }, toggle, el('div', { className: 'sub', textContent: g.note })),
      el('div', { className: 'flags' },
        switchControl('Hide items', f.hide, false, 'Switch on Hide item for every item here; switch single items back afterwards', (v) => set('hide', v)),
        switchControl('Hide names', f.name, !g.namesAlone && !f.hide,
          !g.namesAlone && !f.hide ? 'Some of these items look the same as items outside this group: hide the items to hide their names' : '',
          (v) => set('name', v)),
        switchControl('Play sound', f.sound, f.hide, f.hide ? 'Hidden items play no sound' : 'Switch on Play sound for every item here',
          (v) => set('sound', v))));
    if (!open) return row;
    const { box, fill } = memberList(g);
    row.append(box);
    fill().catch((err) => showToast([err.message], true));
    return row;
  }));
}

async function toggle(map, key, value) {
  if (value) map[key] = true;
  else delete map[key];
  await saveFilter();
  renderAll();
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
}

function renderAll() {
  renderCount();
  renderResults();
  renderGroups();
  renderRarities();
  renderRarityGrid();
  if (!$('#page-filter').hidden) renderFilter();
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
  const next = { ...emptyFilter(), ...Object.fromEntries(Object.entries(shared.filter).filter(([k, v]) => k in emptyFilter() && v && typeof v === 'object')) };
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
}

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

$('#sound-play').addEventListener('click', guarded(async () => {
  if (!sound) await loadSound();
  await new Audio('data:audio/wav;base64,' + sound.wav).play();
}));
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

async function refreshStatus() {
  try {
    const s = await api('GET', '/api/status');
    if (s.version) $('#version').textContent = 'Version ' + s.version;
    $('#status .dot').className = 'dot ' + (!s.gameFound ? 'bad' : s.gameRunning ? 'warn' : 'ok');
    $('#status-text').textContent = !s.gameFound ? 'Game folder not found' : s.gameRunning ? 'Close the game to apply' : 'Game found';
    $('#status').title = s.gameDir;
    if (!busy) $('#apply').disabled = $('#restore').disabled = !s.gameFound || s.gameRunning;
  } catch {
    $('#status .dot').className = 'dot bad';
    $('#status-text').textContent = 'Filter app stopped';
    $('#apply').disabled = $('#restore').disabled = true;
  }
}

async function run(url, doneWord) {
  busy = true;
  $('#apply').disabled = $('#restore').disabled = true;
  try {
    const r = await api('POST', url);
    const warnings = r.warnings || [];
    if (url === '/api/restore') {
      filter = emptyFilter();
      renderAll();
    }
    $('#dirty').hidden = url === '/api/apply' || filterSize() === 0;
    showToast([`${doneWord}: ${r.changed} game file${r.changed === 1 ? '' : 's'} changed.`, ...warnings], warnings.length > 0);
  } catch (err) {
    showToast([err.status === 409 ? 'Close Marvel Heroes Omega, then try again.' : err.message], true);
  } finally {
    busy = false;
    refreshStatus();
  }
}

$('#apply').addEventListener('click', () => run('/api/apply', 'Applied'));
$('#restore').addEventListener('click', () => {
  if (confirm('Put every game file back to its original and clear your filter?')) run('/api/restore', 'Restored');
});
window.addEventListener('hashchange', guarded(showPage));

guarded(async () => {
  refreshStatus();
  const [f, groups, cats] = await Promise.all([api('GET', '/api/filter'), api('GET', '/api/groups'), api('GET', '/api/rarity-categories')]);
  rarityCategories = cats;
  filter = { ...emptyFilter(), ...Object.fromEntries(Object.entries(f).filter(([, v]) => v)) };
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
})();
