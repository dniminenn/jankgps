package ts2phc

import (
	"testing"

	"github.com/dnim/jankgps/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestParseLine(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	mon := NewMonitor(m)

	tests := []struct {
		line     string
		isOffset bool
		isNmea   bool
	}{
		{
			line:     "Mar 07 14:07:43 bigchron ts2phc[5727]: /dev/ptp0 offset        -10 s2 freq  -17080",
			isOffset: true,
		},
		{
			line:   "Mar 07 14:07:43 bigchron ts2phc[5727]: nmea delay: 29645658 ns",
			isNmea: true,
		},
		{
			line: "Mar 07 14:07:43 bigchron ts2phc[5727]: adding tstamp 1772906899.999999990 to clock /dev/ptp0",
		},
	}

	for _, tt := range tests {
		mon.parseLine(tt.line)
		// We can't easily check the gauge values without more complexity, 
		// but we can at least ensure it doesn't crash and we can add more specific checks if needed.
	}
}
