# MHO Loot Filter

A loot filter for **Marvel Heroes Omega 2.16a**, as played on [MHServerEmu](https://github.com/Crypto137/MHServerEmu) servers. Search for any item in the game and choose what to hide when it drops (the item itself, its name label, or its glow), or have it play an alert sound so you never miss it. One click patches your game files. The originals are backed up automatically and can be restored at any time.

> [!IMPORTANT]
> **This project was built with the help of AI.** Most of the code, the research into the game's file formats and this documentation were produced with an AI coding assistant, then tested in game by a human. If you prefer not to use or support AI-assisted software, this project is not for you.

## Features

- Search every droppable item in the game by name, with each item's own icon read from your game files
- Hide a single item (its model and glow) and/or its name label, without touching any other item
- Hidden items cannot be clicked, so you never pick them up by accident
- Play an alert sound when a chosen item drops, again for that item only
- Colour the name of a chosen item, or a whole group, when it drops: the game's colours, colour-blind safe colours or any colour you pick; the stash and tooltips keep their usual colours
- Item groups: hide every relic, every medallion, all team-up gear, all Uru-Forged gear and more, or open a group and pick items one by one
- Hero uniques: hide or hear all of one hero's Unique items at once
- Turn off the glow of regular gear, insignias, relics and medallions by rarity, and play the alert for every Cosmic or Unique drop
- Hide every glow in the game at once, then switch back only the ones you want to see
- Hide gear, rings, insignias, medallions, team-up gear, catalysts and Danger Room scenarios by rarity, for example every medallion below Cosmic; hidden drops cannot be clicked either
- A list of everything you have filtered, with a search box, editable at any time, that you can export and share with friends
- Profiles: keep several filters, each with its own alert sound and volume, and switch between them in one click
- Game tweaks the in-game Options lack: graphics presets (Potato, Balanced, Pretty), a frame rate limit, skipping the startup videos, more texture memory, lower input lag, smaller or bigger item names on the ground, and the map overlay opening by itself in every area
- A larger mouse pointer, in the game's blue or another colour
- Mute any of the game's sounds, one by one or a whole group such as hero voices or music, and preview each one first
- Drop finder: look up where any item drops and its chance per kill on Normal, Heroic and Cosmic, what every boss drops in each place, and what vendors sell or craft, with each item shown as its in-game tooltip
- A Play button that starts the game through Steam or Bifrost
- Finds the game in Steam libraries, Steam depot downloads and archived copies, or lets you browse to it
- Backs up every game file before changing it, and restores everything with one click

## Requirements

- Windows 10 or 11, 64-bit
- The Marvel Heroes Omega 2.16a client (version 1.52.0.1700). The filter checks every file before changing it and leaves other versions alone.
- Microsoft Edge WebView2 Runtime. It is part of Windows 11 and most up-to-date Windows 10 installs; if it is missing, the app tells you where to get it.

## Install

1. Download `MHOLootFilter.exe` from the [releases page](../../releases).
2. Put it anywhere, for example your Desktop. It is a single file and needs no installation.
3. Run it. Windows asks to let it make changes, because the game folder is usually under Program Files. The first time, Windows SmartScreen may warn that the app is unrecognised, because it is not code-signed; choose **More info**, then **Run anyway**, or build it yourself (see [Building from source](#building-from-source)).

## How to use the filter

Close the game before you start. The filter will not change files while the game is running.

### 1. Check the game folder

The bottom left of the window shows **Game found** when the filter has located your install. If it says **Game folder not found**:

1. Open **Game folder** in the sidebar.
2. Click **Browse...** and choose the folder that contains `UnrealEngine3` and `Data`, or paste the path into the box and click **Use this folder**.

The filter looks in your Steam libraries (including Steam console depot downloads), the folder the game is running from, and common places such as Downloads, Desktop and Documents. Other installs it finds are listed on the same page.

### 2. Choose what to hide or hear

**Item search.** Type part of an item name. Each result shows the item's icon, read from your own game files once the filter has found your game folder, and has switches:

- **Hide item**: the item drops without its model or glow, and clicking the ground no longer picks it up. Its name still shows when you hold Alt.
- **Hide name**: also removes the name label, so the item is completely invisible.
- **Hide glow** (on items that have their own glow and look unique): removes just the glow.
- **Play sound**: plays an alert when the item drops, in place of its usual drop sound. The game's Sound Effects, Character and Music Volume settings do not change it; set its loudness under **Alert sound**. Hidden items play no sound.
- **Drop colour**: the colour of the item's name on the ground. Pick one of the game's colours, a colour-blind safe colour, or **Custom colour...** to choose any colour by its hex, RGB or HSL value. **Default** gives the name back its usual colour. Only the name on the ground changes: the item's slot in your stash and inventory, and its tooltip, keep their rarity colours.

The grey line under a name says what the item is when the name alone does not, for example **PvP ring** or **Black Cat gear**, and **drops at any rarity** when one switch covers every rarity from Common to Cosmic. The tag next to the name is its category; **Loot bag** items are all drawn as the game's generic loot bag.

Many items are drawn the same way in game, for example all Uru-Forged gear. These show **Looks the same as N other items**. Hiding the item or playing its sound only affects that one item. Hiding only the name or only the glow of an item you can still see applies to every item that looks the same; click the link to see which ones and to switch them together.

**Item groups.** Ready-made groups such as Relics, Medallions, Rings, Catalysts, Danger Room scenarios, Team-up gear, Uniques, Crafting materials and Fortune Cards. Each item belongs to one group only. **Hide items**, **Hide names** and **Play sound** switch that setting on (or off) for every item in the group at once, and **Drop colour** colours the names of every item in it. Click the group's name to list its items and change any of them, for example hide all Insignias, then switch **Hide item** off for the two you still want to see. A group switch shows as on while every item in the group has it on. Items you have already hidden are listed first.

**Hero uniques** works the same way: **All heroes** switches every hero's own uniques at once, or pick a hero to hide or hear all of that hero's own Unique items (shown in game as "Unique - Rogue" and so on), then switch single ones back, for example the one you are farming. Uniques any hero can use stay in the Uniques group.

**Rarity.** Regular gear, insignias, relics, medallions and team-up gear glow in the colour of their rarity. Switch a colour off to remove that glow from every such item of that rarity. For example, switching off **Cosmic** removes the glow of Cosmic medallions as well as Cosmic gear.

**Every glow** (at the top of the Rarity page) hides the glow, the beam of light on the ground, of every item and every rarity at once, Fortune Cards, artifacts and chests included. Then switch back only the ones you want to see: find an item in **Item search** and turn off its **Hide glow** switch, or turn off **Hide glow** for a rarity on the Rarity page. Items that look the same share their glow, so switching one back switches back all of them.

**Hide by rarity** (on the Rarity page) hides a kind of item only when it drops at the rarities you tick. Rows are Gear, Rings, Insignias, Medallions, Team-up gear, Catalysts and Danger Room scenarios; columns are Common to Unique. A ticked drop has no model, glow or name, and cannot be clicked, so you never pick it up by accident.

These items also play their rarity's sound when they drop as Cosmic or Unique, instead of their own, so a **Play sound** switch on one of them is not heard for a Cosmic or Unique drop. Switch on **Play sound** for Cosmic or Unique on this page to hear every drop of that rarity. About 80 Uniques with their own look always play the Unique sound, so their **Play sound** switch links here instead.

**Alert sound.** Click **Play** to hear the alert. To use your own, click **Choose a sound file...** and pick any common sound file (MP3, WAV, OGG, FLAC and others). Quiet sounds are raised to full volume and only the first 10 seconds are used. **Use the built-in sound** switches back. Drag **Volume** to make the alert louder or quieter (from -12 to +28 dB, +8 dB by default, in the middle of the bar); **Play** uses the volume you set, so you can try it first. The alert ignores the game's Sound Effects, Character and Music Volume settings, so this is the one place to set how loud it is. A new sound or volume takes effect the next time you click **Apply to game**.

**My filter** lists everything you have chosen, by item, group, look and rarity. Type in its search box to find an entry by name, description or drop colour. Click **×** to remove an entry.

### Profiles

A profile is a whole filter with its own alert sound and volume, for example one for Holo-Sim that hides everything but crafting materials and one for the rest of the game. Pick the profile at the top left, then click **Apply to game** with the game closed. On **My filter**, **New profile** starts an empty one, **Copy profile** starts from the one in use, and **Delete profile** removes it. Type in the name box and press Enter to rename it. Your filter from earlier versions becomes the profile **My filter**.

### Sharing a filter

On **My filter**, click **Export filter...** to save your filter as `mho-loot-filter.json` in your Downloads folder. Send that file to a friend; they click **Import filter...**, pick the file and confirm, then click **Apply to game**. Importing replaces the filter of the profile in use. A custom alert sound is not part of the file.

### Game tweaks

**Graphics preset** sets shadows, bloom, depth of field, particles, lens flares, shadow detail and draw distance together: Potato turns everything off for the most frames per second, Balanced keeps the look with the costliest effects off, and Pretty turns everything up. The settings the game's Advanced graphics Options have are written into your Options file (`Documents\My Games\Marvel Heroes\MarvelGame\Config\MarvelSystemSettingsUserSettings.ini`), so the game shows them and you can still change them there; the rest go into `DefaultSystemSettings.ini` with the other tweaks. Choosing Game default, or restoring all game files, puts back the Options you had before the first preset.

**Game tweaks** also changes settings the game's Options do not have: a frame rate limit (or none), skipping the logo videos at start, more texture memory so textures stay sharp, and lower input lag. It also restyles the mouse pointer: pick a colour for the blue arrow and a larger size. The red attack arrow and the badges (pick up, talk, stash...) keep their colours, and the tip of the arrow still marks where you click. **Item names on the ground** makes the names of dropped items smaller or bigger. **Map overlay** shows the big map that Tab opens in the middle of the screen every time you enter an area or terminal; hubs such as Avengers Tower keep the small minimap, and Tab still switches between them. Tweaks are the same for every profile and are used the next time you click **Apply to game**.

### Mute sounds

**Mute sounds** lists the sounds the game plays by name, in groups such as hero voices, boss voices, powers, interface and music. **Mute all** silences a whole group; open a group, or search, to mute single sounds. **Play** previews a sound from your own game files. Muted sounds are the same for every profile and change the next time you click **Apply to game**.

### Drop finder

**Drop finder**, under Lookup, shows where items come from. It changes nothing in your game files.

- **Items**: search any item. Its page shows the item as its tooltip in game (stat ranges, description, what a box contains), the bosses that drop it with their chance per kill, the places it drops (**Loot info**), the boxes it comes in, the vendors that sell it, who crafts it and the achievements that give it.
- **Bosses**: everything a boss drops: its own items first, then random gear, currencies and crafting materials. A boss often drops different things in different places, so **Drops by place** lets you pick the place; places with several versions of the boss (such as the Green, Red and Cosmic terminals) list them underneath. **Location** lists every place the boss is found, on which difficulties; open one to see every boss there.
- **Places**: terminals, patrols, game modes, raids and story chapters, with the bosses in each.
- **Vendors**: what each vendor stocks, the chance an item is in stock after a refresh, its price and the vendor rank it needs. Crafter and Enchanter services are listed with what they do, their ingredients and their credit cost.

Pick **Normal**, **Heroic** or **Cosmic** at the top right to see the chances on that difficulty. **Back** (or mouse button 4) goes back a page. How the chances are worked out, and why they can differ from your server's, is under [How the drop chances are worked out](#how-the-drop-chances-are-worked-out).

### 3. Apply

Click **Apply to game** at the bottom left. A message shows how many game files were changed. Start the game and play.

Change the filter at any time and click **Apply to game** again; the filter always works from the original files, so changes never pile up.

**Play**, under **Apply to game**, applies any changes and starts the game. With a Steam copy it starts the game through Steam, so the launch options that pick your server are used. If a Bifrost launcher is in the game folder (or a folder inside it), Play starts the game with the server and settings saved in Bifrost, as Bifrost's own Play button does. When both are there, pick one in the list above **Play**.

### Undo everything

Open **Backups and restore** and click **Restore all game files**. Every changed file is put back exactly as it was, the class copies made for alert sounds are deleted, and the filter of the profile in use, the game tweaks and the muted sounds are cleared. Other profiles and your chosen alert sound are kept.

### Good to know

- **Hidden items still drop, but cannot be clicked.** A hidden item has no model, glow or click area, so clicking the ground never picks it up by accident. With **Hide name** off its name label still shows when you hold Alt; switch **Hide name** on as well to be sure. Your pet's vacuum works on the server, so it still collects hidden items it is set to collect.
- **After Steam "Verify integrity of game files"**, open the filter and click **Apply to game** again. Files that were changed by something else are skipped and listed, never overwritten.
- **Backups and settings** are kept in `%LOCALAPPDATA%\MHOLootFilter`. Do not delete this folder while a filter is applied, or the filter cannot restore the originals; Steam "Verify integrity of game files" repairs the game in that case.
- **Only one copy runs at a time.** Starting it again while it is open shows a message.
- **Updates.** The app asks GitHub for the latest release when it starts, and again when you click **Check for updates** under the version number at the bottom of the sidebar. If there is a newer one, **Download** opens its page in your browser. Nothing is downloaded or installed by itself.

### Troubleshooting

| Message | What to do |
|---|---|
| Close the game to apply | Exit Marvel Heroes Omega completely, then click Apply again. |
| ... run the filter as administrator | Start the filter again and allow it to make changes when Windows asks. |
| changed outside the filter (Steam verify?) - skipped | The file is not the one the filter expects. If you used Steam verify, that file is now original and nothing is needed. If another mod changed it, restore that mod first. |
| the backup of the original is missing | Use Steam "Verify integrity of game files" (or re-copy the client), then apply again. |
| needs the Microsoft Edge WebView2 Runtime | Install the runtime from the link shown, then start the filter again. |

## How it works

Every dropped item is drawn by an item class whose Unreal Engine 3 package (`UC__MarvelItem_<Type>_SF.upk`) holds its drop effect, model and name label. The filter clears those references and recompresses the package in exactly the original layout. The game identifies these packages by a GUID in their header, which is left untouched.

Many items share one class. To hide a single one of them, the filter points that item's prototype in the game data (`Data/Game/Calligraphy.sip`) at a spare item class that has been made invisible, so only that item changes.

Drop sounds are chosen by each class's audio type and played from a Wwise sound bank in `SFX_Shared_INT.pck`. The filter puts the alert, at the volume you set, in place of the drop sound of an audio type no item uses, and gives that audio type to the classes of the items you want to hear. For an item that shares its class, the filter adds a copy of the class with the alert (a new `UC__MarvelItem_LF…_SF.upk` package), registers it in `Calligraphy.sip` and `AssetPackageCache.bin`, and points only that item at it. Restoring removes the copies.

Clicking picks an item by an invisible click area defined in the game data (its bounds), not by its model, so a hidden item would still be picked up by a click on the ground. The filter gives each hidden item's prototype its own copy of those bounds with `ComplexPickingOnly` set, which leaves the game nothing to click. Visible items keep their bounds, and items that inherit bounds from a hidden one get a plain copy so they stay clickable.

Regular gear and similar items take their glow from their rarity, which is defined in `MarvelGame.upk`. The game checks that file by SHA1, so when rarity glow is changed, the stored hash in `MarvelHeroesOmega.exe` is updated too. The original exe is backed up first.

Hide by rarity adds a few lines of UnrealScript to the end of `MarvelItem.PostAdapterInit` in `MarvelGame.upk`, which runs for every item once its rarity is known: if the item's class is one you chose (checked with `IsA`) and its rarity is ticked, it hides the item and its name label and stops clicks on its mesh. The game normally clicks items by their prototype's bounds, which every rarity shares, so items in a row in use are set to `ComplexPickingOnly` in `Calligraphy.sip`, and the item mesh's `bEnableLineCheckWithBounds` is switched on so drawn items are clicked by their model's bounding box instead. Danger Room scenario portals share their class with hundreds of other items, so while that row is in use they are pointed at a copy of the class that the code can tell apart.

Game tweaks are written at the end of `DefaultEngine.ini` and `DefaultSystemSettings.ini` in `UnrealEngine3/MarvelGame/Config`, after a marker line. The game merges those files into the settings in your Documents folder, and later lines override earlier ones, so your own Options are kept. Removing everything from the marker on gives back the original file.

Drop colours change the label code instead of the game data: the game builds an item's ground label as HTML, and a few lines added to the start of `MarvelGFxActorTooltipComp.CallShowTooltip` in `MarvelGame.upk` read the item's name from it, look it up in the lists of names you coloured, and rewrite the colour of the name's font tag. The size of names on the ground is the `FontSize` of the `TooltipItemTitleInWorld` text style in `Calligraphy.sip`.

The map overlay adds a few lines to the end of `MarvelHUD.InitAvatarHUDData` in `MarvelGame.upk`, which runs each time you enter a new area: outside hubs it toggles the map to the overlay, the same way Tab does.

The mouse pointers are 64x64 pictures in `MarvelGame.upk` and `MarvelHUD_SF.upk`. The filter recolours the blue in them and, for a larger pointer, draws them on a 128x128 canvas, scaled from the top left corner where the tip of the arrow is.

The game plays sounds through Wwise events named by its packages (`Play_sfx_ui_LevelUp`...), whose ID is the hash of the name. Muting changes an event's ID inside its sound bank in the `.pck` files, so the game finds nothing to play. Changing the ID back gives back the exact original file, so these large files are not backed up; instead each one is checked against the game's own copy before it is changed. Previews convert the game's Wwise Vorbis audio into Ogg Vorbis, following ww2ogg.

### How the drop chances are worked out

The drop finder's chances are not collected from players or copied from a website: they are worked out from the game data and the server's loot rules. `tools/build_drops.py` reads every loot table, item and boss in `Calligraphy.sip`, applies the data patches and live tuning defaults that MHServerEmu ships, and follows the server's loot code step by step: how a table picks its rows, drop chances, how many of an item drop, rarity rolls, level and drop restrictions, difficulty, region and dropper conditions. A chance is the chance of at least one of the item per kill (or per box opened, or per vendor refresh) for a level 60 hero, before item find bonuses.

To check the maths, a test harness rolls a large sample of the same tables through MHServerEmu's own loot code (hundreds of tables, 20,000 to 200,000 rolls each, on every difficulty) and compares the counts with the drop finder's chances; the released data matches it.

The data is built from the standard [MHServerEmu](https://github.com/Crypto137/MHServerEmu) as it ships (version 1.0.2, October 2026). **Servers run their own settings, so some drop chances and vendor items may be different on the server you play on.** Live tuning (drop rate and event changes), custom patches or loot changes, which events are on, and vendor stock a server has changed all affect what actually drops. Item find bonuses, party size and live events are not included. Drops that only happen during an event, in a certain place or from a certain boss are tagged.

## Building from source

### On Windows

1. Install [Go 1.23 or newer](https://go.dev/dl/) and [Git](https://git-scm.com/download/win).
2. Open PowerShell and run:

   ```powershell
   git clone https://github.com/smbdev/mho-loot-filter.git
   cd mho-loot-filter
   go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64 --out cmd/mholootfilter/rsrc
   go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/MHOLootFilter.exe ./cmd/mholootfilter
   ```

3. The app is `dist\MHOLootFilter.exe`.

The `go-winres` step embeds the icon, version information and the request to run as administrator. Without it the app still builds, but Windows will not ask for administrator rights and patching games under Program Files will fail.

### On Linux or macOS (cross-compiling)

```sh
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64 --out cmd/mholootfilter/rsrc
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -s -w" -o dist/MHOLootFilter.exe ./cmd/mholootfilter
```

For development, `go run ./cmd/mholootfilter -game "<game folder>"` serves the same interface at http://127.0.0.1:47816 on Linux and macOS.

### Tests

```sh
go test ./...
```

Tests that need real game files are skipped until you create the fixtures from your own install. Game files are not included in this repository.

```sh
pip install lz4
python tools/make_fixtures.py "<game folder>"
```

### Regenerating the item database

The item list, groups and patch offsets in `internal/db/itemdb.json` are generated from a game install:

```sh
pip install lz4 pytest
python tools/build_db.py "<game folder>"
python -m pytest tools/test_build_db.py
```

Item groups are defined in `tools/groups.py` and categories in `tools/categories.py`.

### Regenerating the drop data

The drop finder's data in `internal/db/drops.json.gz` is generated from a game install and an MHServerEmu checkout (for its data patches and live tuning):

```sh
pip install lz4 pytest
python tools/build_drops.py "<game folder>" "<MHServerEmu checkout>"
MHO_GAME="<game folder>" MHSERVER="<MHServerEmu checkout>" python -m pytest tools
```

Tests that need the game files are skipped unless `MHO_GAME` (and for some, `MHSERVER`) is set.

The list of sounds in `internal/db/sounds.json` is generated from a game install with `go run ./tools/soundindex "<game folder>"`.

The built-in alert, `internal/wwise/alert.wem`, is made from a sound file with `python tools/make_alert.py "<sound file>"` (needs `pip install miniaudio`).

## License

The code is released under the [MIT License](LICENSE). The Oswald font is included under the [SIL Open Font License](internal/web/static/OFL-Oswald.txt). Sound previews convert the game's Wwise Vorbis audio following ww2ogg by Adam Gashlin, whose codebook library is included under its [BSD license](internal/wwise/ww2ogg-LICENSE).

## Disclaimer

This is a fan-made tool. It is not affiliated with or endorsed by Marvel, Gazillion Entertainment or the MHServerEmu project. Marvel Heroes and all related names are trademarks of their owners. Use it at your own risk and check your server's rules on client modifications.
