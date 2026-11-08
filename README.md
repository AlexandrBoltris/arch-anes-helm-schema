# helm-schema

This repository is a fork of [arch-anes/helm-schema](https://github.com/arch-anes/helm-schema),
maintained at [AlexandrBoltris/arch-anes-helm-schema](https://github.com/AlexandrBoltris/arch-anes-helm-schema).
It extends the upstream generator with:

- Read-only `--check` mode for CI schema-freshness checks.
- werf and Nelm deployment profiles, including runtime service values and werf `.helm/` discovery.
- Value descriptions that retain YAML comments and name the template files using each value.
- Indented JSON output.
- Multi-architecture Docker images for Linux amd64/arm64, built by the release workflow.

Release assets and images for this fork use its own repository namespace; see the
installation sections below. The feature list describes the current source, not
every published revision; the CI examples pin the upcoming release and note when
it must be published.

`helm-schema` generates `values.schema.json` from values that Helm templates use.

It reads `values.yaml` for defaults, types, descriptions, and default text passed to `tpl`. A declaration alone does not make a value configurable.

Chart loading and schema validation use Helm 4.2.4. Deployment profiles also support werf and Nelm service values.

## Current behavior

The generator:

- Parses templates without rendering them.
- Follows reachable named templates and common Helm and Sprig functions.
- Tracks values through variables, pipelines, conditions, maps, and lists.
- Separates collection membership from fields used inside each element.
- Resolves literal selectors, calculated string keys, and helper names built from immutable chart fields.
- Tracks object changes from `merge`, `mergeOverwrite`, `set`, and `unset` through copied template contexts.
- Supports local dependencies, aliases, `.Subcharts`, conditions, tags, imports, and global values.
- Generates schemas for the selected chart and each unpacked dependency chart.
- Keeps unused defaults valid with exact `const` rules.
- Rejects unknown properties unless template behavior can use their names or values.
- Analyzes template expressions in defaults only when that value reaches `tpl`.
- Keeps inferred fields strict inside dynamic maps that templates inspect by field.
- Treats rollout checksums as exact use instead of permission for unknown fields.
- Ignores commented-out keys because comments do not prove runtime use.
- Names the chart files that use each value in its `description`.
- Writes stable, indented Draft 7 JSON and rejects a schema above Helm's chart-file limit.
- Replaces existing schemas in the selected chart and its unpacked dependencies after successful generation.
- Replaces an oversized unpacked schema even when Helm refuses to load that old file.

The command stays quiet for local precision limits.

If a generated schema permits root names that templates do not name statically, the command writes one note.

Argument parsing uses Go's standard `flag` package. The command has one
optional path and no subcommands, so a larger command framework is not needed.

## Open and closed objects

The terms *open* and *closed* describe which property names an object schema permits.

| Schema rule | Permitted property names | Permitted property values |
|---|---|---|
| `additionalProperties: false` | Only names in `properties` | Each named property follows its schema. |
| `additionalProperties: true` | Names in `properties` and all other names | An unnamed property can contain any JSON value. |
| `additionalProperties: { ... }` | Names in `properties` and all other names | Each unnamed property follows the given entry schema. |

A closed object uses `additionalProperties: false`. It rejects a misspelled or unsupported child name.

An open object uses one of the other two rules. The entry schema can still be closed.

Complete use, such as direct YAML output, makes all declared descendants configurable. It also permits new descendants because they affect the output.

A dynamic field selection is narrower than complete use. It can permit arbitrary service names while each service entry rejects an unsupported `enablede` field.

A dynamic map uses an entry schema when analysis can infer one. The map then permits arbitrary entry names but keeps every entry closed.

Dynamic `tpl` context has a narrower rule. A specific values subtree permits new direct properties. The complete values root stays closed. Known properties follow their own usage evidence.

A truth test with no known field contract permits new direct properties in an empty object. If templates also access specific fields, that field contract stays closed. Helm removes null members, so a null-only object is empty.

A checksum is exact use, not open use. A checksum makes the selected value configurable but does not permit unsupported descendants.

The same distinction applies to `keys` and `len`. These functions can depend on direct property names. They do not use the values below those names.

The root precision note means that at least one schema permits a root name without a static template reference. It does not mean that all nested objects are open.

## Build

Install Go 1.26 or later. Install Helm 4 to run the integration tests or use the Helm plugin.

```sh
make build
```

The binary is written to `bin/helm-schema`.

## Use the command

```sh
bin/helm-schema ./path/to/chart
helm schema ./path/to/chart
```

The chart path defaults to the current directory:

```sh
cd ./path/to/chart
/path/to/helm-schema
```

Check that committed schemas are current without writing them, for example in CI:

```sh
bin/helm-schema --check ./path/to/chart
```

The command exits with status 0 when every schema file in the chart tree matches
the generated output. It exits with status 1 and lists each file that differs or is missing.
A generation error also exits with status 1. Put `--check` before the chart path.
See [Check committed schemas in CI](#check-committed-schemas-in-ci) for complete
GitHub Actions and GitLab CI jobs for a chart in `.helm/`.

Generation uses only local chart files. It does not download dependencies or contact a Kubernetes cluster.

Generation validates each schema against the chart's effective defaults. Run
`helm lint` after generation to validate an override or the chart's complete
values tree.

## Use with werf or Nelm

Select a deployment profile before the chart path:

```sh
bin/helm-schema --engine werf ./path/to/project
bin/helm-schema --engine nelm ./path/to/chart
bin/helm-schema --engine werf --check ./path/to/project
```

The werf profile accepts either a chart directory or a project containing
`.helm/Chart.yaml`. A chart in the supplied directory takes precedence.
For a custom werf chart directory, pass that directory explicitly.

Both profiles permit runtime service values under `werf`, `global.werf`,
`global.env` (the legacy environment value injected by werf), and
`dockerconfigjson` in the root chart and its unpacked dependencies. These
namespaces are engine-owned and intentionally unconstrained, including when
values.yaml contains placeholders. Other values retain the same strict rules
as the default `--engine helm` profile; the profile does not open the values root.

Generation remains offline: it does not run werf/Nelm, build images, decrypt
secret-values.yaml, or load environment-specific overrides. Extra template
functions are parsed without execution; their value arguments use the analyzer’s
generic function rules. Charts must contain Chart.yaml and use Helm-compatible
dependency metadata; werf-specific export-values metadata is not interpreted.
Use the standalone binary; the packaged plugin targets Helm 4, not the werf/Nelm
plugin interfaces.

## Check committed schemas in CI

Add a separate job to your chart repository to check `.helm/values.schema.json`
on every push and pull request. Check the committed files directly: do not
generate schemas before `--check`, because that would hide stale or missing files.
No Go toolchain, Kubernetes cluster, kubeconfig, or Helm/werf/Nelm installation
is needed. Both examples use this fork’s container image.

### Image version

The examples target the upcoming **0.2.0** release. They become runnable after
`ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0` has been published;
the `0.1.0` image does not support `--check` or `--engine`. For testing before
release, build the same image locally:

```sh
docker build --build-arg VERSION=0.2.0 \
  -t ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0 .
```

Run that build from this generator’s source checkout, not your chart repository.
A local build is available only to that Docker daemon; hosted CI runners need
the published image. Pin the same image version (or immutable image digest)
and engine locally and in CI: upgrades can change generated output. The GHCR
package must be public for these examples to work without registry credentials.

### GitHub Actions

Create `.github/workflows/chart-schema.yml` in the repository containing `.helm/`:

```yaml
name: Chart schema

on:
  push:
  pull_request:

permissions:
  contents: read

jobs:
  chart-schema:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - name: Check committed schemas
        run: |
          docker run --rm \
            --mount "type=bind,source=${GITHUB_WORKSPACE},target=/chart,readonly" \
            ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0 \
            --check .helm/
```

### GitLab CI

Add this job to `.gitlab-ci.yml`. GitLab checks out your chart repository before
running the script. It uses the default `test` stage; if you define a custom
`stages` list without `test`, add that stage or choose an existing stage. This
example assumes a Linux Docker runner; it does not need Docker-in-Docker.
Existing pipeline-level `workflow: rules` must permit the desired pipelines.

```yaml
chart-schema:
  stage: test
  image:
    name: ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0
    entrypoint: [""]
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
    - if: '$CI_COMMIT_BRANCH && $CI_OPEN_MERGE_REQUESTS && $CI_PIPELINE_SOURCE == "push"'
      when: never
    - if: '$CI_COMMIT_BRANCH'
    - if: '$CI_COMMIT_TAG'
  script:
    - helm-schema --check .helm/
```

The empty GitLab entrypoint lets the runner execute its script instead of passing
it to `helm-schema`. GitLab runs the script in the checked-out repository, not
the image’s default `/chart` working directory.

If the chart has dependencies, make their files available under `.helm/charts/`
before the check, for example by restoring your existing dependency artifact.
The generator does not download dependencies and the image does not include
Helm; prepare dependencies in a separate job if needed. Missing dependencies
fail the job. Commit the root schema and schemas in unpacked dependency charts
you intend to check.

A current schema tree exits with status 0. Stale or missing schemas exit with
status 1 and list the affected files; generation errors also fail the job. The
check does not rewrite schema files. CI does not update or commit them. Configure
the job as a required check in your repository settings if it should block merging.

### Fixing a failed check locally

From your chart repository’s root, regenerate with the same image and engine,
then review and commit the resulting schema files:

```sh
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$(pwd),target=/chart" \
  ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0 .helm/
```

Generation needs a writable mount; the GitHub check uses a read-only mount.

### werf and Nelm profiles

The 0.2.0 image includes both profiles. In the GitHub example, replace the final
`--check .helm/` arguments with `--engine werf --check .helm/` or
`--engine nelm --check .helm/`. In the GitLab example, use the matching command:

```sh
helm-schema --engine werf --check .helm/
# Or, for Nelm:
helm-schema --engine nelm --check .helm/
```

Use the same `--engine` locally, omitting only `--check` when regenerating.
Flags must precede the chart path.

## Read a generated description

Each `description` keeps the `values.yaml` comment and adds the chart files that
use the value. This example uses the chart that `helm create` makes in Helm
4.2.4:

```sh
helm create my-app
helm schema ./my-app/
```

The `service.port` value reaches six files, so they become a list:

```sh
jq -r '.properties.service.properties.port.description' ./my-app/values.schema.json
```

```text
This sets the ports more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/#field-spec-ports

Used in:
- my-app/templates/NOTES.txt
- my-app/templates/deployment.yaml
- my-app/templates/httproute.yaml
- my-app/templates/ingress.yaml
- my-app/templates/service.yaml
- my-app/templates/tests/test-connection.yaml
```

Each path starts with the chart name from `Chart.yaml`.

A value that one file uses stays on the label line:

```sh
jq -r '.properties.image.properties.tag.description' ./my-app/values.schema.json
```

```text
Overrides the image tag whose default is the chart appVersion.

Used in: my-app/templates/deployment.yaml
```

A value that a named template reads names the file that defines the template:

```sh
jq -r '.properties.serviceAccount.properties.create.description' ./my-app/values.schema.json
```

```text
Specifies whether a service account should be created.

Used in:
- my-app/templates/_helpers.tpl
- my-app/templates/serviceaccount.yaml
```

An editor that uses
[yaml-language-server](https://github.com/redhat-developer/yaml-language-server)
shows this text when you hover over a key. Add a modeline comment to a values
file to select the schema. A relative path is resolved from the values file:

```yaml
# yaml-language-server: $schema=./my-app/values.schema.json
service:
  port: 8080
```

The chart's own `values.yaml` can use the same line with
`$schema=values.schema.json`. The generator does not copy this comment into a
description.

## Use the Helm 4 plugin

Prepare and install a local development plugin:

```sh
make plugin-dir
helm plugin install ./.dist/schema
helm schema ./path/to/chart
```

Installing directly from the Git repository also works when Go is installed:

```sh
helm plugin install https://github.com/AlexandrBoltris/arch-anes-helm-schema.git --verify=false
```

The install hook builds the native executable in the plugin directory. A
release package is preferred for users who do not have Go installed.

Create an unsigned package for local testing:

```sh
make plugin-package
```

Release packages must contain a native binary for their target platform.

## Download a prebuilt binary

Each `v*` tag publishes a GitHub release with archives for macOS (arm64 and
amd64), Linux (amd64 and arm64), and Windows (amd64), and a `checksums.txt`
file:

```sh
version=0.2.0
curl -LO "https://github.com/AlexandrBoltris/arch-anes-helm-schema/releases/download/v$version/helm-schema_${version}_darwin_arm64.tar.gz"
tar -xzf "helm-schema_${version}_darwin_arm64.tar.gz"
"helm-schema_${version}_darwin_arm64/helm-schema" --version
```

The 0.2.0 examples require the upcoming release to be published first.

The macOS binaries are not signed or notarized. If you download an archive with
a browser, remove the quarantine attribute before you run the binary:

```sh
xattr -d com.apple.quarantine helm-schema
```

## Use the container image

Each `v*` tag also publishes a multi-architecture (`linux/amd64`,
`linux/arm64`) image to `ghcr.io/alexandrboltris/arch-anes-helm-schema`. Mount the chart at
`/chart` and run as your own user so that the generated schemas belong to you:

```sh
docker run --rm -u "$(id -u):$(id -g)" -v "$PWD:/chart" ghcr.io/alexandrboltris/arch-anes-helm-schema:0.2.0
```

The upcoming 0.2.0 image includes `--check` and `--engine`; it must be
published before the examples can pull it. See the CI section for a local build.

On macOS, Docker Desktop, OrbStack, and Colima run the `linux/arm64` image. The
image includes `/bin/sh`, so you can also use it as a CI job image and run
`helm-schema path/to/chart` in a script.

## Development

Run all unit, plugin, and Helm lint tests:

```sh
make test
```

Run the Lean proofs, axiom audit, executable formal tests, and Go-to-Lean conformance suite:

```sh
make verify
```

This command requires `lake` and `rg`. The `formal/lean-toolchain` file selects
the exact Lean release.

Run both suites:

```sh
make test-all
```

The [design document](DESIGN.md) defines the intended behavior and known limits.
The [formal verification document](FORMAL_VERIFICATION.md) explains the implemented Lean proof boundary and its remaining limits.

Dynamic `tpl` text can refer to an unknown property of a specific values context. The generator permits direct properties in that subtree. A root context does not permit unrelated root values. The generator also analyzes default template text when the corresponding value reaches `tpl`.

An override can make dynamic `tpl` text refer to a new root property. Static analysis cannot identify that property. The generated schema rejects it without separate usage evidence.

The generator does not merge rules from existing schemas. A successful run replaces schemas in the selected chart and its unpacked dependencies.

Commented-out YAML can document a possible value, but it does not prove that Helm or a template reads that value. The schema rejects it without other evidence.

A chart can also declare a value that templates replace with a value derived
from other inputs. That superseded value is not configurable: the generated
schema preserves its default for Helm compatibility but rejects a changed
override. This can expose obsolete chart configuration.

## Code map

Each production file has one primary responsibility:

| File | Responsibility |
|---|---|
| `cmd/helm-schema/main.go` | Parses arguments, generates schemas, validates generated output, and replaces schema files. |
| `internal/helmchart/load.go` | Loads Helm 4 charts, dependencies, defaults, scopes, and metadata value flows. |
| `internal/helmchart/descriptions.go` | Reads descriptions from comments in root and dependency `values.yaml` files. |
| `internal/helmchart/templates.go` | Converts chart templates, contexts, and deferred `tpl` text into analyzer inputs. |
| `internal/helmchart/validation.go` | Validates generated schemas with Helm value processing. |
| `internal/analyze/types.go` | Defines analyzer inputs, diagnostics, value flows, and the usage tree. |
| `internal/analyze/analyze.go` | Parses template files, creates entry contexts, and starts analysis. |
| `internal/analyze/eval.go` | Evaluates control flow, variables, pipelines, and named-template output. |
| `internal/analyze/functions.go` | Defines generic behavior for Helm, Go template, and Sprig functions. |
| `internal/analyze/function_contracts.go` | Maps every pinned function to one formal operation class. |
| `internal/analyze/value.go` | Defines abstract values and field, item, and collection selection. |
| `internal/analyze/usage.go` | Converts abstract value observations into usage-tree evidence. |
| `internal/analyze/flow.go` | Maps usage through imported and global dependency values. |
| `internal/schema/schema.go` | Combines defaults and usage evidence into a Draft 7 schema with inferred property rules. |
| `formal/HelmSchema` | Defines the Lean models, direct contracts, and machine-checked proofs. |
| `formal/HelmSchema/Flow.lean` | Defines dependency value-flow paths and executable propagation. |
| `formal/HelmSchema/Proofs/Flow.lean` | Proves prefix, direction, mode, and propagation properties. |
| `formal/Verify/Main.lean` | Checks the versioned Go-to-Lean conformance protocol. |
| `formal/Audit/Main.lean` | Rejects unexpected axioms in compiled formal declarations. |
| `internal/formalconformance/conformance_test.go` | Compares Go, Lean, and Helm 4 semantics at the trusted boundary. |
| `plugin.yaml` | Defines the Helm 4 `cli/v1` plugin. |

## Test data

The `testdata` directory contains small charts created for this project. Each chart isolates behavior that the tests assert.

Full third-party charts are acceptance inputs, not permanent fixtures. This keeps tests stable and prevents large licensed copies in the repository.

Acceptance runs use temporary chart copies because generation replaces `values.schema.json`.
