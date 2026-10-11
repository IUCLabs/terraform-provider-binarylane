package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-binarylane/internal/binarylane"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestServerReverseNameResourceUnit(t *testing.T) {
	ctx := context.Background()

	var (
		postStatus  = http.StatusOK
		serverFound = true
		serverIp    = "203.0.113.7"
		serverName  = "ptr.example.com"
		lastBody    map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/servers/1/actions":
			b, _ := io.ReadAll(r.Body)
			lastBody = nil
			if err := json.Unmarshal(b, &lastBody); err != nil {
				t.Errorf("decode POST body: %v", err)
			}
			w.WriteHeader(postStatus)
			if postStatus == http.StatusOK {
				_, _ = w.Write([]byte(`{"action":{"id":9,"status":"in-progress"}}`))
			}
		case r.Method == http.MethodGet && r.URL.Path == "/servers/1/actions/9":
			_, _ = w.Write([]byte(`{"action":{"id":9,"status":"completed","completed_at":"2026-01-01T00:00:00Z"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/servers/1":
			if !serverFound {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"server":{"id":1,"networks":{"v4":[{"ip_address":%q,"type":"public","reverse_name":%q}]}}}`, serverIp, serverName)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	client, err := binarylane.NewClientWithConfig(srv.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := &serverReverseNameResource{bc: &BinarylaneClient{client: client}}

	schemaResp := resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema
	emptyState := func() tfsdk.State {
		return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	}
	model := serverReverseNameResourceModel{
		Id:          types.StringUnknown(),
		ServerId:    types.Int64Value(1),
		Ipv4Address: types.StringValue("203.0.113.7"),
		ReverseName: types.StringValue("ptr.example.com"),
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}

	createResp := resource.CreateResponse{State: emptyState()}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatal(createResp.Diagnostics)
	}
	if lastBody["reverse_name"] != "ptr.example.com" || lastBody["ipv4_address"] != "203.0.113.7" {
		t.Fatalf("create: unexpected request body %v", lastBody)
	}
	state := createResp.State

	read := func() (serverReverseNameResourceModel, bool) {
		t.Helper()
		readResp := resource.ReadResponse{State: state}
		r.Read(ctx, resource.ReadRequest{State: state}, &readResp)
		if readResp.Diagnostics.HasError() {
			t.Fatal(readResp.Diagnostics)
		}
		if readResp.State.Raw.IsNull() {
			return serverReverseNameResourceModel{}, false
		}
		var got serverReverseNameResourceModel
		if diags := readResp.State.Get(ctx, &got); diags.HasError() {
			t.Fatal(diags)
		}
		return got, true
	}

	serverName = "drifted.example.com"
	if got, ok := read(); !ok || got.ReverseName.ValueString() != "drifted.example.com" {
		t.Fatalf("read: expected drift to drifted.example.com, got %v (present=%v)", got.ReverseName, ok)
	}

	serverIp = "198.51.100.1"
	if _, ok := read(); ok {
		t.Fatal("read: expected state removed when IP is not on the server")
	}

	serverFound = false
	if _, ok := read(); ok {
		t.Fatal("read: expected state removed when server is 404")
	}

	postStatus = http.StatusAccepted
	updateResp := resource.UpdateResponse{State: emptyState()}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: 202 without action should succeed: %v", updateResp.Diagnostics)
	}

	deleteResp := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: 202 without action should succeed: %v", deleteResp.Diagnostics)
	}
	if _, ok := lastBody["reverse_name"]; ok {
		t.Fatalf("delete: request body must not contain reverse_name, got %v", lastBody)
	}
	if lastBody["ipv4_address"] != "203.0.113.7" {
		t.Fatalf("delete: unexpected request body %v", lastBody)
	}
}
