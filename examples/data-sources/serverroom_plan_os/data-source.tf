data "serverroom_plan_os" "m710x" {
  plan = "M710x"
}

output "installable" {
  value = data.serverroom_plan_os.m710x.names
}
