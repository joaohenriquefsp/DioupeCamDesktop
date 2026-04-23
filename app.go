package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	coreapp "dioupecamdesktop/internal/app"
	"dioupecamdesktop/internal/domain"
	"dioupecamdesktop/internal/infrastructure/capture"
	"dioupecamdesktop/internal/infrastructure/config"
	"dioupecamdesktop/internal/infrastructure/network"
	goruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	core    *coreapp.App
	encoder *previewEncoder

	// UnityCaptureWriter vive enquanto o app estiver aberto.
	// Criado no Startup para que Discord/Teams/OBS encontrem a câmera
	// virtual imediatamente, mesmo antes de conectar o stream.
	capture *capture.UnityCaptureWriter
	capW    int
	capH    int

	manualDisconnect atomic.Bool  // true quando o usuário clicou Desconectar
	reconnecting     atomic.Bool  // impede loops de reconexão duplicados
	fpsCounter       atomic.Int64 // incrementado a cada frame recebido
	cancelFps        context.CancelFunc
	mu               sync.Mutex // protege cancelFps
}

func NewApp() *App {
	return &App{core: coreapp.New()}
}

func setupLogger() {
	appData := os.Getenv("APPDATA")
	logDir := filepath.Join(appData, "DioupeCamDesktop")
	os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, "dioupecam.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("[App] log iniciado em: %s", logPath)
}

func (a *App) Startup(ctx context.Context) {
	setupLogger()
	a.ctx = ctx
	log.Printf("[App] Startup — DioupeCam Desktop")
	a.encoder = newPreviewEncoder(func(jpg string) {
		goruntime.EventsEmit(a.ctx, "frame", jpg)
	})

	// Cria shared memory com as dimensões da config salva.
	// Se Discord já estiver aberto, o filtro DirectShow encontrará
	// o header correto ao inicializar a câmera virtual.
	cfg := config.Load()
	if err := a.ensureCapture(cfg.Width, cfg.Height); err != nil {
		log.Printf("aviso: Unity Capture não disponível: %v", err)
	}
}

// Shutdown é chamado pelo Wails quando o app fecha.
// Libera a shared memory e para a goroutine do UnityCaptureWriter.
func (a *App) Shutdown(ctx context.Context) {
	if a.capture != nil {
		a.capture.Close()
		a.capture = nil
	}
}

// ensureCapture cria ou recria o UnityCaptureWriter se as dimensões mudaram.
func (a *App) ensureCapture(w, h int) error {
	if a.capture != nil && a.capW == w && a.capH == h {
		return nil // dimensões iguais, reutiliza
	}
	if a.capture != nil {
		a.capture.Close()
		a.capture = nil
	}
	wr, err := capture.New(uint32(w), uint32(h))
	if err != nil {
		return err
	}
	a.capture = wr
	a.capW = w
	a.capH = h
	return nil
}

func (a *App) Connect() error {
	if a.core.IsRunning() {
		return nil
	}
	a.manualDisconnect.Store(false)

	cfg := config.Load()
	log.Printf("[App] Connect — cfg: IP=%q Port=%d Width=%d Height=%d", cfg.IP, cfg.Port, cfg.Width, cfg.Height)

	if err := a.ensureCapture(cfg.Width, cfg.Height); err != nil {
		return fmt.Errorf("Unity Capture: %w", err)
	}

	a.startFpsGoroutine()

	enc := a.encoder
	cap := a.capture
	src := network.NewH264Client(cfg, a.onStreamEnd)
	return a.core.Start(src, &previewWriter{
		// noCloseWriter impede que core.Stop() feche o UnityCaptureWriter —
		// a shared memory deve persistir entre conexões para o Discord não perder a câmera.
		inner: noCloseWriter{cap},
		onFrame: func(rgba []byte) {
			a.fpsCounter.Add(1)
			enc.push(rgba, cfg.Width, cfg.Height)
		},
	})
}

func (a *App) Disconnect() {
	log.Printf("[App] Disconnect")
	a.manualDisconnect.Store(true)
	a.stopFpsGoroutine()
	a.core.Stop()
}

// onStreamEnd é chamado pela goroutine de leitura do FFmpeg quando o stream cai
// inesperadamente (não por Disconnect). Dispara o loop de auto-reconexão.
func (a *App) onStreamEnd(err error) {
	if a.manualDisconnect.Load() {
		return
	}
	if !a.reconnecting.CompareAndSwap(false, true) {
		return // já há um loop rodando
	}
	go func() {
		defer a.reconnecting.Store(false)
		a.stopFpsGoroutine()
		a.core.Stop()
		goruntime.EventsEmit(a.ctx, "reconnecting", 0)
		a.reconnectLoop()
	}()
}

