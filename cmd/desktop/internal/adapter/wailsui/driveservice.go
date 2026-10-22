package wailsui

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/pkg/logutils"
)

var tracer = observe.Tracer("/internal/adapter/wailsui")

// DriveService exposes Hive Drive to the frontend: spaces, trees, search,
// the reader, and the user's own reorganization. Every space argument is a
// space key ("owner/repo", "workspaces/<name>", "shared") and every path is
// relative to that space's root.
type DriveService struct {
	drive  *app.DriveService
	logger zerolog.Logger
}

func NewDriveService(d *app.DriveService, logger zerolog.Logger) *DriveService {
	return &DriveService{drive: d, logger: logutils.Component(logger, "wails.drive")}
}

func (s *DriveService) Spaces(ctx context.Context) ([]SpaceWire, error) {
	spaces, err := s.drive.Spaces(ctx)
	if err != nil {
		return nil, s.fail(ctx, "spaces", err)
	}
	return spaceWiresOf(spaces), nil
}

func (s *DriveService) Tree(ctx context.Context, space string) (EntryWire, error) {
	root, err := s.drive.Tree(ctx, space)
	if err != nil {
		return EntryWire{}, s.fail(ctx, "tree", err)
	}
	return entryWireOf(root), nil
}

// Search matches query against paths and titles in every space. limit 0 is
// the drive's default cap.
func (s *DriveService) Search(ctx context.Context, query string, limit int) ([]HitWire, error) {
	hits, err := s.drive.Search(ctx, query, limit)
	if err != nil {
		return nil, s.fail(ctx, "search", err)
	}
	return hitWiresOf(hits), nil
}

func (s *DriveService) Document(ctx context.Context, space, path string) (_ DocumentWire, err error) {
	ctx, span := startCommand(ctx, "open", space, path)
	defer observe.End(span, &err)
	doc, err := s.drive.Document(ctx, space, path)
	if err != nil {
		return DocumentWire{}, s.fail(ctx, "open", err)
	}
	return documentWireOf(doc), nil
}

// Markdown is a markdown document's body or a canvas rendered as markdown.
func (s *DriveService) Markdown(ctx context.Context, space, path string) (string, error) {
	markdown, err := s.drive.Markdown(ctx, space, path)
	return markdown, s.fail(ctx, "markdown", err)
}

// Export writes Markdown's answer to dest, an absolute path from the native
// save dialog.
func (s *DriveService) Export(ctx context.Context, space, path, dest string) (err error) {
	ctx, span := startCommand(ctx, "export", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "export", s.drive.Export(ctx, space, path, dest))
}

func (s *DriveService) Delete(ctx context.Context, space, path string) (err error) {
	ctx, span := startCommand(ctx, "delete", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "delete", s.drive.Delete(ctx, space, path))
}

func (s *DriveService) CreateFolder(ctx context.Context, space, path string) (_ EntryWire, err error) {
	ctx, span := startCommand(ctx, "create-folder", space, path)
	defer observe.End(span, &err)
	entry, err := s.drive.CreateFolder(ctx, space, path)
	if err != nil {
		return EntryWire{}, s.fail(ctx, "create-folder", err)
	}
	return entryWireOf(entry), nil
}

// Rename gives path a new name in the same folder. A document keeps its
// kind's extension: "b" renames a.md to b.md.
func (s *DriveService) Rename(ctx context.Context, space, path, name string) (_ EntryWire, err error) {
	ctx, span := startCommand(ctx, "rename", space, path)
	defer observe.End(span, &err)
	entry, err := s.drive.Rename(ctx, space, path, name)
	if err != nil {
		return EntryWire{}, s.fail(ctx, "rename", err)
	}
	return entryWireOf(entry), nil
}

// Move puts path into the folder dir under its own name; dir "" is the
// space root.
func (s *DriveService) Move(ctx context.Context, space, path, dir string) (_ EntryWire, err error) {
	ctx, span := startCommand(ctx, "move", space, path)
	defer observe.End(span, &err)
	entry, err := s.drive.Move(ctx, space, path, dir)
	if err != nil {
		return EntryWire{}, s.fail(ctx, "move", err)
	}
	return entryWireOf(entry), nil
}

// Locate is the absolute path on disk of path in space; path "" is the
// space's own folder.
func (s *DriveService) Locate(ctx context.Context, space, path string) (string, error) {
	full, err := s.drive.Locate(ctx, space, path)
	return full, s.fail(ctx, "locate", err)
}

func (s *DriveService) Reveal(ctx context.Context, space, path string) (err error) {
	ctx, span := startCommand(ctx, "reveal", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "reveal", s.drive.Reveal(ctx, space, path))
}

// startCommand roots a trigger span for a command the user gave. The reads
// the frontend repeats on every drive change get none
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
func startCommand(ctx context.Context, command, space, path string) (context.Context, trace.Span) {
	return tracer.Start(ctx, "ui.drive "+command,
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("drive.space", space), attribute.String("drive.path", path)),
	)
}

// fail logs what MarshalError hands the frontend without a trace in the log.
func (s *DriveService) fail(ctx context.Context, command string, err error) error {
	if err == nil {
		return nil
	}
	if kind := app.KindOf(err); kind != app.KindInternal {
		s.logger.Debug().Ctx(ctx).Err(err).Str("command", command).Str("kind", string(kind)).Msg("drive call refused")
		return err
	}
	s.logger.Error().Ctx(ctx).Err(err).Str("command", command).Msg("drive call failed")
	return err
}

func (s *DriveService) AcknowledgePane(ctx context.Context, requestID, status string) error {
	return s.drive.AcknowledgePane(ctx, requestID, status)
}
