terraform {
  required_providers {
    serverroom = {
      source  = "primcast/serverroom"
      version = "~> 0.1"
    }
  }
}

# The key comes from SERVERROOM_API_KEY. It also decides which website's API the
# provider talks to, so there is nothing else to set.
provider "serverroom" {}
