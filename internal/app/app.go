package app

import (
	"sync/atomic"

	"dioupecamdesktop/internal/domain"
)

// App orquestra a fonte de stream e o destino de frames.
// Depende apenas de interfaces do domínio — nunca de infraestrutura diretamente.
type App struct {
	running atomic.Bool
	source  domain.StreamSource
	writer  domain.FrameWriter
}

func New() *App {
	return &App{}
}

func (a *App) Start(source domain.StreamSource, writer domain.FrameWriter) error {
	if err := source.Start(writer.WriteFrame); err != nil {
		writer.Close()
		return err
	}
	a.source = source
	a.writer = writer
	a.running.Store(true)
	return nil
}

func (a *App) Stop() {
	if !a.running.Load() {
		return
	}
	a.source.Stop()
	a.writer.Close()
	a.running.Store(false)
}

func (a *App) IsRunning() bool {
	return a.running.Load()
}
