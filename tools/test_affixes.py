import os

import pytest

GAME = os.environ.get('MHO_GAME', '')
RING = 'Entity/Items/Rings/BloodstoneRing.prototype'


@pytest.fixture(scope='module')
def game():
    if not os.path.isdir(GAME):
        pytest.skip('game folder missing')
    from calligraphy import GameData
    return GameData(GAME)


def names(game, pids):
    return sorted(game.prototypes[p][1].replace('Entity/Items/Affixes/RingAffixes/', '').rsplit('.', 1)[0] for p in pids)


def test_bloodstone_demonband_rolls(game):
    # 200,000 Mystic Mayhem Coffers opened with MHServerEmu's own loot code (the private checker, 2026-10-24): every
    # Bloodstone Demonband was Unique at item level 63 with 2 suffixes and 2 uniques, from exactly these affixes
    from affixes import rolls
    r = rolls(game, game.paths[RING], 'R6Unique', 63, 'MysteryChest')
    assert {pos: (lo, hi) for pos, (lo, hi, _) in r['positions'].items()} == {'Suffix': (2, 2), 'Unique': (2, 2)}
    assert names(game, r['positions']['Suffix'][2]) == [
        'BloodstoneAffixes/BloodstoneDefense', 'BloodstoneAffixes/BloodstoneDurability', 'BloodstoneAffixes/BloodstoneHealth',
        'BloodstoneAffixes/BloodstoneMovespeed', 'BloodstoneAffixes/BloodstoneSpeed', 'BloodstoneAffixes/BloodstoneSummonDuration',
        'BloodstoneAffixes/BloodstoneSummonHealth', 'RingLoot20/BrutalDamage/BrutalDamT1', 'RingLoot20/BrutalDamage/BrutalDamT2',
        'RingLoot20/BrutalDamage/BrutalDamT3', 'RingLoot20/BrutalStrike/BrutalStrikeT1', 'RingLoot20/BrutalStrike/BrutalStrikeT2',
        'RingLoot20/BrutalStrike/BrutalStrikeT3']
    assert names(game, r['positions']['Unique'][2]) == [
        'BloodstoneAffixes/BloodstoneBrutalDamage', 'BloodstoneAffixes/BloodstoneBrutalRating', 'BloodstoneAffixes/BloodstoneEnergy',
        'BloodstoneAffixes/BloodstoneFighting', 'BloodstoneAffixes/BloodstoneIntelligence', 'BloodstoneAffixes/BloodstoneStrength',
        'BloodstoneAffixes/BloodstoneSummonDamage']
    assert r['unknown'] == 0
