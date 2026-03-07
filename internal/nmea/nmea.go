package nmea

import (
	"fmt"

	"github.com/dnim/jankgps/internal/ubx"
)

func timeStr(pvt *ubx.NavPVT) string {
	return fmt.Sprintf("%02d%02d%02d.00", pvt.Hour, pvt.Min, pvt.Sec)
}

func RMCFromPVT(pvt *ubx.NavPVT) []byte {
	if pvt.FixType == 0 {
		return nil
	}
	ts := timeStr(pvt)
	status := "V"
	if pvt.GnssFixOK() {
		status = "A"
	}
	latStr, latNS := degToNMEALat(pvt.LatDeg())
	lonStr, lonEW := degToNMEALon(pvt.LonDeg())
	speedKnots := pvt.GSpeedMPS() * 1.94384
	course := float64(pvt.HeadMot) * 1e-5
	if course < 0 {
		course += 360
	}
	dateStr := fmt.Sprintf("%02d%02d%02d", pvt.Day, pvt.Month, pvt.Year%100)
	s := fmt.Sprintf("$GPRMC,%s,%s,%s,%s,%s,%s,%.1f,%.1f,%s,,*",
		ts, status, latStr, latNS, lonStr, lonEW, speedKnots, course, dateStr)
	return append(append([]byte(s), checksum(s)...), '\r', '\n')
}

func GGAFromPVT(pvt *ubx.NavPVT, dop *ubx.NavDOP) []byte {
	if pvt.FixType == 0 {
		return nil
	}
	latStr, latNS := degToNMEALat(pvt.LatDeg())
	lonStr, lonEW := degToNMEALon(pvt.LonDeg())
	quality := byte('0')
	if pvt.FixType >= 2 {
		quality = '1'
	}
	hdop := pvt.PDOPVal()
	if dop != nil {
		hdop = dop.HDOPVal()
	}
	if hdop > 99.9 {
		hdop = 99.9
	}
	s := fmt.Sprintf("$GPGGA,%s,%s,%s,%s,%s,%c,%02d,%.1f,%.1f,M,,M,,*",
		timeStr(pvt), latStr, latNS, lonStr, lonEW, quality, pvt.NumSV, hdop, pvt.HMSLMeters())
	return append(append([]byte(s), checksum(s)...), '\r', '\n')
}

func ZDAFromPVT(pvt *ubx.NavPVT) []byte {
	s := fmt.Sprintf("$GPZDA,%s,%02d,%02d,%04d,00,00*",
		timeStr(pvt), pvt.Day, pvt.Month, pvt.Year)
	return append(append([]byte(s), checksum(s)...), '\r', '\n')
}

func VTGFromPVT(pvt *ubx.NavPVT) []byte {
	if pvt.FixType == 0 {
		return nil
	}
	course := float64(pvt.HeadMot) * 1e-5
	if course < 0 {
		course += 360
	}
	speedKnots := pvt.GSpeedMPS() * 1.94384
	speedKmh := pvt.GSpeedMPS() * 3.6
	s := fmt.Sprintf("$GPVTG,%.1f,T,,M,%.1f,N,%.1f,K,A*", course, speedKnots, speedKmh)
	return append(append([]byte(s), checksum(s)...), '\r', '\n')
}

// gsaTalker maps gnssID to talker prefix and NMEA system ID (NMEA 4.10+).
var gsaTalker = [...]struct{ talker, sysID string }{
	0: {"GP", "1"}, // GPS
	1: {"GP", "1"}, // SBAS → fold into GPS
	2: {"GA", "3"}, // Galileo
	3: {"GB", "4"}, // BeiDou
	6: {"GL", "2"}, // GLONASS
}