// reconnectLoop tenta reconectar a cada 3 segundos até ter sucesso ou o
// usuário clicar Desconectar.
func (a *App) reconnectLoop() {
	for attempt := 1; ; attempt++ {
		time.Sleep(3 * time.Second)
		if a.manualDisconnect.Load() || a.core.IsRunning() {
			return
		}
		log.Printf("[auto-reconexão] tentativa %d", attempt)
		goruntime.EventsEmit(a.ctx, "reconnecting", attempt)
		if err := a.Connect(); err == nil {
			log.Printf("[auto-reconexão] reconectado na tentativa %d", attempt)
			goruntime.EventsEmit(a.ctx, "connected", nil)
			return
		}
	}
}

func (a *App) startFpsGoroutine() {
	a.stopFpsGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancelFps = cancel
	a.mu.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fps := a.fpsCounter.Swap(0)
				goruntime.EventsEmit(a.ctx, "fps", fps)
			}
		}
	}()
}

func (a *App) stopFpsGoroutine() {
	a.mu.Lock()
	if a.cancelFps != nil {
		a.cancelFps()
		a.cancelFps = nil
	}
	a.mu.Unlock()
}

func (a *App) IsConnected() bool {
	return a.core.IsRunning()
}

func (a *App) GetConfig() domain.Config {
	return config.Load()
}

func (a *App) SaveConfig(cfg domain.Config) error {
	config.Save(cfg)
	// Recria a shared memory se a resolução mudou, mesmo sem stream ativo.
	// Assim o filtro DirectShow reflete a nova resolução imediatamente.
	if err := a.ensureCapture(cfg.Width, cfg.Height); err != nil {
		log.Printf("aviso: Unity Capture: %v", err)
	}
	if a.core.IsRunning() {
		// manualDisconnect=true evita que o onStreamEnd do Stop() dispare
		// o loop de auto-reconexão antes de Connect() ser chamado logo abaixo.
		a.manualDisconnect.Store(true)
		a.stopFpsGoroutine()
		a.core.Stop()
		return a.Connect()
	}
	return nil
}

// ---------------------------------------------------------------------------
// previewWriter: repassa frames ao UnityCaptureWriter e ao encoder de preview
// ---------------------------------------------------------------------------

type previewWriter struct {
	inner   domain.FrameWriter
	onFrame func([]byte)
}

func (w *previewWriter) WriteFrame(rgba []byte) {
	w.inner.WriteFrame(rgba)
	w.onFrame(rgba)
}

func (w *previewWriter) Close() {
	w.inner.Close()
}

// noCloseWriter envolve um FrameWriter ignorando o Close().
// Permite que core.Stop() não feche o UnityCaptureWriter gerenciado pelo App.
type noCloseWriter struct{ domain.FrameWriter }

func (noCloseWriter) Close() {}

// ---------------------------------------------------------------------------
// previewEncoder: throttle + JPEG encode + emit para o frontend
// ---------------------------------------------------------------------------

type previewEncoder struct {
	mu       sync.Mutex
	latest   []byte
	latestW  int
	latestH  int
	hasFrame bool
	notify   chan struct{}
	lastPush atomic.Int64
}

func newPreviewEncoder(emit func(string)) *previewEncoder {
	pe := &previewEncoder{
		notify: make(chan struct{}, 1),
	}
	go func() {
		for range pe.notify {
			pe.mu.Lock()
			if !pe.hasFrame {
				pe.mu.Unlock()
				continue
			}
			bgra := pe.latest
			w := pe.latestW
			h := pe.latestH
			pe.hasFrame = false
			pe.mu.Unlock()

			if jpg := encodePreviewJPEG(bgra, w, h); jpg != "" {
				emit(jpg)
			}
		}
	}()
	return pe
}

func (pe *previewEncoder) push(rgba []byte, w, h int) {
	now := time.Now().UnixMilli()
	if now-pe.lastPush.Load() < 16 {
		return
	}
	pe.lastPush.Store(now)

	pe.mu.Lock()
	pe.latest = rgba
	pe.latestW = w
	pe.latestH = h
	pe.hasFrame = true
	pe.mu.Unlock()

	select {
	case pe.notify <- struct{}{}:
	default:
	}
}

func encodePreviewJPEG(bgra []byte, srcW, srcH int) string {
	dstW := srcW / 2
	dstH := srcH / 2
	rgba := make([]byte, dstW*dstH*4)

	for y := 0; y < dstH; y++ {
		// Flipa o eixo Y: o buffer vem em ordem bottom-up (convenção DIB do
		// Windows usada pelo Unity Capture). Discord/OBS recebem correto porque
		// o Unity Capture lê assim; o preview precisa inverter para ficar certo.
		srcY := srcH - 2 - y*2
		for x := 0; x < dstW; x++ {
			si := (srcY*srcW + x*2) * 4
			di := (y*dstW + x) * 4
			rgba[di+0] = bgra[si+0]
			rgba[di+1] = bgra[si+1]
			rgba[di+2] = bgra[si+2]
			rgba[di+3] = bgra[si+3]
		}
	}

	img := &image.RGBA{
		Pix:    rgba,
		Stride: dstW * 4,
		Rect:   image.Rect(0, 0, dstW, dstH),
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		log.Printf("preview encode erro: %v", err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
