# MHO Loot Filter

A loot filter for **Marvel Heroes Omega 2.16a**, as played on [MHServerEmu](https://github.com/Crypto137/MHServerEmu) servers. Search for any item in the game and choose what to hide when it drops (the item itself, its name label, or its glow), or have it play an alert sound so you never miss it. One click patches your game files. The originals are backed up automatically and can be restored at any time.

> [!IMPORTANT]
> **This project was built with the help of AI.** Most of the code, the research into the game's file formats and this documentation were produced with an AI coding assistant, then tested in game by a human. If you prefer not to use or support AI-assisted software, this project is not for you.

## Features

- Search every droppable item in the game by name, with each item's own icon read from your game files
- Hide a single item (its model and glow) and/or its name label, without touching any other item
- Hidden items cannot be clicked, so you never pick them up by accident
- Play an alert sound when a chosen item drops, again for that item only
- Item groups: hide every relic, every medallion, all team-up gear, all Uru-Forged gear and more, or open a group and pick items one by one
- Hero uniques: hide or hear all of one hero's Unique items at once
- Turn off the glow of regular gear, insignias, relics and medallions by rarity, and play the alert for every Cosmic or Unique drop
- Hide gear, rings, insignias, medallions, team-up gear, catalysts and Danger Room scenarios by rarity, for example every medallion below Cosmic
- A list of everything you have filtered, editable at any time, that you can export and share with friends
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
- **Play sound**: plays an alert when the item drops, in place of its usual drop sound. It follows the game's **Sound Effects Volume**. Hidden items play no sound.

The grey line under a name says what the item is when the name alone does not, for example **PvP ring** or **Black Cat gear**, and **drops at any rarity** when one switch covers every rarity from Common to Cosmic. The tag next to the name is its category; **Loot bag** items are all drawn as the game's generic loot bag.

Many items are drawn the same way in game, for example all Uru-Forged gear. These show **Looks the same as N other items**. Hiding the item or playing its sound only affects that one item. Hiding only the name or only the glow of an item you can still see applies to every item that looks the same; click the link to see which ones and to switch them together.

**Item groups.** Ready-made groups such as Relics, Medallions, Rings, Catalysts, Danger Room scenarios, Team-up gear, Uniques, Crafting materials and Fortune Cards. Each item belongs to one group only. **Hide items**, **Hide names** and **Play sound** switch that setting on (or off) for every item in the group at once. Click the group's name to list its items and change any of them, for example hide all Insignias, then switch **Hide item** off for the two you still want to see. A group switch shows as on while every item in the group has it on. Items you have already hidden are listed first.

**Hero uniques** works the same way: **All heroes** switches every hero's own uniques at once, or pick a hero to hide or hear all of that hero's own Unique items (shown in game as "Unique - Rogue" and so on), then switch single ones back, for example the one you are farming. Uniques any hero can use stay in the Uniques group.

**Rarity.** Regular gear, insignias, relics, medallions and team-up gear glow in the colour of their rarity. Switch a colour off to remove that glow from every such item of that rarity. For example, switching off **Cosmic** removes the glow of Cosmic medallions as well as Cosmic gear.

**Hide by rarity** (on the Rarity page) hides a kind of item only when it drops at the rarities you tick. Rows are Gear, Rings, Insignias, Medallions, Team-up gear, Catalysts and Danger Room scenarios; columns are Common to Unique. A ticked drop has no model, glow or name. Rarity is only known once the item has dropped, so a click right where one lands can still pick it up; to make an item impossible to click, hide it in Item search or Item groups instead. Danger Room scenario crates are separate items for each rarity, so ticked crates cannot be clicked either.

These items also play their rarity's sound when they drop as Cosmic or Unique, instead of their own, so a **Play sound** switch on one of them is not heard for a Cosmic or Unique drop. Switch on **Play sound** for Cosmic or Unique on this page to hear every drop of that rarity. About 80 Uniques with their own look always play the Unique sound, so their **Play sound** switch links here instead.

**Alert sound.** Click **Play** to hear the alert. To use your own, click **Choose a sound file...** and pick any common sound file (MP3, WAV, OGG, FLAC and others). Quiet sounds are raised to full volume and only the first 10 seconds are used. **Use the built-in sound** switches back. A new sound takes effect the next time you click **Apply to game**.

**My filter** lists everything you have chosen, by item, group, look and rarity. Click **×** to remove an entry.

### Sharing a filter

On **My filter**, click **Export filter...** to save your filter as `mho-loot-filter.json` in your Downloads folder. Send that file to a friend; they click **Import filter...**, pick the file and confirm, then click **Apply to game**. Importing replaces their current filter. A custom alert sound is not part of the file.

### 3. Apply

Click **Apply to game** at the bottom left. A message shows how many game files were changed. Start the game and play.

Change the filter at any time and click **Apply to game** again; the filter always works from the original files, so changes never pile up.

### Undo everything

Open **Backups and restore** and click **Restore all game files**. Every changed file is put back exactly as it was, the class copies made for alert sounds are deleted, and your filter is cleared. Your chosen alert sound is kept.

### Good to know

- **Hidden items still drop, but cannot be clicked.** A hidden item has no model, glow or click area, so clicking the ground never picks it up by accident. With **Hide name** off its name label still shows when you hold Alt; switch **Hide name** on as well to be sure. Your pet's vacuum works on the server, so it still collects hidden items it is set to collect.
- **After Steam "Verify integrity of game files" or a game update**, open the filter and click **Apply to game** again. Files that were changed by something else are skipped and listed, never overwritten.
- **Backups and settings** are kept in `%LOCALAPPDATA%\MHOLootFilter`. Do not delete this folder while a filter is applied, or the filter cannot restore the originals; Steam "Verify integrity of game files" repairs the game in that case.
- **Only one copy runs at a time.** Starting it again while it is open shows a message.
- **Updates.** The app asks GitHub for the latest release when it starts, and again when you click **Check for updates** under the version number at the bottom of the sidebar. If there is a newer one, **Download** opens its page in your browser. Nothing is downloaded or installed by itself.

### Troubleshooting

| Message | What to do |
|---|---|
| Close the game to apply | Exit Marvel Heroes Omega completely, then click Apply again. |
| ... run the filter as administrator | Start the filter again and allow it to make changes when Windows asks. |
| changed outside the filter (Steam verify or game update?) - skipped | The file is not the one the filter expects. If you used Steam verify or updated the game, that file is now original and nothing is needed. If another mod changed it, restore that mod first. |
| the backup of the original is missing | Use Steam "Verify integrity of game files" (or re-copy the client), then apply again. |
| needs the Microsoft Edge WebView2 Runtime | Install the runtime from the link shown, then start the filter again. |

## How it works

Every dropped item is drawn by an item class whose Unreal Engine 3 package (`UC__MarvelItem_<Type>_SF.upk`) holds its drop effect, model and name label. The filter clears those references and recompresses the package in exactly the original layout. The game identifies these packages by a GUID in their header, which is left untouched.

Many items share one class. To hide a single one of them, the filter points that item's prototype in the game data (`Data/Game/Calligraphy.sip`) at a spare item class that has been made invisible, so only that item changes.

Drop sounds are chosen by each class's audio type and played from a Wwise sound bank in `SFX_Shared_INT.pck`. The filter puts the alert in place of the drop sound of an audio type no item uses, and gives that audio type to the classes of the items you want to hear. For an item that shares its class, the filter adds a copy of the class with the alert (a new `UC__MarvelItem_LF…_SF.upk` package), registers it in `Calligraphy.sip` and `AssetPackageCache.bin`, and points only that item at it. Restoring removes the copies.

Clicking picks an item by an invisible click area defined in the game data (its bounds), not by its model, so a hidden item would still be picked up by a click on the ground. The filter gives each hidden item's prototype its own copy of those bounds with `ComplexPickingOnly` set, which leaves the game nothing to click. Visible items keep their bounds, and items that inherit bounds from a hidden one get a plain copy so they stay clickable.

Regular gear and similar items take their glow from their rarity, which is defined in `MarvelGame.upk`. The game checks that file by SHA1, so when rarity glow is changed, the stored hash in `MarvelHeroesOmega.exe` is updated too. The original exe is backed up first.

Hide by rarity adds a few lines of UnrealScript to the end of `MarvelItem.PostAdapterInit` in `MarvelGame.upk`, which runs for every item once its rarity is known: if the item's class is one you chose (checked with `IsA`) and its rarity is ticked, it hides the item and its name label. Danger Room scenario portals share their class with hundreds of other items, so while that row is in use they are pointed at a copy of the class that the code can tell apart.

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

The built-in alert, `internal/wwise/alert.wem`, is made from a sound file with `python tools/make_alert.py "<sound file>"` (needs `pip install miniaudio`).

## License

The code is released under the [MIT License](LICENSE). The Oswald font is included under the [SIL Open Font License](internal/web/static/OFL-Oswald.txt).

## Disclaimer

This is a fan-made tool. It is not affiliated with or endorsed by Marvel, Gazillion Entertainment or the MHServerEmu project. Marvel Heroes and all related names are trademarks of their owners. Use it at your own risk and check your server's rules on client modifications.
