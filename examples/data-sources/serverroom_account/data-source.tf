data "serverroom_account" "me" {}

output "my_services" {
  value = data.serverroom_account.me.account_ids
}
