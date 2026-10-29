# Porting hqlink into Portal Armory

The `hqlink` package is the whole tool. It has no file, console or UI code.

**Two calls (checklist):**

```go
rep, err := hqlink.Find(spatialBytes)          // rep.HQs[i].Found = the checklist (SpawnerID, VehicleType)
out, rep, err := hqlink.Apply(spatialBytes, pick) // pick: HQ ID -> spawner IDs that stay ticked
```

`rep.PickAll()` gives an all-ticked pick; `hqlink.Link(b)` is Find + Apply(all).

- `out` – the updated spatial (identical bytes to the input when `rep.Changed` is false).
- `rep` – `hqlink.Report`: one `HQResult` per HQ (Status, Added, AlreadyLinked,
  CreatedList, Note), plus `Overlaps` and `AddedCount()`.
- `err` – wraps `hqlink.ErrNotSpatial` when the file isn't a spatial.

`rep.Text(fileName)` gives the same plain-text report the .exe writes.

**In Armory:** `hqlink` already lives in this repo (`github.com/GekkoGear/portal-armory/hqlink`).
The Portal Finalization tab calls `Find`, shows each HQ's `Found` list as ticked checkboxes, then `Apply`, next to Spatial Poly Snap's `polysnap.Process`, and
uses `hqlink.OutputNames` for folder/file names. Render the `Report` fields in the web view (or show `Text()`),
and save `out` using Armory's normal output-folder rules.

**Order with Poly Snap:** either order works. `Link` only edits `VehicleSpawners`
and `linked`; Poly Snap only edits polygon points. Running Link first means
it sees the same polygons Godot shows.

**Real-map tests** (`realmaps_test.go`) need local copies; they're skipped unless
`HQLINK_DEADSEAS`, `HQLINK_BADLANDS`, `HQLINK_GULF` point at files. Never commit maps.
