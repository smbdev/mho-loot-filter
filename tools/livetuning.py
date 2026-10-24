"""Which loot tables and items MHServerEmu's live-tuning files switch off, and which events switch them on."""
import json
import os

SWITCHES = ('eLTTV_Enabled', 'eWETV_Enabled')


def _rows(path):
    return json.load(open(path, encoding='utf-8-sig'))


def load(mhserver_dir):
    """(baseline, events): baseline is path -> switch value in the files the server always loads (LiveTuningData*.json
    in the top folder), events is path -> display names of the events whose files set the switch to 2 (on)."""
    lt = os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'LiveTuning')
    baseline = {}
    for name in sorted(os.listdir(lt)):
        if name.startswith('LiveTuningData') and name.endswith('.json'):
            for r in _rows(os.path.join(lt, name)):
                if r['Setting'] in SWITCHES and r['Prototype']:
                    baseline[r['Prototype']] = r['Value']
    events = {}
    for event in _rows(os.path.join(lt, 'Events.json')).values():
        for r in _rows(os.path.join(lt, event['FilePath'])):
            if r['Setting'] in SWITCHES and r['Value'] >= 2:
                events.setdefault(r['Prototype'], []).append(event['DisplayName'])
    return baseline, {k: sorted(set(v)) for k, v in events.items()}


def load_groups(mhserver_dir):
    """Table path -> (event name, item path, the item's own no-drop chance or None) for every event loot group: an
    event file puts items in a numbered group (eWETV_LootGroupNum) that tables with that number roll (eLTTV_GroupNum)."""
    lt = os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'LiveTuning')
    out = {}
    for event in _rows(os.path.join(lt, 'Events.json')).values():
        tables, items, nodrop = {}, {}, {}
        for r in _rows(os.path.join(lt, event['FilePath'])):
            if r['Setting'] == 'eLTTV_GroupNum':
                tables.setdefault(int(r['Value']), []).append(r['Prototype'])
            elif r['Setting'] == 'eWETV_LootGroupNum':
                items.setdefault(int(r['Value']), []).append(r['Prototype'])
            elif r['Setting'] == 'eWETV_LootNoDropPercent':
                nodrop[r['Prototype']] = r['Value']
        for num, paths in tables.items():
            for t in paths:
                out.setdefault(t, []).extend((event['DisplayName'], i, nodrop.get(i)) for i in items.get(num, []))
    return out


def load_visible(mhserver_dir):
    """(hidden, events): entities the always-loaded files hide (eWETV_Visible 0, such as the event vendors in Avengers
    Tower), and path -> display names of the events that show them."""
    lt = os.path.join(mhserver_dir, 'src', 'MHServerEmu.Games', 'Data', 'v52', 'LiveTuning')
    hidden = set()
    for name in sorted(os.listdir(lt)):
        if name.startswith('LiveTuningData') and name.endswith('.json'):
            hidden |= {r['Prototype'] for r in _rows(os.path.join(lt, name)) if r['Setting'] == 'eWETV_Visible' and not r['Value']}
    events = {}
    for event in _rows(os.path.join(lt, 'Events.json')).values():
        for r in _rows(os.path.join(lt, event['FilePath'])):
            if r['Setting'] == 'eWETV_Visible' and r['Value']:
                events.setdefault(r['Prototype'], []).append(event['DisplayName'])
    return hidden, {k: sorted(set(v)) for k, v in events.items()}
