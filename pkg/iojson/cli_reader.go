package iojson

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

type FileReader[T any] struct {
	fileFlagValue string
}

func (fr *FileReader[T]) Flag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "file",
		Aliases:     []string{"f"},
		Usage:       "path to JSON file (reads from stdin if not provided)",
		Destination: &fr.fileFlagValue,
	}
}

func (fr *FileReader[T]) Read() (T, error) {
	var reader io.Reader
	var input T

	if fr.fileFlagValue != "" {
		f, err := os.Open(fr.fileFlagValue)
		if err != nil {
			return input, fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		reader = f
	} else {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return input, fmt.Errorf("no input provided (stdin is a terminal); use -f flag or pipe JSON input")
		}
		reader = os.Stdin
	}

	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		return input, fmt.Errorf("decode JSON: %w", err)
	}

	return input, nil
}
