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
  name = "storage-pool-model"
}

resource "juju_storage_pool" "this" {
  name             = "storage-pool"
  model_uuid       = juju_model.this.uuid
  storage_provider = "tmpfs"
}

resource "juju_application" "this" {
  model_uuid = juju_model.this.uuid
  name       = "storage-pool-db"
 charm {
    name    = "postgresql-k8s"
    channel = "14/stable"
    base    = "ubuntu@22.04"
  }

  storage_directives = {
    "pgdata" = "128M,${juju_storage_pool.this.name}"
  }

  depends_on = [ juju_storage_pool.this ]
}
