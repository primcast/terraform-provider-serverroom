data "serverroom_servers" "all" {}

output "server_names" {
  value = { for s in data.serverroom_servers.all.services : s.account_id => s.name }
}
