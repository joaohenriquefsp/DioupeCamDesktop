package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"sync/atomic"
	"time"
)

type H264Client struct {
	cfg      Config
	onFrame  func([]byte)
	ffmpeg   *exec.Cmd
	conn     net.Conn
	stopping atomic.Bool
}

func NewH264Client(cfg Config, onFrame func([]byte)) *H264Client {
	return &H264Client{cfg: cfg, onFrame: onFrame}
}

// Start connects to DioupeCam (USB first, then WiFi) and begins decoding.
// FFmpeg reads H.264 from stdin and outputs raw BGRA frames to stdout.
func (c *H264Client) Start() error {
	conn, mode, err := c.connect()
	if err != nil {
		return fmt.Errorf("sem conexão com DioupeCam (tentou USB e WiFi): %w", err)
	}
	log.Printf("conectado via %s", mode)
	c.conn = conn

	frameSize := c.cfg.Width * c.cfg.Height * 4 // BGRA

	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "h264",
		"-i", "pipe:0",
		"-f", "rawvideo",
		"-pix_fmt", "bgra",
		"-vf", fmt.Sprintf("scale=%d:%d,setsar=1", c.cfg.Width, c.cfg.Height),
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

	if err := cmd.Start(); err != nil {
		conn.Close()
		return fmt.Errorf("ffmpeg não encontrado — instale e adicione ao PATH: %w", err)
	}

	// TCP → FFmpeg stdin
	go func() {
		io.Copy(stdinPipe, conn)
		stdinPipe.Close()
	}()

	// FFmpeg stdout → frames BGRA
	go func() {
		buf := make([]byte, frameSize)
		for {
			if _, err := io.ReadFull(stdoutPipe, buf); err != nil {
				if !c.stopping.Load() {
					log.Printf("stream encerrado: %v", err)
				}
				return
			}
			if !c.stopping.Load() {
				frame := make([]byte, frameSize)
				copy(frame, buf)
				c.onFrame(frame)
			}
		}
	}()

	return nil
}

// connect tenta USB (localhost) com timeout curto, depois WiFi com timeout maior.
func (c *H264Client) connect() (net.Conn, string, error) {
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
}
