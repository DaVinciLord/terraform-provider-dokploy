package provider

import (
	"context"
	"encoding/json"
	"github.com/ahmedali6/terraform-provider-dokploy/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// Exercise the resource lifecycle against a fake API, never a real Dokploy.
func TestEnvironmentVariablesRedeploy(t *testing.T) {
	for _, tc := range []struct {
		name                                                 string
		update, enabled, saveFails, deployFails, application bool
	}{
		{name: "create_disabled"},
		{name: "application_create", application: true, enabled: true},
		{name: "application_update", application: true, update: true, enabled: true}, {name: "create_enabled", enabled: true},
		{name: "update_enabled", update: true, enabled: true}, {name: "update_disabled", update: true},
		{name: "save_failure", enabled: true, saveFails: true}, {name: "deploy_failure", enabled: true, deployFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			env := "KEY=old"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.URL.Path {
				case "/compose.one", "/application.one":
					json.NewEncoder(w).Encode(map[string]string{"composeId": "compose-test", "env": env})
				case "/compose.saveEnvironment", "/application.saveEnvironment":
					calls = append(calls, "save")
					if tc.saveFails {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"message":"save failed"}`))
						return
					}
					var body struct {
						Env string `json:"env"`
					}
					json.NewDecoder(req.Body).Decode(&body)
					env = body.Env
					w.Write([]byte(`{}`))
				case "/compose.deploy", "/application.deploy":
					calls = append(calls, "deploy")
					var body struct {
						ComposeID     string `json:"composeId"`
						ApplicationID string `json:"applicationId"`
					}
					json.NewDecoder(req.Body).Decode(&body)
					if tc.application && body.ApplicationID != "application-test" {
						t.Errorf("wrong application target %q", body.ApplicationID)
					}
					if !tc.application && body.ComposeID != "compose-test" {
						t.Errorf("wrong target %q", body.ComposeID)
					}
					if env != "KEY=new" {
						t.Errorf("deployment before variables were saved")
					}
					if tc.deployFails {
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"message":"deploy failed"}`))
						return
					}
					w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected request %s", req.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			ctx := context.Background()
			r := &EnvironmentVariablesResource{client: client.NewDokployClient(srv.URL, "fake-key")}
			sr := resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, &sr)
			vars, d := types.MapValueFrom(ctx, types.StringType, map[string]string{"KEY": "new"})
			if d.HasError() {
				t.Fatal(d)
			}
			model := EnvironmentVariablesResourceModel{ID: types.StringNull(), ApplicationID: types.StringNull(), ComposeID: types.StringValue("compose-test"), Variables: vars, CreateEnvFile: types.BoolValue(true), RedeployOnUpdate: types.BoolNull()}
			if tc.application {
				model.ApplicationID = types.StringValue("application-test")
				model.ComposeID = types.StringNull()
			}
			if tc.enabled {
				model.RedeployOnUpdate = types.BoolValue(true)
			}
			plan := tfsdk.Plan{Schema: sr.Schema}
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			hasError := false
			if tc.update {
				old := model
				old.ID = types.StringValue("compose-test")
				old.Variables, _ = types.MapValueFrom(ctx, types.StringType, map[string]string{"KEY": "old"})
				state := tfsdk.State{Schema: sr.Schema}
				if d := state.Set(ctx, &old); d.HasError() {
					t.Fatal(d)
				}
				resp := resource.UpdateResponse{State: state}
				r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
				hasError = resp.Diagnostics.HasError()
			} else {
				resp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
				r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
				hasError = resp.Diagnostics.HasError()
			}
			if hasError != (tc.saveFails || tc.deployFails) {
				t.Fatalf("unexpected error status %v", hasError)
			}
			if tc.saveFails {
				for _, c := range calls {
					if c == "deploy" {
						t.Fatal("deploy after failed save")
					}
				}
				return
			}
			expected := []string{"save"}
			if tc.enabled {
				expected = append(expected, "deploy")
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("calls %v want %v", calls, expected)
			}
		})
	}
}
