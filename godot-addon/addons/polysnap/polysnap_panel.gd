@tool
extends Control
## The "PolySnap" main screen: scan the open scene's PolygonVolume nodes, show
## a top-down BEFORE / AFTER of the proposed line-up, let the user keep, undo or
## drag each change, then write it all back to the scene in one undoable step.

const Align = preload("polysnap_align.gd")
const MapView = preload("polysnap_view.gd")
const Updater = preload("polysnap_updater.gd")

const NEAR_MISS := 0.5

var plugin: EditorPlugin = null

# --- scene data (index = volume number in scene tree order) ---
var scene_root: Node = null
var nodes: Array = []          # Node3D
var paths: Array = []          # String, path from the scene root
var short_names: Array = []    # String, node name
var local_orig: Array = []     # PackedVector2Array as stored on the node
var world_y: Array = []        # PackedFloat64Array, height of each point in the world
var before: Array = []         # PackedVector2Array, world X/Z as in the scene
var after: Array = []          # PackedVector2Array, world X/Z after lining up
var point_count := 0
var skipped_locked := 0
var skipped_hidden := 0
var empty_message := "Open a scene that has PolygonVolume nodes, then press Scan Scene."

# --- user decisions, keyed by Vector2i(volume, point) ---
var fixed := {}     # key -> Vector2 world position the point must stay at
var manual := {}    # key -> true when the user dragged it
var undone := {}    # key -> the move that was undone (so it can be re-ticked)

# --- results of the last run ---
var moves := {}
var issues: Array = []
var near_list: Array = []
var lower_list: Array = []
var change_keys: Array = []

# --- view ---
var cam_center := Vector2.ZERO
var cam_zoom := 1.0
var show_names := true
var view_rot := 0      # quarter turns clockwise from Godot's 3D top view
var _rot_btn: Button
var _icon_box: CheckBox

# --- colors (saved in Editor Settings, so they stick across projects) ---
const COLOR_ROWS := [
	["anchor", "Anchor points (don't move)", Color(1, 1, 1)],
	["off", "Off points (BEFORE, will move)", Color(1.0, 0.25, 0.25)],
	["fixed", "Fixed points (AFTER, lined up)", Color(0.3, 0.9, 0.4)],
	["manual", "Points you moved", Color(0.35, 0.75, 1.0)],
	["undone", "Undone changes", Color(0.6, 0.6, 0.66)],
	["outline", "Polygon outlines", Color(0.55, 0.75, 0.95)],
]
var colors := {}
var per_volume_outlines := true
var _color_buttons := {}
var _per_volume_check: CheckBox
var _colors_popup: PopupPanel
var _updater = null
var hover_key = null
var _card_key = null
var _card_hide := 0.0
## The hover card waits this long on a point before it opens (or switches to
## another point), and stays this long after the mouse leaves, so it can be reached.
const CARD_SHOW_DELAY := 0.5
const CARD_HIDE_DELAY := 3.0
var _pending_key = null      # point the mouse is resting on, not shown yet
var _pending_wait := 0.0
var _pending_view = null
var _pending_pos := Vector2.ZERO

# --- controls ---
var _snap: SpinBox
var _status: Label
var _finalize: Button
var _views: Array = []
var _changes_tree: Tree
var _look_tree: Tree
var _tabs: TabContainer
var _card: PanelContainer
var _card_title: Label
var _card_info: Label
var _card_keep: CheckBox
var _card_reset: Button
var _card_hint: Label
var _select_token := 0
var _credits: AcceptDialog = null
var _confirm: ConfirmationDialog
var _alert: AcceptDialog
var _filling_tree := false


func _init() -> void:
	name = "PolySnapPanel"
	size_flags_horizontal = Control.SIZE_EXPAND_FILL
	size_flags_vertical = Control.SIZE_EXPAND_FILL
	_build_ui()


# ======================================================================== UI

