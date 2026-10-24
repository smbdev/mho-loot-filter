import pytest
from loot import Ctx, Env, Stat, roll, chances


def item(path, num=(1, 1), weight=100, **kw):
    return {'kind': 'item', 'item': path, 'abstract': False, 'num': num, 'weight': weight, 'live': True, 'restrict': [], 'mods': [], **kw}


def table(choices, pick='all', num=(1, 1), nodrop=0.0, weight=100, path='T', mods=(), live=True):
    return {'kind': 'table', 'path': path, 'pick': pick, 'num': num, 'nodrop': nodrop, 'weight': weight, 'live': live,
            'mods': list(mods), 'choices': choices}


def p(result, path):
    return sum(s.p for (i, _, _), s in result.items() if i == path)


G = Ctx('Green')


def test_nodrop_scales_the_chance():
    r = roll(table([item('A')], nodrop=0.985), G, Env())
    assert p(r, 'A') == pytest.approx(0.015)


def test_pick_weight_splits_by_weight():
    r = roll(table([item('A', weight=99), item('B', weight=1)], pick='weight'), G, Env())
    assert p(r, 'A') == pytest.approx(0.99) and p(r, 'B') == pytest.approx(0.01)


def test_same_item_in_two_independent_branches():
    t = table([table([item('A')], nodrop=0.5, path='X'), table([item('A')], nodrop=0.5, path='Y')])
    r = roll(t, G, Env())
    assert p(r, 'A') == pytest.approx(0.75)
    assert sum(s.n for s in r.values()) == pytest.approx(1.0)


def test_repeated_picks_combine_as_independent_rolls():
    r = roll(table([item('A', weight=1), item('B', weight=3)], pick='weight', num=(2, 2)), G, Env())
    assert p(r, 'A') == pytest.approx(1 - 0.75 ** 2)


def test_num_range_is_averaged():
    r = roll(table([table([item('A')], nodrop=0.5, path='In')], num=(1, 3)), G, Env())
    assert p(r, 'A') == pytest.approx((0.5 + 0.75 + 0.875) / 3)


def test_nodrop_is_checked_once_before_the_picks():
    r = roll(table([item('A')], nodrop=0.5, num=(1, 3)), G, Env())
    assert p(r, 'A') == pytest.approx(0.5)


def test_item_count_range():
    r = roll(item('A', num=(0, 2)), G, Env())
    assert r[('A', '', ())] == Stat(pytest.approx(2 / 3), pytest.approx(1.0))


def test_tryall_skips_choices_that_fail():
    red_only = {'kind': 'RequireDifficultyTier', 'tiers': ['Red']}
    t = table([item('A'), item('B'), table([item('C')], mods=[red_only], path='R')], pick='tryall')
    r = roll(t, G, Env())
    assert p(r, 'A') == pytest.approx(0.5) and p(r, 'C') == 0


def test_zero_weight_choice_never_picked():
    r = roll(table([item('A', weight=0), item('B')], pick='weight'), G, Env())
    assert p(r, 'A') == 0 and p(r, 'B') == pytest.approx(1)


def test_chances_rolls_all_three_tiers():
    assert set(chances(table([item('A')]), Env())) == {'Green', 'Red', 'Cosmic'}


def test_difficulty_curve_multiplies_drop_chance():
    t = table([item('A')], nodrop=0.5, mods=[{'kind': 'ModifyDropByDifficultyTier', 'curve': [1.0, 0.5, 0.25]}])
    c = chances(t, Env())
    assert p(c['Green'], 'A') == pytest.approx(0.5) and p(c['Cosmic'], 'A') == pytest.approx(0.125)


def test_require_level_uses_hero_level():
    t = table([item('A')], mods=[{'kind': 'RequireLevel', 'min': 60, 'max': 0}])
    assert p(roll(t, G, Env()), 'A') == 1
    assert roll(t, Ctx('Green', hero_level=59), Env()) is None


def test_item_level_restriction_blocks_low_level_medal():
    medal = item('Medal', restrict=[(1, 25)])
    medallion = item('Medallion', restrict=[(27, 100)])
    for tier, r in chances(table([medal, medallion]), Env()).items():
        assert p(r, 'Medal') == 0 and p(r, 'Medallion') == 1, tier


