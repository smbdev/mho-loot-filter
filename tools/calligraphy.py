"""Reads item names and Unreal classes from Marvel Heroes Omega's Calligraphy.sip game data."""
import glob
import os
import struct

import lz4.block

SIMPLE_STRUCT = 0x52  # RHStruct: an embedded prototype instead of a 64-bit value
SUBTYPED = (0x41, 0x43, 0x50, 0x52)  # asset, curve, prototype and RHStruct fields carry a subtype id


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
        r = Reader(sip.read('Calligraphy/Prototype.directory'), 4)
        for _ in range(r.read('<I')):
            pid, _, blueprint, _ = r.read('<QQQB')
            self.prototypes[pid] = (blueprint, r.string16().replace('\\', '/'))

        self.assets = {}  # asset id -> name
        r = Reader(sip.read('Calligraphy/Type.directory'), 4)
        for _ in range(r.read('<I')):
            r.read('<QQB')
            t = Reader(sip.read('Calligraphy/' + r.string16().replace('\\', '/')), 4)
            for _ in range(t.read('<H')):
                asset_id, _, _ = t.read('<QQB')
                self.assets[asset_id] = t.string16()

        self.fields = {}  # field id -> field name
        self.blueprints = {}  # blueprint path -> blueprint id
        r = Reader(sip.read('Calligraphy/Blueprint.directory'), 4)
        for _ in range(r.read('<I')):
            blueprint_id, _, _ = r.read('<QQB')
            path = r.string16().replace('\\', '/')
            self.blueprints[path] = blueprint_id
            b = Reader(sip.read('Calligraphy/' + path), 4)
            b.string16()
            b.read('<Q')
            for _ in range(b.read('<H')):
                b.read('<QB')
            for _ in range(b.read('<H')):
                b.read('<QB')
            for _ in range(b.read('<H')):
                field_id = b.read('<Q')
                name = b.string16()
                base_type, _ = b.read('<BB')
                if base_type in SUBTYPED:
                    b.read('<Q')
                self.fields[field_id] = name

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
