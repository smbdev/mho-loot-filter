"""Exact drop chances from Marvel Heroes Omega loot tables, rolled the way MHServerEmu's LootTablePrototype rolls them.

A node is a plain dict (see build_drops.node). roll() gives, for each (item, pool, labels), the chance of at least one
and the expected count from one roll of the node. A weighted pick is exclusive, so its choices' chances add up;
independent rolls (PickAll children, repeated picks) combine as 1 - product(1 - p)."""
from dataclasses import dataclass, field, replace
from math import comb, prod

TIERS = ('Green', 'Red', 'Cosmic')  # curve positions 0, 1, 2
LEVEL_OFFSET = 3  # LootGlobals.LootLevelDistribution: the only mob-to-item level offset in 1.52
ITEM_LEVEL = (1, 75)  # the ItemLevel property range


@dataclass(frozen=True)
class Ctx:
    tier: str
    level: int = 60  # dropper level
    hero_level: int = 60  # LevelForRequirementCheck
    verbatim: bool = False
    nodrop_mult: float = 1.0
    labels: tuple = ()
    pool: str = ''
    rarity_filter: tuple = ()  # LootRollSetRarity choices

    def item_level(self):
        return max(ITEM_LEVEL[0], min(ITEM_LEVEL[1], self.level + (0 if self.verbatim else LEVEL_OFFSET)))


@dataclass
class Stat:
    p: float
    n: float


@dataclass(frozen=True)
class Env:
    baseline: dict = field(default_factory=dict)  # path -> eLTTV_Enabled / eWETV_Enabled value in the baseline files
    events: dict = field(default_factory=dict)  # path -> names of events whose files switch it on
    pools: frozenset = frozenset()  # shared tables shown as one pool line
    rarities: dict = field(default_factory=dict)  # rarity name -> (weight curve values, curve start, downgrade-to name)
    groups: dict = field(default_factory=dict)  # table path -> [(event, item path, own no-drop or None)] (live loot groups)


def _weights(ctx, env):
    """Rarity name -> weight at the item level among the rarities the roll may pick (ItemResolver.ResolveRarity,
    no rarity bonus: weights are truncated to whole numbers)."""
    level = ctx.item_level()
    return {name: int(values[max(0, min(len(values) - 1, level - start))])
            for name, (values, start, _) in env.rarities.items() if not ctx.rarity_filter or name in ctx.rarity_filter}


def rarity_success(allowed, ctx, env):
    """Chance that a random item roll finds an item: the rarity is picked by weight at the item level, then stepped
    down (RarityPrototype.DowngradeTo) until the item family allows it."""
    if not env.rarities:
        return 1.0
    weights = _weights(ctx, env)
    total = sum(weights.values())
    if not total:
        return 0.0  # no rarity to pick: ResolveRarity fails
    if allowed and isinstance(allowed[0], dict):  # groups of family members: only those that drop at this level count
        level = ctx.item_level()
        groups = [g['rarities'] for g in allowed
                  if not any(level < lmin or (rng >= 0 and level > lmin + rng) for lmin, rng in g['levels'])]
        if not groups:
            return 0.0
        allowed = None if any(g is None for g in groups) else {r for g in groups for r in g}
    if allowed is None:
        return 1.0

    def found(name, steps=0):
        while name and steps < 10:
            if name in allowed:
                return True
            name, steps = env.rarities[name][2], steps + 1
        return False
    return sum(w for name, w in weights.items() if found(name)) / total


def named_rarity_ok(allowed, ctx, env):
    """A named item rolls its rarity among the ones it allows (ResolveRarity with the item): it drops only when one
    of them is left with weight."""
    if not env.rarities:
        return True
    return any(w > 0 and (allowed is None or name in allowed) for name, w in _weights(ctx, env).items())


OPPOSITES = {'only in: ': 'not in: ', 'only from: ': 'not from: ', 'event: ': 'not during: '}
OPPOSITES.update({v: k for k, v in OPPOSITES.items()})


def label(ctx, text):
    return ctx if text in ctx.labels else replace(ctx, labels=tuple(sorted(ctx.labels + (text,))))


def contradicts(labels):
    """A roll that must be in a place (or from a dropper, or during a condition) and also must not be never drops."""
    return any(l.startswith(k) and v + l[len(k):] in labels for l in labels for k, v in OPPOSITES.items())


