@tool
extends EditorPlugin
## Adds a "PolySnap" tab next to 2D / 3D / Script.

const PolySnapPanel = preload("polysnap_panel.gd")
const PointSnap = preload("polysnap_point_snap.gd")
const TabIcon = preload("polysnap_tab_icon.gd")

var _panel = null
var _snap = null
var _snap_bar: Control = null
var _tab_icon = null


func _enter_tree() -> void:
	_panel = PolySnapPanel.new()
	_panel.plugin = self
	_panel.load_colors()
	get_editor_interface().get_editor_main_screen().add_child(_panel)
	_make_visible(false)
	scene_changed.connect(_on_scene_changed)
	# Live point snapping toggle in the 3D view's toolbar.
	_snap = PointSnap.new()
	_snap.plugin = self
	add_child(_snap)
	_snap_bar = _snap.make_toolbar()
	add_control_to_container(CONTAINER_SPATIAL_EDITOR_MENU, _snap_bar)
	# Animated tab icon (the open map, dragging into a "P" on hover).
	_tab_icon = TabIcon.new()
	_tab_icon.plugin = self
	add_child(_tab_icon)
	_tab_icon.load_setting()
	_panel.sync_map_icon_box()


func _exit_tree() -> void:
	if scene_changed.is_connected(_on_scene_changed):
		scene_changed.disconnect(_on_scene_changed)
	if _panel != null:
		_panel.queue_free()
		_panel = null
	if _snap_bar != null:
		remove_control_from_container(CONTAINER_SPATIAL_EDITOR_MENU, _snap_bar)
		_snap_bar.queue_free()
		_snap_bar = null
	if _snap != null:
		_snap.queue_free()
		_snap = null
	if _tab_icon != null:
		_tab_icon.restore()
		_tab_icon.queue_free()
		_tab_icon = null


func _has_main_screen() -> bool:
	return true


func _make_visible(visible: bool) -> void:
	if _panel == null:
		return
	_panel.visible = visible
	if visible:
		_panel.on_shown()


func _get_plugin_name() -> String:
	return "PolySnap"


func _get_plugin_icon() -> Texture2D:
	var base := get_editor_interface().get_base_control()
	if base.has_theme_icon("CollisionPolygon2D", "EditorIcons"):
		return base.get_theme_icon("CollisionPolygon2D", "EditorIcons")
	return base.get_theme_icon("Node", "EditorIcons")


## The "Map icon" box in the PolySnap tab: animated map icon on, or the plain "PolySnap" tab.
func set_map_icon(on: bool) -> void:
	if _tab_icon != null:
		_tab_icon.set_map_icon(on)


func map_icon_on() -> bool:
	return _tab_icon != null and _tab_icon.map_icon


## The PolySnap tab's Rotate button was pressed: turn the tab icon the same way.
func on_view_rotated() -> void:
	if _tab_icon != null:
		_tab_icon.refresh_map()


func _on_scene_changed(root) -> void:
	if _panel != null:
		_panel.on_scene_changed(root)
	if _tab_icon != null:
		_tab_icon.schedule_refresh()
