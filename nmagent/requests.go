package nmagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/Azure/azure-container-networking/nmagent/internal"
	"github.com/pkg/errors"
)

// Request represents an abstracted HTTP request, capable of validating itself,
// producing a valid Path, Body, and its Method.
type Request interface {
	// Validate should ensure that the request is valid to submit
	Validate() error

	// Path should produce a URL path, complete with any URL parameters
	// interpolated
	Path() string

	// Body produces the HTTP request body necessary to submit the request
	Body() (io.Reader, error)

	// Method returns the HTTP Method to be used for the request.
	Method() string
}

var _ Request = &PutNetworkContainerRequest{}

// PutNetworkContainerRequest is a collection of parameters necessary to create
// a new network container
type PutNetworkContainerRequest struct {
	// NOTE(traymond): if you are adding a new field to this struct, ensure that it is also added
	// to the MarshallJSON, UnmarshallJSON and  method as well.
	ID     string // the id of the network container
	VNetID string // the id of the customer's vnet

	// Version is the new network container version
	Version uint64

	// SubnetName is the name of the delegated subnet. This is used to
	// authenticate the request. The list of ipv4addresses must be contained in
	// the subnet's prefix.
	SubnetName string

	// IPv4 addresses in the customer virtual network that will be assigned to
	// the interface.
	IPv4Addrs []string

	Policies []Policy // policies applied to the network container

	// VlanID is used to distinguish Network Containers with duplicate customer
	// addresses. "0" is considered a default value by the API.
	VlanID int

	GREKey uint16

	// AuthenticationToken is the base64 security token for the subnet containing
	// the Network Container addresses
	AuthenticationToken string

	// PrimaryAddress is the primary customer address of the interface in the
	// management VNet
	PrimaryAddress string

	// AzID is the home AZ ID of the network container
	AzID uint

	// AZREnabled denotes whether AZR is enabled for network container or not
	AZREnabled bool
}

type internalNC struct {
	// NMAgent expects this to be a string, except that the contents of that string have to be a uint64.
	// Therefore, the type we expose to clients uses a uint64 to guarantee that, but we
	// convert it to a string here.
	Version string `json:"version"`

	// The rest of these are copied verbatim from the above struct and should be kept in sync.
	VNetID     string   `json:"virtualNetworkId"`
	SubnetName string   `json:"subnetName"`
	IPv4Addrs  []string `json:"ipV4Addresses"`
	Policies   []Policy `json:"policies"`
	VlanID     int      `json:"vlanId"`
	GREKey     uint16   `json:"greKey"`
	AzID       uint     `json:"azID"`
	AZREnabled bool     `json:"azrEnabled"`
}

func (p *PutNetworkContainerRequest) MarshalJSON() ([]byte, error) {
	pBody := internalNC{
		Version:    strconv.Itoa(int(p.Version)),
		VNetID:     p.VNetID,
		SubnetName: p.SubnetName,
		IPv4Addrs:  p.IPv4Addrs,
		Policies:   p.Policies,
		VlanID:     p.VlanID,
		GREKey:     p.GREKey,
		AzID:       p.AzID,
		AZREnabled: p.AZREnabled,
	}

	body, err := json.Marshal(pBody)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling PutNetworkContainerRequest")
	}
	return body, nil
}

func (p *PutNetworkContainerRequest) UnmarshalJSON(in []byte) error {
	var req internalNC
	err := json.Unmarshal(in, &req)
	if err != nil {
		return errors.Wrap(err, "unmarshal network container request")
	}

	//nolint:gomnd // these magic numbers are well-documented in ParseUint
	version, err := strconv.ParseUint(req.Version, 10, 64)
	if err != nil {
		return errors.Wrap(err, "parsing version string as uint64")
	}

	p.Version = version
	p.VNetID = req.VNetID
	p.SubnetName = req.SubnetName
	p.IPv4Addrs = req.IPv4Addrs
	p.Policies = req.Policies
	p.VlanID = req.VlanID
	p.GREKey = req.GREKey
	p.AzID = req.AzID
	p.AZREnabled = req.AZREnabled

	return nil
}

