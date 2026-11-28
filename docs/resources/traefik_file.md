---
page_title: "dokploy_traefik_file Resource - terraform-provider-dokploy"
subcategory: ""
description: |-
  Manages a Traefik dynamic configuration file through Dokploy.
---

# dokploy_traefik_file (Resource)

Manages a Traefik dynamic configuration file through Dokploy. Import existing files before managing them. The resource reads the file contents as returned by the API so changes made outside Terraform appear during refresh.

Dokploy does not expose a delete endpoint for Traefik files. Destroy overwrites an existing file with a single comment, removing the configuration it provided. Destroy leaves an already missing file alone. To reuse a destroyed path, re-add it to the Terraform configuration. Create overwrites the exact comment left by destroy without requiring an import.

## Example Usage

```terraform
resource "dokploy_traefik_file" "example" {
  path    = "/etc/dokploy/traefik/dynamic/example.yaml"
  content = file("${path.module}/example.yaml")
}
```

## Schema

### Required

- `path` (String) Absolute path to a direct child of `/etc/dokploy/traefik/dynamic/` with a `.yml` or `.yaml` extension. It must not contain `..` or name either Dokploy-managed file (`dokploy.yml` or `middlewares.yml`). Changing this value replaces the resource.
- `content` (String) Exact non-empty contents to write.

### Optional

- `server_id` (String) Dokploy server ID. Omit it to use the local server. Changing this value replaces the resource.

### Read-Only

- `id` (String) The full file path, prefixed with the server ID and a colon if set.

## Import

```shell
terraform import dokploy_traefik_file.example /etc/dokploy/traefik/dynamic/example.yaml
```

For a file on a remote server, use `server_id:path` as the import ID.
