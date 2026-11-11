# The three cheapest GPU servers in stock in New York or Miami.
data "serverroom_stock" "gpu" {
  location = ["New York", "Miami"]
  gpu      = ["RTX"]
  limit    = 3
}

output "cheapest_gpu_server" {
  value = data.serverroom_stock.gpu.servers[0].order_url
}
