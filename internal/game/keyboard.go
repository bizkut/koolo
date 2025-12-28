package game

import (
	"math/rand"
	"strings"
	"time"

	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/koolo/internal/utils/winproc"
	"github.com/lxn/win"
)

const (
	keyPressMinTime = 40 // ms
	keyPressMaxTime = 90 // ms
)

// PressKey receives an ASCII code and sends a key press event to the game window
func (hid *HID) PressKey(key byte) {
	// Skip key presses when paused (cursor override disabled)
	if !hid.gi.CursorOverrideActive() {
		return
	}
	win.PostMessage(hid.gr.HWND, win.WM_KEYDOWN, uintptr(key), hid.calculatelParam(key, true))
	sleepTime := rand.Intn(keyPressMaxTime-keyPressMinTime) + keyPressMinTime
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	win.PostMessage(hid.gr.HWND, win.WM_KEYUP, uintptr(key), hid.calculatelParam(key, false))
}

func (hid *HID) KeySequence(keysToPress ...byte) {
	for _, key := range keysToPress {
		hid.PressKey(key)
		time.Sleep(200 * time.Millisecond)
	}
}

// PressKeyWithModifier works the same as PressKey but with a modifier key (shift, ctrl, alt)
func (hid *HID) PressKeyWithModifier(key byte, modifier ModifierKey) {
	hid.gi.OverrideGetKeyState(byte(modifier))
	hid.PressKey(key)
	hid.gi.RestoreGetKeyState()
}

func (hid *HID) PressKeyBinding(kb data.KeyBinding) {
	keys := getKeysForKB(kb)

	// Check if this is a mouse button binding (VK_LBUTTON=1, VK_RBUTTON=2, VK_MBUTTON=4, VK_XBUTTON1=5, VK_XBUTTON2=6)
	// Mouse buttons cannot be sent as keyboard events, they need mouse messages
	keyCode := keys[0]
	if keyCode == win.VK_LBUTTON || keyCode == win.VK_RBUTTON || keyCode == win.VK_MBUTTON ||
		keyCode == win.VK_XBUTTON1 || keyCode == win.VK_XBUTTON2 {
		hid.pressMouseButton(keyCode)
		return
	}

	if keys[1] == 0 || keys[1] == 255 {
		hid.PressKey(keys[0])
		return
	}

	hid.PressKeyWithModifier(keys[0], ModifierKey(keys[1]))
}

// pressMouseButton sends a mouse button click event to the game window
func (hid *HID) pressMouseButton(button byte) {
	// Skip when paused (cursor override disabled)
	if !hid.gi.CursorOverrideActive() {
		return
	}

	var buttonDown, buttonUp uint32
	var wParam uintptr = 0

	switch button {
	case win.VK_LBUTTON: // 1
		buttonDown = win.WM_LBUTTONDOWN
		buttonUp = win.WM_LBUTTONUP
		wParam = win.MK_LBUTTON
	case win.VK_RBUTTON: // 2
		buttonDown = win.WM_RBUTTONDOWN
		buttonUp = win.WM_RBUTTONUP
		wParam = win.MK_RBUTTON
	case win.VK_MBUTTON: // 4
		buttonDown = win.WM_MBUTTONDOWN
		buttonUp = win.WM_MBUTTONUP
		wParam = win.MK_MBUTTON
	case win.VK_XBUTTON1: // 5
		buttonDown = win.WM_XBUTTONDOWN
		buttonUp = win.WM_XBUTTONUP
		// For XBUTTON messages: LOWORD = key state flags, HIWORD = which X button (1 or 2)
		wParam = uintptr(win.MK_XBUTTON1) | (1 << 16)
	case win.VK_XBUTTON2: // 6
		buttonDown = win.WM_XBUTTONDOWN
		buttonUp = win.WM_XBUTTONUP
		wParam = uintptr(win.MK_XBUTTON2) | (2 << 16)
	default:
		return
	}

	// Use the last known cursor position from the memory injector
	// This is the virtual cursor position set by MovePointer()
	cursorX, cursorY := hid.gi.GetLastCursorPos()
	lParam := uintptr(cursorY<<16 | cursorX)

	// Use SendMessage instead of PostMessage to match the behavior of Click()
	// This ensures the input is processed synchronously
	win.SendMessage(hid.gr.HWND, buttonDown, wParam, lParam)
	sleepTime := rand.Intn(keyPressMaxTime-keyPressMinTime) + keyPressMinTime
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	win.SendMessage(hid.gr.HWND, buttonUp, wParam, lParam)
}

