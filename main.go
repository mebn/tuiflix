package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"

	"tuiflix/internal/api"
	"tuiflix/internal/app"
)

func main() {
	_ = godotenv.Load(".env")

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
