package main

import (
	"log"

	"waygu-music/internal/app"
	"waygu-music/internal/builtin"
	"waygu-music/internal/platformcatalog"
)

func main() {
	platforms := append(builtin.Platforms(), platformcatalog.ExternalPlatforms()...)
	if err := app.RunManaged(platforms); err != nil {
		log.Fatal(err)
	}
}
