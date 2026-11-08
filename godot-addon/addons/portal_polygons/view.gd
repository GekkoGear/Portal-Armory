@tool
extends Control
## One top-down map view (BEFORE or AFTER). Both views share the panel's camera,
## so zooming or panning one moves the other with it.

var panel = null          # the Polygons panel that owns the data
var is_after := false
var hover_key = null      # Vector2i(volume, point) under the mouse, or null
var _drag_key = null
var _panning := false
var _magnet = null        # world spot the dragged point is snapping to


func _ready() -> void:
	clip_contents = true
	mouse_filter = Control.MOUSE_FILTER_STOP
	size_flags_horizontal = Control.SIZE_EXPAND_FILL
	size_flags_vertical = Control.SIZE_EXPAND_FILL
	custom_minimum_size = Vector2(200, 200)


func w2s(p: Vector2) -> Vector2:
	return size * 0.5 + (p - panel.cam_center) * panel.cam_zoom


func s2w(s: Vector2) -> Vector2:
	return panel.cam_center + (s - size * 0.5) / panel.cam_zoom


func _pts(vi: int) -> PackedVector2Array:
	return panel.after[vi] if is_after else panel.before[vi]


## The point under the mouse (within 9 px). Changed points win over plain ones.
func pick(s: Vector2):
	if panel == null or panel.before.is_empty():
		return null
	var best = null
	var best_d := 9.0
	var best_changed := false
	for vi in range(panel.before.size()):
		var pts := _pts(vi)
		for k in range(pts.size()):
			var d := w2s(pts[k]).distance_to(s)
			if d > 9.0:
				continue
			var key := Vector2i(vi, k)
			var changed: bool = panel.change_state(key) != ""
			if (changed and not best_changed) or (changed == best_changed and d < best_d):
				best = key
				best_d = d
				best_changed = changed
	return best


func _gui_input(event: InputEvent) -> void:
	if panel == null:
		return
	if event is InputEventMouseButton:
		var mb := event as InputEventMouseButton
		if mb.pressed and (mb.button_index == MOUSE_BUTTON_WHEEL_UP or mb.button_index == MOUSE_BUTTON_WHEEL_DOWN):
			var f := 1.25 if mb.button_index == MOUSE_BUTTON_WHEEL_UP else 0.8
			panel.zoom_at(s2w(mb.position), mb.position, size, f)
			accept_event()
		elif mb.button_index == MOUSE_BUTTON_RIGHT or mb.button_index == MOUSE_BUTTON_MIDDLE:
			_panning = mb.pressed
			accept_event()
		elif mb.button_index == MOUSE_BUTTON_LEFT:
			if mb.pressed:
				var k = pick(mb.position)
				if is_after and k != null:
					_drag_key = k
					panel.begin_drag(k)
				else:
					_panning = true
			else:
				if _drag_key != null:
					panel.end_drag(_drag_key)
					_drag_key = null
					_magnet = null
				_panning = false
			accept_event()
	elif event is InputEventMouseMotion:
		var mm := event as InputEventMouseMotion
		if _drag_key != null:
			_magnet = panel.drag_to(_drag_key, s2w(mm.position), Input.is_key_pressed(KEY_ALT))
			accept_event()
		elif _panning:
			panel.pan(-mm.relative / panel.cam_zoom)
			accept_event()
		else:
			var k = pick(mm.position)
			if k != hover_key:
				hover_key = k
			panel.view_hover(k, self, mm.position)


func _notification(what: int) -> void:
	if what == NOTIFICATION_MOUSE_EXIT and panel != null and _drag_key == null:
		hover_key = null
		panel.view_hover(null, self, Vector2.ZERO)