func _build_ui() -> void:
	var root := VBoxContainer.new()
	root.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(root)

	# Toolbar.
	var bar := HBoxContainer.new()
	root.add_child(bar)
	var title := Label.new()
	title.text = "  PolySnap  "
	bar.add_child(title)
	var scan := Button.new()
	scan.text = "Scan Scene"
	scan.tooltip_text = "Read every PolygonVolume in the open scene again. Clears your keep/undo/move choices."
	scan.pressed.connect(_on_scan)
	bar.add_child(scan)
	bar.add_child(VSeparator.new())
	var sl := Label.new()
	sl.text = "Snap distance"
	bar.add_child(sl)
	_snap = SpinBox.new()
	_snap.min_value = 0.01
	_snap.max_value = 2.0
	_snap.step = 0.01
	_snap.value = 0.1
	_snap.suffix = "m"
	_snap.tooltip_text = "Points closer than this to a corner or edge of a volume above them (in the scene tree) get lined up."
	_snap.value_changed.connect(_on_snap_changed)
	bar.add_child(_snap)
	var fit := Button.new()
	fit.text = "Fit"
	fit.tooltip_text = "Zoom out to show every volume."
	fit.pressed.connect(fit_view)
	bar.add_child(fit)
	var names := CheckBox.new()
	names.text = "Names"
	names.button_pressed = true
	names.toggled.connect(_on_names_toggled)
	bar.add_child(names)
	_rot_btn = Button.new()
	_rot_btn.text = "Rotate 0°"
	_rot_btn.tooltip_text = "Turn the BEFORE/AFTER views (and the tab icon) 90° clockwise. 0° is exactly Godot's 3D top view, seen from above."
	_rot_btn.pressed.connect(_on_rotate)
	bar.add_child(_rot_btn)
	var col_btn := Button.new()
	col_btn.text = "Colors"
	col_btn.tooltip_text = "Pick the colors of anchor, off, fixed and other points, and the polygon outlines."
	col_btn.pressed.connect(_open_colors)
	bar.add_child(col_btn)
	_icon_box = CheckBox.new()
	_icon_box.text = "Map icon"
	_icon_box.tooltip_text = "Show the open map as an animated icon on the PolySnap tab instead of its name."
	_icon_box.toggled.connect(_on_map_icon_toggled)
	bar.add_child(_icon_box)
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	bar.add_child(spacer)
	var credits := Button.new()
	credits.text = "Credits"
	credits.tooltip_text = "Who made PolySnap."
	credits.pressed.connect(_show_credits)
	bar.add_child(credits)
	var upd := Button.new()
	upd.text = "Check for Update"
	if Updater.UPDATES_ENABLED:
		upd.tooltip_text = "Check GitHub for a newer PolySnap. This is the only time the add-on goes online."
		upd.pressed.connect(_open_updater)
	else:
		upd.text = "Check for Update (Coming Later)"
		upd.tooltip_text = "Coming Later"
		upd.disabled = true
	bar.add_child(upd)
	var reset := Button.new()
	reset.text = "Reset Choices"
	reset.tooltip_text = "Forget every keep/undo/move choice and go back to the automatic line-up."
	reset.pressed.connect(_reset_choices)
	bar.add_child(reset)
	_finalize = Button.new()
	_finalize.text = "Finalize"
	_finalize.tooltip_text = "Write the AFTER positions into the scene. One step, so Ctrl+Z undoes it."
	_finalize.pressed.connect(_ask_finalize)
	bar.add_child(_finalize)

	_status = Label.new()
	_status.clip_text = true
	_status.text = ""
	root.add_child(_status)

	# Views + side list.
	var split := HSplitContainer.new()
	split.size_flags_vertical = Control.SIZE_EXPAND_FILL
	root.add_child(split)
	var maps := HBoxContainer.new()
	maps.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	maps.add_theme_constant_override("separation", 4)
	split.add_child(maps)
	for i in range(2):
		var v = MapView.new()
		v.panel = self
		v.is_after = i == 1
		maps.add_child(v)
		_views.append(v)

	_tabs = TabContainer.new()
	_tabs.custom_minimum_size = Vector2(330, 0)
	split.add_child(_tabs)
	_changes_tree = _make_tree(true)
	_changes_tree.name = "Changes"
	_tabs.add_child(_changes_tree)
	_look_tree = _make_tree(false)
	_look_tree.name = "Needs a look"
	_tabs.add_child(_look_tree)

	var help := Label.new()
	help.text = "  Wheel: zoom    Right/middle drag (or left drag on empty space): pan    Hover a point to keep or undo its change    Drag a point in AFTER to move it (snaps to nearby corners/edges, hold Alt to place freely)"
	help.clip_text = true
	help.modulate = Color(1, 1, 1, 0.7)
	root.add_child(help)

	# Hover card.
	_card = PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.11, 0.12, 0.15, 0.97)
	sb.border_color = Color(1, 0.55, 0.1)
	sb.set_border_width_all(1)
	sb.set_corner_radius_all(4)
	sb.content_margin_left = 10
	sb.content_margin_right = 10
	sb.content_margin_top = 8
	sb.content_margin_bottom = 8
	_card.add_theme_stylebox_override("panel", sb)
	_card.visible = false
	_card.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(_card)
	var cv := VBoxContainer.new()
	_card.add_child(cv)
	_card_title = Label.new()
	_card_title.add_theme_color_override("font_color", Color(1, 0.7, 0.35))
	cv.add_child(_card_title)
	_card_info = Label.new()
	_card_info.custom_minimum_size = Vector2(280, 0)
	_card_info.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	cv.add_child(_card_info)
	_card_keep = CheckBox.new()
	_card_keep.text = "Keep this change"
	_card_keep.toggled.connect(_on_card_keep)
	cv.add_child(_card_keep)
	var cb := HBoxContainer.new()
	cv.add_child(cb)
	_card_reset = Button.new()
	_card_reset.text = "Reset point"
	_card_reset.tooltip_text = "Throw away your move for this point."
	_card_reset.pressed.connect(_on_card_reset)
	cb.add_child(_card_reset)
	_card_hint = Label.new()
	_card_hint.text = "Double-click the point to show it in 3D."
	_card_hint.modulate = Color(1, 1, 1, 0.6)
	cv.add_child(_card_hint)

	_confirm = ConfirmationDialog.new()
	_confirm.title = "Finalize polygon changes"
	_confirm.confirmed.connect(_finalize_now)
	add_child(_confirm)
	_alert = AcceptDialog.new()
	_alert.title = "PolySnap"
	add_child(_alert)
	_build_colors_popup()
	_update_status()


