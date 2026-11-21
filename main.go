package main

import (
	"log"
	"os"

	"waygu-music/internal/app"
	"waygu-music/internal/builtin"
	"waygu-music/internal/player/mpv"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--mpv-probe" {
		if err := mpv.ProbePath(os.Args[2], os.Stderr); err != nil {
			log.Fatal(err)
		}
		return
	}
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
