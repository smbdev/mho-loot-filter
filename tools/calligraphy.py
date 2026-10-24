"""Reads item names and Unreal classes from Marvel Heroes Omega's Calligraphy.sip game data."""
import glob
import os
import struct

import lz4.block

SIMPLE_STRUCT = 0x52  # RHStruct: an embedded prototype instead of a 64-bit value
SUBTYPED = (0x41, 0x43, 0x50, 0x52)  # asset, curve, prototype and RHStruct fields carry a subtype id


def double(q):
    """A raw 64-bit field value read as the double it stores."""
    return struct.unpack('<d', struct.pack('<Q', q))[0]


def signed(q):
    return q - (1 << 64) if q >= 1 << 63 else q


class Reader:
    def __init__(self, data, pos=0):
        self.data, self.pos = data, pos

    def read(self, fmt):
        values = struct.unpack_from(fmt, self.data, self.pos)
        self.pos += struct.calcsize(fmt)
        return values[0] if len(values) == 1 else values

    def string16(self):
        length = self.read('<H')
        value = self.data[self.pos:self.pos + length].decode('utf-8', 'replace')
        self.pos += length
        return value


class Sip:
    """A Gazillion pak archive (KAPG): file name -> LZ4-compressed bytes."""

    def __init__(self, path):
        data = open(path, 'rb').read()
        if data[:4] != b'KAPG':
            raise ValueError(f'{path} is not a pak archive')
        count = struct.unpack_from('<i', data, 8)[0]
        p = 12
        entries = {}
        for _ in range(count):
            p += 8
            length = struct.unpack_from('<i', data, p)[0]
            p += 4
            name = data[p:p + length].decode()
            p += length
            _, offset, csize, size = struct.unpack_from('<iiii', data, p)
            p += 16
            entries[name] = (offset, csize, size)
        self._data, self._base, self._entries = data, p, entries

    def read(self, name):
        offset, csize, size = self._entries[name]
        start = self._base + offset
        return lz4.block.decompress(self._data[start:start + csize], uncompressed_size=size)


