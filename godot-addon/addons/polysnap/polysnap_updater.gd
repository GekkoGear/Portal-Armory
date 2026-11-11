@tool
extends Node
## "Check for Update" for PolySnap.
##
## Goes online ONLY when the user presses Check for Update, and only to read the
## releases of the GitHub repo below and, if there is a newer version, download
## its zip. Nothing else in the add-on uses the internet.
##
## Steps:
##   1. Read the installed version from the chosen folder's plugin.cfg.
##   2. Ask GitHub for the releases, pick the newest one tagged "polysnap-rev<version>" (e.g. polysnap-rev3.7.1).
##   3. Only if it is newer: download its .zip to a temporary file, and check that
##      it really is PolySnap (right name, right version, only plain files).
##   4. Back up the old tool files, delete ONLY the tool's own files (the list in
##      its manifest.txt, plus the .uid files Godot makes next to them), write the
##      new files, and restart the add-on.
## Nothing outside the tool's own file list is ever deleted.

## Off until releases are being published. Flip to true to turn the button on.
const UPDATES_ENABLED := false

const REPO := "GekkoGear/portal-armory"
const TAG_PREFIX := "polysnap-rev"
const PLUGIN_NAME := "PolySnap"
## Earlier name of this add-on; installs that still carry it can be updated too.
const ZIP_FOLDER := "polysnap/"
## Files of versions that came before manifest.txt existed.
const KNOWN_FILES := ["README.md", "manifest.txt", "plugin.cfg", "polysnap_align.gd", "polysnap_panel.gd", "polysnap_plugin.gd", "polysnap_point_snap.gd", "polysnap_tab_icon.gd", "polysnap_updater.gd", "polysnap_view.gd"]
const DOWNLOAD_PATH := "user://polysnap_update.zip"
const BACKUP_DIR := "user://polysnap_backup"

var plugin: EditorPlugin = null
## Tests point this at a local server instead of GitHub.
var api_base := "https://api.github.com"

var _dialog: AcceptDialog
var _path: LineEdit
var _check: Button
var _status: Label
var _browse_dialog = null
var _http: HTTPRequest
var _pending := {}


# ============================================================ pure helpers

## -1 if a < b, 0 if equal, 1 if a > b ("3.10" > "3.9", "3.0" == "3.0.0").
static func compare_versions(a: String, b: String) -> int:
	var pa := _clean(a).split(".")
	var pb := _clean(b).split(".")
	for i in range(maxi(pa.size(), pb.size())):
		var x := int(pa[i]) if i < pa.size() else 0
		var y := int(pb[i]) if i < pb.size() else 0
		if x != y:
			return -1 if x < y else 1
	return 0


static func _clean(v: String) -> String:
	v = v.strip_edges().to_lower()
	for p in ["rev", "v"]:
		v = v.trim_prefix(p).strip_edges()
	return v


static func _is_version(s: String) -> bool:
	if s == "":
		return false
	for part in s.split("."):
		if part == "" or not part.is_valid_int():
			return false
	return true


## From GitHub's release list, the newest published PolySnap release
## that has a .zip attached: { version, tag, zip_url } or {} if none.
static func pick_release(releases) -> Dictionary:
	var best := {}
	if typeof(releases) != TYPE_ARRAY:
		return best
	for r in releases:
		if typeof(r) != TYPE_DICTIONARY or r.get("draft", false) or r.get("prerelease", false):
			continue
		var tag: String = str(r.get("tag_name", ""))
		if not tag.begins_with(TAG_PREFIX):
			continue
		var ver := tag.trim_prefix(TAG_PREFIX)
		if not _is_version(ver):
			continue
		var url := ""
		for a in r.get("assets", []):
			if typeof(a) == TYPE_DICTIONARY and str(a.get("name", "")).to_lower().ends_with(".zip"):
				url = str(a.get("browser_download_url", ""))
				break
		if url == "":
			continue
		if best.is_empty() or compare_versions(ver, best.version) > 0:
			best = {"version": ver, "tag": tag, "zip_url": url}
	return best


