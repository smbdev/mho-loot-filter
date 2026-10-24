"""The random affixes an item can roll, worked out as MHServerEmu rolls them (LootUtilities.UpdateAffixes).

For each affix position the item's AffixLimits allow at its rarity and loot context, the pool is every affix of
that position with a weight whose DropRestrictions all allow the item (AffixPrototype.AllowAttachment), tested with
the arguments the server rolls affixes with (LootCloneRecord.Initialize): the item, its level and rarity, the rank
its own restrictions give it and the hero slot it goes in."""
from calligraphy import signed

# AffixLimits fields per position (AffixLimitsPrototype.GetLimit); Blessing, Runeword and pet slots are not rolled here
LIMITS = {'Prefix': ('MinPrefixes', 'MaxPrefixes'), 'Suffix': ('MinSuffixes', 'MaxSuffixes'),
          'Ultimate': ('NumUltimates', 'NumUltimates'), 'Cosmic': ('NumCosmics', 'NumCosmics'),
          'Unique': ('MinUniques', 'MaxUniques'), 'TeamUp': ('MinTeamUps', 'MaxTeamUps'),
          'RegionAffix': ('RegionAffixMin', 'RegionAffixMax'), 'Socket1': ('NumSocket1', 'NumSocket1'),
          'Socket2': ('NumSocket2', 'NumSocket2'), 'Socket3': ('NumSocket3', 'NumSocket3')}

_pool = {}


def _kind(r):
    return r.get('_root', '').split('/')[-1].rsplit('.', 1)[0]


def _chain(game, pid):
    out = []
    while pid in game.prototypes and pid not in out:
        out.append(pid)
        pid = game.parent(pid)
    return out


def _is_a_blueprint(game, pid, blueprint):
    """DataDirectory.PrototypeIsChildOfBlueprint: the prototype's blueprint is the blueprint or inherits from it."""
    todo, seen = [game.prototypes[pid][0]], set()
    while todo:
        b = todo.pop()
        if b == blueprint:
            return True
        if b not in seen:
            seen.add(b)
            todo += game.blueprint_parents.get(b, [])
    return False


def _rank(game, item, context):
    """ItemPrototype.GetRank: the rank its own LootDropRestrictions set (OutputRank), else 0."""
    rank = 0

    def walk(r):
        nonlocal rank
        r = game.merged_struct(r)
        k = _kind(r)
        if k == 'OutputRank':
            rank = signed(r.get('Value') or 0)
        if k == 'Conditional':
            contexts = {game.assets.get(c, '') for c in r.get('ApplyFor') or []}
            for c in (r.get('Apply') if not contexts or context in contexts else r.get('Else')) or []:
                walk(c)
        for c in r.get('Children') or []:
            walk(c)
    for r in game.merged(item).get('LootDropRestrictions') or []:
        walk(r)
    return rank


def allows(game, r, args):
    """DropRestrictionPrototype.Allow with every test on: True, False, or None for a restriction this does not know."""
    r = game.merged_struct(r)
    k = _kind(r)
    if k == 'Slot':
        return args['slot'] in {game.assets.get(s, '') for s in r.get('AllowedSlots') or []}
    if k == 'Rank':
        bits = signed(r.get('AllowedRanks') or 0)
        return bits & args['rank'] == args['rank']  # MathHelper.BitTestAll(AllowedRanks, rank)
    if k == 'Rarity':
        return args['rarity'] in (r.get('AllowedRarities') or [])
    if k == 'Level':
        lo, span = signed(r.get('LevelMin') or 0), signed(r.get('LevelRange') or 0)
        return args['level'] >= lo and (span < 0 or args['level'] <= lo + span)
    if k == 'ItemType':
        return any(_is_a_blueprint(game, args['item'], t) for t in r.get('AllowedTypes') or [])
    if k == 'ItemParent':
        chain = _chain(game, args['item'])
        return any(p in chain for p in r.get('AllowedParents') or [])
    if k == 'UsableBy':  # outside Crafting: the item is usable by one of the heroes (an any-hero item always is)
        avatars = r.get('Avatars') or []
        usable = game.merged(args['item']).get('UsableBy')
        return bool(avatars) and (not usable or usable in avatars)
    if k == 'Context':
        return args['context'] in {game.assets.get(c, '') for c in r.get('UsableFor') or []}
    if k == 'RestrictionList':
        kids = r.get('Children') or []
        results = [allows(game, c, args) for c in kids]
        if not kids or False in results:
            return False
        return None if None in results else True
    if k in ('OutputLevel', 'OutputRarity', 'OutputRank'):  # these set values, they never refuse (Allow is true)
        return True
    return None


def _affixes_at(game, position):
    """Non-abstract affixes of a position with a weight (LootPickingTable.GetAffixesByPosition)."""
    if not _pool:
        for path, pid in game.paths.items():
            if not path.startswith('Entity/Items/Affixes/') or pid in game.abstract:
                continue
            m = game.merged(pid)
            if (m.get('Weight') or 0) <= 0:  # an affix is always approved (only entities have a DesignState)
                continue
            _pool.setdefault(game.assets.get(m.get('Position') or 0, ''), []).append(pid)
    return _pool.get(position, [])


def rolls(game, item, rarity, level, context):
    """{'positions': {position: (min, max, [affix ids])}, 'unknown': affixes left out for a restriction this cannot
    test} for an item of that rarity (e.g. 'R6Unique') and level opened in a loot context (e.g. 'MysteryChest'), or
    None when the item has no AffixLimits for them."""
    m = game.merged(item)
    rarity_pid = game.paths[f'Entity/Items/Rarity/{rarity}.prototype']
    limits = None
    for lim in m.get('AffixLimits') or []:
        lim = game.merged_struct(lim) if isinstance(lim, dict) else {}
        if lim.get('ItemRarity') == rarity_pid and context in {game.assets.get(c, '') for c in lim.get('AllowedContexts') or []}:
            limits = lim  # AffixLimitsPrototype.Matches: the first for the rarity and context
            break
    if limits is None:
        return None
    args = {'item': item, 'rarity': rarity_pid, 'level': level, 'rank': _rank(game, item, context), 'context': context,
            'slot': game.assets.get(m.get('DefaultEquipmentSlot') or 0, 'Invalid')}
    out, unknown = {}, 0
    for position, (lo_f, hi_f) in LIMITS.items():
        lo, hi = signed(limits.get(lo_f) or 0), signed(limits.get(hi_f) or 0)
        if hi <= 0:
            continue
        pool = []
        for pid in _affixes_at(game, position):
            a = game.merged(pid)
            if a.get('AffixTable') or a.get('RequiredRegion') or a.get('RequiredRegionKeywords'):
                continue  # a portal's or a region's own affix: never on an item opened from a box
            results = [allows(game, r, args) for r in a.get('DropRestrictions') or []]
            if False in results:
                continue
            if None in results:
                unknown += 1
                continue
            pool.append(pid)
        out[position] = (lo, hi, pool)
    return {'positions': out, 'unknown': unknown}
