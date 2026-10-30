package wailsui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type popOutFake struct {
	urls   []string
	names  []string
	raised []uint
	closed func(id uint)
}

func (f *popOutFake) create(name string, _ popOutKind, url string, closed func(id uint)) popOutWindow {
	f.names = append(f.names, name)
	f.urls = append(f.urls, url)
	f.closed = closed
	id := uint(len(f.urls))
	return popOutWindow{id: id, raise: func() { f.raised = append(f.raised, id) }}
}

func driveRequest(doc string) PopOutRequest {
	return PopOutRequest{Kind: "drive", Params: map[string]string{"space": "colonyops/hive", "doc": doc, "session": ""}}
}

func TestPopOutService_OpenBuildsTheSurfaceURL(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	result, err := service.Open(driveRequest("plans/a b.md"))
	require.NoError(t, err)

	assert.True(t, result.Opened)
	assert.Equal(t, "/?doc=plans%2Fa+b.md&popout=drive&space=colonyops%2Fhive", result.URL)
	assert.Equal(t, []string{result.URL}, fake.urls)
}

func TestPopOutService_OpenRaisesTheWindowAlreadyShowingTheScope(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	_, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	_, err = service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	assert.Len(t, fake.urls, 1)
	assert.Equal(t, []uint{1}, fake.raised)

	_, err = service.Open(driveRequest("b.md"))
	require.NoError(t, err)
	assert.Equal(t, []string{"popout-1", "popout-2"}, fake.names, "a window name must be unique")
}

func TestPopOutService_OpenCreatesAgainAfterTheWindowClosed(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	_, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	fake.closed(1)
	_, err = service.Open(driveRequest("a.md"))
	require.NoError(t, err)

	assert.Len(t, fake.urls, 2)
	assert.Empty(t, fake.raised, "a closed window must not be called")
}

func TestPopOutService_WithoutNativeWindowsReturnsOnlyTheURL(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(false, fake.create)

	result, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)

	assert.False(t, result.Opened)
	assert.NotEmpty(t, result.URL)
	assert.Empty(t, fake.urls, "a window in this build would never leave the window map")
}

func TestPopOutService_RejectsWhatNoKindDeclares(t *testing.T) {
	service := NewPopOutService(true, (&popOutFake{}).create)

	_, err := service.Open(PopOutRequest{Kind: "nope"})
	assert.Equal(t, app.KindInvalid, app.KindOf(err))

	_, err = service.Open(PopOutRequest{Kind: "drive", Params: map[string]string{"url": "https://example.com"}})
	assert.Equal(t, app.KindInvalid, app.KindOf(err))
}
