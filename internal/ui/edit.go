package ui

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mlange-42/tramp/internal/track"
	"github.com/ncruces/zenity"
)

// maxUndo is the number of changes that can be undone.
const maxUndo = 100

// editState is the state of a file while it is edited.
type editState struct {
	// undo and redo are the states before the changes that can be undone, and after those that can be redone.
	undo, redo []fileSnapshot
	// rev identifies the current state of the routes and waypoints, savedRev the saved one.
	rev, savedRev, nextRev int
	// saved is the saved state, for discarding changes.
	saved fileSnapshot
	// overwriteOK is set once the user agreed to overwrite a file not written by TRAMP.
	overwriteOK bool
}

// fileSnapshot is a copy of the editable content of a file: its routes and waypoints.
// Tracks are not edited.
type fileSnapshot struct {
	routes    []track.Route
	waypoints []track.Waypoint
	rev       int
}

// snapshot returns a copy of the routes and waypoints of the file.
func (f *openFile) snapshot() fileSnapshot {
	return fileSnapshot{routes: f.data.Routes, waypoints: f.data.Waypoints, rev: f.edit.rev}.clone()
}

// restore sets the routes and waypoints of the file to those of a snapshot.
func (f *openFile) restore(s fileSnapshot) {
	// Copy, so that the snapshot stays intact when the file is changed.
	c := s.clone()
	f.data.Routes, f.data.Waypoints = c.routes, c.waypoints
	f.edit.rev = s.rev
	f.rebuild()
}

// clone returns a deep copy of the snapshot.
func (s fileSnapshot) clone() fileSnapshot {
	c := fileSnapshot{routes: slices.Clone(s.routes), waypoints: slices.Clone(s.waypoints), rev: s.rev}
	for i := range c.routes {
		c.routes[i].Points = slices.Clone(c.routes[i].Points)
	}
	return c
}

// dirty reports whether the file has unsaved changes.
func (f *openFile) dirty() bool {
	return f.edit.rev != f.edit.savedRev
}

// startEdit starts an edit session on the current state of the file, which is taken as saved.
func (f *openFile) startEdit() {
	f.edit = editState{overwriteOK: f.edit.overwriteOK}
	f.edit.saved = f.snapshot()
}

// change applies a change to the file data, which can be undone.
func (f *openFile) change(fn func(d *track.File)) {
	f.edit.undo = append(f.edit.undo, f.snapshot())
	if len(f.edit.undo) > maxUndo {
		f.edit.undo = slices.Delete(f.edit.undo, 0, 1)
	}
	f.edit.redo = nil
	fn(f.data)
	f.edit.nextRev++
	f.edit.rev = f.edit.nextRev
	f.rebuild()
}

// undo reverts the last change, and returns false if there is none.
func (f *openFile) undo() bool {
	n := len(f.edit.undo)
	if n == 0 {
		return false
	}
	f.edit.redo = append(f.edit.redo, f.snapshot())
	f.restore(f.edit.undo[n-1])
	f.edit.undo = f.edit.undo[:n-1]
	return true
}

// redo applies the last undone change again, and returns false if there is none.
func (f *openFile) redo() bool {
	n := len(f.edit.redo)
	if n == 0 {
		return false
	}
	f.edit.undo = append(f.edit.undo, f.snapshot())
	f.restore(f.edit.redo[n-1])
	f.edit.redo = f.edit.redo[:n-1]
	return true
}

// discard reverts all unsaved changes.
func (f *openFile) discard() {
	f.restore(f.edit.saved)
	f.edit.undo, f.edit.redo = nil, nil
}

// runOnUI runs fn on the UI goroutine, at the next frame. Safe for concurrent use.
func (a *App) runOnUI(fn func()) {
	a.bgMu.Lock()
	a.uiTasks = append(a.uiTasks, fn)
	a.bgMu.Unlock()
	a.invalidate()
}

// runUITasks runs the functions passed to [App.runOnUI].
func (a *App) runUITasks() {
	a.bgMu.Lock()
	tasks := a.uiTasks
	a.uiTasks = nil
	a.bgMu.Unlock()
	for _, fn := range tasks {
		fn()
	}
}

// dialog runs fn in the background, for showing dialogs without blocking the UI.
// Only one dialog is shown at a time: it returns false if another one is open.
// The function returned by fn, if not nil, then runs on the UI goroutine.
func (a *App) dialog(fn func() func()) bool {
	if !a.dialogOpen.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		then := fn()
		// Release before then runs, so that it can show the next dialog.
		a.dialogOpen.Store(false)
		if then != nil {
			a.runOnUI(then)
		}
	}()
	return true
}

