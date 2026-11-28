package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ahmedali6/terraform-provider-dokploy/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTraefikFilePathValidation(t *testing.T) {
	for _, tc := range []struct {
		path  string
		valid bool
	}{
		{traefikDynamicDir + "route.yaml", true},
		{traefikDynamicDir + "route.yml", true},
		{"relative.yaml", false},
		{"/somewhere/route.yaml", false},
		{traefikDynamicDir + "sub/route.yaml", false},
		{traefikDynamicDir + "../route.yaml", false},
		{traefikDynamicDir + "route..yaml", false},
		{traefikDynamicDir + "route.json", false},
		{traefikDynamicDir + "dokploy.yml", false},
		{traefikDynamicDir + "middlewares.yml", false},
		{traefikDynamicDir, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var resp validator.StringResponse
			traefikFilePathValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path: path.Root("path"), ConfigValue: types.StringValue(tc.path),
			}, &resp)
			if resp.Diagnostics.HasError() == tc.valid {
				t.Fatalf("expected valid=%v, got %v", tc.valid, resp.Diagnostics)
			}
		})
	}
}

func TestTraefikFileLifecycle(t *testing.T) {
	const filePath = traefikDynamicDir + "example & route.yaml"
	const content = "http: {} \r\n\n"
	const updated = "http: {routers: {}}"
	for _, serverID := range []string{"", "server-1"} {
		t.Run("server="+serverID, func(t *testing.T) {
			ctx := context.Background()
			var remoteContent *string
			var rejectRead bool
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls = append(calls, req.Method+" "+req.URL.Path)
				switch req.URL.Path {
				case "/settings.readTraefikFile":
					query := req.URL.Query()
					if query.Get("path") != filePath || query.Get("serverId") != serverID || query.Has("serverId") != (serverID != "") {
						t.Errorf("unexpected query: %s", req.URL.RawQuery)
					}
					if rejectRead {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_ = json.NewEncoder(w).Encode(remoteContent)
				case "/settings.updateTraefikFile":
					var body map[string]string
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					want := map[string]string{"path": filePath, "traefikConfig": body["traefikConfig"]}
					if serverID != "" {
						want["serverId"] = serverID
					}
					if !reflect.DeepEqual(body, want) {
						t.Errorf("payload = %#v, want %#v", body, want)
					}
					value := body["traefikConfig"]
					remoteContent = &value
				default:
					t.Errorf("unexpected endpoint: %s", req.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			r := &TraefikFileResource{client: client.NewDokployClient(server.URL, "fake-key")}
			var sr resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &sr)
			plan := tfsdk.Plan{Schema: sr.Schema}
			model := traefikFileModel{Path: types.StringValue(filePath), Content: types.StringValue(content)}
			wantID := filePath
			if serverID != "" {
				model.ServerID = types.StringValue(serverID)
				wantID = serverID + ":" + filePath
			}
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			created := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			if d := created.State.Get(ctx, &model); d.HasError() || model.ID.ValueString() != wantID || remoteContent == nil || *remoteContent != content {
				t.Fatalf("create state=%+v, remote=%v, diagnostics=%v", model, remoteContent, d)
			}
			duplicate := resource.CreateResponse{}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &duplicate)
			if !duplicate.Diagnostics.HasError() || !strings.Contains(duplicate.Diagnostics.Errors()[0].Detail(), "Import") {
				t.Fatalf("expected import instruction, got %v", duplicate.Diagnostics)
			}

			drift := content + "# changed remotely "
			remoteContent = &drift
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if d := read.State.Get(ctx, &model); read.Diagnostics.HasError() || d.HasError() || model.Content.ValueString() != drift {
				t.Fatalf("read state=%+v, diagnostics=%v %v", model, read.Diagnostics, d)
			}

			model.Content = types.StringValue(updated)
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			update := resource.UpdateResponse{State: read.State}
			r.Update(ctx, resource.UpdateRequest{Plan: plan, State: read.State}, &update)
			if update.Diagnostics.HasError() || *remoteContent != updated || !update.State.Raw.Equal(plan.Raw) {
				t.Fatalf("update remote=%q, diagnostics=%v", *remoteContent, update.Diagnostics)
			}

			deleted := resource.DeleteResponse{State: update.State}
			r.Delete(ctx, resource.DeleteRequest{State: update.State}, &deleted)
			const tombstone = "# Removed by Terraform (dokploy_traefik_file). Delete this file on the host.\n"
			if deleted.Diagnostics.HasError() || *remoteContent != tombstone {
				t.Fatalf("delete remote=%q, diagnostics=%v", *remoteContent, deleted.Diagnostics)
			}

			recreated := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &recreated)
			if recreated.Diagnostics.HasError() || *remoteContent != updated || !recreated.State.Raw.Equal(update.State.Raw) {
				t.Fatalf("recreate remote=%q, diagnostics=%v", *remoteContent, recreated.Diagnostics)
			}

			similar := strings.TrimSuffix(tombstone, "\n")
			remoteContent = &similar
			duplicate = resource.CreateResponse{}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &duplicate)
			if !duplicate.Diagnostics.HasError() || !strings.Contains(duplicate.Diagnostics.Errors()[0].Detail(), "Import") {
				t.Fatalf("expected import instruction for a non-exact placeholder, got %v", duplicate.Diagnostics)
			}

			remoteContent = nil
			missing := resource.ReadResponse{State: update.State}
			r.Read(ctx, resource.ReadRequest{State: update.State}, &missing)
			if missing.Diagnostics.HasError() || !missing.State.Raw.IsNull() {
				t.Fatalf("expected state removal, got %v, diagnostics=%v", missing.State.Raw, missing.Diagnostics)
			}

			deleteMissing := resource.DeleteResponse{State: update.State}
			r.Delete(ctx, resource.DeleteRequest{State: update.State}, &deleteMissing)
			if deleteMissing.Diagnostics.HasError() || remoteContent != nil {
				t.Fatalf("delete missing remote=%v, diagnostics=%v", remoteContent, deleteMissing.Diagnostics)
			}

			rejectRead = true
			deleteDenied := resource.DeleteResponse{State: update.State}
			r.Delete(ctx, resource.DeleteRequest{State: update.State}, &deleteDenied)
			if !deleteDenied.Diagnostics.HasError() {
				t.Fatal("expected a diagnostic when the read failed")
			}

			const get = "GET /settings.readTraefikFile"
			const post = "POST /settings.updateTraefikFile"
			want := []string{
				get, post, // Create.
				get,       // Reject an existing file.
				get, post, // Read and update.
				get, post, // Delete.
				get, post, // Create over the exact removal comment.
				get,      // Reject a non-exact removal comment.
				get, get, // Read and delete when missing.
				get, // Delete when the read fails.
			}
			if !reflect.DeepEqual(calls, want) {
				t.Errorf("requests = %v, want %v", calls, want)
			}
		})
	}
}

func TestTraefikFileImport(t *testing.T) {
	const filePath = traefikDynamicDir + "example.yaml"
	ctx := context.Background()
	r := &TraefikFileResource{}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	for _, tc := range []struct {
		id     string
		server types.String
		valid  bool
	}{
		{filePath, types.StringNull(), true},
		{"server-1:" + filePath, types.StringValue("server-1"), true},
		{"server-1", types.StringNull(), false},
		{":" + filePath, types.StringNull(), false},
		{"server-1:/outside.yaml", types.StringNull(), false},
		{traefikDynamicDir + "dokploy.yml", types.StringNull(), false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			resp := resource.ImportStateResponse{State: tfsdk.State{Schema: sr.Schema}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, &resp)
			if resp.Diagnostics.HasError() == tc.valid {
				t.Fatalf("expected valid=%v, got %v", tc.valid, resp.Diagnostics)
			}
			if !tc.valid {
				return
			}
			var state traefikFileModel
			if d := resp.State.Get(ctx, &state); d.HasError() {
				t.Fatal(d)
			}
			if state.Path.ValueString() != filePath || state.ID.ValueString() != tc.id || !state.ServerID.Equal(tc.server) {
				t.Fatalf("unexpected imported state: %+v", state)
			}
		})
	}
}
