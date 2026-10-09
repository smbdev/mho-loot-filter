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

    items = {}
    for pid, (_, path) in sorted(game.prototypes.items(), key=lambda kv: kv[1][1]):
        if not (path.startswith('Entity/Items/') and path.endswith('.prototype')) or '/Test' in path:
            continue
        name, unreal_class = game.item(pid)
        name, key = strip_markup(name), unreal_class.lower()
        if not name or key not in db['types'] or 'Test' in name:
            continue
        slot = game.unreal_class_slot(pid)
        if slot is None:
            continue
        item = items.setdefault((name, key), {'name': name, 'type': key, 'protos': [], 'groups': set()})
        item['protos'].append({'path': 'Calligraphy/' + path, 'blueprint': slot[0], 'copy': slot[1], 'field': slot[2]})
        category = db['types'][key]['category']
        item['groups'].update(gid for gid, _, _, test in GROUPS if test(name, path, category))
    db['items'] = sorted(({**i, 'groups': sorted(i['groups'])} for i in items.values()), key=lambda i: i['name'].lower())
    db['unrealClassFields'] = sorted({p['field'] for i in db['items'] for p in i['protos']})

    with open(OUT, 'w') as f:
        json.dump(db, f, separators=(',', ':'))
    print(f"{len(db['items'])} items, {len(db['types'])} types")


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