KEYWORD_LABELS = {'RequireConditionKeyword': 'event', 'RequireRegionKeyword': 'only in', 'RequireDropperKeyword': 'only from',
                  'ForbidConditionKeyword': 'not during', 'ForbidRegionKeyword': 'not in', 'ForbidDropperKeyword': 'not from'}
LABELS = {'OnceDaily': 'first kill each day', 'CooldownOncePerRollover': 'once a day', 'CooldownByChannel': 'cooldown',
          'RequireKillCount': 'after enough kills', 'MissionStateRequired': 'during a mission',
          'RequireOmegaLevel': 'needs Omega levels', 'RequireRegionScenarioRarity': 'Danger Room scenario rarity'}


def apply(mods, ctx):
    """ctx after the node's roll modifiers (LootRollModifierPrototype.Apply), or None when they restrict the roll."""
    for m in mods:
        k = m['kind']
        if k == 'RequireDifficultyTier':
            if m['tiers'] and ctx.tier not in m['tiers']:
                return None
        elif k == 'ModifyDropByDifficultyTier':
            ctx = replace(ctx, nodrop_mult=ctx.nodrop_mult * m['curve'][TIERS.index(ctx.tier)])
        elif k == 'RequireLevel':
            if (m['min'] > 0 and ctx.hero_level < m['min']) or (m['max'] > 0 and ctx.hero_level > m['max']):
                return None
        elif k == 'OffsetLevel':
            ctx = replace(ctx, level=ctx.level + m['offset'])
        elif k == 'UseLevelVerbatim':
            ctx = replace(ctx, verbatim=m['on'])
        elif k == 'SetItemLevel':
            ctx = replace(ctx, level=m['level'], verbatim=True)
        elif k == 'ClampLevel':
            level = max(ctx.level, m['min']) if m['min'] > 0 else ctx.level
            ctx = replace(ctx, level=min(level, m['max']) if m['max'] > 0 else level)
        elif k == 'SetRarity':
            ctx = replace(ctx, rarity_filter=tuple(m['rarities']))
        elif k == 'RequireWeekday':
            ctx = label(ctx, 'on ' + ', '.join(m['days']))
        elif k in KEYWORD_LABELS and m['names']:  # with no keywords the modifier does nothing
            ctx = label(ctx, f"{KEYWORD_LABELS[k]}: {', '.join(m['names'])}")
            if contradicts(ctx.labels):
                return None
        elif k in LABELS:
            ctx = label(ctx, LABELS[k])
    return ctx


def switched_on(node, ctx, env):
    """ctx for a node that live tuning lets roll (labelled with the events that switch it on), or None."""
    value = env.baseline.get(node.get('path') or node.get('item'), 1)
    if value >= 2 or (value >= 1 and node.get('live', True)):
        return ctx
    events = env.events.get(node.get('path') or node.get('item'))
    return label(ctx, 'event: ' + ', '.join(sorted(events))) if events else None


def independent(results):
    out = {}
    for r in results:
        for k, s in r.items():
            o = out.setdefault(k, Stat(0.0, 0.0))
            o.p, o.n = 1 - (1 - o.p) * (1 - s.p), o.n + s.n
    return out


def mixed(weighted):
    out = {}
    for w, r in weighted:
        for k, s in r.items():
            o = out.setdefault(k, Stat(0.0, 0.0))
            o.p, o.n = o.p + w * s.p, o.n + w * s.n
    return out


def repeated(one, num):
    """one roll's results rolled k times, k even over num[0]..num[1]."""
    ks = range(num[0], num[1] + 1)
    return {k: Stat(sum(1 - (1 - s.p) ** n for n in ks) / len(ks), s.n * sum(ks) / len(ks)) for k, s in one.items()}


def first_success(qs):
    """For PickWeightTryAll: the chance that each choice is the one a pick ends on. The choices are tried in a random
    order until one succeeds, so choice i ends the pick when it succeeds and every choice tried before it failed."""
    n, out = len(qs), []
    for i, qi in enumerate(qs):
        e = [1.0]  # elementary symmetric sums of the other choices' failure chances
        for j, qj in enumerate(qs):
            if j != i:
                e = [a + (1 - qj) * b for a, b in zip(e + [0.0], [0.0] + e)]
        out.append(qi * sum(e[m] / comb(n - 1, m) for m in range(n)) / n)
    return out


def roll(node, ctx, env):
    """The results of one roll of node, or None when the node itself does not roll (restricted or switched off)."""
    return outcome(node, ctx, env)[0]


