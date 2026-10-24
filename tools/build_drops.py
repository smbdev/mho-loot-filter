"""Builds the Drop finder's data: internal/db/drops.json.gz for the app (bosses, named enemies and boxes, with display
data) and dist/drops.json with every source, for the private server checker.

Usage: python tools/build_drops.py "<game folder>" "<MHServerEmu checkout>"
Needs the lz4 package."""
import collections
import gzip
import json
import os
import re
import sys

import affixes
import livetuning
import patches
import tooltip
from calligraphy import GameData, double, signed
from loot import Env, LEVEL_OFFSET, Stat, TIERS, chances

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'dist', 'drops.json')
TIER_PATHS = {'Difficulty/Tiers/Tier1Normal.prototype': 'Green', 'Difficulty/Tiers/Tier2Heroic.prototype': 'Red',
              'Difficulty/Tiers/Tier3Superheroic.prototype': 'Cosmic'}
LOOT_PROP = 'Property/Mixin/LootTablePrototypeProp.blueprint'
POOL_USERS = 10  # a shared pool is chosen by this many other tables...
POOL_ITEMS = 10  # ...and lists at least this many items itself (EndGameBossSpecialDrops: 63 uniques)
APP_OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'internal', 'db', 'drops.json.gz')
BOSS_RANKS = ('Mods/Ranks/Boss.prototype', 'Mods/Ranks/GroupBoss.prototype')  # mini-bosses are not listed
# cut content and tests; the mode folders lead to no region any waypoint reaches (checked 2026-10-21)
# CosmicGate is the Cosmic Trial, where nothing drops (user, 2026-10-21)
CUT = re.compile(r'/(Redacted|Deprecated|ZZZ[^/]*|[Tt]est|PVEInstances|PVEWaveBattle|PatrolSavage|PvEWaveGame|CosmicGate)/')
# a template: directly in Bosses/ or named as one; it is one when nothing else refers to it (checked 2026-10-21 against
# the cell and encounter files: every such file nothing refers to is placed nowhere). Elsewhere an unreferenced boss
# can still be real (Loki's terminal phase 2 is spawned by the terminal's script).
TEMPLATE = re.compile(r'^Entity/Characters/Bosses/[^/]+\.prototype$|(Base|BASETest|OLD)\.prototype$')
CHASE_BOSSES = 3  # an item is a boss's own chase drop when at most this many bosses drop it
NOT_LOOT = ('BannerMessage', 'ChatMessage', 'UsePower', 'PlayVisualEffect', 'VendorXP', 'Agent', 'OmegaPrestige',
            'VanityTitle')  # a title an achievement grants is not an item
RANDOM_NAMES = {'Armor.defaults': 'gear piece', 'Insignia.defaults': 'insignia', 'Relic.defaults': 'relic',
                'TeamUpGear.defaults': 'team-up gear', 'CraftingElement.defaults': 'crafting element',
                'CostumeCore.defaults': 'costume core', 'Artifact.defaults': 'artifact', 'Medal.defaults': 'medallion'}
SLOT_NAMES = {'Artifact01': 'artifact', 'Gear01': 'gear piece (slot 1)', 'Gear02': 'gear piece (slot 2)',
              'Gear03': 'gear piece (slot 3)', 'Gear04': 'gear piece (slot 4)', 'Gear05': 'gear piece (slot 5)',
              'Insignia': 'insignia', 'Medal': 'medallion', 'Costume': 'costume', 'Ring': 'ring'}
SECTION_CURRENCY = ('/CurrencyItems/', '/Crafting/', '/Consumables/', '/RandomGiftboxes/')
OTHER_NAMES = {'CharacterToken': 'A random hero token'}
POOL_NAMES = {'EndGameBossSpecialDrops.prototype': 'Endgame boss uniques'}


def kind(t):
    return t.get('_root', '').split('/')[-1].rsplit('.', 1)[0]


def short(game, pid):
    return game.prototypes[pid][1].split('/')[-1].rsplit('.', 1)[0] if pid in game.prototypes else str(pid)


def mod(game, m):
    m = game.merged_struct(m)
    k = kind(m)
    if k == 'RequireDifficultyTier':
        return {'kind': k, 'tiers': [TIER_PATHS[game.prototypes[c][1]] for c in m.get('Choices') or []
                                     if game.prototypes[c][1] in TIER_PATHS] or ['none'] if m.get('Choices') else []}
    if k == 'ModifyDropByDifficultyTier':
        cid = m.get('ModifierCurve')
        if not cid:
            return {'kind': k, 'curve': [1.0, 1.0, 1.0]}
        values, start = game.curve(cid), game.curve_start(cid)
        at = lambda i: values[max(0, min(len(values) - 1, i - start))]  # Curve.GetAt clamps to its ends
        return {'kind': k, 'curve': [at(i) for i in range(3)]}
    if k in ('RequireLevel', 'ClampLevel'):
        return {'kind': k, 'min': signed(m.get('LevelMin') or 0), 'max': signed(m.get('LevelMax') or 0)}
    if k == 'OffsetLevel':
        return {'kind': k, 'offset': signed(m.get('LevelOffset') or 0)}
    if k == 'UseLevelVerbatim':
        return {'kind': k, 'on': bool(m.get('UseLevelVerbatim', 1))}
    if k == 'SetItemLevel':
        return {'kind': k, 'level': signed(m.get('Level') or 0)}
    if k == 'SetRarity':
        return {'kind': k, 'rarities': [short(game, c) for c in m.get('Choices') or []]}
    if k == 'RequireWeekday':
        return {'kind': k, 'days': [game.assets.get(c, str(c)) for c in m.get('Choices') or []]}
    if k.endswith('Keyword'):
        return {'kind': k, 'names': sorted(short(game, c) for c in m.get('Choices') or [])}
    return {'kind': k}


def rank_ok(game, item, rank):
    """RankRestrictionPrototype.Allow: a rank restriction lets an item drop only for ranks inside AllowedRanks."""
    ok = True

    def walk(r):
        nonlocal ok
        r = game.merged_struct(r)
        if kind(r) == 'Rank' and r.get('AllowedRanks') is not None:
            ok &= (signed(r['AllowedRanks']) & rank) == rank
        for c in r.get('Children') or []:
            walk(c)
    for r in game.merged(item).get('LootDropRestrictions') or []:
        walk(r)
    return ok


def restrictions(game, item, context='Drop'):
    """(level restrictions as (LevelMin, LevelRange), allowed rarity names or None for any, usable in this loot
    context) of an item: restriction lists are walked, conditional ones use Apply when they apply to the context,
    else Else, and a context restriction (UsableFor) always allows CashShop too (ContextRestrictionPrototype)."""
    levels, allowed, usable = [], None, True

    def walk(r):
        nonlocal allowed, usable
        r = game.merged_struct(r)
        k = kind(r)
        if k == 'Level' or ('LevelRange' in r and 'LevelMin' in r):
            levels.append((max(signed(r.get('LevelMin') or 0), 1), max(signed(r.get('LevelRange') or 0), -1)))
        if r.get('AllowedRarities'):
            names = {short(game, x) for x in r['AllowedRarities']}
            allowed = names if allowed is None else allowed & names
        if k == 'Context' and r.get('UsableFor'):
            usable &= context in {game.assets.get(c, '') for c in r['UsableFor']} | {'CashShop'}
        if k == 'Conditional':
            contexts = {game.assets.get(c, '') for c in r.get('ApplyFor') or []}
            for c in (r.get('Apply') if not contexts or context in contexts else r.get('Else')) or []:
                walk(c)
        for c in r.get('Children') or []:
            walk(c)
    for r in game.merged(item).get('LootDropRestrictions') or []:
        walk(r)
    return levels, allowed, usable


def approved(game, pid):
    """Prototype.ApprovedForUse: the server only uses items whose DesignState is Live."""
    return game.assets.get(game.merged(pid).get('DesignState') or 0) == 'Live'


def family(game, pid):
    """The concrete items the server picks from for an abstract item: every non-abstract prototype whose blueprint
    descends from the abstract item's (LootPickingTable.GetConcreteLootPicker)."""
    if pid in _families:
        return _families[pid]
    if not _children:
        for child, parents in game.blueprint_parents.items():
            for p in parents:
                _children.setdefault(p, []).append(child)
        for q, (bp, path) in game.prototypes.items():
            _by_blueprint.setdefault(bp, []).append(q)
    todo, seen = [game.prototypes[pid][0]], set()
    while todo:
        bp = todo.pop()
        if bp not in seen:
            seen.add(bp)
            todo += _children.get(bp, [])
    _families[pid] = [q for bp in seen for q in _by_blueprint.get(bp, []) if q not in game.abstract and approved(game, q)
                      and abs(double(game.merged(q).get('LootDropWeightMultiplier', 0x3FF0000000000000) or 0)) > 1e-6]  # 0: never picked
    return _families[pid]


_families, _children, _by_blueprint = {}, {}, {}


def allowed_rarities(game, pids, context='Drop', rank=0):
    """The family's members usable in this context and rank, grouped by level restrictions: [{levels, rarities}],
    rarities None when a member takes any rarity. loot.rarity_success keeps the groups that drop at the roll level."""
    groups = {}
    for q in pids:
        levels, allowed, usable = restrictions(game, q, context)
        if usable and rank_ok(game, q, rank):
            key = tuple(sorted(levels))
            if key not in groups or groups[key] is not None:
                groups[key] = None if allowed is None else (groups.get(key) or set()) | allowed
    return [{'levels': list(k), 'rarities': None if v is None else sorted(v)} for k, v in sorted(groups.items())]


def slot_types(game):
    """UISlot name -> the item types any hero's inventory for that slot takes (LootUtilities.BuildInventoryLootPicker)."""
    out = {}
    for path, pid in game.paths.items():
        if path.startswith('Entity/Characters/Avatars/Shipping/') and pid not in game.abstract:
            for inv in game.merged(pid).get('EquipmentInventories') or []:
                inv = game.merged_struct(inv)
                types = game.merged(inv['Inventory']).get('EntityTypeFilter') or [] if inv.get('Inventory') in game.prototypes else []
                out.setdefault(game.assets.get(inv.get('UISlot') or 0, '?'), set()).update(types)
    return out


def rarities(game):
    """Rarity name -> (weight curve values, curve start, downgrade-to name), for loot.Env."""
    out = {}
    for path, pid in game.paths.items():
        if path.startswith('Entity/Items/Rarity/') and pid not in game.abstract:
            t = game.merged(pid)
            w = t.get('Weight')
            values, start = (game.curve(w), game.curve_start(w)) if w in game.curve_paths else ([0.0], 0)
            out[short(game, pid)] = (values, start, short(game, t['DowngradeTo']) if t.get('DowngradeTo') in game.prototypes else None)
    return out


