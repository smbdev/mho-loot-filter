"""Builds internal/db/itemdb.json from a Marvel Heroes Omega 2.16a install.

Usage: python tools/build_db.py "<game folder>"   (the folder that contains UnrealEngine3 and Data)
Needs the lz4 package: pip install lz4
"""
import hashlib
import json
import os
import re
import sys

from calligraphy import GameData
from categories import CATEGORIES, NEVER
from groups import GROUPS
from upk import Package, unpack

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'internal', 'db', 'itemdb.json')
SINKS = {'shown': 'MarvelItem_ReputationQuest_BaseItem', 'hidden': 'MarvelItem_Loot_VibraniumOre'}
RARITIES = {'Common': 'CommonWhite', 'Uncommon': 'UncommonGreen', 'Rare': 'RareBlue',
            'Epic': 'EpicPurple', 'Cosmic': 'Cosmic', 'Unique': 'Unique'}


def is_placeholder(name):
    """Designer placeholders such as ARMOR_ITEM_BLUEPRINT or THIS ITEM NEEDS A NAME, which never drop."""
    return not re.search('[a-z]', name) and bool(re.search(r'BLUEPRINT|NAME|TESTING|REDESIGN|_|^RUNE$', name))


ROLLED = ('Gear', 'Insignias', 'Medallions', 'Relics', 'Team-up gear')  # categories whose rarity is rolled at drop


def describe(path, category, rarity_glow):
    """A short line that tells an item apart from others of the same name and says what one switch covers."""
    parts = []
    hero = re.match(r'Entity/Items/Armor/Prototypes/([^/]+)/', path)
    if path.startswith('Entity/Items/Rings/PVPRings/'):
        parts.append('PvP ring')
    elif path.startswith('Entity/Items/Rings/') and category == 'Gear':
        parts.append('Ring')
    elif hero:
        parts.append(re.sub(r'(?<=[a-z])(?=[A-Z])', ' ', hero.group(1)) + ' gear')
    elif category == 'Recipes' and path.startswith('Entity/Items/Crafting/Recipes/'):
        parts.append('Crafting recipe')
    if category in ROLLED and rarity_glow:
        parts.append('drops at any rarity' if parts else 'Drops at any rarity')
    return ', '.join(parts)


def strip_markup(name):
    return re.sub(r'\s+', ' ', re.sub(r'#[^#]*#|\$[^$]*\$', '', name)).strip()


def item_offsets(pkg):
    """Offsets of the glow, model and name references of an item type, whether it brings its own glow, and where its
    drop sound is set: the AudioType value, or 0 and the end of the default object's properties to add it before."""
    glow, model, name, own_glow, audio, audio_end = [], [], [], False, 0, 0
    for index, export in enumerate(pkg.exports, 1):
        parts = pkg.path(index).split('.')
        if len(parts) < 2 or not parts[1].startswith('default__marvelitem'):
            continue
        if len(parts) == 2:
            props, audio_end = pkg.properties_and_end(export)
            for prop, _, value, offset in props:
                if prop == 'audiotype':
                    audio = offset
                if prop == 'm_tooltipcomp' and value != 'None':
                    name.append(offset)
                if prop == 'rarityeffectoverride' and value:
                    own_glow = True
        elif len(parts) == 3 and pkg.class_name(export).split('.')[-1] in ('entityfxparticle', 'skeletalmeshcomponent'):
            for prop, kind, value, offset in pkg.properties(export, 16):
                if kind == 'ObjectProperty' and value != 'None':
                    if prop == 'particlesystemtemplate':
                        glow.append(offset)
                    elif prop == 'skeletalmesh':
                        model.append(offset)
    return glow, model, name, own_glow, audio, audio_end


def rarity_offsets(marvel_game):
    pkg = Package(unpack(marvel_game))
    out = {}
    for rarity, effect in RARITIES.items():
        path = f'Default__RarityEffect{effect}.Particles'
        export = next(e for i, e in enumerate(pkg.exports, 1) if pkg.path(i) == path)
        out[rarity] = [offset for prop, _, _, offset in pkg.properties(export, 16) if prop == 'ParticleSystemTemplate']
    return out