func _build_colors_popup() -> void:
	for row in COLOR_ROWS:
		colors[row[0]] = row[2]
	_colors_popup = PopupPanel.new()
	add_child(_colors_popup)
	var box := VBoxContainer.new()
	_colors_popup.add_child(box)
	var head := Label.new()
	head.text = "Colors"
	box.add_child(head)
	var grid := GridContainer.new()
	grid.columns = 2
	grid.add_theme_constant_override("h_separation", 12)
	box.add_child(grid)
	for row in COLOR_ROWS:
		var l := Label.new()
		l.text = row[1]
		grid.add_child(l)
		var b := ColorPickerButton.new()
		b.custom_minimum_size = Vector2(70, 26)
		b.edit_alpha = false
		b.color = row[2]
		b.get_picker().picker_shape = ColorPicker.SHAPE_HSV_WHEEL
		b.color_changed.connect(_on_color_changed.bind(row[0]))
		grid.add_child(b)
		_color_buttons[row[0]] = b
	_per_volume_check = CheckBox.new()
	_per_volume_check.text = "Give each polygon its own outline color"
	_per_volume_check.button_pressed = true
	_per_volume_check.toggled.connect(_on_per_volume_toggled)
	box.add_child(_per_volume_check)
	var reset := Button.new()
	reset.text = "Reset colors"
	reset.pressed.connect(_reset_colors)
	box.add_child(reset)


func _settings():
	var ei = _ei()
	return ei.get_editor_settings() if ei != null else null


## Load saved colors. Called by the plugin once it is set up.
func load_colors() -> void:
	var es = _settings()
	if es == null:
		return
	for row in COLOR_ROWS:
		var k: String = "polysnap/colors/" + row[0]
		if es.has_setting(k) and typeof(es.get_setting(k)) == TYPE_COLOR:
			colors[row[0]] = es.get_setting(k)
		_color_buttons[row[0]].color = colors[row[0]]
	if es.has_setting("polysnap/view/rotation"):
		view_rot = int(es.get_setting("polysnap/view/rotation")) % 4
	_rot_btn.text = "Rotate %d°" % (view_rot * 90)
	var pk := "polysnap/colors/per_volume_outlines"
	if es.has_setting(pk):
		per_volume_outlines = bool(es.get_setting(pk))
	_per_volume_check.set_pressed_no_signal(per_volume_outlines)
	_redraw()


func _save_colors() -> void:
	var es = _settings()
	if es == null:
		return
	for row in COLOR_ROWS:
		es.set_setting("polysnap/colors/" + row[0], colors[row[0]])
	es.set_setting("polysnap/colors/per_volume_outlines", per_volume_outlines)


## Change one color by name ("anchor", "off", "fixed", "manual", "undone", "outline").
func set_color(which: String, c: Color) -> void:
	colors[which] = c
	if _color_buttons.has(which):
		_color_buttons[which].color = c
	_save_colors()
	if not before.is_empty():
		_fill_trees()
	_redraw()


func _on_color_changed(c: Color, which: String) -> void:
	set_color(which, c)


func _on_per_volume_toggled(on: bool) -> void:
	per_volume_outlines = on
	_save_colors()
	_redraw()


func _reset_colors() -> void:
	for row in COLOR_ROWS:
		colors[row[0]] = row[2]
		_color_buttons[row[0]].color = row[2]
	per_volume_outlines = true
	_per_volume_check.set_pressed_no_signal(true)
	_save_colors()
	if not before.is_empty():
		_fill_trees()
	_redraw()


func _open_updater() -> void:
	if _updater == null:
		_updater = Updater.new()
		_updater.plugin = plugin
		add_child(_updater)
	_updater.open()


func _open_colors() -> void:
	_colors_popup.popup_centered()


func _on_names_toggled(on: bool) -> void:
	show_names = on
	_redraw()


## Turn v by q quarter turns clockwise on screen (exact, no rounding).
static func rot(v: Vector2, q: int) -> Vector2:
	match posmod(q, 4):
		1:
			return Vector2(-v.y, v.x)
		2:
			return -v
		3:
			return Vector2(v.y, -v.x)
	return v


