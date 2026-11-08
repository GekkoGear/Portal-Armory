@tool
extends Node
## The animated PolySnap tab icon.
##
## Idle: the open map from above - its play area (the CombatArea's CombatVolume)
## in white with 14 red points, Team 1's starting HQ in blue and Team 2's in
## orange. With no map open it shows a fixed random-looking shape.
## Hover: the HQs fade out and the 14 points drag one after another into a "P".
## Mouse leaves: they drag back. The tab shows only the icon (no text).
##
## Maps are drawn from above like Godot's 3D top view, turned by the same Rotate
## setting as the PolySnap tab (never mirrored).

const PolySnapPanel = preload("polysnap_panel.gd")

## The "P": one outline that goes in to trace the bowl's hole (16 x 16 icon grid).
const P := [Vector2(3, 14), Vector2(3, 2), Vector2(9, 2), Vector2(12, 3.4), Vector2(13.2, 6), Vector2(12, 8.6), Vector2(9, 10),
	Vector2(6, 10), Vector2(6, 4.6), Vector2(8.6, 4.6), Vector2(10, 6), Vector2(8.6, 7.4), Vector2(6, 7.4), Vector2(6, 14)]
## Order the points peel off in, like being dragged one after another.
const ORDER := [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 0, 13]
const STAGGER := 0.035
const MOVE := 0.42
const COL_PLAY := Color(0.96, 0.96, 0.96)
const COL_P := Color(0.553, 0.647, 0.953)
const COL_T1 := Color(0.35, 0.67, 1.0)
const COL_T2 := Color(1.0, 0.59, 0.16)
const COL_DOT := Color(1.0, 0.373, 0.373)

var plugin: EditorPlugin = null

var _button: Button = null
var _home: Array = []       # 14 Vector2, idle positions (icon grid)
var _hqs: Array = []        # [[team:int, PackedVector2Array (icon grid)]]
var _hovered := false
var _since := 99.0          # seconds since hover changed
var _last_key := ""
var _cache := {}
var _root_seen = null
var _refresh_in := -1.0
var _from_map := false      # true once the open map's play area was found
var _retry_left := 0.0      # keep rescanning this many more seconds while a map is loading
var _scan_nodes := -1       # node count at the last scan
var _orig := {}             # the tab's own look, put back when the map icon is turned off

## Off (the default): the tab is the normal "PolySnap" tab. On: the tab shows the
## animated map icon instead. Turned on with the "Map icon" box in the PolySnap tab.
var map_icon := false


func _ready() -> void:
	set_process(true)
	load_setting()
	if map_icon:
		refresh_map()


func load_setting() -> void:
	if plugin != null:
		var es = plugin.get_editor_interface().get_editor_settings()
		if es.has_setting("polysnap/tab/map_icon"):
			map_icon = bool(es.get_setting("polysnap/tab/map_icon"))


func set_map_icon(on: bool) -> void:
	if on == map_icon:
		return
	map_icon = on
	if plugin != null:
		plugin.get_editor_interface().get_editor_settings().set_setting("polysnap/tab/map_icon", on)
	if on:
		_button = null              # hooked again on the next frame
		_root_seen = null
		refresh_map()
	else:
		restore()
		_button = null


## Find this add-on's tab button among the editor's main screen buttons.
func _find_button() -> Button:
	if plugin == null:
		return null
	var base: Control = plugin.get_editor_interface().get_base_control()
	for b in base.find_children("*", "Button", true, false):
		if b.text == "PolySnap" or b.get_meta("polysnap_tab", false):
			# main screen buttons are toggle buttons in the title bar
			if b.toggle_mode:
				return b
	return null


