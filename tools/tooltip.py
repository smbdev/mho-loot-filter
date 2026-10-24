"""Item tooltip text from the game data: built-in stat ranges at level 60, written with the game's own strings.

Values are intervals (lo, hi): the item level is fixed (63 for a level 60 drop) and the item's variation roll
(ItemVariation, 0 to 1, Item.cs) spans its range. Anything this module cannot work out exactly gives None, and the
caller leaves that line off rather than guess."""
import math
import re

from calligraphy import double, signed

ITEM_LEVEL = 63  # a level 60 drop: itembase shows Item Grade 63, and the rifle's Damage Rating curve gives its 883 at 63
ENTRY = ('Property/PropertyPickInRangeEntry.blueprint', 0)
INFO = ('Property/PropertyInfo.blueprint', 0)
TOKEN = re.compile(r'\$([^$]*)\$')


def _path(game, pid):
    return game.prototypes.get(pid, (0, ''))[1]


def _ops(a, b, f):
    vals = [f(x, y) for x in a for y in b]
    return (min(vals), max(vals))


def value(game, node, level=ITEM_LEVEL):
    """An eval struct as an interval, or None for an eval kind this module does not know."""
    if not isinstance(node, dict):
        return None
    n = game.merged_struct(node)
    kind = _path(game, node.get('_parent')).split('/')[-1].rsplit('.', 1)[0]
    if kind == 'LoadInt':
        v = signed(n.get('Value') or 0)
        return (v, v)
    if kind == 'LoadFloat':
        v = double(n.get('Value') or 0)
        return (v, v)
    if kind in ('Mult', 'Add', 'Sub', 'Div', 'Max', 'Min'):
        a, b = value(game, n.get('Arg1'), level), value(game, n.get('Arg2'), level)
        if a is None or b is None or (kind == 'Div' and b[0] <= 0 <= b[1]):
            return None
        return _ops(a, b, {'Mult': lambda x, y: x * y, 'Add': lambda x, y: x + y, 'Sub': lambda x, y: x - y,
                           'Div': lambda x, y: x / y, 'Max': max, 'Min': min}[kind])
    if kind == 'LoadProp':
        prop = _path(game, (n.get('Prop') or {}).get('_parent')).split('/')[-1]
        if prop.startswith('ItemLevelProp'):
            return (level, level)
        if prop.startswith('ItemVariationProp'):
            return (0.0, 1.0)
        return None
    if kind.startswith('LoadCurve'):  # ItemLevelValueCurve, BasePropertyCurve: a curve read at an index
        idx = value(game, n.get('Index'), level)
        cid = n.get('Curve')
        if idx is None or idx[0] != idx[1] or cid not in game.curve_paths:
            return None
        curve, start = game.curve(cid), game.curve_start(cid)
        i = int(idx[0]) - start
        return (curve[i], curve[i]) if 0 <= i < len(curve) else None
    return None


def fmt(v, mods):
    """An interval as the game would print it with a token's modifiers (pct, nofraction, abs, precisionN, mulbyN)."""
    lo, hi = v
    if lo < 0 < hi:  # one sign cannot cover it: the caller leaves the line off
        return None
    for m in mods:
        if m.startswith('mulby') and m[5:].isdigit():
            lo, hi = lo * int(m[5:]), hi * int(m[5:])
    if 'pct' in mods:
        lo, hi = lo * 100, hi * 100
    if 'abs' in mods:
        lo, hi = sorted((abs(lo), abs(hi)))
    precision = next((int(m[9:]) for m in mods if m.startswith('precision') and m[9:].isdigit()), None)
    if 'nofraction' in mods:
        precision = 0
    if precision is None:
        precision = 1 if 'pct' in mods else 0 if all(abs(x - round(x)) < 1e-6 for x in (lo, hi)) else 1

    def one(x):
        if precision == 0:
            x = math.trunc(x + (1e-6 if x >= 0 else -1e-6))  # the game truncates to whole numbers
        return f'{x:,.{precision}f}'
    a, b = one(lo), one(hi)
    return a if a == b else f'{a}-{b}'


