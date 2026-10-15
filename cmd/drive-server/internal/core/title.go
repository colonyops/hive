package core

import (
	"bytes"
	"html"
	"path"
	"regexp"
	"strings"

	"github.com/colonyops/hive/internal/domain/drive"
)

// titleScanBytes bounds how much of a body title derivation reads.
const titleScanBytes = 64 * 1024

var htmlTitlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// DeriveTitle names a document: the frontmatter title (markdown), else the
// first "# " heading (markdown) or the <title> element (HTML), else the file
// name without its extension. It never returns an empty string.
func DeriveTitle(docPath string, kind drive.Kind, body []byte) string {
	head := body[:min(len(body), titleScanBytes)]
	var title string
	switch kind {
	case drive.KindMarkdown:
		title = markdownTitle(head)
	case drive.KindHtml:
		title = htmlTitle(head)
	case drive.KindDir, drive.KindCanvas, drive.KindFile:
	}
	if title != "" {
		return title
	}
	base := path.Base(docPath)
	if stem := strings.TrimSuffix(base, path.Ext(base)); stem != "" {
		return stem
	}
	return base
}

func markdownTitle(head []byte) string {
	fm, present, err := drive.ParseMarkdownFrontmatter(head)
	if err == nil && present {
		if t, ok := fm["title"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.Join(strings.Fields(t), " ")
		}
	}

	text := strings.ReplaceAll(strings.TrimPrefix(string(head), "\xef\xbb\xbf"), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if err == nil && present {
		lines = afterFrontmatter(lines)
	}
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "# ") {
			continue
		}
		if t := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[2:]), "#")); t != "" {
			return t
		}
	}
	return ""
}

func afterFrontmatter(lines []string) []string {
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			return lines[i+1:]
		}
	}
	return nil
}

func htmlTitle(head []byte) string {
	m := htmlTitlePattern.FindSubmatch(head)
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(html.UnescapeString(string(bytes.TrimSpace(m[1])))), " ")
}
