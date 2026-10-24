import json, os, re
import pytest

OUT = os.path.join(os.path.dirname(__file__), '..', 'dist', 'drops.json')
pytestmark = pytest.mark.skipif(not os.path.exists(OUT), reason='run build_drops.py first')
DOOM = 'Loot/Tables/Mob/Bosses/EndgameDailies/Terminals/CastleDoom/DrDoomTerminalLoot.prototype'
MEDALLION = 'Entity/Items/Medals/MedalBlueprints/Endgame/DoctorDoomEGM.defaults'
COSMIC_MEDALLION = 'Entity/Items/Medals/MedalBlueprints/Endgame/CosmicMedallions/CosmicDoctorDoomEGM.prototype'


@pytest.fixture(scope='module')
def db():
    return json.load(open(OUT))


def p(db, table, tier, item, pool='', labels=()):
    return sum(r[3] for r in db['tables'][table][tier] if r[0] == item and r[1] == pool and tuple(r[2]) == labels)


def test_doom_medallions(db):
    assert p(db, DOOM, 'Cosmic', MEDALLION) == pytest.approx(0.99)
    assert p(db, DOOM, 'Green', COSMIC_MEDALLION) == 0
    assert p(db, DOOM, 'Red', COSMIC_MEDALLION) == pytest.approx(0.01)
    assert p(db, DOOM, 'Cosmic', COSMIC_MEDALLION) == pytest.approx(0.01)


def test_low_level_medal_never_drops(db):
    medal = 'Entity/Items/Medals/MedalBlueprints/Boss/DoctorDoomMedalCH8.defaults'
    assert not any(r[0] == medal for t in db['tiers'] for r in db['tables'][DOOM][t])


