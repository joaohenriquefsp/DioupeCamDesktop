package tray

import (
	"os/exec"

	"dioupecamdesktop/internal/app"
	"dioupecamdesktop/internal/infrastructure/config"
	"fyne.io/systray"
)

// ConnectFunc é injetada pelo cmd/dioupecam/main.go e cria as dependências de
// infraestrutura (capture + network) antes de chamar app.Start.
type ConnectFunc func() error

// Run inicia o loop da bandeja do sistema. Bloqueia até o usuário clicar Sair.
func Run(a *app.App, connect ConnectFunc) {
	systray.Run(
		func() { onReady(a, connect) },
		func() { a.Stop() },
	)
}

func onReady(a *app.App, connect ConnectFunc) {
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
				if a.IsRunning() {
					a.Stop()
					mToggle.SetTitle("Conectar")
					mStatus.SetTitle("Desconectado")
				} else {
					if err := connect(); err != nil {
						mStatus.SetTitle("Erro: " + err.Error())
					} else {
						cfg := config.Load()
						mToggle.SetTitle("Desconectar")
						mStatus.SetTitle("Conectado · " + cfg.IP)
					}
				}

			case <-mConfig.ClickedCh:
				_ = exec.Command("notepad.exe", config.Path()).Start()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
