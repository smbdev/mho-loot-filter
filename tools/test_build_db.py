import json, os, re
DB = json.load(open(os.path.join(os.path.dirname(__file__), '..', 'internal', 'db', 'itemdb.json')))

def by_name(n):
    return [i for i in DB['items'] if i['name'] == n]

def test_relic_of_atlantis_has_its_own_type():
    [it] = by_name('Relic of Atlantis')
    t = DB['types'][it['type']]
    assert t['file'] == 'UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk'
    assert sum(1 for i in DB['items'] if i['type'] == it['type']) == 1
    assert t['model'] and t['name']

def test_gem_of_the_kursed_shares_crimson_crystal():
    [it] = by_name('Gem of the Kursed')
    assert DB['types'][it['type']]['file'] == 'UC__MarvelItem_Artifact_CrimsonCrystal_SF.upk'
    assert sum(1 for i in DB['items'] if i['type'] == it['type']) > 10

def test_uniques_have_own_glow_and_insignias_use_rarity():
    u = [t for k, t in DB['types'].items() if k == 'marvelitem_uniquebasecommon'][0]
    assert u['glow'] and not u['rarityGlow']
    ins = [t for k, t in DB['types'].items() if k.startswith('marvelitem_insignia')]
    assert ins and all(t['rarityGlow'] for t in ins)

def test_rarity_offsets_present_and_no_quest_or_currency():
    assert sorted(DB['rarities']) == ['Common', 'Cosmic', 'Epic', 'Rare', 'Uncommon', 'Unique']
    assert not any(k.startswith(('marvelitem_quest_', 'marvelitem_loot_money')) for k in DB['types'])
    assert len(DB['items']) > 3000 and len(DB['types']) > 500  # distinct (name, type) pairs; prototypes repeat per level


def items_named(n):
    return [i for i in DB['items'] if i['name'] == n]


def test_uru_axe_is_its_own_item_in_a_shared_type():
    [axe] = items_named('Uru-Forged Battle Axe')
    assert 'uru' in axe['groups'] and len(axe['protos']) >= 1
    assert sum(1 for i in DB['items'] if i['type'] == axe['type']) == 23


def test_medallions_group_holds_every_medallion():
    # A medallion's Cosmic version is the same item rolled at Cosmic rarity, so there is one group for all of them.
    medallions = [i for i in DB['items'] if 'medallions' in i['groups']]
    assert len(medallions) > 69  # normal medallions live in .defaults files
    for name in ('Lizard Medallion', 'Bullseye Medallion', 'Hulk Medallion', 'Cosmic Medallion of The Collector'):
        [m] = items_named(name)
        assert 'medallions' in m['groups'], name
    assert not any(g['id'] == 'cosmic-medallions' for g in DB['groups'])


def test_every_item_has_prototypes():
    assert all(i['protos'] and all(p['path'].startswith('Calligraphy/Entity/Items/') and p['field'] for p in i['protos']) for i in DB['items'])


def test_sinks_exist_and_are_unused():
    for key in ('shown', 'hidden'):
        sink = DB['sinks'][key]
        assert sink['type'] in DB['types'] and sink['asset'] > 0
        assert not any(i['type'] == sink['type'] for i in DB['items'])
    assert DB['calligraphySha1'] and DB['unrealClassFields']


def test_groups_are_unique_and_used():
    ids = [g['id'] for g in DB['groups']]
    assert len(ids) == len(set(ids))
    used = {g for i in DB['items'] for g in i['groups']}
    assert set(ids) <= used


def test_items_defined_in_defaults_files_are_included():
    [tomb] = items_named('Tombstone Medallion')
    paths = [p['path'] for p in tomb['protos']]
    assert any(p.endswith('/TombstoneMedalEG.defaults') for p in paths), paths
    assert any(p.endswith('/CosmicTombstoneEGM.prototype') for p in paths), paths
    assert not [i['name'] for i in DB['items'] if not re.search('[a-z]', i['name']) and re.search(r'BLUEPRINT|NAME|_', i['name'])]
    assert items_named('H.E.R.B.I.E.') and items_named('X-23') and items_named('BFG 9000')  # real names in capitals stay


def test_prototypes_list_the_items_that_inherit_their_class():
    [flag] = items_named('Flag of the Skrull Empire')
    inherited = [c for p in flag['protos'] for c in p.get('inheritors', [])]
    assert inherited and all(c['asset'] and c['field'] and c['path'].startswith('Calligraphy/') for c in inherited)
    own = {p['path'] for p in flag['protos']}
    assert not own & {c['path'] for c in inherited}


def test_catalysts_have_their_own_group():
    names = {i['name'] for i in DB['items'] if 'catalysts' in i['groups']}
    assert {'Genetic Mutation', 'Mystical Energies', 'Radioactive Isotope', 'Cosmic Spirit', 'Advanced Technological Systems'} <= names
    assert not any('crafting' in i['groups'] for i in DB['items'] if i['name'] in names)
    assert not [i['name'] for i in DB['items'] if i['name'] in ('NEEDSREDESIGN', 'RUNE')]


def test_every_item_is_in_at_most_one_group():
    # Group switches act on every item of a group, so overlapping groups would switch each other.
    shared = [i['name'] for i in DB['items'] if len(i['groups']) > 1]
    assert not shared, shared[:5]
    cosmic = [i for i in DB['items'] if 'Cosmically Enhanced' in i['name'] and not i['name'].endswith('Box')]
    assert cosmic and all(i['groups'] == ['cosmic-artifacts'] for i in cosmic)


def test_rings_have_their_own_group():
    rings = {i['name'] for i in DB['items'] if i['groups'] == ['rings']}
    assert {'Ring', 'Signet of Odin', 'Stone of Jordan'} <= rings
    assert not [i for i in DB['items'] if i['name'] == 'Rare Ring/Insignia Upgrade' and i['groups'] != ['recipes']]


def test_danger_room_scenarios_have_their_own_group():
    names = {i['name'] for i in DB['items'] if i['groups'] == ['dangerroom']}
    assert {'Danger Room Cosmic Scenario', 'Danger Room Common Scenario', 'Danger Room Scenario', 'Unique Challenge Scenario'} <= names
    assert not names & {'Cosmic Danger Room Scenario', 'Sentinel Scenario Medallion'}  # a recipe and a medallion


def detail(name, type_part=''):
    return [i.get('detail', '') for i in DB['items'] if i['name'] == name and type_part in i['type']]


def test_items_say_what_they_are():
    assert detail('Ring', 'defaultring') == ['Ring, drops at any rarity']
    assert detail('Ring', 'marvelitem_loot') == ['PvP ring']
    assert detail('Claws', 'blackcat') == ['Black Cat gear, drops at any rarity']
    assert detail('Adrenal Formula', 'respecpotion') == ['Crafting recipe']
    assert detail('Insignia of Thor') == ['Drops at any rarity']
    assert detail('Relic of Atlantis') == ['Drops at any rarity']
    assert not any(t['category'] == 'Legendaries' for t in DB['types'].values())


def test_prototypes_carry_their_click_bounds():
    pick = DB['picking']
    assert pick['boundsField'] and pick['flagField'] and pick['boundsBlueprint']
    [qs] = items_named('Insignia of Quicksilver')
    proto = qs['protos'][0]
    bounds = DB['bounds'][proto['bounds']]
    assert bounds['data'] and bounds['blueprint'] and not proto.get('boundsOwn')  # inherited from Insignia.defaults
    with_bounds = sum(1 for i in DB['items'] for p in i['protos'] if 'bounds' in p)
    assert with_bounds > 0.95 * sum(len(i['protos']) for i in DB['items'])
