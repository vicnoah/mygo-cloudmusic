package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type asset struct {
	Platform string `json:"platform"`
	Archive  string `json:"archive"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type index struct {
	Format  int     `json:"format"`
	Version string  `json:"version"`
	Assets  []asset `json:"assets"`
}

type signedIndex struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

var platforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"}

func main() {
	switch {
	case len(os.Args) == 3 && os.Args[1] == "keygen":
		public, private, err := ed25519.GenerateKey(nil)
		must(err)
		must(os.WriteFile(os.Args[2], []byte(base64.StdEncoding.EncodeToString(private.Seed())), 0o600))
		fmt.Println(base64.StdEncoding.EncodeToString(public))
	case len(os.Args) == 4 && (os.Args[1] == "sign" || os.Args[1] == "sign-local"):
		must(sign(os.Args[2], os.Args[3], os.Args[1] == "sign-local"))
	default:
		log.Fatal("usage: go run ./scripts/mpv keygen <path> | sign <version> <directory>")
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func sign(version, directory string, local bool) error {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		return errors.New("expected a component version like 1.0.0")
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("MPV_RELEASE_SIGNING_KEY")))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("missing or invalid MPV_RELEASE_SIGNING_KEY")
	}
	private := ed25519.NewKeyFromSeed(seed)
	release := index{Format: 1, Version: version}
	for _, platform := range platforms {
		name := fmt.Sprintf("mpv-%s-%s.zip", version, platform)
		file, err := os.Open(filepath.Join(directory, name))
		if local && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, file)
		if err := errors.Join(err, file.Close()); err != nil {
			return err
		}
		release.Assets = append(release.Assets, asset{platform, name, size, hex.EncodeToString(hash.Sum(nil))})
	}
	if len(release.Assets) == 0 {
		return errors.New("no MPV component archives")
	}
	payload, err := json.Marshal(release)
	if err != nil {
		return err
	}
	envelope := signedIndex{payload, ed25519.Sign(private, payload)}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "index.json"), encoded, 0o644)
}
