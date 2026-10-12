resource "binarylane_server" "example" {
  # ...
}

resource "binarylane_server_reverse_name" "example" {
  server_id    = binarylane_server.example.id
  ipv4_address = binarylane_server.example.public_ipv4_addresses.0
  reverse_name = "mx1.example.com"
}
