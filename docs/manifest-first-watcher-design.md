# Manifest-First Watcher Design

## Goal

New watcher setup should use `version.json` as the primary deployment contract for org releases. Repo asset discovery remains available only as a legacy/manual fallback.

## Problem

The current create wizard normalizes any pasted release URL down to the repo URL. That forces repo asset-discovery mode even when a `version.json` manifest exists. In monorepos this is unsafe because the wizard can infer broad service entries from repository releases/assets instead of one selected deployment target.

At deploy time, Watcher correctly downloads the selected manifest artifact, but the deployer loops over all service definitions attached to the watcher. If creation attached unrelated service entries or repeated `binary_name` values, NSSM/IIS configuration can target binaries from the wrong release contents.

## Manifest Contract

`version.json` stays backward-compatible:

```json
{
  "services": {
    "prs": {
      "version": "services/prs/v0.1.0",
      "artifact": "prs-v0.1.0.zip",
      "artifact_url": "https://github.com/org/repo/releases/download/services/prs/v0.1.0/prs-v0.1.0.zip",
      "published_at": "2026-06-01T04:19:05Z"
    }
  }
}
```

Optional deployment hints allow the UI to create the right service config without guessing:

```json
{
  "services": {
    "prs": {
      "version": "services/prs/v0.1.0",
      "artifact": "prs-v0.1.0.zip",
      "artifact_url": "https://github.com/org/repo/releases/download/services/prs/v0.1.0/prs-v0.1.0.zip",
      "published_at": "2026-06-01T04:19:05Z",
      "app_kind": "nssm",
      "windows_service_name": "prs",
      "binary_name": "prs.exe",
      "start_arguments": "",
      "env_file": ".env",
      "health_check_url": "http://localhost:8080/health"
    }
  }
}
```

For IIS-hosted apps:

```json
{
  "app_kind": "php",
  "iis_app_pool": "prs",
  "iis_site_name": "prs",
  "public_url": "https://prs.example.com",
  "health_check_url": "https://prs.example.com/health"
}
```

`app_kind` mapping:

- `nssm` creates `service_type=nssm`.
- `static`, `php`, and `aspnet_classic` create `service_type=iis` with `iis_app_kind` set to the same value.

## UI Behavior

The create wizard accepts either a repo URL or direct `version.json` URL.

On inspect:

1. Try to load `version.json` for the selected release ref.
2. If found, show manifest services as deployable targets.
3. Selecting one target sets `watcher.service_name` to that manifest key.
4. Generate exactly one service definition from the selected manifest service.
5. Save `metadata_url` as the manifest URL so runtime polling uses manifest mode.
6. Keep repo asset discovery only as a clearly labeled fallback if no manifest exists.

The form should prefill watcher/service fields from manifest hints:

- watcher name
- service name
- install directory
- service type
- binary name for NSSM
- IIS app kind, app pool, site, and public URL
- health check URL
- env file

Users can still edit the generated service definition before saving.

## Runtime Guard

The deploy path continues to deploy only `services[watcher.service_name].artifact_url`. The creation/update flow must not attach unrelated services in manifest mode. A manifest-selected watcher defaults to one service definition, preventing unrelated binaries with the same `binary_name` from being configured during deploy.

## Compatibility

Existing repo URL watchers still work. If no manifest can be found, inspect falls back to current repo asset discovery and labels it as legacy/manual discovery.