def fill(text, look, own=''):
    """Replace every $token$ in a tooltip string. look(base, mods) gives an interval or a string for a token's
    base name (lower case) and its modifiers; any token it cannot fill makes the whole line None."""
    signed_text = 'propertyvaluesign' in text.lower()
    out, pos = [], 0
    for m in TOKEN.finditer(text):
        out.append(text[pos:m.start()])
        pos = m.end()
        parts = m.group(1).lower().split('.')
        mods = [p for p in parts if re.fullmatch(r'pct|nofraction|abs|precision\d+|mulby\d+', p)]
        base = '.'.join(p for p in parts if p not in mods)
        if own and base.split('.')[0] == own:  # $DamagePctResistVsPowerKeyword.pct$ names the line's own property
            base = 'propertyvalue' + base[len(own):]
        if base == 'propertyvaluesign':
            v = look('propertyvalue', [])
            if v is None:
                return None
            out.append('-' if v[1] < 0 else '+')
            continue
        v = look(base, mods)
        if v is None:
            return None
        if isinstance(v, str):
            out.append(v)
        else:
            extra = ['abs'] if signed_text and base == 'propertyvalue' else []
            # "gain $...nofraction$% reduction" with a stored 0.02: the game shows 2%, so a fraction before a % is one
            plain = not any(m == 'pct' or m.startswith(('mulby', 'precision')) for m in mods)
            if plain and text[pos:pos + 1] == '%' and min(abs(v[0]), abs(v[1])) < 1:
                extra.append('pct')
            shown = fmt(v, mods + extra)
            if shown is None:
                return None
            out.append(shown)
    out.append(text[pos:])
    return re.sub(r' {2,}', ' ', ''.join(out)).strip()


def prop_info(game, mixin_pid):
    """The PropertyInfo fields for a property mixin (CritDamageRatingProp -> Property/Info/CritDamageRating)."""
    stem = _path(game, mixin_pid).split('/')[-1].rsplit('.', 1)[0]
    for name in (stem[:-4] if stem.endswith('Prop') else stem, stem):
        pid = game.paths.get(f'Property/Info/{name}.defaults')
        if pid:
            return pid, game.merged(pid).get('_props', {}).get(INFO, {})
    return None, {}


def text_of(game, ref):
    """A translation prototype's English text."""
    return game.strings.get(game.merged(ref).get('Value')) if ref in game.prototypes else None


def _params(prop):
    """A property struct's params: Param0.. from its mixin group."""
    return next(iter((prop.get('_props') or {}).values()), {})


def item_level(game, item_pid):
    """The item level of a level 60 drop: an OutputLevel restriction, else ITEM_LEVEL."""
    for r in game.merged(item_pid).get('LootDropRestrictions') or []:
        if isinstance(r, dict) and _path(game, r.get('_parent')).endswith('/OutputLevel.defaults') and r.get('Value'):
            return r['Value']
    return ITEM_LEVEL


def stat_lines(game, item_pid, level=None):
    """([{'text', 'base', 'info'}], hidden count): the item's built-in stats in order; base lines are the ones the
    tooltip template puts in its first block (the rifle's Damage Rating)."""
    m = game.merged(item_pid)
    blocks = m.get('TooltipCustomPropertyBlocks') or []
    base_infos = set((game.merged_struct(blocks[0]).get('IncludeOnlyProperties') or []) if blocks else [])
    return entry_lines(game, m.get('PropertiesBuiltIn') or [], level or item_level(game, item_pid), base_infos)


def entry_lines(game, entries, level, base_infos=frozenset()):
    """Stat lines for a list of property entries (an item's built-ins or an affix's PropertyEntries)."""
    lines, hidden = [], 0
    for e in entries:
        body = _entry(game, e)
        if not body:
            hidden += 1
            continue
        prop = body.get('Prop') or {}
        info_pid, info = prop_info(game, prop.get('_parent'))
        lo, hi = value(game, body.get('ValueMin'), level), value(game, body.get('ValueMax'), level)
        text = game.strings.get(body.get('TooltipOverrideText') or 0) or text_of(game, info.get('TooltipText'))
        if text == 'TooltipOverrideText':  # the Proc property's placeholder: a proc with no wording, which the game
            continue                      # cannot show either (The Broken Crown's second proc)
        if lo is None or hi is None or not text:
            hidden += 1
            continue
        rng = (min(lo[0], hi[0]), max(lo[1], hi[1]))
        if body.get('RollAsInteger') and game.assets.get(info.get('Type')) == 'Real':
            # the roll is floored (Item.GenerateTruncatedFloatWithinRange): a range below 1, like the S.H.I.E.L.D
            # Graduation Ring's 6-10% proc, is 0 on every server, so the stat does not exist in game
            rng = (math.floor(rng[0]), math.floor(rng[1]))
            if rng == (0, 0):
                continue
        params = {**_params(game.merged(prop['_parent'])), **_params(prop)} if prop.get('_parent') in game.prototypes else _params(prop)
        own = _path(game, info_pid).split('/')[-1].rsplit('.', 1)[0].lower()
        line = fill(text, lambda base, mods: _look(game, base, mods, rng, params, level), own)
        if line is None:
            hidden += 1
            continue
        lines.append({'text': line, 'base': info_pid in base_infos, 'info': _path(game, info_pid)})
    return lines, hidden