## Reads plugin.cfg in folder (an absolute path). { ok, version, error }
static func read_installed(folder: String) -> Dictionary:
	var cfg := ConfigFile.new()
	if cfg.load(folder.path_join("plugin.cfg")) != OK:
		return {"ok": false, "error": "There's no plugin.cfg in that folder. Pick your project's addons/polysnap folder."}
	var nm := str(cfg.get_value("plugin", "name", ""))
	if nm != PLUGIN_NAME:
		return {"ok": false, "error": "That folder holds a different add-on (%s), not PolySnap. Nothing was changed." % str(cfg.get_value("plugin", "name", "?"))}
	var ver := _clean(str(cfg.get_value("plugin", "version", "")))
	if not _is_version(ver):
		return {"ok": false, "error": "Couldn't read the installed version from plugin.cfg."}
	return {"ok": true, "version": ver}


## Opens and checks a downloaded zip. Everything must sit directly inside a
## polysnap/ folder (no sub-folders, no ".." tricks), and its plugin.cfg
## must be PolySnap at the expected version.
## { ok, files: { name: PackedByteArray }, error }
static func read_zip(zip_path: String, expect_version: String) -> Dictionary:
	var z := ZIPReader.new()
	if z.open(zip_path) != OK:
		return {"ok": false, "error": "The download isn't a readable zip."}
	var files := {}
	for entry in z.get_files():
		if entry.ends_with("/"):
			if entry == ZIP_FOLDER:
				continue
			z.close()
			return {"ok": false, "error": "The download has an unexpected folder (%s). Nothing was changed." % entry}
		if not entry.begins_with(ZIP_FOLDER):
			z.close()
			return {"ok": false, "error": "The download has a file outside polysnap (%s). Nothing was changed." % entry}
		var name := entry.trim_prefix(ZIP_FOLDER)
		if name == "" or name.contains("/") or name.contains("\\") or name.contains("..") or name.begins_with("."):
			z.close()
			return {"ok": false, "error": "The download has an unexpected file name (%s). Nothing was changed." % entry}
		files[name] = z.read_file(entry)
	z.close()
	if not files.has("plugin.cfg"):
		return {"ok": false, "error": "The download is missing plugin.cfg. Nothing was changed."}
	var cfg := ConfigFile.new()
	if cfg.parse(files["plugin.cfg"].get_string_from_utf8()) != OK or str(cfg.get_value("plugin", "name", "")) != PLUGIN_NAME:
		return {"ok": false, "error": "The download isn't PolySnap. Nothing was changed."}
	var script_file := str(cfg.get_value("plugin", "script", ""))
	if script_file == "" or not files.has(script_file):
		return {"ok": false, "error": "The download is missing its main script (%s). Nothing was changed." % script_file}
	var ver := str(cfg.get_value("plugin", "version", ""))
	if expect_version != "" and compare_versions(ver, expect_version) != 0:
		return {"ok": false, "error": "The download says it is Rev %s, but the release is Rev %s. Nothing was changed." % [ver, expect_version]}
	return {"ok": true, "files": files, "version": ver}


## The tool's own files in folder: the names in its manifest.txt (or, for older
## versions without one, KNOWN_FILES), plus the .uid file Godot keeps next to
## each. Only plain file names that really exist in the folder are returned.
static func tool_files(folder: String) -> Array:
	var names: Array = []
	var mf := FileAccess.open(folder.path_join("manifest.txt"), FileAccess.READ)
	if mf != null:
		for line in mf.get_as_text().split("\n"):
			var n := line.strip_edges()
			if n != "" and not n.begins_with("#"):
				names.append(n)
		mf.close()
	else:
		names = KNOWN_FILES.duplicate()
	var out: Array = []
	for n in names:
		if n.contains("/") or n.contains("\\") or n.contains("..") or n.begins_with("."):
			continue   # never anything outside this folder
		for f in [n, n + ".uid"]:
			if FileAccess.file_exists(folder.path_join(f)) and not out.has(f):
				out.append(f)
	return out


