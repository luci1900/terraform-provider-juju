package provider

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAcc_Temporal(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ConfigDirectory: config.StaticDirectory("."),
				Check:           check,
			},
		},
	})
}

func check(_ *terraform.State) error {
	cmd := exec.Command(
		"juju", "switch",
		"temporal-model",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed juju switch: %w, %s", err, out)
	}

	cmd = exec.Command(
		"juju", "wait-for",
		"application", "temporal",
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed juju wait-for: %w, %s", err, out)
	}
	return nil
}
