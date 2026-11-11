# A whole server as code: read it, name its address, watch it, and get its events.
#
#   export SERVERROOM_API_KEY=...        # API & Keys in your control panel
#   terraform init && terraform apply -var account_id=246447
terraform {
  required_providers {
    serverroom = {
      source  = "primcast/serverroom"
      version = "~> 0.1"
    }
  }
}

provider "serverroom" {}

variable "account_id" {
  type        = number
  description = "The server's service id (serverroom servers list)."
}

variable "hostname" {
  type    = string
  default = ""
}

data "serverroom_server" "this" {
  account_id = var.account_id
}

# Keep the installed system as it is; flip allow_reinstall to change os.
resource "serverroom_server_os" "this" {
  account_id = var.account_id
  os         = data.serverroom_server.this.os
}

resource "serverroom_reverse_dns" "this" {
  count = var.hostname == "" ? 0 : 1
  ip    = data.serverroom_server.this.ipv4[0]
  name  = var.hostname
}

resource "serverroom_monitor" "ping" {
  account_id = var.account_id
  name       = "ping ${data.serverroom_server.this.ipv4[0]}"
  type       = "ping"
  target     = data.serverroom_server.this.ipv4[0]
}

resource "serverroom_port_alert" "this" {
  account_id = var.account_id
  threshold  = 90
  sustain    = 15
}

output "server" {
  value = {
    server_id = data.serverroom_server.this.server_id
    os        = data.serverroom_server.this.os
    location  = data.serverroom_server.this.location
    ipv4      = data.serverroom_server.this.ipv4
  }
}
