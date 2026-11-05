//go:build windows

package dlna

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
)

var firewallOnce sync.Once

// ensureFirewallRule registers a Windows Defender Firewall inbound rule for the current executable,
// allowing DLNA DMR devices on the local area network to connect back to the local streaming proxy.
func ensureFirewallRule() {
	firewallOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil || exe == "" {
			return
		}
		cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			"name=MyGo 云音乐",
			"dir=in",
			"action=allow",
			"program="+exe,
			"enable=yes",
			"profile=private,public",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	})
}
