"""Node conversion (build_drops.node, mod, restrictions) on small hand-made prototypes."""
import struct
import build_drops


def q(x):
    return struct.unpack('<Q', struct.pack('<d', x))[0]


class Game:
    """Just enough of calligraphy.GameData for build_drops: prototypes by id, merged dicts, asset names."""
    def __init__(self, protos, assets):
        self.prototypes = {pid: (0, path) for pid, (path, _) in protos.items()}
        self._fields = {pid: fields for pid, (_, fields) in protos.items()}
        self.paths = {path: pid for pid, (path, _) in protos.items()}
        self.assets, self.abstract, self.curve_paths = assets, set(), {}

    def merged(self, pid):
        return {'_path': self.prototypes[pid][1], **self._fields[pid]}

    def merged_struct(self, s):
        base = self.merged(s['_parent']) if s.get('_parent') in self.prototypes else {}
        return {**base, **{k: v for k, v in s.items() if k != '_parent'}, '_path': base.get('_path', '')}


ROOTS = {1: ('Loot/Tables/Table.defaults', {'_root': 'Loot/Tables/Table.defaults', 'NumMin': 1, 'Weight': 100,
                                             'LiveTuningDefaultEnabled': 1, 'PickMethod': 900}),
         2: ('Loot/DropTypes/Item.defaults', {'_root': 'Loot/DropTypes/Item.defaults', 'NumMin': 1, 'Weight': 100}),
         3: ('Loot/Modifiers/RequireDifficultyTier.defaults', {'_root': 'Loot/Modifiers/RequireDifficultyTier.defaults'}),
         4: ('Difficulty/Tiers/Tier2Heroic.prototype', {}), 5: ('Difficulty/Tiers/Tier5Omega1.prototype', {}),
         6: ('Entity/Items/DropRestrictions/Main/Conditional.defaults', {'_root': 'Entity/Items/DropRestrictions/Main/Conditional.defaults'}),
         7: ('Entity/Items/DropRestrictions/Main/Rarity.defaults', {'_root': 'Entity/Items/DropRestrictions/Main/Rarity.defaults'}),
         8: ('Entity/Items/DropRestrictions/Main/Level.defaults', {'_root': 'Entity/Items/DropRestrictions/Main/Level.defaults'}),
         9: ('Entity/Items/Rarity/R3Rare.prototype', {}), 10: ('Entity/Items/Rarity/R4Epic.prototype', {}),
         11: ('Entity/Items/DropRestrictions/Main/Context.defaults', {'_root': 'Entity/Items/DropRestrictions/Main/Context.defaults'})}
ASSETS = {900: 'PickWeight', 901: 'Drop', 902: 'Crafting', 903: 'MysteryChest'}


def game(item_fields):
    return Game({**ROOTS, 20: ('Entity/Items/Thing.prototype', {'DesignState': 950, **item_fields})}, {**ASSETS, 950: 'Live'})


def test_table_node_defaults_and_nodrop():
    g = game({})
    t = {'_parent': 1, 'NoDropPercent': q(0.25), 'Choices': [{'_parent': 2, 'Item': 20, 'NumMax': 3}]}
    n = build_drops.node(g, g.merged_struct(t))
    assert n['pick'] == 'weight' and n['nodrop'] == 0.25 and n['num'] == (1, 1)
    assert n['choices'][0]['num'] == (1, 3) and n['choices'][0]['item'] == 'Entity/Items/Thing.prototype'


def test_difficulty_mod_keeps_only_released_tiers():
    g = game({})
    assert build_drops.mod(g, {'_parent': 3, 'Choices': [4, 5]}) == {'kind': 'RequireDifficultyTier', 'tiers': ['Red']}
    assert build_drops.mod(g, {'_parent': 3, 'Choices': [5]})['tiers'] == ['none']


def test_conditional_restriction_follows_the_roll_context():
    cond = {'_parent': 6, 'ApplyFor': [902], 'Apply': [{'_parent': 7, 'AllowedRarities': [9]}],
            'Else': [{'_parent': 7, 'AllowedRarities': [10]}, {'_parent': 8, 'LevelMin': 38, 'LevelRange': 99}]}
    g = game({'LootDropRestrictions': [cond]})
    levels, allowed, ok = build_drops.restrictions(g, 20, 'Drop')
    assert allowed == {'R4Epic'} and levels == [(38, 99)] and ok
    assert build_drops.restrictions(g, 20, 'Crafting')[1] == {'R3Rare'}


def test_context_restriction_blocks_other_contexts():
    g = game({'LootDropRestrictions': [{'_parent': 11, 'UsableFor': [902, 903]}]})
    assert build_drops.restrictions(g, 20, 'Drop')[2] is False
    assert build_drops.restrictions(g, 20, 'MysteryChest')[2] is True


def test_patched_build_starts_with_fresh_random_item_caches():
    build_drops._families[1] = ['x']
    build_drops._allowed_cache[(('t',), 'Drop')] = {'R1Common'}
    build_drops._slots['Artifact01'] = {1}
    build_drops.reset_caches()
    assert not build_drops._families and not build_drops._allowed_cache and not build_drops._slots