def node(game, t, _depth=0, context='Drop'):
    """A loot node dict (see loot.py) from a merged table, drop or embedded struct, for a roll in this loot context
    (Drop for kills, MysteryChest for opening a box)."""
    mods = [mod(game, m) for m in t.get('Modifiers') or []]
    weight = signed(t.get('Weight', 100) or 0)
    num = (signed(t.get('NumMin', 1) or 0),)
    num = (num[0], max(signed(t.get('NumMax', 0) or 0), num[0]))
    if 'Choices' in t and 'PickMethod' in t and _depth < 50:
        pick = {'PickAll': 'all', 'PickWeight': 'weight', 'PickWeightTryAll': 'tryall'}[game.assets[t['PickMethod']]]
        return {'kind': 'table', 'path': t.get('_path', ''), 'pick': pick, 'num': num,
                'nodrop': min(max(double(t.get('NoDropPercent') or 0), 0.0), 1.0), 'weight': weight,
                'live': bool(t.get('LiveTuningDefaultEnabled', 1)), 'mods': mods,
                'choices': [node(game, game.merged_struct(c), _depth + 1, context) for c in t.get('Choices') or []]}
    if t.get('Item') in game.prototypes:
        i = t['Item']
        abstract = i in game.abstract
        levels, allowed, usable = ([], None, True) if abstract else restrictions(game, i, context)
        return {'kind': 'item', 'item': game.prototypes[i][1], 'abstract': abstract, 'num': num,
                'weight': weight, 'live': bool(game.merged(i).get('LiveTuningDefaultEnabled', 1)),
                'restrict': levels, 'mods': mods,
                'allowed': _allowed(game, (i,), context) if abstract else (None if allowed is None else sorted(allowed)),
                'approved': abstract or (approved(game, i) and usable)}
    if kind(t) == 'ItemFilter':  # a random item for an equipment slot of the hero it rolls for
        slot = game.assets.get(t.get('UISlot') or 0, '?')
        if not _slots:
            _slots.update(slot_types(game))
        return {'kind': 'other', 'other': 'ItemFilter:' + slot, 'weight': weight, 'mods': mods,
                'allowed': _allowed(game, tuple(sorted(_slots.get(slot, ()))), context, signed(t.get('ItemRank') or 0))}
    return {'kind': 'other', 'other': kind(t), 'weight': weight, 'mods': mods}


_slots, _allowed_cache = {}, {}


def reset_caches():
    """Forget the random-item families, slot types and their rarities: server patches change items too."""
    for cache in (_families, _children, _by_blueprint, _slots, _allowed_cache):
        cache.clear()


def _allowed(game, types, context, rank=0):
    """Level groups of the rarities the concrete items of these abstract item types can drop as (cached)."""
    key = (types, context, rank)
    if key not in _allowed_cache:
        _allowed_cache[key] = allowed_rarities(game, [q for t in types for q in family(game, t)], context, rank)
    return _allowed_cache[key]


def find_pools(game):
    users = collections.Counter()
    for path, pid in game.paths.items():
        if path.startswith('Loot/Tables/'):
            for c in game.merged(pid).get('Choices') or []:
                if isinstance(c, dict) and c.get('_parent') in game.prototypes:
                    users[game.prototypes[c['_parent']][1]] += 1
    def is_item(c):  # an item, or a sub-table holding just one item
        c = game.merged_struct(c) if isinstance(c, dict) else {}
        inner = c.get('Choices') or []
        return c.get('Item') in game.prototypes or (len(inner) == 1 and is_item(inner[0]))

    def items(path):
        return sum(1 for c in game.merged(game.paths[path]).get('Choices') or [] if is_item(c))
    return {p for p, n in users.items() if n >= POOL_USERS and p.startswith('Loot/Tables/') and items(p) >= POOL_ITEMS}


def sources(game):
    """Entities with a loot table property, and boxes whose use replaces them with a loot table."""
    out = []
    for path, pid in game.paths.items():
        if not path.startswith('Entity/') or pid in game.abstract:
            continue
        if len(game._merged) > 20000:  # the merge cache is only a memo, capped so the build stays near 1 GB
            game._merged.clear()
        t = game.merged(pid)
        name = clean(game.strings.get(t.get('DisplayName') or 0, ''))
        for (bp, copy), prop in sorted(t['_props'].items()):
            if bp == LOOT_PROP and prop.get('Value') in game.prototypes:
                event = game.assets.get(prop.get('Param0') or 0, 'OnKilled')
                out.append({'id': path, 'kind': 'entity', 'name': name, 'rank': game.prototypes.get(t.get('Rank'), (0, ''))[1],
                            'event': event, 'table': game.prototypes[prop['Value']][1], 'context': 'Drop', 'level': 60,
                            '_node': None})
        actions = t.get('ActionsTriggeredOnItemEvent')
        for a in (actions or {}).get('Choices') or [] if isinstance(actions, dict) else []:
            if isinstance(a, dict) and isinstance(a.get('LootTable'), dict):
                # Item.DoItemActionReplaceSelfLootTable rolls as a mystery chest at the box's own item level (a box
                # dropped for a level 60 hero is level 63), or at the hero's level when the action says so
                level = 60 if a.get('UseCurrentAvatarLevelForRoll') else 60 + LEVEL_OFFSET
                out.append({'id': path, 'kind': 'box', 'name': name, 'rank': '', 'event': 'OnUse',
                            'table': path + '#contents', 'context': 'MysteryChest', 'level': level,
                            '_node': game.merged_struct(a['LootTable'])})
    out += mission_sources(game) + raid_sources(game)
    return out


def clean(text):
    return re.sub(r'#[^#]*#|\$[^$]*\$', '', text).strip().replace(' \u2014 ', ' - ')


def mission_targets(game, x, out):
    """The entity prototypes a mission's objectives name (HasPrototype filters), in order."""
    if isinstance(x, dict):
        if x.get('EntityPrototype') in game.prototypes and x['EntityPrototype'] not in out:
            out.append(x['EntityPrototype'])
        for k, v in x.items():
            if k != '_props':
                mission_targets(game, v, out)
    elif isinstance(x, list):
        for v in x:
            mission_targets(game, v, out)
    return out


def rewards_node(game, mission):
    """Mission.Rewards as one table: the server rolls every reward table once (Mission.RollLootSummary)."""
    pick_all = next(k for k, v in game.assets.items() if v == 'PickAll')
    return {'_path': game.prototypes[mission][1] + '#rewards', 'PickMethod': pick_all,
            'Choices': [r for r in game.merged(mission).get('Rewards') or [] if isinstance(r, dict)]}


def spawn_loot_targets(game, x, out):
    """Entities a mission kills with SpawnLoot when it succeeds (a raid's invisible loot containers)."""
    if isinstance(x, dict):
        if x.get('SpawnLoot') and isinstance(x.get('EntityFilter'), dict):
            mission_targets(game, x['EntityFilter'], out)
        for k, v in x.items():
            if k != '_props':
                spawn_loot_targets(game, v, out)
    elif isinstance(x, list):
        for v in x:
            spawn_loot_targets(game, v, out)
    return out


def kill_table(game, pid):
    for (bp, _), prop in game.merged(pid)['_props'].items():
        if bp == LOOT_PROP and prop.get('Value') in game.prototypes and \
                game.assets.get(prop.get('Param0') or 0, 'OnKilled') == 'OnKilled':
            return game.prototypes[prop['Value']][1]
    return None


def raid_sources(game):
    """Raid encounters: winning one kills an invisible loot container (EntityKill, SpawnLoot), whose kill table is the
    encounter's loot. One source per table; the FiveMan copies of an encounter share its table."""
    by_table = {}
    for path, pid in sorted(game.paths.items()):
        if not path.startswith('Missions/Prototypes/Raids/') or pid in game.abstract or CUT.search('/' + path):
            continue
        t = game.merged(pid)
        targets = [(game.prototypes[e][1], game.strings.get(game.merged(e).get('DisplayName') or 0, ''),
                    game.prototypes.get(game.merged(e).get('Rank'), (0, ''))[1])
                   for e in mission_targets(game, t.get('Objectives'), [])]
        # the raid's final boss has its own rank; else every named boss of the encounter
        bosses = [(e, n) for e, n, r in targets if n and r.endswith('/BossNoOverheadInfo.prototype')] or \
                 [(e, n) for e, n, r in targets if n and r in BOSS_RANKS]
        for container in spawn_loot_targets(game, t.get('OnSuccessActions'), []):
            table = kill_table(game, container)
            if bosses and table and (table not in by_table or '/FiveMan/' in by_table[table]['id']):
                by_table[table] = {'id': path + '~loot', 'kind': 'mission', 'name': clean(game.strings.get(t.get('Name') or 0, '')),
                                   'rank': '', 'event': 'OnComplete', 'table': table, 'context': 'Drop', 'level': 60,
                                   'bosses': bosses, '_node': None}
    return list(by_table.values())


def mission_sources(game):
    """Missions that reward beating bosses, such as Hightown's Skrull impostor fights: the impostors drop only XP orbs,
    their uniques come from the event's reward. Rolled as MissionReward at the hero's level (Mission.GetLootLevel)."""
    out = []
    for path, pid in game.paths.items():
        if not path.startswith('Missions/') or pid in game.abstract or '/WeeklyEventMissions/' in path:
            continue  # weekly event missions only run during their event
        t = game.merged(pid)
        if not any(isinstance(r, dict) for r in t.get('Rewards') or []):
            continue
        bosses = [(game.prototypes[e][1], game.strings.get(game.merged(e).get('DisplayName') or 0, ''))
                  for e in mission_targets(game, t.get('Objectives'), [])
                  if game.prototypes.get(game.merged(e).get('Rank'), (0, ''))[1] in BOSS_RANKS]
        bosses = [b for b in bosses if b[1]]  # an unnamed target is no boss to list
        if bosses:
            out.append({'id': path, 'kind': 'mission', 'name': clean(game.strings.get(t.get('Name') or 0, '')),
                        'rank': '', 'event': 'OnComplete', 'table': path + '#rewards', 'context': 'MissionReward',
                        'level': 60, 'bosses': bosses, '_node': rewards_node(game, pid)})
    return out


LABEL_WORDS = {'only in: CosmicGameMode': 'Cosmic version only',  # the keyword of the Cosmic patrol and mode regions
               'not in: CosmicGameMode': 'not in the Cosmic version'}


