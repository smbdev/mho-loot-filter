import os
import pytest
from calligraphy import GameData, double

GAME = os.environ.get('MHO_GAME', '')
pytestmark = pytest.mark.skipif(not os.path.exists(os.path.join(GAME, 'Data', 'Game', 'Calligraphy.sip')), reason='needs the game install')


@pytest.fixture(scope='module')
def game():
    return GameData(GAME)


def test_entity_loot_property_group_is_kept_apart(game):
    t = game.merged(game.paths['Entity/Characters/Bosses/PVEDailies/Cosmic/EGD09CDrDoomPhase3.prototype'])
    v = t['_props'][('Property/Mixin/LootTablePrototypeProp.blueprint', 0)]['Value']
    assert game.prototypes[v][1] == 'Loot/Tables/Mob/Bosses/EndgameDailies/zzzDeprecated/DrDoomLootCosmicMASTER.prototype'


def test_embedded_choice_keeps_parent_fields(game):
    doom = game.merged(game.paths['Loot/Tables/Mob/Bosses/EndgameDailies/Terminals/CastleDoom/DrDoomTerminalLoot.prototype'])
    pool = next(game.merged_struct(c) for c in doom['Choices']
                if game.prototypes.get(c.get('_parent'), (0, ''))[1].endswith('/EndGameBossSpecialDrops.prototype'))
    assert double(pool['NoDropPercent']) == pytest.approx(0.997)  # overridden by Doom's table
    assert len(pool['Choices']) == 63  # kept from the pool prototype


def test_abstract_flag(game):
    assert game.paths['Entity/Items/Insignias/Insignia.defaults'] in game.abstract
    assert game.paths['Entity/Items/Medals/MedalBlueprints/Endgame/DoctorDoomEGM.defaults'] not in game.abstract


def test_difficulty_curve_reads(game):
    tiers = game.merged(game.paths['Globals/Globals.defaults'])['DifficultyTiers']
    assert [game.prototypes[t][1] for t in tiers] == ['Difficulty/Tiers/Tier1Normal.prototype', 'Difficulty/Tiers/Tier2Heroic.prototype', 'Difficulty/Tiers/Tier3Superheroic.prototype']
    any_curve = next(iter(game.curve_paths))
    assert len(game.curve(any_curve)) >= 1
