# Editing

GPX files with routes and waypoints can be edited, files with recorded tracks are read-only.
A file can contain several routes, as well as waypoints.
Create a new file with the page button or Ctrl+N, or click the pencil of an opened file to edit it.
One file at a time is in edit mode. Unsaved changes are marked with `*` after the file name.

## Tools

In edit mode, the toolbar shows the edit tools, also selected with the keys S, W and R:

- **Select**: click a point to select it, drag it to move it, and press Del to delete it.
  Drag or click the small circles between route points to insert a point.
  Route points dropped on a waypoint snap to it and are linked to it; hold Shift to not snap.
  Esc or a right click clears the selection.
- **Waypoint**: click to add a waypoint.
- **Route**: click to add points to a new route. Click the first or last point of a route,
  or select the route in the side panel, to continue it.
  Esc, Enter, a right click or a double click finishes the route, Del removes the last point.
  Clicking a waypoint adds it to the route, linked to it (see [below](#linked-route-points)); unnamed waypoints get a name like `WP001`.
  The pointer snaps to waypoints, which are highlighted. Hold Shift to place a point without snapping.

Hover a tool for a short help.
Moved and added points have no elevation.

## Properties

The names of the points of the edited file are shown on the map.
Below the file list, the *Properties* box shows the name and description of the selected route,
route point or waypoint. Changes apply on Enter, Tab or when clicking elsewhere, and Esc reverts them.
The box also has a button to delete the whole route.

## Linked route points

Many GPS devices build routes from stored waypoints, and write each route point
with the same name and position as its waypoint.
TRAMP treats such points as linked: they share one handle on the map and are moved together.
Hold Shift while dragging a linked route point to move it alone.
Released with Shift held, it is unlinked and loses the waypoint's name, description and symbol.
Deleting a linked route point removes it from the route only.

## Saving

Save with Ctrl+S. Undo and redo with the toolbar buttons, or with Ctrl+Z and Ctrl+Y or Ctrl+Shift+Z.
When leaving edit mode, closing the file or closing TRAMP with unsaved changes,
TRAMP asks whether to save or discard them.
Before overwriting a file written by other software for the first time, TRAMP asks for confirmation,
as content it doesn't read, like vendor extensions, is lost.