## Replaces the tool's files in folder with new_files. Backs the old ones up
## first and puts them back if anything fails. { ok, deleted, written, error }
static func install(folder: String, new_files: Dictionary, backup_dir: String) -> Dictionary:
	var old := tool_files(folder)
	# 1. back up
	DirAccess.make_dir_recursive_absolute(backup_dir)
	for f in old:
		var src := folder.path_join(f)
		if DirAccess.copy_absolute(src, backup_dir.path_join(f)) != OK:
			return {"ok": false, "error": "Couldn't back up %s, so nothing was changed." % f}
	# 2. delete only the tool's own files
	var deleted: Array = []
	for f in old:
		if DirAccess.remove_absolute(folder.path_join(f)) == OK:
			deleted.append(f)
	# 3. write the new files
	var written: Array = []
	for name in new_files.keys():
		var fa := FileAccess.open(folder.path_join(name), FileAccess.WRITE)
		if fa == null:
			_restore(folder, backup_dir, old, written)
			return {"ok": false, "error": "Couldn't write %s. Your previous version was put back." % name}
		fa.store_buffer(new_files[name])
		fa.close()
		written.append(name)
	return {"ok": true, "deleted": deleted, "written": written}


static func _restore(folder: String, backup_dir: String, old: Array, written: Array) -> void:
	for f in written:
		DirAccess.remove_absolute(folder.path_join(f))
	for f in old:
		DirAccess.copy_absolute(backup_dir.path_join(f), folder.path_join(f))


# ===================================================================== UI

func _build() -> void:
	_dialog = AcceptDialog.new()
	_dialog.title = "PolySnap - Check for Update"
	_dialog.ok_button_text = "Close"
	_dialog.files_dropped.connect(_on_files_dropped)
	add_child(_dialog)
	var box := VBoxContainer.new()
	box.custom_minimum_size = Vector2(560, 0)
	_dialog.add_child(box)
	var info := Label.new()
	info.text = "Checks GitHub for a newer PolySnap and installs it into this folder.\nThis is the only time the add-on goes online. Drag your polysnap folder here, or press Browse."
	info.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	box.add_child(info)
	var row := HBoxContainer.new()
	box.add_child(row)
	_path = LineEdit.new()
	_path.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_path.placeholder_text = "Path to your addons/polysnap folder"
	_path.text_changed.connect(_on_path_changed)
	row.add_child(_path)
	var browse := Button.new()
	browse.text = "Browse..."
	browse.pressed.connect(_on_browse)
	row.add_child(browse)
	_check = Button.new()
	_check.text = "Check for Update"
	_check.pressed.connect(_on_check_pressed)
	box.add_child(_check)
	_status = Label.new()
	_status.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	box.add_child(_status)
	# Made by name so this script still loads where editor classes don't exist.
	_browse_dialog = ClassDB.instantiate("EditorFileDialog")
	_browse_dialog.file_mode = 2   # open a folder
	_browse_dialog.access = 2      # anywhere on the computer
	_browse_dialog.title = "Pick your polysnap folder"
	_browse_dialog.dir_selected.connect(_set_folder)
	add_child(_browse_dialog)


func _on_check_pressed() -> void:
	check_for_update(_path.text.strip_edges())


## This copy's own folder, as an absolute path.
func own_folder() -> String:
	return ProjectSettings.globalize_path(get_script().resource_path.get_base_dir())


func open() -> void:
	if _dialog == null:
		_build()
	if _path.text == "":
		_set_folder(own_folder())
	_dialog.popup_centered()


func _set_folder(p: String) -> void:
	_path.text = p
	_on_path_changed(p)


func _on_path_changed(p: String) -> void:
	var r := read_installed(p.strip_edges())
	_status.text = ("Installed here: Rev %s" % r.version) if r.ok else r.error


func _on_files_dropped(files: PackedStringArray) -> void:
	if files.is_empty():
		return
	var p: String = files[0]
	if not DirAccess.dir_exists_absolute(p):
		p = p.get_base_dir()   # a file from inside the folder was dropped
	_set_folder(p)


