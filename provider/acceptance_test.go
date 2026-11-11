package provider

// Acceptance tests: real API, real Terraform. They run only with TF_ACC=1 and need
//
//	SERVERROOM_API_KEY          a key on a TEST account (servers, alerts, webhooks: read+write)
//	SERVERROOM_TEST_ACCOUNT_ID  a dedicated server on that account
//	SERVERROOM_TEST_REINSTALL   set to an OS name ("Debian 12") to also reinstall the server -- ERASES IT
//
//	TF_ACC=1 go test ./provider -run TestAcc -v -timeout 120m

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var protoV6 = map[string]func() (tfprotov6.ProviderServer, error){
	"serverroom": providerserver.NewProtocol6WithError(New("test")()),
}

func preCheck(t *testing.T) string {
	t.Helper()
	if os.Getenv("SERVERROOM_API_KEY") == "" {
		t.Skip("SERVERROOM_API_KEY not set")
	}
	id := os.Getenv("SERVERROOM_TEST_ACCOUNT_ID")
	if id == "" {
		t.Skip("SERVERROOM_TEST_ACCOUNT_ID not set")
	}
	return id
}

func TestAccCatalogue(t *testing.T) {
	if os.Getenv("SERVERROOM_API_KEY") == "" {
		t.Skip("SERVERROOM_API_KEY not set")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6,
		Steps: []resource.TestStep{{
			Config: `
data "serverroom_account" "me" {}
data "serverroom_stock" "cheap" {
  max_price = 100
  limit     = 3
}
data "serverroom_plan_os" "m710x" {
  plan = "M710x"
}`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.serverroom_account.me", "uid"),
				resource.TestMatchResourceAttr("data.serverroom_stock.cheap", "servers.0.order_url", regexp.MustCompile(`^https://.+/order/`)),
				resource.TestCheckResourceAttrSet("data.serverroom_plan_os.m710x", "names.0"),
			),
		}},
	})
}

func TestAccServerMonitorWebhook(t *testing.T) {
	id := preCheck(t)
	config := func(name string) string {
		return fmt.Sprintf(`
data "serverroom_server" "s" {
  account_id = %s
}
resource "serverroom_server_os" "s" {
  account_id = data.serverroom_server.s.account_id
  os         = data.serverroom_server.s.os
}
resource "serverroom_monitor" "ping" {
  account_id = data.serverroom_server.s.account_id
  name       = %q
  type       = "ping"
  target     = data.serverroom_server.s.ipv4[0]
}
resource "serverroom_webhook" "w" {
  url    = "https://example.com/acc-test"
  events = ["*"]
  active = false
}`, id, name)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6,
		Steps: []resource.TestStep{
			{
				Config: config("acc ping"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.serverroom_server.s", "server_id"),
					resource.TestCheckResourceAttrSet("serverroom_monitor.ping", "id"),
					resource.TestCheckResourceAttrSet("serverroom_webhook.w", "secret"),
					// adopting the running OS must not reinstall
					resource.TestCheckResourceAttrPair("serverroom_server_os.s", "os", "data.serverroom_server.s", "os"),
				),
			},
			{Config: config("acc ping renamed"),
				Check: resource.TestCheckResourceAttr("serverroom_monitor.ping", "name", "acc ping renamed")},
			{ResourceName: "serverroom_monitor.ping", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"state"}},
		},
	})
}

func TestAccReinstall(t *testing.T) {
	id := preCheck(t)
	osName := os.Getenv("SERVERROOM_TEST_REINSTALL")
	if osName == "" {
		t.Skip("SERVERROOM_TEST_REINSTALL not set (this test erases the server)")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6,
		Steps: []resource.TestStep{
			{
				// without allow_reinstall a different OS is refused
				Config: fmt.Sprintf(`
resource "serverroom_server_os" "s" {
  account_id = %s
  os         = %q
}`, id, osName+" (refused)"),
				ExpectError: regexp.MustCompile(`allow_reinstall`),
			},
			{
				Config: fmt.Sprintf(`
resource "serverroom_server_os" "s" {
  account_id      = %s
  os              = %q
  allow_reinstall = true
  wait            = true
}`, id, osName),
				Check: resource.TestCheckResourceAttr("serverroom_server_os.s", "os", osName),
			},
		},
	})
}