def test_level_verbatim_and_offset():
    low = item('Low', restrict=[(1, 61)])  # levels 1-62
    assert p(roll(table([low]), G, Env()), 'Low') == 0  # 60 + 3 = 63
    verbatim = table([low], mods=[{'kind': 'UseLevelVerbatim', 'on': True}])
    assert p(roll(verbatim, G, Env()), 'Low') == 1
    offset = table([low], mods=[{'kind': 'UseLevelVerbatim', 'on': True}, {'kind': 'OffsetLevel', 'offset': 3}])
    assert p(roll(offset, G, Env()), 'Low') == 0


def test_condition_keyword_becomes_an_event_label():
    t = table([item('Cake')], mods=[{'kind': 'RequireConditionKeyword', 'names': ['Anniversary']}])
    [(key, s)] = roll(t, G, Env()).items()
    assert key == ('Cake', '', ('event: Anniversary',)) and s.p == 1


def test_once_daily_is_labelled():
    t = table([item('A')], mods=[{'kind': 'OnceDaily'}])
    assert list(roll(t, G, Env()))[0][2] == ('first kill each day',)


def test_switched_off_table_without_event_gives_nothing():
    off = table([item('A')], path='Off', live=False)
    assert roll(table([off]), G, Env()) == {}


def test_event_switch_labels_the_drop():
    off = table([item('Chest')], path='Off', live=False)
    r = roll(table([off]), G, Env(events={'Off': ["Odin's Bounty"]}))
    assert list(r) == [('Chest', '', ("event: Odin's Bounty",))]


def test_baseline_zero_switches_a_table_off():
    on = table([item('A')], path='On')
    assert roll(table([on]), G, Env(baseline={'On': 0.0})) == {}


def test_pool_keeps_its_name_on_the_rows():
    pool = table([item('Chair'), item('Helm')], pick='weight', nodrop=0.997, path='Pool')
    r = roll(table([pool]), G, Env(pools=frozenset({'Pool'})))
    assert {k[1] for k in r} == {'Pool'}


RARITIES = {'Common': ([19000.0], 1, None), 'Uncommon': ([15000.0], 1, 'Common'), 'Rare': ([1710.0], 1, 'Uncommon'),
            'Cosmic': ([25.0], 1, 'Rare')}


def test_random_item_fails_when_rarity_cannot_step_down_to_an_allowed_one():
    insignia = item('Insignia.defaults', abstract=True, allowed=['Uncommon', 'Rare'])
    r = roll(table([insignia]), G, Env(rarities=RARITIES))
    assert p(r, 'Insignia.defaults') == pytest.approx((15000 + 1710 + 25) / (19000 + 15000 + 1710 + 25))


def test_set_rarity_limits_the_rarity_roll():
    insignia = item('Insignia.defaults', abstract=True, allowed=['Uncommon', 'Rare'], mods=[{'kind': 'SetRarity', 'rarities': ['Cosmic']}])
    assert p(roll(insignia, G, Env(rarities=RARITIES)), 'Insignia.defaults') == pytest.approx(1.0)


def test_random_slot_item_uses_its_slot_rarities():
    slot = {'kind': 'other', 'other': 'ItemFilter:Insignia', 'weight': 100, 'mods': [], 'allowed': ['Rare']}
    assert p(roll(slot, G, Env(rarities=RARITIES)), 'ItemFilter:Insignia') == pytest.approx((1710 + 25) / 35735)


def test_item_not_approved_for_the_game_never_drops():
    t = table([item('Unshipped', approved=False), item('A')], pick='tryall')
    r = roll(t, G, Env())
    assert p(r, 'Unshipped') == 0 and p(r, 'A') == 1


HALF = {'C': ([1.0], 1, None), 'U': ([1.0], 1, 'C')}  # a random item allowed only as U succeeds half the time


def test_tryall_moves_on_when_a_choice_fails_sometimes():
    # Random order: A first (1/2) gives A half the time and B otherwise; B first gives B. So A 1/4, B 3/4.
    t = table([item('A', abstract=True, allowed=['U']), item('B')], pick='tryall')
    r = roll(t, G, Env(rarities=HALF))
    assert p(r, 'A') == pytest.approx(0.25) and p(r, 'B') == pytest.approx(0.75)