func _draw() -> void:
	var bg := Color(0.09, 0.1, 0.12)
	draw_rect(Rect2(Vector2.ZERO, size), bg)
	if panel == null:
		return
	var font: Font = get_theme_font("font", "Label")
	var fs := 14

	if panel.before.is_empty():
		var msg: String = panel.empty_message
		draw_string(font, Vector2(16, size.y * 0.5), msg, HORIZONTAL_ALIGNMENT_CENTER, size.x - 32, fs, Color(0.75, 0.78, 0.82))
		_draw_title(font, fs)
		return

	_draw_grid(font)
	var view_rect := Rect2(Vector2.ZERO, size).grow(20)

	# Volume outlines.
	for vi in range(panel.before.size()):
		var pts := _pts(vi)
		if pts.size() < 2:
			continue
		var col: Color = panel.vol_color(vi)
		var sp := PackedVector2Array()
		var box := Rect2(w2s(pts[0]), Vector2.ZERO)
		for p in pts:
			var s := w2s(p)
			sp.append(s)
			box = box.expand(s)
		if not box.intersects(view_rect):
			continue
		sp.append(sp[0])
		draw_polyline(sp, col, 1.6, true)
		if panel.cam_zoom > 3.0 or panel.point_count < 2500:
			for s in sp:
				if view_rect.has_point(s):
					draw_circle(s, 2.0, col.lightened(0.4))
		if panel.show_names:
			var label: String = panel.short_names[vi]
			var tw := font.get_string_size(label, HORIZONTAL_ALIGNMENT_LEFT, -1, 12).x
			if box.size.x > tw + 10 and box.size.y > 16:
				var c := Vector2.ZERO
				for s in sp:
					c += s
				c /= sp.size()
				draw_string(font, c - Vector2(tw * 0.5, -4), label, HORIZONTAL_ALIGNMENT_LEFT, -1, 12, col.lightened(0.3))

	# Changed points.
	for key in panel.change_keys:
		var state: String = panel.change_state(key)
		var from: Vector2 = panel.before[key.x][key.y]
		var to: Vector2 = panel.after[key.x][key.y]
		var col: Color = panel.state_color(state)
		var a := w2s(from)
		var b := w2s(to)
		if not view_rect.has_point(a) and not view_rect.has_point(b):
			continue
		var halo := Color(col.r, col.g, col.b, 0.35)
		if not is_after:
			draw_arc(a, 11.0, 0, TAU, 28, halo, 2.0, true)
			draw_arc(a, 5.5, 0, TAU, 20, col, 2.5, true)
		elif state == "undone":
			draw_arc(a, 11.0, 0, TAU, 28, halo, 2.0, true)
			draw_arc(a, 5.5, 0, TAU, 20, col, 2.0, true)
			draw_line(a + Vector2(-4, -4), a + Vector2(4, 4), col, 2.0, true)
		else:
			draw_arc(b, 11.0, 0, TAU, 28, halo, 2.0, true)
			if a.distance_to(b) > 1.5:
				draw_line(a, b, Color(0.75, 0.75, 0.8, 0.8), 1.2, true)
				draw_arc(a, 4.5, 0, TAU, 18, Color(0.75, 0.75, 0.8, 0.8), 1.5, true)
			draw_circle(b, 5.0, col)

	# Hover highlight (shared between both views).
	var hk = panel.hover_key
	if hk != null and hk.x < panel.before.size():
		var hs := w2s(_pts(hk.x)[hk.y])
		draw_arc(hs, 9.0, 0, TAU, 24, Color(1, 1, 1, 0.95), 2.0, true)
		# also outline the volume it belongs to
		var pts := _pts(hk.x)
		var sp := PackedVector2Array()
		for p in pts:
			sp.append(w2s(p))
		sp.append(sp[0])
		draw_polyline(sp, Color(1, 1, 1, 0.55), 3.0, true)

	if _magnet != null and is_after:
		var m := w2s(_magnet)
		draw_line(m + Vector2(-8, 0), m + Vector2(8, 0), Color(0.4, 1, 0.6), 2.0)
		draw_line(m + Vector2(0, -8), m + Vector2(0, 8), Color(0.4, 1, 0.6), 2.0)

	_draw_title(font, fs)


func _draw_title(font: Font, fs: int) -> void:
	var title := "AFTER" if is_after else "BEFORE"
	var tw := font.get_string_size(title, HORIZONTAL_ALIGNMENT_LEFT, -1, fs).x
	draw_rect(Rect2(6, 6, tw + 16, 24), Color(0, 0, 0, 0.55))
	draw_string(font, Vector2(14, 23), title, HORIZONTAL_ALIGNMENT_LEFT, -1, fs, Color(1, 0.62, 0.2) if is_after else Color(0.8, 0.84, 0.9))


func _draw_grid(font: Font) -> void:
	# Pick a grid step that is at least ~70 px on screen.
	var steps := [0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 50.0, 100.0, 200.0, 500.0, 1000.0, 2000.0]
	var step := 1.0
	for s in steps:
		step = s
		if s * panel.cam_zoom >= 70.0:
			break
	var tl := s2w(Vector2.ZERO)
	var br := s2w(size)
	var gc := Color(1, 1, 1, 0.05)
	var x := floorf(tl.x / step) * step
	var guard := 0
	while x <= br.x and guard < 400:
		var sx := w2s(Vector2(x, 0)).x
		draw_line(Vector2(sx, 0), Vector2(sx, size.y), gc, 1.0)
		x += step
		guard += 1
	var z := floorf(tl.y / step) * step
	guard = 0
	while z <= br.y and guard < 400:
		var sy := w2s(Vector2(0, z)).y
		draw_line(Vector2(0, sy), Vector2(size.x, sy), gc, 1.0)
		z += step
		guard += 1
	# Scale bar.
	var px: float = step * panel.cam_zoom
	var y := size.y - 16
	var label := ("%s cm" % str(snappedf(step * 100.0, 0.1))) if step < 1.0 else ("%s m" % str(step))
	draw_rect(Rect2(6, y - 14, px + 70, 24), Color(0, 0, 0, 0.5))
	draw_line(Vector2(12, y), Vector2(12 + px, y), Color(0.9, 0.9, 0.9), 2.0)
	draw_line(Vector2(12, y - 5), Vector2(12, y + 5), Color(0.9, 0.9, 0.9), 2.0)
	draw_line(Vector2(12 + px, y - 5), Vector2(12 + px, y + 5), Color(0.9, 0.9, 0.9), 2.0)
	draw_string(font, Vector2(18 + px, y + 5), label, HORIZONTAL_ALIGNMENT_LEFT, -1, 12, Color(0.9, 0.9, 0.9))
