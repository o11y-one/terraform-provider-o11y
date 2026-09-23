# Releasing the O11y.one provider

One signed GitHub release is the distribution source for Terraform and
OpenTofu. GoReleaser builds immutable provider archives, includes the protocol
v6 registry manifest, creates SHA-256 checksums, signs the checksum file with
GPG, and publishes the assets to GitHub. Both registries index those assets;
provider binaries are not uploaded twice.

## One-time repository setup

Everything in this section is done once, by an organization owner, in this
order. The registries derive the provider address `o11y-one/o11y` from the
repository name `terraform-provider-o11y`; the repository already carries that
name.

1. **Create the release-signing key.** Terraform Registry accepts RSA only, so
   pass the algorithm explicitly; a default `gpg --generate-key` produces an ECC
   key the registry rejects.

   ```shell
   gpg --full-generate-key --expert
   # (1) RSA and RSA, 4096 bits, expiry 2y, name "O11y One Release Signing",
   # email the maintainers' shared address, a passphrase you store in the
   # password manager.
   gpg --list-secret-keys --keyid-format long   # note the fingerprint
   gpg --armor --export-secret-keys <fingerprint> > /tmp/release-signing.asc
   gpg --armor --export <fingerprint> > /tmp/release-signing.pub
   ```

2. **Store the private key in the `release` environment.** The environment
   already exists with its deployments restricted to `v*` tags. Add the two
   secrets, then delete the exported file:

   ```shell
   gh secret set GPG_PRIVATE_KEY --repo o11y-one/terraform-provider-o11y --env release < /tmp/release-signing.asc
   gh secret set PASSPHRASE       --repo o11y-one/terraform-provider-o11y --env release   # paste the passphrase
   /bin/rm /tmp/release-signing.asc
   ```

   The required-reviewer rule on that environment cannot be added while the
   repository is private on the Team plan; the "Going public" section below
   adds it the moment the repository is public.

