package polysnap

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"path/filepath"
	"strings"
)

// ---------- embedded font ----------

type fontMeta struct {
	height, ascent int
	x, w, adv      []int
}

//go:embed fontdata/small.png
var smallPNG []byte

//go:embed fontdata/big.png
var bigPNG []byte

type font struct {
	meta  fontMeta
	atlas *image.Gray
}

func loadFont(name string, data []byte) *font {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		b := img.Bounds()
		g = image.NewGray(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				g.Set(x, y, img.At(x, y))
			}
		}
	}
	return &font{fontMetrics[name], g}
}

var fSmall, fBig *font

func fonts() {
	if fSmall == nil {
		fSmall, fBig = loadFont("small", smallPNG), loadFont("big", bigPNG)
	}
}

func (f *font) width(s string) int {
	w := 0
	for _, r := range s {
		if r < 32 || r > 126 {
			r = '?'
		}
		w += f.meta.adv[r-32]
	}
	return w
}

// fit shortens s with "..." so it is at most maxW pixels wide.
func (f *font) fit(s string, maxW int) string {
	if f.width(s) <= maxW {
		return s
	}
	for len(s) > 0 && f.width(s+"...") > maxW {
		s = s[:len(s)-1]
	}
	return s + "..."
}

// ---------- canvas ----------

type canvas struct{ img *image.RGBA }

func (c *canvas) blend(x, y int, col color.RGBA, a float64) {
	if !(image.Point{x, y}.In(c.img.Rect)) || a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	i := c.img.PixOffset(x, y)
	p := c.img.Pix[i : i+4 : i+4]
	ca := a * float64(col.A) / 255
	p[0] = uint8(float64(p[0])*(1-ca) + float64(col.R)*ca)
	p[1] = uint8(float64(p[1])*(1-ca) + float64(col.G)*ca)
	p[2] = uint8(float64(p[2])*(1-ca) + float64(col.B)*ca)
	p[3] = 255
}

func (c *canvas) rect(r image.Rectangle, col color.RGBA) {
	r = r.Intersect(c.img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c.blend(x, y, col, 1)
		}
	}
}

func (c *canvas) frame(r image.Rectangle, col color.RGBA) {
	c.rect(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), col)
	c.rect(image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), col)
	c.rect(image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), col)
	c.rect(image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), col)
}

// disc draws an anti-aliased filled circle.
func (c *canvas) disc(cx, cy, rad float64, col color.RGBA, clip image.Rectangle) {
	for y := int(cy - rad - 1); y <= int(cy+rad+1); y++ {
		for x := int(cx - rad - 1); x <= int(cx+rad+1); x++ {
			if !(image.Point{x, y}.In(clip)) {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			c.blend(x, y, col, rad+0.5-d)
		}
	}
}

// ring draws an anti-aliased circle outline.
func (c *canvas) ring(cx, cy, rad, thick float64, col color.RGBA, clip image.Rectangle) {
	for y := int(cy - rad - thick - 1); y <= int(cy+rad+thick+1); y++ {
		for x := int(cx - rad - thick - 1); x <= int(cx+rad+thick+1); x++ {
			if !(image.Point{x, y}.In(clip)) {
				continue
			}
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - rad)
			c.blend(x, y, col, thick/2+0.5-d)
		}
	}
}

// line draws an anti-aliased line of the given thickness, clipped to clip.
func (c *canvas) line(x0, y0, x1, y1, thick float64, col color.RGBA, clip image.Rectangle) {
	cr := clip
	// Liang-Barsky clip to the (slightly grown) rectangle so huge edges stay cheap.
	minX, minY := float64(cr.Min.X)-thick, float64(cr.Min.Y)-thick
	maxX, maxY := float64(cr.Max.X)+thick, float64(cr.Max.Y)+thick
	dx, dy := x1-x0, y1-y0
	t0, t1 := 0.0, 1.0
	for _, e := range [][2]float64{{-dx, x0 - minX}, {dx, maxX - x0}, {-dy, y0 - minY}, {dy, maxY - y0}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return
			}
			continue
		}
		r := q / p
		if p < 0 {
			t0 = math.Max(t0, r)
		} else {
			t1 = math.Min(t1, r)
		}
	}
	if t0 > t1 {
		return
	}
	ax, ay, bx, by := x0+t0*dx, y0+t0*dy, x0+t1*dx, y0+t1*dy
	half := thick / 2
	lx0, lx1 := int(math.Min(ax, bx)-half-1), int(math.Max(ax, bx)+half+1)
	ly0, ly1 := int(math.Min(ay, by)-half-1), int(math.Max(ay, by)+half+1)
	sx, sy := bx-ax, by-ay
	l2 := sx*sx + sy*sy
	for y := ly0; y <= ly1; y++ {
		for x := lx0; x <= lx1; x++ {
			if !(image.Point{x, y}.In(clip)) {
				continue
			}
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if l2 > 0 {
				t = math.Max(0, math.Min(1, ((px-ax)*sx+(py-ay)*sy)/l2))
			}
			d := math.Hypot(px-(ax+t*sx), py-(ay+t*sy))
			c.blend(x, y, col, half+0.5-d)
		}
	}
}