func _hook_button() -> void:
	_button = _find_button()
	if _button == null:
		return
	_button.set_meta("polysnap_tab", true)
	if _orig.is_empty():
		_orig = {"icon": _button.icon, "clip": _button.clip_text, "expand": _button.expand_icon,
			"font_size": _button.get_theme_font_size("font_size") if _button.has_theme_font_size_override("font_size") else -1}
	# Never change the button's text: Godot finds this tab by its text ("PolySnap")
	# when it switches tabs or reloads the plugin. The text is only hidden: clipped
	# to nothing and drawn see-through, so the tab shows just the icon.
	_button.clip_text = true
	_button.add_theme_font_size_override("font_size", 1)
	for c in ["font_color", "font_hover_color", "font_pressed_color", "font_hover_pressed_color", "font_focus_color", "font_disabled_color", "font_outline_color"]:
		_button.add_theme_color_override(c, Color(0, 0, 0, 0))
	_button.tooltip_text = "PolySnap"
	_button.expand_icon = false
	# keep our colors: don't let the editor tint the icon when hovered or selected
	for c in ["icon_normal_color", "icon_hover_color", "icon_pressed_color", "icon_hover_pressed_color", "icon_focus_color", "icon_disabled_color"]:
		_button.add_theme_color_override(c, Color.WHITE)
	if not _button.mouse_entered.is_connected(_on_enter):
		_button.mouse_entered.connect(_on_enter)
		_button.mouse_exited.connect(_on_exit)
	_last_key = ""


func _on_enter() -> void:
	refresh_map()
	_hovered = true
	_since = 0.0


func _on_exit() -> void:
	_hovered = false
	_since = 0.0


## Read the open scene's play area and HQs again (called on scene change and hover).
func refresh_map() -> void:
	var root = plugin.get_editor_interface().get_edited_scene_root() if plugin != null else null
	var data := _map_from_scene(root)
	_from_map = not data.is_empty()
	_scan_nodes = _count_nodes(root)
	if data.is_empty():
		data = {"play": _random_shape(), "hqs": [], "fitted": true}
	_fit(data)
	_cache.clear()
	_last_key = ""


func _world_xz(n: Node3D) -> PackedVector2Array:
	var out := PackedVector2Array()
	var xf: Transform3D = n.global_transform
	for p in n.get("points"):
		var g: Vector3 = xf * Vector3(p.x, 0, p.y)
		out.append(PolySnapPanel.rot(Vector2(g.x, g.z), _rotation()))
	return out


func _rotation() -> int:
	if plugin == null:
		return 0
	var es = plugin.get_editor_interface().get_editor_settings()
	if es.has_setting("polysnap/view/rotation"):
		return int(es.get_setting("polysnap/view/rotation")) % 4
	return 0


## The node an export points at. The SDK stores CombatVolume / HQArea as node
## exports (a Node once the scene is loaded); older scenes may hold a NodePath.
static func _linked(n: Node, prop: String) -> Node:
	var v = n.get(prop)
	if v is Node:
		return v if is_instance_valid(v) else null
	if typeof(v) == TYPE_NODE_PATH and not v.is_empty():
		return n.get_node_or_null(v)
	return null


static func _count_nodes(root) -> int:
	if root == null or not is_instance_valid(root) or not root.is_inside_tree():
		return -1
	return root.find_children("*", "", true, false).size()


func _map_from_scene(root: Node) -> Dictionary:
	if root == null or not root.is_inside_tree():
		return {}
	var play = null
	var hqs: Array = []
	var seen := {}
	for n in root.find_children("*", "", true, false):
		if play == null and "CombatVolume" in n:
			var v = _linked(n, "CombatVolume")
			if v != null and PolySnapPanel._is_polygon_volume(v) and v.get("points").size() >= 3:
				play = _world_xz(v)
		if "HQArea" in n:
			var team := 2 if int(n.get("Team")) == 2 else 1
			if seen.has(team):
				continue          # each team's first HQ in the scene = its starting HQ
			var a = _linked(n, "HQArea")
			if a != null and PolySnapPanel._is_polygon_volume(a) and a.get("points").size() >= 3:
				seen[team] = true
				hqs.append([team, _world_xz(a)])
	if play == null:
		return {}
	return {"play": play, "hqs": hqs, "fitted": false}


