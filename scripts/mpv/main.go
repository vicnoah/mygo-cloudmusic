package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type assetInfo struct {
	Platform string
	SrcFile  string
	Target   string
	AssetGz  string
}

func main() {
	assets := []assetInfo{
		{Platform: "windows-amd64", SrcFile: "/tmp/mpv-pack/mpv-2-win-amd64.dll", Target: "mpv-2.dll", AssetGz: "windows-amd64-mpv-2.dll.gz"},
		{Platform: "windows-arm64", SrcFile: "/tmp/mpv-pack/mpv-2-win-arm64.dll", Target: "mpv-2.dll", AssetGz: "windows-arm64-mpv-2.dll.gz"},
		{Platform: "linux-amd64", SrcFile: "/tmp/mpv-pack/libmpv-linux-amd64.so.2", Target: "libmpv.so.2", AssetGz: "linux-amd64-libmpv.so.2.gz"},
		{Platform: "linux-arm64", SrcFile: "/tmp/mpv-pack/libmpv-linux-arm64.so.2", Target: "libmpv.so.2", AssetGz: "linux-arm64-libmpv.so.2.gz"},
		{Platform: "darwin-arm64", SrcFile: "/tmp/mpv-pack/libmpv-darwin-arm64.dylib", Target: "libmpv.dylib", AssetGz: "darwin-arm64-libmpv.dylib.gz"},
		{Platform: "darwin-amd64", SrcFile: "/tmp/mpv-pack/libmpv-darwin-amd64.dylib", Target: "libmpv.dylib", AssetGz: "darwin-amd64-libmpv.dylib.gz"},
	}

	outDir := "build/mpv"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(err)
	}

	type packedResult struct {
		Platform string
		Name     string
		Asset    string
		Size     int64
		GzSize   int64
		SHA      string
		GzSHA    string
	}

	var results []packedResult

	for _, a := range assets {
		dstPath := filepath.Join(outDir, a.AssetGz)
		fmt.Printf("Packing %s -> %s...\n", a.SrcFile, dstPath)

		rawBytes, err := os.ReadFile(a.SrcFile)
		if err != nil {
			panic(err)
		}
		rawSHA := sha256Sum(rawBytes)
		rawSize := int64(len(rawBytes))

		// Gzip
		outF, err := os.Create(dstPath)
		if err != nil {
			panic(err)
		}
		gzWriter, err := gzip.NewWriterLevel(outF, gzip.BestCompression)
		if err != nil {
			panic(err)
		}
		if _, err := gzWriter.Write(rawBytes); err != nil {
			panic(err)
		}
		gzWriter.Close()
		outF.Close()

		gzBytes, err := os.ReadFile(dstPath)
		if err != nil {
			panic(err)
		}
		gzSHA := sha256Sum(gzBytes)
		gzSize := int64(len(gzBytes))

		results = append(results, packedResult{
			Platform: a.Platform,
			Name:     a.Target,
			Asset:    a.AssetGz,
			Size:     rawSize,
			GzSize:   gzSize,
			SHA:      rawSHA,
			GzSHA:    gzSHA,
		})
	}

	// Generate natives.go
	nativesCode := `package mpv

import (
	"runtime"
)

// LibVersion is the target MPV library version for downloaded assets.
const LibVersion = "0.38.0"

type nativeFile struct {
	Name     string
	Asset    string
	Size     int64
	GzSize   int64
	SHA256   string
	GzSHA256 string
}

// SystemLibNames returns candidate dynamic library names for the current OS.
func SystemLibNames() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"mpv-2.dll", "libmpv-2.dll", "libmpv.dll", "mpv-1.dll"}
	case "darwin":
		return []string{
			"libmpv.2.dylib",
			"libmpv.dylib",
			"/opt/homebrew/lib/libmpv.dylib",
			"/usr/local/lib/libmpv.dylib",
		}
	default:
		return []string{
			"libmpv.so.2",
			"libmpv.so",
			"/usr/lib/x86_64-linux-gnu/libmpv.so.2",
			"/usr/lib/aarch64-linux-gnu/libmpv.so.2",
			"/usr/lib64/libmpv.so.2",
			"/usr/lib/libmpv.so.2",
		}
	}
}

// Platform returns the current GOOS-GOARCH string.
func Platform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// NativeAssets contains known precompiled releases for each platform.
var NativeAssets = map[string]nativeFile{
`
	for _, r := range results {
		nativesCode += fmt.Sprintf("\t%q: {\n", r.Platform)
		nativesCode += fmt.Sprintf("\t\tName:     %q,\n", r.Name)
		nativesCode += fmt.Sprintf("\t\tAsset:    %q,\n", r.Asset)
		nativesCode += fmt.Sprintf("\t\tSize:     %d,\n", r.Size)
		nativesCode += fmt.Sprintf("\t\tGzSize:   %d,\n", r.GzSize)
		nativesCode += fmt.Sprintf("\t\tSHA256:   %q,\n", r.SHA)
		nativesCode += fmt.Sprintf("\t\tGzSHA256: %q,\n", r.GzSHA)
		nativesCode += "\t},\n"
	}
	nativesCode += "}\n"

	if err := os.WriteFile("internal/player/mpv/natives.go", []byte(nativesCode), 0644); err != nil {
		panic(err)
	}

	fmt.Println("Successfully generated build/mpv/* and internal/player/mpv/natives.go!")
}

func sha256Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
