@tool
extends EditorPlugin
## Adds a "Polygons" tab next to 2D / 3D / Script.

const PolygonsPanel = preload("panel.gd")

var _panel = null


func _enter_tree() -> void:
	_panel = PolygonsPanel.new()
	_panel.plugin = self
	_panel.load_colors()
	get_editor_interface().get_editor_main_screen().add_child(_panel)
	_make_visible(false)
	scene_changed.connect(_on_scene_changed)


func _exit_tree() -> void:
	if scene_changed.is_connected(_on_scene_changed):
		scene_changed.disconnect(_on_scene_changed)
	if _panel != null:
		_panel.queue_free()
		_panel = null


func _has_main_screen() -> bool:
	return true


func _make_visible(visible: bool) -> void:
	if _panel == null:
		return
	_panel.visible = visible
	if visible:
		_panel.on_shown()


func _get_plugin_name() -> String:
	return "Polygons"


func _get_plugin_icon() -> Texture2D:
	var base := get_editor_interface().get_base_control()
	if base.has_theme_icon("CollisionPolygon2D", "EditorIcons"):
		return base.get_theme_icon("CollisionPolygon2D", "EditorIcons")
	return base.get_theme_icon("Node", "EditorIcons")


func _on_scene_changed(root) -> void:
	if _panel != null:
		_panel.on_scene_changed(root)
