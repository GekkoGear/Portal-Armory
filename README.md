# Portal Armory

A tool to help with mundane tasks to help you get Battlefield 6 Portal projects out faster.

Portal Armory v2 is in development. Its **Portal Finalization** tab is built on the
`polysnap` package in this repo, which also ships on its own as **Spatial Poly Snap**.

## Spatial Poly Snap

Drag one or more `.spatial.json` exports onto `Spatial Poly Snap.exe`. For each file it makes a
folder named after the spatial on your Desktop containing:

| File | What it is |
|---|---|
| `<name>.spatial.json` | Your spatial with the polygon volume points lined up |
| `<name>.spatial.minified.json` | The same file minified - upload this one to Portal |
| `<name> - Snap Report.txt` | What changed, and anything you should fix by hand |

**How points are lined up.** Volumes are handled top-down in file (scene tree) order. The first
volume never moves. Each later volume's points move onto the closest corner of a volume above it
within 10 cm, or, if there is no corner that close, slide straight onto the closest edge. A point
is never moved twice, so a second run changes nothing. Only the X/Z numbers of moved points
change; heights and every other byte of the file stay exactly as they were. Put the volumes you
trust most (like the combat area) near the top of the scene.

**What the report flags but leaves alone:** corners 10-50 cm apart (near misses), corners of a
higher volume sitting on a lower volume's edge, volumes with crossing edges or stacked points,
and anything that failed a safety check (in which case no spatial is saved).

**Minifying.** The minified file follows the rules of defanz0r's JsonMinifier with its default
settings (short names/ids with links updated, editor-only `metadata/*` and `color` removed,
positions rounded to 4 decimals, right/up/front to 6, points to 3), and was checked to produce
byte-for-byte the same output as his tool. Thanks to defanz0r for the original minifier.

Everything runs offline.

## Building

Requires Go 1.24+.

```
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "Spatial Poly Snap.exe" ./cmd/spatial-poly-snap
go test ./...
```

## Using the engine from Go

```go
res := polysnap.Process(data, "Map.spatial.json")
if res.Snapped == nil { /* res.Failed says why; res.ReportText() explains */ }
// res.Snapped, res.Minified, res.Summary(), res.Sections(), res.ReportText()
folder, snappedName, minifiedName, reportName := polysnap.OutputNames("Map.spatial.json")
```
