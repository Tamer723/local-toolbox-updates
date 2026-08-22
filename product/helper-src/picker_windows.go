//go:build windows

package main

import (
	"errors"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	comdlg32                 = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

const (
	OFN_FILEMUSTEXIST    = 0x00001000
	OFN_PATHMUSTEXIST    = 0x00000800
	OFN_EXPLORER         = 0x00080000
	OFN_NOCHANGEDIR      = 0x00000008
	BIF_RETURNONLYFSDIRS = 0x00000001
	BIF_NEWDIALOGSTYLE   = 0x00000040
)

type openFileName struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

type browseInfo struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

func utf16Multi(s string) []uint16 {
	r := []rune(s)
	return append(utf16.Encode(r), 0)
}

func pickMediaFile() (string, bool, error) {
	buf := make([]uint16, 32768)
	filter := utf16Multi("Media files\x00*.mp4;*.mkv;*.mov;*.avi;*.webm;*.m4v;*.mp3;*.m4a;*.wav;*.aac;*.flac;*.ogg\x00All files\x00*.*\x00")
	title, _ := syscall.UTF16PtrFromString("Select a video or audio file")
	ofn := openFileName{LpstrFilter: &filter[0], NFilterIndex: 1, LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), LpstrTitle: title, Flags: OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_EXPLORER | OFN_NOCHANGEDIR}
	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
	ret, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		code, _, _ := procCommDlgExtendedError.Call()
		if code == 0 {
			return "", true, nil
		}
		return "", false, errors.New("Windows file dialog error")
	}
	return syscall.UTF16ToString(buf), false, nil
}

func pickFolder() (string, bool, error) {
	display := make([]uint16, 260)
	title, _ := syscall.UTF16PtrFromString("Choose Local Toolbox output folder")
	bi := browseInfo{PszDisplayName: &display[0], LpszTitle: title, UlFlags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", true, nil
	}
	defer procCoTaskMemFree.Call(pidl)
	pathBuf := make([]uint16, 32768)
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ok == 0 {
		return "", false, errors.New("Unable to resolve selected folder")
	}
	return syscall.UTF16ToString(pathBuf), false, nil
}
