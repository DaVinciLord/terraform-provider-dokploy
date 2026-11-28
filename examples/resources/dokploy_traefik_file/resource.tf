resource "dokploy_traefik_file" "example" {
  path    = "/etc/dokploy/traefik/dynamic/example.yaml"
  content = file("${path.module}/example.yaml")
}
