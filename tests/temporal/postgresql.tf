resource "juju_model" "db" {
  name = "db-model"
}

resource "juju_application" "postgresql" {
  name  = "db"
  trust = true
  units = 1

  charm {
    name    = "postgresql-k8s"
    channel = "14/stable"
    base    = "ubuntu@22.04"
  }

  config = {
    plugin_pg_trgm_enable   = true
    plugin_uuid_ossp_enable = true
    plugin_btree_gin_enable = true
  }
  model_uuid = juju_model.db.uuid
}

resource "juju_offer" "database" {
  depends_on = [juju_application.postgresql]

  application_name = juju_application.postgresql.name
  endpoints        = ["database"]
  model_uuid       = juju_model.db.uuid
}
