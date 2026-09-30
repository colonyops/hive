package main

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type plistValue struct {
	kind string
	text string
}

func readPlistValues(t *testing.T, path string) map[string]plistValue {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	values := make(map[string]plistValue)
	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return values
		}
		require.NoError(t, err)

		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "key" {
			continue
		}

		var key string
		require.NoError(t, decoder.DecodeElement(&key, &start))
		for {
			token, err = decoder.Token()
			require.NoError(t, err)
			valueStart, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			var text string
			require.NoError(t, decoder.DecodeElement(&text, &valueStart))
			values[key] = plistValue{kind: valueStart.Name.Local, text: text}
			break
		}
	}
}

func TestDarwinBundlesDescribeChildProcessPrivacyUsage(t *testing.T) {
	expected := map[string]string{
		"NSAppleEventsUsageDescription":          "A command running within Hive may control another application using AppleScript.",
		"NSDesktopFolderUsageDescription":        "A command running within Hive may access files in your Desktop folder.",
		"NSDocumentsFolderUsageDescription":      "A command running within Hive may access files in your Documents folder.",
		"NSDownloadsFolderUsageDescription":      "A command running within Hive may access files in your Downloads folder.",
		"NSLocalNetworkUsageDescription":         "A command running within Hive may connect to development servers and other devices on your local network.",
		"NSNetworkVolumesUsageDescription":       "A command running within Hive may access files on network volumes.",
		"NSRemovableVolumesUsageDescription":     "A command running within Hive may access files on removable volumes.",
		"NSSystemAdministrationUsageDescription": "A command running within Hive may request elevated privileges.",
	}

	for _, name := range []string{"Info.plist", "Info.dev.plist"} {
		t.Run(name, func(t *testing.T) {
			values := readPlistValues(t, filepath.Join("..", "..", "desktop", "build", "darwin", name))
			for key, text := range expected {
				assert.Equal(t, plistValue{kind: "string", text: text}, values[key], key)
			}
		})
	}
}

func TestDarwinEntitlementsAllowChildProcessAppleEvents(t *testing.T) {
	values := readPlistValues(t, filepath.Join("..", "..", "desktop", "build", "darwin", "entitlements.plist"))
	assert.Equal(t, plistValue{kind: "true"}, values["com.apple.security.automation.apple-events"])
}
