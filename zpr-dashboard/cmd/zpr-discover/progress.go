package main

import (
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

type progressDisplay struct {
	writer io.Writer
	width  func() (int, error)
}

func newProgressDisplay(writer io.Writer) progressDisplay {
	display := progressDisplay{writer: writer}
	if file, ok := writer.(*os.File); ok && os.Getenv("TERM") != "dumb" && term.IsTerminal(file.Fd()) {
		display.width = func() (int, error) {
			width, _, err := term.GetSize(file.Fd())
			return width, err
		}
	}
	return display
}

func (display progressDisplay) write(progress discovery.Progress) error {
	if display.width == nil {
		line, err := graphicalProgress(progress)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(display.writer, "Progress: %s\n", line)
		return err
	}
	width, err := display.width()
	if err != nil {
		return fmt.Errorf("terminal size: %w", err)
	}
	line, err := graphicalProgress(progress)
	if err != nil {
		return err
	}
	// Leave the final column unused to prevent wrapping into another line.
	if len(line) > max(0, width-1) {
		line = line[:max(0, width-1)]
	}
	ending := ""
	if progress.Done {
		ending = "\n"
	}
	_, err = fmt.Fprintf(display.writer, "\r\x1b[2K%s%s", line, ending)
	return err
}

func graphicalProgress(progress discovery.Progress) (string, error) {
	tcp, tcpOK := new(big.Int).SetString(progress.TCPPlanned, 10)
	host, hostOK := new(big.Int).SetString(progress.HostPlanned, 10)
	if !tcpOK || !hostOK || tcp.Sign() <= 0 || host.Sign() < 0 ||
		progress.TCPCompleted < 0 || progress.HostCompleted < 0 {
		return "", fmt.Errorf("invalid planned or completed progress counts")
	}
	total := new(big.Int).Add(tcp, host)
	completed := new(big.Int).Add(big.NewInt(int64(progress.TCPCompleted)), big.NewInt(int64(progress.HostCompleted)))
	if completed.Cmp(total) > 0 {
		return "", fmt.Errorf("completed progress exceeds planned checks")
	}
	// Integer arithmetic preserves large IPv6 scopes and never rounds partial
	// completion up to 100 percent.
	hundredths := new(big.Int).Quo(new(big.Int).Mul(completed, big.NewInt(10000)), total).Int64()
	filled := int(hundredths * 10 / 10000)
	label := "Scanning"
	if progress.Done {
		label = "Finished"
		if completed.Cmp(total) < 0 {
			label = "Stopped"
		}
	}
	return fmt.Sprintf("[%s%s] %d.%02d%% %s %d/%s %s | clients: %d  hosts: %d  services: %d  ",
		strings.Repeat("=", filled), strings.Repeat("-", 10-filled),
		hundredths/100, hundredths%100, label, progress.HostCompleted, progress.HostPlanned,
		progress.Elapsed.Round(time.Second),
		progress.DiscoveredClients, progress.DiscoveredHosts, progress.OpenEndpoints), nil
}
