resource "serverroom_monitor" "shop" {
  account_id = 246447
  name       = "Shop homepage"
  type       = "http"
  target     = "https://shop.example.com/"
  keyword    = "Add to cart"
  interval   = 60
}

resource "serverroom_monitor" "postgres" {
  account_id = 246447
  name       = "Postgres"
  type       = "tcp"
  target     = "db.example.com"
  port       = 5432
}