def test_doom_chase_items(db):
    assert p(db, DOOM, 'Cosmic', 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot3/Unique341.prototype') == pytest.approx(0.02)
    assert p(db, DOOM, 'Cosmic', 'Entity/Items/Artifacts/Prototypes/SpecialArtifacts/CosmicArtifacts/Art153Cosmic.prototype') == pytest.approx(0.005)


def test_odin_chest_is_event_only(db):
    rows = [r for r in db['tables'][DOOM]['Cosmic'] if r[0].endswith('/OdinsBountyChest.prototype')]
    assert rows and all(any("Odin's Bounty" in l for l in r[2]) for r in rows)


def test_chair_comes_through_the_shared_pool(db):
    chair = 'Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot2/Unique218.prototype'
    rows = [r for r in db['tables'][DOOM]['Cosmic'] if r[0] == chair]
    assert rows and all(r[1].endswith('/EndGameBossSpecialDrops.prototype') for r in rows)


def test_cosmic_doom_entity_is_a_source(db):
    [s] = [s for s in db['sources'] if s['id'] == 'Entity/Characters/Bosses/PVEDailies/Cosmic/EGD09CDrDoomPhase3.prototype' and s['event'] == 'OnKilled']
    assert s['table'].endswith('/DrDoomLootCosmicMASTER.prototype') and s['name'] == 'Doctor Doom'


def test_cosmic_reliquary_contents(db):
    box = 'Entity/Items/Consumables/Prototypes/RandomGiftboxes/CosmicReliquaryOfRarity.prototype#contents'
    assert p(db, box, 'Cosmic', COSMIC_MEDALLION) == pytest.approx(1 / 6 / 55, rel=0.01)


def test_random_slot_items_are_named_by_slot(db):
    assert p(db, DOOM, 'Cosmic', 'ItemFilter:Artifact01') > 0


def test_only_item_lists_count_as_pools(db):
    assert 'Loot/Tables/Mob/Bosses/EndGameBossSpecialDrops.prototype' in db['pools']
    assert not any(p.endswith('/SharedEndgameDailiesCosmic.prototype') for p in db['pools'])


def test_random_insignia_counts_rarity_step_down(db):
    # Insignias drop as Uncommon to Epic; a Common roll (19000 of 36087 weight at level 63) finds none, while
    # SetRarity rolls always find one. The server's own roll gave 0.195 over 5,000 kills (2026-10-18).
    assert p(db, DOOM, 'Cosmic', 'Entity/Items/Insignias/Insignia.defaults') == pytest.approx(0.1988, abs=0.001)


def test_unshipped_wizard_artifact_never_drops(db):
    # Wizard's Id Machine is DesignState NotInGame, which ItemResolver.CheckItem refuses (ApprovedForUse).
    art = 'Entity/Items/Artifacts/Prototypes/SpecialArtifacts/Art360.prototype'
    assert not any(r[0] == art for t in db['tables'].values() for rows in t.values() for r in rows)


def test_boxes_roll_as_mystery_chests_at_their_own_level(db):
    [s] = [s for s in db['sources'] if s['id'] == 'Entity/Items/Consumables/Prototypes/RandomGiftboxes/CosmicReliquaryOfRarity.prototype']
    assert s['context'] == 'MysteryChest' and s['level'] == 60  # its action sets UseCurrentAvatarLevelForRoll
    assert any(b['kind'] == 'box' and b['level'] == 63 for b in db['sources'])  # most boxes roll at their own level


def test_crafting_only_ring_is_not_an_enemy_drop(db):
    # Midtown Guardian's Signet is UsableFor Crafting and MysteryChest, so a kill never drops it (ContextRestriction).
    ring = 'Entity/Items/Rings/Ring012.prototype'
    assert not any(r[0] == ring for t in db['tables']['Loot/Tables/Mob/Bosses/PatrolMidtown/VenomMidtown.prototype'].values() for r in t)


import gzip

APP = os.path.join(os.path.dirname(__file__), '..', 'internal', 'db', 'drops.json.gz')


@pytest.fixture(scope='module')
def app():
    return json.load(gzip.open(APP))


def rows_for(app, source_id):
    [s] = [s for s in app['sources'] if s['id'] == source_id]
    return [(app['items'][r['i']], r) for r in app['tables'][s['table']]['rows']]


COSMIC_DOOM = 'Entity/Characters/Bosses/PVEDailies/Cosmic/EGD09CDrDoomPhase3.prototype'


def test_app_file_holds_bosses_boxes_and_shops(app):
    assert {s['kind'] for s in app['sources']} == {'boss', 'box', 'vendor', 'recipe', 'achievement', 'store'}
    assert any(s['id'] == COSMIC_DOOM for s in app['sources'])
    assert os.path.getsize(APP) < 8_000_000


def test_cosmic_doom_rows(app):
    rows = {(i['path'].split('/')[-1], tuple(app['labels'][x] for x in r['l'])): r for i, r in rows_for(app, COSMIC_DOOM)}
    # the Cosmic terminal runs on Cosmic only (a terminal's colour is its difficulty)
    assert rows[('CosmicDoctorDoomEGM.prototype', ())]['p'] == pytest.approx([0.0, 0.0, 0.01], abs=1e-9)
    assert not any(k[0] in ('BannerMessage', 'ChatMessage', 'UsePower') for k in rows)


def test_same_name_items_get_labels(app):
    # The normal and the Cosmic Doctor Doom Medallion share a name; each must carry its own label.
    doom = [i for i in app['items'] if i['name'] == 'Doctor Doom Medallion']
    assert sorted(i['label'] for i in doom) == ['Cosmic', 'Normal']


def test_random_lines_have_readable_names(app):
    insignia = [i for i in app['items'] if i['path'] == 'Entity/Items/Insignias/Insignia.defaults']
    assert insignia and insignia[0]['name'] == 'A random insignia' and insignia[0]['random']


def test_patch_only_rows_are_tagged(app):
    # MHServerEmu adds fortune cards to SpecialEventsTable, which every endgame boss rolls; in the original game they
    # come only from boxes such as the Cosmic Mystery Box. So each boss row of one carries the server tag.
    seen = 0
    for s in app['sources']:
        for r in app['tables'][s['table']]['rows']:
            if s['kind'] == 'boss' and app['items'][r['i']]['path'].endswith('/FortuneCardMark3.prototype') and not r['l']:
                assert r['s'] in (1, 2)
                seen += 1
    assert seen


def test_where_hints_read_as_words(app):
    [s] = [s for s in app['sources'] if s['id'] == COSMIC_DOOM]
    assert s['where'] == 'Castle Doom (Cosmic terminal)'
    [green] = [s for s in app['sources'] if s['id'].endswith('/Green/EGD09GDrDoomPhase3.prototype')]
    assert green['where'] == 'Castle Doom (Green terminal)'  # the Green terminal map (it runs on any difficulty)
    assert all(not s['where'] for s in app['sources'] if s['kind'] == 'box')


def test_same_name_fallback_labels_keep_the_difference(app):
    rings = sorted(i['label'] for i in app['items'] if i['path'].endswith(('/RingDefenseLoot20.prototype', '/RingOffenseLoot20.prototype')))
    assert rings == ['Defense', 'Offense']
    assert not any(i['name'].startswith('A random random') for i in app['items'])


def test_random_lines_and_tokens_read_well(app):
    assert not any(i['random'] and i['label'] for i in app['items'])
    assert any(i['path'] == 'CharacterToken' and i['name'] == 'A random hero token' for i in app['items'])


def test_random_artifact_slot_follows_rank_and_rarity():
    # SpecialsArtifactsRespec picks a random rank-1 artifact 99 times in 110; at item level 63 no rank-1 artifact
    # drops as Common, so a Common roll finds nothing. The server measured 0.4309 over 20,000 rolls (2026-10-18).
    import build_drops
    from calligraphy import GameData
    from loot import Ctx, Env, roll
    import patches as P
    game, server = os.environ.get('MHO_GAME', ''), os.environ.get('MHSERVER', '')
    if not (os.path.isdir(game) and os.path.isdir(server)):
        pytest.skip('set MHO_GAME to the game folder and MHSERVER to an MHServerEmu checkout')
    g = GameData(game)
    for e in P.load(server):
        if e['Prototype'] in g.paths:
            g.patches.setdefault(g.paths[e['Prototype']], []).append(e)
    build_drops.reset_caches()
    n = build_drops.node(g, g.merged(g.paths['Loot/Tables/ItemType/SpecialsArtifactsRespec.prototype']))
    r = roll(n, Ctx('Cosmic'), Env(rarities=build_drops.rarities(g)))
    assert r[('ItemFilter:Artifact01', '', ())].p == pytest.approx(0.4309, abs=0.012)


def test_only_real_bosses_are_listed(app):
    names = {s['name'] for s in app['sources'] if s['kind'] == 'boss'}
    assert {'Doctor Doom', 'MODOK', 'Wizard', 'Mole Man', 'Kraven'} <= names
    # a mini-boss, Skrull copies of heroes with empty tables, a training dummy, a cut placeholder, a patrol mercenary
    assert not names & {'Agent Coulson', 'Captain America', 'Lvl 60 DPS Target Dummy', 'PLACEHOLDER: Cyclops', 'Ata-Boy',
                        'Punisher', 'MoltenMan'}
    assert len(names) < 100  # 84 on 2026-10-21, with the raid encounter bosses


def test_cut_and_template_versions_are_left_out(app):
    ids = [s['id'] for s in app['sources'] if s['kind'] == 'boss']
    assert not [i for i in ids if '/Redacted/' in i or '/Deprecated/' in i]
    # templates nothing spawns: Punisher and MoltenMan only copy Pyro's table
    assert not [i for i in ids if i.endswith(('/Bosses/Punisher.prototype', '/Bosses/MoltenMan.prototype',
                                              '/Bosses/Batroc.prototype', '/SpidermanCloneSuperiorBase.prototype'))]
    assert not any(i.endswith('/InfernalWarSkrullTeleportSpawn.prototype') for i in ids)  # a mini-boss version


def test_event_rows_are_left_out(app):
    # from boss and box tables; a vendor's event stock keeps its label
    rows = [r for s in app['sources'] if s['kind'] in ('boss', 'box') for r in app['tables'][s['table']]['rows']]
    assert not any(app['labels'][l].startswith('event: ') for r in rows for l in r['l'])
    assert not any(app['items'][r['i']]['path'].endswith('/OdinsBountyChest.prototype') for r in rows)


def test_chase_items_are_a_bosses_own(app):
    by_path = {i['path'].split('/')[-1]: i for i in app['items']}
    assert by_path['CosmicDoctorDoomEGM.prototype']['chase'] and by_path['Art153Cosmic.prototype']['chase']
    assert by_path['Unique341.prototype']['chase']  # Latverian Monarch Cloak, Doom's own unique
    runes_and_uru = [i for i in app['items'] if '/Runewords/' in i['path']]
    assert runes_and_uru and not any(i['chase'] for i in runes_and_uru)
    assert not by_path['RingDefenseLoot20.prototype']['chase']


def test_uniques_get_the_unique_colour(app):
    # Latverian Monarch Cloak has no text style of its own; it is unique because it only drops as R6Unique
    [cloak] = [i for i in app['items'] if i['path'].endswith('/Unique341.prototype')]
    assert cloak['rarity'] == 'unique'


def test_shared_items_are_not_chase_drops(app):
    # Ruby Topped Cane is Kingpin's own unique (others get it only through the shared pool); the patrol badge, the
    # X-Defense chip and Cargill's Rewrites drop from many bosses
    by_name = {}
    for i in app['items']:
        by_name.setdefault(i['name'], []).append(i['chase'])
    assert by_name['Ruby Topped Cane'] == [True]
    for name in ('Midtown Patrol Badge', "Danger's A.I. Chip", "Cargill's Cosmically Enhanced Rewrites"):
        assert not any(by_name[name]), name


def test_skrull_group_fights_are_bosses(app):
    # The Hightown Skrull impostors drop only XP orbs when killed; their uniques come from the event's reward,
    # rolled once when the group is beaten (HightownEventSkrullAvengers.Rewards, HightownEventSkrullXMen.Rewards)
    names = {s['name'] for s in app['sources'] if s['kind'] == 'boss'}
    assert {'Skrull Captain America', 'Skrull Ms Marvel', 'Skrull Thor', 'Skrull Cyclops', 'Skrull Psylocke',
            'Skrull X-23'} <= names
    [cap] = [s for s in app['sources'] if s['name'] == 'Skrull Captain America' and not s['version'].endswith('Cosmic')]
    assert 'Alienated Vengeance' in cap['where']
    rows = {app['items'][r['i']]['path'].split('/')[-1]: r for r in app['tables'][cap['table']]['rows']}
    assert app['items'][rows['Art326.prototype']['i']]['chase']  # Flag of the Skrull Empire
    [cyc] = [s for s in app['sources'] if s['name'] == 'Skrull Cyclops' and not s['version'].endswith('Cosmic')]
    assert any(app['items'][r['i']]['path'].endswith('/Unique475.prototype') for r in app['tables'][cyc['table']]['rows'])


def test_every_boss_has_a_name_and_a_clean_hint(app):
    bosses = [s for s in app['sources'] if s['kind'] == 'boss']
    assert all(s['name'] for s in bosses)
    assert not any(s['where'].endswith('shared with ') for s in bosses)


def test_fight_rewards_are_flagged_and_cosmic_mode_reads_well(app):
    [cap] = [s for s in app['sources'] if s['name'] == 'Skrull Captain America' and not s['version'].endswith('Cosmic')]
    assert cap['fight'] and not any(s.get('fight') for s in app['sources'] if s['id'] == COSMIC_DOOM)
    assert not any('CosmicGameMode' in l or 'Cosmic version' in l for l in app['labels'])  # split into versions


def test_cut_modes_are_left_out(app):
    ids = [s['id'] for s in app['sources'] if s['kind'] == 'boss']
    for folder in ('/PVEInstances/', '/PVEWaveBattle/', '/PatrolSavage/', '/PvEWaveGame/'):
        assert not [i for i in ids if folder in i], folder


def test_keyword_labels_read_as_words(app):
    raw = [l for l in app['labels'] if re.search(r'(only in|not in|only from): [A-Z][a-z]+[A-Z]', l)]
    assert not raw, raw
    assert 'X-Defense only' in app['labels']
    assert not any(re.search(r'\bTier4', l) for l in app['labels'])  # no live region carries Tier4 (PvPTier4 is a PvP map)


def test_one_label_per_meaning(app):
    shown = [l for l in app['labels']]
    assert len(shown) == len(set(shown)), [l for l in shown if shown.count(l) > 1]


def place(app, s):
    return app['places'][s['place']] if s['place'] >= 0 else None


def test_terminal_versions_get_their_terminal(app):
    [s] = [s for s in app['sources'] if s['id'] == COSMIC_DOOM]
    assert place(app, s) == {'id': 'terminal-castle-doom', 'name': 'Castle Doom', 'group': 'Terminals'}
    assert s['version'] == 'Cosmic terminal' and s['where'] == 'Castle Doom (Cosmic terminal)'


def test_modes_story_and_overrides(app):
    by_id = {s['id'].split('/')[-1]: s for s in app['sources'] if s['kind'] == 'boss'}
    assert place(app, by_id['MidtownEventDoctorDoom.prototype'])['name'] == 'Midtown Patrol'
    assert place(app, by_id['HoodCH8.prototype'])['name'] == 'Chapter 8'
    assert place(app, by_id['ElektraCH0410.prototype'])['name'] == 'Chapter 4'
    assert place(app, by_id['DrDoomPhase3.prototype'])['name'] == 'Chapter 8'  # an override
    assert by_id['DangerRoomRandomBossCosmicHood.prototype']['version'] == 'random boss'


def test_fight_rewards_are_placed_by_their_bosses(app):
    [cap] = [s for s in app['sources'] if s['name'] == 'Skrull Captain America' and not s['version'].endswith('Cosmic')]
    assert place(app, cap)['name'] == 'Hightown Patrol'


def test_unplaced_versions_stay_listed(app):
    # random terminal sub-bosses spawn in no single place the data names: no place, still a boss
    [blob] = [s for s in app['sources'] if s['id'].endswith('/EGDGSBBlob.prototype')]
    assert blob['place'] == -1 and blob['kind'] == 'boss' and blob['where']


def test_places_are_grouped_and_named(app):
    groups = [p['group'] for p in app['places']]
    assert groups == sorted(groups, key=['Terminals', 'One-shot stories', 'Patrols', 'Game modes', 'Raids', 'Story'].index)
    names = {p['name'] for p in app['places']}
    assert {'Castle Doom', 'Fisk Tower', 'Midtown Patrol', 'X-Defense', 'Muspelheim Raid', 'Chapter 8'} <= names
    used = {s['place'] for s in app['sources'] if s['kind'] == 'boss'}
    assert used - {-1} == set(range(len(app['places'])))  # every place has a boss


def test_crossbones_is_a_patrol_boss(app):
    # each version is used only by a patrol's ambient boss list (Metagame/Patrol*/Populations/.../Crossbones*)
    got = {s['id'].split('/')[-1]: place(app, s)['name'] for s in app['sources'] if s['name'] == 'Crossbones' and not s['version']}
    assert got == {'CrossbonesPatrolBossMidtown.prototype': 'Midtown Patrol',
                   'CrossbonesPatrolBossBrooklyn.prototype': 'Industry City Patrol',
                   'CrossbonesPatrolBossHightown.prototype': 'Hightown Patrol'}


def test_dark_dimension_bosses_are_in_dimensions_collide(app):
    # the terminal versions spawn in Dimensions Collide; the Midtown ones only during the Doctor Strange event
    assert 'Dark Dimension' not in {p['name'] for p in app['places']}
    for s in app['sources']:
        if '/DarkDimension/' in s['id']:
            assert (s['place'] == -1) == ('Midtown' in s['id']), s['id']


def test_raid_bosses_from_loot_containers(app):
    # raid encounters kill an invisible loot container when won (EntityKill with SpawnLoot): its table is the loot
    by_name = {s['name']: s for s in app['sources'] if s['kind'] == 'boss'}
    for name, where in (('Surtur, God of Flame', 'Muspelheim Raid'), ('Lady Hellfire', 'Muspelheim Raid'),
                        ('Red Skull Onslaught', 'Axis Raid'), ('Stark Sentinel Alpha', 'Axis Raid')):
        assert place(app, by_name[name])['name'] == where, name
        assert by_name[name]['fight']


def test_place_names_follow_the_game_map(app):
    groups = {p['name']: p['group'] for p in app['places']}
    assert groups['Daily Bugle'] == 'Terminals' and groups['Dimensions Collide'] == 'Terminals'
    assert {groups[n] for n in ('Bronx Zoo', 'March to Axis', 'Vibranium Mines')} == {'One-shot stories'}
    for n in ('S.H.I.E.L.D. Holo-Sim', 'Infernal Limbo', 'Prologue'):  # Age of Ultron: its Ultron drops only in an event
        assert n in groups, n
    assert not {'Cosmic Gate', 'One-Shots', 'Chapter 0', 'Holo-Sim', 'Limbo'} & set(groups)


def test_cosmic_patrols_are_their_own_versions(app):
    # the Cosmic patrols and modes are separate maps (MidtownPatrolCosmic...); rows for one of them only belong there
    cyc = [s for s in app['sources'] if s['name'] == 'Skrull Cyclops']
    assert sorted(s['version'] for s in cyc) == ['', 'Cosmic']
    for s in cyc:
        uniforms = [r for r in app['tables'][s['table']]['rows'] if r['pool'] == -1
                    and app['items'][r['i']]['path'].endswith('/Unique475.prototype')]
        assert len(uniforms) == 1, s['version']
    assert not any('Cosmic version' in l for l in app['labels'])


def test_dimensions_collide_and_one_shots(app):
    by_id = {s['id'].split('/')[-1]: s for s in app['sources'] if s['kind'] == 'boss'}
    assert place(app, by_id['KaeciliusDarkDimension.prototype'])['name'] == 'Dimensions Collide'
    assert place(app, by_id['KravenOneShot.prototype'])['name'] == 'Bronx Zoo'


def test_icons_carry_the_items_fallback_chain(app):
    [art] = [i for i in app['items'] if i['path'].endswith('/Art331Cosmic.prototype')]
    assert art['icon'].startswith('MarvelUIIcons.Art_331|MarvelUIIcons_HD.Item_Art331')


def test_icon_chain_tries_the_item_texture_spelling(app):
    # Godkiller names MarvelUIIcons.Unique_484; the texture is Item_Unique484
    [god] = [i for i in app['items'] if i['name'] == 'Godkiller']
    assert god['icon'].split('|')[-1] == 'MarvelUIIcons.Item_Unique484'


def test_source_ids_have_no_hash(app):
    # an id goes into the page address (#drops/boss/<name>/<id>); a '#' inside it froze the Windows app's page
    assert not [s['id'] for s in app['sources'] if '#' in s['id']]


def test_dropper_keyword_rows_follow_the_boss(app):
    # Industry City patrol tables carry Broken Crown rows "only from the Edda of Ice and Fire bosses"; only the
    # Surtur demon event bosses have that keyword, so Batroc and the rest never drop it. itembase: Loki only.
    i = next(n for n, it in enumerate(app['items']) if it['path'].endswith('/Art355Cosmic.prototype'))
    droppers = {s['name'] for s in app['sources'] if s['kind'] == 'boss'
                for r in app['tables'][s['table']]['rows'] if r['i'] == i and max(r['p']) > 0}
    assert 'Loki' in droppers and not droppers & {'Batroc', 'Kingpin', 'Crossbones', 'Taskmaster'}, droppers
    assert not any('Edda' in l for l in app['labels'])
    assert app['items'][i]['chase']  # now a boss's own drop, on Loki's page


def test_game_modes_only_on_heroic(app):
    modes = {i for i, p in enumerate(app['places']) if p['id'] in ('danger-room', 'x-defense', 'holo-sim')}
    rows = [r for s in app['sources'] if s['place'] in modes for r in app['tables'][s['table']]['rows']]
    assert rows and all(r['p'][0] == 0 and r['p'][2] == 0 for r in rows) and any(r['p'][1] > 0 for r in rows)
    # Doctor Doom's Danger Room random boss shared a table with his Cosmic terminal, which keeps its Cosmic chances
    doom = [s for s in app['sources'] if s['name'] == 'Doctor Doom' and s['version'] == 'Cosmic terminal']
    assert any(r['p'][2] > 0 for s in doom for r in app['tables'][s['table']]['rows'])


def kind_sources(app, kind):
    return [s for s in app['sources'] if s['kind'] == kind]


def test_vendors_sell_with_a_chance_per_refresh_and_a_price(app):
    cosmic = [s for s in kind_sources(app, 'vendor') if s['name'] == 'Cosmic Chaos Supplier']
    assert cosmic, {s['name'] for s in kind_sources(app, 'vendor')}
    rows = [r for s in cosmic for r in app['tables'][s['table']]['rows'] if not app['items'][r['i']]['random']]
    assert rows and all(0 < r['p'][0] <= 1 and r['p'][0] == r['p'][2] for r in rows)  # stock does not follow difficulty
    assert any(app['items'][r['i']].get('price') for r in rows)
    assert not any(s['name'] in ('', 'Crafter') and 'Test' in s['id'] for s in kind_sources(app, 'vendor'))


def test_event_vendors_are_tagged(app):
    # the Cosmic Chaos Supplier is hidden until the Cosmic Chaos week makes it visible (eWETV_Visible)
    cosmic = [s for s in kind_sources(app, 'vendor') if s['name'] == 'Cosmic Chaos Supplier']
    assert cosmic and all(s.get('event') == 'Cosmic Chaos' for s in cosmic)
    gear = [s for s in kind_sources(app, 'vendor') if s['name'] == 'Gear Vendor']
    assert gear and not any(s.get('event') for s in gear)
    # event stock on an always-there vendor keeps its event label instead of going
    galactic = [s for s in kind_sources(app, 'vendor') if s['name'] == 'Galactic Event Vendor']
    assert any(any('Guardians of the Galaxy' in app['labels'][l] for l in r['l']) for s in galactic for r in app['tables'][s['table']]['rows'])


def test_recipes_name_their_crafter_and_ingredients(app):
    recipes = kind_sources(app, 'recipe')
    assert recipes and all(s['crafter'] for s in recipes)
    assert any(any(i['item'] and i['count'] > 0 for i in s['inputs']) for s in recipes)
    texts = [i['text'] for s in recipes for i in s['inputs'] if ' or ' in i['text']]
    assert texts and all(len(set(t.split(' or '))) == len(t.split(' or ')) for t in texts)  # each name once
    # recipes that rework the item you put in (Mutations in their output) are left out
    assert not any(s['id'].endswith('/UpgradeItemSlot4Level63.prototype') for s in recipes)


def test_achievement_rewards_name_the_achievement(app):
    ach = kind_sources(app, 'achievement')
    assert ach and all(s['name'] for s in ach)
    assert any(not app['items'][r['i']]['random'] for s in ach for r in app['tables'][s['table']]['rows'])
    assert not any(i['path'] == 'VanityTitle' for i in app['items'])  # titles are not items


def test_store_items_are_findable(app):
    store = kind_sources(app, 'store')
    paths = {app['items'][r['i']]['path'] for s in store for r in app['tables'][s['table']]['rows']}
    assert any('/Costumes/' in p for p in paths)


def test_a_price_with_a_part_we_cannot_work_out_is_not_shown(app):
    # the Gear Vendor's Epic gear boxes also cost credits (a level formula): showing only "1 Eternity Splinters" misleads
    gear = [i for i in app['items'] if i['name'] == 'Epic Slot 2 Gear (Level 20)']
    assert gear and not any(i.get('price') for i in gear)


def test_skrull_impostors_are_named_as_skrulls(app):
    # the game names the impostors after the hero; itembase and players call them Skrull Elektra, Skrull Nick Fury
    for stem, name in (('/HightownEventSkrullElektra.prototype', 'Skrull Elektra'), ('/SkrullNickFury.prototype', 'Skrull Nick Fury')):
        found = [s for s in app['sources'] if s['kind'] == 'boss' and s['id'].split('~')[0].endswith(stem)]
        assert found and all(s['name'] == name for s in found), [(s['id'], s['name']) for s in found]
    assert any(s['name'] == 'Elektra' and '/Story/' in s['id'] for s in app['sources'])  # the real one keeps her name
    assert not any(s['name'] == "Skrull Kl'rt" for s in app['sources'])  # a Skrull general, no impostor


def test_no_colour_tags_in_source_names(app):
    assert not [s['name'] for s in app['sources'] if '#' in s['name']]


def test_every_named_item_has_a_card(app):
    # boxes, consumables and vendor stock included: the card carries what the item is and does (its description)
    named = [i for i in app['items'] if not i['random'] and i['path'].startswith('Entity/')]
    missing = [i['name'] for i in named if not i.get('card')]
    assert not missing, (len(missing), missing[:20])
    box = next(i for i in named if i['name'] == 'Mysterious Blue Omega Box')
    assert any(s['kind'] == 'text' for s in box['card']['sections'])


def test_no_placeholders_left_on_any_card(app):
    import re
    left = [(i['name'], t) for i in app['items'] for s in (i.get('card') or {}).get('sections', [])
            for t in [''.join(x[0] for x in (s.get('text') or [seg for l in s.get('lines', []) + s.get('options', []) for seg in l]))]
            if re.search(r'\$[^$\s]+\$', t)]
    assert not left, (len(left), left[:5])


def test_recipe_card_lists_its_ingredients(app):
    r = next(i for i in app['items'] if i['path'] == 'Entity/Items/Crafting/Recipes/Tab4Misc/HamburgerRecipe.prototype')
    texts = [''.join(x[0] for x in s['text']) for s in r['card']['sections'] if s['kind'] == 'text']
    assert any(t.startswith('Ingredients:') for t in texts), texts


GRADE1 = 'Entity/Items/Crafting/Recipes/Tab1Catalyst/Grade1Offense.prototype'


def vendor_rows(app, name):
    return [(s, r) for s in kind_sources(app, 'vendor') if s['name'] == name for r in app['tables'][s['table']]['rows']]


def test_vendor_stock_lists_each_item_once_with_its_ranks(app):
    # the server rolls only the highest rank table a player has reached, so each rank's table repeats the stock
    for s in kind_sources(app, 'vendor'):
        keys = [(r['i'], r['pool'], tuple(r['l'])) for r in app['tables'][s['table']]['rows']]
        dup = {k for k in keys if keys.count(k) > 1}
        assert all(r.get('from', 0) >= 1 for r in app['tables'][s['table']]['rows']), s['name']
        # one item can sit twice only with different ranks (its chance changes between them)
        for k in dup:
            spans = sorted((r['from'], r.get('to') or 99) for r in app['tables'][s['table']]['rows'] if (r['i'], r['pool'], tuple(r['l'])) == k)
            assert all(a[1] < b[0] for a, b in zip(spans, spans[1:])), (s['name'], app['items'][k[0]]['name'], spans)
    crafter = [s for s in kind_sources(app, 'vendor') if s['name'] == 'Crafter']
    assert crafter and not any('~' in s['id'] for s in crafter)  # one source per vendor type, not per rank
    assert crafter[0]['tabs'][:3] == ['Catalyst', 'Gear', 'Misc'], crafter[0].get('tabs')
    rows = [r for s, r in vendor_rows(app, 'Crafter') if app['items'][r['i']]['path'] == GRADE1]
    assert len(rows) == 1 and rows[0]['from'] == 2 and not rows[0].get('to'), rows  # UnlockAtCrafterRank 2: rank 2 and up


def test_crafting_services_say_what_they_do(app):
    it = next(i for i in app['items'] if i['path'] == GRADE1)
    assert it['service'] == {'tab': 'Catalyst', 'unlock': 2, 'credits': 500}, it.get('service')
    c = it['card']
    assert c['category'] == 'Catalyst service', c['category']
    texts = [''.join(x[0] for x in s['text']) for s in c['sections'] if s['kind'] == 'text']
    assert texts[0].startswith('Adds a Grade 1 offensive affix to your Catalyst.'), texts
    assert 'Requires: Catalyst' in texts, texts
    assert any(t.startswith('Ingredients:') and 'Astral Essence' in t and 'Enhanced Genome' in t for t in texts), texts
    assert 'Cost: 500 Credits' in texts, texts


def test_services_use_their_crafting_panel_icon(app):
    it = next(i for i in app['items'] if i['path'] == GRADE1)
    assert it['icon'].split('|')[0] == 'MarvelUIIcons.Craft_Grade1Offense', it['icon']


def terminal_rows(app, stem):
    s = next(s for s in app['sources'] if s['kind'] == 'boss' and s['id'].endswith('/' + stem + '.prototype'))
    return [(app['items'][r['i']]['name'], [app['labels'][l] for l in r['l']], r['p']) for r in app['tables'][s['table']]['rows']]


def test_terminal_region_rows_are_settled_per_terminal():
    # Green, Red and Cosmic terminals are separate maps; "only in" region keyword rows are settled by the level 60
    # map of each: CosmicDailies is the Cosmic terminals', Tier4 the Cosmic and Red ones', Juggernaut the Green one's
    app = json.load(gzip.open(APP))
    green = terminal_rows(app, 'EGD12GJuggernaut')
    cosmic = terminal_rows(app, 'EGD12CJuggernaut')
    red = terminal_rows(app, 'EGD12RJuggernaut')
    assert not any(l == 'Cosmic terminals only' for _, ls, _ in green + red + cosmic for l in ls)
    # the "CosmicDailies or Tier4" Shillelagh roll: the Cosmic map and the Red level 60 map (Tier4) drop it, the
    # Green one (Juggernaut) does not
    assert max(p[2] for n, ls, p in cosmic if n == "Black Tom's Shillelagh") >= 0.01
    assert max(p[0] for n, ls, p in green if n == "Black Tom's Shillelagh") < 0.001  # the Green terminal is Normal
    assert max(p[1] for n, ls, p in red if n == "Black Tom's Shillelagh") > 0.0055  # the Red terminal is Heroic


def test_green_terminals_are_named_green():
    app = json.load(gzip.open(APP))
    s = next(s for s in app['sources'] if s['kind'] == 'boss' and s['id'].endswith('/EGD12GJuggernaut.prototype'))
    assert s['version'] == 'Green terminal' and s['where'] == 'Church of Purification (Green terminal)', s


def test_terminal_colour_is_its_difficulty():
    # the colour is picked with the difficulty before joining: Green Normal, Red Heroic, Cosmic Cosmic (user, 2026-10-24)
    app = json.load(gzip.open(APP))
    for stem, tier in (('EGD12GJuggernaut', 0), ('EGD12RJuggernaut', 1), ('EGD12CJuggernaut', 2)):
        s = next(s for s in app['sources'] if s['kind'] == 'boss' and s['id'].endswith('/' + stem + '.prototype'))
        rows = app['tables'][s['table']]['rows']
        assert rows and all(r['p'][t] == 0 for r in rows for t in range(3) if t != tier), stem
        assert any(r['p'][tier] > 0 for r in rows), stem
    green = terminal_rows(app, 'EGD12GJuggernaut')
    assert not any('Cosmically Enhanced' in n for n, _, p in green if any(p)), [n for n, _, p in green if 'Cosmic' in n and any(p)]
    assert all(any(p) for _, _, p in green)  # rows a version can never drop are left out


def test_slot_from_game_data(app):
    # the hero's Gear Slot 1 to 5 ("[Gear Slot 5]" in the hero window); other kinds have their own slots
    # team-up gear's Slot folders are the team-up's own slots, not the hero's
    folder = [i for i in app['items'] if re.search(r'/Slot\d/', i['path']) and '/TeamUps/' not in i['path']]
    assert len(folder) >= 100
    assert not [i['name'] for i in app['items'] if '/TeamUps/' in i['path'] and 'slot' in i]
    assert all(i.get('slot') == int(re.search(r'/Slot(\d)/', i['path']).group(1)) for i in folder)
    by_path = {i['path']: i for i in app['items']}
    # hero uniques have no slot folder: their slot is the hero's gear inventory that takes their type
    assert by_path['Entity/Items/Armor/UniquePrototypes/Avatars/NickFury/Unique506.prototype']['slot'] == 1
    assert by_path['Entity/Items/Armor/UniquePrototypes/Avatars/BlackCat/Unique474.prototype']['slot'] == 5
    own_slot = {'rings', 'artifacts', 'medallions', 'legendaries', 'relics', 'insignias'}
    assert not [i['name'] for i in app['items'] if own_slot & set(i.get('types', [])) and 'slot' in i]


def test_types_overlap(app):
    def types(category):
        found = [set(i.get('types', [])) for i in app['items'] if (i.get('card') or {}).get('category', '') == category]
        assert found, category
        return found
    assert all({'rings', 'uniques'} <= t for t in types('Unique - Ring'))
    assert all('uniques' in t for t in types('Uru-Forged - Any Hero') + types('Unique - Blade'))
    assert all('artifacts' in t for t in types('Cosmic Artifact') + types('Cosmically Enhanced Artifact'))
    assert all('medallions' in t for t in types('Medallion of Victory') + types('Cosmic Medallion'))
    assert all('legendaries' in t for t in types('Legendary'))
    assert all('relics' in t for t in types('Relic'))
    assert any('teamup' in set(i.get('types', [])) for i in app['items']
               if (i.get('card') or {}).get('category', '').endswith('Team-Up Gear'))


def achievement(app, name):
    [a] = [a for a in app['achievements'] if a['name'] == name]
    return a


def test_achievement_mandarin(app):
    a = achievement(app, 'Cosmic Bounty Hunter: Mandarin')
    cats = app['achCategories']
    assert (a['points'], cats[a['cat']]['name'], cats[a['cat']]['subs'][a['sub']]) == (10, 'Game Modes', 'Terminals')
    assert a['reward'] == 'Coffer of Achievement' and a['rtypes'] == ['coffer']
    assert [app['items'][i]['name'] for i in a['items']] == ['Coffer of Achievement']
    assert 'Achievement_Boss_CosmicMandarin' in a['icon']
    assert ['Mandarin', 'FF00FF'] in [list(s) for s in a['desc']] or any(t == 'Mandarin' and c for t, c in a['desc'])
    expert = achievement(app, 'Cosmic Bounty Expert: Mandarin')
    assert (expert['points'], expert['parent']) == (20, a['id'])


def test_achievement_reward_is_the_item_inside_its_wrapper(app):
    # "Hannibal King's Hand Cannon Achievement Reward" is a box that always gives the gun: the reward is the gun
    a = [a for a in app['achievements'] if a['reward'] == "Hannibal King's Hand Cannon"]
    assert a and all([app['items'][i]['name'] for i in x['items']] == ["Hannibal King's Hand Cannon"] for x in a)
    assert all('gear' in x['rtypes'] for x in a)


def test_achievement_counts(app):
    shown = app['achievements']
    secret = [a for a in shown if a['vis'] == 4]
    assert (len(secret), sum(a['points'] for a in secret)) == (33, 365)
    assert len([a for a in shown if a['vis'] == 2]) == 34
    assert not [a for a in shown if a['vis'] == 6]
    assert {'Super Hero', 'Buried Treasure'} <= {a['name'] for a in shown}
    assert 1180 <= len(shown) <= 1200


def test_achievement_rtypes_cover_all(app):
    assert not [a['reward'] for a in app['achievements'] if not a['rtypes']]
    assert all(a['rtypes'] == ['none'] for a in app['achievements'] if not a['reward'] and not a['items'])
    assert any('title' in a['rtypes'] for a in app['achievements'])


def test_achievement_reward_items_exist(app):
    assert all(0 <= i < len(app['items']) for a in app['achievements'] for i in a['items'])
    assert all(a['parent'] in {x['id'] for x in app['achievements']} for a in app['achievements'] if a['parent'])


def test_achievement_with_items_but_no_reward_text_is_not_no_reward(app):
    # Transferable Concepts gives a Coffer of Achievement although the game writes no reward line for it
    a = achievement(app, 'Transferable Concepts')
    assert a['items'] and 'none' not in a['rtypes'] and 'coffer' in a['rtypes']
    assert all(not x['items'] for x in app['achievements'] if 'none' in x['rtypes'])


def test_legendaries_are_named_by_grade(app):
    # two Gungnirs: the Legendary (Item Grade 70) and its upgraded version (80); the label tells them apart (user ruling)
    gungnirs = {i['path'].rsplit('/', 1)[-1]: i for i in app['items'] if i['name'] == 'Gungnir'}
    assert (gungnirs['Legendary003.prototype']['label'], gungnirs['Legendary019A.prototype']['label']) == ('Grade 70', 'Grade 80')
    assert all(i['label'] == f"Grade {i['card']['grade']}" for i in app['items'] if (i.get('card') or {}).get('category') == 'Legendary')


def test_achievement_objectives(app):
    # the game's expanded row: The Collector lists the achievements it needs, Arm Yourself the uniques to collect
    by_id = {a['id']: a for a in app['achievements']}
    collector = achievement(app, 'The Collector')
    names = [o['name'] for o in collector['objectives']]
    assert names == sorted(names) and len(names) == 10
    assert {'Arm Yourself', 'Cow Tags', 'Very Special Things', 'Guns & Ammo'} <= set(names)
    cow = next(o for o in collector['objectives'] if o['name'] == 'Cow Tags')
    assert by_id[cow['dep']]['name'] == 'Cow Tags'
    arm = achievement(app, 'Arm Yourself')
    assert len(arm['objectives']) == 67
    darkhold = next(o for o in arm['objectives'] if o['name'] == 'The Darkhold')
    assert app['items'][darkhold['item']]['path'].endswith('/AnyHero/Slot1/Unique079.prototype')
    assert all(o['item'] == -1 or 0 <= o['item'] < len(app['items']) for a in app['achievements'] for o in a['objectives'])


def test_bloodstone_demonband_card_lists_its_rolls(app):
    # opened from a Mystic Mayhem Coffer (the only way it drops, user, 2026-10-24): 2 of 13 and 2 of 7, at item level 63
    [ring] = [i for i in app['items'] if i['path'] == 'Entity/Items/Rings/BloodstoneRing.prototype']
    rolls = [s for s in ring['card']['sections'] if s['kind'] == 'options']
    text = lambda opts: [''.join(t for t, _ in o) for o in opts]
    assert [(s['title'], s['note'], len(s['options'])) for s in rolls] == [
        ('Random bonuses', '2 different bonuses of these:', 13), ('', 'And 2 different bonuses of these:', 7)]
    assert '+900-1,500 Brutal Damage Rating' in text(rolls[0]['options']) and '+1 Fighting' in text(rolls[1]['options'])
    assert all(s['hidden'] == 0 for s in rolls)
    assert ring['rarity'] == 'unique' and ring['card']['color'] == 'B67B38'  # always Unique: the RarityUnique style
    others = [i for i in app['items'] if 'card' in i and i['path'] not in {'Entity/Items/Rings/BloodstoneRing%s.prototype' % x for x in ('', 'A', 'B', 'C', 'D')}]
    assert not [i['name'] for i in others if any(s.get('title') == 'Random bonuses' for s in i['card']['sections'])]
