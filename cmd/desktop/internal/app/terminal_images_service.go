package app

import (
	"context"
	"errors"
	"io"

	"github.com/colonyops/hive/cmd/desktop/internal/app/terminalimg"
)

type TerminalImagesService struct {
	store *terminalimg.Store
}

func (s *TerminalImagesService) Prepare(ctx context.Context, paths []string) ([]string, error) {
	pastes, err := s.store.Prepare(ctx, paths)
	return pastes, imageInputError(err)
}

func (s *TerminalImagesService) Save(ctx context.Context, images []io.Reader) ([]string, error) {
	pastes, err := s.store.Save(ctx, images)
	return pastes, imageInputError(err)
}

func imageInputError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, terminalimg.ErrInvalid) {
		return Wrap(err, KindInvalid, "%s", err)
	}
	if errors.Is(err, terminalimg.ErrQuota) {
		return Wrap(err, KindConflict, "%s", err)
	}
	return Wrap(err, KindInternal, "Could not prepare the image. Check that image storage is writable and has free space.")
}