func _on_rotate() -> void:
	view_rot = (view_rot + 1) % 4
	_rot_btn.text = "Rotate %d°" % (view_rot * 90)
	var es = _settings()
	if es != null:
		es.set_setting("polysnap/view/rotation", view_rot)
	fit_view()
	if plugin != null and plugin.has_method("on_view_rotated"):
		plugin.on_view_rotated()


func _reset_choices() -> void:
	fixed.clear()
	manual.clear()
	undone.clear()
	recompute(true)


func _on_card_keep(on: bool) -> void:
	if _card_key != null:
		set_keep(_card_key, on)


func _on_card_reset() -> void:
	if _card_key != null:
		set_keep(_card_key, false)


func _on_scan() -> void:
	rescan(false)


func _on_snap_changed(_v: float) -> void:
	recompute(true)


func _make_tree(checks: bool) -> Tree:
	var t := Tree.new()
	t.hide_root = true
	t.columns = 2 if checks else 1
	t.select_mode = Tree.SELECT_ROW
	if checks:
		t.set_column_expand(0, false)
		t.set_column_custom_minimum_width(0, 30)
		t.item_edited.connect(_on_tree_edited)
	t.item_selected.connect(func(): _on_tree_selected(t))
	return t


# ================================================================ scanning

## Called when the tab is opened. Only rescans when the scene changed.
func on_shown() -> void:
	if _is_stale():
		rescan(true)
	else:
		_redraw()


func on_scene_changed(_root) -> void:
	if visible:
		rescan(true)
	else:
		scene_root = null   # force a scan next time the tab opens


func _ei():
	return plugin.get_editor_interface() if plugin != null else null


func _is_stale() -> bool:
	var ei = _ei()
	if ei == null:
		return false
	var r = ei.get_edited_scene_root()
	if r != scene_root:
		return true
	if r == null:
		return false
	for vi in range(nodes.size()):
		if not is_instance_valid(nodes[vi]) or nodes[vi].get("points") != local_orig[vi]:
			return true
		if not nodes[vi].is_visible_in_tree():
			return true
		if not nodes[vi].global_transform.is_equal_approx(_scan_xforms[vi]):
			return true
	return _find_volumes(r).size() != nodes.size()


var _scan_xforms: Array = []


static func _is_polygon_volume(n: Node) -> bool:
	if not (n is Node3D):
		return false
	if typeof(n.get("points")) != TYPE_PACKED_VECTOR2_ARRAY:
		return false
	var s = n.get_script()
	if s == null:
		return false
	# PolygonVolume.gd from the Portal SDK, or anything that extends it.
	while s != null:
		var path: String = s.resource_path
		if path.get_file() == "PolygonVolume.gd":
			return true
		if s.has_method("get_global_name") and str(s.get_global_name()) == "PolygonVolume":
			return true
		s = s.get_base_script()
	return false


func _find_volumes(r: Node) -> Array:
	var out: Array = []
	skipped_locked = 0
	skipped_hidden = 0
	var stack: Array = [r]
	# depth-first, in scene tree order
	while not stack.is_empty():
		var n: Node = stack.pop_back()
		if _is_polygon_volume(n):
			if not (n == r or n.owner == r or (n.owner != null and r.is_editable_instance(n.owner))):
				skipped_locked += 1
			elif not (n as Node3D).is_visible_in_tree():
				# Hidden itself, or under a hidden parent: leave it out of the
				# preview, the lists and Finalize, and don't snap onto it.
				skipped_hidden += 1
			else:
				out.append(n)
		var kids := n.get_children()
		for i in range(kids.size() - 1, -1, -1):
			stack.append(kids[i])
	return out


## Read the scene again. keep_choices tries to carry over the user's
## keep/undo/move choices (by node path and point number).
func rescan(keep_choices: bool) -> void:
	var saved := {}
	if keep_choices:
		for key in fixed.keys():
			if key.x < paths.size():
				saved["%s|%d" % [paths[key.x], key.y]] = {"pos": fixed[key], "manual": manual.has(key), "undone": undone.get(key)}
	fixed.clear()
	manual.clear()
	undone.clear()
	nodes.clear(); paths.clear(); short_names.clear(); local_orig.clear()
	world_y.clear(); before.clear(); _scan_xforms.clear()
	point_count = 0
	var ei = _ei()
	_scan(ei.get_edited_scene_root() if ei != null else null, saved)


