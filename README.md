# Terraform Provider for BinaryLane

A Terraform provider for managing [BinaryLane](https://www.binarylane.com.au/) cloud resources: servers, firewall rules, reverse DNS, SSH keys, VPCs and load balancers.

This is a fork of [oscarhermoso/terraform-provider-binarylane](https://github.com/oscarhermoso/terraform-provider-binarylane). It tracks upstream and adds:

- `binarylane_server_reverse_name` resource for managing IPv4 PTR records
- `destination_ports` is optional on `binarylane_server_firewall_rules`, allowing rules for protocols such as ICMP
- server action waits are bounded by a timeout instead of polling forever

## Quick start

```terraform
terraform {
  required_providers {
    binarylane = {
      source = "IUCLabs/binarylane"
    }
  }
}

provider "binarylane" {
  # Or set BINARYLANE_API_TOKEN
  api_token = var.binarylane_api_token
}

resource "binarylane_server" "example" {
  region            = "per"
  image             = "ubuntu-24.04"
  size              = "std-min"
  public_ipv4_count = 1
}
```

More complete configurations live in [`examples/`](./examples/README.md).

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 (plugin protocol 6)
- [Go](https://go.dev/doc/install) >= 1.25 to build the provider
- A BinaryLane API token

## Documentation

Provider and resource documentation is in [`docs/`](./docs/index.md), in the Terraform Registry layout. Regenerate it with `make docs` after changing a schema.

## Developing

```sh
make build   # build the provider binary
make test    # unit tests
make docs    # regenerate docs/
```

Acceptance tests run against the live BinaryLane API and need `TF_ACC=1` and `BINARYLANE_API_TOKEN`. See [CONTRIBUTING.md](./CONTRIBUTING.md) for local development overrides.

## License

[MIT](./LICENSE), as inherited from upstream.
