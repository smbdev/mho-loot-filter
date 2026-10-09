"""Ready-made item groups shown on the Groups page. Each test gets (name, prototype path, category).

An item joins only the first group that matches, so put narrower groups before the wider ones they overlap.
"""

GROUPS = [
    ('relics', 'Relics', 'Every relic', lambda n, p, c: p.startswith('Entity/Items/Relics/')),
    # A medallion's Cosmic version is the same item rolled at Cosmic rarity, so only its glow can be told apart.
    ('medallions', 'Medallions', 'Every medallion, normal and Cosmic. To remove only the Cosmic glow, use Rarity.',
     lambda n, p, c: p.startswith('Entity/Items/Medals/')),
    ('cosmic-artifacts', 'Cosmically Enhanced artifacts', 'Only the Cosmically Enhanced versions',
     lambda n, p, c: 'CosmicArtifacts' in p),
    ('artifacts', 'Artifacts', 'Every artifact except the Cosmically Enhanced versions', lambda n, p, c: c == 'Artifacts'),
    ('insignias', 'Insignias', 'Every insignia', lambda n, p, c: c == 'Insignias'),
    ('teamup', 'Team-up gear', 'Communicators, Biometrics Enhancers and other team-up gear', lambda n, p, c: c == 'Team-up gear'),
    ('uniques', 'Uniques', 'Every Unique item', lambda n, p, c: c == 'Uniques'),
    ('uru', 'Uru-Forged gear', 'Uru-Forged weapons and armor', lambda n, p, c: 'Uru-Forged' in n),
    ('runes', 'Runes', 'Runes and runeword glyphs', lambda n, p, c: c == 'Runes' and 'Uru-Forged' not in n),
    ('catalysts', 'Catalysts', 'Genetic Mutation, Mystical Energies and the other equippable catalysts', lambda n, p, c: c == 'Catalysts'),
    ('crafting', 'Crafting materials', 'Ionic Particles, Nanotech Filaments, costume cores and more', lambda n, p, c: c == 'Crafting'),
    ('fortune', 'Fortune Cards and reward boxes', 'Fortune Cards and achievement rewards', lambda n, p, c: c == 'Fortune Cards'),
    ('chests', 'Chests', "Chest of Odin's Bounty, Cosmic Reliquary and other boxes", lambda n, p, c: c == 'Chests'),
    ('event', 'Event items', 'Halloween Candy, seasonal gifts and other event currency', lambda n, p, c: c == 'Event items'),
    ('costumes', 'Costumes', 'Costume drops', lambda n, p, c: c == 'Costumes'),
    ('heroes', 'Hero tokens', 'Hero and team-up unlock tokens', lambda n, p, c: c == 'Hero tokens'),
    ('consumables', 'Consumables', 'Boosts, potions and other consumables', lambda n, p, c: c == 'Consumables'),
    ('recipes', 'Recipes and credit chests', 'Crafting recipes, upgrades and credit chests', lambda n, p, c: c == 'Recipes'),
    ('legendaries', 'Legendaries and special loot', 'Items drawn with the generic loot model', lambda n, p, c: c == 'Legendaries'),
]
