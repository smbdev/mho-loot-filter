"""LZO1X decompression (the format Marvel Heroes Omega packages use), in pure Python."""


def _run_length(src, ip, base):
    n = base
    while src[ip] == 0:
        n += 255
        ip += 1
    return n + src[ip], ip + 1


def _copy_match(out, distance, length):
    start = len(out) - distance
    if distance >= length:
        out += out[start:start + length]
    else:
        for i in range(length):
            out.append(out[start + i])


def decompress(src, size):
    out = bytearray()
    ip = 0
    state = 0
    if src[0] > 17:
        t = src[0] - 17
        ip = 1
        out += src[ip:ip + t]
        ip += t
        state = t if t < 4 else 4
    while True:
        t = src[ip]
        ip += 1
        if t < 16:
            if state == 0:
                if t == 0:
                    t, ip = _run_length(src, ip, 15)
                t += 3
                out += src[ip:ip + t]
                ip += t
                state = 4
                continue
            following = t & 3
            if state != 4:
                _copy_match(out, 1 + (t >> 2) + (src[ip] << 2), 2)
            else:
                _copy_match(out, 1 + 0x800 + (t >> 2) + (src[ip] << 2), 3)
            ip += 1
        else:
            if t >= 64:
                following = t & 3
                distance = 1 + ((t >> 2) & 7) + (src[ip] << 3)
                length = (t >> 5) + 1
                ip += 1
            elif t >= 32:
                length = t & 31
                if length == 0:
                    length, ip = _run_length(src, ip, 31)
                length += 2
                v = src[ip] | src[ip + 1] << 8
                ip += 2
                distance = 1 + (v >> 2)
                following = v & 3
            else:
                length = t & 7
                if length == 0:
                    length, ip = _run_length(src, ip, 7)
                length += 2
                v = src[ip] | src[ip + 1] << 8
                ip += 2
                distance = ((t & 8) << 11) + (v >> 2)
                following = v & 3
                if distance == 0:
                    break
                distance += 0x4000
            _copy_match(out, distance, length)
        state = following
        out += src[ip:ip + following]
        ip += following
    if len(out) != size:
        raise ValueError(f'LZO block decompressed to {len(out)} bytes, expected {size}')
    return bytes(out)
