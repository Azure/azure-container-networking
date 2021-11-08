package wireserver

import (
	"bytes"
	"encoding/xml"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXMLDecode(t *testing.T) {
	b, err := os.ReadFile("testdata/interfaces.xml")
	require.NoError(t, err)
	var resp GetInterfacesResult
	require.NoError(t, xml.NewDecoder(bytes.NewReader(b)).Decode(&resp))
	want := GetInterfacesResult{
		Interface: []Interface{
			{
				MacAddress: "002248263DBD",
				IsPrimary:  true,
				IPSubnet: []Subnet{
					{
						Prefix: "10.240.0.0/16",
						IPAddress: []Address{
							{
								Address:   "10.240.0.4",
								IsPrimary: true,
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, want, resp)
}
