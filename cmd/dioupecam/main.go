package main

import (
	"dioupecamdesktop/internal/app"
	"dioupecamdesktop/internal/infrastructure/capture"
	"dioupecamdesktop/internal/infrastructure/config"
	"dioupecamdesktop/internal/infrastructure/network"
	"dioupecamdesktop/internal/ui/tray"
)

func main() {
	a := app.New()

	// Tenta conectar automaticamente na inicialização.
	// Se falhar (sem Android conectado), o usuário reconecta pelo menu da bandeja.
	_ = connect(a)

	tray.Run(a, func() error {
		return connect(a)
	})
}

// connect cria as dependências de infraestrutura e inicia o stream.
// Separado para ser reutilizado no startup e no botão "Conectar" da bandeja.
func connect(a *app.App) error {
	cfg := config.Load()

	writer, err := capture.New(uint32(cfg.Width), uint32(cfg.Height))
	if err != nil {
		return err
	}

	src := network.NewH264Client(cfg)
	return a.Start(src, writer)
}
