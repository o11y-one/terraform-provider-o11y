# Security policy

## Reporting a vulnerability

Do not open a public issue for a security problem.

Report it through GitHub's private channel: **Security > Report a vulnerability**
on this repository
(`https://github.com/o11y-one/terraform-provider-o11y/security/advisories/new`).
Only the maintainers can read what you submit there.

Include the provider version, the resource or data source involved, how to
reproduce, and what an attacker gains. Never include a real API token or
Terraform state in the report.

## What to expect

- Acknowledgement within 3 business days.
- A fix or a stated mitigation before any public disclosure, coordinated with you.
- Credit in the advisory unless you prefer none.

## Verifying what you install

Every release is built by `.github/workflows/release.yml` from a tag on `main`
and signed with the `o11y-one` GPG key registered with the Terraform Registry
and OpenTofu. `terraform init` verifies that signature; `docs/releasing.md`
describes the release process.
