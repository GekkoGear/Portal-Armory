# HQ Vehicle Linker Implementation Plan

> **For agentic workers:** Implement this plan task-by-task, test first, committing after each task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A drag-and-drop offline Windows .exe that adds every vehicle spawner inside an HQ's `HQArea` polygon to that HQ's `VehicleSpawners` array, with its core ready to drop into Portal Armory.

**Architecture:** A pure Go package `hqlink` takes spatial bytes and returns new bytes plus a `Report`; it never touches files or the console. It parses the JSON once to get HQs, spawners and polygons, then edits the original bytes in place (only inside `VehicleSpawners` arrays), so key order, indentation and minification stay exactly as they were. A thin `cmd/hqvehiclelinker` wrapper handles the dropped file, the output folder, the report file and "press Enter to close".

**Tech Stack:** Go 1.24, standard library only plus `golang.org/x/sys/windows` (Desktop known-folder lookup). Cross-compiled with `GOOS=windows GOARCH=amd64`.

**Spec:** `docs/design.md` (HQ Vehicle Linker — Design, approved 2026-10-29; slot-creation + shared-polygon rules revised same day after testing Andy's maps)

> **Revision (2026-10-29):** where tasks below mention `SkippedNoVehicles` or "requires at least one existing element", read the Global Constraints above instead: status is `NothingFound`, and the splice also handles creating a new array (copying the formatting of `InfantrySpawns`) and empty arrays.

## Global Constraints

- Offline: no network calls at runtime.
- Add only: never remove or reorder existing `VehicleSpawners` entries; never add a duplicate.
- Top-down (X/Z) inside test only; Y and polygon `height` are ignored. A point on an edge or vertex counts as inside.
- An HQ with nothing new inside its polygon is left completely alone. An HQ with no `VehicleSpawners` key gets one created after `InfantrySpawns`, and `"VehicleSpawners"` appended to `linked`.
- A spawner inside two or more *different* HQ polygons is added to none of them and reported; HQs sharing one polygon all get its spawners.
- Nothing outside the edited `VehicleSpawners` arrays may change by a single byte; minified input stays minified, pretty input keeps its layout.
- The dropped original file is never written to.
- Only `Portal_Dynamic` is scanned (`Static` holds no HQs or spawners in either test map).
- Core package `hqlink`: no `os`, `fmt.Print*`, or file paths inside it.

## Review Focus

1. **Windows CRLF line endings** — a spatial saved with `\r\n` should get new entries with `\r\n` too, not mixed endings. (Task 3 test `TestSpliceCRLF`.)
2. **UTF-8 BOM at file start** — Godot/editors sometimes add one; parsing must not fail, and the BOM must still be there in the output. (Task 4 test `TestLinkKeepsBOM`.)
3. **Desktop redirected to OneDrive** — output must land on the Desktop the user actually sees. (Task 6: known-folder lookup, fallback test `TestOutputDirFallback`.)
4. **Wrong thing dropped** (not JSON, no `Portal_Dynamic`, multiple files, nothing dropped) — clear message, no crash, window stays open. (Task 6 test `TestRunBadInput`.)
5. **Spawner ids with `&`, `<`, quotes or non-ASCII** — written exactly as Godot would (`&` not `\u0026`). (Task 3 test `TestSpliceEscaping`.)

---

## File Structure

```
hq-vehicle-linker/
  go.mod                         module github.com/GekkoGear/hq-vehicle-linker
  hqlink/
    polygon.go                   insideXZ
    model.go                     parse Portal_Dynamic into HQs / spawners / polygons
    locate.go                    find byte spans of each HQ's VehicleSpawners array
    splice.go                    insert new entries into a span, copying its formatting
    link.go                      Link(): the decision logic + Report types
    report.go                    Report.Text()
    *_test.go, testdata/*.json   small synthetic spatials (pretty, minified, CRLF)
  cmd/hqvehiclelinker/
    main.go                      drag-drop wrapper
    output.go                    desktop + output folder naming
    main_test.go
  docs/design.md, docs/PORTING.md
```

Dead Seas is never committed (the Armory repo is public). `hqlink/deadseas_test.go` reads `HQLINK_DEADSEAS` env var and `t.Skip`s when unset.

---

### Task 1: Module + polygon test

**Files:** Create `go.mod`, `hqlink/polygon.go`, `hqlink/polygon_test.go`

**Interfaces:**
- Produces: `type Point struct{ X, Z float64 }`; `func insideXZ(x, z float64, poly []Point) bool` (false when `len(poly) < 3`).

- [ ] **Step 1: Write failing tests** `TestInsideSquare` (10×10 square: (5,5) true, (15,5) false), `TestInsideConcaveL` (L-shape: point in the notch false, point in each arm true), `TestOnEdgeAndVertex` ((0,5) true, (0,0) true), `TestTooFewPoints` (2 points → false).
- [ ] **Step 2: Run** `go test ./hqlink -run Inside\|Edge\|TooFew` → FAIL (undefined: insideXZ).
- [ ] **Step 3: Implement** ray-casting even-odd test, with an explicit point-on-segment check first (collinear and within the segment's bounding box, tolerance 1e-6) returning true.
- [ ] **Step 4: Run** same command → PASS.
- [ ] **Step 5: Commit** `feat: top-down point-in-polygon`

### Task 2: Parse the spatial

**Files:** Create `hqlink/model.go`, `hqlink/model_test.go`, `hqlink/testdata/small_pretty.json`

`small_pretty.json` (4-space indent, keys in Godot export order with `id` after `VehicleSpawners`): HQ_A (HQArea → PolyA, VehicleSpawners [VA1]), HQ_B (HQArea → PolyB, VehicleSpawners [VB1]), HQ_Empty (HQArea → PolyA, VehicleSpawners []), HQ_NoArea (no HQArea, VehicleSpawners [VA1]), HQ_BadArea (HQArea → "Missing/Poly", VehicleSpawners [VA1]); PolyA square (0..100), PolyB square (200..300); spawners VA1 (50,50), VA2 (60,60), VB1 (250,250), VB2 (260,260), VOut (500,500); plus a few unrelated objects (a SpawnPoint, an AreaTrigger) so the walker has to skip them.

**Interfaces:**
- Produces:
  ```go
  type hq struct{ ID, Name, AreaID string; Vehicles []string }
  type spawner struct{ ID string; X, Z float64 }
  type spatial struct {
      HQs      []hq                 // file order
      Spawners []spawner            // file order
      Polys    map[string][]Point   // PolygonVolume id -> points
  }
  func parseSpatial(b []byte) (spatial, error)
  ```
  Error `ErrNotSpatial` (exported) when there's no `Portal_Dynamic` array.

- [ ] **Step 1: Failing tests** `TestParseSmall` (5 HQs in order; HQ_A.AreaID == "PolyA", Vehicles == ["VA1"]; 5 spawners; Polys has PolyA/PolyB with 4 points), `TestParseNotSpatial` (`{"foo":1}` and `not json` → `errors.Is(err, ErrNotSpatial)` / non-nil).
- [ ] **Step 2: Run** `go test ./hqlink -run Parse` → FAIL.
- [ ] **Step 3: Implement** with `encoding/json` into lightweight structs (`type`, `id`, `name`, `HQArea`, `VehicleSpawners`, `position.x/z`, `points[].x/z`); strip a leading UTF-8 BOM before unmarshalling.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: parse HQs, spawners, polygons`

### Task 3: Locate and splice arrays

**Files:** Create `hqlink/locate.go`, `hqlink/splice.go`, `hqlink/locate_test.go`, `hqlink/splice_test.go`, `hqlink/testdata/small_min.json` (minified twin of `small_pretty.json`), `hqlink/testdata/small_crlf.json` (CRLF twin).

**Interfaces:**
- Produces:
  ```go
  type arraySpan struct{ Open, Close int } // byte index of '[' and ']'
  func findVehicleArrays(b []byte) (map[string]arraySpan, error) // key = HQ id
  func spliceArrays(b []byte, adds map[string][]string, spans map[string]arraySpan) []byte
  ```

`findVehicleArrays` walks `Portal_Dynamic` with `json.Decoder.Token()` and `InputOffset()`, tracking per object the span of its `VehicleSpawners` array and its `id` string (which may come after the array), and records the span when the object closes. Only objects whose `type` is `HQ_PlayerSpawner`.

`spliceArrays` (requires at least one existing element — guaranteed by the skip rule): separator = `","` + the exact whitespace between `[` and the first element; insertion point = just after the last element's closing quote. Apply edits from the highest offset down so earlier offsets stay valid. Each id encoded with a `json.Encoder` with `SetEscapeHTML(false)`, trailing newline trimmed.

- [ ] **Step 1: Failing tests**
  - `TestFindArraysPrettyAndMin`: both fixtures → span for HQ_A where `b[Open]=='['`, `b[Close]==']'`, and the text between contains `"VA1"`.
  - `TestSplicePretty`: add `VA2` to HQ_A → result equals the input with exactly `,\n` + original indent + `"VA2"` inserted after `"VA1"`; `json.Valid` true.
  - `TestSpliceMinified`: same on `small_min.json` → inserted text is exactly `,"VA2"`; output has no `\n`.
  - `TestSpliceCRLF`: on `small_crlf.json` → output contains no bare `\n` (every `\n` preceded by `\r`).
  - `TestSpliceEscaping`: id `Veh & "Boat" é` → output contains `"Veh & \"Boat\" é"` literally.
- [ ] **Step 2: Run** `go test ./hqlink -run Find\|Splice` → FAIL.
- [ ] **Step 3: Implement** both functions as above.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: format-preserving VehicleSpawners splice`

### Task 4: Link (decision logic)

**Files:** Create `hqlink/link.go`, `hqlink/link_test.go`, `hqlink/deadseas_test.go`

**Interfaces:**
- Consumes: `parseSpatial`, `insideXZ`, `findVehicleArrays`, `spliceArrays`.
- Produces:
  ```go
  type HQStatus string
  const (
      Linked            HQStatus = "linked"
      SkippedNoVehicles HQStatus = "skipped: no vehicle spawners"
      SkippedNoArea     HQStatus = "skipped: no HQArea"
      SkippedBadArea    HQStatus = "skipped: HQArea polygon not found"
  )
  type HQResult struct {
      ID, Name      string
      Status        HQStatus
      Added         []string
      AlreadyLinked []string // spawners inside the polygon that were already in the array
  }
  type Overlap struct{ SpawnerID string; HQIDs []string }
  type Report struct {
      HQs      []HQResult // file order
      Overlaps []Overlap  // file order of spawner
      Changed  bool
  }
  func Link(spatial []byte) ([]byte, Report, error)
  ```

Order of rules per HQ: no vehicles → `SkippedNoVehicles`; no `HQArea` → `SkippedNoArea`; area id not in `Polys` (or < 3 points) → `SkippedBadArea`; else `Linked`. Overlaps are computed across `Linked` HQs only; an overlapping spawner is excluded from every HQ's `Added` (but stays in `AlreadyLinked` if it already was). When nothing is added, return the input bytes unchanged (same slice contents) and `Changed=false`. A leading BOM is kept.

- [ ] **Step 1: Failing tests** (on `small_pretty.json`)
  - `TestLinkAddsInside`: HQ_A.Added == ["VA2"], AlreadyLinked == ["VA1"]; HQ_B.Added == ["VB2"]; `VOut` added nowhere.
  - `TestLinkSkips`: HQ_Empty → SkippedNoVehicles and its array is still `[]` in output; HQ_NoArea → SkippedNoArea; HQ_BadArea → SkippedBadArea.
  - `TestLinkNoDuplicates`: running `Link` on its own output → `Changed == false`, bytes identical.
  - `TestLinkOverlap`: fixture variant where PolyB is widened to x/z 55..300 (covers VA2 at 60,60 but not VA1 at 50,50) → VA2 in no `Added`, `Overlaps == [{VA2, [HQ_A, HQ_B]}]`.
  - `TestLinkKeepsBOM`: BOM + fixture → output starts with BOM, rest valid JSON.
  - `TestLinkOnlyArraysChange`: strip every `VehicleSpawners` array span from input and output → remaining bytes identical (pretty and minified fixtures).
  - `TestDeadSeas` (skips without `HQLINK_DEADSEAS`): real file → `Changed == false`, both HQs `Linked` with 13 AlreadyLinked each. Second case: remove the 7 boat ids from both arrays in memory → `Link` re-adds exactly those 7 per HQ.
- [ ] **Step 2: Run** `go test ./hqlink -run Link` → FAIL.
- [ ] **Step 3: Implement** `Link`.
- [ ] **Step 4: Run** `go test ./hqlink` and `HQLINK_DEADSEAS=<copy path> go test ./hqlink -run DeadSeas -v` → PASS (DeadSeas not skipped).
- [ ] **Step 5: Commit** `feat: Link adds HQ-area spawners`

### Task 5: Report text

**Files:** Create `hqlink/report.go`, `hqlink/report_test.go`

**Interfaces:** Produces `func (r Report) Text(inputName string) string` (plain text, `\r\n` line endings for Notepad).

Layout: title line `HQ Vehicle Linker report — <inputName>`; then one block per HQ: `<Name> (<ID>)`, status, `Added (n):` list, `Already linked (n):` list; then `Spawners inside more than one HQ polygon (not added):` with each spawner and its HQs, or `None`; last line `No changes were needed.` when `!Changed`.

- [ ] **Step 1: Failing test** `TestReportText`: Report from Task 4's small fixture → contains `Added (1):`, `VA2`, `skipped: no vehicle spawners`, `\r\n`, and no bare `\n`.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** → PASS.
- [ ] **Step 5: Commit** `feat: plain-text report`

### Task 6: Drag-and-drop .exe + Armory port notes

**Files:** Create `cmd/hqvehiclelinker/main.go`, `cmd/hqvehiclelinker/output.go`, `cmd/hqvehiclelinker/main_test.go`, `docs/PORTING.md`

**Interfaces:**
- Consumes: `hqlink.Link`, `hqlink.Report.Text`, `hqlink.ErrNotSpatial`.
- Produces:
  ```go
  func desktopDir() string                       // Windows FOLDERID_Desktop via x/sys/windows; fallback filepath.Join(home, "Desktop")
  func outputDir(desktop, inputPath string) string // <desktop>/<spatial base name> - HQ Vehicles
  func run(args []string, desktop string, out io.Writer) int // exit code; no pause inside
  ```
  Base name strips `.json`, then `.spatial` / `.spatial.minified`. Output file keeps the original file name; report is `HQ Vehicle Linker report.txt`. `main` calls `run` then waits for Enter.

- [ ] **Step 1: Failing tests**
  - `TestOutputDir`: `("C:\\D", "x\\DeadSeas Overcast.spatial.json")` → `C:\D\DeadSeas Overcast - HQ Vehicles` (use `filepath.Join` for expectations).
  - `TestOutputDirFallback`: `desktopDir()` on non-Windows returns `<home>/Desktop`.
  - `TestRunWritesOutput`: temp desktop + copy of `small_pretty.json` → exit 0, output file + report exist, original file bytes unchanged.
  - `TestRunBadInput`: no args → exit 2 and message contains `Drag a .spatial.json`; two args → exit 2; a text file `hello` → exit 1, message contains `not a spatial`; nothing written.
- [ ] **Step 2: Run** `go test ./cmd/...` → FAIL.
- [ ] **Step 3: Implement** `output.go` (with `//go:build windows` / `!windows` split for `desktopDir`) and `main.go`. Console summary: one line per HQ (`Name: +n added` or status), overlap count, output folder path.
- [ ] **Step 4: Run** `go test ./...` → PASS; `GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "HQ Vehicle Linker.exe" ./cmd/hqvehiclelinker` → builds.
- [ ] **Step 5: Write `docs/PORTING.md`**: import path `.../hqlink`, the single call `Link(bytes) ([]byte, Report, error)`, that Armory renders `Report` fields directly (or `Text()`), and that the package has no file/console access, so it plugs into the Portal Finalization tab next to Spatial Poly Snap. ~20 lines.
- [ ] **Step 6: Final check** — run the Windows .exe logic once more via `go run ./cmd/hqvehiclelinker` on the Dead Seas copy (temp desktop) and diff input vs output: identical; report says `No changes were needed.`
- [ ] **Step 7: Commit** `feat: drag-and-drop exe + porting notes`
