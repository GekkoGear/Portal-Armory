# HQ Vehicle Linker — Design (short)

## Intent
Skip the manual Godot step of adding an array slot to an HQ and clicking each vehicle spawner. Drop a spatial on an .exe; every vehicle spawner sitting inside an HQ's polygon gets put into that HQ's `VehicleSpawners` array. Super basic, offline. Later becomes an option inside Portal Armory.

## Goals
- Drag-and-drop a `.spatial.json` (regular or minified) onto the .exe.
- For each `HQ_PlayerSpawner`: read its `HQArea` polygon, find every `VehicleSpawner` whose position is inside that shape seen from above, add them to the HQ's `VehicleSpawners` array.
- Works offline, single .exe, nothing to install.
- Core logic written so it can be dropped into Portal Armory with no changes.

## Non-goals
- No team filtering, no vehicle-type rules, no GUI.
- No changes to anything else in the spatial (positions, polygons, ObjIds, other arrays).

## Anti-goals (would be a failure even if it "works")
- Removing spawners you linked by hand.
- Reformatting or reordering the file (a giant diff / changed minification).
- Overwriting your original file.

## Behavior (decided)
- **Checklist before adding** (added 2026-10-29 by user): for each HQ, the newly found spawners are listed with their vehicle type, all ticked; the player unticks what they don't want (numbers, ranges like 5-7, A = all, N = none), Enter moves on. Already-linked spawners aren't listed. Reason: some maps (Dead Seas) have capture-point spawners on purpose inside an HQ polygon. The engine is split into `Find` (the checklist) and `Apply` (add the picked ones) so Armory can show real checkboxes.
- **Add only.** Existing entries stay; no duplicates; new ones appended in file order.
- **Top-down check, height ignored.** Verified on Dead Seas: both HQs match their current 13 spawners exactly.
- **Original untouched.** Output goes to a Desktop folder named after the spatial (same pattern as Spatial Poly Snap), with a `.txt` report.
- **Nothing found → HQ left alone.** If no spawners are inside an HQ's polygon, that HQ is not touched (user's rule, clarified 2026-10-29 after testing Andy's SDK 5 maps).
- **Missing slot is created.** Andy's older maps have HQs with no `VehicleSpawners` at all. The tool creates the array right after `InfantrySpawns` (Godot export order) and adds `"VehicleSpawners"` to the HQ's `linked` list.
- **A spawner should never be inside two different HQ polygons** (user's rule). If one is, it is added to neither HQ and reported. HQs that share the *same* polygon (Gulf of Lyndon's two Team 2 HQs) each get that polygon's spawners.
- Report lists, per HQ: spawners added, spawners already linked, and warnings (nothing found, no `HQArea`, `HQArea` pointing at a missing polygon, spawner inside two different HQ polygons).

## Approaches considered
1. **Go .exe, drag-and-drop — CHOSEN.** Same language as Portal Armory, small offline file, core ports straight over. Must avoid Go's JSON re-save (it reorders keys), so only the `VehicleSpawners` arrays get rewritten in place.
2. Python packed as .exe — faster to write, bigger file, antivirus flags, would need a Go rewrite for Armory.
3. Build directly into Portal Armory — no standalone tool; ties it to Armory releases.

## Armory port prep (decided: prep it now)
- Core lives in its own Go package (e.g. `hqlink`) with one entry point: takes the spatial's bytes, returns new bytes + a report struct. No file paths, no console, no UI inside it.
- The .exe is a thin wrapper: read dropped file → call core → write output folder + report → "press Enter to close".
- In Armory, the Portal Finalization tab calls the same function; the report struct renders in the web view.
- Small test set using copies of Dead Seas (expect zero changes), a Dead Seas copy with the boats removed (expect them re-added), and Andy's SDK 5 Badlands (expect 14 per HQ, slot created).

## My calls (check these)
- Minified input stays minified; pretty input keeps its 4-space layout.
- Tool name "HQ Vehicle Linker" is a placeholder.

## Left to the builder
- Exact output folder/report wording, Go package layout details, point-on-the-edge handling (counts as inside).
