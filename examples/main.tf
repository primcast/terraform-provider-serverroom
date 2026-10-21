terraform {
  required_providers {
    servcast = {
      source = "serverroom/servcast"
    }
  }
}

# The key comes from SERVCAST_API_KEY; brand defaults to serverroom.net.
provider "servcast" {
  brand = "serverroom.net"
}

data "servcast_server" "web" {
  account_id = 246447
}

# Reverse DNS for the server's first address.
resource "servcast_reverse_dns" "web" {
  ip   = data.servcast_server.web.ipv4[0]
  name = "web-01.example.com"
}

# Watch the shop and the database from outside.
resource "servcast_monitor" "shop" {
  account_id = data.servcast_server.web.account_id
  name       = "Shop homepage"
  type       = "http"
  target     = "https://shop.example.com/"
  keyword    = "Add to cart"
  interval   = 60
}

resource "servcast_monitor" "postgres" {
  account_id = data.servcast_server.web.account_id
  name       = "Postgres"
  type       = "tcp"
  target     = "db.example.com"
  port       = 5432
}

# Tell me when the port has been above 90% for 15 minutes.
resource "servcast_port_alert" "web" {
  account_id = data.servcast_server.web.account_id
  threshold  = 90
  sustain    = 15
}

# Everything that happens on the account, posted to our own endpoint.
resource "servcast_webhook" "ops" {
  url    = "https://ops.example.com/hooks/servcast"
  events = ["*"]
}

output "webhook_secret" {
  value     = servcast_webhook.ops.secret
  sensitive = true
}

# The operating system. Changing os REINSTALLS the server -- only with allow_reinstall = true.
resource "servcast_server_os" "web" {
  account_id      = data.servcast_server.web.account_id
  os              = "Ubuntu 24.04"
  ssh_keys        = file("~/.ssh/id_ed25519.pub")
  allow_reinstall = false
}
