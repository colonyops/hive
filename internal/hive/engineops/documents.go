package engineops

import (
	"cmp"
	"context"
	"time"

	driveops "github.com/colonyops/hive/internal/ops/drive"
)

func (h *Handlers) DocumentList(ctx context.Context, in driveops.DocumentListIn) (driveops.DocumentListOut, error) {
	c, err := h.engine.DriveCaller(ctx, in.Caller)
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
	c, err := h.engine.DriveCaller(ctx, in.Caller)
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
	c, err := h.engine.DriveCaller(ctx, in.Caller)
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
	c, err := h.engine.DriveCaller(ctx, in.Caller)
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
	c, err := h.engine.DriveCaller(ctx, in.Caller)
	if err != nil {
		return driveops.DocumentDeleteOut{}, err
	}
	if err := h.engine.Drive().DeleteDocument(ctx, c, in.Path); err != nil {
		return driveops.DocumentDeleteOut{}, err
	}
	return driveops.DocumentDeleteOut{Path: in.Path, Deleted: true, At: time.Now()}, nil
}
