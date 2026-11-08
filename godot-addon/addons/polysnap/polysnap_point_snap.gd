@tool
extends Node
## Live point snapping in the 3D view.
##
## Works with the Portal SDK's own PolygonVolume point handles: while you drag a
## point, every frame (before it is drawn) this checks the point and, if it is
## within the snap distance of a corner of another polygon volume, pulls it onto
## that corner (or, if no corner is that close, onto the closest edge). Pull away
## further and it follows the mouse again. Hold Alt to drag without snapping.

const PolySnapPanel = preload("polysnap_panel.gd")

var plugin: EditorPlugin = null
var enabled := true
var distance := 0.25

## Tests can force the mouse state (-1 = read the real mouse).
var test_mouse_down := -1

var _toolbar: HBoxContainer
var _toggle: Button
var _spin: SpinBox
var _label: Label

var _node: Node3D = null
var _last := PackedVector2Array()
var _targets: Array = []        # [[name, PackedVector2Array world X/Z]] of the other volumes
var _drag_idx := -1
var _snapped := false
var _was_down := false


func _ready() -> void:
	set_process(true)


func make_toolbar() -> Control:
	_toolbar = HBoxContainer.new()
	_toolbar.add_child(VSeparator.new())
	_toggle = Button.new()
	_toggle.toggle_mode = true
	_toggle.flat = true
	_toggle.text = "Point Snap"
	_toggle.tooltip_text = "PolySnap: while you drag a PolygonVolume point, snap it onto corners (or edges) of other polygon volumes within the distance. Hold Alt to drag freely."
	_toggle.button_pressed = enabled
	_toggle.toggled.connect(_on_toggled)
	_toolbar.add_child(_toggle)
	_spin = SpinBox.new()
	_spin.min_value = 0.01
	_spin.max_value = 5.0
	_spin.step = 0.01
	_spin.value = distance
	_spin.suffix = "m"
	_spin.tooltip_text = "How close a dragged point has to get to snap."
	_spin.custom_minimum_size = Vector2(90, 0)
	_spin.value_changed.connect(_on_distance)
	_toolbar.add_child(_spin)
	_label = Label.new()
	_label.modulate = Color(0.45, 1.0, 0.6)
	_toolbar.add_child(_label)
	_load()
	return _toolbar


func _settings():
	return plugin.get_editor_interface().get_editor_settings() if plugin != null else null


func _load() -> void:
	var es = _settings()
	if es == null:
		return
	if es.has_setting("polysnap/point_snap/enabled"):
		enabled = bool(es.get_setting("polysnap/point_snap/enabled"))
	if es.has_setting("polysnap/point_snap/distance"):
		distance = float(es.get_setting("polysnap/point_snap/distance"))
	_toggle.set_pressed_no_signal(enabled)
	_spin.set_value_no_signal(distance)
	_spin.editable = enabled


func _save() -> void:
	var es = _settings()
	if es == null:
		return
	es.set_setting("polysnap/point_snap/enabled", enabled)
	es.set_setting("polysnap/point_snap/distance", distance)


func _on_toggled(on: bool) -> void:
	enabled = on
	_spin.editable = on
	_save()
	_reset()


func _on_distance(v: float) -> void:
	distance = v
	_save()


func _reset() -> void:
	_node = null
	_targets = []
	_drag_idx = -1
	_snapped = false
	if _label != null:
		_label.text = ""


func _mouse_down() -> bool:
	if test_mouse_down >= 0:
		return test_mouse_down == 1
	return Input.is_mouse_button_pressed(MOUSE_BUTTON_LEFT)


func _selected_volume() -> Node3D:
	if plugin == null:
		return null
	var sel = plugin.get_editor_interface().get_selection().get_selected_nodes()
	if sel.size() != 1:
		return null
	var n: Node = sel[0]
	if not PolySnapPanel._is_polygon_volume(n):
		return null
	return n if (n as Node3D).is_visible_in_tree() else null


func _process(_delta: float) -> void:
	if not enabled:
		return
	tick()


