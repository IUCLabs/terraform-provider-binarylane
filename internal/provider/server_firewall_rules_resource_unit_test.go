package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-binarylane/internal/binarylane"
	"terraform-provider-binarylane/internal/resources"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestServerFirewallRulesResourceOmittedPorts(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"firewall_rules":[{"action":"accept","protocol":"icmp","source_addresses":["10.0.0.0/8"],"destination_addresses":["10.0.0.1"],"destination_ports":null}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := binarylane.NewClientWithConfig(srv.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := &serverFirewallRulesResource{bc: &BinarylaneClient{client: client}}

	schemaResp := resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema
	emptyState := func() tfsdk.State {
		return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	}

	ruleType := resources.FirewallRulesValue{}
	rule := resources.NewFirewallRulesValueMust(ruleType.AttributeTypes(ctx), map[string]attr.Value{
		"description":           types.StringNull(),
		"action":                types.StringValue("accept"),
		"protocol":              types.StringValue("icmp"),
		"destination_addresses": types.ListValueMust(types.StringType, []attr.Value{types.StringValue("10.0.0.1")}),
		"destination_ports":     types.ListUnknown(types.StringType),
		"source_addresses":      types.ListValueMust(types.StringType, []attr.Value{types.StringValue("10.0.0.0/8")}),
	})
	model := serverFirewallRulesResourceModel{}
	model.ServerId = types.Int64Value(1)
	model.FirewallRules = types.ListValueMust(ruleType.Type(ctx), []attr.Value{rule})

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}

	createResp := resource.CreateResponse{State: emptyState()}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatal(createResp.Diagnostics)
	}
	var ports types.List
	createResp.State.GetAttribute(ctx, path.Root("firewall_rules").AtListIndex(0).AtName("destination_ports"), &ports)
	if !ports.IsNull() {
		t.Fatalf("create: expected null destination_ports, got %s", ports)
	}

	updateResp := resource.UpdateResponse{State: emptyState()}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatal(updateResp.Diagnostics)
	}
	updateResp.State.GetAttribute(ctx, path.Root("firewall_rules").AtListIndex(0).AtName("destination_ports"), &ports)
	if !ports.IsNull() {
		t.Fatalf("update: expected null destination_ports, got %s", ports)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}
	readResp.State.GetAttribute(ctx, path.Root("firewall_rules").AtListIndex(0).AtName("destination_ports"), &ports)
	if !ports.IsNull() {
		t.Fatalf("read: expected null destination_ports, got %s", ports)
	}
}
