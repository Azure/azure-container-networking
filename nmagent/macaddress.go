package nmagent

import (
	"encoding/hex"
	"encoding/xml"
	"net"

	"github.com/pkg/errors"
)

const (
	MACAddressSize = 6
)

type MACAddress net.HardwareAddr

func (h MACAddress) Equal(other MACAddress) bool {
	if len(h) != len(other) {
		return false
	}
	for i := range h {
		if h[i] != other[i] {
			return false
		}
	}
	return true
}

func (h *MACAddress) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var macStr string
	if err := d.DecodeElement(&macStr, &start); err != nil {
		return errors.Wrap(err, "decoding MAC address")
	}

	// Convert the string (without colons) into a valid MACAddress
	mac, err := hex.DecodeString(macStr)
	if err != nil {
		return &net.ParseError{Type: "MAC address", Text: macStr}
	}

	*h = MACAddress(mac)
	return nil
}

func (h *MACAddress) UnmarshalXMLAttr(attr xml.Attr) error {
	macStr := attr.Value
	mac, err := hex.DecodeString(macStr)
	if err != nil {
		return &net.ParseError{Type: "MAC address", Text: macStr}
	}

	*h = MACAddress(mac)
	return nil
}

func (h MACAddress) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if len(h) != MACAddressSize {
		return &net.AddrError{Err: "invalid MAC address", Addr: hex.EncodeToString(h)}
	}

	macStr := hex.EncodeToString(h)
	err := e.EncodeElement(macStr, start)
	return errors.Wrap(err, "encoding MAC address")
}

func (h MACAddress) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	if len(h) != MACAddressSize {
		return xml.Attr{}, &net.AddrError{Err: "invalid MAC address", Addr: hex.EncodeToString(h)}
	}

	macStr := hex.EncodeToString(h)
	attr := xml.Attr{
		Name:  name,
		Value: macStr,
	}

	return attr, nil
}