func _on_browse() -> void:
	_browse_dialog.popup_centered_ratio(0.6)


func _say(text: String) -> void:
	if _status != null:
		_status.text = text
	print("PolySnap update: " + text)


# =============================================================== online

## Step 1 + 2: read the installed version, then ask GitHub for releases.
func check_for_update(folder: String = "") -> void:
	if _http != null:
		return   # already checking
	if folder == "":
		folder = _path.text.strip_edges() if _path != null else own_folder()
	var inst := read_installed(folder)
	if not inst.ok:
		_say(inst.error)
		return
	_pending = {"folder": folder, "installed": inst.version}
	_say("Installed: Rev %s. Checking GitHub..." % inst.version)
	if _check != null:
		_check.disabled = true
	_http = HTTPRequest.new()
	_http.max_redirects = 8
	_http.timeout = 30.0
	add_child(_http)
	_http.request_completed.connect(_on_releases)
	var err := _http.request("%s/repos/%s/releases?per_page=100" % [api_base, REPO], _headers())
	if err != OK:
		_done("Couldn't start the check (error %d). You may be offline." % err)


func _headers() -> PackedStringArray:
	return PackedStringArray(["User-Agent: PolySnap-Updater", "Accept: application/vnd.github+json"])


func _on_releases(result: int, code: int, _h: PackedStringArray, body: PackedByteArray) -> void:
	_http.queue_free()
	_http = null
	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		_done("Couldn't reach GitHub (result %d, HTTP %d). You may be offline; try again later." % [result, code])
		return
	var rel := pick_release(JSON.parse_string(body.get_string_from_utf8()))
	if rel.is_empty():
		_done("No PolySnap release is published yet. You have Rev %s." % _pending.installed)
		return
	if compare_versions(rel.version, _pending.installed) <= 0:
		_done("You're up to date (Rev %s). Nothing was downloaded." % _pending.installed)
		return
	# Step 3: download.
	_pending["release"] = rel
	_say("Rev %s is out (you have Rev %s). Downloading..." % [rel.version, _pending.installed])
	_http = HTTPRequest.new()
	_http.max_redirects = 8
	_http.timeout = 120.0
	_http.download_file = DOWNLOAD_PATH
	add_child(_http)
	_http.request_completed.connect(_on_downloaded)
	var err := _http.request(rel.zip_url, _headers())
	if err != OK:
		_done("Couldn't start the download (error %d)." % err)


func _on_downloaded(result: int, code: int, _h: PackedStringArray, _body: PackedByteArray) -> void:
	_http.queue_free()
	_http = null   # offline again from here on
	if result != HTTPRequest.RESULT_SUCCESS or code != 200:
		_done("The download failed (result %d, HTTP %d). Nothing was changed." % [result, code])
		return
	var rel: Dictionary = _pending.release
	var z := read_zip(ProjectSettings.globalize_path(DOWNLOAD_PATH), rel.version)
	if not z.ok:
		_cleanup_download()
		_done(z.error)
		return
	# Step 4: replace the tool's files.
	var folder: String = _pending.folder
	var res := install(folder, z.files, ProjectSettings.globalize_path(BACKUP_DIR).path_join(_pending.installed))
	_cleanup_download()
	if not res.ok:
		_done(res.error)
		return
	_done("Updated from Rev %s to Rev %s. Replaced %d tool file(s); nothing else was touched." % [_pending.installed, rel.version, res.written.size()])
	if plugin != null:
		var ei = plugin.get_editor_interface()
		ei.get_resource_filesystem().scan()
		if folder.simplify_path() == own_folder().simplify_path():
			# Restart this add-on so the new version loads (no need to reopen Godot).
			var name := own_folder().get_file()
			ei.call_deferred("set_plugin_enabled", name, false)
			ei.call_deferred("set_plugin_enabled", name, true)


func _cleanup_download() -> void:
	DirAccess.remove_absolute(ProjectSettings.globalize_path(DOWNLOAD_PATH))


func _done(text: String) -> void:
	_say(text)
	if _check != null:
		_check.disabled = false