func GSAFromPVT(pvt *ubx.NavPVT, sat *ubx.NavSAT, dop *ubx.NavDOP) [][]byte {
	mode := "A"
	fix := "1"
	if pvt.FixType == 2 {
		fix = "2"
	} else if pvt.FixType >= 3 {
		fix = "3"
	}

	pdop, hdop, vdop := pvt.PDOPVal(), pvt.PDOPVal(), pvt.PDOPVal()
	if dop != nil {
		pdop = dop.PDOPVal()
		hdop = dop.HDOPVal()
		vdop = dop.VDOPVal()
	}
	clamp := func(v float64) float64 {
		if v > 99.9 {
			return 99.9
		}
		return v
	}
	pdop, hdop, vdop = clamp(pdop), clamp(hdop), clamp(vdop)

	// Collect used SVs per constellation.
	type consSVs struct {
		talker string
		sysID  string
		svids  []int
	}
	consMap := map[string]*consSVs{}
	order := []string{"GP", "GL", "GA", "GB"}
	sysIDs := map[string]string{"GP": "1", "GL": "2", "GA": "3", "GB": "4"}
	for _, t := range order {
		consMap[t] = &consSVs{talker: t, sysID: sysIDs[t]}
	}

	if sat != nil {
		for _, sv := range sat.Svs {
			if !sv.SvUsed() {
				continue
			}
			var tk string
			if int(sv.GnssID) < len(gsaTalker) {
				tk = gsaTalker[sv.GnssID].talker
			}
			if tk == "" {
				tk = "GP"
			}
			if c, ok := consMap[tk]; ok {
				c.svids = append(c.svids, int(sv.SvID))
			}
		}
	}

	var out [][]byte
	for _, tk := range order {
		c := consMap[tk]
		if sat != nil && len(c.svids) == 0 {
			continue // skip constellations with no used SVs when we have SAT data
		}
		svFields := ""
		for i := 0; i < 12; i++ {
			if i < len(c.svids) {
				svFields += fmt.Sprintf(",%02d", c.svids[i])
			} else {
				svFields += ","
			}
		}
		s := fmt.Sprintf("$%sGSA,%s,%s%s,%.1f,%.1f,%.1f,%s*",
			c.talker, mode, fix, svFields, pdop, hdop, vdop, c.sysID)
		out = append(out, append(append([]byte(s), checksum(s)...), '\r', '\n'))
	}
	return out
}

func degToNMEALat(deg float64) (string, string) {
	ns := "N"
	if deg < 0 {
		deg = -deg
		ns = "S"
	}
	d := int(deg)
	m := (deg - float64(d)) * 60
	return fmt.Sprintf("%02d%07.4f", d, m), ns
}

func degToNMEALon(deg float64) (string, string) {
	ew := "E"
	if deg < 0 {
		deg = -deg
		ew = "W"
	}
	d := int(deg)
	m := (deg - float64(d)) * 60
	return fmt.Sprintf("%03d%07.4f", d, m), ew
}

func checksum(sentence string) []byte {
	var xor byte
	for i := 1; i < len(sentence)-1; i++ { // skip leading '$' and trailing '*'
		xor ^= sentence[i]
	}
	return []byte(fmt.Sprintf("%02X", xor))
}

var gnssTalker = map[uint8]string{
	0: "GP",
	1: "GP",
	2: "GA",
	3: "GB",
	6: "GL",
}

func GSVFromSAT(sat *ubx.NavSAT) [][]byte {
	if len(sat.Svs) == 0 {
		return nil
	}
	byTalker := make(map[string][]ubx.SatInfo)
	for _, s := range sat.Svs {
		t := gnssTalker[s.GnssID]
		if t == "" {
			t = "GP"
		}
		byTalker[t] = append(byTalker[t], s)
	}
	var out [][]byte
	for _, talker := range []string{"GP", "GL", "GA", "GB"} {
		svs := byTalker[talker]
		if len(svs) == 0 {
			continue
		}
		nMsg := (len(svs) + 3) / 4
		for i := 0; i < nMsg; i++ {
			start := i * 4
			end := start + 4
			if end > len(svs) {
				end = len(svs)
			}
			chunk := svs[start:end]
			s := fmt.Sprintf("$%sGSV,%d,%d,%02d", talker, nMsg, i+1, len(svs))
			for _, svi := range chunk {
				s += fmt.Sprintf(",%02d,%02d,%03d,%02d", svi.SvID, svi.Elev, (svi.Azim+360)%360, svi.Cno)
			}
			s += "*"
			out = append(out, append(append([]byte(s), checksum(s)...), '\r', '\n'))
		}
	}
	return out
}
