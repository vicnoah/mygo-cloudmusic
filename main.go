package main

import (
	"log"

	"waygu-music/internal/app"
	"waygu-music/internal/builtin"
)

func main() {
	if err := app.Run(builtin.Platforms()); err != nil {
		log.Fatal(err)
	}
}