## A fixed random-looking 14-corner shape, used when no map is open.
func _random_shape() -> PackedVector2Array:
	var rng := RandomNumberGenerator.new()
	rng.seed = 23
	var out := PackedVector2Array()
	var step := TAU / 14.0
	for i in range(14):
		var a := -PI * 0.62 + i * step + rng.randf_range(-0.32, 0.32) * step
		var r := rng.randf_range(4.0, 6.6)
		out.append(Vector2(8.2 + cos(a) * r, 8.0 + sin(a) * r))
	return out


## Keep the corners that matter most to the outline until 14 are left (or add
## points in the middle of the longest sides if there are fewer).
static func to_14(src: PackedVector2Array) -> Array:
	var pts: Array = Array(src)
	while pts.size() > 14:
		var m := pts.size()
		var best := 0
		var best_a := INF
		for i in range(m):
			var a: Vector2 = pts[(i - 1 + m) % m]
			var b: Vector2 = pts[i]
			var c: Vector2 = pts[(i + 1) % m]
			var area := absf((b - a).cross(c - a))
			if area < best_a:
				best_a = area
				best = i
		pts.remove_at(best)
	while pts.size() < 14:
		var m := pts.size()
		var best := 0
		for i in range(m):
			if pts[i].distance_to(pts[(i + 1) % m]) > pts[best].distance_to(pts[(best + 1) % m]):
				best = i
		pts.insert(best + 1, (pts[best] + pts[(best + 1) % m]) * 0.5)
	return pts


func _fit(data: Dictionary) -> void:
	var play := to_14(data.play)
	# go around the same way as the P (clockwise on screen)
	var sa := 0.0
	for i in range(14):
		sa += play[i].cross(play[(i + 1) % 14])
	if sa < 0:
		play.reverse()
	var hqs: Array = data.hqs
	if not data.fitted:
		var box := Rect2(data.play[0], Vector2.ZERO)
		for p in data.play:
			box = box.expand(p)
		for h in hqs:
			for p in h[1]:
				box = box.expand(p)
		var sc := 13.0 / maxf(maxf(box.size.x, box.size.y), 0.001)
		var c := box.get_center()
		var f := func(p: Vector2) -> Vector2: return Vector2(8, 8) + (p - c) * sc
		for i in range(play.size()):
			play[i] = f.call(play[i])
		var fitted: Array = []
		for h in hqs:
			var poly := PackedVector2Array()
			for p in h[1]:
				poly.append(f.call(p))
			fitted.append([h[0], poly])
		hqs = fitted
	# pair each idle point with a P point: same order around, best starting offset
	var best_off := 0
	var best_cost := INF
	for off in range(14):
		var cost := 0.0
		for i in range(14):
			cost += play[(i + off) % 14].distance_to(P[i])
		if cost < best_cost:
			best_cost = cost
			best_off = off
	_home = []
	for i in range(14):
		_home.append(play[(i + best_off) % 14])
	_hqs = hqs


static func _ease(t: float) -> float:
	t = clampf(t, 0.0, 1.0)
	var c := 1.4
	return 1.0 + (c + 1.0) * pow(t - 1.0, 3) + c * pow(t - 1.0, 2)


## 0 = map, 1 = P, for each point at this moment.
func _progress() -> Array:
	var out: Array = []
	for i in range(14):
		var k := ORDER.find(i)
		var d := k * STAGGER
		if _hovered:
			out.append(_ease((_since - d) / MOVE))
		else:
			out.append(1.0 - _ease((_since - (0.45 - d) * 0.6) / (MOVE * 0.8)))
	return out


## Re-read the map once a scene has opened. Big maps take a while to finish
## loading, so until the play area is found it keeps checking for up to 30 s,
## and keeps going while the scene is still growing. Hovering the tab reads it again too.
func schedule_refresh() -> void:
	_refresh_in = 0.6
	_retry_left = 30.0


