package main

import (
	"bytes"
	"os/exec"

	"fyne.io/systray"
)

func runTray(app *App) {
	systray.Run(
		func() { onTrayReady(app) },
		func() { app.Stop() },
	)
}

func onTrayReady(app *App) {
	systray.SetIcon(generateIcon())
	systray.SetTitle("DioupeCam")
	systray.SetTooltip("DioupeCamDesktop — câmera virtual")

	mStatus := systray.AddMenuItem("Desconectado", "")
	mStatus.Disable()
	systray.AddSeparator()

	mToggle := systray.AddMenuItem("Conectar", "Inicia/para o stream")
	mConfig := systray.AddMenuItem("Editar configuração", "Abre config.json no Notepad")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Sair", "")

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				if app.IsRunning() {
					app.Stop()
					mToggle.SetTitle("Conectar")
					mStatus.SetTitle("Desconectado")
				} else {
					cfg := loadConfig()
					if err := app.Start(cfg); err != nil {
						mStatus.SetTitle("Erro: " + err.Error())
					} else {
						mToggle.SetTitle("Desconectar")
						mStatus.SetTitle("Conectado · " + cfg.IP)
					}
				}

			case <-mConfig.ClickedCh:
				_ = exec.Command("notepad.exe", configPath()).Start()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// generateIcon gera um arquivo ICO 16x16 azul em memória.
// Formato: ICONDIR + ICONDIRENTRY + BITMAPINFOHEADER + pixels BGRA + máscara AND.
func generateIcon() []byte {
	const (w, h = 16, 16)

	// --- dados da imagem (BITMAPINFOHEADER + XOR mask + AND mask) ---
	var img bytes.Buffer

	put32 := func(b *bytes.Buffer, v uint32) {
		b.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	}
	put16 := func(b *bytes.Buffer, v uint16) {
		b.Write([]byte{byte(v), byte(v >> 8)})
	}

	// BITMAPINFOHEADER (40 bytes)
	put32(&img, 40)    // biSize
	put32(&img, w)     // biWidth
	put32(&img, h*2)   // biHeight dobrado (convenção ICO)
	put16(&img, 1)     // biPlanes
	put16(&img, 32)    // biBitCount
	put32(&img, 0)     // biCompression = BI_RGB
	put32(&img, 0)     // biSizeImage
	put32(&img, 0)     // biXPelsPerMeter
	put32(&img, 0)     // biYPelsPerMeter
	put32(&img, 0)     // biClrUsed
	put32(&img, 0)     // biClrImportant

	// XOR mask: pixels BGRA bottom-up — azul #2196F3
	pixel := [4]byte{243, 150, 33, 255} // BGRA
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			img.Write(pixel[:])
		}
	}

	// AND mask: 1 bit/pixel, bottom-up, linhas alinhadas a 4 bytes (tudo 0 = opaco)
	rowStride := ((w + 31) / 32) * 4
	for y := 0; y < h; y++ {
		for i := 0; i < rowStride; i++ {
			img.WriteByte(0)
		}
	}

	imgBytes := img.Bytes()

	// --- arquivo ICO completo ---
	var ico bytes.Buffer

	// ICONDIR (6 bytes)
	ico.Write([]byte{0, 0, 1, 0, 1, 0}) // reserved, type=ICO, count=1

	// ICONDIRENTRY (16 bytes)
	imgLen := uint32(len(imgBytes))
	ico.Write([]byte{w, h, 0, 0}) // width, height, colorCount, reserved
	put16(&ico, 1)                 // wPlanes
	put16(&ico, 32)                // wBitCount
	put32(&ico, imgLen)            // dwBytesInRes
	put32(&ico, 22)                // dwImageOffset (6 + 16)

	// dados da imagem
	ico.Write(imgBytes)

	return ico.Bytes()
}