KEYWORD_WORDS = {'MidtownManhattan': 'Midtown Patrol', 'XDefense': 'X-Defense', 'DailyBugle': 'the Daily Bugle',
                 'CosmicDailies': 'Cosmic terminals', 'CH09LowerAsgard': 'Lower Asgard', 'CH09Norway': 'Norway',
                 'EddaFireAndIceBoss': 'the Edda of Ice and Fire bosses', 'SplinterBoost': 'the Splinter bonus',
                 }
KEYWORD_ALIASES = {'Tier4': 'CosmicDailies'}  # the only live regions with Tier4 are the Cosmic terminals (2026-10-21)


def region_areas(game, x, out):
    """The area prototypes a region generator names (its Area fields, at any depth)."""
    if isinstance(x, dict):
        if x.get('Area') in game.prototypes:
            out.add(x['Area'])
        for k, v in x.items():
            if k != '_props':
                region_areas(game, v, out)
    elif isinstance(x, list):
        for v in x:
            region_areas(game, v, out)
    return out


def live_keywords(game):
    """Keywords (short names) of the live regions, those a waypoint leads to, and of their areas: a roll's region
    keywords are its region's and its area's (RegionLocation.GetKeywordsMask)."""
    out = set()
    for path, pid in game.paths.items():
        if not path.startswith('Waypoints/') or pid in game.abstract or CUT.search('/' + path):
            continue
        target = game.merged(pid).get('Destination')
        region = game.merged(target).get('Region') if target in game.prototypes else None
        if region not in game.prototypes or CUT.search('/' + game.prototypes[region][1]):
            continue
        t = game.merged(region)
        for proto in [region, *region_areas(game, t.get('RegionGenerator'), set())]:
            out |= {short(game, k) for k in game.merged(proto).get('Keywords') or [] if k in game.prototypes}
    return out


def live_labels(row_labels, live):
    """A row's labels with the keywords no live region carries dropped from its "only in" labels, or None when such
    a label keeps none (the row can never drop: only in: Tier4)."""
    out = []
    for l in row_labels:
        if l.startswith('only in: '):
            names = list(dict.fromkeys(KEYWORD_ALIASES.get(n, n) for n in l[9:].split(', ') if n in live))
            if not names:
                return None
            l = 'only in: ' + ', '.join(names)
        out.append(l)
    return tuple(out)


def pretty_label(text):
    """Display text for a row label: rarity names lose their R1-R6 prefix (R5Cosmic -> Cosmic), keywords read as
    words (only in: XDefense -> X-Defense only)."""
    if text in LABEL_WORDS:
        return LABEL_WORDS[text]
    if text.startswith('rarity: '):
        return 'rarity: ' + ', '.join(re.sub(r'^R\d', '', r) for r in text[8:].split(', '))
    kind, _, names = text.partition(': ')
    if kind in ('only in', 'not in', 'only from', 'not during') and names:
        words = ', '.join(KEYWORD_WORDS.get(n, n) for n in names.split(', '))
        return words[0].upper() + words[1:] + ' only' if kind == 'only in' and ',' not in words else f'{kind} {words}'
    return text


def item_info(game, path):
    """Display data for a row's item: name, rarity colour, icon, section, whether it is a random-item line."""
    if path.startswith('ItemFilter:'):
        slot = path.split(':', 1)[1]
        return {'path': path, 'name': 'A random ' + SLOT_NAMES.get(slot, slot.lower()), 'label': '', 'icon': '',
                'rarity': '', 'section': 'gear', 'random': True}
    if path not in game.paths:  # a drop that is not an item: Credits, CharacterToken
        return {'path': path, 'name': OTHER_NAMES.get(path, path), 'label': '', 'icon': '', 'rarity': '',
                'section': 'currency', 'random': path in OTHER_NAMES}
    pid = game.paths[path]
    t = game.merged(pid)
    if pid in game.abstract:
        stem = path.split('/')[-1]
        name = 'A random ' + RANDOM_NAMES.get(stem, game.item(pid)[0].lower() or stem.rsplit('.', 1)[0].lower())
        return {'path': path, 'name': name, 'label': '', 'icon': '', 'rarity': '', 'section': 'gear', 'random': True}
    style = game.prototypes.get(t.get('TextStyleOverride'), (0, ''))[1]
    unique = 'RarityUnique' in style or restrictions(game, pid)[1] == {'R6Unique'}  # most uniques: by their rarity
    rarity = 'cosmic' if 'RarityCosmic' in style else 'unique' if unique else ''
    return {'path': path, 'name': clean(game.item(pid)[0]) or path.split('/')[-1],
            'label': '', 'icon': '|'.join(icon_chain(game, t)),
            'rarity': rarity,
            'section': 'currency' if any(x in path for x in SECTION_CURRENCY) else 'gear', 'random': False}


def with_card(game, it):
    """An item's tooltip card (tools/tooltip.py), for named items that have one."""
    if not it['random'] and it['path'] in game.paths:
        c = tooltip.card(game, game.paths[it['path']], unique=it['rarity'] == 'unique')
        if c:
            it['card'] = c
            if it['path'].startswith('Entity/Items/Crafting/Recipes/'):
                service_card(game, it, c)
            if roll_sections(game, game.paths[it['path']], c):  # only items with Unique AffixLimits get any
                it['rarity'] = 'unique'  # it always rolls Unique: named and coloured as one
                c['color'] = tooltip._style_colour(game, game.paths['UI/TextStyles/RarityUnique.prototype']) or c['color']
        slots = item_slots(game, game.paths[it['path']])
        gear = [int(n[4:]) for n in slots if re.fullmatch(r'Gear0[1-5]', n)]
        if gear:
            it['slot'] = gear[0]
        types = item_types((c or {}).get('category', ''), slots)
        if types:
            it['types'] = types
    return it


def roll_sections(game, pid, c):
    """A unique that rolls random bonuses (the Bloodstone Demonbands) lists them: opened from a box (the Mystic
    Mayhem Coffer is the only way they drop, user, 2026-10-24) at its item grade, every bonus it can roll per position,
    as MHServerEmu picks them (tools/affixes.py, checked against 200,000 coffers rolled with the server's own code)."""
    r = affixes.rolls(game, pid, 'R6Unique', c.get('grade') or tooltip.item_level(game, pid), 'MysteryChest')
    if not r or not any(pool for _, _, pool in r['positions'].values()):
        return False
    level = c.get('grade') or tooltip.item_level(game, pid)
    at = max((i + 1 for i, sec in enumerate(c['sections']) if sec['kind'] == 'lines'), default=0)  # under its own stats
    first = True
    for position, (lo, hi, pool) in r['positions'].items():
        if not pool:
            continue
        options, left = [], 0
        for a in pool:
            lines, hidden = tooltip.entry_lines(game, game.merged(a).get('PropertyEntries') or [], level)
            if lines and not hidden:
                options.append(tooltip.segments(game, ' '.join(l['text'] for l in lines)))
            else:
                left += 1
        options.sort(key=lambda o: (re.sub(r'[\d,.%+-]+', '', ''.join(t for t, _ in o)).strip(), ''.join(t for t, _ in o)))
        count = f'{lo}' if lo == hi else f'{lo} to {hi}'
        note = f'{count} different bonus{"" if hi == 1 else "es"} of these:'
        c['sections'].insert(at, {'kind': 'options', 'color': 'FFFFFF', 'title': 'Random bonuses' if first else '',
                              'note': note if first else 'And ' + note, 'options': options, 'hidden': left})
        at += 1
        first = False
    c['hidden'] += r['unknown']
    return True


SLOT_TYPES = {'Ring': 'rings', 'Artifact01': 'artifacts', 'Artifact02': 'artifacts', 'Artifact03': 'artifacts',
              'Artifact04': 'artifacts', 'Medal': 'medallions', 'Legendary': 'legendaries', 'Relic': 'relics',
              'Insignia': 'insignias', 'UruForged': 'uniques'}


def item_slots(game, pid):
    """The hero equipment slots an item goes in (UISlot names such as Gear01 or Ring): its DefaultEquipmentSlot, else
    the slots whose inventory takes one of its types (a hero's own uniques name no slot)."""
    own = game.assets.get(game.merged(pid).get('DefaultEquipmentSlot') or 0, 'Invalid')
    if own != 'Invalid':
        return [own]
    if not _slots:
        _slots.update(slot_types(game))
    chain = set()
    while pid in game.prototypes and pid not in chain:
        chain.add(pid)
        pid = game.parent(pid)
    return sorted(n for n, types in _slots.items() if chain & set(types))


def item_types(category, slots):
    """Advanced search kinds of an item, one item can be several (a unique ring is a ring and a unique)."""
    out = {SLOT_TYPES[s] for s in slots if s in SLOT_TYPES}
    if category.startswith('Unique'):
        out.add('uniques')
    if category.endswith('Team-Up Gear'):
        out.add('teamup')
    return sorted(out)


def service_card(game, it, c):
    """A crafter's or enchanter's service (a crafting recipe the vendor holds): what it does (RecipeTooltip), what it
    works on (RecipeDescription), its ingredients and its credit cost, as the crafting panel shows them."""
    m = game.merged(game.paths[it['path']])
    tab = game.strings.get(game.merged(m['RecipeCategory']).get('DisplayText') or 0, '') if m.get('RecipeCategory') in game.prototypes else ''
    tab = tab.rstrip('.')  # "Misc."
    cost = game.merged_struct(m['CraftingCost']) if isinstance(m.get('CraftingCost'), dict) else {}
    credits = tooltip.value(game, cost['CostEvalCredits']) if isinstance(cost.get('CostEvalCredits'), dict) else None
    credits = int(credits[0]) if credits and credits[0] == credits[1] and credits[0] > 0 else None
    it['service'] = {'tab': tab, 'unlock': signed(m.get('UnlockAtCrafterRank') or 0), 'credits': credits}
    if tab:
        c['category'] = f'{tab} service'
    sections = []
    for field in ('RecipeTooltip', 'RecipeDescription'):
        body, left = tooltip._item_text(game, m, game.strings.get(m.get(field) or 0, ''), tooltip.item_level(game, game.paths[it['path']]))
        c['hidden'] += left
        if body:
            sections.append({'kind': 'text', 'color': 'FFFFFF', 'text': tooltip.segments(game, body)})
    needs = recipe_inputs(game, m)
    if needs:
        sections.append({'kind': 'text', 'color': 'FFFFFF', 'text': [
            ['Ingredients:', 'B0B0B0'], *[['\n' + (f"{n['count']} x " if n['count'] > 1 else '') + n['text'], None] for n in needs]]})
    if credits:
        sections.append({'kind': 'text', 'color': 'FFFFFF', 'text': [['Cost: ', 'B0B0B0'], [f'{credits:,} Credits', None]]})
    c['sections'] = sections + [x for x in c['sections'] if x['kind'] != 'text']


