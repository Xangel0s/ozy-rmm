//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC              = gdi32.NewProc("DeleteDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject          = gdi32.NewProc("SelectObject")
	procDeleteObject          = gdi32.NewProc("DeleteObject")
	procBitBlt                = gdi32.NewProc("BitBlt")
	procGetDIBits             = gdi32.NewProc("GetDIBits")

	procSendInput = user32.NewProc("SendInput")
)

const (
	SM_CXSCREEN     = 0
	SM_CYSCREEN     = 1
	SRCCOPY         = 0x00CC0020
	DIB_RGB_COLORS  = 0
	BI_RGB          = 0
	INPUT_MOUSE     = 0
	INPUT_KEYBOARD  = 1
	MOUSEEVENTF_MOVE      = 0x0001
	MOUSEEVENTF_LEFTDOWN  = 0x0002
	MOUSEEVENTF_LEFTUP    = 0x0004
	MOUSEEVENTF_RIGHTDOWN = 0x0008
	MOUSEEVENTF_RIGHTUP   = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP  = 0x0040
	MOUSEEVENTF_WHEEL     = 0x0800
	MOUSEEVENTF_ABSOLUTE  = 0x8000
	KEYEVENTF_KEYDOWN     = 0x0000
	KEYEVENTF_KEYUP       = 0x0002
)

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [4]byte
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type HARDWAREINPUT struct {
	UMsg    uint32
	WParamL  uint16
	WParamH  uint16
	DwExtraInfo uintptr
}

type INPUT struct {
	Type uint32
	_    [4]byte // padding
	Mi   MOUSEINPUT
	Ki   KEYBDINPUT
	Hi   HARDWAREINPUT
}

func captureScreenJPEG(quality int) ([]byte, error) {
	width, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	height, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)

	if width == 0 || height == 0 {
		return nil, fmt.Errorf("GetSystemMetrics returned 0")
	}

	deskDC, _, _ := procGetDC.Call(0)
	if deskDC == 0 {
		return nil, fmt.Errorf("GetDC(0) failed")
	}
	defer procReleaseDC.Call(0, deskDC)

	memDC, _, _ := procCreateCompatibleDC.Call(deskDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	hBitmap, _, _ := procCreateCompatibleBitmap.Call(deskDC, width, height)
	if hBitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(hBitmap)

	procSelectObject.Call(memDC, hBitmap)

	ret, _, _ := procBitBlt.Call(memDC, 0, 0, width, height, deskDC, 0, 0, SRCCOPY)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	var bmi BITMAPINFO
	bmi.BmiHeader.BiSize = uint32(unsafe.Sizeof(bmi.BmiHeader))
	bmi.BmiHeader.BiWidth = int32(width)
	bmi.BmiHeader.BiHeight = -int32(height)
	bmi.BmiHeader.BiPlanes = 1
	bmi.BmiHeader.BiBitCount = 32
	bmi.BmiHeader.BiCompression = BI_RGB

	pixels := make([]byte, width*height*4)

	ret, _, _ = procGetDIBits.Call(deskDC, hBitmap, 0, uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			srcIdx := (y*int(width) + x) * 4
			dstIdx := (y*int(width) + x) * 4
			img.Pix[dstIdx+0] = pixels[srcIdx+2]
			img.Pix[dstIdx+1] = pixels[srcIdx+1]
			img.Pix[dstIdx+2] = pixels[srcIdx+0]
			img.Pix[dstIdx+3] = 255
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("JPEG encode failed: %v", err)
	}

	return buf.Bytes(), nil
}