def main(game_dir):
    cooked = os.path.join(game_dir, 'UnrealEngine3', 'MarvelGame', 'CookedPCConsole')
    marvel_game = open(os.path.join(cooked, 'MarvelGame.upk'), 'rb').read()
    db = {'version': 1, 'marvelGameSha1': hashlib.sha1(marvel_game).hexdigest(),
          'rarities': rarity_offsets(marvel_game), 'types': {}, 'items': []}

    for file in sorted(os.listdir(cooked)):
        match = re.match(r'(?i)UC__(MarvelItem_.+)_SF\.upk$', file)
        if not match:
            continue
        key = match.group(1).lower()
        short = key[len('marvelitem_'):]
        if re.search(NEVER, short):
            continue
        data = open(os.path.join(cooked, file), 'rb').read()
        glow, model, name, own_glow, audio, audio_end = item_offsets(Package(unpack(data)))
        if not (glow or model or name):
            continue
        db['types'][key] = {
            'file': file,
            'origSha1': hashlib.sha1(data).hexdigest(),
            'category': next((label for label, pattern in CATEGORIES if re.search(pattern, short)), 'Other'),
            'rarityGlow': not own_glow,
            'glow': glow, 'model': model, 'name': name, 'audio': audio, 'audioEnd': audio_end,
        }

    game = GameData(game_dir)
    for field, name in (('assetPackageCacheSha1', 'AssetPackageCache.bin'), ('soundPackageSha1', 'SFX_Shared_INT.pck')):
        db[field] = hashlib.sha1(open(os.path.join(cooked, name), 'rb').read()).hexdigest()
    db['calligraphySha1'] = hashlib.sha1(open(os.path.join(game_dir, 'Data', 'Game', 'Calligraphy.sip'), 'rb').read()).hexdigest()
    assets = {name.lower(): asset_id for asset_id, name in game.assets.items()}
    db['sinks'] = {key: {'type': cls.lower(), 'asset': assets[cls.lower()]} for key, cls in SINKS.items()}
    db['groups'] = [{'id': gid, 'label': label, 'note': note} for gid, label, note, _ in GROUPS]

    # Items live in .prototype files, and medallions also in blueprint .defaults files (most normal medallions).
    # Other .defaults files are bases (gear slots, insignias, gems) that never drop.
    # A prototype's children inherit its UnrealClass, so each prototype lists the other items' prototypes that
    # do, and the filter pins those to their own class when it retargets this one.
    item_pids = [pid for pid, (_, path) in game.prototypes.items()
                 if path.startswith('Entity/Items/') and path.endswith(('.prototype', '.defaults'))]
    inheritors = game.class_inheritors(item_pids)
    item_set = {pid for pid in item_pids
                if game.prototypes[pid][1].endswith('.prototype') or game.prototypes[pid][1].startswith('Entity/Items/Medals/')}

    def slot_entry(pid):
        slot = game.unreal_class_slot(pid)
        if slot is None:
            return None
        return {'path': 'Calligraphy/' + game.prototypes[pid][1], 'blueprint': slot[0], 'copy': slot[1], 'field': slot[2]}

    items, item_of = {}, {}
    for pid, (_, path) in sorted(game.prototypes.items(), key=lambda kv: kv[1][1]):
        if pid not in item_set or re.search(r'/test|/unused/', path, re.I):
            continue
        name, unreal_class = game.item(pid)
        name, key = strip_markup(name), unreal_class.lower()
        if not name or key not in db['types'] or 'Test' in name or is_placeholder(name):
            continue
        entry = slot_entry(pid)
        if entry is None:
            continue
        item_of[pid] = (name, key)
        item = items.setdefault((name, key), {'name': name, 'type': key, 'protos': [], 'groups': set()})
        item.setdefault('detail', describe(path, db['types'][key]['category'], db['types'][key]['rarityGlow']))
        item['protos'].append(entry)
        entry['pid'] = pid
        category = db['types'][key]['category']
        item['groups'].update(gid for gid, _, _, test in GROUPS if test(name, path, category))
    for item in items.values():
        for entry in item['protos']:
            pins = []
            for child in inheritors.get(entry.pop('pid'), []):
                if item_of.get(child) == (item['name'], item['type']):
                    continue  # same item: retargeted along with this prototype
                pin = slot_entry(child)
                child_class = assets.get(game.item(child)[1].lower())
                if pin and child_class:
                    pins.append({**pin, 'asset': child_class})
            if pins:
                entry['inheritors'] = pins
    # Each item belongs to the first group in GROUPS that matches any of its prototypes: group switches act on
    # every item of a group, so overlapping groups would switch each other.
    order = [gid for gid, _, _, _ in GROUPS]
    for item in items.values():
        item['groups'] = sorted(item['groups'], key=order.index)[:1]
        if not item['detail']:
            del item['detail']
    db['items'] = sorted(items.values(), key=lambda i: i['name'].lower())
    db['unrealClassFields'] = sorted({p['field'] for i in db['items'] for p in i['protos']})

    with open(OUT, 'w') as f:
        json.dump(db, f, separators=(',', ':'))
    print(f"{len(db['items'])} items, {len(db['types'])} types")


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