// Body marshals the JSON fields of the request and produces an Reader intended
// for use with an HTTP request
func (p *PutNetworkContainerRequest) Body() (io.Reader, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling PutNetworkContainerRequest")
	}

	return bytes.NewReader(body), nil
}

// Method returns the HTTP method for this request type
func (p *PutNetworkContainerRequest) Method() string {
	return http.MethodPost
}

// Path returns the URL path necessary to submit this PutNetworkContainerRequest
func (p *PutNetworkContainerRequest) Path() string {
	const PutNCRequestPath string = "/NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1"
	return fmt.Sprintf(PutNCRequestPath, p.PrimaryAddress, p.ID, p.AuthenticationToken)
}

// Validate ensures that all of the required parameters of the request have
// been filled out properly prior to submission to NMAgent
func (p *PutNetworkContainerRequest) Validate() error {
	err := internal.ValidationError{}

	// URL requirements:
	if p.PrimaryAddress == "" {
		err.MissingFields = append(err.MissingFields, "PrimaryAddress")
	}

	if p.ID == "" {
		err.MissingFields = append(err.MissingFields, "ID")
	}

	if p.AuthenticationToken == "" {
		err.MissingFields = append(err.MissingFields, "AuthenticationToken")
	}

	// Documented requirements:
	if p.SubnetName == "" {
		err.MissingFields = append(err.MissingFields, "SubnetName")
	}

	if len(p.IPv4Addrs) == 0 {
		err.MissingFields = append(err.MissingFields, "IPv4Addrs")
	}

	if p.VNetID == "" {
		err.MissingFields = append(err.MissingFields, "VNetID")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

type Policy struct {
	ID   string
	Type string
}

// MarshalJson encodes policies as a JSON string, separated by a comma. This
// specific format is requested by the NMAgent documentation
func (p Policy) MarshalJSON() ([]byte, error) {
	out := bytes.NewBufferString(p.ID)
	out.WriteString(", ")
	out.WriteString(p.Type)

	outStr := out.String()
	// nolint:wrapcheck // wrapping this error provides no useful information
	return json.Marshal(outStr)
}

// UnmarshalJSON decodes a JSON-encoded policy string
func (p *Policy) UnmarshalJSON(in []byte) error {
	const expectedNumParts = 2

	var raw string
	err := json.Unmarshal(in, &raw)
	if err != nil {
		return errors.Wrap(err, "decoding policy")
	}

	parts := strings.Split(raw, ",")
	if len(parts) != expectedNumParts {
		return errors.New("policies must be two comma-separated values")
	}

	p.ID = strings.TrimFunc(parts[0], unicode.IsSpace)
	p.Type = strings.TrimFunc(parts[1], unicode.IsSpace)

	return nil
}

var _ Request = JoinNetworkRequest{}

type JoinNetworkRequest struct {
	NetworkID string `json:"-"` // the customer's VNet ID
}

// Path constructs a URL path for invoking a JoinNetworkRequest using the
// provided parameters
func (j JoinNetworkRequest) Path() string {
	const JoinNetworkPath string = "/NetworkManagement/joinedVirtualNetworks/%s/api-version/1"
	return fmt.Sprintf(JoinNetworkPath, j.NetworkID)
}

// Body returns nothing, because JoinNetworkRequest has no request body
func (j JoinNetworkRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method returns the HTTP request method to submit a JoinNetworkRequest
func (j JoinNetworkRequest) Method() string {
	return http.MethodPost
}

// Validate ensures that the provided parameters of the request are valid
func (j JoinNetworkRequest) Validate() error {
	err := internal.ValidationError{}

	if j.NetworkID == "" {
		err.MissingFields = append(err.MissingFields, "NetworkID")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

var _ Request = DeleteNetworkRequest{}

// DeleteNetworkRequest represents all information necessary to request that
// NMAgent delete a particular network
type DeleteNetworkRequest struct {
	NetworkID string `json:"-"` // the customer's VNet ID
}

// Path constructs a URL path for invoking a DeleteNetworkRequest using the
// provided parameters
func (d DeleteNetworkRequest) Path() string {
	const DeleteNetworkPath = "/NetworkManagement/joinedVirtualNetworks/%s/api-version/1/method/DELETE"
	return fmt.Sprintf(DeleteNetworkPath, d.NetworkID)
}

// Body returns nothing, because DeleteNetworkRequest has no request body
func (d DeleteNetworkRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method returns the HTTP request method to submit a DeleteNetworkRequest
func (d DeleteNetworkRequest) Method() string {
	return http.MethodPost
}

// Validate ensures that the provided parameters of the request are valid
func (d DeleteNetworkRequest) Validate() error {
	err := internal.ValidationError{}

	if d.NetworkID == "" {
		err.MissingFields = append(err.MissingFields, "NetworkID")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

var _ Request = DeleteContainerRequest{}

// DeleteContainerRequest represents all information necessary to request that
// NMAgent delete a particular network container
type DeleteContainerRequest struct {
	NCID       string `json:"-"`          // the Network Container ID
	AzID       uint   `json:"azID"`       // home AZ of the Network Container
	AZREnabled bool   `json:"azrEnabled"` // whether AZR is enabled or not

	// PrimaryAddress is the primary customer address of the interface in the
	// management VNET
	PrimaryAddress      string `json:"-"`
	AuthenticationToken string `json:"-"`
}

// Path returns the path for submitting a DeleteContainerRequest with
// parameters interpolated correctly
func (d DeleteContainerRequest) Path() string {
	const DeleteNCPath string = "/NetworkManagement/interfaces/%s/networkContainers/%s/authenticationToken/%s/api-version/1/method/DELETE"
	return fmt.Sprintf(DeleteNCPath, d.PrimaryAddress, d.NCID, d.AuthenticationToken)
}

// Body returns nothing, because DeleteContainerRequests have no HTTP body
func (d DeleteContainerRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method returns the HTTP method required to submit a DeleteContainerRequest
func (d DeleteContainerRequest) Method() string {
	return http.MethodPost
}

// Validate ensures that the DeleteContainerRequest has the correct information
// to submit the request
func (d DeleteContainerRequest) Validate() error {
	err := internal.ValidationError{}

	if d.NCID == "" {
		err.MissingFields = append(err.MissingFields, "NCID")
	}

	if d.PrimaryAddress == "" {
		err.MissingFields = append(err.MissingFields, "PrimaryAddress")
	}

	if d.AuthenticationToken == "" {
		err.MissingFields = append(err.MissingFields, "AuthenticationToken")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

var _ Request = GetNetworkConfigRequest{}

// GetNetworkConfigRequest is a collection of necessary information for
// submitting a request for a customer's network configuration
type GetNetworkConfigRequest struct {
	VNetID string `json:"-"` // the customer's virtual network ID
}

// Path produces a URL path used to submit a request
func (g GetNetworkConfigRequest) Path() string {
	const GetNetworkConfigPath string = "/NetworkManagement/joinedVirtualNetworks/%s/api-version/1"
	return fmt.Sprintf(GetNetworkConfigPath, g.VNetID)
}

// Body returns nothing because GetNetworkConfigRequest has no HTTP request
// body
func (g GetNetworkConfigRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method returns the HTTP method required to submit a GetNetworkConfigRequest
func (g GetNetworkConfigRequest) Method() string {
	return http.MethodGet
}

// Validate ensures that the request is complete and the parameters are correct
func (g GetNetworkConfigRequest) Validate() error {
	err := internal.ValidationError{}

	if g.VNetID == "" {
		err.MissingFields = append(err.MissingFields, "VNetID")
	}

	if err.IsEmpty() {
		return nil
	}
	return err
}

var _ Request = &SupportedAPIsRequest{}

// SupportedAPIsRequest is a collection of parameters necessary to submit a
// valid request to retrieve the supported APIs from an NMAgent instance.
type SupportedAPIsRequest struct{}

// Body is a no-op method to satisfy the Request interface while indicating
// that there is no body for a SupportedAPIs Request.
func (s *SupportedAPIsRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method indicates that SupportedAPIs requests are GET requests.
func (s *SupportedAPIsRequest) Method() string {
	return http.MethodGet
}

// Path returns the necessary URI path for invoking a supported APIs request.
func (s *SupportedAPIsRequest) Path() string {
	return "/GetSupportedApis"
}

// Validate is a no-op method because SupportedAPIsRequests have no parameters,
// and therefore can never be invalid.
func (s *SupportedAPIsRequest) Validate() error {
	return nil
}

var _ Request = NCVersionRequest{}

type NCVersionRequest struct {
	AuthToken          string `json:"-"`
	NetworkContainerID string `json:"-"`
	PrimaryAddress     string `json:"-"`
}

func (n NCVersionRequest) Body() (io.Reader, error) {
	// there is no body to an NCVersionRequest, so return nil
	return nil, nil
}

// Method indicates this request is a GET request
func (n NCVersionRequest) Method() string {
	return http.MethodGet
}

// Path returns the URL Path for the request with parameters interpolated as
// necessary.
func (n NCVersionRequest) Path() string {
	const path = "/NetworkManagement/interfaces/%s/networkContainers/%s/version/authenticationToken/%s/api-version/1"
	return fmt.Sprintf(path, n.PrimaryAddress, n.NetworkContainerID, n.AuthToken)
}

// Validate ensures the presence of all parameters of the NCVersionRequest, as
// none are optional.
func (n NCVersionRequest) Validate() error {
	err := internal.ValidationError{}

	if n.AuthToken == "" {
		err.MissingFields = append(err.MissingFields, "AuthToken")
	}

	if n.NetworkContainerID == "" {
		err.MissingFields = append(err.MissingFields, "NetworkContainerID")
	}

	if n.PrimaryAddress == "" {
		err.MissingFields = append(err.MissingFields, "PrimaryAddress")
	}

	if err.IsEmpty() {
		return nil
	}

	return err
}

var _ Request = NCVersionListRequest{}

// NCVersionListRequest is a collection of parameters necessary to submit a
// request to receive a list of NCVersions available from the NMAgent instance.
type NCVersionListRequest struct{}

func (NCVersionListRequest) Body() (io.Reader, error) {
	// there is no body for this request so...
	return nil, nil
}

// Method returns the HTTP method required for the request.
func (NCVersionListRequest) Method() string {
	return http.MethodGet
}

// Path returns the path required to issue the request.
func (NCVersionListRequest) Path() string {
	return "/NetworkManagement/interfaces/api-version/2"
}

// Validate performs any necessary validations for the request.
func (NCVersionListRequest) Validate() error {
	// there are no parameters, thus nothing to validate. Since the request
	// cannot be made invalid it's fine for this to simply...
	return nil
}

var _ Request = &GetHomeAzRequest{}

type GetHomeAzRequest struct{}

// Body is a no-op method to satisfy the Request interface while indicating
// that there is no body for a GetHomeAz Request.
func (g *GetHomeAzRequest) Body() (io.Reader, error) {
	return nil, nil
}

// Method indicates that GetHomeAz requests are GET requests.
func (g *GetHomeAzRequest) Method() string {
	return http.MethodGet
}

// Path returns the necessary URI path for invoking a GetHomeAz request.
func (g *GetHomeAzRequest) Path() string {
	return "/GetHomeAz/api-version/1"
}

// Validate is a no-op method because GetHomeAzRequest have no parameters,
// and therefore can never be invalid.
func (g *GetHomeAzRequest) Validate() error {
	return nil
}
