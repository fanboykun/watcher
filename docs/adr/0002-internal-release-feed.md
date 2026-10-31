# Internal release feeds: a static, write-once directory tree

Some Windows hosts sit on private networks that cannot reach `github.com`.
Their releases are built by a self-hosted runner inside the same network and
published to company storage (MinIO/S3, an IIS/nginx directory, a Nexus or
Artifactory raw repository). A watcher with `source_type = http` polls that
storage instead of GitHub. A watcher with `source_type = fs` reads the same
tree from a local directory or SMB/UNC share.

A feed is a plain directory tree, so any static file server can host it:

```text
<feed root>/
  version.json                        latest pointer, published LAST
  releases.json                       release index (pinning and catalog)
  releases/<version>/version.json     per-release manifest, write-once
  releases/<version>/<artifact>.zip   artifact, write-once
```

The watcher's `metadata_url` is the feed root. A pasted
`<feed root>/version.json` is accepted and trimmed to the root.

## Manifest

Both `version.json` files use the existing manifest shape
(`docs/version-json.md`). Watcher compares only `version`.

`artifact_url` may be relative. It resolves against the URL of the manifest
that lists it, the same way a browser resolves a link, so the root manifest
uses `releases/<version>/<artifact>` and a per-release manifest uses
`<artifact>`. Relative URLs keep a feed relocatable: the same tree can be
mirrored to another host, path or a UNC share without rewriting
manifests. For `http` feeds, absolute `http(s)` URLs remain valid and
anything that does not resolve to `http(s)` is rejected. For `fs` feeds,
relative paths use `/` and resolve against the manifest's directory, absolute
drive or UNC paths are allowed, and URLs are rejected.

Versions may contain slashes (`services/prs/v0.1.0`); they map to nested
directories under `releases/`.

## Publishing contract

Publishers write in this order:

1. `releases/<version>/<artifact>`
2. `releases/<version>/version.json`
3. `releases.json`
4. `version.json`

Writing the root pointer last means a poll never sees a version whose
artifact is not there yet. `releases/<version>/` is write-once: a publisher
fails rather than overwriting an existing version, because rollback, the
`max_ignored_version` pin and retry accounting all assume a version names one
immutable artifact.

A pinned `release_ref` reads `releases/<ref>/version.json`. Refs are tags:
`/` nests directories, but empty, `.` and `..` segments and characters a path
cannot safely hold are rejected.

`releases.json` drives the release catalog. It lists releases newest first,
and each entry repeats that release's service entries so a catalog page is a
single request:

```json
{
  "releases": [
    {
      "tag": "v1.4.2",
      "name": "optional title",
      "published_at": "2026-10-31T04:19:05Z",
      "prerelease": false,
      "notes": "optional release notes",
      "services": {
        "admin-be": {
          "version": "v1.4.2",
          "artifact": "admin-be-v1.4.2.zip",
          "artifact_url": "releases/v1.4.2/admin-be-v1.4.2.zip",
          "sha256": "…",
          "size": 18350112
        }
      }
    }
  ]
}
```

`tag` is the directory under `releases/` and the value a watcher pins. Paths
in `releases.json` resolve against the feed root. A watcher's catalog shows
only releases that list its `service_name`, so one feed can index a monorepo.
Feeds have no server-assigned IDs, so catalog release and asset IDs are
derived from the tag, service and artifact name; they stay stable when newer
releases are prepended.

## Trust boundaries

The GitHub token (watcher override or global) is never sent to a feed host,
for metadata or artifacts. Feed URLs must not embed credentials or a query
string. An `http` feed may instead carry its own credential (bearer, basic or
a custom header), stored write-only on the watcher and added per request hop
only when the hop's scheme, host and port match the feed root, so manifests
and redirects cannot send it elsewhere. `fs` feeds authenticate as the
Windows account Watcher runs as.

GitHub-only features are skipped for feed watchers rather than failing: the
GitHub Deployment API (deploy and rollback reporting) and GitHub repository
inspection. The release catalog reads the feed's `releases.json` instead.

## Considered options

- **Point `artifact_url` at absolute URLs only.** Rejected: every mirror or
  hostname change would mean regenerating manifests, and a feed copied to a
  different store would silently keep downloading from the old one.
- **List the bucket/directory to discover versions.** Rejected: directory
  listing is disabled on most static hosts and object stores, and listing
  cannot express "latest" or prerelease intent. An explicit index is
  portable.
- **Reuse `metadata_url` ending in `version.json` with the GitHub client.**
  This already worked for public, unauthenticated, `latest`-only feeds by
  accident, but the GitHub client sends the GitHub token and parses URLs as
  GitHub repositories as soon as a token or pinned ref is configured. A
  separate source keeps those paths apart.
