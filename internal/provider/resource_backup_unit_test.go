package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ahmedali6/terraform-provider-dokploy/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func backupTestPlan(t *testing.T, r *BackupResource) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	model := BackupResourceModel{
		ID: types.StringValue("backup-test"), DestinationID: types.StringValue("destination-test"),
		BackupType: types.StringValue("compose"), DatabaseType: types.StringValue("postgres"),
		ComposeID: types.StringValue("compose-test"), ServiceName: types.StringValue("db"),
		Schedule: types.StringValue("0 2 * * *"), Enabled: types.BoolValue(true),
		Prefix: types.StringValue("database"), Database: types.StringValue("usesend"),
		KeepLatestCount: types.Int64Value(30), Metadata: types.MapNull(types.StringType),
	}
	plan := tfsdk.Plan{Schema: sr.Schema}
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	return plan
}

func setBackupTestAttribute(t *testing.T, plan *tfsdk.Plan, name string, value any) {
	t.Helper()
	if d := plan.SetAttribute(context.Background(), path.Root(name), value); d.HasError() {
		t.Fatal(d)
	}
}

// Run the real resource and HTTP client against a local fake, including refresh.
func TestBackupComposeDatabaseUser(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []struct {
			name                        string
			user, legacy, emptyMetadata bool
		}{
			{name: "nested", user: true},
			{name: "nested_legacy", user: true, legacy: true},
			{name: "legacy_only", legacy: true},
			{name: "no_user"},
			{name: "empty_metadata", user: true, emptyMetadata: true},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				user := "postgres"
				if operation == "update" {
					user = "backup-role"
				}
				var saved map[string]any
				writes, reads := 0, 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch req.URL.Path {
					case "/backup." + operation:
						writes++
						if req.Method != http.MethodPost {
							t.Errorf("write method = %s", req.Method)
						}
						if err := json.NewDecoder(req.Body).Decode(&saved); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						want := make(map[string]any)
						if tc.user {
							want["postgres"] = map[string]any{"databaseUser": user}
						}
						if tc.legacy {
							want["label"] = "existing"
						}
						if !(operation == "create" && len(want) == 0 && saved["metadata"] == nil) && !reflect.DeepEqual(saved["metadata"], want) {
							t.Errorf("metadata = %#v, want %#v", saved["metadata"], want)
						}
						if saved["databaseType"] != "postgres" {
							t.Errorf("databaseType = %#v", saved["databaseType"])
						}
						if operation == "create" && saved["composeId"] != "compose-test" {
							t.Errorf("composeId = %#v", saved["composeId"])
						}
						// Dokploy's update endpoint may return an empty body.
						if operation == "update" {
							return
						}
					case "/backup.one":
						reads++
						if req.Method != http.MethodGet || req.URL.Query().Get("backupId") != "backup-test" {
							t.Errorf("unexpected read: %s %s", req.Method, req.URL)
						}
					default:
						t.Errorf("unexpected request: %s", req.URL.Path)
						w.WriteHeader(http.StatusNotFound)
						return
					}
					saved["backupId"] = "backup-test"
					saved["backupType"] = "compose"
					saved["composeId"] = "compose-test"
					_ = json.NewEncoder(w).Encode(saved)
				}))
				defer srv.Close()
				r := &BackupResource{client: client.NewDokployClient(srv.URL, "fake-key")}
				plan := backupTestPlan(t, r)
				if tc.user {
					setBackupTestAttribute(t, &plan, "database_user", types.StringValue(user))
				}
				if tc.legacy || tc.emptyMetadata {
					values := make(map[string]string)
					if tc.legacy {
						values["label"] = "existing"
					}
					metadata, d := types.MapValueFrom(ctx, types.StringType, values)
					if d.HasError() {
						t.Fatal(d)
					}
					setBackupTestAttribute(t, &plan, "metadata", metadata)
				}
				state := tfsdk.State{Schema: plan.Schema}
				if operation == "create" {
					resp := resource.CreateResponse{State: state}
					r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
					if resp.Diagnostics.HasError() {
						t.Fatal(resp.Diagnostics)
					}
					state = resp.State
				} else {
					state.Raw = plan.Raw
					if d := state.SetAttribute(ctx, path.Root("database_user"), types.StringValue("old-role")); d.HasError() {
						t.Fatal(d)
					}
					resp := resource.UpdateResponse{State: state}
					r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
					if resp.Diagnostics.HasError() {
						t.Fatal(resp.Diagnostics)
					}
					state = resp.State
				}
				resp := resource.ReadResponse{State: state}
				r.Read(ctx, resource.ReadRequest{State: state}, &resp)
				if resp.Diagnostics.HasError() {
					t.Fatal(resp.Diagnostics)
				}
				if !resp.State.Raw.Equal(plan.Raw) {
					t.Errorf("refresh changed the planned values: got %s, want %s", resp.State.Raw, plan.Raw)
				}
				if writes != 1 || reads < 1 {
					t.Errorf("writes=%d reads=%d", writes, reads)
				}
			})
		}
	}
}

func TestBackupReadMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, databaseType string
		wantUser                     types.String
		wantMetadata                 map[string]string
	}{
		{"nested", `{"postgres":{"databaseUser":"postgres"}}`, "postgres", types.StringValue("postgres"), nil},
		{"legacy", `{"label":"existing"}`, "postgres", types.StringNull(), map[string]string{"label": "existing"}},
		{"mixed", `{"postgres":{"databaseUser":"postgres"},"label":"existing","other":{"nested":true}}`, "postgres", types.StringValue("postgres"), map[string]string{"label": "existing"}},
		{"absent", `null`, "postgres", types.StringNull(), nil},
		{"empty", `{}`, "postgres", types.StringNull(), nil},
		{"null_user", `{"postgres":{"databaseUser":null}}`, "postgres", types.StringNull(), nil},
		{"other_engine", `{"mongo":{"databaseUser":"mongo-user","databasePassword":"fake"}}`, "mongo", types.StringValue("mongo-user"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/backup.one" {
					t.Errorf("unexpected request %s", req.URL.Path)
				}
				_, _ = w.Write([]byte(`{"backupId":"backup-test","backupType":"compose","databaseType":"` + tc.databaseType + `","metadata":` + tc.metadata + `}`))
			}))
			defer srv.Close()
			r := &BackupResource{client: client.NewDokployClient(srv.URL, "fake-key")}
			plan := backupTestPlan(t, r)
			setBackupTestAttribute(t, &plan, "database_user", types.StringValue("stale-user"))
			state := tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}
			resp := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var user types.String
			if d := resp.State.GetAttribute(context.Background(), path.Root("database_user"), &user); d.HasError() {
				t.Fatal(d)
			}
			if !user.Equal(tc.wantUser) {
				t.Errorf("database_user = %s, want %s", user, tc.wantUser)
			}
			var metadata types.Map
			if d := resp.State.GetAttribute(context.Background(), path.Root("metadata"), &metadata); d.HasError() {
				t.Fatal(d)
			}
			want := types.MapNull(types.StringType)
			if tc.wantMetadata != nil {
				want, _ = types.MapValueFrom(context.Background(), types.StringType, tc.wantMetadata)
			}
			if !metadata.Equal(want) {
				t.Errorf("metadata = %s, want %s", metadata, want)
			}
		})
	}
}

func TestBackupDatabaseUserValidation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		backupType   types.String
		databaseType types.String
		user         types.String
		wantError    bool
	}{
		{"compose", types.StringValue("compose"), types.StringValue("postgres"), types.StringValue("postgres"), false},
		{"default_engine", types.StringValue("compose"), types.StringNull(), types.StringValue("postgres"), false},
		{"database", types.StringValue("database"), types.StringValue("postgres"), types.StringValue("postgres"), true},
		{"default_backup", types.StringNull(), types.StringValue("postgres"), types.StringValue("postgres"), true},
		{"mysql", types.StringValue("compose"), types.StringValue("mysql"), types.StringValue("root"), true},
		{"mariadb", types.StringValue("compose"), types.StringValue("mariadb"), types.StringValue("root"), true},
		{"mongo", types.StringValue("compose"), types.StringValue("mongo"), types.StringValue("root"), true},
		{"metadata_collision", types.StringValue("compose"), types.StringValue("postgres"), types.StringValue("postgres"), true},
		{"empty_user", types.StringValue("compose"), types.StringValue("postgres"), types.StringValue("  "), true},
		{"unknown_backup", types.StringUnknown(), types.StringValue("postgres"), types.StringValue("postgres"), false},
		{"unknown_engine", types.StringValue("compose"), types.StringUnknown(), types.StringValue("postgres"), false},
		{"unknown_user", types.StringValue("compose"), types.StringValue("postgres"), types.StringUnknown(), false},
		{"legacy_unset", types.StringValue("database"), types.StringValue("postgres"), types.StringNull(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &BackupResource{}
			plan := backupTestPlan(t, r)
			setBackupTestAttribute(t, &plan, "backup_type", tc.backupType)
			setBackupTestAttribute(t, &plan, "database_type", tc.databaseType)
			setBackupTestAttribute(t, &plan, "database_user", tc.user)
			if tc.name == "metadata_collision" {
				metadata, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{"postgres": "legacy"})
				setBackupTestAttribute(t, &plan, "metadata", metadata)
			}
			validator, ok := any(r).(resource.ResourceWithValidateConfig)
			if !ok {
				t.Fatal("backup resource must validate database_user before apply")
			}
			var resp resource.ValidateConfigResponse
			validator.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: plan.Schema, Raw: plan.Raw}}, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Errorf("validation diagnostics = %v, wantError %t", resp.Diagnostics, tc.wantError)
			}
		})
	}
}
