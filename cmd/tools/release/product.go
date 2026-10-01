package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"

	desktopnotes "github.com/colonyops/hive/cmd/desktop/releasenotes"
	clinotes "github.com/colonyops/hive/cmd/hive/releasenotes"
	"github.com/colonyops/hive/internal/releasenotes"
)

// product is a program that ships from this repository with release notes of
// its own. Every release ships every product under one version, so promotion
// writes an entry for each of them at once.
type product struct {
	// name is what `changelog new --product` takes.
	name  string
	title string
	// dir is the changelog directory, relative to the repository root.
	dir string
	// changelog is the same directory as the product's binary embeds it.
	// What a release publishes is read through it, so a published version and
	// the notes its binary shows cannot drift apart.
	changelog fs.FS
}

var (
	cliProduct = product{
		name:      "cli",
		title:     "hive CLI",
		dir:       "cmd/hive/releasenotes/changelog",
		changelog: clinotes.Changelog,
	}
	desktopProduct = product{
		name:      "desktop",
		title:     "Hive Desktop",
		dir:       "cmd/desktop/releasenotes/changelog",
		changelog: desktopnotes.Changelog,
	}
	products = []product{cliProduct, desktopProduct}
)

func productNames() []string {
	names := make([]string, 0, len(products))
	for _, p := range products {
		names = append(names, p.name)
	}
	return names
}

func parseProduct(name string) (product, error) {
	for _, p := range products {
		if p.name == name {
			return p, nil
		}
	}
	return product{}, fmt.Errorf("unknown product %q: expected one of %v", name, productNames())
}

func (p product) unreleasedDir() string { return path.Join(p.dir, releasenotes.UnreleasedDir) }

func (p product) entryPath(version string) string { return path.Join(p.dir, version+".md") }

// onDisk is the changelog in the worktree. The commands that write it read it
// here: the embed is fixed when the tool is compiled, and a promotion from a
// stale build would delete only the fragments that build knew about.
func (p product) onDisk() fs.FS { return os.DirFS(p.dir) }

// embedded reads the changelog as the product's binary embeds it.
func (p product) embedded() (releasenotes.Entries, error) { return p.load(p.changelog) }

func (p product) load(fsys fs.FS) (releasenotes.Entries, error) {
	entries, err := releasenotes.Load(fsys)
	if err != nil {
		return nil, fmt.Errorf("read %s changelog: %w", p.name, err)
	}
	return entries, nil
}