def outcome(node, ctx, env):
    """(results or None, chance the roll counts as a success). A pick of PickWeightTryAll moves on from a choice whose
    roll is not a success: one that is restricted, does not roll, or fails to find an item. A table whose NoDrop
    check stops it still succeeds (LootTablePrototype.Roll)."""
    ctx = apply(node.get('mods', ()), ctx)
    if ctx is None:
        return None, 0.0
    if node['kind'] == 'table':
        return _table(node, ctx, env)
    if node['kind'] == 'item':
        return _item(node, ctx, env)
    s = rarity_success(node.get('allowed'), ctx, env)  # ItemFilter nodes roll a random item for a slot
    return {(node['other'], ctx.pool, rarity_labels(ctx, node)): Stat(s, s)}, s


def rarity_labels(ctx, node):
    """Labels of a row; a random item rolled with a SetRarity filter keeps it, so its rarity rule stays visible."""
    if ctx.rarity_filter and (node.get('abstract') or node['kind'] == 'other'):
        return tuple(sorted(ctx.labels + ('rarity: ' + ', '.join(ctx.rarity_filter),)))
    return ctx.labels


def _table(node, ctx, env):
    ctx = switched_on(node, ctx, env)
    lo, hi = node['num']
    if ctx is None or lo < 1 or not node['choices']:
        return None, 0.0
    if node['path'] in env.pools and not ctx.pool:
        ctx = replace(ctx, pool=node['path'])
    rolled = [(c, *outcome(c, ctx, env)) for c in node['choices']]
    if node['pick'] == 'all':
        one = independent(r for _, r, _ in rolled if r is not None)
        q1 = 1 - prod(1 - q for _, _, q in rolled)
    elif node['pick'] == 'weight':
        total = sum(c['weight'] for c, _, _ in rolled if c['weight'] > 0)
        picks = [(c['weight'] / total, r, q) for c, r, q in rolled if c['weight'] > 0] if total else []
        one = mixed((w, r) for w, r, _ in picks if r is not None)
        q1 = sum(w * q for w, _, q in picks)
    else:  # tryall: every choice weighs the same
        ends = first_success([q for _, _, q in rolled])
        one = mixed((e / q, r) for e, (_, r, q) in zip(ends, rolled) if r is not None and q > 0)
        q1 = 1 - prod(1 - q for _, _, q in rolled)
    ks = range(lo, hi + 1)
    drop = (1 - node['nodrop']) * ctx.nodrop_mult
    q = (1 - min(drop, 1.0)) + min(drop, 1.0) * sum(1 - (1 - q1) ** k for k in ks) / len(ks)
    rows = repeated(one, (lo, hi))
    for event, path, own in env.groups.get(node['path'], ()):  # LootTablePrototype.PickLiveTuningNodes
        key = (path, ctx.pool, tuple(sorted(ctx.labels + ('event: ' + event,))))
        chance = 1.0 if own is None else 1 - own
        o = rows.setdefault(key, Stat(0.0, 0.0))
        o.p, o.n = 1 - (1 - o.p) * (1 - chance), o.n + chance
    return {k: Stat(s.p * drop, s.n * drop) for k, s in rows.items()}, q


def _item(node, ctx, env):
    ctx = switched_on(node, ctx, env)
    lo, hi = node['num']
    if ctx is None or hi < 1 or not node.get('approved', True):  # ApprovedForUse: DesignState Live; usable here
        return None, 0.0
    level = ctx.item_level()
    if any(level < lmin or (rng >= 0 and level > lmin + rng) for lmin, rng in node['restrict']):
        return None, 0.0  # ItemResolver.CheckItem fails the item's LootDropRestrictions
    if node['abstract']:
        s = rarity_success(node.get('allowed'), ctx, env)
    else:
        s = 1.0 if named_rarity_ok(node.get('allowed'), ctx, env) else 0.0
    if s == 0:
        return None, 0.0
    ks = range(lo, hi + 1)
    p = sum(1 - (1 - s) ** k for k in ks) / len(ks)  # each of the k items rolls its own rarity
    return {(node['item'], ctx.pool, rarity_labels(ctx, node)): Stat(p, s * sum(ks) / len(ks))}, p


def chances(node, env, **ctx):
    """Results per tier; ctx sets the roll level for sources that do not roll at the hero's (boxes)."""
    return {t: roll(node, Ctx(t, **ctx), env) or {} for t in TIERS}
