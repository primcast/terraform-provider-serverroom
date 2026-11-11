# Everything that happens on the account, POSTed to your endpoint and signed.
resource "serverroom_webhook" "ops" {
  url         = "https://ops.example.com/hooks/servers"
  events      = ["*"]
  description = "ops alerts"
}

output "webhook_secret" {
  value     = serverroom_webhook.ops.secret
  sensitive = true
}