class GameData:
    def __init__(self, game_dir):
        sip = Sip(os.path.join(game_dir, 'Data', 'Game', 'Calligraphy.sip'))
        self._sip = sip

        self.prototypes = {}  # prototype id -> (blueprint id, path)
        self.abstract = set()  # ids with PrototypeRecordFlags.Abstract
        self.guids = {}  # prototype guid -> prototype id, for patch values given as guids
        r = Reader(sip.read('Calligraphy/Prototype.directory'), 4)
        for _ in range(r.read('<I')):
            pid, guid, blueprint, flags = r.read('<QQQB')
            self.prototypes[pid] = (blueprint, r.string16().replace('\\', '/'))
            self.guids[guid] = pid
            if flags & 1:
                self.abstract.add(pid)
        self.paths = {path: pid for pid, (_, path) in self.prototypes.items()}

        self.assets = {}  # asset id -> name
        r = Reader(sip.read('Calligraphy/Type.directory'), 4)
        for _ in range(r.read('<I')):
            r.read('<QQB')
            t = Reader(sip.read('Calligraphy/' + r.string16().replace('\\', '/')), 4)
            for _ in range(t.read('<H')):
                asset_id, _, _ = t.read('<QQB')
                self.assets[asset_id] = t.string16()

        self.fields = {}  # field id -> field name
        self.blueprint_parents = {}  # blueprint id -> parent blueprint ids
        self.blueprints = {}  # blueprint path -> blueprint id
        r = Reader(sip.read('Calligraphy/Blueprint.directory'), 4)
        for _ in range(r.read('<I')):
            blueprint_id, _, _ = r.read('<QQB')
            path = r.string16().replace('\\', '/')
            self.blueprints[path] = blueprint_id
            b = Reader(sip.read('Calligraphy/' + path), 4)
            b.string16()
            b.read('<Q')
            self.blueprint_parents[blueprint_id] = [b.read('<QB')[0] for _ in range(b.read('<H'))]
            for _ in range(b.read('<H')):
                b.read('<QB')
            for _ in range(b.read('<H')):
                field_id = b.read('<Q')
                name = b.string16()
                base_type, _ = b.read('<BB')
                if base_type in SUBTYPED:
                    b.read('<Q')
                self.fields[field_id] = name

        self.blueprint_paths = {v: k for k, v in self.blueprints.items()}
        self.curve_paths = {}  # curve id -> path
        r = Reader(sip.read('Calligraphy/Curve.directory'), 4)
        for _ in range(r.read('<I')):
            curve_id, _, _ = r.read('<QQB')
            self.curve_paths[curve_id] = r.string16().replace('\\', '/')
        self._merged = {}
        self.patches = {}  # prototype id -> patch entries applied by merged() (see tools/patches.py)

        self.strings = {}  # locale string id -> English text
        for path in glob.glob(os.path.join(game_dir, 'Data', 'Game', 'Loco', 'eng.all', '*.string')):
            data = open(path, 'rb').read()
            r = Reader(data, 4)
            for _ in range(r.read('<H')):
                string_id, variants, _, offset = r.read('<QHHI')
                r.pos += 14 * max(variants - 1, 0)
                end = data.find(b'\0', offset)
                self.strings[string_id] = data[offset:end if end >= 0 else len(data)].decode('utf-8', 'replace')

        self._cache = {}

    def _tree(self, r):
        flags = r.read('<B')
        out = {'_parent': r.read('<Q') if flags & 1 else 0}
        if flags & 2:
            for _ in range(r.read('<H')):
                blueprint, copy = r.read('<QB')
                bp_path = self.blueprint_paths.get(blueprint, '')
                into = out.setdefault('_props', {}).setdefault((bp_path, copy), {}) if bp_path.startswith('Property/') else out
                for _ in range(r.read('<H')):
                    fid, base_type = r.read('<QB')
                    into[self.fields.get(fid, fid)] = self._tree(r) if base_type == SIMPLE_STRUCT else r.read('<Q')
                for _ in range(r.read('<H')):
                    fid, base_type = r.read('<QB')
                    into[self.fields.get(fid, fid)] = [self._tree(r) if base_type == SIMPLE_STRUCT else r.read('<Q')
                                                       for _ in range(r.read('<H'))]
        return out

    def tree(self, pid):
        """A prototype's own fields: embedded structs as dicts with their _parent, property groups under _props."""
        return self._tree(Reader(self._sip.read('Calligraphy/' + self.prototypes[pid][1]), 4))

    def merged(self, pid):
        """A prototype with its parent chain (own parent, else its blueprint defaults) merged in, child fields winning."""
        if pid in self._merged:
            return self._merged[pid]
        own = self.tree(pid)
        parent = own.pop('_parent') or self.prototypes[pid][0]
        base = self.merged(parent) if parent and parent != pid and parent in self.prototypes else {'_root': self.prototypes[pid][1]}
        out = {**base, **{k: v for k, v in own.items() if k != '_props'}}
        props = {k: dict(v) for k, v in base.get('_props', {}).items()}
        for k, v in own.get('_props', {}).items():
            props[k] = {**props.get(k, {}), **v}
        out['_props'], out['_path'] = props, self.prototypes[pid][1]
        if pid in self.patches:
            import copy
            import patches  # only the drop builder sets patches
            out = copy.deepcopy(out)  # nested lists are shared with the parent's cached dict
            patches.apply(self, out, self.patches[pid])
        self._merged[pid] = out
        return out

    def merged_struct(self, s):
        """An embedded struct merged with the prototype it inherits from."""
        base = self.merged(s['_parent']) if s.get('_parent') in self.prototypes else {}
        return {**base, **{k: v for k, v in s.items() if k != '_parent'}, '_path': base.get('_path', '')}

    def curve_start(self, curve_id):
        return Reader(self._sip.read('Calligraphy/' + self.curve_paths[curve_id]), 4).read('<i')

    def curve(self, curve_id):
        """A curve's values (CRV header, start and end position, then one double per position)."""
        r = Reader(self._sip.read('Calligraphy/' + self.curve_paths[curve_id]), 4)
        start, end = r.read('<ii')
        return [r.read('<d') for _ in range(end - start + 1)]

    def _read_prototype(self, r, out):
        flags = r.read('<B')
        parent = r.read('<Q') if flags & 1 else 0
        if flags & 2:
            for _ in range(r.read('<H')):
                r.read('<QB')
                for _ in range(r.read('<H')):
                    field_id, base_type = r.read('<QB')
                    if base_type == SIMPLE_STRUCT:
                        self._read_prototype(r, {})
                        out.setdefault(self.fields.get(field_id, field_id), None)  # set, value is a struct
                        continue
                    out.setdefault(self.fields.get(field_id, field_id), r.read('<Q'))
                for _ in range(r.read('<H')):
                    _, base_type = r.read('<QB')
                    for _ in range(r.read('<H')):
                        if base_type == SIMPLE_STRUCT:
                            self._read_prototype(r, {})
                        else:
                            r.read('<Q')
        return parent

    def field_values(self, pid):
        """All simple field values of a prototype, including those inherited from its parents."""
        if pid in self._cache:
            return self._cache[pid]
        out = {}
        if pid in self.prototypes:
            blueprint, path = self.prototypes[pid]
            parent = self._read_prototype(Reader(self._sip.read('Calligraphy/' + path), 4), out) or blueprint
            if parent and parent != pid:
                for key, value in self.field_values(parent).items():
                    out.setdefault(key, value)
        self._cache[pid] = out
        return out

    def parent(self, pid):
        """The prototype pid inherits from: its own parent, or else its blueprint's defaults (0 for none)."""
        blueprint, path = self.prototypes[pid]
        r = Reader(self._sip.read('Calligraphy/' + path), 4)
        parent = r.read('<Q') if r.read('<B') & 1 else 0
        parent = parent or blueprint
        return 0 if parent == pid else parent

    def struct_field(self, pid, field_id):
        """(blueprint, copy, raw bytes) of a top-level embedded-struct field the prototype sets itself, or None."""
        data = self._sip.read('Calligraphy/' + self.prototypes[pid][1])
        r = Reader(data, 4)
        flags = r.read('<B')
        if flags & 1:
            r.read('<Q')
        if not flags & 2:
            return None
        for _ in range(r.read('<H')):
            blueprint, copy = r.read('<QB')
            for _ in range(r.read('<H')):
                fid, base_type = r.read('<QB')
                if base_type == SIMPLE_STRUCT:
                    start = r.pos
                    self._read_prototype(r, {})
                    if fid == field_id:
                        return blueprint, copy, data[start:r.pos]
                    continue
                r.read('<Q')
            for _ in range(r.read('<H')):
                _, base_type = r.read('<QB')
                for _ in range(r.read('<H')):
                    if base_type == SIMPLE_STRUCT:
                        self._read_prototype(r, {})
                    else:
                        r.read('<Q')
        return None

    def field_source(self, pid, field_id):
        """struct_field() of the nearest prototype in pid's parent chain (pid included) that sets the field."""
        depth = 0
        while pid in self.prototypes and depth < 64:
            found = self.struct_field(pid, field_id)
            if found:
                return found
            pid, depth = self.parent(pid), depth + 1
        return None

    def class_inheritors(self, pids, field='UnrealClass'):
        """For each prototype in pids, the prototypes among pids whose value of field comes from it: descendants
        that reach it without passing a prototype that sets the field itself."""
        sets_class = {pid: field in self._own_fields(pid) for pid in pids}
        out = {}
        for child in pids:
            if sets_class[child]:
                continue
            node, depth = self.parent(child), 0
            while node in sets_class and depth < 64:
                out.setdefault(node, []).append(child)
                if sets_class[node]:
                    break
                node, depth = self.parent(node), depth + 1
        return out

    def _own_fields(self, pid):
        out = {}
        self._read_prototype(Reader(self._sip.read('Calligraphy/' + self.prototypes[pid][1]), 4), out)
        return out

    def unreal_class_slot(self, pid):
        """(declaring blueprint, copy number, field id) of the UnrealClass value a prototype uses, own or inherited."""
        while pid in self.prototypes:
            blueprint, path = self.prototypes[pid]
            r = Reader(self._sip.read('Calligraphy/' + path), 4)
            flags = r.read('<B')
            parent = r.read('<Q') if flags & 1 else 0
            if flags & 2:
                for _ in range(r.read('<H')):
                    group_blueprint, copy = r.read('<QB')
                    for _ in range(r.read('<H')):
                        field_id, base_type = r.read('<QB')
                        if base_type == SIMPLE_STRUCT:
                            self._read_prototype(r, {})
                            continue
                        r.read('<Q')
                        if self.fields.get(field_id) == 'UnrealClass':
                            return group_blueprint, copy, field_id
                    for _ in range(r.read('<H')):
                        _, base_type = r.read('<QB')
                        for _ in range(r.read('<H')):
                            if base_type == SIMPLE_STRUCT:
                                self._read_prototype(r, {})
                            else:
                                r.read('<Q')
            next_pid = parent or blueprint
            if next_pid == pid:
                break
            pid = next_pid
        return None

    def item(self, pid):
        """(English display name, Unreal class name) of an item prototype."""
        values = self.field_values(pid)
        return self.strings.get(values.get('DisplayName', 0), ''), self.assets.get(values.get('UnrealClass', 0), '')
