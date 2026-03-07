package export

import (
	"fmt"
	"log"
	"os"

	"github.com/creack/pty"
)

// PTYExport creates a PTY pair for ts2phc.
// ts2phc reads the slave side; we write NMEA sentences to the master.
type PTYExport struct {
	master  *os.File
	slave   *os.File
	link    string
}

// NewPTY creates a PTY pair and optionally symlinks the slave to linkPath.
// If linkPath is empty, no symlink is created and the caller must use SlavePath().
func NewPTY(linkPath string) (*PTYExport, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("pty open: %w", err)
	}
	p := &PTYExport{master: master, slave: slave, link: linkPath}
	slaveName := slave.Name()
	log.Printf("pty: slave=%s", slaveName)

	if linkPath != "" {
		os.Remove(linkPath) // best-effort remove stale
		if err := os.Symlink(slaveName, linkPath); err != nil {
			p.Close()
			return nil, fmt.Errorf("pty symlink %s -> %s: %w", linkPath, slaveName, err)
		}
		log.Printf("pty: symlink %s -> %s", linkPath, slaveName)
	}
	return p, nil
}

func (p *PTYExport) SlavePath() string {
	return p.slave.Name()
}

// Write sends raw bytes (an NMEA sentence) to the master side of the PTY.
func (p *PTYExport) Write(data []byte) error {
	_, err := p.master.Write(data)
	return err
}

func (p *PTYExport) Close() error {
	if p.link != "" {
		os.Remove(p.link)
	}
	p.slave.Close()
	return p.master.Close()
}
