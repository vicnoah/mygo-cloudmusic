package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }
