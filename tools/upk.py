"""Reads Unreal Engine 3 packages (version 868) as cooked for Marvel Heroes Omega."""
import struct

import lzo

CHUNK_TAG = 0x9E2A83C1
FLAG_COMPRESSED = 0x02000000
PROPERTY_TYPES = {name.lower(): name for name in (
    'ByteProperty', 'BoolProperty', 'StructProperty', 'ObjectProperty', 'ClassProperty', 'IntProperty',
    'FloatProperty', 'NameProperty', 'ArrayProperty', 'StrProperty', 'ComponentProperty')}


def unpack(data):
    """Returns the package with its LZO chunks decompressed, laid out like the Go internal/upk.Unpack."""
    p = 12
    p += 4 + struct.unpack_from('<i', data, p)[0]
    flag_pos = p
    p += 4 + 40 + 4 + 16
    p += 4 + struct.unpack_from('<i', data, p)[0] * 12 + 8
    table = p
    compression, count = struct.unpack_from('<ii', data, p)
    if compression != 2:
        raise ValueError(f'unsupported compression {compression}')
    chunks = [struct.unpack_from('<iiii', data, table + 8 + i * 16) for i in range(count)]
    first = chunks[0][0]
    header = bytearray(data[:first])
    struct.pack_into('<I', header, flag_pos, struct.unpack_from('<I', header, flag_pos)[0] & ~FLAG_COMPRESSED)
    header = header[:table] + bytes(8) + header[table + 8 + count * 16:]
    out = bytearray(header) + bytes(max(first - len(header), 0))
    for uoff, usize, coff, _ in chunks:
        tag, _, _, total = struct.unpack_from('<Iiii', data, coff)
        if tag != CHUNK_TAG:
            raise ValueError('bad chunk tag')
        q = coff + 16
        blocks = []
        while sum(u for _, u in blocks) < total:
            blocks.append(struct.unpack_from('<ii', data, q))
            q += 8
        chunk = bytearray()
        for csize, size in blocks:
            chunk += lzo.decompress(data[q:q + csize], size)
            q += csize
        if len(out) < uoff:
            out += bytes(uoff - len(out))
        out[uoff:uoff + usize] = chunk
    return bytes(out)


class Package:
    """Name, import and export tables of a decompressed package, plus its tagged properties."""

    def __init__(self, data):
        self.data = data
        p = 12
        p += 4 + struct.unpack_from('<i', data, p)[0] + 4
        name_count, name_off, export_count, export_off, import_count, import_off = struct.unpack_from('<6i', data, p)

        self.names = []
        p = name_off
        for _ in range(name_count):
            length = struct.unpack_from('<i', data, p)[0]
            p += 4
            if length < 0:
                self.names.append(data[p:p - 2 * length].decode('utf-16-le').rstrip('\0'))
                p += -2 * length
            else:
                self.names.append(data[p:p + length - 1].decode('latin-1'))
                p += length
            p += 8

        self.imports = []
        p = import_off
        for _ in range(import_count):
            _, _, _, _, outer, name, number = struct.unpack_from('<7i', data, p)
            p += 28
            self.imports.append({'outer': outer, 'name': self.name(name, number)})

        self.exports = []
        p = export_off
        for _ in range(export_count):
            cls, sup, outer, name, number, _ = struct.unpack_from('<6i', data, p)
            p += 32
            size, offset, _, net_count = struct.unpack_from('<iiIi', data, p)
            p += 16 + 4 * net_count + 20
            self.exports.append({'cls': cls, 'super': sup, 'outer': outer, 'name': self.name(name, number),
                                 'size': size, 'offset': offset})

    def name(self, index, number=0):
        return self.names[index] + (f'_{number - 1}' if number else '')

    def _object(self, index):
        return self.imports[-index - 1] if index < 0 else self.exports[index - 1]

    def path(self, index):
        """Full dotted path of an object reference (positive = export, negative = import, 0 = None)."""
        if index == 0:
            return 'None'
        obj = self._object(index)
        path = obj['name']
        while obj['outer']:
            obj = self._object(obj['outer'])
            path = obj['name'] + '.' + path
        return path

    def class_name(self, export):
        return self.path(export['cls']) if export['cls'] else 'Class'

    def properties(self, export, start=4):
        """Tagged properties of an export as (name, type, value, value_offset) tuples.

        start skips the object header: 4 bytes for plain objects, 16 for component subobjects.
        """
        return self.properties_and_end(export, start)[0]

    def properties_and_end(self, export, start=4):
        """properties() plus the offset of the None tag that ends them."""
        data = self.data
        p = export['offset'] + start
        out = []
        while True:
            name = self.name(*struct.unpack_from('<ii', data, p))
            if name.lower() == 'none':
                return out, p
            type_index, _, size, _ = struct.unpack_from('<iiii', data, p + 8)
            kind = PROPERTY_TYPES.get(self.names[type_index].lower(), self.names[type_index])
            p += 24
            if kind == 'StructProperty':
                p += 8
            if kind == 'ByteProperty':
                p += 8
            value_offset = p
            if kind == 'BoolProperty':
                value = data[p]
                p += 1
            else:
                raw = data[p:p + size]
                p += size
                if kind in ('ObjectProperty', 'ClassProperty'):
                    value = self.path(struct.unpack('<i', raw)[0])
                elif kind == 'IntProperty':
                    value = struct.unpack('<i', raw)[0]
                else:
                    value = raw
            out.append((name, kind, value, value_offset))