## Read the volumes under root (no editor needed, so tests can call it).
func _scan(root: Node, saved: Dictionary) -> void:
	scene_root = root
	if scene_root == null:
		empty_message = "Open a scene that has PolygonVolume nodes, then press Scan Scene."
	else:
		for n in _find_volumes(scene_root):
			var lp: PackedVector2Array = n.get("points")
			var xf: Transform3D = n.global_transform
			var wp := PackedVector2Array()
			var wy := PackedFloat64Array()
			for p in lp:
				var g: Vector3 = xf * Vector3(p.x, 0, p.y)
				wp.append(Vector2(g.x, g.z))
				wy.append(g.y)
			nodes.append(n)
			paths.append(str(scene_root.get_path_to(n)))
			short_names.append(str(n.name))
			local_orig.append(lp.duplicate())
			world_y.append(wy)
			before.append(wp)
			_scan_xforms.append(xf)
			point_count += lp.size()
		if nodes.is_empty():
			if skipped_hidden > 0:
				empty_message = "No visible PolygonVolume nodes found in this scene (%d hidden)." % skipped_hidden
			else:
				empty_message = "No PolygonVolume nodes found in this scene."
		else:
			empty_message = ""
	for vi in range(paths.size()):
		for k in range(before[vi].size()):
			var s = saved.get("%s|%d" % [paths[vi], k])
			if s == null:
				continue
			var key := Vector2i(vi, k)
			if s.manual:
				fixed[key] = s.pos
				manual[key] = true
			elif s.undone != null:
				fixed[key] = before[vi][k]
				undone[key] = s.undone
	recompute(true)
	fit_view()


# ============================================================== line-up

## Short node names, or the full path when two volumes share a name.
func _labels() -> Array:
	var count := {}
	for n in short_names:
		count[n] = count.get(n, 0) + 1
	var out: Array = []
	for i in range(short_names.size()):
		out.append(short_names[i] if count[short_names[i]] == 1 else paths[i])
	return out


func recompute(rebuild_lists: bool) -> void:
	if before.is_empty():
		after = []
		moves = {}
		issues = []; near_list = []; lower_list = []; change_keys = []
	else:
		var r: Dictionary = Align.run(before, _labels(), fixed, _snap.value, NEAR_MISS)
		after = r.after
		moves = r.moves
		issues = r.issues
		near_list = r.near
		lower_list = r.lower
		var keys := {}
		for k in moves.keys():
			keys[k] = true
		for k in manual.keys():
			keys[k] = true
		for k in undone.keys():
			keys[k] = true
		change_keys = keys.keys()
		change_keys.sort_custom(func(a, b): return a.x < b.x or (a.x == b.x and a.y < b.y))
	if rebuild_lists:
		_fill_trees()
	_update_status()
	_redraw()


## "" (not changed), "corner", "edge", "manual", or "undone".
func change_state(key) -> String:
	if manual.has(key):
		return "manual"
	if undone.has(key):
		return "undone"
	if moves.has(key):
		return moves[key].kind
	return ""


## Color of a changed point where it ends up (AFTER), by state.
func state_color(state: String) -> Color:
	match state:
		"manual":
			return colors.manual
		"undone":
			return colors.undone
	return colors.fixed


func vol_color(vi: int) -> Color:
	if not per_volume_outlines:
		return colors.outline
	return Color.from_hsv(fmod(0.58 + vi * 0.618034, 1.0), 0.45, 0.95, 0.9)


func _moved_count() -> int:
	var n := 0
	for vi in range(before.size()):
		for k in range(before[vi].size()):
			if before[vi][k] != after[vi][k]:
				n += 1
	return n


func _update_status() -> void:
	if _status == null:
		return
	if before.is_empty():
		_status.text = "  " + empty_message
		_finalize.disabled = true
		return
	var c := 0
	var e := 0
	for k in moves.keys():
		if manual.has(k):
			continue
		if moves[k].kind == "corner":
			c += 1
		else:
			e += 1
	var moved := _moved_count()
	var t := "%d volumes, %d points.  %d onto corners, %d onto edges, %d moved by you, %d undone.  %d point(s) will move." % [
		nodes.size(), point_count, c, e, manual.size(), undone.size(), moved]
	if skipped_locked > 0:
		t += "  (%d inside locked instances skipped)" % skipped_locked
	if skipped_hidden > 0:
		t += "  (%d hidden skipped)" % skipped_hidden
	_status.text = "  " + t
	_finalize.disabled = moved == 0
	_finalize.text = "Finalize (%d)" % moved if moved > 0 else "Finalize"


func _fmt_dist(d: float) -> String:
	if d < 1.0:
		return "%s cm" % str(snappedf(d * 100.0, 0.1))
	return "%s m" % str(snappedf(d, 0.01))


func _describe(key) -> String:
	var state := change_state(key)
	var from: Vector2 = before[key.x][key.y]
	var to: Vector2 = after[key.x][key.y]
	match state:
		"manual":
			return "Moved by you, %s." % _fmt_dist(from.distance_to(to))
		"undone":
			var u = undone[key]
			return "Change undone, stays where it was (was going to move %s onto %s)." % [_fmt_dist(u.dist), u.onto]
		"corner":
			return "Snaps %s onto %s." % [_fmt_dist(moves[key].dist), moves[key].onto]
		"edge":
			return "Slides %s onto %s." % [_fmt_dist(moves[key].dist), moves[key].onto]
	return "Not moved."


