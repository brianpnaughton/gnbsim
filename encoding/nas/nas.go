// Copyright 2019-2020 hhorai. All rights reserved.
// Use of this source code is governed by a MIT license that can be found
// in the LICENSE file.

// Package nas is implementation for non-access stratum (NAS) procedure
// in the 5GS Sytem.
// document version: 3GPP TS 24.501 v16.3.0 (2019-12)
package nas

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strconv"

	"github.com/wmnsk/milenage"
)

type UE struct {
	MSIN             string
	MCC              uint8
	MNC              uint8
	RoutingIndicator uint16
	ProtectionScheme string
	AuthParam        AuthParam
}

// 9.1.1 NAS message format
type NasMessageMM struct {
	ExtendedProtocolDiscriminator uint8
	SecurityHeaderType            uint8
	MessageType                   uint8
}

// 8.2.6 Registration request
type RegistrationRequest struct {
	head                     NasMessageMM
	registrationTypeAndngKSI uint8
	fiveGSMobileID           FiveGSMobileID
}

// TS 24.007 11.2.3.1.1A Extended protocol discriminator (EPD)
const (
	EPD5GSSessionManagement  = 0x2e
	EPD5GSMobilityManagement = 0x7e
)

var epdStr = map[int]string{
	EPD5GSSessionManagement:  "5G Session Management",
	EPD5GSMobilityManagement: "5G Mobility Management",
}

/*
type NasMessageSM struct {
	ExtendedProtocolDiscriminator uint8
	PDUSessionID uint8
	ProcedureTransactionID uint8
	MessageType uint8
}
*/

// 9.3 Security header type
const (
	SecurityHeaderTypePlain = iota
	SecurityHeaderTypeIntegrityProtected
	SecurityHeaderTypeIntegrityProtectedAndCiphered
)

// 9.7 Message type
const (
	MessageTypeRegistrationRequest   = 0x41
	MessageTypeAuthenticationRequest = 0x56
)

var msgTypeStr = map[int]string{
	MessageTypeRegistrationRequest:   "Registration Request",
	MessageTypeAuthenticationRequest: "Authentication Request",
}

// 9.11.3.1 5GMM capability
type FiveGMMCapability struct {
	iei         uint8
	length      uint8
	capability1 uint8
}

const (
	FiveGMMCapN3data = 0x20
)

// 9.11.3.4 5GS mobile identity
type FiveGSMobileID struct {
	length                 uint16
	supiFormatAndTypeID    uint8
	plmn                   [3]uint8
	routingIndicator       [2]uint8
	protectionScheme       uint8
	homeNetworkPublicKeyID uint8
	schemeOutput           [5]uint8
}

const (
	TypeIDNoIdentity = iota
	TypeIDSUCI
)

const (
	SUPIFormatIMSI = iota
	SUPIFormatNetworkSpecificID
)

const (
	ProtectionSchemeNull = iota
	ProtectionSchemeProfileA
	ProtectionSchemeProfileB
)

// 9.11.3.7 5GS registration type
const (
	RegistrationTypeInitialRegistration        = 0x01
	RegistrationTypeFlagFollowOnRequestPending = 0x08
)

// 9.11.3.32 NAS key set identifier
const (
	KeySetIdentityNoKeyIsAvailable          = 0x07
	KeySetIdentityFlagMappedSecurityContext = 0x08
)

// 9.11.3.54 UE security capability
type UESecurityCapability struct {
	iei    uint8
	length uint8
	ea     uint8
	ia     uint8
	eea    uint8
	eia    uint8
}

const (
	EA0 = 0x80
	EA1 = 0x40
	EA2 = 0x20
	IA0 = 0x80
	IA1 = 0x40
	IA2 = 0x20
)

const (
	iei5GMMCapability       = 0x10
	ieiAuthParamAUTN        = 0x20
	ieiAuthParamRAND        = 0x21
	ieiUESecurityCapability = 0x2e
)

func Str2BCD(str string) (bcd []byte) {

	byteArray := []byte(str)
	bcdlen := len(byteArray) / 2
	if len(byteArray)%2 == 1 {
		bcdlen++
	}
	bcd = make([]byte, bcdlen, bcdlen)

	for i, v := range byteArray {

		n, _ := strconv.ParseUint(string(v), 16, 8)
		j := i / 2

		if i%2 == 0 {
			bcd[j] = byte(n)
		} else {
			bcd[j] |= (byte(n) << 4)
		}
	}

	return
}

