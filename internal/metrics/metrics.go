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
	fixType  prometheus.Gauge
	numSV    prometheus.Gauge
	lat      prometheus.Gauge
	lon      prometheus.Gauge
	altMSL   prometheus.Gauge
	hAcc     prometheus.Gauge
	vAcc     prometheus.Gauge
	speed    prometheus.Gauge
	pdop     prometheus.Gauge
	timeAcc  prometheus.Gauge
	timeValid prometheus.Gauge
	clkBias  prometheus.Gauge
	clkDrift prometheus.Gauge
	clkAcc   prometheus.Gauge
	freqAcc  prometheus.Gauge
	tpQErr   prometheus.Gauge
	satCno   *prometheus.GaugeVec

	// track which sat labels we've seen so we can reset stale ones
	mu       sync.Mutex
	satSeen  map[string]struct{}
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		satSeen: make(map[string]struct{}),
	}
	ns := "gps"

	m.fixType = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "fix_type", Help: "GNSS fix type (0=none,2=2D,3=3D)"})
	m.numSV = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "satellites_used", Help: "Number of SVs used in nav solution"})
	m.lat = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "latitude_degrees"})
	m.lon = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "longitude_degrees"})
	m.altMSL = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "altitude_msl_meters"})
	m.hAcc = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "horizontal_accuracy_meters"})
	m.vAcc = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "vertical_accuracy_meters"})
	m.speed = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "speed_mps", Help: "Ground speed in m/s"})
	m.pdop = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "pdop"})
	m.timeAcc = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "time_accuracy_ns"})
	m.timeValid = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "time_valid", Help: "1 if UTC is valid"})
	m.clkBias = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_bias_ns"})
	m.clkDrift = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_drift_nps", Help: "Clock drift in ns/s"})
	m.clkAcc = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "clock_accuracy_ns"})
	m.freqAcc = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "freq_accuracy_pps", Help: "Frequency accuracy in ps/s"})
	m.tpQErr = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: "timepulse_quantization_error_ps"})
	m.satCno = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: "satellite_cno_dbhz", Help: "C/N0 per satellite"}, []string{"gnss", "svid"})

	reg.MustRegister(m.fixType, m.numSV, m.lat, m.lon, m.altMSL, m.hAcc, m.vAcc,
		m.speed, m.pdop, m.timeAcc, m.timeValid, m.clkBias, m.clkDrift,
		m.clkAcc, m.freqAcc, m.tpQErr, m.satCno)

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
	}
	// Remove satellites that disappeared
	for key := range m.satSeen {
		if _, ok := newSeen[key]; !ok {
			var gnss, svid string
			fmt.Sscanf(key, "%[^/]/%s", &gnss, &svid)
			m.satCno.DeleteLabelValues(gnss, svid)
		}
	}
	m.satSeen = newSeen
}
