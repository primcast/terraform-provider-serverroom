# Adopts the running system as it is. Changing os, ssh_keys or user_data later
# REINSTALLS the server and erases its disks, and only with allow_reinstall = true.
resource "serverroom_server_os" "web" {
  account_id      = 246447
  os              = "Ubuntu 24.04"
  ssh_keys        = file("~/.ssh/id_ed25519.pub")
  user_data       = file("${path.module}/cloud-init.yaml")
  allow_reinstall = false
  wait            = true
}