def fold_vendor_ranks(items, sources, tables):
    """One source per vendor type instead of one per rank table. The server stocks a vendor from the highest rank
    table the player has reached (Player.RollVendorInventory; a later table at the same rank replaces an earlier
    one), so each row gets the ranks it is stocked at: from, and to (absent: that rank and up). A crafting service
    also waits for its UnlockAtCrafterRank (Player.CanCraftRecipe)."""
    by_type = collections.OrderedDict()
    for o in sources:
        if o['kind'] == 'vendor':
            by_type.setdefault(o['id'].rsplit('~', 1)[0], {})[o['rank']] = o  # the last table of a rank wins
    out, folded = [o for o in sources if o['kind'] != 'vendor'], {}
    for vid, by_rank in by_type.items():
        ranks = sorted(by_rank)
        spans = {}  # row key -> [[from, to]], to 0 for "and up"
        for j, rank in enumerate(ranks):
            end = ranks[j + 1] - 1 if j + 1 < len(ranks) else 0
            for r in tables[by_rank[rank]['table']]['rows']:
                key = (r['i'], r['pool'], tuple(r['l']), tuple(r['p']), tuple(r['n']), r['s'])
                runs = spans.setdefault(key, [])
                if runs and runs[-1][1] == rank - 1:
                    runs[-1][1] = end
                else:
                    runs.append([rank, end])
        rows = []
        for (i, pool, l, p, n, server), runs in spans.items():
            unlock = (items[i].get('service') or {}).get('unlock') or 0
            for a, b in runs:
                a = max(a, unlock)
                if b and a > b:
                    continue  # unlocked only after this table is replaced
                row = {'i': i, 'pool': pool, 'l': list(l), 'p': list(p), 'n': list(n), 's': server, 'from': a}
                if b:
                    row['to'] = b
                rows.append(row)
        rows.sort(key=lambda r: (r['from'], r['i']))
        key = json.dumps(rows)
        if key not in folded:
            folded[key] = len(tables)
            tables.append({'rows': rows})
        o = dict(by_rank[ranks[0]], id=vid, table=folded[key])
        o.pop('rank', None)
        o['where'] = f"during {o['event']}" if o.get('event') else ''
        out.append(o)
    used = sorted({o['table'] for o in out})  # tables only the per-rank sources used go
    remap = {t: k for k, t in enumerate(used)}
    for o in out:
        o['table'] = remap[o['table']]
    return out, [tables[t] for t in used]


def icon_chain(game, t):
    """Texture names to try for an item, first that exists wins: RecipeIconPath (services), IconPath, IconPathHiRes, StoreIconPath, then the
    Item_ spelling some textures use (MarvelUIIcons.Unique_484 is the texture Item_Unique484)."""
    # a crafting service shows its own icon in the crafting panel (RecipeIconPath); its IconPath is the plain scroll
    names = [n for n in (game.assets.get(t.get(f) or 0, '') for f in ('RecipeIconPath', 'RecipeIconPathHiRes', 'IconPath', 'IconPathHiRes', 'StoreIconPath')) if n]
    names += [re.sub(r'^(\w+)\.([A-Za-z]+)_(\d+)$', r'\1.Item_\2\3', n) for n in names]
    return list(dict.fromkeys(names))


def label_same_names(items):
    """Items sharing a name get a label telling them apart: their rarity (or Normal), else their file name."""
    by_name = collections.defaultdict(list)
    for it in items:
        if not it['random']:  # random lines are told apart by their rarity labels instead
            by_name[it['name']].append(it)
    for group in by_name.values():
        if len(group) < 2:
            continue
        for it in group:
            it['label'] = it['rarity'].capitalize() or 'Normal'
        if len({it['label'] for it in group}) < len(group):
            stems = [it['path'].split('/')[-1].rsplit('.', 1)[0] for it in group]
            common = os.path.commonprefix(stems)
            for it, stem in zip(group, stems):  # keep the part that differs: RingDefenseLoot20 -> Defense
                rest = re.sub(r'Loot\d*$|\d+$', '', stem[len(common):]) or stem
                it['label'] = re.sub(r'(?<=[a-z])(?=[A-Z])', ' ', rest)
    for it in items:  # a Legendary and its upgraded version share a name: the game tells them apart by Item Grade
        card = it.get('card') or {}
        if card.get('category') == 'Legendary' and card.get('grade'):
            it['label'] = f"Grade {card['grade']}"


WHERE_WORDS = {'PVEDailies': 'Terminal', 'PatrolMidtown': 'Midtown Patrol', 'PatrolBrooklyn': 'Brooklyn Patrol',
               'PatrolHightown': 'Hightown Patrol', 'DangerRoom': 'Danger Room', 'RandomBossViaAffix': 'random boss',
               'XMansionDefense': 'X-Mansion defense', 'XMansionDef': 'X-Mansion defense'}


TERMINALS = {'Green': 'Green terminal', 'Red': 'Red terminal', 'Cosmic': 'Cosmic terminal'}


def where_hint(path):
    """A short hint telling a boss's versions apart until milestone 3 names their places: its folders, in words."""
    parts = [p for p in path.split('/')[2:-1] if p not in ('Bosses', 'Characters')]
    if parts[-2:-1] == ['PVEDailies'] and parts[-1] in TERMINALS:  # every terminal runs on any difficulty
        return TERMINALS[parts[-1]]
    return ', '.join(WHERE_WORDS.get(p, re.sub(r'(?<=[a-z])(?=[A-Z])', ' ', p)) for p in parts[-2:])


# Terminal number -> region name, by a majority vote of a walk from the terminal regions (2026-10-21)
TERMINAL_NUMBERS = {'01': 'Abandoned Subway', '02': "Kingpin's Warehouse", '03': 'Taskmaster Institute',
                    '04': "The Hood's Hideout", '05': 'Stryker Command Bunker', '06': 'Sinister Lab',
                    '07': 'A.I.M. Weapon Facility', '08': 'HYDRA Island', '09': 'Castle Doom', '10': 'Fisk Tower',
                    '11': "Odin's Palace", '12': 'Church of Purification', '13': 'Skrull Invasion', '14': 'Times Square'}
# names as the game's M.A.P. lists them
MODE_FOLDERS = {'PatrolMidtown': ('midtown-patrol', 'Midtown Patrol', 'Patrols'),
                'PatrolHightown': ('hightown-patrol', 'Hightown Patrol', 'Patrols'),
                'PatrolBrooklyn': ('industry-city-patrol', 'Industry City Patrol', 'Patrols'),
                'XMansionDefense': ('x-defense', 'X-Defense', 'Game modes'),
                'HoloSim': ('holo-sim', 'S.H.I.E.L.D. Holo-Sim', 'Game modes'),
                'DangerRoom': ('danger-room', 'Danger Room', 'Game modes'),
                'AgeOfUltron': ('age-of-ultron', 'The Age of Ultron', 'Game modes'),
                'CivilWar': ('civil-war', 'Civil War', 'Game modes'),
                'Limbo': ('limbo', 'Infernal Limbo', 'Game modes'),
                'DailyBugle': ('daily-bugle', 'Daily Bugle', 'Terminals'),
                'SurturRaid': ('muspelheim-raid', 'Muspelheim Raid', 'Raids'),
                'OnslaughtRaid': ('axis-raid', 'Axis Raid', 'Raids')}
# one-shot story of each one-shot boss, by the region walk (2026-10-21: each in exactly one story's regions)
ONE_SHOTS = {'Kraven': 'bronx-zoo', 'Lizard': 'bronx-zoo', 'MrHyde': 'bronx-zoo', 'RedSkull': 'march-to-axis',
             'MadameHydra': 'march-to-axis', 'ManApe': 'vibranium-mines', 'Wizard': 'vibranium-mines'}
GROUPS = ['Terminals', 'One-shot stories', 'Patrols', 'Game modes', 'Raids', 'Story']


def slug(name):
    return re.sub(r'[^a-z0-9]+', '-', name.lower()).strip('-')


PLACES = {**{'terminal-' + slug(n): (n, 'Terminals') for n in TERMINAL_NUMBERS.values()},
          **{pid: (name, group) for pid, name, group in MODE_FOLDERS.values()},
          'terminal-dimensions-collide': ('Dimensions Collide', 'Terminals'),
          'bronx-zoo': ('Bronx Zoo', 'One-shot stories'), 'march-to-axis': ('March to Axis', 'One-shot stories'),
          'vibranium-mines': ('Vibranium Mines', 'One-shot stories'),
          'story-0': ('Prologue', 'Story'), **{f'story-{n}': (f'Chapter {n}', 'Story') for n in range(1, 12)}}
# versions the rules miss, each checked with the region walk on 2026-10-21 (its one live region in the comment)
PLACE_OVERRIDES = {
    'Entity/Characters/Bosses/Story/BlackCat.prototype': 'story-0',  # TutorialBankRegion
    'Entity/Characters/Bosses/Story/DrDoomPhase3.prototype': 'story-8',  # CH0809DrDoomBossRegion
    'Entity/Characters/Bosses/Story/LokiPhase2.prototype': 'story-9',  # CH0906LokiBossRegion
    'Entity/Characters/Bosses/MoleMan.prototype': 'story-4',  # CH0409MoloidRegion, MoloidInvasionRegion
    'Entity/Characters/Bosses/SauronBase.prototype': 'story-7',  # CH0702SauronCavesRegion
    'Entity/Characters/Mobs/Sentinels/Chapter/SentinelsCH05/SentinelOMCH05.prototype': 'story-5',  # CH0501MutantTown
    'Entity/Characters/Bosses/SecretInvasion/SkrullNickFury.prototype': 'story-10',  # HelicarrierInvasionRegion
    'Entity/Characters/Bosses/SecretInvasion/SkrullElektra.prototype': 'story-10',  # HandDojoRegion
    'Entity/Characters/Mobs/DoopAllChapters/CosmicDoop/CosmicDoopOverlordLargeAggro.prototype': 'danger-room',
    'Entity/Characters/Mobs/Sentinels/Event/StarktechSentinelBossBase.prototype': 'danger-room',  # Trainyard
    # in the CivilWar folder, but each is used only by a patrol's ambient boss list
    'Entity/Characters/Bosses/CivilWar/CrossbonesPatrolBossMidtown.prototype': 'midtown-patrol',
    'Entity/Characters/Bosses/CivilWar/CrossbonesPatrolBossBrooklyn.prototype': 'industry-city-patrol',
    'Entity/Characters/Bosses/CivilWar/CrossbonesPatrolBossHightown.prototype': 'hightown-patrol',
}
# DarkDimension is not a place: the Doctor Strange invasion spawns its bosses inside patrols and terminals


