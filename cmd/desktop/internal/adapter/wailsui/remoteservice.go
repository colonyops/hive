package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/remoteconn"
)

type RemoteService struct{ manager *remoteconn.Manager }

func NewRemoteService(manager *remoteconn.Manager) *RemoteService {
	return &RemoteService{manager: manager}
}

func (s *RemoteService) Connect(ctx context.Context, profile remoteconn.Profile) (string, error) {
	return s.manager.Connect(ctx, profile)
}
func (s *RemoteService) Disconnect(id string) { s.manager.Disconnect(id) }
func (s *RemoteService) Request(ctx context.Context, id, path, body string) (remoteconn.Response, error) {
	return s.manager.Request(ctx, id, path, body)
}

func (s *RemoteService) OpenStream(ctx context.Context, id, slug string) (string, error) {
	return s.manager.OpenStream(ctx, id, slug)
}

func (s *RemoteService) ReadStream(ctx context.Context, id, stream string) (remoteconn.Batch, error) {
	return s.manager.ReadStream(ctx, id, stream)
}

func (s *RemoteService) WriteStream(ctx context.Context, id, stream, frame string) error {
	return s.manager.WriteStream(ctx, id, stream, frame)
}

func (s *RemoteService) CloseStream(id, stream string) error {
	return s.manager.CloseStream(id, stream)
}
