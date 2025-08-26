// Copyright 2023 Canonical Ltd.
// Licensed under the Apache License, Version 2.0, see LICENCE file for details.

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAcc_ResourceAccessModel(t *testing.T) {
	SkipJAAS(t)
	userName := acctest.RandomWithPrefix("tfuser")
	userPassword := acctest.RandomWithPrefix("tf-test-user")
	userName2 := acctest.RandomWithPrefix("tfuser")
	userPassword2 := acctest.RandomWithPrefix("tf-test-user")
	modelName1 := acctest.RandomWithPrefix("tf-access-model-one")
	modelName2 := acctest.RandomWithPrefix("tf-access-model-two")
	accessSuccess := "write"
	accessFail := "bogus"

	resourceName := "juju_access_model.test"
	modelResourceName := "juju_model.test-model"
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: frameworkProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccResourceAccessModel(userName, userPassword, modelName1, accessFail),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Match.*"),
			},
			{
				Config: testAccResourceAccessModel(userName, userPassword, modelName1, accessSuccess),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceName, "model_uuid", modelResourceName, "uuid"),
					resource.TestCheckResourceAttr(resourceName, "access", accessSuccess),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", userName),
				),
			},
			{
				Destroy:           true,
				ImportStateVerify: true,
				ImportState:       true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources[resourceName]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					id := rs.Primary.Attributes["model_uuid"]
					if id == "" {
						return "", fmt.Errorf("model_uuid is empty in state")
					}
					return fmt.Sprintf("%s:%s:%s", id, accessSuccess, userName), nil
				},
				ResourceName: resourceName,
			},
			{
				Config: testAccResourceAccessModel(userName2, userPassword2,
					modelName2, accessSuccess),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "access", accessSuccess),
					resource.TestCheckResourceAttrPair(resourceName, "model_uuid", modelResourceName, "uuid"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", userName2),
				),
			},
			{
				Destroy:           true,
				ImportStateVerify: true,
				ImportState:       true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources[resourceName]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					id := rs.Primary.Attributes["model_uuid"]
					if id == "" {
						return "", fmt.Errorf("model_uuid is empty in state")
					}
					return fmt.Sprintf("%s:%s:%s", id, accessSuccess, userName2), nil
				},
				ResourceName: resourceName,
			},
		},
	})
}

func TestAcc_ResourceAccessModel_UpgradeProvider(t *testing.T) {
	SkipJAAS(t)
	if testingCloud != LXDCloudTesting {
		t.Skip(t.Name() + " only runs with LXD")
	}

	userName := acctest.RandomWithPrefix("tfuser")
	userPassword := acctest.RandomWithPrefix("tf-test-user")
	modelName := acctest.RandomWithPrefix("tf-access-model")
	access := "write"

	resourceName := "juju_access_model.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },

		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"juju": {
						VersionConstraint: TestProviderPreV1Version,
						Source:            "juju/juju",
					},
				},
				Config: testAccResourceAccessModelV0(userName, userPassword, modelName, access),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "access", access),
					resource.TestCheckResourceAttr(resourceName, "model", modelName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", userName),
				),
			},
			{
				ProtoV6ProviderFactories: frameworkProviderFactories,
				Config:                   testAccResourceAccessModel(userName, userPassword, modelName, access),
				PlanOnly:                 true,
			},
		},
	})
}

func TestAcc_ResourceAccessModel_UpgradeV0tToV1(t *testing.T) {
	userName := acctest.RandomWithPrefix("tfuser")
	userPassword := acctest.RandomWithPrefix("tf-test-user")
	modelName := acctest.RandomWithPrefix("tf-access-model")
	access := "write"

	resourceName := "juju_access_model.test"
	modelResourceName := "juju_model.test-model"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"juju": {
						VersionConstraint: TestProviderPreV1Version,
						Source:            "juju/juju",
					},
				},
				Config: testAccResourceAccessModelV0(userName, userPassword, modelName, access),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "access", access),
					resource.TestCheckResourceAttr(resourceName, "model", modelName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", userName),
				),
			},
			{
				ProtoV6ProviderFactories: frameworkProviderFactories,
				Config:                   testAccResourceAccessModel(userName, userPassword, modelName, access),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceName, "model_uuid", modelResourceName, "uuid"),
				),
			},
		},
	})
}

func TestAcc_ResourceAccessModel_ErrorWhenUsedWithJAAS(t *testing.T) {
	OnlyTestAgainstJAAS(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: frameworkProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccResourceAccessModelFixedUser(),
				ExpectError: regexp.MustCompile("This resource is not supported with JAAS"),
			},
		},
	})
}

func testAccResourceAccessModelFixedUser() string {
	return `
resource "juju_access_model" "test" {
  access = "write"
  model = "foo"
  users = ["bob"]
}`
}

func testAccResourceAccessModel(userName, userPassword, modelName, access string) string {
	return fmt.Sprintf(`
resource "juju_user" "test-user" {
  name = %q
  password = %q
}

resource "juju_model" "test-model" {
  name = %q
}

resource "juju_access_model" "test" {
  access = %q
  model_uuid = juju_model.test-model.uuid
  users = [juju_user.test-user.name]
}`, userName, userPassword, modelName, access)
}

func testAccResourceAccessModelV0(userName, userPassword, modelName, access string) string {
	return fmt.Sprintf(`
resource "juju_user" "test-user" {
  name = %q
  password = %q
}

resource "juju_model" "test-model" {
  name = %q
}

resource "juju_access_model" "test" {
  access = %q
  model = juju_model.test-model.name
  users = [juju_user.test-user.name]
}`, userName, userPassword, modelName, access)
}
