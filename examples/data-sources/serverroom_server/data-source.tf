data "serverroom_server" "web" {
  account_id = 246447
}

output "web_ip" {
  value = data.serverroom_server.web.ipv4[0]
}
