# Plan: Generic Windows Capability Installer + IIS Expansion

## Context

Watcher currently offers a profile-based installer in `shell/install-watcher.ps1`:

- Binary services only
- Static sites only
- Both binaries + static sites
- Full stack

That worked when the product surface was mostly:

- NSSM-managed binary services
- IIS-hosted static sites
- optional ARR reverse proxy

But the product is starting to grow in a more modular direction:

- IIS should become more deeply managed by Watcher
- IIS provisioning should eventually be automated, not documented as a manual step
- future IIS targets may include not only static files, but also PHP applications

The current installer shape is starting to become limiting because it bundles several Windows capabilities into coarse profiles rather than letting operators choose the exact modules they need.

---

## Why Change This First

Improving the installer first gives us a cleaner foundation for upcoming IIS work.

Right now, the installer logic is tightly coupled to `Profile`:

- `Profile` drives whether NSSM is installed
- `Profile` drives whether IIS features are enabled
- `Profile` drives whether URL Rewrite is installed
- `Profile` drives whether ARR is installed and enabled

That becomes awkward once we add more IIS variants, for example:

- IIS for static sites
- IIS for PHP sites
- IIS + URL Rewrite only
- IIS + FastCGI/PHP without ARR
- ARR proxy without static hosting

A module-based installer will let us support those combinations cleanly without inventing more and more preset profiles.

---

## Goals

1. Replace the profile-based installer model with a capability/module-based model.
2. Make installer choices clearer by grouping modules by service type and use case.
3. Prepare the product for richer IIS support, including PHP hosting.
4. Keep the installer friendly for simple setups by still offering recommended bundles or presets.
5. Make future Watcher runtime features line up with installed Windows components.

---

## Non-Goals

- Full IIS site/app/app-pool automation in this first installer phase.
- Full PHP runtime management in the first pass.
- Linux package/module management.
- Replacing existing deployment logic for NSSM services in the same phase.

---

## Current State Summary

### Installer

`shell/install-watcher.ps1` currently uses a single `Profile` selection and derives:

- `$installNSSM`
- `$installIIS`
- `$installARR`

IIS installation currently includes:

- IIS Windows features
- URL Rewrite

ARR installation currently includes:

- ARR package
- ARR proxy enablement

### Runtime

Watcher already distinguishes service types at runtime:

- `nssm`
- `static`

For static services, deploy currently:

- extracts files
- swaps `current`
- optionally recycles an IIS app pool

But it does not create or manage the IIS site itself yet.

### Product Direction

We want IIS support to grow beyond the current "static site with manual IIS registration" model and eventually support PHP-oriented deployments too.

---

## Target State

The installer should become capability-driven.

Instead of asking users to choose only one coarse installation profile, it should show grouped checkboxes for Windows modules and helpers, for example:

### Core

- Watcher service
- Dashboard/API port configuration
- Watcher install directory
- Logs/database directories

### For Binary Services

- NSSM

### For IIS Static Hosting

- IIS Web Server
- IIS Management Scripts/Console
- URL Rewrite

### For IIS PHP Hosting

- IIS Web Server
- IIS Management Scripts/Console
- CGI / FastCGI
- URL Rewrite
- optional PHP runtime/bootstrap support

### For Reverse Proxying

- ARR
- ARR proxy enablement

### Optional Utilities

- Chocolatey bootstrap
- validation/preflight checks

The UI can still offer presets such as:

- Binary services
- IIS static hosting
- IIS PHP hosting
- Full stack

But those presets should simply pre-check modules instead of controlling the entire installation flow.

---

## Recommended Module Model

Introduce an explicit installer capability model instead of overloading `Profile`.

Example conceptual flags:

- `InstallChocolatey`
- `InstallNSSM`
- `InstallIISCore`
- `InstallIISMgmtTools`
- `InstallURLRewrite`
- `InstallARR`
- `EnableARRProxy`
- `InstallFastCGI`
- `PreparePHPHosting`

These do not all need to ship at once, but the structure should support them cleanly.

---

## Service-Type Grouping Proposal

This matches your idea and I think it is the right direction.

The installer should present modules grouped by what they are used for:

### Group 1: Binary Services

- NSSM
- optional related validation

### Group 2: IIS Static Sites

- IIS web server features
- IIS management scripting tools
- URL Rewrite

### Group 3: IIS PHP Sites

- IIS web server features
- IIS management scripting tools
- CGI/FastCGI
- URL Rewrite
- optional PHP runtime setup

### Group 4: Reverse Proxy / Gateway

- ARR
- ARR proxy enablement

This is much more scalable than today’s profile dropdown because it matches the way operators think about workloads.

---

## Proposed Execution Order

### Phase 1: Refactor installer data model

Replace profile-derived booleans with explicit capability flags.

Tasks:

