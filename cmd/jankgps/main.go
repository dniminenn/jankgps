package main

import (
	"bytes"
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
	"github.com/dnim/jankgps/internal/nmea"
	"github.com/dnim/jankgps/internal/ts2phc"
	"github.com/dnim/jankgps/internal/ubx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.bug.st/serial"
)

var (
	cfgFile string
	root   = &cobra.Command{
		Use:   "jankgps",
		Short: "GPS daemon with NMEA/UBX demux, PTY for ts2phc, TCP for gpsd",
		RunE:  run,
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $HOME/.jankgps.yaml)")
	root.PersistentFlags().String("dev", "/dev/ttyACM0", "serial device path")
	root.PersistentFlags().Int("baud", 115200, "serial baud rate")
	root.PersistentFlags().String("metrics-addr", ":9100", "prometheus metrics listen address")
	root.PersistentFlags().String("tcp-addr", ":2948", "TCP NMEA export listen address for gpsd")
	root.PersistentFlags().String("pty-link", "/run/jankgps/ts2phc", "symlink path for ts2phc PTY slave")
	root.PersistentFlags().Int("ant-cable-delay-ns", 38, "antenna cable delay in ns for PPS")
	root.PersistentFlags().Bool("pty", true, "enable PTY export for ts2phc")
	root.PersistentFlags().Bool("tcp", true, "enable TCP NMEA export for gpsd")
	root.PersistentFlags().Bool("metrics", true, "enable Prometheus metrics server")
	root.PersistentFlags().Bool("monitor-ts2phc", true, "monitor ts2phc journal logs for metrics")
	_ = viper.BindPFlag("dev", root.PersistentFlags().Lookup("dev"))
	_ = viper.BindPFlag("baud", root.PersistentFlags().Lookup("baud"))
	_ = viper.BindPFlag("metrics_addr", root.PersistentFlags().Lookup("metrics-addr"))
	_ = viper.BindPFlag("tcp_addr", root.PersistentFlags().Lookup("tcp-addr"))
	_ = viper.BindPFlag("pty_link", root.PersistentFlags().Lookup("pty-link"))
	_ = viper.BindPFlag("ant_cable_delay_ns", root.PersistentFlags().Lookup("ant-cable-delay-ns"))
	_ = viper.BindPFlag("pty", root.PersistentFlags().Lookup("pty"))
	_ = viper.BindPFlag("tcp", root.PersistentFlags().Lookup("tcp"))
	_ = viper.BindPFlag("metrics", root.PersistentFlags().Lookup("metrics"))
	_ = viper.BindPFlag("monitor_ts2phc", root.PersistentFlags().Lookup("monitor-ts2phc"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".jankgps")
	}
	viper.SetEnvPrefix("JANKGPS")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	dev := viper.GetString("dev")
	baud := viper.GetInt("baud")
	metricsAddr := viper.GetString("metrics_addr")
	tcpAddr := viper.GetString("tcp_addr")
	ptyLink := viper.GetString("pty_link")
	antCableDelayNs := viper.GetInt("ant_cable_delay_ns")
	enablePTY := viper.GetBool("pty")
	enableTCP := viper.GetBool("tcp")
	enableMetrics := viper.GetBool("metrics")
	monitorTS2PHC := viper.GetBool("monitor_ts2phc")

	port, err := serial.Open(dev, &serial.Mode{BaudRate: baud})
	if err != nil {
		return fmt.Errorf("serial open %s: %w", dev, err)
	}
	defer port.Close()
	port.SetReadTimeout(2 * time.Second)
	log.Printf("serial: opened %s @ %d baud", dev, baud)

	if err := configureModule(port, antCableDelayNs); err != nil {
		return fmt.Errorf("configure: %w", err)
	}

	if _, err := port.Write(ubx.EncodePoll(ubx.ClassMON, ubx.IDMonVer)); err != nil {
		log.Printf("warn: failed to poll MON-VER: %v", err)
	}

	var ptyExport *export.PTYExport
	if enablePTY {
		ptyExport, err = export.NewPTY(ptyLink)
		if err != nil {
			return fmt.Errorf("pty: %w", err)
		}
		defer ptyExport.Close()
	}

	var tcpExport *export.TCPExport
	if enableTCP {
		tcpExport, err = export.NewTCP(tcpAddr)
		if err != nil {
			return fmt.Errorf("tcp: %w", err)
		}
		defer tcpExport.Close()
	}

	var met *metrics.Metrics
	if enableMetrics {
		reg := prometheus.NewRegistry()
		met = metrics.New(reg)
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
		go func() {
			log.Printf("metrics: listening on %s", metricsAddr)
			if err := http.ListenAndServe(metricsAddr, nil); err != nil {
				log.Printf("metrics http: %v", err)
			}
		}()

		if monitorTS2PHC {
			mon := ts2phc.NewMonitor(met)
			go func() {
				if err := mon.Run(cmd.Context()); err != nil {
					log.Printf("ts2phc monitor: %v", err)
				}
			}()
		}
	}

	h := &handler{
		pty:     ptyExport,
		tcp:     tcpExport,
		metrics: met,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down")
		port.Close()
	}()

	d := demux.New(port, h)
	return d.Run()
}

