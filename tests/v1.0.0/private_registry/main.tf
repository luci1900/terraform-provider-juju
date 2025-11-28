terraform {
  required_providers {
    juju = {
      source = "registry.terraform.io/juju/juju"
      version = "1.0.0"
    }
  }
}

provider "juju" {
}

resource "juju_model" "this" {
  name = "source-model"
}

resource "juju_application" "this" {
  model_uuid = juju_model.this.uuid
  name       = "test-app"
  charm {
    name    = "coredns"
    channel = "latest/stable"
  }
  trust = true
  expose {}
  registry_credentials = {
    "ghcr.io/canonical" = {
      username = "token"
      password = "token"
    }
  }
  resources = {
    "coredns-image" : "ghcr.io/canonical/test:dfb5e3fa84d9476c492c8693d7b2417c0de8742f"
  }
  config = {
    juju-external-hostname = "myhostname"
  }
}
