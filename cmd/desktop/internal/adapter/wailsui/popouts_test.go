package wailsui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"

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

type callerWindow struct {
	application.Window
	id uint
}

func (w callerWindow) ID() uint { return w.id }

func calledFrom(t *testing.T, id uint) context.Context {
	t.Helper()
	return context.WithValue(t.Context(), application.WindowKey, application.Window(callerWindow{id: id}))
}

func TestPopOutService_RetargetMovesTheWindowsIdentity(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	_, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	require.NoError(t, service.Retarget(calledFrom(t, 1), driveRequest("b.md")))

	_, err = service.Open(driveRequest("b.md"))
	require.NoError(t, err)
	assert.Equal(t, []uint{1}, fake.raised, "the window that moved shows b.md")

	_, err = service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	assert.Len(t, fake.urls, 2, "nothing shows a.md any more")
}

func TestPopOutService_RetargetIgnoresACallerThatIsNotAPopOut(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	_, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	require.NoError(t, service.Retarget(calledFrom(t, 99), driveRequest("b.md")))
	require.NoError(t, service.Retarget(t.Context(), driveRequest("b.md")))

	_, err = service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	assert.Equal(t, []uint{1}, fake.raised, "window 1 still shows a.md")
}

func TestPopOutService_RetargetRejectsWhatNoKindDeclares(t *testing.T) {
	service := NewPopOutService(true, (&popOutFake{}).create)

	err := service.Retarget(calledFrom(t, 1), PopOutRequest{Kind: "nope"})
	assert.Equal(t, app.KindInvalid, app.KindOf(err))
}

func TestPopOutService_TwoWindowsOnOneScopeRaiseTheOlder(t *testing.T) {
	fake := &popOutFake{}
	service := NewPopOutService(true, fake.create)

	_, err := service.Open(driveRequest("a.md"))
	require.NoError(t, err)
	_, err = service.Open(driveRequest("b.md"))
	require.NoError(t, err)
	require.NoError(t, service.Retarget(calledFrom(t, 2), driveRequest("a.md")))

	for range 8 {
		_, err = service.Open(driveRequest("a.md"))
		require.NoError(t, err)
	}
	assert.Equal(t, []uint{1, 1, 1, 1, 1, 1, 1, 1}, fake.raised)
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
