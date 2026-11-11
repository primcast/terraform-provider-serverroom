# terraform-provider-serverroom

The Terraform provider and the `serverroom` command-line tool for your dedicated servers. Both talk to the API of
the control panel you already use, with an API key from **API & Keys**. The key decides which account, and which
website's API, they use, so there is usually nothing else to configure. If asked for an API URL, give your
control panel website followed by `/api` (`api_url` in Terraform, `--api-url` or `SERVERROOM_API_URL` for the CLI).

- Terraform Registry: [`primcast/serverroom`](https://registry.terraform.io/providers/primcast/serverroom/latest)
- Docs: [docs/](docs/index.md) (also on the Registry)
- CLI downloads: [Releases](https://github.com/primcast/terraform-provider-serverroom/releases)

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

Download the archive for your system from [Releases](https://github.com/primcast/terraform-provider-serverroom/releases)
(checksums in `serverroom_<version>_checksums.txt`, signed), or `go install github.com/primcast/terraform-provider-serverroom/cmd/serverroom@latest`.

```sh
serverroom login                                   # paste your API key; --profile NAME keeps several
serverroom servers list
serverroom servers get 246447
serverroom servers progress 246447 --watch
serverroom reboot 246447 --yes
serverroom ssh-keys add laptop ~/.ssh/id_ed25519.pub
serverroom os M710x                                # what reinstall --os takes
serverroom reinstall 246447 --os "Debian 12" --ssh-key laptop --user-data setup.sh --yes --wait
serverroom rescue start 246447 --ssh-key laptop --yes
serverroom rescue status 246447
serverroom rescue end 246447 --yes
serverroom stock --location "New York" --gpu RTX --max-price 250
serverroom order s4929                             # opens the machine's order page; you pay there
serverroom rdns set 203.0.113.10 web-01.example.com
serverroom monitors add --id 246447 --name shop --type http --target https://shop.example.com/
serverroom webhooks add --url https://ops.example.com/hooks
serverroom port-alert 246447 --enable --threshold 90 --sustain 15
```

`--json` on any command prints JSON. Exit status: 0 done, 1 refused or failed, 2 bad usage.

| Variable | |
|---|---|
| `SERVERROOM_API_KEY` | key to use instead of the saved login (CI) |
| `SERVERROOM_PROFILE` | saved login to use (`--profile`) |
| `SERVERROOM_API_URL` | API base URL (`--api-url`), e.g. `https://www.example.com/api`; default: the key's own website when known |
| `SERVERROOM_CONFIG` | config file (default: your user config folder, `serverroom/config.json`) |

Permissions a key needs: Account read for the server list, Servers read/write for addresses, reverse DNS, reboots
and reinstalls, Rescue mode for `rescue`, Alerts and Webhooks for those commands. `stock`, `os` and `order` need no key.

## Developing

```sh
go test ./...                                      # unit tests, no network
TF_ACC=1 SERVERROOM_API_KEY=... SERVERROOM_TEST_ACCOUNT_ID=... go test ./provider -run TestAcc -v
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name serverroom
```

`SERVERROOM_TEST_REINSTALL="Debian 12"` adds a real reinstall to the acceptance run. **It erases the test server.**

Releases: push a `v*` tag; [.github/workflows/release.yml](.github/workflows/release.yml) runs GoReleaser twice
(provider in Registry format, then the CLI into the same release), both signed with the key registered on the Registry.

Layout: `client/` is the one API client both use, `provider/` the Terraform Plugin Framework provider,
`cmd/serverroom/` the CLI.
