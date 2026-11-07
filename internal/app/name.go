package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// appName is what the app calls itself in its interface, in Chinese. MyGo
// names it "Wagyu Music" instead: the executable, the bundle and the
// folders under the user's data, in ASCII, while macOS and Windows show
// the Chinese name from the bundle's Info.plist and windows.displayName.
const appName = "肥牛音乐"

// dataName is the name of the app's folders under the user's data, in
// ASCII, as macOS and Windows would otherwise name them in Chinese: there
// App.Name reads the bundle's CFBundleName and the executable's
// ProductName, which are the display name.
const dataName = "Wagyu Music"

// useDataPaths keeps the settings, the song cache and the logs under
// dataName, and the development app's under "Wagyu Music Dev", apart from
// the installed app's. It must run before anything reads them, and before
// moveOldData.
func useDataPaths() {
	name := dataName
	if exe, err := os.Executable(); err == nil {
		// The development app's executable is "Wagyu Music Dev.exe" on
		// Windows, "Wagyu Music Dev" in its bundle, and "wagyu-music-dev"
		// on Linux. The display name is the same in both, so tell them
		// apart by the executable.
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe)))
		if strings.HasSuffix(base, " dev") || strings.HasSuffix(base, "-dev") {
			name += " Dev"
		}
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return
	}
	data := filepath.Join(config, name)
	if err := os.MkdirAll(data, 0o755); err != nil {
		return
	}
	mygo.App.SetPath(mygo.PathUserData, data)
	// Logs sit beside the data on Windows and Linux, and in macOS's Logs
	// folder, as PathLogs would put them. Left for the writer to make, so
	// that moveOldData still finds the data folder empty.
	logs := filepath.Join(data, "logs")
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			logs = filepath.Join(home, "Library", "Logs", name)
		}
	}
	mygo.App.SetPath(mygo.PathLogs, logs)
	if cache, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(cache, name)
		if err := os.MkdirAll(cache, 0o755); err == nil {
			mygo.App.SetPath(mygo.PathCache, cache)
		}
	}
}
