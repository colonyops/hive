package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
)

// AppletFile is one file of an applet being shared.
type AppletFile struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// AppletExportPreview is what an export would contain, for the user to read first.
type AppletExportPreview struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	FileName string       `json:"fileName"`
	Files    []AppletFile `json:"files"`
	LeftOut  []string     `json:"leftOut"`
}

// AppletImportPreview is a checked file waiting for the user. Nothing is on disk yet.
type AppletImportPreview struct {
	// Token names the checked file, so what the user confirms is what they read.
	Token string `json:"token"`
	// ID is what the file asks for, and SuggestedID the nearest id that is free.
	ID          string          `json:"id"`
	SuggestedID string          `json:"suggestedId"`
	Title       string          `json:"title"`
	Entry       string          `json:"entry"`
	Cwd         string          `json:"cwd"`
	Titlebar    *AppletTitlebar `json:"titlebar"`
	Commands    []AppletCommand `json:"commands"`
	Files       []AppletFile    `json:"files"`
	TotalBytes  int64           `json:"totalBytes"`
}

func (s *AppletsService) exportable(id string) (applets.Applet, error) {
	if !s.pages {
		return applets.Applet{}, Errorf(KindUnavailable, "%s", appletPagesUnavailable)
	}
	applet, err := s.applet(id)
	if err != nil {
		return applets.Applet{}, err
	}
	if applet.Builtin {
		return applets.Applet{}, Errorf(KindInvalid, "%s ships with Hive, so there is nothing to share", applet.Title)
	}
	return applet, nil
}

// ExportPreview lists what an export of the applet would contain and leave out.
func (s *AppletsService) ExportPreview(id string) (AppletExportPreview, error) {
	applet, err := s.exportable(id)
	if err != nil {
		return AppletExportPreview{}, err
	}
	list, err := applets.ListPack(applet.Dir)
	if err != nil {
		return AppletExportPreview{}, Errorf(KindInvalid, "%s", err.Error())
	}
	preview := AppletExportPreview{
		ID:       applet.ID,
		Title:    applet.Title,
		FileName: applet.ID + applets.ArchiveExtension,
		Files:    make([]AppletFile, 0, len(list.Files)),
		LeftOut:  list.LeftOut,
	}
	for _, file := range list.Files {
		preview.Files = append(preview.Files, AppletFile{Path: file.Path, Bytes: file.Bytes})
	}
	if preview.LeftOut == nil {
		preview.LeftOut = []string{}
	}
	return preview, nil
}

// Export writes the applet to dest, an absolute path from the native save dialog. The
// file appears whole or not at all.
func (s *AppletsService) Export(id, dest string) (err error) {
	applet, err := s.exportable(id)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(dest), ".applet-export-*")
	if err != nil {
		return Wrap(err, KindInternal, "writing %s", dest)
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	if err := applets.Pack(applet, file); err != nil {
		return Errorf(KindInvalid, "%s", err.Error())
	}
	if err := file.Close(); err != nil {
		return Wrap(err, KindInternal, "writing %s", dest)
	}
	if err := os.Chmod(file.Name(), 0o644); err != nil {
		return Wrap(err, KindInternal, "writing %s", dest)
	}
	if err := os.Rename(file.Name(), dest); err != nil {
		return Wrap(err, KindInternal, "writing %s", dest)
	}
	return nil
}

// PrepareImport checks a shared file and holds it in memory for the user to confirm. It
// writes nothing. A second file replaces one still waiting.
func (s *AppletsService) PrepareImport(encoded string) (AppletImportPreview, error) {
	if !s.pages {
		return AppletImportPreview{}, Errorf(KindUnavailable, "%s", appletPagesUnavailable)
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > applets.MaxArchiveBytes+2 {
		return AppletImportPreview{}, Errorf(KindInvalid, "this file is larger than the %d MB limit", applets.MaxArchiveBytes>>20)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return AppletImportPreview{}, Errorf(KindInvalid, "this file could not be read")
	}
	archive, err := applets.Unpack(raw)
	if err != nil {
		return AppletImportPreview{}, Errorf(KindInvalid, "%s", err.Error())
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return AppletImportPreview{}, Wrap(err, KindInternal, "preparing the import")
	}

	applet := archive.Applet
	preview := AppletImportPreview{
		Token:       hex.EncodeToString(token),
		ID:          applet.ID,
		SuggestedID: applets.FreeID(s.catalog.Root(), applet.ID),
		Title:       applet.Title,
		Entry:       applet.Entry,
		Cwd:         applet.Cwd,
		Commands:    make([]AppletCommand, 0, len(applet.Commands)),
		Files:       make([]AppletFile, 0, len(archive.Files)),
	}
	if applet.Titlebar != nil {
		preview.Titlebar = &AppletTitlebar{Entry: applet.Titlebar.Entry, Width: applet.Titlebar.Width}
	}
	for _, command := range applet.Commands {
		preview.Commands = append(preview.Commands, AppletCommand{ID: command.ID, Title: command.Title, Icon: command.Icon, Keywords: command.Keywords})
	}
	for _, file := range archive.Files {
		size := int64(len(file.Data))
		preview.Files = append(preview.Files, AppletFile{Path: file.Path, Bytes: size})
		preview.TotalBytes += size
	}

	s.importMu.Lock()
	defer s.importMu.Unlock()
	s.pendingImport = &pendingAppletImport{token: preview.Token, archive: archive}
	return preview, nil
}

// ConfirmImport installs the file PrepareImport checked, as a new applet called id. It
// never replaces one. An id that is taken leaves the file waiting, so the user can pick
// another.
func (s *AppletsService) ConfirmImport(token, id string) error {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if s.pendingImport == nil || s.pendingImport.token != token {
		return Errorf(KindNotFound, "that import is no longer waiting. Choose the file again")
	}
	if err := s.pendingImport.archive.Install(s.catalog.Root(), id); err != nil {
		if errors.Is(err, applets.ErrAppletExists) {
			return Errorf(KindConflict, "an applet called %q already exists. Choose another id", id)
		}
		return Wrap(err, KindInvalid, "importing %s", id)
	}
	s.pendingImport = nil
	if err := s.catalog.Reload(); err != nil {
		return Wrap(err, KindInternal, "reloading the applets")
	}
	return nil
}

// CancelImport drops a waiting file.
func (s *AppletsService) CancelImport(token string) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	if s.pendingImport != nil && s.pendingImport.token == token {
		s.pendingImport = nil
	}
}
