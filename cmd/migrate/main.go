package main

import (
	"fmt"
	"os"

	"github.com/YosuaChrialMartono/personal-be/internal/config"
	"github.com/YosuaChrialMartono/personal-be/internal/database/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "down") {
		return fmt.Errorf("usage: migrate [up|down]")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return migrations.Apply(cfg.DatabasePath, args[0] == "down")
}
