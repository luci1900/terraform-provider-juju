package provider

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

//go:embed main.tf
var plan string

func TestAcc_PrivateRegistry(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"juju": {
						VersionConstraint: "1.0.0",
						Source:            "juju/juju",
					},
				},
				Config: plan,
				Check:  check,
			},
		},
	})
}

type k8sSecret struct {
	Data map[string]string `json:"data"`
}

type k8sCreds struct {
	Auths map[string]ociCreds `json:"auths"`
}

type ociCreds struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

func check(_ *terraform.State) error {
	ns := "source-model"
	secretName := "test-app-coredns-secret"

	cmd := exec.Command(
		"microk8s", "kubectl",
		"-n", ns,
		"wait", "--for=create", "secret", secretName,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed call to kubectl: %w, %s", err, out)
	}

	cmd = exec.Command(
		"microk8s", "kubectl",
		"get", "secret",
		"-n", ns,
		"-o", "json", secretName,
	)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed call to kubectl: %w, %s", err, out)
	}
	var secret k8sSecret
	if err := json.Unmarshal(out, &secret); err != nil {
		return err
	}
	encoded := secret.Data[".dockerconfigjson"]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var creds k8sCreds
	if err := json.Unmarshal(decoded, &creds); err != nil {
		return err
	}
	if creds.Auths["ghcr.io"].Username != "token" {
		return fmt.Errorf("invalid username")
	}
	if creds.Auths["ghcr.io"].Password != "token" {
		return fmt.Errorf("invalid password")
	}

	return nil
}
