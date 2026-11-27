//go:build windows

package syswin

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TrayAction represents a clickable menu entry in the system tray popup menu.
type TrayAction struct {
	Label   string
	Checked bool
	OnClick func()
}

// TrayConfig configures the Windows notification area (system tray) icon and menu callbacks.
type TrayConfig struct {
	Tooltip  string
	OnOpen   func()
	OnToggle func()
	OnQuit   func()
	Actions  []TrayAction
}

const (
	wmClose          = 0x0010
	wmDestroy        = 0x0002
	wmCommand        = 0x0111
	wmApp            = 0x8000
	wmTrayCallback   = wmApp + 1
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonUp      = 0x0205
	nimAdd           = 0x00000000
	nimDelete        = 0x00000002
	nifMessage       = 0x00000001
	nifIcon          = 0x00000002
	nifTip           = 0x00000004
	idiApplication   = 32512
	mfString         = 0x00000000
	mfChecked        = 0x00000008
	mfSeparator      = 0x00000800
	tpmRightButton   = 0x0002
	tpmBottomAlign   = 0x0020
	wsOverlappedWin  = 0x00CF0000
	cmdOpenWindow    = 1001
	cmdToggleService = 1002
	cmdQuitApp       = 1099
	cmdActionBase    = 2000
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
)

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type point struct {
	X int32
	Y int32
}

type winMsg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

var (
	trayMu       sync.Mutex
	activeTrays  = make(map[windows.Handle]*trayInstance)
	wndProcOnce  sync.Once
	trayWndProcP uintptr
)

type trayInstance struct {
	hwnd windows.Handle
	cfg  TrayConfig
}

// OpenBrowser opens an http or https URL in the default Windows web browser.
func OpenBrowser(rawURL string) error {
	validated, err := validateHTTPURL(rawURL)
	if err != nil {
		return err
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", validated).Start()
}

func validateHTTPURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", errors.New("browser url cannot be empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid browser url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("unsupported browser url scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

// RunTray creates a zero-CGO Win32 notification icon and processes tray events until stopCh closes or the user quits.
func RunTray(cfg TrayConfig, stopCh <-chan struct{}) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hIcon, _, _ := procLoadIconW.Call(0, idiApplication)
	className, err := windows.UTF16PtrFromString("SingTailTrayWindowClass")
	if err != nil {
		return err
	}

	wndProcOnce.Do(func() {
		trayWndProcP = windows.NewCallback(trayWindowProc)
	})

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   trayWndProcP,
		HInstance:     windows.Handle(hInstance),
		HIcon:         windows.Handle(hIcon),
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	winTitle, _ := windows.UTF16PtrFromString("SingTailTray")
	hwndRaw, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		wsOverlappedWin,
		0, 0, 0, 0,
		0, 0, hInstance, 0,
	)
	if hwndRaw == 0 {
		return fmt.Errorf("create tray message window: %w", createErr)
	}
	hwnd := windows.Handle(hwndRaw)

	inst := &trayInstance{hwnd: hwnd, cfg: cfg}
	trayMu.Lock()
	activeTrays[hwnd] = inst
	trayMu.Unlock()
	defer func() {
		trayMu.Lock()
		delete(activeTrays, hwnd)
		trayMu.Unlock()
	}()

	nid := notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            windows.Handle(hIcon),
	}
	copyUTF16Tip(&nid.SzTip, cfg.Tooltip, "SingTail")

	if ok, _, addErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); ok == 0 {
		procDestroyWindow.Call(uintptr(hwnd))
		return fmt.Errorf("add tray icon: %w", addErr)
	}
	defer procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))

	doneStopWatcher := make(chan struct{})
	defer close(doneStopWatcher)
	if stopCh != nil {
		go func() {
			select {
			case <-stopCh:
				procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
			case <-doneStopWatcher:
			}
		}()
	}

	var msg winMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func copyUTF16Tip(dst *[128]uint16, tooltip, fallback string) {
	text := strings.TrimSpace(tooltip)
	if text == "" {
		text = fallback
	}
	encoded, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	n := len(encoded)
	if n > len(dst) {
		n = len(dst)
		encoded[n-1] = 0
	}
	copy(dst[:], encoded[:n])
}

func trayWindowProc(hwnd windows.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	trayMu.Lock()
	inst := activeTrays[hwnd]
	trayMu.Unlock()

	switch msg {
	case wmTrayCallback:
		if inst != nil {
			switch uint32(lParam) {
			case wmLButtonUp, wmLButtonDblClk:
				if inst.cfg.OnOpen != nil {
					inst.cfg.OnOpen()
				}
			case wmRButtonUp:
				showTrayContextMenu(hwnd, inst.cfg)
			}
		}
		return 0
	case wmCommand:
		if inst != nil {
			handleTrayMenuCommand(hwnd, inst.cfg, int(wParam&0xFFFF))
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func showTrayContextMenu(hwnd windows.Handle, cfg TrayConfig) {
	menuRaw, _, _ := procCreatePopupMenu.Call()
	if menuRaw == 0 {
		return
	}
	defer procDestroyMenu.Call(menuRaw)

	appendMenuText(menuRaw, mfString, cmdOpenWindow, "Open SingTail")
	if cfg.OnToggle != nil {
		appendMenuText(menuRaw, mfString, cmdToggleService, "Toggle Routing")
	}
	if len(cfg.Actions) > 0 {
		procAppendMenuW.Call(menuRaw, mfSeparator, 0, 0)
		for idx, act := range cfg.Actions {
			flags := uintptr(mfString)
			if act.Checked {
				flags |= mfChecked
			}
			appendMenuText(menuRaw, flags, uintptr(cmdActionBase+idx), act.Label)
		}
	}
	procAppendMenuW.Call(menuRaw, mfSeparator, 0, 0)
	appendMenuText(menuRaw, mfString, cmdQuitApp, "Quit")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWin.Call(uintptr(hwnd))
	procTrackPopupMenu.Call(
		menuRaw,
		tpmRightButton|tpmBottomAlign,
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(hwnd),
		0,
	)
}

func appendMenuText(menu uintptr, flags uintptr, id uintptr, text string) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(ptr)))
}

func handleTrayMenuCommand(hwnd windows.Handle, cfg TrayConfig, cmdID int) {
	switch {
	case cmdID == cmdOpenWindow:
		if cfg.OnOpen != nil {
			cfg.OnOpen()
		}
	case cmdID == cmdToggleService:
		if cfg.OnToggle != nil {
			cfg.OnToggle()
		}
	case cmdID == cmdQuitApp:
		if cfg.OnQuit != nil {
			cfg.OnQuit()
		}
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	case cmdID >= cmdActionBase && cmdID < cmdActionBase+len(cfg.Actions):
		act := cfg.Actions[cmdID-cmdActionBase]
		if act.OnClick != nil {
			act.OnClick()
		}
	}
}