3. **Register the public key with Terraform Registry.** Sign in at
   <https://registry.terraform.io> with the GitHub account that owns the
   `o11y-one` organization. The registry authenticates through the "Terraform
   Registry" OAuth app; grant that app access to the `o11y-one` organization
   when GitHub asks (Organization settings > Third-party access), otherwise the
   organization never appears in the namespace or publish pickers. Then open
   **User Settings > Signing Keys** (<https://registry.terraform.io/settings/gpg-keys>),
   choose the `o11y-one` namespace, and paste `/tmp/release-signing.pub`.

4. **Register the same public key with OpenTofu** through the
   [Submit new Provider Signing Key](https://github.com/opentofu/registry/issues/new?assignees=&labels=provider-key%2Csubmission&projects=&template=provider_key.yml&title=Provider+Key%3A+)
   issue form. Use the browser form; the OpenTofu registry refuses submissions
   made by pull request, `gh`, or the API.

Do not store the private key, passphrase, or exported secret key in the
repository, release assets, workflow logs, or Terraform state.

## Repository protections

These are configured on the GitHub repository (not in this tree) and are the
reason a release cannot be cut from anywhere but a reviewed tag on `main`.
Verify them with the commands shown; re-apply them if a listing comes back empty.

| protection | what it enforces | verify |
|---|---|---|
| Ruleset `main` (branch) | pull request required, `verify` and `opentofu-protocol` checks green, no force-push, no deletion | `gh api repos/o11y-one/terraform-provider-o11y/rulesets --jq '.[].name'` |
| Ruleset `release tags` (tag, `v*`) | only repository Admin and Maintain roles can create a `v*` tag; nobody can move or delete one | same listing |
| Environment `release` | deployments only from `v*` tags; holds `GPG_PRIVATE_KEY` and `PASSPHRASE`; required reviewer added once public | `gh api repos/o11y-one/terraform-provider-o11y/environments/release` |
| Code-security configuration `Public SDK repos` | dependency graph, Dependabot alerts and security updates, private vulnerability reporting; Secret Protection (secret scanning + push protection) is switched on in it once the repository is public | `gh api repos/o11y-one/terraform-provider-o11y/code-security-configuration` |
| `SECURITY.md` | tells reporters to use GitHub's private advisory form | in this tree |
| `.github/dependabot.yml` | weekly grouped version updates for Go modules and Actions | in this tree |

Secret scanning and push protection are free on public repositories only, so
the configuration takes effect when the repository is made public. The full
history was scanned with `gitleaks git` on 2026-09-24 before that step: no
findings.

## Going public

Order matters. The server-side token confinement (o11y-api `auth`, merged
2026-09-22) must be deployed to production before customers can follow the
README, because the README tells them to mint a platform API token.

1. Confirm the production deployment carries the token confinement: call any
   RPC outside the alert services (for example a query-engine or dashboards
   method) with a platform API token in `x-o11y-key`. Expected:
   `PERMISSION_DENIED` with the message "platform API tokens are accepted only
   on alert rule, runbook, notification and SLO services". A token that reaches
   the method means the deployment predates the fix; stop here.

2. Make the repository public: **Settings > General > Danger Zone > Change
   visibility**. The rulesets, environment and code-security configuration
   survive the change.

3. Add the required reviewer to the `release` environment (available on public
   repositories):

   ```shell
   gh api -X PUT repos/o11y-one/terraform-provider-o11y/environments/release \
     --input - <<'EOF'
   {"reviewers":[{"type":"User","id":10788442}],"prevent_self_review":false,
    "deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}
   EOF
   ```

   `prevent_self_review` stays `false` while there is one maintainer; flip it
   once a second maintainer exists.

4. Confirm secret scanning, push protection and private vulnerability reporting
   switched on with the visibility change:

   ```shell
   gh api repos/o11y-one/terraform-provider-o11y \
     --jq '.security_and_analysis | {secret_scanning, secret_scanning_push_protection}'
   gh api repos/o11y-one/terraform-provider-o11y/private-vulnerability-reporting
   ```

   If they read `disabled`, switch Secret Protection on in the organisation
   configuration "Public SDK repos" (id 278536, created 2026-09-24 and already
   attached to both publishing repositories; it carries the dependency graph,
   Dependabot alerts and security updates, and private vulnerability
   reporting). Secret Protection is a paid product on private repositories and
   free on public ones, which is why it is added only now. A configuration
   edit propagates to every attached repository; the call needs a token with
   `admin:org` (`gh auth refresh -h github.com -s admin:org`):

   ```sh
   gh api -X PATCH orgs/o11y-one/code-security/configurations/278536 --input - <<'EOF'
   {"secret_protection":"enabled","secret_scanning":"enabled","secret_scanning_push_protection":"enabled"}
   EOF
   ```

5. Cut the first release (next section).

## First publication

1. `main` is green and the "Going public" steps are done.
2. Create the first release from an up-to-date local `main`. The tag ruleset
   lets only an Admin or Maintain role push a `v*` tag, so run this from a
   maintainer's checkout:

   ```shell
   ./scripts/release.sh v0.1.0
   ```

3. Approve the `release` job when GitHub asks for the environment review, then
   wait for the `Release` workflow. Verify the release contains platform
   ZIP archives, `terraform-provider-o11y_0.1.0_manifest.json`,
   `terraform-provider-o11y_0.1.0_SHA256SUMS`, and its detached `.sig`.
4. In Terraform Registry, sign in with the GitHub account that administers the
   `o11y-one` organization, select **Publish > Provider**, and choose
   `terraform-provider-o11y`. Terraform installs a release webhook; future
   GitHub releases are ingested automatically.
5. Submit the provider to OpenTofu through its
   [Submit new Provider](https://github.com/opentofu/registry/issues/new?assignees=&labels=provider%2Csubmission&projects=&template=provider.yml&title=Provider%3A+)
   issue form. The issue must be created through the browser
   form, not with a pull request, `gh`, or the GitHub API. After approval,
   OpenTofu indexes future releases from the same GitHub repository.

## Subsequent releases

Use a new semantic version for every release. Never move, replace, or rebuild a
published tag because existing lock-file checksums would stop matching.

```shell
git switch main
git pull --ff-only
./scripts/release.sh v0.2.0
```

The script requires a clean `main` equal to `origin/main`, runs tests, vet,
reachable-vulnerability scanning, and documentation generation, then pushes an
annotated tag. The tag starts the signed GitHub release workflow. Terraform's
webhook and OpenTofu's registry index then ingest the new version without a
second build or upload.

If a Terraform version does not appear, use **Resync** from the provider's
Terraform Registry settings after checking the repository release webhook.

## Surface

The provider vendors the alerts contract only: `proto/o11y_one/alerts/v1` plus
any `o11y_one/common` file it imports, and the Go bindings generated from them.
That matches the platform API token the provider authenticates with, which the
API confines to the alerting services. `scripts/check-proto-surface.sh` runs in
CI and in the release `verify` job and fails on any other vendored domain; a new
common file is admitted only by importing it from the alerts contract. The
agentic closure is published separately by the
[SDK repository](https://github.com/o11y-one/o11y-one-sdk), not by this provider.