def place_of(path):
    """(place id, version label) of a boss version, or None when the data names no single place."""
    if path in PLACE_OVERRIDES:
        return PLACE_OVERRIDES[path], ''
    m = re.search(r'/PVEDailies/(Green|Red|Cosmic)/EGD(\d\d)', path)
    if m and m.group(2) == '15':  # the Vulture, at the Daily Bugle
        return 'daily-bugle', ''
    if m and m.group(2) in TERMINAL_NUMBERS:
        return 'terminal-' + slug(TERMINAL_NUMBERS[m.group(2)]), TERMINALS[m.group(1)]
    if '/PVEDailies/Green/DailyBugle' in path:
        return 'daily-bugle', ''
    m = re.search(r'/OneShots/(\w+?)OneShot\.prototype$', path)
    if m:
        return (ONE_SHOTS[m.group(1)], '') if m.group(1) in ONE_SHOTS else None
    if '/DarkDimension/' in path:  # Dimensions Collide; the Midtown ones come only with the Doctor Strange event
        stem = path.split('/')[-1]
        return None if 'Midtown' in stem else ('terminal-dimensions-collide', 'Cosmic' if 'Cosmic.' in stem else '')
    m = re.search(r'CH(\d+)', path.split('/')[-1], re.I)
    if m and ('/Story/' in path or path.count('/') == 3):  # a chapter tag; CH0410 is chapter 4, mission 10
        digits = m.group(1)
        return f'story-{int(digits[:2] if len(digits) > 2 else digits)}', ''
    for folder in path.split('/'):
        if folder in MODE_FOLDERS:
            return MODE_FOLDERS[folder][0], 'random boss' if '/RandomBossViaAffix/' in path else ''
    return None


def patrol_tiers(game):
    """{(place id, version): tier indexes}: the difficulties each patrol map opens on, from its regions'
    AccessDifficulties (the regular patrols Normal and Heroic, their Cosmic maps Cosmic only; no list means all),
    the Heroic-only game modes, and each terminal colour's own difficulty."""
    cosmic = game.paths['Regions/Keywords/MiscModes/CosmicGameMode.prototype']
    tier_of = {game.paths[f'Difficulty/Tiers/{t}.prototype']: i
               for i, t in enumerate(('Tier1Normal', 'Tier2Heroic', 'Tier3Superheroic'))}
    out = {}
    for folder, (place, _, group) in MODE_FOLDERS.items():
        if group != 'Patrols':
            continue
        for pid, (_, path) in game.prototypes.items():
            if not path.startswith(f'Regions/EndGame/TierX/{folder}/') or pid in game.abstract:
                continue
            m = game.merged(pid)
            if 'AccessDifficulties' not in m:
                continue  # not a region
            version = 'Cosmic' if cosmic in (m.get('Keywords') or []) else ''
            tiers = {tier_of[t] for t in m.get('AccessDifficulties') or [] if t in tier_of} or {0, 1, 2}
            out.setdefault((place, version), set()).update(tiers)
    # game modes that only run on Heroic, their Cosmic scenarios too (user 2026-10-24; region data agrees except
    # Civil War, whose maps list Normal and Heroic)
    for place in HEROIC_ONLY:
        for version in ('', 'Cosmic', 'random boss', 'random boss, Cosmic'):
            out[place, version] = {1}
    # a terminal's colour is its difficulty, picked before joining and fixed inside: Green Normal, Red Heroic,
    # Cosmic Cosmic (user 2026-10-24; the region data sets no limit, each colour has its own waypoint)
    for name in TERMINAL_NUMBERS.values():
        for tier, version in enumerate(TERMINALS.values()):
            out['terminal-' + slug(name), version] = {tier}
    return out


HEROIC_ONLY = ('danger-room', 'x-defense', 'holo-sim', 'civil-war', 'limbo')


def limit_patrol_tiers(game, sources, tables):
    """Clear the chances of a patrol, game mode or terminal version's table on difficulties it cannot be played on. A table shared
    with a place under other rules (Mister Sinister: Industry City Patrol and X-Defense) is copied for the patrol."""
    allowed = patrol_tiers(game)
    by_table = collections.defaultdict(list)
    for o in sources:
        by_table[o['table']].append(o)
    for t, users in list(by_table.items()):
        groups = collections.defaultdict(list)
        for o in users:
            groups[frozenset(allowed.get((o['place'], o['version']), ()))].append(o)
        for tiers, group in groups.items():
            if not tiers or tiers == {0, 1, 2}:
                continue
            if len(groups) > 1:  # other places use this table too: give this group its own copy
                tables.append({'rows': [dict(r) for r in tables[t]['rows']]})
                for o in group:
                    o['table'] = len(tables) - 1
            for row in tables[group[0]['table']]['rows']:  # new lists: a table's versions share the rolled ones
                row['p'] = [v if i in tiers else 0.0 for i, v in enumerate(row['p'])]
                row['n'] = [v if i in tiers else 0.0 for i, v in enumerate(row['n'])]
            table = tables[group[0]['table']]
            table['rows'] = [r for r in table['rows'] if any(r['p'])]  # rows it can never drop go


def vendor_npcs(game):
    """Vendor type id -> the vendor characters that use it (test characters left out)."""
    types = {pid for path, pid in game.paths.items() if path.startswith('Entity/Characters/Vendors/VendorTypes/')}
    out = collections.defaultdict(list)

    def refs(x, found):
        if isinstance(x, dict):
            for k, v in x.items():
                if k != '_parent':
                    refs(v, found)
        elif isinstance(x, list):
            for v in x:
                refs(v, found)
        elif x in types:
            found.add(x)
    for path, pid in game.paths.items():
        if path.startswith('Entity/Characters/Vendors/') and not path.startswith('Entity/Characters/Vendors/VendorTypes/') \
                and '/Test' not in path and pid not in game.abstract:
            found = set()
            refs(game.tree(pid), found)
            for t in found:
                out[t].append(path)
    return out


def vendor_sources(game, mhserver_dir):
    """Each vendor type's stock tabs: a loot table per vendor rank, rolled once per stock refresh. A vendor whose
    characters the server hides until an event shows them is tagged with those events; one no event shows goes."""
    hidden, shown_by = livetuning.load_visible(mhserver_dir)
    npcs = vendor_npcs(game)
    out = []
    for path, pid in game.paths.items():
        if not path.startswith('Entity/Characters/Vendors/VendorTypes/') or pid in game.abstract or 'Test' in path:
            continue
        v = game.merged(pid)
        name = game.strings.get(v.get('TypeName') or 0, '')
        if not name:  # internal vendors with no name (the Bifrost token exchange)
            continue
        chars = npcs.get(pid, [])
        if chars and all(c in hidden for c in chars):
            events = sorted({e for c in chars for e in shown_by.get(c, [])})
            if not events:
                continue  # hidden and no event shows it
            event = ', '.join(events)
        else:
            event = ''
        tabs = [game.strings.get(game.merged(c).get('DisplayText') or 0, '').rstrip('.') for c in v.get('CraftingRecipeCategories') or []
                if c in game.prototypes]  # a crafter's panel tabs, in the game's order
        for inv in v.get('Inventories') or []:
            inv = game.merged_struct(inv) if isinstance(inv, dict) else {}
            if inv.get('LootTable') not in game.prototypes:
                continue
            rank = max(1, signed(inv.get('UseStartingAtVendorLevel') or 0))
            out.append({'id': f'{path}~{rank}', 'kind': 'vendor', 'name': name, 'rank': rank, 'during': event, 'event': 'OnBuy',
                        'table': game.prototypes[inv['LootTable']][1], 'context': 'Vendor', 'level': 60, '_node': None,
                        'tabs': [t for t in tabs if t]})
    return out


def _contains(x, key):
    if isinstance(x, dict):
        return key in x or any(_contains(v, key) for k, v in x.items() if k != '_parent')
    return isinstance(x, list) and any(_contains(v, key) for v in x)


def recipe_inputs(game, recipe):
    """A recipe's inputs in words: named ingredients with counts, a choice of items, or an item you supply."""
    out = []
    for i in recipe.get('RecipeInputs') or []:
        if not isinstance(i, dict):
            continue
        kind = short(game, i.get('_parent') or 0)
        m = game.merged_struct(i)
        count = signed(m.get('Quantity') or 1) or 1
        if kind == 'AutoPopulated' and m.get('Ingredient') in game.prototypes:
            out.append({'item': game.prototypes[m['Ingredient']][1], 'count': count, 'text': clean(game.item(m['Ingredient'])[0])})
        elif kind == 'AllowedItemList' and m.get('AllowedItems'):
            names = [clean(game.item(x)[0]) for x in m['AllowedItems'] if x in game.prototypes]
            out.append({'item': '', 'count': count, 'text': ' or '.join(dict.fromkeys(n for n in names if n))})  # Personal copies share a name
        elif kind == 'MatchFirst':
            out.append({'item': '', 'count': count, 'text': 'the same item again'})
        else:  # RestrictionSet: an item of a kind you put in
            slot = game.merged(m['SlotDisplayInfo']) if m.get('SlotDisplayInfo') in game.prototypes else {}
            text = clean(game.strings.get(slot.get('DisplayName') or slot.get('Tooltip') or 0, ''))
            out.append({'item': '', 'count': count, 'text': text or 'the item you put in'})
    return out


def recipe_sources(game):
    """Recipes a crafter knows that make a new item (upgrades, which rework the item you put in, are left out)."""
    crafters = collections.defaultdict(list)
    for path, pid in game.paths.items():
        if path.startswith('Entity/Characters/Vendors/VendorTypes/') and pid not in game.abstract and 'Test' not in path:
            v = game.merged(pid)
            for c in v.get('CraftingRecipeCategories') or []:
                name = game.strings.get(v.get('TypeName') or 0, '')
                if name and name not in crafters[c]:
                    crafters[c].append(name)
    out = []
    for path, pid in game.paths.items():
        if not path.startswith('Entity/Items/Crafting/Recipes/') or pid in game.abstract:
            continue
        r = game.merged(pid)
        crafter = crafters.get(r.get('RecipeCategory'))
        if not crafter or not isinstance(r.get('RecipeOutput'), dict) or _contains(r['RecipeOutput'], 'Mutations'):
            continue
        out.append({'id': path, 'kind': 'recipe', 'name': clean(game.item(pid)[0]) or short(game, pid), 'rank': '',
                    'crafter': ', '.join(crafter), 'inputs': recipe_inputs(game, r), 'table': path + '#output', 'event': 'OnCraft',
                    'context': 'Crafting', 'level': 60, '_node': game.merged_struct(r['RecipeOutput'])})
    return out