func (c *canvas) text(f *font, x, y int, s string, col color.RGBA) {
	for _, r := range s {
		if r < 32 || r > 126 {
			r = '?'
		}
		i := int(r - 32)
		gx, gw := f.meta.x[i], f.meta.w[i]
		for yy := 0; yy < f.meta.height; yy++ {
			for xx := 0; xx < gw; xx++ {
				a := f.atlas.GrayAt(gx+xx, yy).Y
				if a > 0 {
					c.blend(x+xx, y+yy, col, float64(a)/255)
				}
			}
		}
		x += f.meta.adv[i]
	}
}

// ---------- preview ----------

var (
	colBG      = color.RGBA{14, 15, 18, 255}
	colPanel   = color.RGBA{24, 26, 31, 255}
	colFrame   = color.RGBA{58, 62, 72, 255}
	colText    = color.RGBA{232, 234, 238, 255}
	colDim     = color.RGBA{150, 156, 168, 255}
	colOrange  = color.RGBA{255, 140, 26, 255}
	colBlue    = color.RGBA{120, 200, 255, 255}
	colGhost   = color.RGBA{190, 190, 200, 160}
	shapeHues  = []float64{200, 32, 140, 280, 55, 340, 100, 240, 0, 175, 310, 75}
	maxTiles   = 60
	previewW   = 1800
	pad        = 28
	tileBox    = 262
	tileGap    = 8
	tileHeader = 40
)

