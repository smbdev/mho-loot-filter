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
const PAGES = ['search', 'groups', 'heroes', 'rarity', 'sound', 'filter', 'settings', 'tweaks', 'mutes', 'backups'];
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
  if (page === 'tweaks') return loadTweaks();
  if (page !== 'mutes' && previewing) {
    preview.pause();
    previewing = '';
  }
  if (page === 'mutes') return loadMutes();
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
  button.addEventListener('click', () => { location.hash = page; });
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

  const active = f.hide || f.name || f.sound || lookHidden;
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
    if (next.hide || next.name || next.sound) filter.items[m.key] = next;
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
  const some = g.keys.some((k) => filter.items[k] && (filter.items[k].hide || filter.items[k].name || filter.items[k].sound));
  const row = el('div', { className: 'row' + (some ? ' active' : ''), role: 'listitem' },
    el('div', { className: 'item' }, toggle, el('div', { className: 'sub', textContent: g.note })),
    el('div', { className: 'flags' },
      switchControl('Hide items', f.hide, false, 'Switch on Hide item for every item here; switch single items back afterwards', (v) => set('hide', v), 'hide|' + g.id),
      switchControl('Hide names', f.name, !g.namesAlone && !f.hide,
        !g.namesAlone && !f.hide ? 'Some of these items look the same as items outside this group: hide the items to hide their names' : '',
        (v) => set('name', v), 'name|' + g.id),
      switchControl('Play sound', f.sound, f.hide, f.hide ? 'Hidden items play no sound' : 'Switch on Play sound for every item here',
        (v) => set('sound', v), 'sound|' + g.id)));
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
}

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
  location.hash = 'filter';
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
  { key: 'fps', name: 'Frame rate limit', sub: 'The game stops at 200 frames per second, and its Options only have VSync. '
      + 'A lower limit keeps your PC cooler and quieter, a higher one suits fast screens.',
    options: [[0, 'Game default (200)'], [30, '30'], [60, '60'], [120, '120'], [144, '144'], [165, '165'], [240, '240'], [360, '360'], [-1, 'No limit']] },
  { key: 'skipIntro', name: 'Startup videos', label: 'Skip videos', sub: 'Goes straight to the login screen instead of playing the logo videos first.' },
  { key: 'textureMB', name: 'Texture memory', sub: 'The game keeps 160 MB of textures loaded, little for today\'s graphics cards. '
      + 'More means fewer blurry textures sharpening as you play. Pick at most half of your graphics card\'s memory.',
    options: [[0, 'Game default (160 MB)'], [512, '512 MB'], [1024, '1 GB'], [2048, '2 GB'], [4096, '4 GB']] },
  { key: 'lowLag', name: 'Input lag', label: 'Lower', sub: 'The game shows each frame one frame late to gain speed. '
      + 'Lower makes clicks and powers feel more direct, for a few frames per second less.' },
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
      control.addEventListener('change', guarded(() => setTweak(t.key, Number(control.value))));
    } else {
      control = switchControl(t.label, tweaks[t.key], false, '', (on) => setTweak(t.key, on), 'tweak|' + t.key);
    }
    const on = t.options ? tweaks[t.key] !== 0 : tweaks[t.key];
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
      if (location.hash === '#tweaks') await loadTweaks();
      if (location.hash === '#mutes') await loadMutes();
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
window.addEventListener('hashchange', guarded(showPage));
for (const link of document.querySelectorAll('#sidebar a[data-page]')) {
  const go = () => { location.hash = link.dataset.page; };
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
