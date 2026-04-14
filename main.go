package main

import "sync/atomic"

// App coordena o H264Client e o UnityCaptureWriter.
type App struct {
	running atomic.Bool
	client  *H264Client
	capture *UnityCaptureWriter
}

func (a *App) Start(cfg Config) error {
	writer, err := NewUnityCaptureWriter(uint32(cfg.Width), uint32(cfg.Height))
	if err != nil {
		return err
	}

	client := NewH264Client(cfg, func(frame []byte) {
		writer.WriteFrame(frame)
	})

	if err := client.Start(); err != nil {
		writer.Close()
		return err
	}

	a.capture = writer
	a.client = client
	a.running.Store(true)
	return nil
}

func (a *App) Stop() {
	if !a.running.Load() {
		return
	}
	a.client.Stop()
	a.capture.Close()
	a.running.Store(false)
}

func (a *App) IsRunning() bool {
	return a.running.Load()
}

func main() {
	app := &App{}

	cfg := loadConfig()
	_ = app.Start(cfg) // ignora erro — usuário pode reconectar pelo menu

	runTray(app)
}
