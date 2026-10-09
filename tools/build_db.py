"""Builds internal/db/itemdb.json from a Marvel Heroes Omega 2.16a install.

Usage: python tools/build_db.py "<game folder>"   (the folder that contains UnrealEngine3 and Data)
Needs the lz4 package: pip install lz4
"""
import hashlib
import json
import os
import re
import struct
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


ROLLED = ('Gear', 'Rings', 'Insignias', 'Medallions', 'Relics', 'Team-up gear')  # categories whose rarity is rolled at drop


def describe(path, category, rarity_glow):
    """A short line that tells an item apart from others of the same name and says what one switch covers."""
    parts = []
    hero = re.match(r'Entity/Items/Armor/Prototypes/([^/]+)/', path)
    if path.startswith('Entity/Items/Rings/PVPRings/'):
        parts.append('PvP ring')
    elif path.startswith('Entity/Items/Rings/') and category == 'Rings':
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


def rarity_script(marvel_game):
    """Where hide-by-rarity code goes in MarvelGame.upk, and the objects it refers to.

    The code runs at the end of MarvelItem.PostAdapterInit, after the item's Rarity has been set, and before the
    function's final return (04 0b) and end of script (53). lineCheck is where the item mesh's default properties
    end, for switching on clicks that hit the mesh's bounding box.
    """
    pkg = Package(unpack(marvel_game))
    index = {pkg.path(i): i for i in range(1, len(pkg.exports) + 1)}
    func = pkg.exports[index['MarvelItem.PostAdapterInit'] - 1]
    memory, storage = struct.unpack_from('<ii', pkg.data, func['offset'] + 40)
    if pkg.data[func['offset'] + 48 + storage - 3:func['offset'] + 48 + storage] != b'\x04\x0b\x53':
        raise ValueError('MarvelItem.PostAdapterInit does not end in return')
    imports = {pkg.path(-i): -i for i in range(1, len(pkg.imports) + 1)}
    return {'function': func['offset'], 'memory': memory, 'storage': storage,
            'rarity': index['MarvelItem.Rarity'], 'tooltip': index['MarvelEntity.m_tooltipComp'],
            'hideTooltip': index['MarvelGFxActorTooltipComp.HideTooltip'], 'setHidden': imports['Engine.Actor.SetHidden'],
            'mesh': index['MarvelEntity.Mesh'], 'setTraceBlocking': imports['Engine.PrimitiveComponent.SetTraceBlocking'],
            'lineCheck': pkg.properties_and_end(pkg.exports[index['Default__MarvelItem.InitialSkeletalMesh'] - 1], 16)[1],
            'boolProperty': pkg.names.index('BoolProperty')}


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
          'rarities': rarity_offsets(marvel_game), 'rarityScript': rarity_script(marvel_game), 'types': {}, 'items': []}

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

    # Hero uniques live in Entity/Items/Armor/UniquePrototypes/Avatars/<Hero>/; the hero's display name comes from
    # its avatar prototype, matched by file name. AnyHero, GunHeroes and CapeHeroes are shared, not one hero's.
    avatar_names = {}
    for pid, (_, path) in game.prototypes.items():
        if path.startswith('Entity/Characters/Avatars/Shipping/') and path.count('/') == 4 and path.endswith('.prototype'):
            shown = strip_markup(game.strings.get(game.field_values(pid).get('DisplayName', 0), ''))
            if shown:
                avatar_names[path.split('/')[-1][:-len('.prototype')].lower()] = shown

    def hero_of(path):
        found = re.match(r'Entity/Items/Armor/UniquePrototypes/Avatars/([^/]+)/', path)
        if not found or found.group(1) == 'AnyHero' or found.group(1).endswith('Heroes'):
            return None
        folder = found.group(1)
        return avatar_names.get(folder.lower()) or re.sub(r'(?<=[a-z])(?=[A-Z])', ' ', folder)

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
        if hero_of(path):
            item['hero'] = hero_of(path)
        icon = game.assets.get(game.field_values(pid).get('IconPath') or 0)
        if icon and not item.get('icon'):
            item['icon'] = icon  # a texture such as MarvelUIIcons.Item_Unique335, read from the user's install
        if not item['detail'] and (hero_of(path) or '/Avatars/AnyHero/' in path):
            # the same wording as the game's tooltip
            item['detail'] = 'Unique - ' + (hero_of(path) or 'Any hero')
        item['protos'].append(entry)
        entry['pid'] = pid
        category = db['types'][key]['category']
        item['groups'].update(gid for gid, _, _, test in GROUPS if test(name, path, category))
    # Clicking picks items by their Bounds, not their model. A hidden item gets its own copy of its Bounds with
    # ComplexPickingOnly set, which makes it unclickable; items that inherit Bounds from it keep a plain copy.
    field_ids = {name: fid for fid, name in game.fields.items()}
    bounds_field = field_ids['Bounds']
    db['picking'] = {'boundsField': bounds_field, 'flagField': field_ids['ComplexPickingOnly'],
                     'boundsBlueprint': game.blueprints['Entity/Components/Bounds/Bounds.blueprint']}
    bounds_inheritors = game.class_inheritors(item_pids, 'Bounds')
    bounds_table, bounds_index = [], {}

    def bounds_entry(pid):
        found = game.field_source(pid, bounds_field)
        if not found:
            return None
        key = (found[0], found[1], found[2].hex())
        if key not in bounds_index:
            bounds_index[key] = len(bounds_table)
            bounds_table.append({'blueprint': found[0], 'copy': found[1], 'data': found[2].hex()})
        return bounds_index[key]

    for item in items.values():
        for entry in item['protos']:
            pid = entry['pid']
            index = bounds_entry(pid)
            if index is not None:
                entry['bounds'] = index
                if game.struct_field(pid, bounds_field):
                    entry['boundsOwn'] = True
                kids = [game.prototypes[c][1] for c in bounds_inheritors.get(pid, []) if item_of.get(c) != (item['name'], item['type'])]
                if kids:
                    entry['boundsInheritors'] = ['Calligraphy/' + k for k in kids]
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
    db['bounds'] = bounds_table
    db['unrealClassFields'] = sorted({p['field'] for i in db['items'] for p in i['protos']})

    with open(OUT, 'w') as f:
        json.dump(db, f, separators=(',', ':'))
    print(f"{len(db['items'])} items, {len(db['types'])} types")


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