def test_tryall_sees_a_subtable_that_fails_sometimes():
    sub = table([item('A', abstract=True, allowed=['U'])], pick='weight', path='Sub')
    r = roll(table([sub, item('B')], pick='tryall'), G, Env(rarities=HALF))
    assert p(r, 'A') == pytest.approx(0.25) and p(r, 'B') == pytest.approx(0.75)


def test_nodrop_counts_as_success_for_tryall():
    # A table that rolls nothing (NoDrop) still ends the pick, so B is never tried after it.
    empty = table([item('A')], nodrop=1.0, path='Empty')
    r = roll(table([empty, item('B')], pick='tryall'), G, Env())
    assert p(r, 'B') == pytest.approx(0.5)


def test_set_rarity_is_part_of_a_random_item_row():
    a = item('Armor.defaults', abstract=True, allowed=['Cosmic', 'Rare'])
    r = roll(table([a, table([dict(a)], mods=[{'kind': 'SetRarity', 'rarities': ['Cosmic']}], path='U')]), G, Env(rarities=RARITIES))
    assert {k[2] for k in r} == {(), ('rarity: Cosmic',)}


def test_named_item_needs_a_rarity_it_allows():
    rune = item('Rune', allowed=['Runewords'], mods=[{'kind': 'SetRarity', 'rarities': ['Rare']}])
    assert roll(table([rune]), G, Env(rarities={**RARITIES, 'Runewords': ([6.0], 1, 'Cosmic')})) == {}
    ok = item('Rune', allowed=['Runewords'], mods=[{'kind': 'SetRarity', 'rarities': ['Runewords']}])
    assert p(roll(table([ok]), G, Env(rarities={**RARITIES, 'Runewords': ([6.0], 1, 'Cosmic')})), 'Rune') == 1


def test_random_item_fails_when_its_rarity_filter_has_no_weight():
    a = item('Armor.defaults', abstract=True, allowed=None, mods=[{'kind': 'SetRarity', 'rarities': ['Ultimate']}])
    assert roll(table([a]), G, Env(rarities={**RARITIES, 'Ultimate': ([0.0], 1, 'Rare')})) == {}


def test_event_loot_group_adds_its_items_to_the_table():
    t = table([item('A')], path='Boss')
    r = roll(t, G, Env(groups={'Boss': [("Saint Patrick's Day", 'Shamrock', None), ("Saint Patrick's Day", 'Hat', 0.75)]}))
    assert r[('Shamrock', '', ("event: Saint Patrick's Day",))].p == pytest.approx(1.0)
    assert r[('Hat', '', ("event: Saint Patrick's Day",))].p == pytest.approx(0.25)
    assert p(r, 'A') == 1


def test_contradicting_conditions_never_drop():
    inner = table([item('Chip')], mods=[{'kind': 'ForbidRegionKeyword', 'names': ['XDefense']}], path='In')
    t = table([inner, item('A')], mods=[{'kind': 'RequireRegionKeyword', 'names': ['XDefense']}])
    r = roll(t, G, Env())
    assert p(r, 'Chip') == 0 and p(r, 'A') == 1


def test_keyword_condition_with_no_keywords_does_nothing():
    # LootRollRequire*KeywordPrototype.Apply returns at once when Choices is empty.
    t = table([item('Tag')], mods=[{'kind': 'RequireDropperKeyword', 'names': []}])
    assert list(roll(t, G, Env())) == [('Tag', '', ())]



def test_random_item_rarities_count_only_members_that_drop_at_the_roll_level():
    groups = [{'levels': [(1, 25)], 'rarities': None}, {'levels': [(40, 99)], 'rarities': ['Uncommon', 'Rare']}]
    a = item('Artifact.defaults', abstract=True, allowed=groups)
    assert p(roll(table([a]), G, Env(rarities=RARITIES)), 'Artifact.defaults') == pytest.approx((15000 + 1710 + 25) / 35735)
    assert p(roll(table([a]), Ctx('Green', level=10), Env(rarities=RARITIES)), 'Artifact.defaults') == pytest.approx(1.0)
