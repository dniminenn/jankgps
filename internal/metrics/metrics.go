package metrics

import (
	"fmt"
	"sync"

	"github.com/dnim/jankgps/internal/ubx"
	"github.com/prometheus/client_golang/prometheus"
)

var gnssNames = map[uint8]string{
	0: "gps", 1: "sbas", 2: "galileo", 3: "beidou", 5: "qzss", 6: "glonass",
}

type Metrics struct {
	fixType   prometheus.Gauge
	numSV     prometheus.Gauge
	lat       prometheus.Gauge
	lon       prometheus.Gauge
	altMSL    prometheus.Gauge
	hAcc      prometheus.Gauge
	vAcc      prometheus.Gauge
	speed     prometheus.Gauge
	heading   prometheus.Gauge
	headAcc   prometheus.Gauge
	velN      prometheus.Gauge
	velE      prometheus.Gauge
	velD      prometheus.Gauge
	pdop      prometheus.Gauge
	hdop      prometheus.Gauge
	vdop      prometheus.Gauge
	tdop      prometheus.Gauge
	gdop      prometheus.Gauge
	timeAcc   prometheus.Gauge
	timeValid prometheus.Gauge
	clkBias   prometheus.Gauge
	clkDrift  prometheus.Gauge
	clkAcc    prometheus.Gauge
	freqAcc   prometheus.Gauge
	tpQErr    prometheus.Gauge
	satCno    *prometheus.GaugeVec
	satElev   *prometheus.GaugeVec
	satAzim   *prometheus.GaugeVec
	satUsed   *prometheus.GaugeVec
	ts2phcOffset    *prometheus.GaugeVec
	ts2phcFreq      *prometheus.GaugeVec
	ts2phcNmeaDelay *prometheus.GaugeVec

	mu      sync.Mutex
	satSeen map[string]struct{}
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		satSeen: make(map[string]struct{}),
	}
	ns := "gps"

	svLabels := []string{"gnss", "svid"}

	m.fixType   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "fix_type", Help: "GNSS fix type (0=none,2=2D,3=3D)"})
	m.numSV     = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "satellites_used", Help: "Number of SVs used in nav solution"})
	m.lat       = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "latitude_degrees"})
	m.lon       = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "longitude_degrees"})
	m.altMSL    = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "altitude_msl_meters"})
	m.hAcc      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "horizontal_accuracy_meters"})
	m.vAcc      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "vertical_accuracy_meters"})
	m.speed     = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "speed_mps", Help: "Ground speed in m/s"})
	m.heading   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "heading_degrees", Help: "Vehicle heading in degrees"})
	m.headAcc   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "heading_accuracy_degrees"})
	m.velN      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "velocity_north_mps"})
	m.velE      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "velocity_east_mps"})
	m.velD      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "velocity_down_mps"})
	m.pdop      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "pdop"})
	m.hdop      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "hdop"})
	m.vdop      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "vdop"})
	m.tdop      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "tdop"})
	m.gdop      = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "gdop"})
	m.timeAcc   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "time_accuracy_ns"})
	m.timeValid = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "time_valid", Help: "1 if UTC is valid"})
	m.clkBias   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_bias_ns"})
	m.clkDrift  = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_drift_nps", Help: "Clock drift in ns/s"})
	m.clkAcc    = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_accuracy_ns"})
	m.freqAcc   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "freq_accuracy_pps", Help: "Frequency accuracy in ps/s"})
	m.tpQErr    = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "timepulse_quantization_error_ps"})
	m.satCno    = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "satellite_cno_dbhz", Help: "C/N0 per satellite"}, svLabels)
	m.satElev   = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "satellite_elevation_degrees"}, svLabels)
	m.satAzim   = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "satellite_azimuth_degrees"}, svLabels)
	m.satUsed   = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "satellite_used", Help: "1 if SV used in nav solution"}, svLabels)

	ts2phcLabels := []string{"clock"}
	m.ts2phcOffset = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "ts2phc", Name: "offset_ns", Help: "PTP clock offset in nanoseconds"}, ts2phcLabels)
	m.ts2phcFreq = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "ts2phc", Name: "freq_ppb", Help: "PTP clock frequency adjustment in ppb"}, ts2phcLabels)
	m.ts2phcNmeaDelay = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "ts2phc", Name: "nmea_delay_ns", Help: "NMEA delay reported by ts2phc"}, ts2phcLabels)

	reg.MustRegister(
		m.fixType, m.numSV, m.lat, m.lon, m.altMSL, m.hAcc, m.vAcc,
		m.speed, m.heading, m.headAcc, m.velN, m.velE, m.velD,
		m.pdop, m.hdop, m.vdop, m.tdop, m.gdop,
		m.timeAcc, m.timeValid, m.clkBias, m.clkDrift, m.clkAcc, m.freqAcc,
		m.tpQErr, m.satCno, m.satElev, m.satAzim, m.satUsed,
		m.ts2phcOffset, m.ts2phcFreq, m.ts2phcNmeaDelay,
	)

	return m
}