func configureModule(port serial.Port, antCableDelayNs int) error {
	frame := ubx.EncodeValset(ubx.LayerRAM,
		ubx.CfgU1(ubx.CfgNavspgDynModel, 2),
		ubx.CfgI2(ubx.CfgTpAntCableDelay, int16(antCableDelayNs)),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavPvtUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavDopUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavTimeUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavClkUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutUbxNavSatUSB, 5),
		ubx.CfgU1(ubx.CfgMsgoutUbxTimTpUSB, 1),
		ubx.CfgU1(ubx.CfgMsgoutNmeaRmcUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaZdaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGgaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGllUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGsaUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaGsvUSB, 0),
		ubx.CfgU1(ubx.CfgMsgoutNmeaVtgUSB, 0),
		ubx.CfgL(ubx.CfgUSBOutprotUBX, true),
		ubx.CfgL(ubx.CfgUSBOutprotNMEA, false),
		ubx.CfgL(ubx.CfgUSBInprotUBX, true),
	)

	if _, err := port.Write(frame); err != nil {
		return fmt.Errorf("write VALSET: %w", err)
	}
	log.Println("config: sent VALSET (RAM)")

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
	lastDOP *ubx.NavDOP
	lastSAT *ubx.NavSAT
}

func (h *handler) OnUBX(frame ubx.Frame) {
	switch frame.ClassID() {
	case ubx.MsgNavPVT:
		pvt, err := ubx.ParseNavPVT(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-PVT: %v", err)
			return
		}
		if h.metrics != nil {
			h.metrics.UpdateNavPVT(pvt)
		}
		sendNMEA(h, pvt, h.lastSAT, h.lastDOP)

	case ubx.MsgNavTimeUTC:
		t, err := ubx.ParseNavTimeUTC(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-TIMEUTC: %v", err)
			return
		}
		if h.metrics != nil {
			h.metrics.UpdateNavTimeUTC(t)
		}

	case ubx.MsgNavClock:
		c, err := ubx.ParseNavClock(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-CLOCK: %v", err)
			return
		}
		if h.metrics != nil {
			h.metrics.UpdateNavClock(c)
		}

	case ubx.MsgNavDOP:
		dop, err := ubx.ParseNavDOP(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-DOP: %v", err)
			return
		}
		h.lastDOP = dop
		if h.metrics != nil {
			h.metrics.UpdateNavDOP(dop)
		}

	case ubx.MsgNavSAT:
		sat, err := ubx.ParseNavSAT(frame.Payload)
		if err != nil {
			log.Printf("parse NAV-SAT: %v", err)
			return
		}
		if h.metrics != nil {
			h.metrics.UpdateNavSAT(sat)
		}
		h.lastSAT = sat
		sendNMEA(h, nil, sat, nil)

	case ubx.MsgTimTP:
		tp, err := ubx.ParseTimTP(frame.Payload)
		if err != nil {
			log.Printf("parse TIM-TP: %v", err)
			return
		}
		if h.metrics != nil {
			h.metrics.UpdateTimTP(tp)
		}

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

func sendNMEA(h *handler, pvt *ubx.NavPVT, sat *ubx.NavSAT, dop *ubx.NavDOP) {
	if h.tcp == nil && h.pty == nil {
		return
	}
	broadcast := func(b []byte) {
		if h.tcp != nil && len(b) > 0 {
			h.tcp.Broadcast(b)
		}
	}
	ptyWrite := func(b []byte) {
		if h.pty != nil && len(b) > 0 {
			if err := h.pty.Write(b); err != nil {
				log.Printf("pty write: %v", err)
			}
		}
	}
	if pvt != nil {
		if b := nmea.GGAFromPVT(pvt, dop); len(b) > 0 {
			broadcast(b)
		}
		if b := nmea.RMCFromPVT(pvt); len(b) > 0 {
			broadcast(b)
			ptyWrite(b)
		}
		for _, b := range nmea.GSAFromPVT(pvt, sat, dop) {
			broadcast(b)
		}
		if b := nmea.ZDAFromPVT(pvt); len(b) > 0 {
			broadcast(b)
		}
		if b := nmea.VTGFromPVT(pvt); len(b) > 0 {
			broadcast(b)
		}
	}
	if sat != nil {
		for _, b := range nmea.GSVFromSAT(sat) {
			broadcast(b)
		}
	}
}

func (h *handler) OnNMEA(sentence []byte) {
	if h.tcp != nil {
		h.tcp.Broadcast(sentence)
	}
	if h.pty != nil && isRMC(sentence) {
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
