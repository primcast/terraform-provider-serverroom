# Alert when the server's port stays above 90% of its speed for 15 minutes.
resource "serverroom_port_alert" "web" {
  account_id = 246447
  threshold  = 90
  sustain    = 15
}
