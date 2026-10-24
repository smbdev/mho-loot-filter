import os
import pytest

from tooltip import fmt, fill

GAME = os.environ.get('MHO_GAME', '')
FLAG = 'Entity/Items/Artifacts/Prototypes/SpecialArtifacts/Art326.prototype'
RIFLE = 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique485.prototype'
CROSSBONES = 'Entity/Items/Artifacts/Prototypes/SpecialArtifacts/CosmicArtifacts/Art338Cosmic.prototype'


def test_fmt_ranges_and_signs():
    assert fmt((515, 800), []) == '515-800'
    assert fmt((3, 3), []) == '3'
    assert fmt((1483.7, 1483.7), ['nofraction']) == '1,483'
    assert fmt((0.06, 0.06), ['pct']) == '6.0'
    assert fmt((0.07, 0.1), ['pct', 'nofraction']) == '7-10'
    assert fmt((-0.1, -0.1), ['pct', 'nofraction', 'abs']) == '10'
    assert fmt((0.123, 0.123), ['pct', 'precision2']) == '12.30'


def test_fill_property_text():
    look = lambda base, mods: (200, 300) if base == 'propertyvalue' else None
    assert fill('$propertyvaluesign$$propertyvalue.nofraction$ Damage Rating', look) == '+200-300 Damage Rating'
    neg = lambda base, mods: (-0.1, -0.1) if base == 'propertyvalue' else None
    assert fill('$propertyvaluesign$$propertyvalue.pct.nofraction$% Cost', neg) == '-10% Cost'


def test_fill_unknown_token_gives_none():
    assert fill('$somethingodd$ Damage', lambda base, mods: None) is None


@pytest.fixture(scope='module')
def game():
    if not os.path.isdir(GAME):
        pytest.skip('game folder missing')
    from calligraphy import GameData
    return GameData(GAME)


def texts(game, path):
    from tooltip import stat_lines
    lines, hidden = stat_lines(game, game.paths[path])
    return [l['text'] for l in lines], hidden


def test_flag_stats(game):
    got, _ = texts(game, FLAG)
    for want in ('+1-3 Fighting', '+515-800 Critical Damage Rating', '+515-800 Brutal Damage Rating', '+200-300 Damage Rating'):
        assert want in got, got


def test_rifle_base_stat_tops_at_itembase_value(game):
    from tooltip import stat_lines
    lines, _ = stat_lines(game, game.paths[RIFLE])
    base = [l for l in lines if l['base']]
    assert len(base) == 1 and base[0]['text'].endswith('-883 Damage Rating'), base
    assert any(l['text'].startswith('+') and 'Base Damage vs. Bosses' in l['text'] and '6.0%' in l['text'] for l in lines), lines


def test_flag_proc_line(game):
    got, hidden = texts(game, FLAG)
    assert any('chance when you Brutally Strike to gain +' in t and '% Attack Speed for 10s' in t for t in got), (got, hidden)


def test_crossbones_proc_line(game):
    got, hidden = texts(game, CROSSBONES)
    assert any(t.startswith('When you land a Critical Hit gain +') and 'Brutal Damage Rating for 5s' in t for t in got), (got, hidden)


def test_rifle_proc_cooldown(game):
    got, hidden = texts(game, RIFLE)
    assert any(t.endswith('(10s cooldown)') for t in got), (got, hidden)


