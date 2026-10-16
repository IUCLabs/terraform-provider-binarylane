package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"terraform-provider-binarylane/internal/binarylane"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &serverReverseNameResource{}
	_ resource.ResourceWithConfigure   = &serverReverseNameResource{}
	_ resource.ResourceWithImportState = &serverReverseNameResource{}
)

func NewServerReverseNameResource() resource.Resource {
	return &serverReverseNameResource{}
}

type serverReverseNameResource struct {
	bc *BinarylaneClient
}

type serverReverseNameResourceModel struct {
	Id          types.String `tfsdk:"id"`
	ServerId    types.Int64  `tfsdk:"server_id"`
	Ipv4Address types.String `tfsdk:"ipv4_address"`
	ReverseName types.String `tfsdk:"reverse_name"`
}

func (r *serverReverseNameResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	bc, ok := req.ProviderData.(BinarylaneClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *BinarylaneClient, got: %T.", req.ProviderData),
		)
		return
	}
	r.bc = &bc
}

func (r *serverReverseNameResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_reverse_name"
}

func (r *serverReverseNameResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage the reverse name (PTR record) for a public IPv4 address of a BinaryLane server. " +
			"Destroying this resource clears the custom reverse name, restoring the BinaryLane default.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource, in the format `<server_id>:<ipv4_address>`.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server_id": schema.Int64Attribute{
				Description: "The ID of the server the IPv4 address is assigned to.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"ipv4_address": schema.StringAttribute{
				Description: "The public IPv4 address to set the reverse name for.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"reverse_name": schema.StringAttribute{
				Description: "The reverse name (PTR record) for the IPv4 address.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (r *serverReverseNameResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data serverReverseNameResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create API call logic
	tflog.Debug(ctx, fmt.Sprintf("Creating server reverse name: server_id=%d, ipv4_address=%s",
		data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
	err := r.changeReverseName(ctx, data.ServerId.ValueInt64(), data.Ipv4Address.ValueString(), data.ReverseName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating server reverse name: server_id=%d, ipv4_address=%s", data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()),
			err.Error(),
		)
		return
	}

	data.Id = types.StringValue(formatServerReverseNameId(data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverReverseNameResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data serverReverseNameResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read API call logic
	tflog.Debug(ctx, fmt.Sprintf("Reading server reverse name: server_id=%d, ipv4_address=%s",
		data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
	serverResp, err := r.bc.client.GetServersServerIdWithResponse(ctx, data.ServerId.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading server reverse name: server_id=%d", data.ServerId.ValueInt64()),
			err.Error(),
		)
		return
	}
	if serverResp.StatusCode() == http.StatusNotFound {
		tflog.Warn(ctx, fmt.Sprintf("Server not found, removing reverse name from state: server_id=%d", data.ServerId.ValueInt64()))
		resp.State.RemoveResource(ctx)
		return
	}
	if serverResp.StatusCode() != http.StatusOK {
		resp.Diagnostics.AddError(
			"Unexpected HTTP status code reading server reverse name",
			fmt.Sprintf("Received %s reading server reverse name: server_id=%d. Details: %s", serverResp.Status(), data.ServerId.ValueInt64(), serverResp.Body))
		return
	}

	var network *binarylane.Network
	for i, n := range serverResp.JSON200.Server.Networks.V4 {
		if n.IpAddress == data.Ipv4Address.ValueString() {
			network = &serverResp.JSON200.Server.Networks.V4[i]
			break
		}
	}
	if network == nil {
		tflog.Warn(ctx, fmt.Sprintf("IPv4 address not found on server, removing reverse name from state: server_id=%d, ipv4_address=%s",
			data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	data.Id = types.StringValue(formatServerReverseNameId(data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
	data.ReverseName = types.StringPointerValue(network.ReverseName)

	// Save into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverReverseNameResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data serverReverseNameResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update API call logic
	tflog.Debug(ctx, fmt.Sprintf("Updating server reverse name: server_id=%d, ipv4_address=%s",
		data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
	err := r.changeReverseName(ctx, data.ServerId.ValueInt64(), data.Ipv4Address.ValueString(), data.ReverseName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating server reverse name: server_id=%d, ipv4_address=%s", data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()),
			err.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverReverseNameResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serverReverseNameResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete API call logic: a null reverse name clears the custom reverse name
	tflog.Debug(ctx, fmt.Sprintf("Deleting server reverse name: server_id=%d, ipv4_address=%s",
		data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()))
	err := r.changeReverseName(ctx, data.ServerId.ValueInt64(), data.Ipv4Address.ValueString(), nil)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting server reverse name: server_id=%d, ipv4_address=%s", data.ServerId.ValueInt64(), data.Ipv4Address.ValueString()),
			err.Error(),
		)
		return
	}
}

func (r *serverReverseNameResource) ImportState(
	ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse,
) {
	serverId, ipv4Address, err := parseServerReverseNameId(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing server reverse name",
			"Could not import server reverse name, unexpected error (ID should be <server_id>:<ipv4_address>): "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), serverId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ipv4_address"), ipv4Address)...)
}

func (r *serverReverseNameResource) changeReverseName(ctx context.Context, serverId int64, ipv4Address string, reverseName *string) error {
	actionResp, err := r.bc.client.PostServersServerIdActionsChangeReverseNameWithResponse(
		ctx,
		serverId,
		binarylane.PostServersServerIdActionsChangeReverseNameJSONRequestBody{
			Type:        "change_reverse_name",
			Ipv4Address: ipv4Address,
			ReverseName: reverseName,
		},
	)
	if err != nil {
		return fmt.Errorf("error changing reverse name for server: server_id=%d, error: %w", serverId, err)
	}
	if actionResp.StatusCode() != http.StatusOK && actionResp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("unexpected HTTP status code changing reverse name for server: server_id=%d, details: %s", serverId, actionResp.Body)
	}
	// The change_reverse_name action can stay unresolvable, so wait for the result on the server instead
	if reverseName == nil {
		return nil
	}
	return r.waitForReverseName(ctx, serverId, ipv4Address, *reverseName)
}

func (r *serverReverseNameResource) waitForReverseName(ctx context.Context, serverId int64, ipv4Address string, want string) error {
	ctx, cancel := context.WithTimeout(ctx, serverActionTimeout)
	defer cancel()

	var last string
	for {
		serverResp, err := r.bc.client.GetServersServerIdWithResponse(ctx, serverId)
		if err == nil && serverResp.StatusCode() == http.StatusOK {
			for _, n := range serverResp.JSON200.Server.Networks.V4 {
				if n.IpAddress != ipv4Address {
					continue
				}
				if n.ReverseName != nil {
					if *n.ReverseName == want {
						return nil
					}
					last = *n.ReverseName
				}
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for reverse name: server_id=%d, ipv4_address=%s, want=%q, last=%q", serverId, ipv4Address, want, last)
		case <-time.After(serverActionPollInterval):
		}
	}
}

func formatServerReverseNameId(serverId int64, ipv4Address string) string {
	return fmt.Sprintf("%d:%s", serverId, ipv4Address)
}

func parseServerReverseNameId(id string) (int64, string, error) {
	serverIdStr, ipv4Address, ok := strings.Cut(id, ":")
	if !ok {
		return 0, "", fmt.Errorf("missing ':' separator in %q", id)
	}
	serverId, err := strconv.ParseInt(serverIdStr, 10, 64)
	if err != nil || serverId < 1 {
		return 0, "", fmt.Errorf("invalid server_id %q", serverIdStr)
	}
	addr, err := netip.ParseAddr(ipv4Address)
	if err != nil || !addr.Is4() {
		return 0, "", fmt.Errorf("invalid ipv4_address %q", ipv4Address)
	}
	return serverId, ipv4Address, nil
}