MASK = 2 ** 64 - 1


def load_achievements(mhserver_dir):
    """(infos by id, en_us text of a string id): AchievementInfoMap*.json, later files overriding earlier ones by Id,
    and the AchievementStringMap*.json strings (ids are 64-bit and can be stored signed)."""
    folder = os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'Achievements')
    infos, strings = {}, {}
    for name in sorted(os.listdir(folder)):
        full = os.path.join(folder, name)
        if name.startswith('AchievementInfoMap') and name.endswith('.json'):
            infos.update({a['Id']: a for a in json.load(open(full, encoding='utf-8'))})
        elif name.startswith('AchievementStringMap') and name.endswith('.json'):
            strings.update(json.load(open(full, encoding='utf-8')))

    def text(sid):
        return (strings.get(str((sid or 0) & MASK)) or strings.get(str(sid or 0)) or {}).get('en_us', '') if sid else ''
    return infos, text


def achievement_sources(game, mhserver_dir):
    """Enabled achievements with a reward loot table (AchievementInfoMap*.json; ids are GUIDs, names in the string map)."""
    infos, text = load_achievements(mhserver_dir)
    out = []
    for a in infos.values():
        guid = (a.get('Context') or {}).get('RewardPrototype') or 0
        table = game.guids.get(guid & MASK) or (guid & MASK)
        title = text(a.get('Name'))
        if a.get('Enabled') and table in game.prototypes and title:
            out.append({'id': f'achievement:{a["Id"]}', 'kind': 'achievement', 'name': title, 'rank': '', 'event': 'OnAchieve',
                        'table': game.prototypes[table][1], 'context': 'AchievementReward', 'level': 60, '_node': None})
    return out


# AchievementVisibleState (MHServerEmu Achievements/AchievementInfo.cs): 1 Visible, 2 Invisible, 3 ParentComplete,
# 4 Complete (shown once earned), 5 InProgress, 6 Objective (a step inside another achievement)
ACH_SHOWN = (1, 2, 3, 4)


def reward_items(items, sources, tables, path):
    """The items an achievement reward really is: a reward box that always gives the same named items is those items
    (Hannibal King's Hand Cannon Achievement Reward gives the gun), anything else is itself."""
    box = sources.get(path)
    if box and box['kind'] == 'box':
        rows = tables[box['table']]['rows']
        if rows and all(not items[r['i']]['random'] and min(r['p']) >= 1 for r in rows):
            return [r['i'] for r in rows]
    return None


def reward_types(game, items, item_ids, reward, titles):
    """Advanced search reward types of an achievement (an achievement can have several)."""
    out = set()
    for i in item_ids:
        it, cat = items[i], (items[i].get('card') or {}).get('category', '')
        name = it['name']
        if 'Credit' in name:
            out.add('credits')
        elif 'Coffer' in name:
            out.add('coffer')
        elif 'Infinity' in name and 'Orb' in name:
            out.add('orb')
        elif 'Commendation' in name:
            out.add('commendation')
        elif 'Costume' in cat or 'Costume' in name:
            out.add('costume')
        elif it.get('types') or it.get('section') == 'gear' or cat in ('Pet', 'Flourish'):
            out.add('gear')
        else:
            out.add('boost')
    if titles:
        out.add('title')
    if not out:  # nothing it gives is an item or a title; some achievements give a coffer with no reward line
        out = {'none'}
    return sorted(out)


def write_achievements(game, mhserver_dir, items, sources_out, tables, patched, keys):
    """(categories, achievements) for the Achievements page: every achievement the game's window can show, with
    its reward items (indexes into items). keys: achievement source id -> its table key in patched."""
    infos, text = load_achievements(mhserver_dir)
    by_id = {s['id']: s for s in sources_out}
    by_path = {s['id']: s for s in sources_out if s['kind'] == 'box'}
    item_of = {it['path']: i for i, it in enumerate(items)}
    shown, left = [], 0
    steps = collections.defaultdict(list)  # parent id -> its objectives (VisibleState 6), as the expanded row lists them
    for a in infos.values():
        if a.get('Enabled') and a.get('VisibleState') == 6 and a.get('ParentId'):
            steps[a['ParentId']].append(a)
    for a in infos.values():
        name, desc = text(a.get('Name')), text(a.get('InProgressStr'))
        vis = a.get('VisibleState')
        if not a.get('Enabled') or vis not in ACH_SHOWN or (vis == 2 and not (name and desc)):
            continue
        if not name:
            left += 1
            continue
        if '$' in desc:  # a token the window fills from the player's own progress: not written out here
            desc, left = '', left + 1
        src = by_id.get(f'achievement:{a["Id"]}')
        ids = []
        for r in tables[src['table']]['rows'] if src else []:
            inner = reward_items(items, by_path, tables, items[r['i']]['path'])
            ids += [i for i in (inner or [r['i']]) if i not in ids]
        key = keys.get(f'achievement:{a["Id"]}')
        titles = key is not None and any(k[0] == 'VanityTitle' for tier in TIERS for k in patched[key][tier])
        reward = re.sub(r'^Reward:\s*', '', text(a.get('RewardStr')))
        icon = game.assets.get((a.get('IconPathAssetId') or 0) & MASK, '')
        objectives = []
        for o in steps.get(a['Id'], []):
            o_name = text(o.get('Name'))
            if not o_name:
                left += 1
                continue
            item = -1  # an objective to loot an item names it in its event data
            for e in (o.get('Context') or {}).get('EventData') or []:
                ref = (e.get('Prototype') or 0) & MASK
                pid = game.guids.get(ref) or ref
                if pid in game.prototypes and game.prototypes[pid][1] in item_of:
                    item = item_of[game.prototypes[pid][1]]
                    break
            objectives.append({'id': o['Id'], 'name': o_name, 'dep': o.get('DependentAchievementId') or 0, 'item': item})
        objectives.sort(key=lambda o: (o['name'].lower(), o['id']))  # the game lists them by name
        shown.append({'id': a['Id'], 'name': name, 'desc': tooltip.segments(game, desc), 'reward': reward, 'items': ids,
                      'icon': icon, 'points': a.get('Score') or 0, 'order': a.get('DisplayOrder') or 0, 'vis': vis,
                      'parent': a.get('ParentId') or 0, 'party': bool(a.get('PartyVisible')),
                      'rtypes': reward_types(game, items, ids, reward, titles), 'objectives': objectives,
                      '_cat': text(a.get('CategoryStr')), '_sub': text(a.get('SubCategoryStr'))})
    ids = {a['id'] for a in shown}
    for a in shown:
        for o in a['objectives']:
            if o['dep'] not in ids:
                o['dep'] = 0  # only an achievement the page lists
    cats = {}
    for a in shown:
        if a['parent'] not in ids:
            a['parent'] = 0  # only a tier the page also lists
        cats.setdefault(a['_cat'], set()).add(a['_sub'])
    names = sorted(cats)  # the game's window lists categories and subcategories by name
    out_cats = [{'name': c, 'subs': sorted(cats[c])} for c in names]
    for a in shown:
        a['cat'] = names.index(a.pop('_cat'))
        a['sub'] = out_cats[a['cat']]['subs'].index(a.pop('_sub'))
    shown.sort(key=lambda a: (a['cat'], a['sub'], a['order'], a['id']))
    print(f'{len(shown)} achievements ({sum(a["vis"] == 4 for a in shown)} secret), {left} names or descriptions left off')
    return out_cats, shown


def store_sources(game, mhserver_dir):
    """Store entries and the items they give (MTXStore/Catalog*.json); not rolled, and no real-money price."""
    folder = os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'MTXStore')
    out = []
    for name in sorted(os.listdir(folder)):
        if not (name.startswith('Catalog') and name.endswith('.json')):
            continue
        for e in json.load(open(os.path.join(folder, name), encoding='utf-8')):
            title = next((l.get('Title', '') for l in e.get('LocalizedEntries') or [] if l.get('LanguageId') == 'en_us'), '')
            paths = [game.prototypes[x][1] for gi in e.get('GuidItems') or []
                     for x in [gi.get('ItemPrototypeRuntimeIdForClient', 0) & (2 ** 64 - 1)] if x in game.prototypes]
            paths = [p for p in paths if p.startswith('Entity/Items/')]
            if paths and title:
                out.append({'id': f'store:{e["SkuId"]}', 'kind': 'store', 'name': title, 'rank': '', 'event': 'OnBuy',
                            'table': f'store:{e["SkuId"]}', 'context': 'CashShop', 'level': 60, '_node': None,
                            '_fixed': paths})
    return out


def item_price(game, path):
    """[{'amount', 'currency'}] for an item's vendor price, when every part is one fixed number (credits use a
    level formula and are left out; a price that depends on the rolled rarity or affixes gives None)."""
    pid = game.paths.get(path)
    cost = (game.merged(pid).get('Cost') or {}) if pid else {}
    cost = game.merged_struct(cost) if isinstance(cost, dict) and cost else {}
    if any(isinstance(cost.get(f), dict) and tooltip.value(game, cost[f]) not in (None, (0, 0)) for f in ('Credits', 'Runestones')):
        return None
    out = []
    for c in cost.get('Components') or []:
        if not isinstance(c, dict):
            continue
        kind, m = short(game, c.get('_parent') or 0), game.merged_struct(c)
        if 'Currency' in m and 'Amount' in m:
            what, amount = m['Currency'], m['Amount']
        elif 'CurrencyItem' in m and 'Number' in m:
            what, amount = m['CurrencyItem'], m['Number']
        else:  # credits, runestones: a part we cannot work out, so no price rather than part of one
            return None
        v = tooltip.value(game, amount) if isinstance(amount, dict) else None
        name = clean(game.item(what)[0]) if what in game.prototypes else ''
        if not v or v[0] != v[1] or v[0] <= 0 or not name:
            return None
        out.append({'amount': int(v[0]), 'currency': name})
    return out or None


FROM = re.compile(r'^(only|not) from: (.+)$')


def _resolved(rows, kws):
    """A tier's rows for one dropper: 'only from' rows kept when it has one of the keywords, 'not from' rows kept
    when it has none, the label gone either way; rows that meet under one key combine as separate rolls."""
    out = {}
    for (path, pool, labels), st in rows.items():
        keep, rest = True, []
        for l in labels:
            m = FROM.match(l)
            if not m:
                rest.append(l)
            elif (m.group(1) == 'only') != bool(kws & set(m.group(2).split(', '))):
                keep = False
        if not keep:
            continue
        k = (path, pool, tuple(rest))
        if k in out:
            out[k] = Stat(1 - (1 - out[k].p) * (1 - st.p), out[k].n + st.n)
        else:
            out[k] = Stat(st.p, st.n)
    return out


