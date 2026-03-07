package ts2phc

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/dnim/jankgps/internal/metrics"
)

var (
	offsetRegex    = regexp.MustCompile(`([^ ]+) offset\s+(-?\d+)\s+s\d\s+freq\s+(-?\d+)`)
	nmeaDelayRegex = regexp.MustCompile(`nmea delay: (\d+) ns`)
)

type Monitor struct {
	metrics *metrics.Metrics
}

func NewMonitor(m *metrics.Metrics) *Monitor {
	return &Monitor{metrics: m}
}

func (m *Monitor) Run(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "journalctl", "-u", "ts2phc", "-f", "-n", "0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	log.Printf("ts2phc: started journal monitor")

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		m.parseLine(line)
	}

	return cmd.Wait()
}

func (m *Monitor) parseLine(line string) {
	if strings.Contains(line, "offset") {
		matches := offsetRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			clock := matches[1]
			offset, _ := strconv.ParseFloat(matches[2], 64)
			freq, _ := strconv.ParseFloat(matches[3], 64)
			m.metrics.UpdateTS2PHCOffset(clock, offset, freq)
		}
	} else if strings.Contains(line, "nmea delay") {
		matches := nmeaDelayRegex.FindStringSubmatch(line)
		if len(matches) == 2 {
			delay, _ := strconv.ParseFloat(matches[1], 64)
			m.metrics.UpdateTS2PHCNMEADelay("nmea", delay)
		}
	}
}
