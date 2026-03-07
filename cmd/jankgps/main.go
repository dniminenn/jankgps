package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dnim/jankgps/internal/demux"
	"github.com/dnim/jankgps/internal/export"
	"github.com/dnim/jankgps/internal/metrics"
	"github.com/dnim/jankgps/internal/ubx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.bug.st/serial"
)

func main() {
	dev := flag.String("dev", "/dev/ttyACM0", "serial device path")
	baud := flag.Int("baud", 115200, "serial baud rate")
	metricsAddr := flag.String("metrics-addr", ":9100", "prometheus metrics listen address")
	tcpAddr := flag.String("tcp-addr", ":2947", "TCP NMEA export listen address for gpsd")
	ptyLink := flag.String("pty-link", "/run/jankgps/ts2phc", "symlink path for ts2phc PTY slave")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// Open serial port
	port, err := serial.Open(*dev, &serial.Mode{BaudRate: *baud})
	if err != nil {
		log.Fatalf("serial open %s: %v", *dev, err)
	}
	defer port.Close()
	port.SetReadTimeout(2 * time.Second)
	log.Printf("serial: opened %s @ %d baud", *dev, *baud)

	// Configure M9N via VALSET
	if err := configureModule(port); err != nil {
		log.Fatalf("configure: %v", err)
	}

	// Poll MON-VER for firmware info
	if _, err := port.Write(ubx.EncodePoll(ubx.ClassMON, ubx.IDMonVer)); err != nil {
		log.Printf("warn: failed to poll MON-VER: %v", err)
	}

	// Set up exports
	ptyExport, err := export.NewPTY(*ptyLink)
	if err != nil {
		log.Fatalf("pty: %v", err)
	}
	defer ptyExport.Close()

	tcpExport, err := export.NewTCP(*tcpAddr)
	if err != nil {
		log.Fatalf("tcp: %v", err)
	}
	defer tcpExport.Close()

	// Prometheus
	reg := prometheus.NewRegistry()
	met := metrics.New(reg)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	go func() {
		log.Printf("metrics: listening on %s", *metricsAddr)
		if err := http.ListenAndServe(*metricsAddr, nil); err != nil {
			log.Fatalf("metrics http: %v", err)
		}
	}()

	// Wire up handler
	h := &handler{
		pty:     ptyExport,
		tcp:     tcpExport,
		metrics: met,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down")
		port.Close()
	}()

	// Run demuxer (blocks until port closes or error)
	d := demux.New(port, h)
	if err := d.Run(); err != nil {
		log.Printf("demux: %v", err)
	}
}

func configureModule(port serial.Port) error {
	frame := ubx.EncodeValset(ubx.LayerRAM|ubx.LayerBBR,
		// Enable UBX messages on USB at rate 1
		ubx.CfgU1(ubx.CfgMsgoutUbxNavPvtUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavTimeUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavClkUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavSatUSB, 5), // every 5th solution to reduce volume
		ubx.CfgU1(ubx.CfgMsgoutUbxTimTpUSB, 1),
		// Enable RMC on USB, disable chatty NMEA
		ubx.CfgU1(ubx.CfgMsgoutNmeaRmcUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutNmeaZdaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGgaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGllUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGsaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGsvUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaVtgUSB, 0),
		// Ensure UBX+NMEA output enabled on USB
		ubx.CfgL(ubx.CfgUSBOutprotUBX, true),
		ubx.CfgL(ubx.CfgUSBOutprotNMEA, true),
		ubx.CfgL(ubx.CfgUSBInprotUBX, true),
	)

	if _, err := port.Write(frame); err != nil {
		return fmt.Errorf("write VALSET: %w", err)
	}
	log.Println("config: sent VALSET (RAM+BBR)")

	// Wait briefly for ACK; not fatal if we don't see it immediately —
	// the demux loop will handle ACKs too.
	time.Sleep(200 * time.Millisecond)
	buf := make([]byte, 256)
	n, _ := port.Read(buf)
	if n > 0 {
		if bytes.Contains(buf[:n], []byte{ubx.SyncA, ubx.SyncB, ubx.ClassACK, ubx.IDAckAck}) {
			log.Println("config: ACK received")
		} else if bytes.Contains(buf[:n], []byte{ubx.SyncA, ubx.SyncB, ubx.ClassACK, ubx.IDAckNak}) {
			log.Println("config: NAK received — check key IDs")
		}
	}
	return nil
}

type handler struct {
	pty     *export.PTYExport
	tcp     *export.TCPExport
	metrics *metrics.Metrics
}

func (h *handler) OnUBX(frame ubx.Frame) {
	switch frame.ClassID() {
	case ubx.MsgNavPVT:
		pvt, err := ubx.ParseNavPVT(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-PVT: %v", err)
			return
		}
		h.metrics.UpdateNavPVT(pvt)

	case ubx.MsgNavTimeUTC:
		t, err := ubx.ParseNavTimeUTC(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-TIMEUTC: %v", err)
			return
		}
		h.metrics.UpdateNavTimeUTC(t)

	case ubx.MsgNavClock:
		c, err := ubx.ParseNavClock(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-CLOCK: %v", err)
			return
		}
		h.metrics.UpdateNavClock(c)

	case ubx.MsgNavSAT:
		sat, err := ubx.ParseNavSAT(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-SAT: %v", err)
			return
		}
		h.metrics.UpdateNavSAT(sat)

	case ubx.MsgTimTP:
		tp, err := ubx.ParseTimTP(frame.Payload)
		if err != nil {
			log.Printf("parse TIM-TP: %v", err)
			return
		}
		h.metrics.UpdateTimTP(tp)

	case ubx.MsgMonVer:
		ver, err := ubx.ParseMonVer(frame.Payload)
		if err != nil {
			log.Printf("parse MON-VER: %v", err)
			return
		}
		log.Printf("firmware: sw=%s hw=%s", ver.SwVersion, ver.HwVersion)
		for _, ext := range ver.Extensions {
			log.Printf("firmware: ext=%s", ext)
		}

	case ubx.MsgAckAck:
		ack, _ := ubx.ParseAck(frame.Payload)
		log.Printf("ACK-ACK cls=0x%02x msg=0x%02x", ack.ClsID, ack.MsgID)

	case ubx.MsgAckNak:
		ack, _ := ubx.ParseAck(frame.Payload)
		log.Printf("ACK-NAK cls=0x%02x msg=0x%02x", ack.ClsID, ack.MsgID)
	}
}

func (h *handler) OnNMEA(sentence []byte) {
	h.tcp.Broadcast(sentence)

	if isRMC(sentence) {
		if err := h.pty.Write(sentence); err != nil {
			log.Printf("pty write: %v", err)
		}
	}
}

func isRMC(sentence []byte) bool {
	if len(sentence) < 6 {
		return false
	}
	return sentence[3] == 'R' && sentence[4] == 'M' && sentence[5] == 'C'
}
