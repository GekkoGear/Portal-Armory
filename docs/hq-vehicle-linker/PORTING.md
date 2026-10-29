# Porting hqlink into Portal Armory

The `hqlink` package is the whole tool. It has no file, console or UI code.

**One call:**

```go
out, rep, err := hqlink.Link(spatialBytes)
```

- `out` – the updated spatial (identical bytes to the input when `rep.Changed` is false).
- `rep` – `hqlink.Report`: one `HQResult` per HQ (Status, Added, AlreadyLinked,
  CreatedList, Note), plus `Overlaps` and `AddedCount()`.
- `err` – wraps `hqlink.ErrNotSpatial` when the file isn't a spatial.

`rep.Text(fileName)` gives the same plain-text report the .exe writes.

**In Armory:** `hqlink` already lives in this repo (`github.com/GekkoGear/portal-armory/hqlink`).
The Portal Finalization tab calls `Link` next to Spatial Poly Snap's `polysnap.Process`, and
uses `hqlink.OutputNames` for folder/file names. Render the `Report` fields in the web view (or show `Text()`),
and save `out` using Armory's normal output-folder rules.

**Order with Poly Snap:** either order works. `Link` only edits `VehicleSpawners`
and `linked`; Poly Snap only edits polygon points. Running Link first means
it sees the same polygons Godot shows.

**Real-map tests** (`realmaps_test.go`) need local copies; they're skipped unless
`HQLINK_DEADSEAS`, `HQLINK_BADLANDS`, `HQLINK_GULF` point at files. Never commit maps.