def resolve_droppers(game, srcs, base, patched):
    """Rows that need (or forbid) a dropper keyword, settled per killed entity: the Industry City patrol tables give
    every boss "only from the Edda of Ice and Fire bosses" Broken Crown rows that only the Surtur demons can drop.
    Sources whose entity has different keywords get their own copy of the table."""
    for s in srcs:
        key = s['table']
        if s['event'] != 'OnKilled' or s['id'] not in game.paths:
            continue
        labels = {l for tier in TIERS for k in patched[key][tier] for l in k[2] if FROM.match(l)}
        if not labels:
            continue
        named = {n for l in labels for n in FROM.match(l).group(2).split(', ')}
        kws = {short(game, k) for k in game.merged(game.paths[s['id']]).get('Keywords') or []} & named
        new = f"{key}@{','.join(sorted(kws))}"
        for rolled in (base, patched):
            if new not in rolled:
                rolled[new] = {tier: _resolved(rolled[key][tier], kws) for tier in TIERS}
        if kws and new not in _PLAIN_TABLES:
            _PLAIN_TABLES[new] = {tier: _resolved(patched[key][tier], set()) for tier in TIERS}
        s['table'] = new


REGION_LABEL = re.compile(r'^(only|not) in: (.+)$')


def terminal_contexts(game):
    """{(terminal name, colour): [keyword sets]}: the keywords a roll can see in each Green, Red and Cosmic terminal
    map a level 60 hero plays, one set per region and area (a roll's region keywords are its region's and its area's,
    RegionLocation.GetKeywordsMask). Terminals are named by their folder's RegionName."""
    regions, names = [], {}  # a terminal's folder takes the name any of its regions carries (Red ones often none)
    for pid, (_, path) in game.prototypes.items():
        if path.startswith('Regions/EndGame/Terminals/') and pid not in game.abstract and 'AccessDifficulties' in game.merged(pid):
            folder = path.split('/')[4]
            regions.append((pid, path.split('/')[3], folder))
            names[folder] = names.get(folder) or game.strings.get(game.merged(pid).get('RegionName') or 0, '')
    # a level 60 hero plays each terminal's level 60 map (the banded entry picks it: DailyRPurifierChurchRegionL60,
    # level 63), whose keywords can differ from the lower maps' (the Red level 60 maps carry Tier4, not RedDailies)
    top = {}
    for pid, colour, folder in regions:
        top[folder, colour] = max(top.get((folder, colour), 0), signed(game.merged(pid).get('Level') or 0))
    out = collections.defaultdict(list)
    for pid, colour, folder in regions:
        m = game.merged(pid)
        name = names.get(folder)
        if not name or colour not in TERMINALS or signed(m.get('Level') or 0) != top[folder, colour]:
            continue
        own = {short(game, k) for k in m.get('Keywords') or [] if k in game.prototypes}
        areas = region_areas(game, m.get('RegionGenerator'), set())
        for area in areas or [None]:
            kws = own | ({short(game, k) for k in game.merged(area).get('Keywords') or [] if k in game.prototypes} if area else set())
            if kws not in out[name, TERMINALS[colour]]:
                out[name, TERMINALS[colour]].append(kws)
    return out


def _region_settled(rows, contexts):
    """A tier's rows for one map: "only in" (any of the keywords) and "not in" (none of them) rows kept when some
    region or area of the map allows them, their labels gone when every one does."""
    out = {}
    for (path, pool, labels), st in rows.items():
        region = [l for l in labels if REGION_LABEL.match(l)]
        def allows(kws):
            return all((m.group(1) == 'only') == bool(kws & set(m.group(2).split(', '))) for m in map(REGION_LABEL.match, region))
        ok = [allows(c) for c in contexts]
        if region and not any(ok):
            continue
        k = (path, pool, tuple(l for l in labels if not (region and all(ok) and l in region)))
        out[k] = Stat(1 - (1 - out[k].p) * (1 - st.p), out[k].n + st.n) if k in out else Stat(st.p, st.n)
    return out


def resolve_terminal_regions(game, srcs, base, patched):
    """Rows that need (or forbid) a region keyword, settled for the terminal map a boss version fights in: a
    Green Church of Purification Juggernaut cannot drop the Cosmic terminals' (CosmicDailies) Shillelagh."""
    contexts = terminal_contexts(game)
    for s in srcs:
        found = place_of(s['id']) if s['id'] in game.paths else None
        if not found or not found[0].startswith('terminal-'):
            continue
        name = next((n for n in TERMINAL_NUMBERS.values() if 'terminal-' + slug(n) == found[0]), None)
        ctx = contexts.get((name, found[1]))
        key = s['table']
        named = {n for tier in TIERS for k in patched[key][tier] for l in k[2] if REGION_LABEL.match(l)
                 for n in REGION_LABEL.match(l).group(2).split(', ')}
        if not ctx or not named:
            continue  # no regions found for this map, or no region rows: the table stays as it is
        # maps that see the same keywords of these rows settle the same way, so they share one table
        seen = sorted({','.join(sorted(c & named)) for c in ctx})
        new = f"{key}@in:{'|'.join(seen)}"
        for rolled in (base, patched):
            if new not in rolled:
                rolled[new] = {tier: _region_settled(rolled[key][tier], ctx) for tier in TIERS}
        s['table'] = new


def roll_all(game, env, srcs):
    """{table key: {tier: {row key: Stat}}} for every source's table, rolled once each."""
    out = {}
    for s in srcs:
        if s.get('_fixed'):  # a store entry gives its items
            out[s['table']] = {t: {(p, '', ()): Stat(1.0, 1.0) for p in s['_fixed']} for t in TIERS}
        elif s['table'] not in out:
            t = s['_node'] or game.merged(game.paths[s['table']])
            out[s['table']] = chances(node(game, t, context=s['context']), env, level=s['level'], hero_level=s['level'])
    return out


def write_debug(game, srcs, rolled):
    """dist/drops.json: every source and row, for the private server checker."""
    tables, items = {}, {}
    for key, tiers in rolled.items():
        tables[key] = {tier: [[k[0], k[1], list(k[2]), st.p, st.n] for k, st in sorted(r.items()) if st.p > 0]
                       for tier, r in tiers.items()}
        for tier in TIERS:
            for row in tables[key][tier]:
                if row[0] in game.paths and row[0] not in items:
                    pid = game.paths[row[0]]
                    items[row[0]] = {'name': game.item(pid)[0], 'abstract': pid in game.abstract}
    pools = sorted({row[1] for t in tables.values() for rows in t.values() for row in rows if row[1]})
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, 'w') as f:
        json.dump({'tiers': list(TIERS), 'items': items, 'pools': pools,
                   'sources': [{k: v for k, v in s.items() if k != '_node'} for s in srcs], 'tables': tables}, f)


def is_event(row_labels):
    return any(l.startswith('event: ') for l in row_labels)


def referenced(game, pids):
    """The pids another prototype refers to, other than by inheriting from them."""
    found = set()

    def walk(x, src):
        if isinstance(x, dict):
            for k, v in x.items():
                if k == '_props':
                    for pv in v.values():
                        walk(pv, src)
                elif k != '_parent':
                    walk(v, src)
        elif isinstance(x, list):
            for v in x:
                walk(v, src)
        elif isinstance(x, int) and x in pids and x != src:
            found.add(x)
    for pid in game.prototypes:
        walk(game.tree(pid), pid)
    return found


def real_bosses(game, srcs, patched):
    """(boss versions, chase item paths): a boss is a named Boss-rank enemy with chase drops of its own, a unique,
    artifact or medallion that at most CHASE_BOSSES named enemies drop outside events and shared pools."""
    templates = {game.paths[s['id']] for s in srcs if TEMPLATE.search(s['id'])}
    templates -= referenced(game, templates)
    candidates = [s for s in srcs if s['event'] == 'OnKilled' and s['rank'] in BOSS_RANKS and s['name']
                  and not CUT.search(s['id']) and game.paths[s['id']] not in templates]
    info = {}

    def own_gear(s):  # the unique, artifact and medallion paths a source drops itself, outside events and pools
        for tier in TIERS:
            for (path, pool, row_labels), st in patched[s['table']][tier].items():
                if st.p > 0 and not pool and not is_event(row_labels) and path not in NOT_LOOT:
                    it = info.get(path) or info.setdefault(path, item_info(game, path))
                    if not it['random'] and it['section'] == 'gear' and (
                            it['rarity'] or '/Artifacts/' in path or '/Medals/' in path):
                        yield path
    missions = [s for s in srcs if s['kind'] == 'mission' and not CUT.search('/' + s['id'])]
    droppers = collections.defaultdict(set)
    for s in srcs:  # every named enemy and boss fight reward counts: an item mobs share is no boss's own
        if s['event'] == 'OnKilled' and s['name'] and not CUT.search(s['id']) and game.paths[s['id']] not in templates:
            # bosses that drop a row through a dropper keyword are one group (the four Edda of Ice and Fire demons)
            group = s['table'].rsplit('@', 1)[1] if '@' in s['table'] else ''
            for path in own_gear(s):
                droppers[path].add(f'@{group}' if group and dropper_only(s, path) else s['name'])
    for s in missions:
        for path in own_gear(s):
            droppers[path].add(s['id'])
    chase = {p for p, names in droppers.items() if len(names) <= CHASE_BOSSES}
    names = {s['name'] for s in candidates if any(p in chase for p in own_gear(s))}
    return ([s for s in candidates if s['name'] in names] + [s for s in missions if any(p in chase for p in own_gear(s))],
            chase)


def dropper_only(s, path):
    """Whether a resolved source's item came from a row that needed its dropper keyword: not in the plain table."""
    plain = _PLAIN_TABLES.get(s['table'])
    return plain is not None and not any(k[0] == path and st.p > 0 for tier in plain.values() for k, st in tier.items())


_PLAIN_TABLES = {}  # resolved table key -> the table's rows without any dropper-keyword row (resolve_droppers)


def fight_sources(s):
    """A boss fight reward as one source per boss in it, named as players know them (Skrull Captain America)."""
    names = [n if 'Skrull' in n or 'Skrull' not in e.split('/')[-1] else 'Skrull ' + n for e, n in s['bosses']]
    out = []
    for (entity, _), name in zip(s['bosses'], names):
        others = [n for n in names if n != name]
        where = s['name'] + ' reward' + (', shared with ' + ' and '.join(others) if others else '')
        # no '#' in an id: it goes into the page address
        out.append({'id': s['id'] + '~' + entity.split('/')[-1].rsplit('.', 1)[0], 'kind': 'boss', 'name': name,
                    'where': where, 'table': s['table'], 'fight': True, 'entity': entity})
    return out


