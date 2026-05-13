package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// openEditor opens the user's preferred editor with initial content and returns
// the saved content. It resolves $VISUAL → $EDITOR → "vim".
func openEditor(initialContent string) (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}

	tmpFile, err := os.CreateTemp("", "groupfdn-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(initialContent); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	cmd := exec.Command(editor, tmpFile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read temp file: %w", err)
	}

	return string(content), nil
}

// parseEditorContent parses editor output into title and description.
// The first non-empty, non-comment line is the title.
// Everything after the first blank line (following the title) is the description.
// Comment lines starting with # are stripped.
func parseEditorContent(content string) (title string, description string, err error) {
	lines := strings.Split(content, "\n")

	// Strip comment lines and find title
	var titleFound bool
	var descLines []string
	var collectingDesc bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comment lines entirely
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !titleFound {
			if trimmed != "" {
				title = trimmed
				titleFound = true
			}
			continue
		}

		// After title found, look for blank line to start description
		if !collectingDesc {
			if trimmed == "" {
				collectingDesc = true
			}
			continue
		}

		// Collecting description
		descLines = append(descLines, line)
	}

	if !titleFound {
		return "", "", fmt.Errorf("title is required")
	}

	// Trim trailing empty lines from description
	for len(descLines) > 0 && strings.TrimSpace(descLines[len(descLines)-1]) == "" {
		descLines = descLines[:len(descLines)-1]
	}

	description = strings.Join(descLines, "\n")
	return title, description, nil
}

// editorTemplate returns the initial content template for the editor.
func editorTemplate() string {
	return "\n# Enter title on the first line above.\n# Leave a blank line, then add description below.\n# Lines starting with # will be ignored.\n"
}
