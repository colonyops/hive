package main

import (
	"fmt"
	"os"

	"github.com/hay-kot/hive/internal/commands"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/tui"
)

func main() {
	// Load config with default paths
	configPath := commands.DefaultConfigPath()
	dataDir := commands.DefaultDataDir()

	cfg, err := config.Load(configPath, dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Hardcode this repo for testing
	owner := "colonyops"
	repo := "hive"

	// Get context directory
	contextDir := cfg.RepoContextDir(owner, repo)

	fmt.Printf("Repository: %s/%s\n", owner, repo)
	fmt.Printf("Context directory: %s\n\n", contextDir)

	// Discover documents
	docs, err := tui.DiscoverDocuments(contextDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering documents: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Printf("Found %d documents:\n\n", len(docs))

	var currentType tui.DocumentType = -1
	for _, doc := range docs {
		// Print category header when type changes
		if doc.Type != currentType {
			if currentType != -1 {
				fmt.Println()
			}
			fmt.Printf("=== %s ===\n", doc.Type.String())
			currentType = doc.Type
		}

		fmt.Printf("  %s\n", doc.RelPath)
		fmt.Printf("    Modified: %s\n", doc.ModTime.Format("2006-01-02 15:04:05"))
	}
}
