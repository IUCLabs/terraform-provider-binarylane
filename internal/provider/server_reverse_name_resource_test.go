package provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"terraform-provider-binarylane/internal/binarylane"

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

	var serverId int64
	var ipv4Address string

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			return testAccCheckServerReverseNameCleared(serverId, ipv4Address)
		},
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
					func(s *terraform.State) error {
						attrs := s.RootModule().Resources["binarylane_server_reverse_name.test"].Primary.Attributes
						var err error
						serverId, err = strconv.ParseInt(attrs["server_id"], 10, 64)
						ipv4Address = attrs["ipv4_address"]
						return err
					},
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
			// Removing the resource clears the reverse name while the server still exists
			{
				Config: providerConfig + serverConfig,
				Check: func(_ *terraform.State) error {
					return testAccCheckServerReverseNameCleared(serverId, ipv4Address)
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccCheckServerReverseNameCleared(serverId int64, ipv4Address string) error {
	client, err := binarylane.NewClientWithConfig("", "")
	if err != nil {
		return err
	}
	resp, err := client.GetServersServerIdWithResponse(context.Background(), serverId)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status %s reading server %d: %s", resp.Status(), serverId, resp.Body)
	}
	for _, n := range resp.JSON200.Server.Networks.V4 {
		if n.IpAddress != ipv4Address || n.ReverseName == nil {
			continue
		}
		switch *n.ReverseName {
		case "tf-test-ptr.example.com", "tf-test-ptr-updated.example.com":
			return fmt.Errorf("reverse name for %s on server %d not cleared: %s", ipv4Address, serverId, *n.ReverseName)
		}
	}
	return nil
}