def test_own_property_token_resource_name_and_minutes(game):
    got, _ = texts(game, 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot2/Unique444.prototype')
    assert 'You take 2.0-5.0% less damage from enemy Area Powers' in got, got
    assert 'When defeated, you instead revive with full Health and Spirit (10m cooldown)' in got, got


def test_resistance_type_names(game):
    got, _ = texts(game, 'Entity/Items/Medals/MedalBlueprints/Endgame/CosmicMedallions/CosmicBlobAndToadEGM.prototype')
    assert '+1.0-2.0% Total Defense' in got, got
    got, _ = texts(game, 'Entity/Items/Medals/MedalBlueprints/Endgame/CosmicMedallions/CosmicHulkMedallion.prototype')
    assert any(t.endswith(' Physical Defense') for t in got), got


def text_of_segments(segs):
    return ''.join(s[0] for s in segs)


def test_cosmic_artifact_card(game):
    from tooltip import card
    c = card(game, game.paths[CROSSBONES])
    assert c['title'] == "Crossbones's Cosmically Enhanced Teaching Credentials" and c['color'] == 'D8E415'
    assert c['category'] == 'Cosmically Enhanced Artifact'
    kinds = [s['kind'] for s in c['sections']]
    assert kinds == ['text', 'lines', 'options', 'text'], kinds
    desc = c['sections'][0]['text']
    assert text_of_segments(desc) == "One Crossbones's Teaching Credentials may be equipped."
    assert ["Crossbones's Teaching Credentials", 'FF9900'] in desc
    cosmic = c['sections'][2]
    assert cosmic['title'] == 'Cosmic Item' and cosmic['color'] == 'FCDB1C' and len(cosmic['options']) == 9
    assert text_of_segments(c['sections'][3]['text']).startswith("Tasky's a fine teacher")
    assert c['grade'] == 63 and c['level'] == 60 and c['tradeable'] is False and c['hidden'] == 0


def test_unique_card_splits_base_and_bonus_lines(game):
    from tooltip import card
    c = card(game, game.paths[RIFLE], unique=True)
    assert c['color'] == 'B67B38' and c['category'] == 'Unique - Any Hero'
    blocks = [s for s in c['sections'] if s['kind'] == 'lines']
    assert blocks[0]['color'] == 'FFFFFF' and [text_of_segments(l) for l in blocks[0]['lines']] == ['+809-883 Damage Rating']
    assert blocks[1]['color'] == '28AA00' and text_of_segments(blocks[1]['lines'][0]) == '+343-442 Critical Hit Rating'


def test_items_without_stats_get_a_card_with_their_description(game):
    from tooltip import card
    c = card(game, game.paths['Entity/Items/Consumables/Prototypes/DailyGift/MysteriousUniqueBox.prototype'])
    assert c['title'] == 'Mysterious Blue Omega Box'
    texts = [text_of_segments(s['text']) for s in c['sections'] if s['kind'] == 'text']
    assert texts[0].startswith('The Mysterious Blue Omega Box contains a Unique') and "Batroc's Savate Boots" in texts[0], texts
    assert texts[1].startswith('Contains an item from the S.H.I.E.L.D. vault'), texts
    assert 'level' not in c and 'grade' not in c  # level and grade are worked out for items with stats only


def test_colour_tags_in_a_title_colour_the_title(game):
    from tooltip import card
    path = next(p for p in game.paths if p.endswith('.prototype') and '#orange#' in game.strings.get(game.merged(game.paths[p]).get('DisplayName') or 0, '')
                and game.merged(game.paths[p]).get('TooltipTemplate'))
    c = card(game, game.paths[path])
    assert '#' not in c['title'] and c['color'] == 'FF9900', (path, c['title'], c['color'])


def test_percent_sign_after_a_fraction_shows_it_as_a_percent(game):
    got, _ = texts(game, 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot3/Unique404.prototype')
    assert 'When you take Area Damage, gain 2% Area Damage reduction for 6s' in got, got


def test_curve_valued_condition_property_is_read_at_item_level(game):
    got, _ = texts(game, 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique299.prototype')
    bleed = [t for t in got if 'bleed for' in t]
    import re
    amounts = [float(n.replace(',', '')) for n in re.findall(r'\d[\d,]*', bleed[0].split('bleed for')[1])] if bleed else []
    assert amounts and max(amounts) < 1e7, bleed  # a damage number, not a curve id read as one


def test_no_double_spaces():
    assert fill('chance when you hit  to gain', lambda b, m: None) == 'chance when you hit to gain'


def test_card_lines_are_coloured_segments(game):
    from tooltip import card
    c = card(game, game.paths['Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique303.prototype'], unique=True)
    lines = [l for s in c['sections'] if s['kind'] == 'lines' for l in s['lines']]
    assert all(isinstance(l, list) for l in lines)
    assert not any('#' in seg[0] for l in lines for seg in l)
    assert any(seg == ['Bulwark of Muspelheim', 'FF0000'] for l in lines for seg in l)


def test_range_across_zero_is_left_off():
    assert fill('$propertyvaluesign$$propertyvalue$ Thing', lambda b, m: (-5, 3) if b == 'propertyvalue' else None) is None


def test_mulby_before_a_percent_is_not_scaled_twice():
    assert fill('$x.mulby100$%', lambda b, m: (0.02, 0.02)) == '2%'


def test_sign_with_the_propertys_own_name_token():
    look = lambda b, m: (-0.1, -0.1) if b == 'propertyvalue' else None
    assert fill('$propertyvaluesign$$DamagePctResistVsPowerKeyword.pct.nofraction$% Cost', look,
                own='damagepctresistvspowerkeyword') == '-10% Cost'


def test_options_sections_count_what_they_leave_off(game):
    from tooltip import card
    c = card(game, game.paths[CROSSBONES])
    opts = [s for s in c['sections'] if s['kind'] == 'options']
    assert opts and opts[0]['hidden'] == 0


def test_stat_that_always_rolls_zero_is_not_on_the_card(game):
    # S.H.I.E.L.D Graduation Ring: its 6-10% proc rolls as an integer, so every server gives 0 and it never fires
    got, hidden = texts(game, 'Entity/Items/Rings/SHIELDRing.prototype')
    assert not any('allies gain +2 Fighting' in t for t in got) and hidden == 0, got
    # Completely Ordinary Cow Device: a 2-5% bonus that rolls to 0, shown in game data only by a placeholder text
    got, _ = texts(game, 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique389.prototype')
    assert 'TooltipOverrideText' not in got, got


def test_integer_rolls_show_whole_numbers(game):
    from tooltip import stat_lines
    lines, _ = stat_lines(game, game.paths[RIFLE])
    assert '+764-1,376 Health' in [l['text'] for l in lines]


def test_placeholder_override_text_is_never_shown(game):
    got, hidden = texts(game, 'Entity/Items/Artifacts/Prototypes/Tier1Artifacts/Art355.prototype')
    assert 'TooltipOverrideText' not in got, got


def card_texts(game, path):
    from tooltip import card
    c = card(game, game.paths[path])
    return [text_of_segments(s['text']) for s in c['sections'] if s['kind'] == 'text'], c


def test_description_names_the_hero_and_the_item(game):
    texts, _ = card_texts(game, 'Entity/Items/Costumes/Prototypes/AntMan/HPClassic.prototype')
    assert any(t.startswith('A costume for Ant-Man ') for t in texts), texts
    texts, _ = card_texts(game, 'Entity/Items/Pets/Pet038SkrullCow.prototype')
    assert 'Use this item to call Incredibly Normal Shrunken Cow to your side.' in texts, texts
    texts, _ = card_texts(game, 'Entity/Items/CharacterTokens/Prototypes/TeamUps/MovieGroot.prototype')
    assert any('Groot (Movie)' in t for t in texts) and not any('$' in t for t in texts), texts


def test_description_fills_numbers_from_the_items_power(game):
    texts, c = card_texts(game, 'Entity/Items/Consumables/Prototypes/PotionOfHealth.prototype')
    assert not any('$' in t for t in texts) and c['hidden'] == 2, (texts, c['hidden'])  # heal amounts scale with the hero: left off
    texts, _ = card_texts(game, 'Entity/Items/Rings/StoneOfJordan.prototype')
    assert not any('$' in t for t in texts), texts


def test_hero_token_says_what_it_unlocks(game):
    from tooltip import card
    texts, c = card_texts(game, 'Entity/Items/CharacterTokens/Prototypes/Shipping/TokenAntMan.prototype')
    assert texts == ['Unlocks the hero Ant-Man.'], texts  # the game writes this line in code: ours names what it unlocks


GUNGNIR = 'Entity/Items/Legendaries/Prototypes/Legendary019A.prototype'


def card_lines(c):
    return [''.join(t for t, _ in l) for s in c['sections'] if s['kind'] == 'lines' for l in s['lines']]


def test_legendary_rank_lines(game):
    # the user's in-game Gungnir tooltip (2026-10-24); the [-1,000] / [+0] after each line is the player's own rank
    from tooltip import card
    assert card_lines(card(game, game.paths[GUNGNIR]))[:5] == [
        'Rank 1: +500 Critical Hit Rating',
        'Rank 2: +1,500 Critical Damage Rating',
        'Rank 3: +500 Critical Hit Rating',
        'Rank 4: +1,500 Critical Damage Rating',
        'Rank 5: 5% chance on hit to enter Odinrage, gaining +1,000 brutal strike rating for 5 seconds (20 second cooldown)']


def test_legendary_card_is_unranked(game):
    # an Unranked Legendary shows every rank locked, in the game's locked affix style (UIGlobals TextStyleItemAffixLocked,
    # red in the user's screenshot), and nothing counts as left off
    from tooltip import card, _style_colour
    locked = _style_colour(game, game.merged(game.paths['Globals/UIGlobals.defaults'])['TextStyleItemAffixLocked'])
    c = card(game, game.paths[GUNGNIR])
    ranks = [s for s in c['sections'] if s['kind'] == 'lines']
    assert locked == 'FF0000' and [s['color'] for s in ranks] == [locked]
    assert c['hidden'] == 0


def test_every_legendary_has_rank_lines(game):
    # the Legendaries the drop finder lists (a template like "New Legendary Base" has no ranks)
    import gzip, json
    from tooltip import card
    app = json.load(gzip.open(os.path.join(os.path.dirname(__file__), '..', 'internal', 'db', 'drops.json.gz')))
    legendaries = [game.paths[i['path']] for i in app['items'] if (i.get('card') or {}).get('category') == 'Legendary']
    assert len(legendaries) == 30
    missing = []
    for pid in legendaries:
        c = card(game, pid)
        if c and c['category'] == 'Legendary' and not any(l.startswith('Rank ') for l in card_lines(c)):
            missing.append(c['title'])
    assert not missing, missing


def test_legendary_item_grade(game):
    # the game shows Item Grade 70 on a Legendary and 80 on its upgraded version (EvalDisplayLevel); user, 2026-10-24
    from tooltip import card
    plain = card(game, game.paths['Entity/Items/Legendaries/Prototypes/Legendary003.prototype'])
    upgraded = card(game, game.paths[GUNGNIR])
    assert (plain['level'], plain['grade']) == (60, 70)
    assert (upgraded['level'], upgraded['grade']) == (60, 80)