func _process(delta: float) -> void:
	if not map_icon:
		return
	if _home.size() != 14:
		refresh_map()
	if _button == null or not is_instance_valid(_button):
		_hook_button()
		if _button == null:
			return
	var root = plugin.get_editor_interface().get_edited_scene_root() if plugin != null else null
	if root != _root_seen:
		_root_seen = root
		schedule_refresh()
	if _refresh_in >= 0.0:
		_refresh_in -= delta
		if _refresh_in < 0.0:
			var before := _scan_nodes
			refresh_map()
			# Done once the play area is found and the scene stopped growing;
			# otherwise look again in a second (for up to 30 s after it opened).
			if _retry_left > 0.0 and root != null and not (_from_map and _scan_nodes == before):
				_refresh_in = 1.0
			else:
				_retry_left = 0.0
	if _retry_left > 0.0:
		_retry_left -= delta
	_since += delta
	var prog := _progress()
	var key := ""
	for v in prog:
		key += "%.3f," % v
	if key == _last_key:
		return
	_last_key = key
	_button.icon = _texture(prog)


func _size_px() -> int:
	var s := 1.0
	if plugin != null:
		s = plugin.get_editor_interface().get_editor_scale()
	return int(round(22.0 * s))   # a little bigger than a 16 px icon, sized next to the tab names


func _texture(prog: Array) -> Texture2D:
	var size := _size_px()
	var ck := "%d|%s" % [size, _last_key]
	if _cache.has(ck):
		return _cache[ck]
	var avg := 0.0
	var pts: Array = []
	for i in range(14):
		var t: float = prog[i]
		avg += clampf(t, 0.0, 1.0)
		pts.append(_home[i].lerp(P[i], t))
	avg /= 14.0
	var px := float(size) / 16.0
	var lw := maxf(1.2 / px, 0.38)
	var dot := maxf(0.9 / px, 0.43)
	var svg := '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">'
	var hq_alpha := 1.0 - minf(1.0, avg * 2.2)
	if hq_alpha > 0.0:
		for h in _hqs:
			var col: Color = COL_T1 if h[0] == 1 else COL_T2
			svg += '<polygon points="%s" fill="none" stroke="#%s" stroke-opacity="%.3f" stroke-width="%.3f" stroke-linejoin="round"/>' % [_pts_attr(h[1]), col.to_html(false), hq_alpha, lw * 0.8]
	var line_col := COL_PLAY.lerp(COL_P, avg)
	svg += '<polygon points="%s" fill="none" stroke="#%s" stroke-width="%.3f" stroke-linejoin="round"/>' % [_pts_attr(pts), line_col.to_html(false), lw]
	for p in pts:
		svg += '<circle cx="%.3f" cy="%.3f" r="%.3f" fill="#%s" stroke="#ffffff" stroke-width="%.3f"/>' % [p.x, p.y, dot, COL_DOT.to_html(false), dot * 0.9]
	svg += '</svg>'
	var img := Image.new()
	if img.load_svg_from_string(svg, px) != OK:
		return _button.icon
	var tex := ImageTexture.create_from_image(img)
	if _cache.size() > 400:
		_cache.clear()
	_cache[ck] = tex
	return tex


static func _pts_attr(pts) -> String:
	var parts := PackedStringArray()
	for p in pts:
		parts.append("%.3f,%.3f" % [p.x, p.y])
	return " ".join(parts)


## Put the tab back the way Godot made it (when the add-on is turned off).
func restore() -> void:
	if _button != null and is_instance_valid(_button):
		if not _orig.is_empty():
			_button.icon = _orig.icon
			_button.clip_text = _orig.clip
			_button.expand_icon = _orig.expand
			if _orig.font_size >= 0:
				_button.add_theme_font_size_override("font_size", _orig.font_size)
			else:
				_button.remove_theme_font_size_override("font_size")
			_orig = {}
		for c in ["font_color", "font_hover_color", "font_pressed_color", "font_hover_pressed_color", "font_focus_color", "font_disabled_color", "font_outline_color"]:
			_button.remove_theme_color_override(c)
		for c in ["icon_normal_color", "icon_hover_color", "icon_pressed_color", "icon_hover_pressed_color", "icon_focus_color", "icon_disabled_color"]:
			_button.remove_theme_color_override(c)
		if _button.mouse_entered.is_connected(_on_enter):
			_button.mouse_entered.disconnect(_on_enter)
			_button.mouse_exited.disconnect(_on_exit)
