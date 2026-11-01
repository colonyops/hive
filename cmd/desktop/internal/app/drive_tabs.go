package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
)

// TabView names a view that opens as a tab beside a chat or Code session.
type TabView string

const (
	TabViewDrive TabView = "drive"
	TabViewDiff  TabView = "diff"
	TabViewTasks TabView = "tasks"
)

func parseTabView(s string) (TabView, error) {
	switch v := TabView(s); v {
	case TabViewDrive, TabViewDiff, TabViewTasks:
		return v, nil
	}
	return "", Errorf(KindInvalid, "view %q is not one of drive, diff, tasks", s)
}

// TabPlacement says where an opened tab goes: behind the tab in front, or in
// a split beside it.
type TabPlacement string

const (
	TabPlacementBehind TabPlacement = "behind"
	TabPlacementBeside TabPlacement = "beside"
)

// parseTabPlacement reads an empty placement as behind, the one that never
// takes the screen.
func parseTabPlacement(s string) (TabPlacement, error) {
	switch p := TabPlacement(s); p {
	case "":
		return TabPlacementBehind, nil
	case TabPlacementBehind, TabPlacementBeside:
		return p, nil
	}
	return "", Errorf(KindInvalid, "placement %q is not one of behind, beside", s)
}

// OpenTab asks the UI to open one tab beside the calling chat or Code
// session and answers the document path it resolved, empty when none was
// named. It is UI intent, applied only while the user is viewing that chat
// or session, so an agent can surface what it made without taking the screen
// from the user. A path names an existing document or canvas of the Drive
// view; a canvas may be given by name.
func (s *DriveService) OpenTab(ctx context.Context, session, view, path, placement string) (string, error) {
	v, err := parseTabView(view)
	if err != nil {
		return "", err
	}
	p, err := parseTabPlacement(placement)
	if err != nil {
		return "", err
	}
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return "", err
	}
	if err := checkTabCaller(caller, v); err != nil {
		return "", err
	}
	resolved := ""
	switch {
	case path != "" && v != TabViewDrive:
		return "", Errorf(KindInvalid, "path applies only to the drive view; %s has no document to pick", v)
	case path != "":
		if resolved, err = s.resolveTabPath(ctx, d, caller.space, path); err != nil {
			return "", err
		}
	}
	s.events.Publish(ctx, events.SessionTabRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, View: string(v), Path: resolved, Placement: string(p), Open: true,
	})
	return resolved, nil
}

// CloseTab asks the UI to close the calling session's tab for view, wherever
// it sits. Documents and canvases are untouched.
func (s *DriveService) CloseTab(ctx context.Context, session, view string) error {
	v, err := parseTabView(view)
	if err != nil {
		return err
	}
	_, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	if err := checkTabCaller(caller, v); err != nil {
		return err
	}
	s.events.Publish(ctx, events.SessionTabRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, View: string(v),
	})
	return nil
}

func checkTabCaller(caller driveCaller, view TabView) error {
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "the shared space sits beside no chat or Code session, so it has no tabs; the user reads it in the Drive page")
	}
	if caller.author.Session != "" && view != TabViewDrive {
		return Errorf(KindInvalid, "a chat has only a Drive tab; Diff and Tasks belong to a Code session")
	}
	return nil
}

// resolveTabPath maps what an agent typed to the path the Drive tab shows: a
// document at that path, else the canvas that name or path stands for.
func (s *DriveService) resolveTabPath(ctx context.Context, d *drivesvc.Service, space drive.Space, path string) (string, error) {
	if err := drive.ValidatePath(path); err != nil {
		return "", documentError(err, space, path)
	}
	isDir, err := s.isDir(d, space, path)
	switch {
	case err == nil && isDir:
		return "", Errorf(KindInvalid, "%q is a folder; open a document inside it", path)
	case err == nil:
		return path, nil
	case KindOf(err) != KindNotFound:
		return "", err
	}
	cp, err := canvasPath(path)
	if err != nil {
		return "", documentNotFound(space, path)
	}
	c, ok, err := d.LoadCanvas(ctx, space, cp)
	if err != nil {
		return "", driveError(err, path)
	}
	if !ok {
		return "", documentNotFound(space, path)
	}
	return c.Path, nil
}
