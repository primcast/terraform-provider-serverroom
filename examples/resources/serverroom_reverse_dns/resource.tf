data "serverroom_server" "web" {
  account_id = 246447
}

resource "serverroom_reverse_dns" "web" {
  ip   = data.serverroom_server.web.ipv4[0]
  name = "web-01.example.com"
}
