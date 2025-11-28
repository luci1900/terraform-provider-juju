resource "juju_model" "traefik" {
  name = "ingress-model"
}

resource "juju_application" "traefik" {
  name  = "ingress"
  trust = true
  units = 1

  charm {
    name    =  "traefik-k8s"
    channel = "latest/edge"
    base    = "ubuntu@20.04"
  }

  config = {
    external_hostname = ""
    routing_mode      = "path"
  }
  model_uuid = juju_model.traefik.uuid
}

resource "juju_offer" "ingress" {
  depends_on = [juju_application.traefik]

  application_name = juju_application.traefik.name
  name             = "${juju_application.traefik.name}-ingress"
  endpoints        = ["ingress"]
  model_uuid       = juju_model.traefik.uuid
}
