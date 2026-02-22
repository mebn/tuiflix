package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tuiflix/internal/api"
	"tuiflix/internal/app"
)

func main() {
	loadDotEnv(".env")

	rdToken := strings.TrimSpace(os.Getenv("REALDEBRID"))
	client := api.NewClient(rdToken)

	program := tea.NewProgram(
		app.NewModel(client),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if err := program.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "tuiflix error: %v\n", err)
		os.Exit(1)
	}
}

func loadDotEnv(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return
	}

	file, err := os.Open(absolute)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"`)

		if key == "" || os.Getenv(key) != "" {
			continue
		}

		if setErr := os.Setenv(key, value); setErr != nil && !errors.Is(setErr, os.ErrPermission) {
			continue
		}
	}
}
