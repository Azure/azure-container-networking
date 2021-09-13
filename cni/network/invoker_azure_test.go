package network

import (
	"net"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

func TestAzureIPAMInvoker_Add(t *testing.T) {
	type fields struct {
		plugin *netPlugin
		nwInfo *network.NetworkInfo
	}
	type args struct {
		nwCfg        *cni.NetworkConfig
		in1          *cniSkel.CmdArgs
		subnetPrefix *net.IPNet
		options      map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *cniTypesCurr.Result
		want1   *cniTypesCurr.Result
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}
			got, got1, err := invoker.Add(tt.args.nwCfg, tt.args.in1, tt.args.subnetPrefix, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Add() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("Add() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestAzureIPAMInvoker_Delete(t *testing.T) {
	type fields struct {
		plugin *netPlugin
		nwInfo *network.NetworkInfo
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		in2     *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}
			if err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.in2, tt.args.options); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAzureIpamInvoker(t *testing.T) {
	type args struct {
		plugin *netPlugin
		nwInfo *network.NetworkInfo
	}
	tests := []struct {
		name string
		args args
		want *AzureIPAMInvoker
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAzureIpamInvoker(tt.args.plugin, tt.args.nwInfo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAzureIpamInvoker() = %v, want %v", got, tt.want)
			}
		})
	}
}