func NewNAS(filename string) (p *UE) {

	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	var ue UE
	p = &ue
	json.Unmarshal(bytes, p)

	return
}

func (ue *UE) Decode(pdu *[]byte, length int) (msgType int) {
	offset := 0
	epd := int((*pdu)[offset])
	fmt.Printf("EPD: %s (0x%x)\n", epdStr[epd], epd)
	offset++

	secHeader := int((*pdu)[offset])
	fmt.Printf("Security Header: 0x%x\n", secHeader)
	offset++

	msgType = int((*pdu)[offset])
	fmt.Printf("Message Type: %s (0x%x)\n", msgTypeStr[msgType], msgType)
	offset++

	switch msgType {
	case MessageTypeAuthenticationRequest:
		ue.decAuthenticationRequest(pdu, length, offset)
		break
	default:
		break
	}
	return
}

func (ue *UE) decInformationElement(pdu *[]byte, length, offset int) {

	for offset < length {
		iei := int((*pdu)[offset])
		offset++

		switch iei {
		case ieiAuthParamAUTN:
			offset = ue.decAuthParamAUTN(pdu, length, offset)
			break
		case ieiAuthParamRAND:
			offset = ue.decAuthParamRAND(pdu, length, offset)
			break
		default:
			fmt.Printf("unsupported IE\n")
			offset = length
			break
		}
	}
}

// 8.2.1 Authentication request
func (ue *UE) decAuthenticationRequest(pdu *[]byte, length, offset int) {
	fmt.Printf("decAuthenticationRequest\n")

	ksi := int((*pdu)[offset])
	fmt.Printf("ngKSI: 0x%x\n", ksi)
	offset++

	offset = ue.decABBA(pdu, offset)

	ue.decInformationElement(pdu, length, offset)

	k, _ := hex.DecodeString(ue.AuthParam.K)
	opc, _ := hex.DecodeString(ue.AuthParam.OPc)
	amf := binary.BigEndian.Uint16(ue.AuthParam.amf)

	m := milenage.NewWithOPc(k, opc, ue.AuthParam.rand, 0, amf)
	m.F2345()
	for n, v := range ue.AuthParam.seqxorak {
		m.SQN[n] = v ^ m.AK[n]
	}
	m.F1()

	fmt.Printf("   K: %x\n", m.K)
	fmt.Printf("   OP: %x\n", m.OP)
	fmt.Printf("   OPc: %x\n", m.OPc)
	fmt.Printf("   AMF: %x\n", m.AMF)
	fmt.Printf("   SQN(%d): %x\n", len(m.SQN), m.SQN)
	fmt.Printf("   CK: %x\n", m.CK)
	fmt.Printf("   IK: %x\n", m.IK)
	fmt.Printf("   AK(%d): %x\n", len(m.AK), m.AK)
	fmt.Printf("   MACA: %x\n", m.MACA)
	fmt.Printf("   MACS: %x\n", m.MACS)
	fmt.Printf("   RAND: %x\n", m.RAND)
	fmt.Printf("   RES: %x\n", m.RES)

	if reflect.DeepEqual(ue.AuthParam.mac, m.MACA) == false {
		fmt.Printf("  received and calculated MAC values do not match.\n")
		// need response for error.
		return
	}
	ue.AuthParam.RES = m.RES

	fmt.Printf("  received and calculated MAC values match.\n")
	return
}

// 8.2.6 Registration request
// 5.5.1.2 Registration procedure for initial registration
func (p *UE) MakeRegistrationRequest() (pdu []byte) {

	var req RegistrationRequest
	var h *NasMessageMM = &req.head
	h.ExtendedProtocolDiscriminator = EPD5GSMobilityManagement
	h.SecurityHeaderType = SecurityHeaderTypePlain
	h.MessageType = MessageTypeRegistrationRequest

	var regType uint8 = RegistrationTypeInitialRegistration |
		RegistrationTypeFlagFollowOnRequestPending
	var ngKSI uint8 = KeySetIdentityNoKeyIsAvailable

	req.registrationTypeAndngKSI = regType | (ngKSI << 4)

	var f *FiveGSMobileID = &req.fiveGSMobileID
	var typeID uint8 = TypeIDSUCI
	var supiFormat uint8 = SUPIFormatIMSI

	/*
	 * it doesn't work with "f.length = uint16(unsafe.Sizeof(*f) - 2)"
	 * because of the octet alignment.
	 */
	f.length = 13
	f.supiFormatAndTypeID = typeID | (supiFormat << 4)
	f.plmn = encPLMN(p.MCC, p.MNC)
	f.routingIndicator = encRoutingIndicator(p.RoutingIndicator)
	f.protectionScheme = encProtectionScheme(p.ProtectionScheme)
	f.homeNetworkPublicKeyID = 0
	f.schemeOutput = encSchemeOutput(p.MSIN)

	data := new(bytes.Buffer)
	binary.Write(data, binary.BigEndian, req)
	binary.Write(data, binary.BigEndian, enc5GMMCapability())
	binary.Write(data, binary.BigEndian, encUESecurityCapability())
	pdu = data.Bytes()

	return
}

