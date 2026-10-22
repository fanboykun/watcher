# Watcher Tasks

Open issues and requested improvements. These tasks are pending implementation;
the reported symptoms below do not establish a confirmed root cause.

## 1. Preserve the public host and base path when opening deployment logs

- [ ] Fix the automatic navigation to deployment logs when a user connected to a watcher's SSE stream receives a new deployment event.

**Reported behavior:** Navigation uses `{API_BASE_URL}/{log_pathname}`, which can
send the browser to the wrong host or omit the dashboard's custom base path.
For example, a dashboard accessible at `https://sasad.socfindo.co.id/watcher`
must keep deployment-log navigation under that public URL.

**Acceptance criteria:**

- The deployment-log destination preserves the public dashboard host and configured base path, including `/watcher` in the example above.
- The destination opens the logs for the deployment announced by SSE.
- Navigation works for both root-hosted dashboards and reverse-proxy subpaths, without missing or duplicated prefixes.

## 2. Make the new-version banner perform the update action

- [ ] Fix the new-version banner so its update action starts the Watcher self-update flow and opens the relevant settings tab.

**Reported behavior:** Clicking the banner only navigates to the settings page;
it neither performs the update nor selects the correct tab.

**Acceptance criteria:**

- The banner's update action initiates the self-update flow instead of stopping at navigation.
- Any navigation to settings selects the tab containing Watcher's version and update controls and preserves the dashboard base path.
- Update progress, completion, and failure are visible; repeated clicks do not start duplicate updates.

## 3. Resolve infinite loading for a newly created watcher's first deployment

- [ ] Investigate and fix the watcher detail page remaining in a loading state after fresh watcher creation and during its first deployment.

**Reported behavior:** Opening the detail page for a newly created watcher can
leave the frontend loading indefinitely. The report is specific to fresh
watchers and their first deployment.

**Acceptance criteria:**

- Creating a watcher and immediately opening its detail page renders the page without requiring a previous successful deployment.
- Empty deployment history and an unset current version render usable empty or pending states.
- The first deployment's progress and final status appear without a manual refresh, including when the page was opened before deployment started.
- Request failures display an actionable error instead of leaving the page loading indefinitely.

## 4. Support all managed config files in deployment candidates

- [ ] Extend candidate configuration beyond the env file to include other managed config files.

**Reported behavior:** Candidate configuration currently manages only the env
file. Users also need to prepare changes to other managed config, including
app-dir config files and release-dir config files.

**Acceptance criteria:**

- Users can stage and review managed config file contents and target paths alongside the env file for an exact-version or `next` candidate.
- Saving a candidate keeps its changes isolated from active service configuration and live files until deployment activation.
- Deployment applies the candidate's managed config files to their declared app-dir or release-dir targets using the existing path-validation rules.
- Config snapshots include the deployed managed config; failed deployment recovery and rollback restore the appropriate previous files.
- Existing env-only candidates remain supported.
