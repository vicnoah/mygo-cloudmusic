package main

import (
	"log"
	"os"

	"waygu-music/internal/app"
	"waygu-music/internal/builtin"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--prepare-installer-shortcuts" {
		if err := app.PrepareInstallerShortcuts(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := app.RunManaged(builtin.Platforms()); err != nil {
		log.Fatal(err)
	}
}
