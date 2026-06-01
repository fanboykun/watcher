# Watcher version.json Contract

`version.json` is the manifest Watcher prefers during setup and polling. New org release workflows should publish it for every Watcher-managed release. Repo asset discovery still exists as a legacy fallback, but manifests are the stable contract.

## Shape

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

The key under `services` must match the Watcher `service_name`. In the dashboard create wizard, each key is shown as one deploy target. Selecting a target creates one service definition from that target's manifest hints.

## Required Fields

| Field | Required | Description |
| --- | --- | --- |
| `services` | yes | Object keyed by Watcher service name. |
| `services.<name>.version` | yes | Version Watcher compares with `current_version`. For monorepos, prefer the full service tag such as `services/prs/v0.1.0` to avoid ambiguity. |
| `services.<name>.artifact` | yes | Exact release asset filename. |
| `services.<name>.artifact_url` | yes | GitHub release download URL for the artifact. Slash tags such as `services/prs/v0.1.0` are supported. |
| `services.<name>.published_at` | recommended | UTC timestamp for display and diagnostics. |

## Deployment Hint Fields

All deployment hint fields are optional for backward compatibility, but org templates should set them so the UI does not guess.

| Field | Applies To | Description |
| --- | --- | --- |
| `app_kind` | all | Deployment kind. Supported values: `nssm`, `static`, `php`, `aspnet_classic`. Missing or unknown values default to NSSM behavior in the UI. |
| `windows_service_name` | all | For NSSM, the Windows service name. For IIS, a stable service identifier used as a default for app pool/site fields. |
| `binary_name` | `nssm` | Executable filename that must exist after the zip is extracted under `current/`. Example: `prs.exe`. |
| `start_arguments` | `nssm` | Optional arguments passed to the NSSM service application. |
| `env_file` | `nssm` | Relative env file path Watcher writes when service env content is configured. Defaults commonly use `.env`. |
| `health_check_url` | all | Service-level health check URL. Overrides watcher-level health URL for this service. |
| `iis_app_pool` | `static`, `php`, `aspnet_classic` | IIS app pool name to create/recycle. |
| `iis_site_name` | `static`, `php`, `aspnet_classic` | IIS site name to create/update. |
| `iis_managed_runtime` | `aspnet_classic` | IIS managed runtime, usually `v4.0`. Leave empty for `static` and `php` so the pool uses No Managed Code. |
| `public_url` | `static`, `php`, `aspnet_classic` | Public URL shown in the UI and useful as a default health target. |

## App Kind Mapping

| `app_kind` | Watcher service config | Artifact expectation |
| --- | --- | --- |
| `nssm` | `service_type=nssm` | Zip contains `binary_name` at the root after extraction. |
| `static` | `service_type=iis`, `iis_app_kind=static` | Zip contains static site files at the root, usually `index.html`. |
| `php` | `service_type=iis`, `iis_app_kind=php` | Zip contains PHP app files at the root; IIS/FastCGI/PHP must be available on the host. |
| `aspnet_classic` | `service_type=iis`, `iis_app_kind=aspnet_classic` | Zip contains classic ASP.NET app files at the root. |

## Monorepo Index Release

A monorepo can publish a `latest` release containing only `version.json`. That manifest can point each service to its own service-scoped release asset.

```json
{
  "services": {
    "hrm": {
      "version": "services/hrm/v0.1.0",
      "artifact": "hrm-v0.1.0.zip",
      "artifact_url": "https://github.com/org/people-function-backend/releases/download/services/hrm/v0.1.0/hrm-v0.1.0.zip",
      "published_at": "2026-06-01T04:19:05Z",
      "app_kind": "nssm",
      "windows_service_name": "hrm",
      "binary_name": "hrm.exe",
      "env_file": ".env"
    },
    "prs": {
      "version": "services/prs/v0.1.0",
      "artifact": "prs-v0.1.0.zip",
      "artifact_url": "https://github.com/org/people-function-backend/releases/download/services/prs/v0.1.0/prs-v0.1.0.zip",
      "published_at": "2026-06-01T04:19:05Z",
      "app_kind": "nssm",
      "windows_service_name": "prs",
      "binary_name": "prs.exe",
      "env_file": ".env"
    }
  }
}
```

Configure Watcher with the repo URL or direct manifest URL. During creation, Watcher tries to load `version.json` first, shows these services as deploy targets, and creates one service definition for the selected target.

## Compatibility Notes

- Existing manifests with only `version`, `artifact`, `artifact_url`, and `published_at` still deploy. The UI will fill missing service fields with conservative defaults.
- Repo asset discovery is a fallback when no manifest exists. It is not the recommended org contract because it derives service names from filenames.
- Watcher compares only `version`. Re-publishing a zip with the same `version` will not trigger a new deploy.
- Use unique `install_dir` values per watcher.
