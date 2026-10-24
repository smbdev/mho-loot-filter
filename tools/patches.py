"""MHServerEmu's prototype patches (Data/v52/Patches/PatchData*.json): drops and fixes the server adds to the game data."""
import glob
import json
import os
import re
import struct


def load(mhserver_dir):
    """Enabled patch entries from the files the server loads (the top folder, not Off/)."""
    out = []
    for path in sorted(glob.glob(os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'Patches', 'PatchData*.json'))):
        out += [e for e in json.load(open(path, encoding='utf-8-sig')) if e.get('Enabled')]
    return out


def _double(x):
    return struct.unpack('<Q', struct.pack('<d', float(x)))[0]


def _value(game, kind, v):
    """A patch value in the form calligraphy.GameData keeps fields: ids, raw doubles, embedded struct dicts."""
    if kind == 'Float':
        return _double(v)
    if kind in ('Integer', 'Boolean', 'PrototypeId', 'PrototypeDataRef'):
        return int(v)
    if kind == 'PrototypeGuid':
        return game.guids.get(int(v), 0)
    if kind == 'Enum':
        return next((i for i, name in game.assets.items() if name == v), 0)
    if kind in ('PrototypeIdArray', 'PrototypeDataRefArray'):
        return [int(x) for x in v]
    if kind == 'Prototype':
        return {'_parent' if k == 'ParentDataRef' else k: (_double(x) if isinstance(x, float) else x) for k, x in v.items()}
    if kind == 'PrototypeArray':
        return [_value(game, 'Prototype', x) for x in v]
    return v


def _wrap(kind, value):
    """A reference appended to a list of choices or modifiers is an embedded struct that inherits it."""
    if kind == 'PrototypeDataRef':
        return [{'_parent': value}]
    if kind == 'PrototypeDataRefArray':
        return [{'_parent': x} for x in value]
    return value if isinstance(value, list) else [value]


def apply(game, t, entries):
    """Apply patch entries in file order. A path may lead into a part an earlier entry appended: the server applies
    those too (its patched DogTagsCurrencyTable holds three Dog Tags, each with a filled RequireDropperKeyword).
    An entry whose part does not exist is skipped, as the server logs and skips it."""
    for e in entries:
        kind = e['ValueType'].replace('[]', 'Array')  # the files write PrototypeDataRef[] for PrototypeDataRefArray
        node = t
        *parts, last = e['Path'].split('.')
        for part in parts:
            name, index = re.fullmatch(r'(\w+)(?:\[(\d*)\])?', part).groups()
            node = node.get(name) if isinstance(node, dict) else None
            if index and isinstance(node, list):
                node = node[int(index)] if int(index) < len(node) else None
            if not isinstance(node, dict):
                break
        if not isinstance(node, dict):
            continue
        name, index = re.fullmatch(r'(\w+)(?:\[(\d*)\])?', last).groups()
        value = _value(game, kind, e['Value'])
        if index is None:
            node[name] = value
        elif index == '':
            node[name] = list(node.get(name) or []) + _wrap(kind, value)
        elif isinstance(node.get(name), list) and int(index) < len(node[name]):
            node[name][int(index)] = value
