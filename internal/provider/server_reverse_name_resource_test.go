package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestParseServerReverseNameId(t *testing.T) {
	serverId, ip, err := parseServerReverseNameId(formatServerReverseNameId(12345, "203.0.113.7"))
	if err != nil || serverId != 12345 || ip != "203.0.113.7" {
		t.Fatalf("round trip: got (%d, %q, %v)", serverId, ip, err)
	}

	for _, id := range []string{
		"",
		"12345",
		"12345:",
		":203.0.113.7",
		"abc:203.0.113.7",
		"0:203.0.113.7",
		"-1:203.0.113.7",
		"12345:not-an-ip",
		"12345:2001:db8::1",
		"12345:203.0.113.7:80",
	} {
		if _, _, err := parseServerReverseNameId(id); err == nil {
			t.Errorf("expected error parsing %q", id)
		}
	}
}

func TestServerReverseNameResource(t *testing.T) {
	// Must assign a password to the server or Binary Lane will send emails
	password := GenerateTestPassword(t)

	serverConfig := `

resource "binarylane_server" "test" {
  name              = "tf-test-server-reverse-name"
  region            = "per"
  image             = "debian-12"
  size              = "std-min"
  password          = "` + password + `"
  public_ipv4_count = 1
}
`

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + serverConfig + `
resource "binarylane_server_reverse_name" "test" {
  server_id    = binarylane_server.test.id
  ipv4_address = binarylane_server.test.public_ipv4_addresses.0
  reverse_name = "tf-test-ptr.example.com"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("binarylane_server_reverse_name.test", "server_id", "binarylane_server.test", "id"),
					resource.TestCheckResourceAttrPair("binarylane_server_reverse_name.test", "ipv4_address", "binarylane_server.test", "public_ipv4_addresses.0"),
					resource.TestCheckResourceAttr("binarylane_server_reverse_name.test", "reverse_name", "tf-test-ptr.example.com"),
					resource.TestCheckResourceAttrSet("binarylane_server_reverse_name.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "binarylane_server_reverse_name.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					resourceState := s.RootModule().Resources["binarylane_server_reverse_name.test"]
					return resourceState.Primary.Attributes["id"], nil
				},
			},
			// Update and Read testing
			{
				Config: providerConfig + serverConfig + `
resource "binarylane_server_reverse_name" "test" {
  server_id    = binarylane_server.test.id
  ipv4_address = binarylane_server.test.public_ipv4_addresses.0
  reverse_name = "tf-test-ptr-updated.example.com" # Updated reverse name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("binarylane_server_reverse_name.test", "reverse_name", "tf-test-ptr-updated.example.com"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
