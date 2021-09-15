package kubecontroller

import (
	"context"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconciler_Reconcile(t *testing.T) {
	type fields struct {
		cnscli cnsclient
		nnccli nncgetter
	}
	type args struct {
		ctx     context.Context
		request reconcile.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				cnscli: tt.fields.cnscli,
				nnccli: tt.fields.nnccli,
			}
			got, err := r.Reconcile(tt.args.ctx, tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconciler.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}