func hsv(h, s, v float64) color.RGBA {
	h = math.Mod(h, 360) / 60
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch int(i) {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}

func shapeColor(i int) color.RGBA {
	return hsv(shapeHues[i%len(shapeHues)]+float64(i/len(shapeHues))*17, 0.55, 0.95)
}

type spot struct {
	center  Pt
	changes []int
	maxMove float64
}

// Spots groups the changes into numbered places on the map (changes within
// half a meter of each other share a spot). The numbers match the preview image.
func (r *Result) spots() []spot {
	var out []spot
	for i, c := range r.Changes {
		placed := false
		for s := range out {
			if math.Hypot(out[s].center.X-c.To.X, out[s].center.Z-c.To.Z) <= 0.5 {
				out[s].changes = append(out[s].changes, i)
				placed = true
				break
			}
		}
		if !placed {
			out = append(out, spot{center: c.To, changes: []int{i}})
		}
	}
	for s := range out {
		var sx, sz float64
		for _, i := range out[s].changes {
			sx += r.Changes[i].To.X
			sz += r.Changes[i].To.Z
			out[s].maxMove = math.Max(out[s].maxMove, r.Changes[i].Distance)
		}
		n := float64(len(out[s].changes))
		out[s].center = Pt{sx / n, sz / n}
	}
	return out
}

// view maps world X/Z into a pixel rectangle (X to the right, Z downward, as in
// Godot's top view).
type view struct {
	box        image.Rectangle
	cx, cz, sc float64
}

func (v view) px(p Pt) (float64, float64) {
	return float64(v.box.Min.X+v.box.Max.X)/2 + (p.X-v.cx)*v.sc, float64(v.box.Min.Y+v.box.Max.Y)/2 + (p.Z-v.cz)*v.sc
}

func (c *canvas) shapes(r *Result, v view, after bool, thick float64) {
	for si, sh := range r.Shapes {
		pts := sh.Before
		if after {
			pts = sh.After
		}
		col := shapeColor(si)
		for k := range pts {
			x0, y0 := v.px(pts[k])
			x1, y1 := v.px(pts[(k+1)%len(pts)])
			c.line(x0, y0, x1, y1, thick, col, v.box)
		}
	}
}

func niceLength(maxMeters float64) float64 {
	steps := []float64{0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000}
	best := steps[0]
	for _, s := range steps {
		if s <= maxMeters {
			best = s
		}
	}
	return best
}

func lengthLabel(m float64) string {
	if m < 1 {
		return fmt.Sprintf("%g cm", math.Round(m*1000)/10)
	}
	return fmt.Sprintf("%g m", m)
}

func (c *canvas) scaleBar(v view) {
	l := niceLength(float64(v.box.Dx()) / 4 / v.sc)
	w := l * v.sc
	x := float64(v.box.Min.X + 10)
	y := float64(v.box.Max.Y - 14)
	c.line(x, y, x+w, y, 2, colText, v.box)
	c.line(x, y-4, x, y+4, 2, colText, v.box)
	c.line(x+w, y-4, x+w, y+4, 2, colText, v.box)
	lbl := lengthLabel(l)
	c.rect(image.Rect(int(x)-6, int(y)-12, int(x+w)+fSmall.width(lbl)+12, int(y)+10), color.RGBA{24, 26, 31, 210})
	c.line(x, y, x+w, y, 2, colText, v.box)
	c.line(x, y-4, x, y+4, 2, colText, v.box)
	c.line(x+w, y-4, x+w, y+4, 2, colText, v.box)
	c.text(fSmall, int(x+w)+6, int(y)-9, lbl, colText)
}

// RenderPreview draws a top-down, side-by-side before/after picture of the
// changes as a PNG. The top shows the whole map with every changed spot
// numbered; below, each numbered spot gets a close-up (before | after).
func RenderPreview(r *Result) ([]byte, error) {
	fonts()
	spots := r.spots()
	tiles := len(spots)
	if tiles > maxTiles {
		tiles = maxTiles
	}
	tileW := tileBox*2 + tileGap
	cols := (previewW - pad) / (tileW + pad)
	rows := (tiles + cols - 1) / cols
	panel := (previewW - 3*pad) / 2
	headerH := 96
	overviewTop := headerH + 34
	tilesTop := overviewTop + panel + 70
	height := tilesTop + rows*(tileHeader+tileBox+pad) + 40
	if tiles == 0 {
		height = tilesTop
	}

	c := &canvas{image.NewRGBA(image.Rect(0, 0, previewW, height))}
	c.rect(c.img.Rect, colBG)

	// Header.
	c.text(fBig, pad, 20, fBig.fit("Spatial Poly Snap - "+filepath.Base(r.File), previewW-2*pad), colText)
	c.text(fSmall, pad, 54, fmt.Sprintf("%d points moved (%d onto corners, %d onto edges) at %d spots. Top-down view, X to the right. Left: before. Right: after.",
		r.Moved, r.CornerSnaps, r.EdgeSnaps, len(spots)), colDim)
	c.text(fSmall, pad, 74, "Numbered rings mark changed spots; each is shown up close below. Moved points: orange. Grey ring + line in AFTER = where the point was.", colDim)

	// Overview: fit every shape.
	minX, maxX, minZ, maxZ := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, sh := range r.Shapes {
		for _, p := range append(append([]Pt{}, sh.Before...), sh.After...) {
			minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
			minZ, maxZ = math.Min(minZ, p.Z), math.Max(maxZ, p.Z)
		}
	}
	if math.IsInf(minX, 0) {
		minX, maxX, minZ, maxZ = -1, 1, -1, 1
	}
	span := math.Max(math.Max(maxX-minX, maxZ-minZ), 1)
	for side := 0; side < 2; side++ {
		box := image.Rect(pad+side*(panel+pad), overviewTop, pad+side*(panel+pad)+panel, overviewTop+panel)
		c.rect(box, colPanel)
		c.frame(box, colFrame)
		title := "BEFORE"
		if side == 1 {
			title = "AFTER"
		}
		c.text(fBig, box.Min.X, overviewTop-30, title, colText)
		v := view{box, (minX + maxX) / 2, (minZ + maxZ) / 2, float64(panel-40) / span}
		c.shapes(r, v, side == 1, 1.6)
		for i, s := range spots {
			x, y := v.px(s.center)
			c.ring(x, y, 9, 2.5, colOrange, box)
			if i < tiles {
				c.text(fSmall, int(x)+11, int(y)-20, fmt.Sprint(i+1), colOrange)
			}
		}
		c.scaleBar(v)
	}

	if tiles == 0 {
		var buf bytes.Buffer
		err := png.Encode(&buf, c.img)
		return buf.Bytes(), err
	}

	title := "CLOSE-UPS (before | after)"
	if len(spots) > tiles {
		title += fmt.Sprintf(" - first %d of %d spots shown; the rest are listed in the report", tiles, len(spots))
	}
	c.text(fBig, pad, tilesTop-40, title, colText)

	for t := 0; t < tiles; t++ {
		s := spots[t]
		tx := pad + (t%cols)*(tileW+pad)
		ty := tilesTop + (t/cols)*(tileHeader+tileBox+pad)

		// Header: number + what moved.
		c.text(fBig, tx, ty, fmt.Sprintf("%d", t+1), colOrange)
		var head, sub string
		if len(s.changes) == 1 {
			ch := r.Changes[s.changes[0]]
			head = fmt.Sprintf("%s pt %d", r.Shapes[ch.Shape].Name, ch.Point+1)
			kind := "corner"
			if ch.ToEdge {
				kind = "edge"
			}
			sub = fmt.Sprintf("moved %s onto %s: %s", lengthLabel(ch.Distance), kind, ch.Onto)
		} else {
			names := map[string]bool{}
			var list []string
			for _, i := range s.changes {
				n := r.Shapes[r.Changes[i].Shape].Name
				if !names[n] {
					names[n] = true
					list = append(list, n)
				}
			}
			head = fmt.Sprintf("%d points moved (up to %s)", len(s.changes), lengthLabel(s.maxMove))
			sub = strings.Join(list, ", ")
		}
		nx := tx + fBig.width(fmt.Sprint(t+1)) + 10
		c.text(fSmall, nx, ty+1, fSmall.fit(head, tileW-(nx-tx)), colText)
		c.text(fSmall, nx, ty+19, fSmall.fit(sub, tileW-(nx-tx)), colDim)

		// Zoom so every moved point in the spot fits with room around it.
		reach := 0.0
		for _, i := range s.changes {
			ch := r.Changes[i]
			reach = math.Max(reach, math.Hypot(ch.From.X-s.center.X, ch.From.Z-s.center.Z))
			reach = math.Max(reach, math.Hypot(ch.To.X-s.center.X, ch.To.Z-s.center.Z))
		}
		half := math.Max(0.2, reach*2.2)
		for side := 0; side < 2; side++ {
			box := image.Rect(tx+side*(tileBox+tileGap), ty+tileHeader, tx+side*(tileBox+tileGap)+tileBox, ty+tileHeader+tileBox)
			c.rect(box, colPanel)
			c.frame(box, colFrame)
			v := view{box, s.center.X, s.center.Z, float64(tileBox) / (2 * half)}
			after := side == 1
			c.shapes(r, v, after, 2.2)
			// all corners in view
			for _, sh := range r.Shapes {
				pts := sh.Before
				if after {
					pts = sh.After
				}
				for _, p := range pts {
					x, y := v.px(p)
					c.disc(x, y, 2.5, colBlue, box)
				}
			}
			for _, i := range s.changes {
				ch := r.Changes[i]
				fx, fy := v.px(ch.From)
				if after {
					x, y := v.px(ch.To)
					c.ring(fx, fy, 5, 1.5, colGhost, box)
					c.line(fx, fy, x, y, 1.2, colGhost, box)
					c.disc(x, y, 5, colOrange, box)
				} else {
					c.ring(fx, fy, 6, 2.5, colOrange, box)
				}
			}
			label := "BEFORE"
			if after {
				label = "AFTER"
			}
			c.rect(image.Rect(box.Min.X+1, box.Min.Y+1, box.Min.X+fSmall.width(label)+14, box.Min.Y+24), color.RGBA{24, 26, 31, 210})
			c.text(fSmall, box.Min.X+8, box.Min.Y+6, label, colDim)
			c.scaleBar(v)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, c.img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