SKRULLS = {"Kl'rt"}  # Skrulls under their own name: no hero name to tell apart


COSMIC_ONLY, NOT_COSMIC = 'only in: CosmicGameMode', 'not in: CosmicGameMode'


def mode_versions(rows):
    """{version: rows}: the Cosmic patrols and modes are their own maps (region keyword CosmicGameMode), so a table
    with rows for one of them only splits into a normal ('') and a 'Cosmic' version, each without that label."""
    if not any(COSMIC_ONLY in k[2] or NOT_COSMIC in k[2] for k in rows):
        return {'': rows}
    out = {}
    for version, drop, strip in (('', COSMIC_ONLY, NOT_COSMIC), ('Cosmic', NOT_COSMIC, COSMIC_ONLY)):
        got = {}
        for (path, pool, row_labels), r in rows.items():
            if drop in row_labels:
                continue
            k = (path, pool, tuple(l for l in row_labels if l != strip))
            if k in got:  # an unlabelled roll and this one meet: combine them as separate rolls
                for f in ('p', 'base'):
                    got[k][f] = [round(1 - (1 - a) * (1 - b), 9) for a, b in zip(got[k][f], r[f])]
                got[k]['n'] = [round(a + b, 9) for a, b in zip(got[k]['n'], r['n'])]
            else:
                got[k] = dict(r)
        out[version] = got
    return out


SHOP_KINDS = ('vendor', 'recipe', 'achievement', 'store')


def shop_source(s):
    """The app's record of a vendor tab, recipe, achievement or store entry."""
    o = {'id': s['id'], 'kind': s['kind'], 'name': s['name'], 'where': ''}
    for k in ('rank', 'crafter', 'inputs', 'tabs'):
        if s.get(k):
            o[k] = s[k]
    if s.get('during'):
        o['event'] = s['during']
    if s['kind'] == 'vendor':
        o['where'] = f"rank {s['rank']}" + (f", during {s['during']}" if s.get('during') else '')
    return o


def write_app(game, srcs, base, patched, mhserver_dir):
    """internal/db/drops.json.gz: bosses and boxes with display data, without event-only rows; rows MHServerEmu's
    patches add (s=1) or change (s=2) are tagged."""
    bosses, chase = real_bosses(game, srcs, patched)
    keep = [s for s in srcs if s['kind'] in SHOP_KINDS + ('box',)] + bosses
    items, item_index, labels, label_index, pools, pool_index, tables, table_index = [], {}, [], {}, [], {}, [], {}

    def index(store, idx, key, make):
        if key not in idx:
            idx[key] = len(store)
            store.append(make())
        return idx[key]

    live = live_keywords(game)
    versions = {}  # table key -> its mode versions ('' and maybe 'Cosmic')
    for s in keep:
        key = s['table']
        if key in versions:
            continue
        rows = {}
        for tier_i, tier in enumerate(TIERS):
            for k, st in patched[key][tier].items():
                labels_now = live_labels(k[2], live)
                # event rows go from boss pages; a vendor's event stock stays, labelled with its event
                if st.p > 0 and k[0] not in NOT_LOOT and (s['kind'] in SHOP_KINDS or not is_event(k[2])) and labels_now is not None:
                    r = rows.setdefault((k[0], k[1], labels_now), {'p': [0.0] * 3, 'n': [0.0] * 3, 'base': [0.0] * 3})
                    # two rolls can meet under one key once a dead keyword is dropped: combine them
                    r['p'][tier_i] = round(1 - (1 - r['p'][tier_i]) * (1 - st.p), 9)
                    r['n'][tier_i] = round(r['n'][tier_i] + st.n, 9)
            for k, st in base[key][tier].items():
                labels_now = live_labels(k[2], live)
                k2 = (k[0], k[1], labels_now)
                if labels_now is not None and k2 in rows:
                    rows[k2]['base'][tier_i] = round(1 - (1 - rows[k2]['base'][tier_i]) * (1 - st.p), 9)
        versions[key] = []
        for version, vrows in (mode_versions(rows) if s['kind'] not in SHOP_KINDS + ('box',) else {'': rows}).items():
            out = []
            for (path, pool, row_labels), r in sorted(vrows.items()):
                server = 0 if r['base'] == r['p'] else 1 if not any(r['base']) else 2
                out.append({'i': index(items, item_index, path, lambda: with_card(game, dict(item_info(game, path), chase=path in chase))),
                            'pool': index(pools, pool_index, pool, lambda: {'path': pool, 'name': POOL_NAMES.get(
                                pool.split('/')[-1], pool.split('/')[-1].rsplit('.', 1)[0])}) if pool else -1,
                            'l': [index(labels, label_index, l, lambda: pretty_label(l)) for l in row_labels],
                            'p': r['p'], 'n': r['n'], 's': server})
            versions[key].append(version)
            table_index[key, version] = len(tables)
            tables.append({'rows': out})
    label_same_names(items)
    used = {}  # place ids in use
    sources_out = []
    for s in keep:
        for mode in versions[s['table']]:
            table = table_index[s['table'], mode]
            if not tables[table]['rows']:
                continue  # a version with only event drops goes
            for o in fight_sources(s) if s['kind'] == 'mission' else [shop_source(s)] if s['kind'] in SHOP_KINDS else [
                    {'id': s['id'], 'kind': 'box' if s['kind'] == 'box' else 'boss', 'name': s['name'],
                     'where': '' if s['kind'] == 'box' else where_hint(s['id'])}]:
                entity = o.pop('entity', o['id'])
                found = place_of(entity) if o['kind'] == 'boss' else None
                if entity in game.paths:  # the boss's portrait, or the box's own icon
                    o['icon'] = '|'.join(icon_chain(game, game.merged(game.paths[entity])))
                o.update(table=table, place=-1, version=mode)
                stem = entity.split('/')[-1]
                if o['kind'] == 'boss' and 'Skrull' in stem and 'Skrull' not in o['name'] and o['name'] not in SKRULLS:
                    o['name'] = 'Skrull ' + o['name']  # an impostor carries the hero's name in game (as fight rewards do)
                if mode:
                    o['id'] += '~' + mode.lower()
                    o['where'] += ' (Cosmic)'
                if found:
                    pid, version = found
                    used[pid] = None
                    version = ', '.join(v for v in (version, mode) if v)
                    o['place'], o['version'] = pid, version
                    if not o.get('fight'):  # a fight's hint already says whose reward it is
                        o['where'] = PLACES[pid][0] + (f' ({version})' if version else '')  # as Loot info writes it
                sources_out.append(o)
    limit_patrol_tiers(game, sources_out, tables)
    sources_out, tables = fold_vendor_ranks(items, sources_out, tables)
    for o in sources_out:  # what a vendor asks for each named item it sells
        if o['kind'] == 'vendor':
            for row in tables[o['table']]['rows']:
                it = items[row['i']]
                if not it['random'] and 'price' not in it:
                    price = item_price(game, it['path'])
                    if price:
                        it['price'] = price
    order = sorted(used, key=lambda p: (GROUPS.index(PLACES[p][1]), int(p[6:]) if p.startswith('story-') else 0,
                                        PLACES[p][0]))  # Chapter 10 after Chapter 9
    index_of = {p: i for i, p in enumerate(order)}
    for o in sources_out:
        if o['place'] != -1:
            o['place'] = index_of[o['place']]
    places_out = [{'id': p, 'name': PLACES[p][0], 'group': PLACES[p][1]} for p in order]
    ach_cats, achievements = write_achievements(game, mhserver_dir, items, sources_out, tables, patched,
                                                {s['id']: s['table'] for s in srcs if s['kind'] == 'achievement'})
    with gzip.open(APP_OUT, 'wt', compresslevel=9) as f:
        json.dump({'version': 1, 'tiers': list(TIERS), 'labels': labels, 'items': items, 'pools': pools,
                   'tables': tables, 'places': places_out, 'sources': sources_out,
                   'achCategories': ach_cats, 'achievements': achievements}, f, separators=(',', ':'))
    print(f'{len(sources_out)} sources, {len(tables)} tables, {len(items)} items -> {APP_OUT}')
    cards = [it['card'] for it in items if 'card' in it]
    left = sum(c['hidden'] for c in cards)
    shown = sum(len(sec['lines']) for c in cards for sec in c['sections'] if sec['kind'] == 'lines')
    print(f'{len(cards)} item cards; stat lines shown {shown}, left off {left} (proc damage, healing and hero-level amounts, stat and description lines)')


def main(game_dir, mhserver_dir):
    game = GameData(game_dir)
    baseline, events = livetuning.load(mhserver_dir)
    env = Env(baseline=baseline, events=events, pools=frozenset(find_pools(game)), rarities=rarities(game),
              groups=livetuning.load_groups(mhserver_dir))
    srcs = sources(game) + vendor_sources(game, mhserver_dir) + recipe_sources(game) + achievement_sources(game, mhserver_dir) \
        + store_sources(game, mhserver_dir)
    base = roll_all(game, env, srcs)
    # MHServerEmu's patches apply on top of the game data; rows they add or change get a server tag
    for e in patches.load(mhserver_dir):
        if e['Prototype'] in game.paths:
            game.patches.setdefault(game.paths[e['Prototype']], []).append(e)
    game._merged.clear()
    reset_caches()
    for s in srcs:  # boxes and missions keep their contents struct: take it again from the patched data
        if s['kind'] == 'mission' and s['table'].endswith('#rewards'):
            s['_node'] = rewards_node(game, game.paths[s['id']])
        if s['kind'] == 'recipe':
            s['_node'] = game.merged_struct(game.merged(game.paths[s['id']])['RecipeOutput'])
        if s['kind'] == 'box':
            actions = game.merged(game.paths[s['id']]).get('ActionsTriggeredOnItemEvent') or {}
            s['_node'] = next((game.merged_struct(a['LootTable']) for a in actions.get('Choices') or []
                               if isinstance(a, dict) and isinstance(a.get('LootTable'), dict)), s['_node'])
    patched = roll_all(game, env, srcs)
    write_debug(game, srcs, patched)
    resolve_droppers(game, srcs, base, patched)  # after the debug file: the server checker rolls the plain tables
    resolve_terminal_regions(game, srcs, base, patched)
    write_app(game, srcs, base, patched, mhserver_dir)


if __name__ == '__main__':
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
