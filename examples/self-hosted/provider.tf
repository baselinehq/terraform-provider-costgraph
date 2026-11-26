terraform {
  required_providers {
    costgraph = {
      source = "baselinehq/costgraph"
    }
  }
}

provider "costgraph" {
  api_url = "https://costgraph-api.internal.example.com"
}
