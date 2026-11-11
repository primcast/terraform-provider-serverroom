package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Every schema must pass the framework's own implementation checks, which is what Terraform would refuse at plan time.
func TestSchemasAreValid(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	var pr provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &pr)
	if pr.Diagnostics.HasError() {
		t.Fatalf("provider schema: %v", pr.Diagnostics)
	}

	names := map[string]bool{}
	for _, f := range p.Resources(ctx) {
		r := f()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "serverroom"}, &meta)
		if names[meta.TypeName] {
			t.Fatalf("duplicate %s", meta.TypeName)
		}
		names[meta.TypeName] = true
		var s resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &s)
		if s.Diagnostics.HasError() {
			t.Fatalf("%s: %v", meta.TypeName, s.Diagnostics)
		}
		if diags := s.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("%s: %v", meta.TypeName, diags)
		}
	}
	for _, f := range p.DataSources(ctx) {
		d := f()
		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "serverroom"}, &meta)
		var s datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &s)
		if diags := s.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Fatalf("%s: %v", meta.TypeName, diags)
		}
		names[meta.TypeName] = true
	}
	for _, want := range []string{"serverroom_webhook", "serverroom_monitor", "serverroom_port_alert", "serverroom_reverse_dns",
		"serverroom_server_os", "serverroom_account", "serverroom_servers", "serverroom_server", "serverroom_stock",
		"serverroom_plan_os"} {
		if !names[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestReinstallNeedsExplicitPermission(t *testing.T) {
	r := &serverOSResource{}
	plan := serverOSModel{}
	if err := r.reinstall(plan); err == nil {
		t.Fatal("a reinstall without allow_reinstall must be refused")
	}
}
