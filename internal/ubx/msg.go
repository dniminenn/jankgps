package ubx

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Message class/ID constants.
const (
	ClassNAV = 0x01
	ClassACK = 0x05
	ClassCFG = 0x06
	ClassMON = 0x0a
	ClassTIM = 0x0d

	IDNavPVT     = 0x07
	IDNavDOP     = 0x04
	IDNavTimeUTC = 0x21
	IDNavClock   = 0x22
	IDNavSAT     = 0x35

	IDAckAck = 0x01
	IDAckNak = 0x00

	IDCfgValset = 0x8a
	IDCfgValget = 0x8b

	IDMonVer = 0x04

	IDTimTP = 0x01
)

// Combined class<<8|id for switch dispatch.
const (
	MsgNavPVT     = uint16(ClassNAV)<<8 | IDNavPVT
	MsgNavDOP     = uint16(ClassNAV)<<8 | IDNavDOP
	MsgNavTimeUTC = uint16(ClassNAV)<<8 | IDNavTimeUTC
	MsgNavClock   = uint16(ClassNAV)<<8 | IDNavClock
	MsgNavSAT     = uint16(ClassNAV)<<8 | IDNavSAT
	MsgAckAck     = uint16(ClassACK)<<8 | IDAckAck
	MsgAckNak     = uint16(ClassACK)<<8 | IDAckNak
	MsgCfgValget  = uint16(ClassCFG)<<8 | IDCfgValget
	MsgMonVer     = uint16(ClassMON)<<8 | IDMonVer
	MsgTimTP      = uint16(ClassTIM)<<8 | IDTimTP
)

// NavPVT is UBX-NAV-PVT (0x01 0x07), 92 bytes.
type NavPVT struct {
	ITOW   uint32 // ms
	Year   uint16
	Month  uint8
	Day    uint8
	Hour   uint8
	Min    uint8
	Sec    uint8
	Valid  uint8
	TAcc   uint32 // ns
	Nano   int32  // ns
	FixType uint8
	Flags  uint8
	Flags2 uint8
	NumSV  uint8
	Lon    int32 // 1e-7 deg
	Lat    int32 // 1e-7 deg
	Height int32 // mm
	HMSL   int32 // mm above MSL
	HAcc   uint32 // mm
	VAcc   uint32 // mm
	VelN   int32  // mm/s
	VelE   int32  // mm/s
	VelD   int32  // mm/s
	GSpeed int32  // mm/s
	HeadMot int32 // 1e-5 deg
	SAcc   uint32 // mm/s
	HeadAcc uint32 // 1e-5 deg
	PDOP   uint16 // 0.01
	Flags3 uint8
}

func (p *NavPVT) GnssFixOK() bool { return p.Flags&0x01 != 0 }
func (p *NavPVT) LonDeg() float64 { return float64(p.Lon) * 1e-7 }
func (p *NavPVT) LatDeg() float64 { return float64(p.Lat) * 1e-7 }
func (p *NavPVT) HMSLMeters() float64 { return float64(p.HMSL) / 1000.0 }
func (p *NavPVT) HAccMeters() float64 { return float64(p.HAcc) / 1000.0 }
func (p *NavPVT) VAccMeters() float64 { return float64(p.VAcc) / 1000.0 }
func (p *NavPVT) GSpeedMPS() float64 { return float64(p.GSpeed) / 1000.0 }
func (p *NavPVT) PDOPVal() float64   { return float64(p.PDOP) * 0.01 }

