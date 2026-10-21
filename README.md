# Server Room / Primcast: Terraform provider and `servcast` CLI

Manage your dedicated servers' webhooks, monitors, port-load alerts, reverse DNS and operating system as code, or from
a terminal. Both use an **API key** from the control panel (**API & Keys**); a key only reaches what its permissions
allow, and never anything that spends money, so servers are referenced by account id rather than ordered here.

## Terraform

| Resource | What it manages |
|---|---|
| `servcast_webhook` | an endpoint that receives your account's events (signed); `secret` is known for endpoints Terraform created |
| `servcast_monitor` | a website / TCP / ping check of your own, within your support tier's allowance |
| `servcast_port_alert` | alert when a server's port stays above a share of its speed (destroy = off) |
| `servcast_reverse_dns` | the PTR name of one of your addresses (destroy leaves it as it is) |
| `servcast_server_os` | the operating system; changing it **reinstalls and erases the server**, only with `allow_reinstall = true` |

| Data source | |
|---|---|
| `servcast_account` | the key's customer, services and permissions |
| `servcast_servers` | every service with plan, name, status |
| `servcast_server` | one server: OS, location, port speed, features, IPv4 addresses |

See [examples/main.tf](examples/main.tf). Permissions the key needs: `account:read`, `servers:read/write`,
`webhooks:read/write`, `alerts:read/write` for what you use.

### Running a local build

```sh
go build -o ~/.terraform.d/plugins/terraform-provider-servcast .
```

then in `~/.terraformrc` (`%APPDATA%\terraform.rc` on Windows):

```hcl
provider_installation {
  dev_overrides { "serverroom/servcast" = "/home/you/.terraform.d/plugins" }
  direct {}
}
```

## CLI

```sh
go build -o servcast ./cmd/servcast
export SERVCAST_API_KEY=srk_...      # or pck_... for Primcast
export SERVCAST_BRAND=serverroom.net # or primcast.com
servcast whoami
servcast servers
servcast server 246447
servcast monitor add --account 246447 --name Shop --type http --target https://shop.example.com/ --interval 60
servcast port-alert 246447 --enable --threshold 90 --sustain 15
servcast reinstall 246447 --os "Debian 12" --ssh-keys ~/.ssh/id_ed25519.pub --user-data setup.sh --yes
```

`--json` on any read prints JSON. Exit status: 0 done, 1 refused or failed, 2 bad usage.

## Layout

- `client/` -- the one API client both use (success is decided per route; see its package comment)
- `provider/` -- the Terraform Plugin Framework provider
- `cmd/servcast/` -- the CLI

## Publishing (not done yet)

The Terraform Registry needs a public GitHub repository named `terraform-provider-servcast`, releases built with
GoReleaser and signed with a GPG key registered on the Registry, and the `alerts` API permission deployed (ServCast
branch `bm-alerts`). Until then, use a local build as above.
