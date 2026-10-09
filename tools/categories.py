"""Groups item types into categories by their Unreal class name (without the "marvelitem_" prefix)."""

# (category, class name pattern) - the first match wins
CATEGORIES = [
    ('Uniques', r'^(unique|uniquebase)'),
    ('Fortune Cards', r'^loot_fortunecard$'),
    ('Chests', r'^(odinbountychest|supershineylootbox|midtownmadnesschest|mysticcratedrop)$'),
    ('Artifacts', r'^(artifact|loot_(embers|mists|flames|smoke)_)'),
    ('Legendaries', r'^loot$'),
    ('Insignias', r'^insignia'),
    ('Medallions', r'^loot_origin_fame$'),
    ('Relics', r'^loot_(origin_)?relic'),
    ('Team-up gear', r'^teamupcommon$'),
    # Equippable catalysts (Hero Synergy), though their classes are named like crafting cores.
    ('Catalysts', r'^loot_core_(xgene|vibranium|promethium|radioactiveisotope|mkraanshard)$'),
    ('Crafting', r'^(loot_costcomp|loot_core|loot_costume_component|loot_unstablemolecule|loot_vibraniumore|repquest|reputationquest)'),
    ('Recipes', r'^loot_consumable_respecpotion$'),
    ('Consumables', r'^(loot_consumable|loot_acorn)'),
    ('Event items', r'^limitededition'),
    ('Runes', r'^(loot_runestone|loot_pvprunestone|uruforged)'),
    ('Costumes', r'^loot_costume$'),
    ('Hero tokens', r'^loot_character$'),
    ('Gear', r'^(armor_|itemarmor)'),
]

# Quest items and currency pickups are never filtered.
NEVER = r'^(quest_|loot_missionitem|loot_money|loot_goldencrown|loot_legendarymark|loot_eternitysplinter|loot_infinitygempoint|loot_guildsanction|loot_matrixofunbinding|interactable_)'