func ParseNavPVT(payload []byte) (*NavPVT, error) {
	if len(payload) < 92 {
		return nil, fmt.Errorf("ubx: NAV-PVT payload too short: %d", len(payload))
	}
	p := &NavPVT{
		ITOW:    binary.LittleEndian.Uint32(payload[0:4]),
		Year:    binary.LittleEndian.Uint16(payload[4:6]),
		Month:   payload[6],
		Day:     payload[7],
		Hour:    payload[8],
		Min:     payload[9],
		Sec:     payload[10],
		Valid:   payload[11],
		TAcc:    binary.LittleEndian.Uint32(payload[12:16]),
		Nano:    int32(binary.LittleEndian.Uint32(payload[16:20])),
		FixType: payload[20],
		Flags:   payload[21],
		Flags2:  payload[22],
		NumSV:   payload[23],
		Lon:     int32(binary.LittleEndian.Uint32(payload[24:28])),
		Lat:     int32(binary.LittleEndian.Uint32(payload[28:32])),
		Height:  int32(binary.LittleEndian.Uint32(payload[32:36])),
		HMSL:    int32(binary.LittleEndian.Uint32(payload[36:40])),
		HAcc:    binary.LittleEndian.Uint32(payload[40:44]),
		VAcc:    binary.LittleEndian.Uint32(payload[44:48]),
		VelN:    int32(binary.LittleEndian.Uint32(payload[48:52])),
		VelE:    int32(binary.LittleEndian.Uint32(payload[52:56])),
		VelD:    int32(binary.LittleEndian.Uint32(payload[56:60])),
		GSpeed:  int32(binary.LittleEndian.Uint32(payload[60:64])),
		HeadMot: int32(binary.LittleEndian.Uint32(payload[64:68])),
		SAcc:    binary.LittleEndian.Uint32(payload[68:72]),
		HeadAcc: binary.LittleEndian.Uint32(payload[72:76]),
		PDOP:    binary.LittleEndian.Uint16(payload[76:78]),
		Flags3:  payload[78],
	}
	return p, nil
}

// NavDOP is UBX-NAV-DOP (0x01 0x04), 18 bytes.
type NavDOP struct {
	ITOW uint32
	GDOP uint16 // 0.01
	PDOP uint16
	TDOP uint16
	VDOP uint16
	HDOP uint16
	NDOP uint16
	EDOP uint16
}

func (d *NavDOP) HDOPVal() float64 { return float64(d.HDOP) * 0.01 }
func (d *NavDOP) VDOPVal() float64 { return float64(d.VDOP) * 0.01 }
func (d *NavDOP) PDOPVal() float64 { return float64(d.PDOP) * 0.01 }

func ParseNavDOP(payload []byte) (*NavDOP, error) {
	if len(payload) < 18 {
		return nil, fmt.Errorf("ubx: NAV-DOP payload too short: %d", len(payload))
	}
	return &NavDOP{
		ITOW: binary.LittleEndian.Uint32(payload[0:4]),
		GDOP: binary.LittleEndian.Uint16(payload[4:6]),
		PDOP: binary.LittleEndian.Uint16(payload[6:8]),
		TDOP: binary.LittleEndian.Uint16(payload[8:10]),
		VDOP: binary.LittleEndian.Uint16(payload[10:12]),
		HDOP: binary.LittleEndian.Uint16(payload[12:14]),
		NDOP: binary.LittleEndian.Uint16(payload[14:16]),
		EDOP: binary.LittleEndian.Uint16(payload[16:18]),
	}, nil
}

// NavTimeUTC is UBX-NAV-TIMEUTC (0x01 0x21), 20 bytes.
type NavTimeUTC struct {
	ITOW  uint32 // ms
	TAcc  uint32 // ns
	Nano  int32  // ns
	Year  uint16
	Month uint8
	Day   uint8
	Hour  uint8
	Min   uint8
	Sec   uint8
	Valid uint8
}

func (t *NavTimeUTC) ValidTOW() bool { return t.Valid&0x01 != 0 }
func (t *NavTimeUTC) ValidWKN() bool { return t.Valid&0x02 != 0 }
func (t *NavTimeUTC) ValidUTC() bool { return t.Valid&0x04 != 0 }

func ParseNavTimeUTC(payload []byte) (*NavTimeUTC, error) {
	if len(payload) < 20 {
		return nil, fmt.Errorf("ubx: NAV-TIMEUTC payload too short: %d", len(payload))
	}
	return &NavTimeUTC{
		ITOW:  binary.LittleEndian.Uint32(payload[0:4]),
		TAcc:  binary.LittleEndian.Uint32(payload[4:8]),
		Nano:  int32(binary.LittleEndian.Uint32(payload[8:12])),
		Year:  binary.LittleEndian.Uint16(payload[12:14]),
		Month: payload[14],
		Day:   payload[15],
		Hour:  payload[16],
		Min:   payload[17],
		Sec:   payload[18],
		Valid: payload[19],
	}, nil
}

// NavClock is UBX-NAV-CLOCK (0x01 0x22), 20 bytes.
type NavClock struct {
	ITOW uint32 // ms
	ClkB int32  // ns, clock bias
	ClkD int32  // ns/s, clock drift
	TAcc uint32 // ns, time accuracy
	FAcc uint32 // ps/s, frequency accuracy
}

