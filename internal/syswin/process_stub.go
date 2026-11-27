//go:build !windows

package syswin

import "github.com/singtail/singtail/internal/contract"

var defaultStubApps = []contract.RunningApp{
	{
		Process: "chrome.exe",
		Path:    `C:\Program Files\Google\Chrome\Application\chrome.exe`,
	},
	{
		Process: "Code.exe",
		Path:    `C:\Users\admin\AppData\Local\Programs\Microsoft VS Code\Code.exe`,
	},
	{
		Process: "Cursor.exe",
		Path:    `C:\Users\admin\AppData\Local\Programs\cursor\Cursor.exe`,
	},
	{
		Process: "steam.exe",
		Path:    `C:\Program Files (x86)\Steam\steam.exe`,
	},
	{
		Process: "Telegram.exe",
		Path:    `C:\Users\admin\AppData\Roaming\Telegram Desktop\Telegram.exe`,
	},
	{
		Process: "WeChat.exe",
		Path:    `C:\Program Files\Tencent\WeChat\WeChat.exe`,
	},
	{
		Process: "YuanShen.exe",
		Path:    `D:\Games\Genshin Impact\Genshin Impact Game\YuanShen.exe`,
	},
}

// ListRunningApps returns a deterministic desktop process snapshot on non-Windows platforms for UI preview and tests.
func ListRunningApps() ([]contract.RunningApp, error) {
	out := make([]contract.RunningApp, len(defaultStubApps))
	copy(out, defaultStubApps)
	return out, nil
}