func (m *Metrics) UpdateNavPVT(p *ubx.NavPVT) {
	m.fixType.Set(float64(p.FixType))
	m.numSV.Set(float64(p.NumSV))
	m.lat.Set(p.LatDeg())
	m.lon.Set(p.LonDeg())
	m.altMSL.Set(p.HMSLMeters())
	m.hAcc.Set(p.HAccMeters())
	m.vAcc.Set(p.VAccMeters())
	m.speed.Set(p.GSpeedMPS())
	m.pdop.Set(p.PDOPVal())
	m.heading.Set(float64(p.HeadMot) * 1e-5)
	m.headAcc.Set(float64(p.HeadAcc) * 1e-5)
	m.velN.Set(float64(p.VelN) / 1000.0)
	m.velE.Set(float64(p.VelE) / 1000.0)
	m.velD.Set(float64(p.VelD) / 1000.0)
}

func (m *Metrics) UpdateNavDOP(d *ubx.NavDOP) {
	m.hdop.Set(d.HDOPVal())
	m.vdop.Set(d.VDOPVal())
	m.pdop.Set(d.PDOPVal())
	m.tdop.Set(float64(d.TDOP) * 0.01)
	m.gdop.Set(float64(d.GDOP) * 0.01)
}

func (m *Metrics) UpdateNavTimeUTC(t *ubx.NavTimeUTC) {
	m.timeAcc.Set(float64(t.TAcc))
	v := float64(0)
	if t.ValidUTC() {
		v = 1
	}
	m.timeValid.Set(v)
}

func (m *Metrics) UpdateNavClock(c *ubx.NavClock) {
	m.clkBias.Set(float64(c.ClkB))
	m.clkDrift.Set(float64(c.ClkD))
	m.clkAcc.Set(float64(c.TAcc))
	m.freqAcc.Set(float64(c.FAcc))
}

func (m *Metrics) UpdateTimTP(tp *ubx.TimTP) {
	m.tpQErr.Set(float64(tp.QErr))
}

func (m *Metrics) UpdateNavSAT(sat *ubx.NavSAT) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newSeen := make(map[string]struct{}, len(sat.Svs))
	for _, sv := range sat.Svs {
		gnss := gnssNames[sv.GnssID]
		if gnss == "" {
			gnss = fmt.Sprintf("%d", sv.GnssID)
		}
		svid := fmt.Sprintf("%d", sv.SvID)
		key := gnss + "/" + svid
		newSeen[key] = struct{}{}
		m.satCno.WithLabelValues(gnss, svid).Set(float64(sv.Cno))
		m.satElev.WithLabelValues(gnss, svid).Set(float64(sv.Elev))
		m.satAzim.WithLabelValues(gnss, svid).Set(float64(sv.Azim))
		used := 0.0
		if sv.SvUsed() {
			used = 1.0
		}
		m.satUsed.WithLabelValues(gnss, svid).Set(used)
	}
	for key := range m.satSeen {
		if _, ok := newSeen[key]; !ok {
			var gnss, svid string
			fmt.Sscanf(key, "%[^/]/%s", &gnss, &svid)
			m.satCno.DeleteLabelValues(gnss, svid)
			m.satElev.DeleteLabelValues(gnss, svid)
			m.satAzim.DeleteLabelValues(gnss, svid)
			m.satUsed.DeleteLabelValues(gnss, svid)
		}
	}
	m.satSeen = newSeen
}

func (m *Metrics) UpdateTS2PHCOffset(clock string, offset float64, freq float64) {
	m.ts2phcOffset.WithLabelValues(clock).Set(offset)
	m.ts2phcFreq.WithLabelValues(clock).Set(freq)
}

func (m *Metrics) UpdateTS2PHCNMEADelay(clock string, delay float64) {
	// ts2phc log doesn't always specify clock for NMEA delay, but it's usually global or first clock
	// for now we'll use "nmea" as a generic label or just empty if we can't determine it easily
	m.ts2phcNmeaDelay.WithLabelValues("nmea").Set(delay)
}