func ParseNavClock(payload []byte) (*NavClock, error) {
	if len(payload) < 20 {
		return nil, fmt.Errorf("ubx: NAV-CLOCK payload too short: %d", len(payload))
	}
	return &NavClock{
		ITOW: binary.LittleEndian.Uint32(payload[0:4]),
		ClkB: int32(binary.LittleEndian.Uint32(payload[4:8])),
		ClkD: int32(binary.LittleEndian.Uint32(payload[8:12])),
		TAcc: binary.LittleEndian.Uint32(payload[12:16]),
		FAcc: binary.LittleEndian.Uint32(payload[16:20]),
	}, nil
}

// NavSAT is UBX-NAV-SAT (0x01 0x35), variable length.
type NavSAT struct {
	ITOW   uint32
	NumSvs uint8
	Svs    []SatInfo
}

type SatInfo struct {
	GnssID uint8
	SvID   uint8
	Cno    uint8  // dBHz
	Elev   int8   // deg
	Azim   int16  // deg
	PrRes  int16  // 0.1 m
	Flags  uint32
}

func (s *SatInfo) QualityInd() uint8 { return uint8(s.Flags & 0x07) }
func (s *SatInfo) SvUsed() bool      { return s.Flags&0x08 != 0 }
func (s *SatInfo) Health() uint8     { return uint8((s.Flags >> 4) & 0x03) }

func ParseNavSAT(payload []byte) (*NavSAT, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("ubx: NAV-SAT payload too short: %d", len(payload))
	}
	numSvs := payload[5]
	need := 8 + int(numSvs)*12
	if len(payload) < need {
		return nil, fmt.Errorf("ubx: NAV-SAT payload too short for %d svs: %d", numSvs, len(payload))
	}
	sat := &NavSAT{
		ITOW:   binary.LittleEndian.Uint32(payload[0:4]),
		NumSvs: numSvs,
		Svs:    make([]SatInfo, numSvs),
	}
	for i := 0; i < int(numSvs); i++ {
		off := 8 + i*12
		sat.Svs[i] = SatInfo{
			GnssID: payload[off],
			SvID:   payload[off+1],
			Cno:    payload[off+2],
			Elev:   int8(payload[off+3]),
			Azim:   int16(binary.LittleEndian.Uint16(payload[off+4 : off+6])),
			PrRes:  int16(binary.LittleEndian.Uint16(payload[off+6 : off+8])),
			Flags:  binary.LittleEndian.Uint32(payload[off+8 : off+12]),
		}
	}
	return sat, nil
}

// TimTP is UBX-TIM-TP (0x0d 0x01), 16 bytes.
type TimTP struct {
	TowMS    uint32 // ms
	TowSubMS uint32 // 2^-32 ms
	QErr     int32  // ps, quantization error
	Week     uint16
	Flags    uint8
	RefInfo  uint8
}

func ParseTimTP(payload []byte) (*TimTP, error) {
	if len(payload) < 16 {
		return nil, fmt.Errorf("ubx: TIM-TP payload too short: %d", len(payload))
	}
	return &TimTP{
		TowMS:    binary.LittleEndian.Uint32(payload[0:4]),
		TowSubMS: binary.LittleEndian.Uint32(payload[4:8]),
		QErr:     int32(binary.LittleEndian.Uint32(payload[8:12])),
		Week:     binary.LittleEndian.Uint16(payload[12:14]),
		Flags:    payload[14],
		RefInfo:  payload[15],
	}, nil
}

// MonVer is UBX-MON-VER (0x0a 0x04), variable length.
type MonVer struct {
	SwVersion string
	HwVersion string
	Extensions []string
}

func ParseMonVer(payload []byte) (*MonVer, error) {
	if len(payload) < 40 {
		return nil, fmt.Errorf("ubx: MON-VER payload too short: %d", len(payload))
	}
	ver := &MonVer{
		SwVersion: strings.TrimRight(string(payload[0:30]), "\x00"),
		HwVersion: strings.TrimRight(string(payload[30:40]), "\x00"),
	}
	rest := payload[40:]
	for len(rest) >= 30 {
		ver.Extensions = append(ver.Extensions, strings.TrimRight(string(rest[:30]), "\x00"))
		rest = rest[30:]
	}
	return ver, nil
}

// Ack is the payload for ACK-ACK and ACK-NAK.
type Ack struct {
	ClsID uint8
	MsgID uint8
}

func ParseAck(payload []byte) (*Ack, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("ubx: ACK payload too short: %d", len(payload))
	}
	return &Ack{ClsID: payload[0], MsgID: payload[1]}, nil
}