def _entry(game, e):
    """A built-in entry's PropertyPickInRangeEntry fields, merged with a preset it inherits from
    (Property/ArmorRangePresets/RangeArmorSlot01Damage gives the rifle's Prop and leaves the values to the item)."""
    if not isinstance(e, dict):
        return None
    base = game.merged(e['_parent']).get('_props', {}).get(ENTRY, {}) if e.get('_parent') in game.prototypes else {}
    own = (e.get('_props') or {}).get(ENTRY)
    return {**base, **(own or {})} if own is not None or base else None


def _display(game, ref):
    t = game.merged(ref) if ref in game.prototypes else {}
    for f in ('DisplayName', 'DisplayNameText'):
        if game.strings.get(t.get(f) or 0):
            return game.strings[t[f]]
    return None


def _look(game, base, mods, rng, params, level):
    if base == 'propertyvalue':
        return rng
    if base.startswith('propertyvalue.'):  # $DamagePctResistVsPowerKeyword.param0.powerkeyword$: the line's own param
        base = base[len('propertyvalue.'):]
    if base == 'procchancestring':
        return fmt(rng, ['pct', 'nofraction']) + '%'
    if base == 'primarymanabehaviorname':
        return 'Spirit'  # the hero's own resource name in game; outside a hero the common one
    parts = base.split('.')
    if not re.fullmatch(r'param\d', parts[0]):
        return None
    ref = params.get('Param' + parts[0][5:])
    if parts[1:] in (['damagetype'], ['resistancetype'], ['primarymanabehaviornamefromtype']):
        kind = game.assets.get(ref, '')
        if parts[1] == 'damagetype':
            return DAMAGE_TYPES.get(kind)
        if parts[1] == 'resistancetype':
            return 'Defense' if kind == 'Any' else DAMAGE_TYPES[kind] + ' Defense' if kind in DAMAGE_TYPES else None
        return 'Spirit' if kind in ('Type1', 'TypeAll') else None
    if len(parts) == 1 or parts[1] in ('keyword', 'powerkeyword', 'entitykeyword'):
        if len(parts) > 2:
            return None
        if ref in game.prototypes:
            return _display(game, ref)
        name = game.assets.get(ref)
        return re.sub(r'(?<=[a-z])(?=[A-Z])', ' ', name) if name else None
    if parts[1] == 'power' and ref in game.prototypes:
        return _power(game, ref, parts[2:], mods, level)
    return None


def _seconds(v):
    a, b = (x / 1000 for x in v)
    if a == b and a >= 60 and a % 60 == 0:
        return fmt((a / 60, b / 60), []) + 'm'
    return fmt((a, b), []) + 's'


DAMAGE_TYPES = {'Physical': 'Physical', 'Energy': 'Energy', 'Mental': 'Mental'}


def _power(game, pid, path, mods, level, node=None):
    """A $paramN.power...$ token walked from a power prototype: conditionN, then a leaf such as duration,
    cooldown or a property name."""
    node = node if node is not None else game.merged(pid)
    if not path:
        return None
    head, rest = path[0], path[1:]
    if re.fullmatch(r'condition\d', head):
        conds = node.get('AppliesConditions') or []
        i = int(head[9:])
        if i < len(conds) and isinstance(conds[i], dict):
            return _power(game, pid, rest, mods, level, game.merged_struct(conds[i]))
        if i == 0 and not conds:  # a condition power: the power itself is the condition
            return _power(game, pid, rest, mods, level, node)
        return None
    if rest:
        return None
    if head == 'duration':
        v = value(game, node.get('DurationMS'), level) if isinstance(node.get('DurationMS'), dict) else (
            (signed(node['DurationMS']),) * 2 if isinstance(node.get('DurationMS'), int) and node['DurationMS'] else None)
        return _seconds(v) if v else None
    if head == 'cooldown':
        v = value(game, node.get('CooldownTimeMS'), level)
        return _seconds(v) if v and v[0] > 0 else None
    if head == 'maxnumstacks':
        v = (node.get('StackingBehaviorLEGACY') or node.get('StackingBehavior') or {}).get('MaxNumStacks')
        return str(signed(v)) if isinstance(v, int) and v else None
    return _node_prop(game, node, head, level)


