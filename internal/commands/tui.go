package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// selectFromList shows an interactive list selector in the terminal.
// Returns the selected index, or an error if cancelled.
func selectFromList(items []string, prompt string) (int, error) {
	if len(items) == 0 {
		return -1, errors.New("no items to select")
	}

	// Save terminal state and switch to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return -1, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Hide cursor
	fmt.Print("\033[?25l")
	defer fmt.Print("\033[?25h")

	selected := 0
	reader := bufio.NewReader(os.Stdin)

	render := func() {
		// Clear screen and move to top-left
		fmt.Print("\033[2J\033[H")
		if prompt != "" {
			fmt.Println(prompt)
			fmt.Println()
		}
		for i, item := range items {
			if i == selected {
				fmt.Printf("> %s\n", item)
			} else {
				fmt.Printf("  %s\n", item)
			}
		}
		fmt.Println()
		fmt.Println("↑/↓ or Ctrl-P/Ctrl-N to navigate, Enter to confirm, q/Esc to cancel")
	}

	render()

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return -1, err
		}

		switch b {
		case 13, 10: // Enter / Return
			return selected, nil
		case 3, 27: // Ctrl-C, Esc
			// For Esc, consume following bytes if it's an escape sequence
			if b == 27 {
				// Peek to see if more bytes follow (arrow key sequence)
				// If next byte is '[', it's an arrow key; we already consumed ESC,
				// so we need to handle the sequence. But if it's just Esc alone, cancel.
				// We'll read ahead non-blocking to check.
				// Simple approach: try to read '[' and then the arrow code.
				// However, since we're in raw mode, ReadByte blocks. Let's handle
				// arrow keys by reading two more bytes when we see 27.
				next, err := reader.ReadByte()
				if err != nil {
					return -1, errors.New("cancelled")
				}
				if next == '[' {
					dir, err := reader.ReadByte()
					if err != nil {
						return -1, errors.New("cancelled")
					}
					switch dir {
					case 'A': // up
						if selected > 0 {
							selected--
						}
					case 'B': // down
						if selected < len(items)-1 {
							selected++
						}
					}
					render()
					continue
				}
				// If it wasn't '[', treat as cancel (we consumed one extra byte though,
				// but for simplicity that's acceptable in this minimal TUI)
			}
			return -1, errors.New("cancelled")
		case 16: // Ctrl-P
			if selected > 0 {
				selected--
			}
			render()
		case 14: // Ctrl-N
			if selected < len(items)-1 {
				selected++
			}
			render()
		case 'q', 'Q':
			return -1, errors.New("cancelled")
		}
	}
}