func mouseMove(x, y int) {
	screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)

	absX := uint32((x * 65535) / (int(screenW) - 1))
	absY := uint32((y * 65535) / (int(screenH) - 1))

	var mi MOUSEINPUT
	mi.Dx = int32(absX)
	mi.Dy = int32(absY)
	mi.DwFlags = MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE

	var input INPUT
	input.Type = INPUT_MOUSE
	input.Mi = mi

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func mouseDown(button string) {
	var flags uint32
	switch button {
	case "left":
		flags = MOUSEEVENTF_LEFTDOWN
	case "right":
		flags = MOUSEEVENTF_RIGHTDOWN
	case "middle":
		flags = MOUSEEVENTF_MIDDLEDOWN
	default:
		flags = MOUSEEVENTF_LEFTDOWN
	}

	var mi MOUSEINPUT
	mi.DwFlags = flags

	var input INPUT
	input.Type = INPUT_MOUSE
	input.Mi = mi

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func mouseUp(button string) {
	var flags uint32
	switch button {
	case "left":
		flags = MOUSEEVENTF_LEFTUP
	case "right":
		flags = MOUSEEVENTF_RIGHTUP
	case "middle":
		flags = MOUSEEVENTF_MIDDLEUP
	default:
		flags = MOUSEEVENTF_LEFTUP
	}

	var mi MOUSEINPUT
	mi.DwFlags = flags

	var input INPUT
	input.Type = INPUT_MOUSE
	input.Mi = mi

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func mouseScroll(delta int) {
	var mi MOUSEINPUT
	mi.MouseData = uint32(delta)
	mi.DwFlags = MOUSEEVENTF_WHEEL

	var input INPUT
	input.Type = INPUT_MOUSE
	input.Mi = mi

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

// virtualKeyCode maps common key names to Windows virtual key codes.
// For unmapped keys we pass through the key code from the event.
func virtualKeyCode(key string) uint16 {
	codes := map[string]uint16{
		"Enter":      13,
		"Escape":     27,
		"Tab":        9,
		"Backspace":  8,
		"Delete":     46,
		"Insert":     45,
		"Home":       36,
		"End":        35,
		"PageUp":     33,
		"PageDown":   34,
		"ArrowUp":    38,
		"ArrowDown":  40,
		"ArrowLeft":  37,
		"ArrowRight": 39,
		"F1":         112, "F2": 113, "F3": 114, "F4": 115,
		"F5":         116, "F6": 117, "F7": 118, "F8": 119,
		"F9":         120, "F10": 121, "F11": 122, "F12": 123,
		"Shift":      16,
		"Control":    17,
		"Alt":        18,
		"CapsLock":   20,
		"Space":      32,
	}
	if code, ok := codes[key]; ok {
		return code
	}
	// Single character keys: pass through as ASCII/uppercase
	if len(key) == 1 {
		c := key[0]
		if c >= 'a' && c <= 'z' {
			return uint16(c - 32) // uppercase
		}
		return uint16(c)
	}
	return 0
}

func keyDown(key string) {
	vk := virtualKeyCode(key)
	if vk == 0 {
		return
	}

	var ki KEYBDINPUT
	ki.WVk = vk
	ki.DwFlags = KEYEVENTF_KEYDOWN

	var input INPUT
	input.Type = INPUT_KEYBOARD
	input.Ki = ki

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

func keyUp(key string) {
	vk := virtualKeyCode(key)
	if vk == 0 {
		return
	}

	var ki KEYBDINPUT
	ki.WVk = vk
	ki.DwFlags = KEYEVENTF_KEYUP

	var input INPUT
	input.Type = INPUT_KEYBOARD
	input.Ki = ki

	syscall.Syscall(procSendInput.Addr(), 3, 1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
}

type screenInputEvent struct {
	Type     string `json:"type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Button   string `json:"button"`
	Delta    int    `json:"delta"`
	Key      string `json:"key"`
	Alt      bool   `json:"alt"`
	Ctrl     bool   `json:"ctrl"`
	Shift    bool   `json:"shift"`
	Code     string `json:"code"`
}

func handleScreenInput(payload string) {
	var evt screenInputEvent
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		log.Printf("screen_input: invalid payload: %v", err)
		return
	}

	// Apply modifier keys
	if evt.Ctrl {
		keyDown("Control")
		defer keyUp("Control")
	}
	if evt.Alt {
		keyDown("Alt")
		defer keyUp("Alt")
	}
	if evt.Shift {
		keyDown("Shift")
		defer keyUp("Shift")
	}

	switch evt.Type {
	case "mouse_move":
		mouseMove(evt.X, evt.Y)
	case "mouse_down":
		mouseDown(evt.Button)
	case "mouse_up":
		mouseUp(evt.Button)
	case "mouse_scroll":
		mouseScroll(evt.Delta)
	case "key_down":
		if evt.Key != "Control" && evt.Key != "Alt" && evt.Key != "Shift" {
			keyDown(evt.Key)
		}
	case "key_up":
		if evt.Key != "Control" && evt.Key != "Alt" && evt.Key != "Shift" {
			keyUp(evt.Key)
		}
	}
}
