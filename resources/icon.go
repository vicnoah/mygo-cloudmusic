// Package resources contains the application icons also used by the desktop packager.
package resources

import _ "embed"

//go:embed icon.png
var Icon []byte

//go:embed icon-dev.png
var DevelopmentIcon []byte
