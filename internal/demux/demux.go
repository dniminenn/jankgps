package demux

import (
	"encoding/binary"
	"io"

	"github.com/dnim/jankgps/internal/ubx"
)

// Handler receives demuxed messages from the serial stream.
type Handler interface {
	OnUBX(frame ubx.Frame)
	OnNMEA(sentence []byte) // includes leading '$' through trailing \r\n
}

// Demuxer reads a mixed UBX/NMEA stream and dispatches to a Handler.
type Demuxer struct {
	r   io.Reader
	h   Handler
	buf [4096]byte
	pos int
	end int
}

func New(r io.Reader, h Handler) *Demuxer {
	return &Demuxer{r: r, h: h}
}

func (d *Demuxer) fill() error {
	if d.pos > 0 {
		n := copy(d.buf[:], d.buf[d.pos:d.end])
		d.end = n
		d.pos = 0
	}
	if d.end >= len(d.buf) {
		// buffer full with no progress — discard
		d.pos = 0
		d.end = 0
	}
	n, err := d.r.Read(d.buf[d.end:])
	d.end += n
	return err
}

func (d *Demuxer) avail() int { return d.end - d.pos }

// Run reads until the reader returns an error (typically context cancellation or EOF).
func (d *Demuxer) Run() error {
	for {
		if d.avail() == 0 {
			if err := d.fill(); err != nil {
				return err
			}
			continue
		}

		b := d.buf[d.pos]
		switch {
		case b == ubx.SyncA:
			if err := d.readUBX(); err != nil {
				return err
			}
		case b == '$':
			if err := d.readNMEA(); err != nil {
				return err
			}
		default:
			d.pos++
		}
	}
}

func (d *Demuxer) readUBX() error {
	// Need at least header (6 bytes) to read length
	for d.avail() < ubx.HeaderLen {
		if err := d.fill(); err != nil {
			return err
		}
	}
	data := d.buf[d.pos:d.end]
	if data[1] != ubx.SyncB {
		d.pos++
		return nil
	}
	payloadLen := int(binary.LittleEndian.Uint16(data[4:6]))
	frameLen := ubx.HeaderLen + payloadLen + ubx.ChecksumLen

	for d.avail() < frameLen {
		if err := d.fill(); err != nil {
			return err
		}
	}
	raw := d.buf[d.pos : d.pos+frameLen]
	frame, err := ubx.Decode(raw)
	if err != nil {
		// Bad frame — skip sync byte and resync
		d.pos++
		return nil
	}
	d.h.OnUBX(frame)
	d.pos += frameLen
	return nil
}

func (d *Demuxer) readNMEA() error {
	// Scan for \n from current position
	for {
		start := d.pos
		for i := start; i < d.end; i++ {
			if d.buf[i] == '\n' {
				sentence := make([]byte, i+1-d.pos)
				copy(sentence, d.buf[d.pos:i+1])
				d.h.OnNMEA(sentence)
				d.pos = i + 1
				return nil
			}
		}
		// \n not found yet, need more data
		if err := d.fill(); err != nil {
			return err
		}
	}
}