def _node_prop(game, node, name, level):
    """A property's value on a power or condition: set by EvalOnCreate, or in its own property groups."""
    def match(mixin_pid):
        stem = _path(game, mixin_pid).split('/')[-1].rsplit('.', 1)[0].lower()
        return name in (stem, stem[:-4] if stem.endswith('prop') else stem)
    def assigns(x):  # every AssignProp in an EvalOnCreate entry, at any depth (some sit under an And or a Scope)
        if isinstance(x, dict):
            if _path(game, x.get('_parent') or 0).endswith('/AssignProp.defaults'):
                yield x
            for k, v in x.items():
                if k != '_parent':
                    yield from assigns(v)
        elif isinstance(x, list):
            for v in x:
                yield from assigns(v)
    for a in assigns([node.get('EvalOnCreate') or [], node.get('EvalPowerSynergies') or {}]):
        prop = a.get('Prop')
        if isinstance(prop, dict) and match(prop.get('_parent')):
            return value(game, a.get('Eval'), level)
    for (bp, _), fields in (node.get('_props') or {}).items():
        stem = bp.split('/')[-1].rsplit('.', 1)[0].lower()
        if name in (stem, stem[:-4] if stem.endswith('prop') else stem):
            v = fields.get('Value')
            if 'CurveIndex' in fields:  # the value is a curve, read at the index property (only item level is known)
                index = _path(game, fields['CurveIndex']).split('/')[-1]
                if index != 'ItemLevel.defaults' or v not in game.curve_paths:
                    return None
                curve, start = game.curve(v), game.curve_start(v)
                return (curve[level - start],) * 2 if 0 <= level - start < len(curve) else None
            if isinstance(v, int) and v:
                info_pid, info = prop_info(game, game.paths.get(bp.replace('.blueprint', '.defaults')))
                kind = game.assets.get(info.get('Type')) if info_pid else None
                if kind not in ('Real', 'Int', 'Integer'):  # an unknown property, a time, a prototype: not a number
                    return None
                x = double(v) if kind == 'Real' else signed(v)
                return (x, x)
    return None


def _style_colour(game, style):
    return game.assets.get(game.merged(style).get('Color')) if style in game.prototypes else None


_TAGS = {}


def segments(game, text):
    """Tooltip text with #tag#...#/tag# colour tags as [[text, colour or None], ...] (tag colours are the game's
    text styles: #orange# is OrangeText FF9900)."""
    if not _TAGS:
        for pid, (_, path) in game.prototypes.items():
            if path.startswith('UI/TextStyles/'):
                m = game.merged(pid)
                tag = game.strings.get(m.get('Tag') or 0)
                if tag:
                    _TAGS.setdefault(tag.lower(), game.assets.get(m.get('Color')))
    out, colour = [], None
    for part in re.split(r'(#/?\w+#)', text.replace('\r', '')):
        tag = re.fullmatch(r'#(/?)(\w+)#', part)
        if tag:
            colour = None if tag.group(1) else _TAGS.get(tag.group(2).lower())
        elif part:
            out.append([part, colour])
    return out


def _affix_options(game, item, level, categories):
    """(options, left off): each affix the item can roll from the given categories as one line of text, and how
    many could not be written exactly."""
    loot = game.merged(game.paths['Globals/LootGlobals.defaults'])
    out, left = [], 0
    for e in loot.get('AffixCategoryTable') or []:
        if e.get('Category') in categories:
            for a in e.get('Affixes') or []:
                lines, hidden = entry_lines(game, game.merged(a).get('PropertyEntries') or [], level)
                if lines and not hidden:
                    out.append(segments(game, ' '.join(l['text'] for l in lines)))
                else:
                    left += 1
    return out, left


_ACTIONS = {'usepower': 'ItemActionUsePower.defaults', 'equippower': 'ItemActionAssignPower.defaults'}


def _item_text(game, m, text, level):
    """An item's description or flavour text with its $tokens$ filled from the item: its name, the hero it is
    for, and its use or equip power. Returns (text, lines left off): a line with a token that cannot be worked
    out exactly is left off."""
    def look(base, mods):
        parts = base.split('.')
        if base == 'displayname':
            return game.strings.get(m.get('DisplayName') or 0) or None
        if parts[1:] == ['displayname'] and parts[0] in ('usableby', 'charactertokencharacter'):
            return _display(game, m.get('UsableBy' if parts[0] == 'usableby' else 'Character'))
        if base == 'primarymanabehaviorname':
            return 'Spirit'  # the hero's own resource name in game; outside a hero the common one
        if base == 'charactertokendescription':  # the game writes this line in code: ours names what the token unlocks
            who = _display(game, m.get('Character'))
            kind = 'hero' if _path(game, m.get('Character') or 0).startswith('Entity/Characters/Avatars/') else 'team-up'
            return f'Unlocks the {kind} {who}.' if who else None
        if parts[0] in _ACTIONS and len(parts) > 1:
            actions = m.get('ActionsTriggeredOnItemEvent')
            for a in (actions or {}).get('Choices') or [] if isinstance(actions, dict) else []:
                if isinstance(a, dict) and _path(game, a.get('_parent') or 0).endswith(_ACTIONS[parts[0]]) and a.get('Power') in game.prototypes:
                    return _power(game, a['Power'], parts[1:], mods, level)
        return None
    kept, left = [], 0
    for line in text.replace('\r', '').split('\n'):
        filled = fill(line, look) if TOKEN.search(line) else line
        if filled is None:
            left += 1
        else:
            kept.append(filled)
    return '\n'.join(kept).strip('\n'), left


