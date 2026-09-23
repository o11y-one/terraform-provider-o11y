# Releasing the O11y.one provider

One signed GitHub release is the distribution source for Terraform and
OpenTofu. GoReleaser builds immutable provider archives, includes the protocol
v6 registry manifest, creates SHA-256 checksums, signs the checksum file with
GPG, and publishes the assets to GitHub. Both registries index those assets;
provider binaries are not uploaded twice.

## One-time repository setup

1. Rename the public GitHub repository to `terraform-provider-o11y`. Both
   registries derive the provider address `o11y-one/o11y` from that repository
   name. Update local remotes after the rename.
2. Create a dedicated RSA release-signing key. Terraform Registry does not
   accept the default ECC key type.
3. Create a protected GitHub environment named `release`. Add environment
   secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`. Restrict deployments to tags and,
   where available, require a maintainer approval.
4. Export the matching public key with `gpg --armor --export <fingerprint>`.
5. Upload the public key under the `o11y-one` namespace in Terraform Registry
   under **User Settings > Signing Keys**.
6. Add the same public key to OpenTofu using its
   [Submit new Provider Signing Key](https://github.com/opentofu/registry/issues/new?assignees=&labels=provider-key%2Csubmission&projects=&template=provider_key.yml&title=Provider+Key%3A+)
   GitHub issue form.

Do not store the private key, passphrase, or exported secret key in the
repository, release assets, workflow logs, or Terraform state.

## First publication

1. Merge the provider to `main` and ensure CI is green.
2. Create the first release from an up-to-date local `main`:

   ```shell
   ./scripts/release.sh v0.1.0
   ```

3. Wait for the GitHub `Release` workflow. Verify the release contains platform
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
