package mpv

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
	"windows-amd64": {
		Name:     "mpv-2.dll",
		Asset:    "windows-amd64-mpv-2.dll.gz",
		Size:     121566720,
		GzSize:   48269403,
		SHA256:   "4b3fc2dbeb350820a00a1651c67f9c15958f5378e235853bc09bf4f17df7c651",
		GzSHA256: "433603caf42bd989bf90c6327e70e9b40a0f54b34dc0881668a042d625c634c0",
	},
	"windows-arm64": {
		Name:     "mpv-2.dll",
		Asset:    "windows-arm64-mpv-2.dll.gz",
		Size:     83929600,
		GzSize:   41495386,
		SHA256:   "fbe6f5a3db2767b0aab2d7d820e1cc1bd7f2b9d8c3a65d35592efdf60b77b269",
		GzSHA256: "e02eb6b341d4c848c3794a59da6c89f4a494620a4a9f0b4f232e4ac13be1dbdf",
	},
	"linux-amd64": {
		Name:     "libmpv.so.2",
		Asset:    "linux-amd64-libmpv.so.2.gz",
		Size:     3009104,
		GzSize:   1170696,
		SHA256:   "f2aa472878adee5f44419b6cce90ccecc43bf81595ac16dbf223de190cdead0a",
		GzSHA256: "16d31b7e7fab7766ae2c5081dab1ada4db1a0d0df73d2d638e0894fab520465a",
	},
	"linux-arm64": {
		Name:     "libmpv.so.2",
		Asset:    "linux-arm64-libmpv.so.2.gz",
		Size:     2956016,
		GzSize:   1151995,
		SHA256:   "f48f5239267447aa37789708c737f560152aa4f854b2d15859c99ea37d1c7fea",
		GzSHA256: "83a5dd89d3ed25f25866d7e906bc4e4c76c57e054db237b241007285ddadfe9d",
	},
	"darwin-arm64": {
		Name:     "libmpv.dylib",
		Asset:    "darwin-arm64-libmpv.dylib.gz",
		Size:     4530224,
		GzSize:   2514259,
		SHA256:   "26d9d92c5cd3610734e2b227a259cb406e23fedf2487dc9ade15ba7195ed840c",
		GzSHA256: "67acf2cc00b5d675b94c64f7471fee8f3a8ac5c63656a7fdbeb207b02d533412",
	},
	"darwin-amd64": {
		Name:     "libmpv.dylib",
		Asset:    "darwin-amd64-libmpv.dylib.gz",
		Size:     4548336,
		GzSize:   2570474,
		SHA256:   "dee683496f915310374e4d19226ff02bcdfcf214e1aeaec749f81bde40a0370e",
		GzSHA256: "912b1dedba439a8c7da7ec0fc20954a998e691ed0b7f7d3634bae6c9e3797a73",
	},
}