def card(game, item_pid, unique=False):
    """The item's tooltip as the game lays it out (its TooltipTemplate's sections, in order), or None for an item
    without a tooltip template. Sections: {'kind': 'text', 'text': segments} / {'kind': 'lines', 'lines': [segments]} /
    {'kind': 'options', 'title', 'options': [segments]}, each with its colour."""
    m = game.merged(item_pid)
    tpl = m.get('TooltipTemplate')
    if tpl not in game.prototypes:
        return None
    level = item_level(game, item_pid)
    lines, hidden = stat_lines(game, item_pid, level)
    asked = {c.get('Category') for lim in m.get('AffixLimits') or [] if isinstance(lim, dict)
             for c in lim.get('CategorizedAffixes') or [] if isinstance(c, dict)}
    sections, subcategory, text_left = [], False, 0
    for s in game.merged(tpl).get('TooltipSectionList') or []:
        sec = game.merged_struct(s)
        text = game.strings.get(sec.get('Text') or 0, '')
        colour = _style_colour(game, sec.get('Style')) or 'FFFFFF'
        if text == '$itemcategorywithsubcategory$':
            subcategory = True
        elif text in ('$itemdescription$', '$itemflavor$'):
            body, left = _item_text(game, m, game.strings.get(m.get('TooltipDescription' if text == '$itemdescription$' else 'TooltipFlavorText') or 0, ''), level)
            text_left += left
            if body:
                sections.append({'kind': 'text', 'color': colour, 'text': segments(game, body)})
        elif text in ('$itembasebonuses$', '$itempropertyblock0$', '$itempropertyblock1$'):
            pick = [segments(game, l['text']) for l in lines if text == '$itembasebonuses$' or l['base'] == (text == '$itempropertyblock0$')]
            if pick:
                sections.append({'kind': 'lines', 'color': colour, 'lines': pick})
        elif sec.get('IncludeCategories') and asked & set(sec['IncludeCategories']):
            options, left = _affix_options(game, m, level, asked & set(sec['IncludeCategories']))
            if options or left:
                sections.append({'kind': 'options', 'color': colour, 'title': text, 'options': options, 'hidden': left})
    hidden += len(lines) - sum(len(sec['lines']) for sec in sections if sec['kind'] == 'lines')  # lines no section shows
    stats_hidden = hidden
    hidden += text_left  # description lines left off count too
    category = game.strings.get(m.get('ItemCategory') or 0, '')
    if subcategory and game.strings.get(m.get('ItemSubcategory') or 0):
        category += ' - ' + game.strings[m['ItemSubcategory']]
    style = m.get('TextStyleOverride') or (game.paths['UI/TextStyles/RarityUnique.prototype'] if unique else None)
    binding = m.get('BindingSettings') or {}
    settings = binding.get('DefaultSettings') or {}
    if unique:  # uniques bind by their rarity's entry
        r6 = game.paths['Entity/Items/Rarity/R6Unique.prototype']
        settings = next((x for x in binding.get('PerRaritySettings') or [] if isinstance(x, dict) and x.get('RarityFilter') == r6), settings)
    title = segments(game, game.strings.get(m.get('DisplayName') or 0, ''))  # some names carry a colour tag: #orange#...#/orange#
    tagged = next((c for _, c in title if c), None)
    out = {'title': ''.join(t for t, _ in title).strip(), 'color': tagged or _style_colour(game, style) or 'FFFFFF',
           'category': category, 'sections': sections, 'hidden': hidden,
           'tradeable': bool(game.merged_struct(settings).get('IsTradable')) if isinstance(settings, dict) else True}
    if lines or stats_hidden or any(s['kind'] == 'options' for s in sections):  # gear: rolled at level 60, at its item grade
        out.update(level=60, grade=level)
    return out
