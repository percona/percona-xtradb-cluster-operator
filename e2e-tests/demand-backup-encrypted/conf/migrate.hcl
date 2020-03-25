storage_source "file" {
  path = "/vault/data"
}

storage_destination "file" {
  address = "vault-service-2-0.vault-service-2.vault-service-2.svc.cluster.local:8200"
  path    = "/vault/data"
}