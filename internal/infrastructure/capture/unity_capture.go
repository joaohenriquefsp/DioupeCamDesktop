//go:build windows

package capture

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sharedMemName é o nome da memória compartilhada que o Unity Capture DirectShow filter lê.
// Verificar layout do header em: github.com/schellingb/UnityCapture → UnityCaptureFilter.cpp
const sharedMemName = "UnityCapture_0"

type UnityCaptureWriter struct {
	width     uint32
	height    uint32
	handle    windows.Handle
	mapAddr   uintptr
	frameSize uint32
}

func New(width, height uint32) (*UnityCaptureWriter, error) {
	frameSize := width * height * 4 // BGRA — 4 bytes por pixel
	totalSize := uint32(16) + frameSize

	namePtr, _ := windows.UTF16PtrFromString(sharedMemName)
	handle, err := windows.CreateFileMapping(
		windows.InvalidHandle,
		nil,
		windows.PAGE_READWRITE,
		0,
		totalSize,
		namePtr,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping %q: %w — Unity Capture instalado?", sharedMemName, err)
	}

	addr, err := windows.MapViewOfFile(handle, windows.FILE_MAP_WRITE, 0, 0, uintptr(totalSize))
	if err != nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("MapViewOfFile: %w", err)
	}

	w := &UnityCaptureWriter{
		width:     width,
		height:    height,
		handle:    handle,
		mapAddr:   addr,
		frameSize: frameSize,
	}
	w.writeHeader()
	return w, nil
}

// writeHeader escreve os metadados no início da shared memory.
// Layout: [width uint32][height uint32][format uint32][flags uint32][pixels...]
// format 0 = BGRA32
func (w *UnityCaptureWriter) writeHeader() {
	*(*uint32)(unsafe.Pointer(w.mapAddr + 0)) = w.width
	*(*uint32)(unsafe.Pointer(w.mapAddr + 4)) = w.height
	*(*uint32)(unsafe.Pointer(w.mapAddr + 8)) = 0  // BGRA32
	*(*uint32)(unsafe.Pointer(w.mapAddr + 12)) = 0 // flags reservados
}

func (w *UnityCaptureWriter) WriteFrame(bgra []byte) {
	if uint32(len(bgra)) != w.frameSize {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(w.mapAddr+16)), w.frameSize)
	copy(dst, bgra)
}

func (w *UnityCaptureWriter) Close() {
	windows.UnmapViewOfFile(w.mapAddr)
	windows.CloseHandle(w.handle)
}
