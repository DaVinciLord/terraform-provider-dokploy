package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahmedali6/terraform-provider-dokploy/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const traefikDynamicDir = "/etc/dokploy/traefik/dynamic/"
const removedTraefikFile = "# Removed by Terraform (dokploy_traefik_file). Delete this file on the host.\n"

var _ resource.Resource = &TraefikFileResource{}
var _ resource.ResourceWithConfigure = &TraefikFileResource{}
var _ resource.ResourceWithImportState = &TraefikFileResource{}

func NewTraefikFileResource() resource.Resource {
	return &TraefikFileResource{}
}

type TraefikFileResource struct {
	client *client.DokployClient
}

type traefikFileModel struct {
	ID       types.String `tfsdk:"id"`
	Path     types.String `tfsdk:"path"`
	Content  types.String `tfsdk:"content"`
	ServerID types.String `tfsdk:"server_id"`
}

func (r *TraefikFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_traefik_file"
}

func (r *TraefikFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Traefik dynamic configuration file. Destroy replaces the file with a comment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The file path, prefixed with the server ID and a colon when a server ID is set.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"path": schema.StringAttribute{
				Required:      true,
				Description:   "Absolute path of a YAML file directly inside /etc/dokploy/traefik/dynamic/.",
				Validators:    []validator.String{traefikFilePathValidator{}},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"content": schema.StringAttribute{
				Required:    true,
				Description: "The exact, non-empty file contents.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"server_id": schema.StringAttribute{
				Optional:      true,
				Description:   "The Dokploy server ID. Omit it to use the local server.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

type traefikFilePathValidator struct{}

func (traefikFilePathValidator) Description(context.Context) string {
	return "Must be a direct .yml or .yaml file under /etc/dokploy/traefik/dynamic/, without '..'. Dokploy owns dokploy.yml and middlewares.yml."
}

func (v traefikFilePathValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v traefikFilePathValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validTraefikFilePath(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Traefik file path", v.Description(ctx))
	}
}

func validTraefikFilePath(filePath string) bool {
	name, ok := strings.CutPrefix(filePath, traefikDynamicDir)
	return ok && !strings.ContainsAny(name, "/\x00") && !strings.Contains(name, "..") &&
		name != "dokploy.yml" && name != "middlewares.yml" &&
		(strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

func (r *TraefikFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.DokployClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.DokployClient, got %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *TraefikFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan traefikFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filePath, serverID := plan.Path.ValueString(), plan.ServerID.ValueString()
	id := filePath
	if serverID != "" {
		id = serverID + ":" + filePath
	}

	_, err := r.client.ReadTraefikFile(filePath, serverID)
	switch {
	case err == nil:
		resp.Diagnostics.AddError("Traefik file already exists", fmt.Sprintf("Import the existing file with ID %q before managing it with Terraform.", id))
		return
	case !errors.Is(err, client.ErrNotFound):
		resp.Diagnostics.AddError("Error checking Traefik file", err.Error())
		return
	}

	if err := r.client.UpdateTraefikFile(filePath, serverID, plan.Content.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error creating Traefik file", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TraefikFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state traefikFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content, err := r.client.ReadTraefikFile(state.Path.ValueString(), state.ServerID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading Traefik file", err.Error())
		return
	}
	state.Content = types.StringValue(content)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *TraefikFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan traefikFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateTraefikFile(plan.Path.ValueString(), plan.ServerID.ValueString(), plan.Content.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating Traefik file", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TraefikFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state traefikFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateTraefikFile(state.Path.ValueString(), state.ServerID.ValueString(), removedTraefikFile); err != nil {
		resp.Diagnostics.AddError("Error clearing Traefik file", err.Error())
	}
}

func (r *TraefikFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	filePath := req.ID
	server := types.StringNull()
	if !strings.HasPrefix(req.ID, "/") {
		serverID, pathPart, ok := strings.Cut(req.ID, ":")
		if ok && serverID != "" && !strings.Contains(serverID, "/") {
			server = types.StringValue(serverID)
			filePath = pathPart
		}
	}
	if !validTraefikFilePath(filePath) {
		resp.Diagnostics.AddError("Invalid Traefik file import ID", "Expected an absolute Traefik file path or server_id:path.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &traefikFileModel{
		ID:       types.StringValue(req.ID),
		Path:     types.StringValue(filePath),
		ServerID: server,
	})...)
}