1. Introduce a config object with named module booleans.
2. Keep temporary backward compatibility by mapping old profiles into module selections internally.
3. Split installation logic into reusable capability installers:
   - `Install-ChocolateyIfNeeded`
   - `Install-NSSMIfNeeded`
   - `Install-IISCoreIfNeeded`
   - `Install-URLRewriteIfNeeded`
   - `Install-ARRIfNeeded`
   - `Enable-ARRProxyIfNeeded`
   - future `Install-FastCGIIfNeeded`
4. Keep preflight and summary output aligned with selected modules.

Deliverable:
- installer logic is modular even if UI still partially resembles the current one

### Phase 2: Replace dropdown with grouped module checkboxes

Redesign the installer wizard UI.

Tasks:

1. Replace `Installation Profile` dropdown with grouped checkboxes.
2. Add short help text under each group describing what it enables.
3. Add optional preset buttons that populate the checkboxes.
4. Show dependencies automatically:
   - selecting ARR implies IIS core
   - selecting PHP hosting implies IIS core + FastCGI
   - selecting URL Rewrite may imply IIS core
5. Keep the wizard understandable for simple installs.

Deliverable:
- installer UI reflects real Windows capability choices

### Phase 3: Add IIS capability tiers for future workloads

Prepare the installer for richer IIS hosting.

Tasks:

1. Separate IIS base features from IIS add-ons.
2. Define the minimal feature set for:
   - static hosting
   - PHP hosting
   - reverse proxy only
3. Add validation checks to confirm required Windows modules are active.
4. Update install summary to show exactly what got enabled.

Deliverable:
- installer can support multiple IIS workload types without new profile explosion

### Phase 4: Runtime alignment for IIS-managed deployments

Once installer groundwork is in place, improve Watcher runtime support.

Tasks:

1. Expand service/runtime metadata for IIS-backed workloads.
2. Add IIS provisioning support for static site deployments.
3. Add status checks for:
   - app pool existence
   - site existence
   - binding/path drift
4. Keep provisioning idempotent and safe.

Deliverable:
- Watcher begins managing IIS resources, not only deployed files

### Phase 5: PHP deployment support design

After installer and basic IIS management are stable, add PHP-specific support.

Tasks:

1. Decide runtime model:
   - system PHP already installed
   - Watcher-assisted PHP bootstrap
   - bundled PHP runtime
2. Define service type semantics for PHP workloads.
3. Decide whether PHP sites are modeled as:
   - new service type, or
   - IIS sub-mode under current static/IIS family
4. Define needed artifacts/config support:
   - `web.config`
   - FastCGI mapping
   - environment/config file management
   - writable directories
5. Add safety/validation around app pool and PHP runtime compatibility.

Deliverable:
- clear implementation path for IIS PHP deployments

---

## Design Principles

### 1. Presets should assist, not constrain

Preset buttons are useful, but they should only pre-select modules. Users should still be able to adjust checkboxes afterward.

### 2. Installer choices should map to real Windows capabilities

We should avoid vague labels where possible. If a checkbox installs or enables a Windows component, the UI should say so.

### 3. Dependencies should be automatic and visible

If a user selects ARR, the installer should make it obvious that IIS core is also required.

### 4. Runtime should not assume modules that installer never offered

As Watcher grows IIS/PHP support, the installer and runtime should stay aligned so operators are not surprised.

### 5. Keep the simple path simple

A user deploying only Go binaries should still be able to install Watcher quickly without learning IIS terminology.

---

## Open Decisions

1. Do we keep the old profile dropdown temporarily during transition, or replace it immediately with checkboxes?
2. Should PHP support mean:
   - install IIS prerequisites only, or
   - also install PHP runtime automatically?
3. Should URL Rewrite be considered part of all IIS workloads, or only static/proxy/PHP presets that need it?
4. Do we want ARR as a standalone capability even when no static/PHP site is being hosted by Watcher?
5. Should the installer write selected capabilities into `.env` or another machine-local metadata file for diagnostics?

---

## Acceptance Criteria

1. Installer no longer relies solely on the current profile enum to decide what Windows components to install.
2. Users can see Windows modules grouped by service type usage.
3. Installer can represent at least these combinations cleanly:
   - NSSM only
   - IIS static only
   - IIS static + ARR
   - IIS PHP prerequisites
4. Installer summary clearly shows what was selected and what was actually installed/enabled.
5. The resulting structure makes upcoming IIS site automation easier to add.

---

## Suggested Immediate Next Step

Start with Phase 1 and Phase 2 together in a minimal form:

1. keep existing install behavior intact underneath
2. refactor the script to use explicit capability flags
3. replace the profile dropdown with grouped checkboxes plus preset buttons

That gives us a much better foundation before we add IIS site creation and later PHP deployment support.
