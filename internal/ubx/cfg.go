package ubx

import "encoding/binary"

// Gen 9 configuration key IDs from NEO-M9N interface description (UBX-19035940).
// Key format: bits[28:31]=size, bits[16:27]=group, bits[0:15]=item.
const (
	// MSGOUT - USB port message rates
	CfgMsgoutNmeaRmcUSB     uint32 = 0x209100ae
	CfgMsgoutNmeaZdaUSB     uint32 = 0x209100db
	CfgMsgoutNmeaGgaUSB     uint32 = 0x209100c0 // to disable
	CfgMsgoutNmeaGllUSB     uint32 = 0x209100cc // to disable
	CfgMsgoutNmeaGsaUSB     uint32 = 0x209100c3 // to disable
	CfgMsgoutNmeaGsvUSB     uint32 = 0x209100c9 // to disable
	CfgMsgoutNmeaVtgUSB     uint32 = 0x209100b4 // to disable
	CfgMsgoutUbxNavPvtUSB   uint32 = 0x20910009
	CfgMsgoutUbxNavTimeUSB  uint32 = 0x2091005e
	CfgMsgoutUbxNavClkUSB   uint32 = 0x20910068
	CfgMsgoutUbxNavSatUSB   uint32 = 0x20910018
	CfgMsgoutUbxTimTpUSB    uint32 = 0x20910180
	CfgMsgoutUbxNavSigUSB   uint32 = 0x20910348

	// RATE
	CfgRateMeas    uint32 = 0x30210001 // U2, measurement period ms
	CfgRateNav     uint32 = 0x30210002 // U2, nav solution ratio
	CfgRateTimeref uint32 = 0x20210003 // U1, time reference (0=UTC, 1=GPS, ...)

	// USB protocol masks
	CfgUSBInprotUBX  uint32 = 0x10770001 // L (bool)
	CfgUSBInprotNMEA uint32 = 0x10770002
	CfgUSBOutprotUBX  uint32 = 0x10780001
	CfgUSBOutprotNMEA uint32 = 0x10780002
)

// Layer bitmask for VALSET.
const (
	LayerRAM   uint8 = 1 << 0
	LayerBBR   uint8 = 1 << 1
	LayerFlash uint8 = 1 << 2
)

// cfgItem is a key-value pair for VALSET.
type cfgItem struct {
	Key uint32
	Val []byte
}

func CfgU1(key uint32, val uint8) cfgItem {
	return cfgItem{Key: key, Val: []byte{val}}
}

func CfgU2(key uint32, val uint16) cfgItem {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, val)
	return cfgItem{Key: key, Val: b}
}

func CfgL(key uint32, val bool) cfgItem {
	v := uint8(0)
	if val {
		v = 1
	}
	return cfgItem{Key: key, Val: []byte{v}}
}

// EncodeValset builds a UBX-CFG-VALSET frame (transactionless).
func EncodeValset(layers uint8, items ...cfgItem) []byte {
	size := 4
	for _, it := range items {
		size += 4 + len(it.Val)
	}
	payload := make([]byte, 4, size)
	payload[0] = 0x00 // version
	payload[1] = layers
	// payload[2], payload[3] = reserved
	for _, it := range items {
		kb := make([]byte, 4)
		binary.LittleEndian.PutUint32(kb, it.Key)
		payload = append(payload, kb...)
		payload = append(payload, it.Val...)
	}
	return Encode(ClassCFG, IDCfgValset, payload)
}

// EncodeValget builds a UBX-CFG-VALGET poll for the given keys from the given layer.
// layer: 0=RAM, 1=BBR, 2=Flash, 7=Default
func EncodeValget(layer uint8, keys ...uint32) []byte {
	payload := make([]byte, 4+len(keys)*4)
	payload[0] = 0x00 // version
	payload[1] = layer
	// payload[2:4] = position (0)
	for i, k := range keys {
		binary.LittleEndian.PutUint32(payload[4+i*4:], k)
	}
	return Encode(ClassCFG, IDCfgValget, payload)
}
