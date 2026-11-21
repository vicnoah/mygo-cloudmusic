//go:build linux

package mpv

import (
	"os"
	"os/exec"
	"testing"

	"github.com/ebitengine/purego"
)

// MPV uses C numeric formatting after GTK sets a UTF-8 locale, while text remains UTF-8.
func TestPrepareRuntimeCorrectsOnlyNumericLocale(t *testing.T) {
	if os.Getenv("WAGYU_TEST_MPV_LOCALE") == "" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPrepareRuntimeCorrectsOnlyNumericLocale$")
		cmd.Env = append(os.Environ(), "WAGYU_TEST_MPV_LOCALE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("locale subprocess: %v\n%s", err, output)
		}
		return
	}

	const lcCType = 0
	var setLocale func(category int32, locale *byte) string
	symbol, err := purego.Dlsym(purego.RTLD_DEFAULT, "setlocale")
	if err != nil {
		t.Fatal(err)
	}
	purego.RegisterFunc(&setLocale, symbol)
	utf8 := cStr("C.UTF-8")
	if setLocale(lcNumeric, &utf8[0]) == "" {
		t.Skip("C.UTF-8 locale is unavailable")
	}
	textLocale := setLocale(lcCType, &utf8[0])
	if textLocale == "" {
		t.Skip("C.UTF-8 text locale is unavailable")
	}
	if err := PrepareRuntime(); err != nil {
		t.Fatal(err)
	}
	if got := setLocale(lcNumeric, nil); got != "C" {
		t.Fatalf("numeric locale is %q", got)
	}
	if got := setLocale(lcCType, nil); got != textLocale {
		t.Fatalf("text locale is %q, want %q", got, textLocale)
	}
}
