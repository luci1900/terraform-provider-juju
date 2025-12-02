package tests

import (
	"context"
	"log"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
)

const tfVersion = "1.14.0"

func setup(workingDir string) func() {
	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion(tfVersion)),
	}

	execPath, err := installer.Install(context.Background())
	if err != nil {
		log.Fatalf("error installing Terraform: %s", err)
	}

	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		log.Fatalf("error running NewTerraform: %s", err)
	}

	err = tf.Init(context.Background(), tfexec.Reconfigure(true))
	if err != nil {
		log.Fatalf("error running Init: %s", err)
	}

	_, err = tf.Plan(context.Background())
	if err != nil {
		log.Fatalf("error running Plan: %s", err)
	}

	err = tf.Apply(context.Background())
	if err != nil {
		log.Fatalf("error running Apply: %s", err)
	}

	return func() {
		err := tf.Destroy(context.Background())
		if err != nil {
			log.Fatalf("error running Destroy: %s", err)
		}
	}
}