func _fill_trees() -> void:
	_filling_tree = true
	_changes_tree.clear()
	var r := _changes_tree.create_item()
	for key in change_keys:
		var it := _changes_tree.create_item(r)
		it.set_cell_mode(0, TreeItem.CELL_MODE_CHECK)
		it.set_editable(0, true)
		it.set_checked(0, change_state(key) != "undone")
		it.set_tooltip_text(0, "Keep this change")
		it.set_text(1, "%s  pt %d\n%s" % [short_names[key.x], key.y + 1, _describe(key)])
		it.set_tooltip_text(1, paths[key.x])
		it.set_custom_color(1, state_color(change_state(key)).lightened(0.2))
		it.set_metadata(1, key)
	_tabs.set_tab_title(0, "Changes (%d)" % change_keys.size())

	_look_tree.clear()
	var lr := _look_tree.create_item()
	var groups := [["Issues", issues], ["Near misses (not snapped)", near_list], ["On a lower volume's edge (not changed)", lower_list]]
	var total := 0
	for g in groups:
		if g[1].is_empty():
			continue
		var head := _look_tree.create_item(lr)
		head.set_text(0, "%s (%d)" % [g[0], g[1].size()])
		head.set_selectable(0, false)
		head.set_custom_color(0, Color(1, 0.7, 0.35))
		for entry in g[1]:
			var it := _look_tree.create_item(head)
			it.set_text(0, entry.text)
			it.set_tooltip_text(0, entry.text)
			it.set_metadata(0, entry.at)
			total += 1
	if total == 0:
		var it := _look_tree.create_item(lr)
		it.set_text(0, "Nothing to look at." if not before.is_empty() else empty_message)
		it.set_selectable(0, false)
	_tabs.set_tab_title(1, "Needs a look (%d)" % total)
	_filling_tree = false


func _on_tree_edited() -> void:
	if _filling_tree:
		return
	var it := _changes_tree.get_edited()
	if it == null:
		return
	var key = it.get_metadata(1)
	set_keep(key, it.is_checked(0))


func _on_tree_selected(t: Tree) -> void:
	var it := t.get_selected()
	if it == null:
		return
	var at
	if t == _changes_tree:
		var key = it.get_metadata(1)
		hover_key = key
		at = after[key.x][key.y]
		focus_on(at, before[key.x][key.y].distance_to(at))
	else:
		at = it.get_metadata(0)
		if at != null:
			focus_on(at, 0.15)


## Keep (true) or undo (false) the change at key. For a point the user dragged,
## false throws the drag away.
func set_keep(key, keep: bool) -> void:
	if manual.has(key):
		if not keep:
			manual.erase(key)
			fixed.erase(key)
	elif keep:
		undone.erase(key)
		fixed.erase(key)
	elif moves.has(key):
		undone[key] = moves[key]
		fixed[key] = before[key.x][key.y]
	recompute(true)
	_refresh_card()


# ============================================================ view / input

func _redraw() -> void:
	for v in _views:
		v.queue_redraw()


func fit_view() -> void:
	if before.is_empty() or _views.is_empty():
		cam_center = Vector2.ZERO
		cam_zoom = 1.0
		_redraw()
		return
	var box := Rect2(before[0][0] if before[0].size() > 0 else Vector2.ZERO, Vector2.ZERO)
	for pts in before:
		for p in pts:
			box = box.expand(p)
	var vs: Vector2 = _views[0].size
	if vs.x < 10 or vs.y < 10:
		vs = Vector2(600, 600)
	var span := Vector2(maxf(box.size.x, 1.0), maxf(box.size.y, 1.0))
	if view_rot % 2 == 1:
		span = Vector2(span.y, span.x)
	cam_zoom = minf(vs.x / span.x, vs.y / span.y) * 0.9
	cam_center = box.get_center()
	_redraw()


## Center on a spot, zoomed so a move of `dist` meters is easy to see.
func focus_on(at: Vector2, dist: float = 0.1) -> void:
	cam_center = at
	var vs: Vector2 = _views[0].size
	var span := clampf(dist * 10.0, 0.06, 2.0)
	cam_zoom = minf(vs.x, vs.y) / span
	_redraw()


func zoom_at(world: Vector2, screen: Vector2, view_size: Vector2, f: float) -> void:
	cam_zoom = clampf(cam_zoom * f, 0.01, 200000.0)
	cam_center = world - rot((screen - view_size * 0.5) / cam_zoom, -view_rot)
	_redraw()


## d is a screen-space drag already divided by the zoom.
func pan(d: Vector2) -> void:
	cam_center += rot(d, -view_rot)
	_redraw()


func view_hover(key, view, local: Vector2) -> void:
	hover_key = key
	if key == null:
		_pending_key = null
	elif _card.visible and key == _card_key:
		_card_hide = 0.0
		_pending_key = null
	elif key != _pending_key:
		# A new point: wait a moment before opening the card for it, so points
		# close together don't make it flick between them.
		_pending_key = key
		_pending_wait = 0.0
	if key != null:
		_pending_view = view
		_pending_pos = local
	_redraw()


