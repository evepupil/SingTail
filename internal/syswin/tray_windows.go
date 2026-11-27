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

// TrayAction represents a clickable menu entry or visual separator in the system tray popup menu.
type TrayAction struct {
	Label     string
	Checked   bool
	Separator bool
	OnClick   func()
}

// TrayConfig configures the Windows notification area (system tray) icon and menu callbacks.
type TrayConfig struct {
	Tooltip      string
	OnOpen       func()
	OnToggle     func()
	OnQuit       func()
	Actions      []TrayAction
	BuildActions func() []TrayAction
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
	user32                 = windows.NewLazySystemDLL("user32.dll")
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procLoadIconW          = user32.NewProc("LoadIconW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procAppendMenuW        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procSetForegroundWin   = user32.NewProc("SetForegroundWindow")
	procShellNotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
)

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

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
	hwnd        windows.Handle
	cfg         TrayConfig
	mu          sync.Mutex
	lastActions []TrayAction
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

// RunTray creates a zero-CGO Win32 notification icon with a custom ST badge and processes tray events until stopCh closes or the user quits.
func RunTray(cfg TrayConfig, stopCh <-chan struct{}) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hIcon, ownsIcon := createBrandTrayIcon()
	if hIcon == 0 {
		fallback, _, _ := procLoadIconW.Call(0, idiApplication)
		hIcon = windows.Handle(fallback)
	}
	if ownsIcon && hIcon != 0 {
		defer procDestroyIcon.Call(uintptr(hIcon))
	}

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
		HIcon:         hIcon,
		HIconSm:       hIcon,
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
		HIcon:            hIcon,
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

// createBrandTrayIcon renders a 32x32 pixel-sharp SingTail exit-node matrix icon (#121214 dark fill, #ECECEE path, #CEFF00 active exit node).
func createBrandTrayIcon() (windows.Handle, bool) {
	const (
		size   = 32
		alpha0 = uint32(0x00000000)
		bgDark = uint32(0xFF121214)
		border = uint32(0xFF36363D)
		muted  = uint32(0xFF3E3E46)
		ink    = uint32(0xFFECECEE)
		fgLime = uint32(0xFFCEFF00)
	)

	var pixels [size * size]uint32
	setPx := func(x, y int, c uint32) {
		if x >= 0 && x < size && y >= 0 && y < size {
			pixels[y*size+x] = c
		}
	}
	fillRect := func(x0, y0, w, h int, c uint32) {
		for dy := 0; dy < h; dy++ {
			for dx := 0; dx < w; dx++ {
				setPx(x0+dx, y0+dy, c)
			}
		}
	}
	strokeRect := func(x0, y0, w, h int, c uint32) {
		for dx := 0; dx < w; dx++ {
			setPx(x0+dx, y0, c)
			setPx(x0+dx, y0+h-1, c)
		}
		for dy := 0; dy < h; dy++ {
			setPx(x0, y0+dy, c)
			setPx(x0+w-1, y0+dy, c)
		}
	}

	// Architectural low-radius tile background with 1px border and clipped 1px outer corners
	fillRect(0, 0, size, size, bgDark)
	strokeRect(0, 0, size, size, border)
	for _, pt := range [][2]int{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
		setPx(pt[0], pt[1], alpha0)
	}
	for _, pt := range [][2]int{{1, 1}, {size - 2, 1}, {1, size - 2}, {size - 2, size - 2}} {
		setPx(pt[0], pt[1], border)
	}

	// 5 inactive hollow peer nodes in the 3x3 mesh (6x6 at coords 5, 13, 21)
	for _, pt := range [][2]int{{5, 5}, {13, 5}, {5, 13}, {21, 13}, {21, 21}} {
		strokeRect(pt[0], pt[1], 6, 6, muted)
	}

	// Routed tail connectors (drawn behind active nodes for crisp corners)
	fillRect(11, 23, 2, 2, ink)
	fillRect(15, 19, 2, 2, ink)
	for i := 0; i <= 5; i++ {
		setPx(17+i, 14-i, fgLime)
		setPx(18+i, 14-i, fgLime)
	}

	// 3 active path nodes + top-right highlighted exit node
	for _, pt := range [][2]int{{5, 21}, {13, 21}, {13, 13}} {
		fillRect(pt[0], pt[1], 6, 6, ink)
	}
	fillRect(21, 5, 6, 6, fgLime)

	var mask [size * (size / 8)]byte
	hbmColor, _, _ := procCreateBitmap.Call(size, size, 1, 32, uintptr(unsafe.Pointer(&pixels[0])))
	if hbmColor == 0 {
		return 0, false
	}
	defer procDeleteObject.Call(hbmColor)

	hbmMask, _, _ := procCreateBitmap.Call(size, size, 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if hbmMask == 0 {
		return 0, false
	}
	defer procDeleteObject.Call(hbmMask)

	ii := iconInfo{
		FIcon:    1,
		HbmMask:  windows.Handle(hbmMask),
		HbmColor: windows.Handle(hbmColor),
	}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if hIcon == 0 {
		return 0, false
	}
	return windows.Handle(hIcon), true
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
					go inst.cfg.OnOpen()
				}
			case wmRButtonUp:
				showTrayContextMenu(hwnd, inst)
			}
		}
		return 0
	case wmCommand:
		if inst != nil {
			handleTrayMenuCommand(hwnd, inst, int(wParam&0xFFFF))
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

func showTrayContextMenu(hwnd windows.Handle, inst *trayInstance) {
	menuRaw, _, _ := procCreatePopupMenu.Call()
	if menuRaw == 0 {
		return
	}
	defer procDestroyMenu.Call(menuRaw)

	cfg := inst.cfg
	var actions []TrayAction
	if cfg.BuildActions != nil {
		actions = cfg.BuildActions()
	} else {
		actions = cfg.Actions
	}
	inst.mu.Lock()
	inst.lastActions = actions
	inst.mu.Unlock()

	appendMenuText(menuRaw, mfString, cmdOpenWindow, "显示 SingTail 主窗口")
	if cfg.OnToggle != nil && len(actions) == 0 {
		appendMenuText(menuRaw, mfString, cmdToggleService, "切换 TUN 开关")
	}
	if len(actions) > 0 {
		procAppendMenuW.Call(menuRaw, mfSeparator, 0, 0)
		for idx, act := range actions {
			if act.Separator {
				procAppendMenuW.Call(menuRaw, mfSeparator, 0, 0)
				continue
			}
			flags := uintptr(mfString)
			if act.Checked {
				flags |= mfChecked
			}
			appendMenuText(menuRaw, flags, uintptr(cmdActionBase+idx), act.Label)
		}
	}
	procAppendMenuW.Call(menuRaw, mfSeparator, 0, 0)
	appendMenuText(menuRaw, mfString, cmdQuitApp, "退出 SingTail")

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

func handleTrayMenuCommand(hwnd windows.Handle, inst *trayInstance, cmdID int) {
	cfg := inst.cfg
	inst.mu.Lock()
	actions := inst.lastActions
	inst.mu.Unlock()

	switch {
	case cmdID == cmdOpenWindow:
		if cfg.OnOpen != nil {
			go cfg.OnOpen()
		}
	case cmdID == cmdToggleService:
		if cfg.OnToggle != nil {
			go cfg.OnToggle()
		}
	case cmdID == cmdQuitApp:
		if cfg.OnQuit != nil {
			go cfg.OnQuit()
		}
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	case cmdID >= cmdActionBase && cmdID < cmdActionBase+len(actions):
		act := actions[cmdID-cmdActionBase]
		if !act.Separator && act.OnClick != nil {
			go act.OnClick()
		}
	}
}
