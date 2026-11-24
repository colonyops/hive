package engineops

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/ops"
	driveops "github.com/colonyops/hive/internal/ops/drive"
)

// driveCaller is the Drive caller of a document operation. A path inside no
// session is the repository of its git remote, the space the sessions of that
// repository use; a directory with no such remote stays not_found.
func (h *Handlers) driveCaller(ctx context.Context, value string) (drivesvc.Caller, error) {
	c, err := h.engine.DriveCaller(ctx, value)
	dir := strings.TrimSpace(value)
	opErr, ok := errors.AsType[*ops.Error](err)
	if err == nil || !ok || opErr.Kind != ops.KindNotFound || !filepath.IsAbs(dir) {
		return c, err
	}
	remote, remoteErr := h.engine.Git().RemoteURL(ctx, dir)
	if remoteErr != nil {
		h.log.Debug().Err(remoteErr).Str("dir", dir).Msg("no git remote for the drive caller's directory")
		return drivesvc.Caller{}, err
	}
	space, found := h.engine.Drive().SpaceForRemote(remote)
	if !found {
		return drivesvc.Caller{}, err
	}
	return drivesvc.Caller{Space: space}, nil
}

func (h *Handlers) DocumentList(ctx context.Context, in driveops.DocumentListIn) (driveops.DocumentListOut, error) {
	c, err := h.driveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentListOut{}, err
	}
	detail := cmp.Or(in.Detail, driveops.DetailChildren)
	entry, err := h.engine.Drive().ListDocuments(ctx, c, in.Path, detail == driveops.DetailTree)
	if err != nil {
		return driveops.DocumentListOut{}, err
	}
	children := make([]any, 0, len(entry.Children))
	for _, child := range entry.Children {
		children = append(children, driveops.NodeOf(child))
	}
	return driveops.DocumentListOut{Space: c.Space.Key, DocumentEntry: driveops.EntryOf(entry), Children: children, Detail: detail}, nil
}

func (h *Handlers) DocumentGet(ctx context.Context, in driveops.DocumentGetIn) (driveops.DocumentGetOut, error) {
	c, err := h.driveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentGetOut{}, err
	}
	entry, body, err := h.engine.Drive().GetDocument(ctx, c, in.Path)
	if err != nil {
		return driveops.DocumentGetOut{}, err
	}
	return driveops.DocumentGetOut{DocumentEntry: driveops.EntryOf(entry), Body: string(body)}, nil
}

func (h *Handlers) DocumentPut(ctx context.Context, in driveops.DocumentPutIn) (driveops.DocumentEntry, error) {
	c, err := h.driveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentEntry{}, err
	}
	entry, err := h.engine.Drive().PutDocument(ctx, c, in.Path, []byte(in.Body))
	if err != nil {
		return driveops.DocumentEntry{}, err
	}
	return driveops.EntryOf(entry), nil
}

func (h *Handlers) DocumentMove(ctx context.Context, in driveops.DocumentMoveIn) (driveops.DocumentEntry, error) {
	c, err := h.driveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentEntry{}, err
	}
	entry, err := h.engine.Drive().MoveDocument(ctx, c, in.From, in.To)
	if err != nil {
		return driveops.DocumentEntry{}, err
	}
	return driveops.EntryOf(entry), nil
}

func (h *Handlers) DocumentDelete(ctx context.Context, in driveops.DocumentDeleteIn) (driveops.DocumentDeleteOut, error) {
	c, err := h.driveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentDeleteOut{}, err
	}
	if err := h.engine.Drive().DeleteDocument(ctx, c, in.Path); err != nil {
		return driveops.DocumentDeleteOut{}, err
	}
	return driveops.DocumentDeleteOut{Path: in.Path, Deleted: true, At: time.Now()}, nil
}
