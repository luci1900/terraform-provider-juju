resource "juju_model" "openfga_model" {
  name = "openfga-model"
}

resource "juju_application" "openfga" {
  name  = "iam"
  trust = true
  units = 1

  charm {
    name    = "openfga-k8s"
    channel = "latest/edge"
    base    = "ubuntu@22.04"
  }
  model_uuid = juju_model.openfga_model.uuid
}

resource "juju_offer" "openfga" {
  depends_on = [juju_application.openfga]

  application_name = juju_application.openfga.name
  endpoints        = ["openfga"]
  model_uuid       = juju_model.openfga_model.uuid
}

resource "juju_integration" "db_integration" {
  application {
    offer_url = juju_offer.database.url
  }

  application {
    name     = juju_application.openfga.name
    endpoint = "database"
  }
  model_uuid = juju_model.openfga_model.uuid
}
