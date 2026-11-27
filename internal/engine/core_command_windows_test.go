package engine

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCoreCommandDoesNotAllocateConsole(t *testing.T) {
	const childMarker = "SINGTAIL_TEST_CORE_COMMAND_CHILD"
	if os.Getenv(childMarker) == "1" {
		getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
		hwnd, _, _ := getConsoleWindow.Call()
		if hwnd != 0 {
			t.Fatalf("child process has a console window (hwnd 0x%x)", hwnd)
		}
		return
	}

	probe := newCoreCommand(t.Context(), "sing-box", "version")
	if probe.SysProcAttr == nil || !probe.SysProcAttr.HideWindow || probe.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("expected HideWindow and CREATE_NO_WINDOW, got %+v", probe.SysProcAttr)
	}

	t.Setenv(childMarker, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := newCoreCommand(t.Context(), executable, "-test.run=^TestCoreCommandDoesNotAllocateConsole$")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("expected a console-free child process: %v\n%s", err, output)
	}
}
