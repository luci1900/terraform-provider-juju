terraform {
  required_providers {
    juju = {
      source = "registry.terraform.io/juju/juju"
      version = "1.0.0"
    }
  }
}

provider "juju" {}


resource "juju_model" "prod" {
  name = "temporal-model"
}

# application and integration top-level
resource "juju_application" "temporal_k8s" {
  name = "temporal"

  charm {
    name = "temporal-k8s"
  }

  config = {
    num-history-shards = 2
  }
  model_uuid = resource.juju_model.prod.uuid
}


resource "juju_integration" "temporal_db" {
  application {
    offer_url = juju_offer.database.url
  }

  application {
    name     = juju_application.temporal_k8s.name
    endpoint = "db"
  }
  model_uuid = resource.juju_model.prod.uuid
}

resource "juju_integration" "temporal_visibility_db" {
  application {
    offer_url = juju_offer.database.url
  }

  application {
    name     = juju_application.temporal_k8s.name
    endpoint = "visibility"
  }
  model_uuid = resource.juju_model.prod.uuid
}

resource "juju_application" "temporal_admin_k8s" {
  name = "temporal-admin"

  charm {
    name = "temporal-admin-k8s"
  }
  model_uuid = juju_model.prod.uuid
}

resource "juju_integration" "temporal_admin" {

  application {
    name     = juju_application.temporal_k8s.name
    endpoint = "admin"
  }

  application {
    name     = juju_application.temporal_admin_k8s.name
    endpoint = "admin"
  }
  model_uuid = juju_model.prod.uuid
}


resource "juju_application" "temporal_k8s_ui" {
  name = "temporalui"

  charm {
    name = "temporal-ui-k8s"
  }
  model_uuid = resource.juju_model.prod.uuid
}

resource "juju_integration" "temporal_ui" {

  application {
    name     = juju_application.temporal_k8s.name
    endpoint = "ui"
  }

  application {
    name     = juju_application.temporal_k8s_ui.name
    endpoint = "ui"
  }
  model_uuid = juju_model.prod.uuid
}
