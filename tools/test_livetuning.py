import json, os
from livetuning import load


def write(path, rows):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    json.dump(rows, open(path, 'w'))


def test_baseline_and_events(tmp_path):
    lt = tmp_path / 'src/MHServerEmu.Games/Data/v52/LiveTuning'
    write(str(lt / 'LiveTuningData.json'), [{'Prototype': 'Loot/Tables/Season.prototype', 'Setting': 'eLTTV_Enabled', 'Value': 0.0}])
    write(str(lt / 'LiveTuningDataGlobal.json'), [{'Prototype': '', 'Setting': 'eGTV_LootDropRate', 'Value': 1.0}])
    write(str(lt / 'Events/Weekly/LiveTuningDataOdin.json'), [{'Prototype': 'Loot/Tables/Odin.prototype', 'Setting': 'eLTTV_Enabled', 'Value': 2.0}])
    write(str(lt / 'Events.json'), {'OdinsBounty': {'DisplayName': "Odin's Bounty", 'FilePath': 'Events/Weekly/LiveTuningDataOdin.json'}})
    baseline, events = load(str(tmp_path))
    assert baseline == {'Loot/Tables/Season.prototype': 0.0}
    assert events == {'Loot/Tables/Odin.prototype': ["Odin's Bounty"]}


from livetuning import load_groups


def test_loot_groups_pair_tables_and_items_by_number(tmp_path):
    lt = tmp_path / 'src/MHServerEmu.Games/Data/v52/LiveTuning'
    write(str(lt / 'LiveTuningData.json'), [])
    write(str(lt / 'Events/LiveTuningDataPat.json'), [
        {'Prototype': 'Loot/Tables/Boss.prototype', 'Setting': 'eLTTV_GroupNum', 'Value': 1703.0},
        {'Prototype': 'Entity/Items/Hat.prototype', 'Setting': 'eWETV_LootGroupNum', 'Value': 1703.0},
        {'Prototype': 'Entity/Items/Hat.prototype', 'Setting': 'eWETV_LootNoDropPercent', 'Value': 0.75}])
    write(str(lt / 'Events.json'), {'Pat': {'DisplayName': "Saint Patrick's Day", 'FilePath': 'Events/LiveTuningDataPat.json'}})
    assert load_groups(str(tmp_path)) == {'Loot/Tables/Boss.prototype': [("Saint Patrick's Day", 'Entity/Items/Hat.prototype', 0.75)]}