func _open_card(key, view, local: Vector2) -> void:
	_card_key = key
	_card_hide = 0.0
	_refresh_card()
	var p: Vector2 = view.global_position + local - global_position + Vector2(18, 14)
	_card.reset_size()
	var cs := _card.get_combined_minimum_size()
	p.x = clampf(p.x, 4, maxf(4, size.x - cs.x - 4))
	p.y = clampf(p.y, 4, maxf(4, size.y - cs.y - 4))
	_card.position = p
	_card.visible = true


func _refresh_card() -> void:
	if _card_key == null or _card_key.x >= before.size():
		_card.visible = false
		return
	var key = _card_key
	var state := change_state(key)
	_card_title.text = "%s  -  point %d" % [short_names[key.x], key.y + 1]
	var w: Vector2 = after[key.x][key.y]
	_card_info.text = "%s\n%s\nX %.3f   Z %.3f" % [paths[key.x], _describe(key), w.x, w.y]
	_card_keep.visible = state != ""
	_card_keep.text = "Keep my move" if state == "manual" else "Keep this change"
	_card_keep.set_pressed_no_signal(state != "undone" and state != "")
	_card_reset.visible = state == "manual"


func _process(delta: float) -> void:
	if _card == null:
		return
	var over_card := _card.visible and _card.get_rect().grow(8).has_point(get_local_mouse_position())
	if _pending_key != null and not over_card:
		if _pending_key == hover_key and _pending_view != null and is_instance_valid(_pending_view):
			_pending_wait += delta
			if _pending_wait >= CARD_SHOW_DELAY:
				_open_card(_pending_key, _pending_view, _pending_pos)
				_pending_key = null
		else:
			_pending_key = null
	if not _card.visible:
		return
	if over_card or (hover_key != null and hover_key == _card_key):
		_card_hide = 0.0
	else:
		_card_hide += delta
		if _card_hide > CARD_HIDE_DELAY:
			_card.visible = false


var _drag_start = null


func begin_drag(key) -> void:
	_drag_start = after[key.x][key.y]
	_card.visible = false
	_pending_key = null


## Move key to world position w. Snaps to the closest corner, or else edge, of
## another volume nearby unless free is true. Returns the spot it snapped to.
func drag_to(key, w: Vector2, free: bool):
	var target := w
	var snapped = null
	if not free:
		var r := maxf(_snap.value, 12.0 / cam_zoom)
		var best := r
		for vi in range(after.size()):
			if vi == key.x:
				continue
			for q in after[vi]:
				var d := w.distance_to(q)
				if d <= best:
					best = d
					snapped = q
		if snapped == null:
			for vi in range(after.size()):
				if vi == key.x:
					continue
				var pts: PackedVector2Array = after[vi]
				for e in range(pts.size()):
					var on := Geometry2D.get_closest_point_to_segment(w, pts[e], pts[(e + 1) % pts.size()])
					var d2 := w.distance_to(on)
					if d2 <= best:
						best = d2
						snapped = on
		if snapped != null:
			target = snapped
	fixed[key] = target
	manual[key] = true
	undone.erase(key)
	hover_key = key
	recompute(false)
	return snapped


func end_drag(key) -> void:
	if manual.has(key) and fixed[key].distance_to(before[key.x][key.y]) < Align.ALIGNED:
		manual.erase(key)   # dropped back where it started
		fixed.erase(key)
	_drag_start = null
	recompute(true)


## Clicking a point in AFTER: select its volume in the scene tree (and the
## Inspector), staying in the PolySnap tab.
func select_in_tree(key) -> void:
	var ei = _ei()
	var vi: int = key.x
	if ei == null or vi >= nodes.size() or not is_instance_valid(nodes[vi]):
		return
	var sel = ei.get_selection()
	if sel.get_selected_nodes().size() == 1 and sel.get_selected_nodes()[0] == nodes[vi]:
		return
	sel.clear()
	sel.add_node(nodes[vi])
	ei.edit_node(nodes[vi])
	# Older Godot versions jump to the 3D view a moment after a 3D node is
	# selected: come straight back, unless a double-click asked for 3D meanwhile.
	_select_token += 1
	var token := _select_token
	for i in range(3):
		await get_tree().process_frame
	if token == _select_token and not is_visible_in_tree():
		ei.set_main_screen_editor("PolySnap")


## Double-clicking a point: select its volume, switch to the 3D view and bring
## the 3D camera to that point (in AFTER, to where the point will end up).
func show_point_in_3d(key, in_after: bool) -> void:
	_select_token += 1          # a double-click wants the 3D view: don't switch back
	var ei = _ei()
	var vi: int = key.x
	if ei == null or vi >= nodes.size() or not is_instance_valid(nodes[vi]):
		return
	var node: Node3D = nodes[vi]
	var xf: Transform3D = node.global_transform
	var lp: Vector2 = local_orig[vi][key.y]
	var spot: Vector3 = xf * Vector3(lp.x, 0, lp.y)
	if in_after:
		var w: Vector2 = after[vi][key.y]
		spot = Vector3(w.x, spot.y, w.y)
	_card.visible = false
	ei.set_main_screen_editor("3D")
	await _focus_3d_camera(spot)
	if not is_instance_valid(node):
		return
	ei.get_selection().clear()
	ei.get_selection().add_node(node)
	ei.edit_node(node)