// changed updates the map and the chart after the items of a file changed.
func (a *App) changed() {
	if a.selected != nil && !slices.ContainsFunc(a.files, func(of *openFile) bool { return slices.Contains(of.items, a.selected) }) {
		a.selected = nil
	}
	a.tracksChanged = true
}

// editable reports whether the file can be edited: a GPX file without recorded tracks.
func (f *openFile) editable() bool {
	return f.data != nil && f.data.Format == track.FormatGPX && len(f.data.Tracks) == 0 &&
		strings.EqualFold(filepath.Ext(f.path), ".gpx")
}

// toggleEdit starts or ends editing f. Unsaved changes of the edited file are saved or discarded first,
// as the user chooses.
func (a *App) toggleEdit(f *openFile) {
	if a.editing == f {
		a.resolveUnsaved(f, func() { a.setEditing(nil) })
		return
	}
	if !f.editable() {
		return
	}
	if a.editing != nil {
		a.resolveUnsaved(a.editing, func() { a.setEditing(f) })
		return
	}
	a.setEditing(f)
}

// setEditing makes f the edited file, or ends editing if it is nil. The edited file must not have unsaved changes.
func (a *App) setEditing(f *openFile) {
	if a.editing != nil {
		a.editing.edit.undo, a.editing.edit.redo = nil, nil
	}
	a.editing = f
	a.editor.reset()
	if f != nil {
		f.startEdit()
		f.visible.Value = true
		a.tracksChanged = true
	}
}

// closeFile closes f, after saving or discarding unsaved changes as the user chooses.
func (a *App) closeFile(f *openFile) {
	a.resolveUnsaved(f, func() {
		i := slices.Index(a.files, f)
		if i < 0 {
			return
		}
		if a.editing == f {
			a.setEditing(nil)
		}
		a.files = slices.Delete(a.files, i, i+1)
		a.changed()
	})
}

// resolveUnsaved asks whether to save or discard the unsaved changes of f, if it has any, and then calls done.
// done is not called if the user cancels, or saving fails.
func (a *App) resolveUnsaved(f *openFile, done func()) {
	a.commitProps()
	if a.editing != f || !f.dirty() {
		done()
		return
	}
	job := a.newSaveJob(f)
	a.dialog(func() func() {
		err := zenity.Question(fmt.Sprintf("Save changes to %s?", f.name), append([]zenity.Option{
			zenity.Title("Unsaved changes"),
			zenity.OKLabel("Save"),
			zenity.ExtraButton("Discard"),
			zenity.CancelLabel("Cancel"),
			zenity.WarningIcon,
		}, a.plat.dialogOptions()...)...)
		switch {
		case err == nil:
			saved := a.runSave(job)
			if saved == nil {
				return nil
			}
			return func() {
				saved()
				done()
			}
		case errors.Is(err, zenity.ErrExtraButton):
			return func() {
				f.discard()
				a.changed()
				done()
			}
		default:
			if !errors.Is(err, zenity.ErrCanceled) {
				log.Printf("unsaved changes dialog: %v", err)
			}
			return nil
		}
	})
}

// saveJob is the data for saving a file in the background.
type saveJob struct {
	file *openFile
	path string
	data track.File
	snap fileSnapshot
	// creator is the creator of a file not written by TRAMP, which needs confirmation to be overwritten.
	creator string
}

// newSaveJob prepares saving the current state of f.
func (a *App) newSaveJob(f *openFile) saveJob {
	j := saveJob{file: f, path: f.path, snap: f.snapshot()}
	// The data is written in the background, so it gets its own copy of the edited content.
	j.data = *f.data
	c := j.snap.clone()
	j.data.Routes, j.data.Waypoints = c.routes, c.waypoints
	if !f.edit.overwriteOK && f.data.Creator != track.Creator {
		j.creator = f.data.Creator
		if j.creator == "" {
			j.creator = "unknown software"
		}
	}
	return j
}

// save saves the edited file, if it has unsaved changes.
func (a *App) save() {
	a.commitProps()
	f := a.editing
	if f == nil || !f.dirty() {
		return
	}
	job := a.newSaveJob(f)
	a.dialog(func() func() { return a.runSave(job) })
}

