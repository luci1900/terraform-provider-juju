provider "juju" {

}

list "juju_model" "my-model" {
    provider = juju
    count = 1
}
