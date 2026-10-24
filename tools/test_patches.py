import json, struct
import patches


class Game:
    def __init__(self):
        self.assets = {900: 'PickWeight', 901: 'PickWeightTryAll'}
        self.guids = {77: 5}


def q(x):
    return struct.unpack('<Q', struct.pack('<d', x))[0]


def test_set_nested_field_and_float():
    t = {'Choices': [{'_parent': 1, 'Choices': [{'_parent': 2, 'Weight': 100}]}], 'NoDropPercent': 0}
    patches.apply(Game(), t, [{'Path': 'Choices[0].Choices[0].Weight', 'ValueType': 'Integer', 'Value': 10},
                              {'Path': 'NoDropPercent', 'ValueType': 'Float', 'Value': 0.99}])
    assert t['Choices'][0]['Choices'][0]['Weight'] == 10 and t['NoDropPercent'] == q(0.99)


def test_append_ref_and_embedded_prototype():
    t = {'Choices': [{'_parent': 1}]}
    patches.apply(Game(), t, [{'Path': 'Choices[]', 'ValueType': 'PrototypeDataRef', 'Value': 3},
                              {'Path': 'Choices[]', 'ValueType': 'Prototype', 'Value': {'ParentDataRef': 4, 'Item': 9, 'NoDropPercent': 0.5}}])
    assert t['Choices'][1] == {'_parent': 3}
    assert t['Choices'][2] == {'_parent': 4, 'Item': 9, 'NoDropPercent': q(0.5)}


def test_enum_guid_and_arrays():
    t = {}
    patches.apply(Game(), t, [{'Path': 'PickMethod', 'ValueType': 'Enum', 'Value': 'PickWeightTryAll'},
                              {'Path': 'Item', 'ValueType': 'PrototypeGuid', 'Value': 77},
                              {'Path': 'Choices[]', 'ValueType': 'PrototypeDataRefArray', 'Value': [6, 7]}])
    assert t == {'PickMethod': 901, 'Item': 5, 'Choices': [{'_parent': 6}, {'_parent': 7}]}


def test_load_reads_enabled_top_level_files(tmp_path):
    d = tmp_path / 'src/MHServerEmu.Games/Data/v52/Patches'
    (d / 'Off').mkdir(parents=True)
    json.dump([{'Enabled': True, 'Prototype': 'A', 'Path': 'X', 'ValueType': 'Integer', 'Value': 1},
               {'Enabled': False, 'Prototype': 'B', 'Path': 'X', 'ValueType': 'Integer', 'Value': 1}], open(d / 'PatchDataOne.json', 'w'))
    json.dump([{'Enabled': True, 'Prototype': 'C', 'Path': 'X', 'ValueType': 'Integer', 'Value': 1}], open(d / 'Off/PatchDataOff.json', 'w'))
    assert [e['Prototype'] for e in patches.load(str(tmp_path))] == ['A']


def test_array_types_written_with_brackets():
    t = {'Choices': []}
    patches.apply(Game(), t, [{'Path': 'Choices[]', 'ValueType': 'PrototypeDataRef[]', 'Value': [6, 7]},
                              {'Path': 'Choices[]', 'ValueType': 'Prototype[]', 'Value': [{'ParentDataRef': 4, 'Weight': 50}]}])
    assert t['Choices'] == [{'_parent': 6}, {'_parent': 7}, {'_parent': 4, 'Weight': 50}]


def test_entries_apply_in_order_even_inside_appended_parts():
    # The server's patched DogTagsCurrencyTable (dumped through MHServerEmu, 2026-10-18) holds three Dog Tags, each
    # with a RequireDropperKeyword holding one keyword: entries reach parts that earlier entries appended.
    t = {'Choices': [{'_parent': 1, 'Choices': [{'_parent': 2}]}]}
    patches.apply(Game(), t, [
        {'Path': 'Choices[0].Choices[0].Modifiers[]', 'ValueType': 'Prototype', 'Value': {'ParentDataRef': 8}},
        {'Path': 'Choices[0].Choices[0].Modifiers[0].Choices[]', 'ValueType': 'PrototypeId', 'Value': 55},
        {'Path': 'Choices[0].Choices[]', 'ValueType': 'Prototype', 'Value': {'ParentDataRef': 2}},
        {'Path': 'Choices[0].Choices[1].Modifiers[]', 'ValueType': 'Prototype', 'Value': {'ParentDataRef': 8}}])
    assert t['Choices'][0]['Choices'] == [{'_parent': 2, 'Modifiers': [{'_parent': 8, 'Choices': [55]}]},
                                          {'_parent': 2, 'Modifiers': [{'_parent': 8}]}]


def test_entry_for_a_missing_part_is_skipped():
    t = {'Choices': []}
    patches.apply(Game(), t, [{'Path': 'Choices[3].Weight', 'ValueType': 'Integer', 'Value': 5},
                              {'Path': 'Missing.Weight', 'ValueType': 'Integer', 'Value': 5}])
    assert t == {'Choices': []}
