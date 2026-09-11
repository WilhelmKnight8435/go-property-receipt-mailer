#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST http://localhost:8080/notifications \
  --header 'Content-Type: application/json' \
  --data '{"id":"maintenance-204","kind":"maintenance_request","tenant_email":"tenant@example.com","property":"17 Pine St","reference":"MR-204"}'
