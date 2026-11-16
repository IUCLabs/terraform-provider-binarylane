terraform {
  required_providers {
    binarylane = {
      source = "IUCLabs/binarylane"
    }
  }
}

provider "binarylane" {
  api_token = var.binarylane_api_token

  # Or, set environment variable BINARYLANE_API_TOKEN
}