// runSave writes the file of a save job, after asking to overwrite a file not written by TRAMP.
// It must run in a dialog, see [App.dialog]. It returns the function that updates the state
// on the UI goroutine, or nil if saving failed or was canceled.
func (a *App) runSave(j saveJob) func() {
	if j.creator != "" {
		err := zenity.Question(fmt.Sprintf("%s was written by %s.\n\nSaving rewrites it. "+
			"Content TRAMP doesn't read, like vendor extensions or links, is lost.", filepath.Base(j.path), j.creator),
			append([]zenity.Option{
				zenity.Title("Overwrite file"),
				zenity.OKLabel("Overwrite"),
				zenity.ExtraButton("Save as…"),
				zenity.CancelLabel("Cancel"),
				zenity.WarningIcon,
			}, a.plat.dialogOptions()...)...)
		switch {
		case err == nil:
		case errors.Is(err, zenity.ErrExtraButton):
			path, ok := a.askSavePath("Save as", j.path)
			if !ok {
				return nil
			}
			j.path = path
		default:
			if !errors.Is(err, zenity.ErrCanceled) {
				log.Printf("overwrite dialog: %v", err)
			}
			return nil
		}
	}
	j.data.Creator = track.Creator
	if err := track.WriteFile(j.path, &j.data); err != nil {
		log.Printf("saving: %v", err)
		_ = zenity.Error(err.Error(), append([]zenity.Option{zenity.Title("Saving failed")}, a.plat.dialogOptions()...)...)
		return nil
	}
	return func() {
		f := j.file
		f.data.Creator = track.Creator
		f.edit.overwriteOK = true
		f.edit.saved, f.edit.savedRev = j.snap, j.snap.rev
		if j.path != f.path {
			f.path, f.name = j.path, filepath.Base(j.path)
		}
	}
}

// askSavePath asks for a GPX file to save to, and adds the extension if missing.
// It must run in a dialog, see [App.dialog].
func (a *App) askSavePath(title, path string) (string, bool) {
	opts := append([]zenity.Option{
		zenity.Title(title),
		zenity.ConfirmOverwrite(),
		zenity.FileFilters{{Name: "GPX files", Patterns: []string{"*.gpx"}, CaseFold: true}},
	}, a.plat.dialogOptions()...)
	if path != "" {
		opts = append(opts, zenity.Filename(path))
	}
	p, err := zenity.SelectFileSave(opts...)
	if err != nil {
		if !errors.Is(err, zenity.ErrCanceled) {
			log.Printf("save dialog: %v", err)
		}
		return "", false
	}
	if !strings.EqualFold(filepath.Ext(p), ".gpx") {
		p += ".gpx"
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p, true
}

// newFile asks for a file name, creates an empty GPX file and opens it for editing.
func (a *App) newFile() {
	a.dialog(func() func() {
		path, ok := a.askSavePath("New file", "")
		if !ok {
			return nil
		}
		data := &track.File{Format: track.FormatGPX, Creator: track.Creator}
		if err := track.WriteFile(path, data); err != nil {
			log.Printf("creating file: %v", err)
			_ = zenity.Error(err.Error(), append([]zenity.Option{zenity.Title("Creating file failed")}, a.plat.dialogOptions()...)...)
			return nil
		}
		return func() {
			// The file was overwritten, so drop an opened old version without asking.
			if i := slices.IndexFunc(a.files, func(f *openFile) bool { return f.path == path }); i >= 0 {
				if a.editing == a.files[i] {
					a.editing = nil
				}
				a.files = slices.Delete(a.files, i, i+1)
			}
			f := newOpenFile(path, data)
			f.setColors(nil, trackColors[a.nextColor%len(trackColors)])
			a.nextColor++
			a.files = append([]*openFile{f}, a.files...)
			a.changed()
			a.toggleEdit(f)
		}
	})
}

// undo reverts the last change of the edited file.
func (a *App) undo() {
	if f := a.editing; f != nil && f.undo() {
		a.changed()
	}
}

// redo applies the last undone change of the edited file again.
func (a *App) redo() {
	if f := a.editing; f != nil && f.redo() {
		a.changed()
	}
}

// confirmClose reports whether the window may close. If there are unsaved changes,
// it asks to save or discard them, and closes the window afterwards.
func (a *App) confirmClose() bool {
	if a.closeOK || a.editing == nil || !a.editing.dirty() {
		return true
	}
	a.resolveUnsaved(a.editing, func() {
		a.closeOK = true
		a.closeWindow()
	})
	return false
}