// KeyDown sends a key down event to the game window
func (hid *HID) KeyDown(kb data.KeyBinding) {
	// Skip key events when paused (cursor override disabled)
	if !hid.gi.CursorOverrideActive() {
		return
	}
	keys := getKeysForKB(kb)
	win.PostMessage(hid.gr.HWND, win.WM_KEYDOWN, uintptr(keys[0]), hid.calculatelParam(keys[0], true))
}

// KeyUp sends a key up event to the game window
func (hid *HID) KeyUp(kb data.KeyBinding) {
	// Skip key events when paused (cursor override disabled)
	if !hid.gi.CursorOverrideActive() {
		return
	}
	keys := getKeysForKB(kb)
	win.PostMessage(hid.gr.HWND, win.WM_KEYUP, uintptr(keys[0]), hid.calculatelParam(keys[0], false))
}

func getKeysForKB(kb data.KeyBinding) [2]byte {
	if kb.Key1[0] == 0 || kb.Key1[0] == 255 {
		return [2]byte{kb.Key2[0], kb.Key2[1]}
	}

	return [2]byte{kb.Key1[0], kb.Key1[1]}
}

func (hid *HID) GetASCIICode(key string) byte {
	char, found := specialChars[strings.ToLower(key)]
	if found {
		return char
	}

	return strings.ToUpper(key)[0]
}

var specialChars = map[string]byte{
	"esc":       win.VK_ESCAPE,
	"enter":     win.VK_RETURN,
	"f1":        win.VK_F1,
	"f2":        win.VK_F2,
	"f3":        win.VK_F3,
	"f4":        win.VK_F4,
	"f5":        win.VK_F5,
	"f6":        win.VK_F6,
	"f7":        win.VK_F7,
	"f8":        win.VK_F8,
	"f9":        win.VK_F9,
	"f10":       win.VK_F10,
	"f11":       win.VK_F11,
	"f12":       win.VK_F12,
	"lctrl":     win.VK_LCONTROL,
	"home":      win.VK_HOME,
	"down":      win.VK_DOWN,
	"up":        win.VK_UP,
	"left":      win.VK_LEFT,
	"right":     win.VK_RIGHT,
	"tab":       win.VK_TAB,
	"space":     win.VK_SPACE,
	"alt":       win.VK_MENU,
	"lalt":      win.VK_LMENU,
	"ralt":      win.VK_RMENU,
	"shift":     win.VK_LSHIFT,
	"backspace": win.VK_BACK,
	"lwin":      win.VK_LWIN,
	"rwin":      win.VK_RWIN,
	"end":       win.VK_END,
	"-":         win.VK_OEM_MINUS,
}

func (hid *HID) calculatelParam(keyCode byte, down bool) uintptr {
	ret, _, _ := winproc.MapVirtualKey.Call(uintptr(keyCode), 0)
	scanCode := int(ret)
	repeatCount := 1
	extendedKeyFlag := 0
	contextCode := 0
	previousKeyState := 0
	transitionState := 0
	if !down {
		transitionState = 1
	}

	lParam := uintptr((repeatCount & 0xFFFF) | (scanCode << 16) | (extendedKeyFlag << 24) | (contextCode << 29) | (previousKeyState << 30) | (transitionState << 31))
	return lParam
}
