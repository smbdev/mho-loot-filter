"""Copies a few small item packages from a game install into testdata/ for the Go tests.

Usage: python tools/make_fixtures.py "<game folder>"
Game files are not redistributable, so testdata/ is not committed.
"""
import os
import shutil
import sys

from upk import unpack

FIXTURES = [
    'UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk',
    'UC__MarvelItem_Loot_FortuneCard_SF.upk',
    'UC__MarvelItem_Insignia_XMen_SF.upk',
    'MarvelGame.upk',
    'UC__MarvelItem_ReputationQuest_BaseItem_SF.upk',
    'UC__MarvelItem_Loot_VibraniumOre_SF.upk',
    'UC__MarvelItem_Loot_SF.upk',
]


def main(game_dir):
    cooked = os.path.join(game_dir, 'UnrealEngine3', 'MarvelGame', 'CookedPCConsole')
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'testdata')
    os.makedirs(out, exist_ok=True)
    for name in FIXTURES:
        shutil.copy(os.path.join(cooked, name), os.path.join(out, name))
        with open(os.path.join(cooked, name), 'rb') as f, open(os.path.join(out, name + '.flat'), 'wb') as flat:
            flat.write(unpack(f.read()))
    for name in ('SFX_Shared_INT.pck', 'AssetPackageCache.bin'):
        shutil.copy(os.path.join(cooked, name), os.path.join(out, name))
    shutil.copy(os.path.join(game_dir, 'Data', 'Game', 'Calligraphy.sip'), os.path.join(out, 'Calligraphy.sip'))
    print(f'wrote {len(FIXTURES) + 3} fixtures to {os.path.normpath(out)}')


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