## Godot only lets the 3D view's own "Focus Selection" move its camera, so a
## temporary marker (never saved, removed straight after) is selected at the spot
## and focused on.
func _focus_3d_camera(spot: Vector3) -> void:
	var ei = _ei()
	var root = ei.get_edited_scene_root()
	if root == null or not root is Node:
		return
	var marker := Node3D.new()
	marker.name = "PolySnapFocus"
	root.add_child(marker)
	marker.global_position = spot
	ei.get_selection().clear()
	ei.get_selection().add_node(marker)
	for i in range(3):
		await get_tree().process_frame
	for vp_i in range(4):
		var vp = ei.get_editor_viewport_3d(vp_i)
		if vp == null:
			continue
		var holder = vp.get_parent()       # the viewport's on-screen container
		if not (holder is Control and holder.is_visible_in_tree()):
			continue
		var menu := _focus_menu(vp)
		if menu != null:
			menu[0].id_pressed.emit(menu[1])
	await get_tree().process_frame
	ei.get_selection().remove_node(marker)
	root.remove_child(marker)
	marker.free()


## The "Focus Selection" item of a 3D viewport's View menu: [PopupMenu, id] or [].
func _focus_menu(vp: Node) -> Array:
	var n: Node = vp
	while n != null and not n.get_class() == "Node3DEditorViewport":
		n = n.get_parent()
	if n == null:
		return []
	for mb in n.find_children("*", "MenuButton", true, false):
		var pm: PopupMenu = mb.get_popup()
		for i in range(pm.item_count):
			var sc: Shortcut = pm.get_item_shortcut(i)
			if pm.get_item_text(i) == "Focus Selection" or (sc != null and sc.resource_name == "spatial_editor/focus_selection"):
				return [pm, pm.get_item_id(i)]
	return []


# ================================================================ finalize

func _ask_finalize() -> void:
	var moved := _moved_count()
	var vols := 0
	for vi in range(before.size()):
		if before[vi] != after[vi]:
			vols += 1
	_confirm.dialog_text = "Move %d point(s) on %d polygon volume(s) to their AFTER positions?\n\nOnly the points' X/Z positions change. You can undo it with Ctrl+Z." % [moved, vols]
	_confirm.popup_centered()


func _finalize_now() -> void:
	if _is_stale():
		_alert.dialog_text = "The scene changed since it was scanned (a volume was moved, edited, added or removed).\nNothing was changed. Press Scan Scene and check the changes again."
		_alert.popup_centered()
		return
	var ur = plugin.get_undo_redo()
	var moved := _moved_count()
	ur.create_action("PolySnap: line up %d point(s)" % moved)
	var count := 0
	for vi in range(before.size()):
		if before[vi] == after[vi]:
			continue
		var node: Node3D = nodes[vi]
		var inv: Transform3D = _scan_xforms[vi].affine_inverse()
		var np: PackedVector2Array = local_orig[vi].duplicate()
		for k in range(np.size()):
			if before[vi][k] == after[vi][k]:
				continue
			var l: Vector3 = inv * Vector3(after[vi][k].x, world_y[vi][k], after[vi][k].y)
			np[k] = Vector2(l.x, l.z)
		ur.add_do_property(node, "points", np)
		ur.add_undo_property(node, "points", local_orig[vi])
		count += 1
	ur.commit_action()
	print("PolySnap: moved %d point(s) on %d polygon volume(s). Ctrl+Z to undo." % [moved, count])
	rescan(false)


func _on_map_icon_toggled(on: bool) -> void:
	if plugin != null:
		plugin.set_map_icon(on)


func sync_map_icon_box() -> void:
	if _icon_box != null and plugin != null:
		_icon_box.set_pressed_no_signal(plugin.map_icon_on())


const CREDITS := [
	["Gekko", "PolySnap, the tool itself"],
	["andy6170", "Visible-only feature: hidden polygon volumes are skipped"],
]


func _show_credits() -> void:
	if _credits == null:
		_credits = AcceptDialog.new()
		_credits.title = "PolySnap Credits"
		var cfg := ConfigFile.new()
		var ver := ""
		if cfg.load(get_script().resource_path.get_base_dir().path_join("plugin.cfg")) == OK:
			ver = str(cfg.get_value("plugin", "version", ""))
		var lines := ["PolySnap Rev %s" % ver, ""]
		for c in CREDITS:
			lines.append("%s  -  %s" % [c[0], c[1]])
		_credits.dialog_text = "\n".join(lines)
		add_child(_credits)
	_credits.popup_centered()
