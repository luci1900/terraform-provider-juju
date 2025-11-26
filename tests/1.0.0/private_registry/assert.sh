#!/bin/bash
set -e

OUTPUT=$(microk8s kubectl get secret -n source-model -o yaml test-app-coredns-secret)
ENCODED=$(yq -r '.data.".dockerconfigjson"' <<< "$OUTPUT")
DECODED=$(base64 --decode <<< "$ENCODED")
USERNAME=$(yq -r '.auths."ghcr.io".Username' <<< "$DECODED")
PASSWORD=$(yq -r '.auths."ghcr.io".Password' <<< "$DECODED")

test $USERNAME = "token"
test $PASSWORD = "token"
