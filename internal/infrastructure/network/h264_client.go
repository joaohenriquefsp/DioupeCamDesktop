package network

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
		log.Printf("[ADB] adb.exe não encontrado em %s — USB não disponível", adb)
		return
	}
	portStr := fmt.Sprintf("tcp:%d", port)
	cmd := exec.Command(adb, "forward", portStr, portStr)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	log.Printf("[ADB] forward %s %s: err=%v output=%q", portStr, portStr, err, strings.TrimSpace(out.String()))
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
	log.Printf("[H264] conectado via %s — addr remoto: %s", mode, conn.RemoteAddr())
	c.conn = conn

	frameSize := c.cfg.Width * c.cfg.Height * 4
	log.Printf("[H264] frameSize=%d bytes (%dx%d RGBA)", frameSize, c.cfg.Width, c.cfg.Height)

	scaleFilter := fmt.Sprintf(
		"crop=in_w:in_h-1:0:0,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1",
		c.cfg.Width, c.cfg.Height, c.cfg.Width, c.cfg.Height,
	)

	ffmpegBin := ffmpegPath()
	args := []string{
		"-hide_banner", "-loglevel", "verbose",
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
	}
	log.Printf("[H264] FFmpeg bin: %s", ffmpegBin)
	log.Printf("[H264] FFmpeg args: %s", strings.Join(args, " "))

	cmd := exec.Command(ffmpegBin, args...)
	// CREATE_NO_WINDOW: impede que o Windows aloque um novo console para o
	// processo filho (ffmpeg.exe é subsistema console). Sem essa flag, num app
	// GUI (windowsgui) o filho recebe handles de console em vez dos nossos pipes,
	// quebrando stdin/stdout e fazendo o preview nunca receber frames.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	c.ffmpeg = cmd

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		conn.Close()
		return fmt.Errorf("StdinPipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		conn.Close()
		return fmt.Errorf("StdoutPipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		conn.Close()
		return fmt.Errorf("StderrPipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		conn.Close()
		return fmt.Errorf("FFmpeg não iniciou (%s): %w", ffmpegBin, err)
	}
	log.Printf("[H264] FFmpeg iniciado — PID %d", cmd.Process.Pid)

	// Log FFmpeg stderr linha a linha
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Printf("[FFmpeg] %s", scanner.Text())
		}
		log.Printf("[FFmpeg] stderr encerrado")
	}()

	// TCP → FFmpeg stdin
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		n, err := io.Copy(stdinPipe, conn)
		log.Printf("[H264] TCP→FFmpeg stdin encerrado: %d bytes copiados, err=%v", n, err)
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
					log.Printf("[H264] ReadFull encerrado após %d frames: %v", frameCount, err)
					if c.onDone != nil {
						c.onDone(err)
					}
				}
				return
			}
			frameCount++
			if frameCount == 1 {
				log.Printf("[H264] PRIMEIRO frame recebido! resolução=%dx%d", c.cfg.Width, c.cfg.Height)
			} else if frameCount%150 == 0 {
				log.Printf("[H264] frame #%d", frameCount)
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
	log.Printf("[H264] iniciando conexão — porta %d, IP WiFi: %q", c.cfg.Port, c.cfg.IP)
	adbForward(c.cfg.Port)

	usbAddr := fmt.Sprintf("localhost:%d", c.cfg.Port)
	log.Printf("[H264] tentando USB: %s (timeout 500ms)", usbAddr)
	if conn, err := net.DialTimeout("tcp", usbAddr, 500*time.Millisecond); err == nil {
		log.Printf("[H264] USB OK: %s", usbAddr)
		return conn, "USB", nil
	} else {
		log.Printf("[H264] USB falhou: %v", err)
	}

	wifiAddr := fmt.Sprintf("%s:%d", c.cfg.IP, c.cfg.Port)
	log.Printf("[H264] tentando WiFi: %s (timeout 2s)", wifiAddr)
	conn, err := net.DialTimeout("tcp", wifiAddr, 2*time.Second)
	if err != nil {
		log.Printf("[H264] WiFi falhou: %v", err)
		return nil, "", err
	}
	log.Printf("[H264] WiFi OK: %s", wifiAddr)
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
