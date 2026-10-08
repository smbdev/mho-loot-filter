import json, os
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
    assert len(medallions) == 69
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