## One check of the dragged point. Runs every frame; tests call it directly.
func tick() -> void:
	var node := _selected_volume()
	if node == null:
		if _node != null:
			_reset()
		return
	var pts: PackedVector2Array = node.points
	if node != _node or pts.size() != _last.size():
		_reset()
		_node = node
		_last = pts.duplicate()
		return

	var down := _mouse_down()
	var changed := -1
	var count := 0
	for i in range(pts.size()):
		if pts[i] != _last[i]:
			changed = i
			count += 1

	# Only points being dragged with the mouse are snapped. Undo/redo, typing in
	# the Inspector and other edits are left exactly as they are.
	var dragging := count == 1 and down
	var released_mid_drag := count == 1 and not down and _was_down and changed == _drag_idx
	if (dragging or released_mid_drag) and not Input.is_key_pressed(KEY_ALT):
		_drag_idx = changed
		if _targets.is_empty():
			_gather(node)
		var fixed := _snap_point(node, pts, changed)
		if fixed != pts:
			if released_mid_drag:
				# The mouse was let go in the same frame as the last move, so the SDK
				# already saved the unsnapped point. Snap it as its own undo step.
				var ur = plugin.get_undo_redo()
				ur.create_action("Snap PolygonVolume point")
				ur.add_do_property(node, "points", fixed)
				ur.add_undo_property(node, "points", pts)
				ur.commit_action()
			else:
				node.points = fixed
			pts = fixed
	elif dragging:
		_drag_idx = changed
		_show("")

	_last = pts.duplicate()
	_was_down = down
	if not down:
		_targets = []
		_drag_idx = -1
		if _label != null and not _snapped:
			_label.text = ""


## World X/Z of every other polygon volume, collected once per drag.
func _gather(node: Node3D) -> void:
	_targets = []
	var root = plugin.get_editor_interface().get_edited_scene_root() if plugin != null else node.get_tree().edited_scene_root
	if root == null:
		root = node.owner if node.owner != null else node.get_parent()
	if root == null:
		return
	var stack: Array = [root]
	while not stack.is_empty():
		var n: Node = stack.pop_back()
		if n != node and PolySnapPanel._is_polygon_volume(n) and (n as Node3D).is_visible_in_tree():
			var xf: Transform3D = n.global_transform
			var w := PackedVector2Array()
			for p in n.points:
				var g: Vector3 = xf * Vector3(p.x, 0, p.y)
				w.append(Vector2(g.x, g.z))
			_targets.append([str(n.name), w])
		for c in n.get_children():
			stack.append(c)


## Returns pts with point i pulled onto a nearby corner or edge (or unchanged).
func _snap_point(node: Node3D, pts: PackedVector2Array, i: int) -> PackedVector2Array:
	var xf: Transform3D = node.global_transform
	var g: Vector3 = xf * Vector3(pts[i].x, 0, pts[i].y)
	var w := Vector2(g.x, g.z)
	var best := distance
	var target = null
	var what := ""
	for t in _targets:
		var tp: PackedVector2Array = t[1]
		for k in range(tp.size()):
			var d := w.distance_to(tp[k])
			if d <= best:
				best = d
				target = tp[k]
				what = "%s point %d" % [t[0], k + 1]
	if target == null:
		for t in _targets:
			var tp: PackedVector2Array = t[1]
			for k in range(tp.size()):
				var on := Geometry2D.get_closest_point_to_segment(w, tp[k], tp[(k + 1) % tp.size()])
				var d2 := w.distance_to(on)
				if d2 <= best:
					best = d2
					target = on
					what = "%s edge %d-%d" % [t[0], k + 1, (k + 1) % tp.size() + 1]
	if target == null:
		_show("")
		return pts
	var l: Vector3 = xf.affine_inverse() * Vector3(target.x, g.y, target.y)
	var out := pts.duplicate()
	out[i] = Vector2(l.x, l.z)
	_show("Snapped to " + what)
	return out


func _show(text: String) -> void:
	_snapped = text != ""
	if _label != null:
		_label.text = text