func encPLMN(mcc, mnc uint8) (plmn [3]byte) {
	format := "%d%d"
	if mnc < 100 {
		format = "%df%d"
	}

	str := fmt.Sprintf(format, mcc, mnc)
	for i, v := range Str2BCD(str) {
		plmn[i] = v
	}
	return
}

func encRoutingIndicator(ind uint16) (ri [2]byte) {
	str := fmt.Sprintf("%d", ind)
	for i, v := range Str2BCD(str) {
		ri[i] = v
	}
	return
}

func encProtectionScheme(profile string) (p uint8) {
	switch profile {
	case "null":
		p = ProtectionSchemeNull
	}
	return
}

func encSchemeOutput(msin string) (so [5]byte) {
	for i, v := range Str2BCD(msin) {
		so[i] = v
	}
	return
}

// 9.11.3.1 5GMM capability
func enc5GMMCapability() (f FiveGMMCapability) {
	f.iei = 0x10
	f.length = 1
	f.capability1 = FiveGMMCapN3data

	return
}

// 9.11.3.10 ABBA
func (ue *UE) decABBA(pdu *[]byte, baseOffset int) (offset int) {

	offset = baseOffset

	length := int((*pdu)[offset])
	offset++
	abba := (*pdu)[offset : offset+length]
	offset += length

	fmt.Printf("ABBA\n")
	fmt.Printf(" Length: %d\n", length)
	fmt.Printf(" Value: %02x\n", abba)

	return
}

// 9.11.3.15 Authentication parameter AUTN
// TS 24.008 10.5.3.1.1 Authentication Parameter AUTN (UMTS and EPS authentication challenge)
type AuthParam struct {
	K        string
	OPc      string
	rand     []byte
	autn     []byte
	seqxorak []byte
	amf      []byte
	mac      []byte
	RES      []byte
}

func (ue *UE) decAuthParamAUTN(pdu *[]byte, length, orig int) (offset int) {

	offset = orig
	fmt.Printf("Auth Param AUTN\n")

	autnlen := int((*pdu)[offset])
	offset++
	ue.AuthParam.autn = (*pdu)[offset : offset+autnlen]
	fmt.Printf(" AUTN: %02x\n", ue.AuthParam.autn)
	ue.AuthParam.seqxorak = ue.AuthParam.autn[:6]
	ue.AuthParam.amf = ue.AuthParam.autn[6:8]
	ue.AuthParam.mac = ue.AuthParam.autn[8:16]
	fmt.Printf("  SEQ xor AK: %02x\n", ue.AuthParam.seqxorak)
	fmt.Printf("  AMF: %02x\n", ue.AuthParam.amf)
	fmt.Printf("  MAC: %02x\n", ue.AuthParam.mac)
	offset += autnlen
	return
}

// 9.11.3.16 Authentication parameter RAND
// TS 24.008 10.5.3.1 Authentication parameter RAND
func (ue *UE) decAuthParamRAND(pdu *[]byte, length, orig int) (offset int) {

	offset = orig
	fmt.Printf("Auth Param RAND\n")

	const randlen = 16
	ue.AuthParam.rand = (*pdu)[offset : offset+randlen]
	fmt.Printf(" RAND: %02x\n", ue.AuthParam.rand)
	offset += randlen
	return
}

// 9.11.3.54 UE security capability
func encUESecurityCapability() (sc UESecurityCapability) {
	sc.iei = ieiUESecurityCapability
	sc.length = 4

	// use null encryption at this moment.
	sc.ea = EA0
	sc.ia = IA0

	return
}
