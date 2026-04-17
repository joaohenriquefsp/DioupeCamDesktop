package network

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"dioupecamdesktop/internal/domain"
)

// exeDir retorna o diretório do executável atual.
func exeDir() string {
	if exePath, err := os.Executable(); err == nil {
		return filepath.Dir(exePath)
	}
	return "."
}

// ffmpegPath retorna o caminho para ffmpeg: primeiro tenta o diretório do exe, depois PATH.
func ffmpegPath() string {
	local := filepath.Join(exeDir(), "ffmpeg.exe")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "ffmpeg"
}

// adbForward tenta rodar "adb forward tcp:PORT tcp:PORT" usando o adb.exe
// bundlado no mesmo diretório do app. Falha silenciosa — USB simplesmente não funciona.
func adbForward(port int) {
	adb := filepath.Join(exeDir(), "adb.exe")
	if _, err := os.Stat(adb); err != nil {
		return // adb não bundlado, ignora
	}
	portStr := fmt.Sprintf("tcp:%d", port)
	cmd := exec.Command(adb, "forward", portStr, portStr)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	_ = cmd.Run()
}

// H264Client implementa domain.StreamSource.
// Conecta via TCP (USB first, depois WiFi), decodifica H.264 via FFmpeg e
// entrega frames BGRA via callback.
type H264Client struct {
	cfg      domain.Config
	ffmpeg   *exec.Cmd
	conn     net.Conn
	stopping atomic.Bool
	wg       sync.WaitGroup
	onDone   func(error) // chamado quando o stream cai por erro (não por Stop)
}

func NewH264Client(cfg domain.Config, onDone func(error)) *H264Client {
	return &H264Client{cfg: cfg, onDone: onDone}
}

func (c *H264Client) Start(onFrame func([]byte)) error {
	conn, mode, err := c.connect()
	if err != nil {
		return fmt.Errorf("sem conexão com DioupeCam (tentou USB e WiFi): %w", err)
	}
	log.Printf("conectado via %s", mode)
	c.conn = conn

	frameSize := c.cfg.Width * c.cfg.Height * 4

	// scale com preservação de aspect ratio + padding preto para preencher a resolução alvo.
	// Evita esticamento horizontal quando câmera e config têm proporções diferentes (ex: 4:3 vs 16:9).
	// crop=in_w:in_h-1:0:0 remove a última linha antes do scale:
	// alguns encoders MediaCodec de hardware Android deixam a última linha
	// com chroma não inicializado (aparece verde após decode YUV→RGBA).
	scaleFilter := fmt.Sprintf(
		"crop=in_w:in_h-1:0:0,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1",
		c.cfg.Width, c.cfg.Height, c.cfg.Width, c.cfg.Height,
	)

	cmd := exec.Command(ffmpegPath(),
		"-hide_banner", "-loglevel", "warning",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-fflags", "nobuffer+genpts",
		"-flags", "low_delay",
		"-f", "h264",
		"-i", "pipe:0",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-fps_mode", "passthrough",
		"-vf", scaleFilter,
		"pipe:1",
	)
	c.ffmpeg = cmd

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		conn.Close()
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		conn.Close()
		return err
	}

	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		conn.Close()
		return fmt.Errorf("ffmpeg não encontrado — instale e adicione ao PATH: %w", err)
	}

	// TCP → FFmpeg stdin
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		io.Copy(stdinPipe, conn)
		stdinPipe.Close()
	}()

	// FFmpeg stdout → frames → callback
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		buf := make([]byte, frameSize)
		frameCount := 0
		for {
			if _, err := io.ReadFull(stdoutPipe, buf); err != nil {
				if !c.stopping.Load() {
					log.Printf("stream encerrado após %d frames: %v", frameCount, err)
					if c.onDone != nil {
						c.onDone(err)
					}
				}
				return
			}
			frameCount++
			if frameCount == 1 {
				log.Printf("stream iniciado (%dx%d)", c.cfg.Width, c.cfg.Height)
			}
			if !c.stopping.Load() {
				frame := make([]byte, frameSize)
				copy(frame, buf)
				onFrame(frame)
			}
		}
	}()

	return nil
}

// connect tenta USB (localhost) com timeout curto, depois WiFi com timeout maior.
func (c *H264Client) connect() (net.Conn, string, error) {
	// Tenta configurar ADB forward automaticamente antes de tentar USB
	adbForward(c.cfg.Port)

	usbAddr := fmt.Sprintf("localhost:%d", c.cfg.Port)
	if conn, err := net.DialTimeout("tcp", usbAddr, 500*time.Millisecond); err == nil {
		return conn, "USB", nil
	}

	wifiAddr := fmt.Sprintf("%s:%d", c.cfg.IP, c.cfg.Port)
	conn, err := net.DialTimeout("tcp", wifiAddr, 2*time.Second)
	if err != nil {
		return nil, "", err
	}
	return conn, "WiFi", nil
}

func (c *H264Client) Stop() {
	c.stopping.Store(true)
	if c.conn != nil {
		c.conn.Close()
	}
	if c.ffmpeg != nil && c.ffmpeg.Process != nil {
		c.ffmpeg.Process.Kill()
	}
	// Aguarda as goroutines de I/O terminarem antes de retornar.
	// Sem isso, onFrame pode ser chamado depois que o caller assumiu que
	// o pipeline foi encerrado, causando acesso a estado já liberado.
	c.wg.Wait()
	if c.ffmpeg != nil {
		c.ffmpeg.Wait() // libera recursos do processo filho
	}
}
