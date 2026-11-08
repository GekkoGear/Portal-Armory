# PolySnap (Godot add-on)

Lines up the PolygonVolume points in a Battlefield 6 Portal scene, right inside Godot.
It adds a **PolySnap** tab, and a **Point Snap** toggle to the 3D view, next to 2D / 3D / Script that shows a top-down BEFORE / AFTER of
every proposed change. Keep, undo or drag each one, then press **Finalize** to write it
into the scene. Ctrl+Z undoes it.

Works with the Battlefield 6 Portal SDK on Godot 4.2 and newer (tested on 4.2.2, 4.4.1,
4.6.2 and 4.6.3, the version the SDK ships with). Godot 3 can't open Portal projects.
Fully offline.

## Install

1. Copy the `portal_polygons` folder into your project's `addons` folder, so you have
   `GodotProject/addons/portal_polygons/plugin.cfg`.
2. In Godot: **Project > Project Settings > Plugins**, tick **Enabled** next to
   *PolySnap*. (Godot asks for this once per project.)
3. A **PolySnap** tab appears at the top of the editor.

## Use

Open your map scene and click the **PolySnap** tab (its icon is your map).

| | |
|---|---|
| **BEFORE / AFTER** | Your volumes from above, side by side. By default anchor points (ones that don't move) are white, off points in BEFORE are red, fixed points in AFTER are green, points you moved are blue and undone changes are grey. In AFTER, a grey ring and line show where a point started. Each view has a legend. |
| **Colors** | Pick your own color for anchor, off, fixed, moved-by-you and undone points, and for the polygon outlines (one color, or a different color per polygon), each with a color wheel. Your colors are remembered in every project. **Reset colors** goes back to the defaults. |
| **Hover a point** | A card shows what changes and by how much, with a **Keep this change** box. Untick it and the point stays where it was. **Show in 3D** selects the volume. |
| **Drag a point in AFTER** | Move it where it should be. It snaps to nearby corners and edges of other volumes; hold **Alt** to place it freely. Untick **Keep my move** to throw the move away. |
| **Changes list** | Every change with its own keep box. Click a row to zoom to it. |
| **Needs a look** | Problems, corners that are close but weren't snapped (10-50 cm), and corners sitting on the edge of a volume lower in the scene. Click to zoom. |
| **Finalize** | Writes the AFTER positions into the scene as one step. Ctrl+Z undoes it. |

## The tab icon

The **PolySnap** tab shows only an icon: the open map seen from above, with its play area
(the CombatArea's CombatVolume) in white with 14 red points, Team 1's starting HQ in blue and
Team 2's in orange. Its hover text says "PolySnap". Hover it and the HQs fade while the 14 points drag one after another into
a "P"; move away and they drag back. With no map open it shows a fixed random shape. It
the map to finish loading (checking again every second, for up to 30 seconds) and then shows it.

## Which way up

Maps are always drawn from above, exactly like Godot's 3D top view (+X to the right, +Z toward
the bottom). Press **Rotate** in the PolySnap tab to turn the BEFORE/AFTER views 90° at a time so
a map sits the way you think of it; the tab icon turns with them. Turning never mirrors a map.

## Updating (Coming Later)

The toolbar has a **Check for Update** button that is switched off for now. When it is
turned on, pressing it is the only time the add-on goes online: it reads the installed
version (Rev 3.5 is the current one), checks GitHub for a newer `polysnap-rev...`
release, and only if there is one downloads it, checks it, backs up the old version and
replaces only the tool's own files (listed in `manifest.txt`) in the folder you pick or
drag in. Nothing else is ever deleted.

## Point Snap (3D view)

The 3D view's toolbar gets a **Point Snap** toggle and a distance box (0.25 m by default).
With it on, drag a PolygonVolume point with the SDK's normal point handles: when it comes
within the distance of a corner of another polygon volume it snaps onto it while you drag,
or onto the closest edge if no corner is that close. A note next to the toggle says what it
snapped to. Keep dragging past the distance and it lets go. Hold **Alt** to drag without
snapping. Only points you drag with the mouse are snapped; undo/redo and values typed in the
Inspector are left alone. The toggle and distance are remembered.

Mouse wheel zooms. Right or middle drag (or left drag on empty space) pans. Both views move
together.

**How points are lined up.** Volumes are handled top-down in scene tree order. The first
volume never moves. Each later volume's points move onto the closest corner of a volume
above it within the snap distance (10 cm by default), or slide onto its closest edge if no
corner is that close. A point is never moved twice, so scanning again after Finalize finds
nothing new. Your choices count as fixed: if you undo or drag a point, the volumes below
it line up with where you put it. Only point X/Z positions change; heights, node positions
and everything else stay as they are. Put the volumes you trust most near the top of the
scene tree.

Gaps under half a millimetre already count as lined up, because Godot stores positions as
32-bit numbers and shared corners can differ by that much.

Volumes inside instanced scenes that don't have *Editable Children* turned on are skipped,
because Godot wouldn't save changes to them.

Same engine as the standalone *Spatial Poly Snap* exe in this repo, which works on
exported `.spatial.json` files instead.
