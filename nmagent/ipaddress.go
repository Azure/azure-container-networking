package nmagent

import (
	"encoding/xml"
	"net/netip"

	"github.com/pkg/errors"
)

type IPAddress netip.Addr

func (h IPAddress) Equal(other IPAddress) bool {
	return netip.Addr(h).Compare(netip.Addr(other)) == 0
}

func (h *IPAddress) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var ipStr string
	if err := d.DecodeElement(&ipStr, &start); err != nil {
		return errors.Wrap(err, "decoding IP address")
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return errors.Wrap(err, "parsing IP address")
	}

	*h = IPAddress(ip)
	return nil
}

func (h *IPAddress) UnmarshalXMLAttr(attr xml.Attr) error {
	ipStr := attr.Value
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return errors.Wrap(err, "parsing IP address")
	}

	*h = IPAddress(ip)
	return nil
}

func (h IPAddress) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	err := e.EncodeElement(netip.Addr(h).String(), start)
	return errors.Wrap(err, "encoding IP address")
}

func (h IPAddress) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{
		Name:  name,
		Value: netip.Addr(h).String(),
	}, nil
}
