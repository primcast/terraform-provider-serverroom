# terraform-provider-serverroom

The Terraform provider for your dedicated servers (the `serverroom` CLI is in
[primcast/serverroom-cli](https://github.com/primcast/serverroom-cli)). Both talk to the API of
the control panel you already use, with an API key from **API & Keys**. The key decides which account, and which
website's API, they use, so there is usually nothing else to configure. If asked for an API URL, give your
control panel website followed by `/api` (`api_url` in Terraform, `--api-url` or `SERVERROOM_API_URL` for the CLI).

- Terraform Registry: [`primcast/serverroom`](https://registry.terraform.io/providers/primcast/serverroom/latest)
- Docs: [docs/](docs/index.md) (also on the Registry)
- CLI: [primcast/serverroom-cli](https://github.com/primcast/serverroom-cli)

## Terraform

```hcl
terraform {
  required_providers {
    serverroom = {
      source  = "primcast/serverroom"
      version = "~> 0.1"
    }
  }
}

provider "serverroom" {} # key from SERVERROOM_API_KEY

data "serverroom_server" "web" {
  account_id = 246447
}

resource "serverroom_monitor" "ping" {
  account_id = data.serverroom_server.web.account_id
  name       = "web ping"
  type       = "ping"
  target     = data.serverroom_server.web.ipv4[0]
}
```

| Resource | What it manages |
|---|---|
| `serverroom_server_os` | the operating system (changing it **reinstalls and erases** the server, only with `allow_reinstall = true`; `wait` follows the install) |
| `serverroom_reverse_dns` | the PTR name of one of your addresses |
| `serverroom_monitor` | a website / TCP / ping check |
| `serverroom_port_alert` | alert when a server's port stays busy |
| `serverroom_webhook` | an endpoint that receives your account's events, signed |

| Data source | |
|---|---|
| `serverroom_account` | the key's customer, services and permissions |
| `serverroom_servers` | every service with plan, name and status |
| `serverroom_server` | one server: OS, location, port speed, features, IPv4 addresses |
| `serverroom_stock` | servers in stock now, with prices and order links |
| `serverroom_plan_os` | the operating systems a plan installs |

A key can never order, pay for or cancel anything, so servers are referenced by their service id, not created.
More examples: [examples/](examples/), and a whole server in [examples/complete](examples/complete/main.tf).

## CLI

The `serverroom` command-line tool uses the same API client and lives in
[primcast/serverroom-cli](https://github.com/primcast/serverroom-cli) (downloads on its Releases page, or
`go install github.com/primcast/serverroom-cli@latest`).

## Developing

```sh
go test ./...                                      # unit tests, no network
TF_ACC=1 SERVERROOM_API_KEY=... SERVERROOM_TEST_ACCOUNT_ID=... go test ./provider -run TestAcc -v
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name serverroom
```

`SERVERROOM_TEST_REINSTALL="Debian 12"` adds a real reinstall to the acceptance run. **It erases the test server.**

Releases: push a `v*` tag; [.github/workflows/release.yml](.github/workflows/release.yml) runs GoReleaser (Registry
format, signed with the key registered on the Registry). Release only provider assets here: the Registry reads every
asset of every tag.

Layout: `client/` is the one API client both use, `provider/` the Terraform Plugin Framework provider,
the CLI imports `client/` from its own repository.
