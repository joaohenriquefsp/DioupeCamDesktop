package tray

import "bytes"

// generateIcon gera um arquivo ICO 16x16 azul em memória.
// Formato: ICONDIR + ICONDIRENTRY + BITMAPINFOHEADER + pixels BGRA + máscara AND.
// Necessário porque fyne.io/systray passa os bytes direto para CreateIconFromResourceEx
// do Windows, que exige formato ICO nativo — não PNG.
func generateIcon() []byte {
	const (w, h = 16, 16)

	var img bytes.Buffer

	put32 := func(b *bytes.Buffer, v uint32) {
		b.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	}
	put16 := func(b *bytes.Buffer, v uint16) {
		b.Write([]byte{byte(v), byte(v >> 8)})
	}

	// BITMAPINFOHEADER (40 bytes)
	put32(&img, 40)  // biSize
	put32(&img, w)   // biWidth
	put32(&img, h*2) // biHeight dobrado (convenção ICO)
	put16(&img, 1)   // biPlanes
	put16(&img, 32)  // biBitCount
	put32(&img, 0)   // biCompression = BI_RGB
	put32(&img, 0)   // biSizeImage
	put32(&img, 0)   // biXPelsPerMeter
	put32(&img, 0)   // biYPelsPerMeter
	put32(&img, 0)   // biClrUsed
	put32(&img, 0)   // biClrImportant

	// XOR mask: pixels BGRA bottom-up — azul #2196F3
	pixel := [4]byte{243, 150, 33, 255} // BGRA
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			img.Write(pixel[:])
		}
	}

	// AND mask: 1 bit/pixel, bottom-up, linhas alinhadas a 4 bytes (0 = opaco)
	rowStride := ((w + 31) / 32) * 4
	for y := 0; y < h; y++ {
		for i := 0; i < rowStride; i++ {
			img.WriteByte(0)
		}
	}

	imgBytes := img.Bytes()

	// Arquivo ICO completo: ICONDIR + ICONDIRENTRY + dados
	var ico bytes.Buffer
	ico.Write([]byte{0, 0, 1, 0, 1, 0}) // ICONDIR: reserved, type=ICO, count=1

	imgLen := uint32(len(imgBytes))
	ico.Write([]byte{w, h, 0, 0}) // ICONDIRENTRY: width, height, colorCount, reserved
	put16(&ico, 1)                 // wPlanes
	put16(&ico, 32)                // wBitCount
	put32(&ico, imgLen)            // dwBytesInRes
	put32(&ico, 22)                // dwImageOffset (6 + 16)

	ico.Write(imgBytes)
	return ico.Bytes()
}
