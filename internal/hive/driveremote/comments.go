package driveremote

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

// ErrNoComments means the server predates comments. The server answers a
// missing route and a missing document the same way, so the client asks
// first.
var ErrNoComments = errors.New("this drive server does not support comments")

// Threads lists the threads on a document the caller owns.
func (s *Service) Threads(ctx context.Context, server string, space drive.Space, path string) (driveapi.ThreadList, error) {
	client, err := s.commentsClient(ctx, server)
	if err != nil {
		return driveapi.ThreadList{}, err
	}
	return client.Threads(ctx, space, cleanPath(path))
}

// Reply answers the thread that thread names on the document at path.
func (s *Service) Reply(ctx context.Context, server string, space drive.Space, path, thread, body string) (driveapi.Comment, error) {
	client, t, err := s.findThread(ctx, server, space, path, thread)
	if err != nil {
		return driveapi.Comment{}, err
	}
	return client.Reply(ctx, t.ID, body)
}

// SetResolved resolves or reopens the thread that thread names on the
// document at path.
func (s *Service) SetResolved(ctx context.Context, server string, space drive.Space, path, thread string, resolved bool) (driveapi.Thread, error) {
	client, t, err := s.findThread(ctx, server, space, path, thread)
	if err != nil {
		return driveapi.Thread{}, err
	}
	return client.SetResolved(ctx, t.ID, resolved)
}

// findThread resolves thread among the document's threads, which also
// proves the thread is on that document.
func (s *Service) findThread(ctx context.Context, server string, space drive.Space, path, thread string) (*driveapi.Client, driveapi.Thread, error) {
	client, err := s.commentsClient(ctx, server)
	if err != nil {
		return nil, driveapi.Thread{}, err
	}
	list, err := client.Threads(ctx, space, cleanPath(path))
	if err != nil {
		return nil, driveapi.Thread{}, err
	}
	t, err := ResolveThread(list.Threads, thread)
	if err != nil {
		return nil, driveapi.Thread{}, err
	}
	return client, t, nil
}

func (s *Service) commentsClient(ctx context.Context, server string) (*driveapi.Client, error) {
	client, _, err := s.client(server)
	if err != nil {
		return nil, err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(me.Features, driveapi.FeatureComments) {
		return nil, ErrNoComments
	}
	return client, nil
}

// ResolveThread matches ref against the threads' ids: a full id, or a unique
// suffix. A suffix, because a UUIDv7 starts with its timestamp and threads
// made close together share their leading characters.
func ResolveThread(threads []driveapi.Thread, ref string) (driveapi.Thread, error) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "#")
	if ref == "" {
		return driveapi.Thread{}, errors.New("thread id is empty")
	}
	var matches []driveapi.Thread
	for _, t := range threads {
		if t.ID == ref {
			return t, nil
		}
		if strings.HasSuffix(t.ID, ref) {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		return driveapi.Thread{}, fmt.Errorf("no thread matches %q", ref)
	case 1:
		return matches[0], nil
	default:
		return driveapi.Thread{}, fmt.Errorf("%q matches %d threads: give more of the id", ref, len(matches))
	}
}

// Read returns the body of the document at path on the server.
func (s *Service) Read(ctx context.Context, server string, space drive.Space, path string) ([]byte, error) {
	client, _, err := s.client(server)
	if err != nil {
		return nil, err
	}
	_, body, err := client.Get(ctx, space, cleanPath(path))
	return body, err
}
