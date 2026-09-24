package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/ahmedali6/terraform-provider-dokploy/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &EnvironmentVariablesResource{}
var _ resource.ResourceWithImportState = &EnvironmentVariablesResource{}

func NewEnvironmentVariablesResource() resource.Resource {
	return &EnvironmentVariablesResource{}
}

type EnvironmentVariablesResource struct {
	client *client.DokployClient
}

type EnvironmentVariablesResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	ComposeID     types.String `tfsdk:"compose_id"`
	Variables     types.Map    `tfsdk:"variables"`
	CreateEnvFile types.Bool   `tfsdk:"create_env_file"`
}

func (r *EnvironmentVariablesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_variables"
}

func (r *EnvironmentVariablesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages all environment variables for a Dokploy application or compose stack as a single resource. " +
			"Exactly one of application_id or compose_id must be set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"application_id": schema.StringAttribute{
				Optional:    true,
				Description: "Application ID. Mutually exclusive with compose_id.",
			},
			"compose_id": schema.StringAttribute{
				Optional:    true,
				Description: "Compose stack ID. Mutually exclusive with application_id.",
			},
			"variables": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Sensitive:   true,
			},
			"create_env_file": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *EnvironmentVariablesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*client.DokployClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Type", fmt.Sprintf("Expected *client.DokployClient, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (m EnvironmentVariablesResourceModel) targetID() (kind, id string, err error) {
	appSet := !m.ApplicationID.IsNull() && !m.ApplicationID.IsUnknown() && m.ApplicationID.ValueString() != ""
	composeSet := !m.ComposeID.IsNull() && !m.ComposeID.IsUnknown() && m.ComposeID.ValueString() != ""
	switch {
	case appSet && composeSet:
		return "", "", fmt.Errorf("exactly one of application_id or compose_id must be set, not both")
	case appSet:
		return "application", m.ApplicationID.ValueString(), nil
	case composeSet:
		return "compose", m.ComposeID.ValueString(), nil
	default:
		return "", "", fmt.Errorf("exactly one of application_id or compose_id must be set")
	}
}

func (r *EnvironmentVariablesResource) updateEnv(kind, id string, updateFn func(map[string]string), createEnvFile *bool) error {
	switch kind {
	case "application":
		return r.client.UpdateApplicationEnv(id, updateFn, createEnvFile)
	case "compose":
		return r.client.UpdateComposeEnv(id, updateFn, createEnvFile)
	default:
		return fmt.Errorf("unknown environment variables target kind %q", kind)
	}
}

func (r *EnvironmentVariablesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EnvironmentVariablesResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, targetID, err := plan.targetID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	envMap := make(map[string]string)
	diags = plan.Variables.ElementsAs(ctx, &envMap, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.updateEnv(kind, targetID, func(m map[string]string) {
		for k, v := range envMap {
			m[k] = v
		}
	}, plan.CreateEnvFile.ValueBoolPointer())

	if err != nil {
		resp.Diagnostics.AddError("Error creating environment variables", err.Error())
		return
	}

	plan.ID = types.StringValue(targetID)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *EnvironmentVariablesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EnvironmentVariablesResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, targetID, err := state.targetID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid state", err.Error())
		return
	}

	var envStr string
	switch kind {
	case "application":
		app, err := r.client.GetApplication(targetID)
		if err != nil {
			if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Error reading application", err.Error())
			return
		}
		envStr = app.Env
	case "compose":
		comp, err := r.client.GetCompose(targetID)
		if err != nil {
			if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Error reading compose", err.Error())
			return
		}
		envStr = comp.Env
	}

	envMap := client.ParseEnv(envStr)
	state.Variables, diags = types.MapValueFrom(ctx, types.StringType, envMap)
	resp.Diagnostics.Append(diags...)
	state.ID = types.StringValue(targetID)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *EnvironmentVariablesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EnvironmentVariablesResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, targetID, err := plan.targetID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	envMap := make(map[string]string)
	diags = plan.Variables.ElementsAs(ctx, &envMap, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.updateEnv(kind, targetID, func(m map[string]string) {
		for k := range m {
			delete(m, k)
		}
		for k, v := range envMap {
			m[k] = v
		}
	}, plan.CreateEnvFile.ValueBoolPointer())

	if err != nil {
		resp.Diagnostics.AddError("Error updating environment variables", err.Error())
		return
	}

	plan.ID = types.StringValue(targetID)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *EnvironmentVariablesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EnvironmentVariablesResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	kind, targetID, err := state.targetID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid state", err.Error())
		return
	}

	err = r.updateEnv(kind, targetID, func(m map[string]string) {
		for k := range m {
			delete(m, k)
		}
	}, state.CreateEnvFile.ValueBoolPointer())

	if err != nil {
		if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
			return
		}
		resp.Diagnostics.AddError("Error deleting environment variables", err.Error())
		return
	}
}

func (r *EnvironmentVariablesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID formats:
	//   application: <application-id>
	//   compose:     compose:<compose-id>
	id := req.ID
	if strings.HasPrefix(id, "compose:") {
		composeID := strings.TrimPrefix(id, "compose:")
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), composeID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("compose_id"), composeID)...)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_id"), id)...)
}
