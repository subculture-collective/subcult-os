# Project Maintenance

Use this guide when changing `subcult-os` shared project conventions or operational defaults.

## Maintenance principles

- Keep the project runnable with one canonical verification command.
- Prefer reusable docs, Makefile targets, and Open Pilot conventions over ad hoc workflow notes.
- Keep local-only data, generated caches, and secrets out of git.
- Add tools only when they are useful for `subcult-os`, not as unrelated experiments.

## Safe project changes

Good project-level changes include:

- issue and PR template improvements
- docs structure improvements
- generic security and contribution guidance
- Makefile targets that keep verification deterministic
- `.gitignore`, `.gitattributes`, and editor defaults
- Open Pilot workflow documentation

Avoid changes such as:

- adding production deployment assumptions
- adding unrelated stack scaffolds
- committing generated outputs or local runtime state

## Update checklist

1. Create an issue describing why `subcult-os` should change.
2. Make the change in a branch.
3. Run:

   ```bash
   make verify
   docker compose config --quiet
   ```

4. Update docs if the workflow changes.
5. Open a PR and link the issue.
6. After merge, decide whether any downstream projects need the same change.

## Propagating to downstream projects

For each downstream project:

1. Review the `subcult-os` change for relevance.
2. Apply only the useful parts.
3. Preserve project-specific verification commands.
4. Run the downstream project's canonical verification command.
5. Note any intentional divergence in downstream docs.

## Versioning convention

Use git tags when a `subcult-os` update is meaningful enough for downstream projects to target:

```bash
git tag vYYYY.MM.DD
git push origin vYYYY.MM.DD
```

Do not tag trivial typo fixes unless a derived template needs a stable reference.

## Open Pilot smoke test

After substantial template maintenance, create a small Open Pilot task that changes a docs-only file and uses a narrow test command such as:

```bash
test -f docs/reference/example.md
```

Queue it only after labels are bootstrapped and the issue body is complete.

## Dependency maintenance (MAINT-01)

### Install build scripts

State on 2026-10-02:

- Web and mobile install with pnpm 10.33.0. `web/package.json` declares it in
  `packageManager`, and Gitea CI pins the same version. `mobile/package.json`
  has no `packageManager` field.
- Neither package has a `pnpm` section, so there is no
  `onlyBuiltDependencies` or `ignoredBuiltDependencies` list. The repository
  has no `.npmrc` or `pnpm-workspace.yaml`.
- pnpm 10 does not run dependency install scripts unless a package is
  allowlisted. The only skipped script pnpm records is `esbuild@0.27.7`, in
  the `ignoredBuilds` list of both `web/node_modules/.modules.yaml` and
  `mobile/node_modules/.modules.yaml`.
- esbuild's binary comes from its platform package (for example
  `@esbuild/linux-x64@0.27.7`). Vite builds and the web and mobile tests pass
  without the skipped install script.
- Earlier logs record a mobile install that exited nonzero over the
  unapproved esbuild script. Use the pinned pnpm 10.33.0 with
  `--frozen-lockfile` (`make deps`).

Policy:

- Dependency install scripts stay disabled by default.
- Allowing a package's script, through `pnpm.onlyBuiltDependencies` or
  `pnpm approve-builds`, is a trusted-package decision for the repository
  owner. Make that change in its own pull request. Name the package, exact
  version, what the script does and why it is needed, and link the owner's
  approval.
- Do not approve builds as a side effect of an upgrade or of making a failing
  install pass. No esbuild approval has been made; the current setup does not
  need one.
- Apply the same rule to any future native dependency.

### Bumping the pinned Indigo

Indigo is pinned in `backend/go.mod` at
`v0.0.0-20260903211445-41278964ec8e` (commit
`41278964ec8e3253e70d4e919dfb8e34211c543d`). Production imports stay inside
`backend/internal/atproto`: `atproto/syntax`, `atproto/auth/oauth`,
`atproto/identity`, `atproto/lexicon`, `atproto/atcrypto` and `util/ssrf`.
`.blacktower/clonedeps.json` records the same commit for read-only source
inspection. The clone under `.blacktower/clonedeps/repos/` is git-ignored and
may be absent from a checkout.

1. On a branch, read the upstream diff between the pinned and target commits
   for the imported packages. Note any change in the OAuth store interface,
   identity-directory shape or default datetime validation.
2. Update the pin with
   `cd backend && go get github.com/bluesky-social/indigo@<commit> && go mod tidy`.
   Review every transitive module change in `go.sum` as part of the same
   reviewed unit.
3. Update the `ref` and `resolvedVersion` in `.blacktower/clonedeps.json`.
4. Run `make test-backend` and check the drift tests in
   `backend/internal/atproto`:
   - `TestSharedSyntaxConformanceFixture` and the Lexicon conformance test
     read the shared corpora in `contracts/atproto-syntax.fixtures.json` and
     `contracts/atproto-lexicon.fixtures.json`;
   - `TestEmbeddedLexiconsMatchContracts` fails if
     `backend/internal/atproto/lexicons` differs from `contracts/lexicons`;
   - `TestIdentityDirectoryHardeningFailsClosedOnSDKShapeDrift` and
     `TestOAuthFlowSurfacesIgnoredUpstreamPersistenceError` cover the local
     hardening described in
     [the AT kernel notes](development/atproto-kernel.md) and
     [upstream contributions](development/upstream.md).
5. Run `make test-db` against a disposable database for the OAuth store and
   revocation integration tests, then `make verify`.
6. If upstream fixed a gap that a local wrapper covers, remove the wrapper
   only in a separate change with its regression test kept.
7. Update the pinned version in `docs/development/atproto-kernel.md`.

Do not edit a fixture to make a new Indigo pass. A fixture change needs its
own reason and must pass both the Go and TypeScript validators.

### Bumping TypeScript and the `@atproto` packages

TypeScript is `^5.9.3` in web and `~5.9.2` in mobile; both lockfiles resolve
5.9.3. `@atproto/lexicon` and `@atproto/syntax` are pinned exactly at 0.7.6 in
web.

1. Change the specifier in the relevant `package.json` and run
   `pnpm --dir web install` or `pnpm --dir mobile install` with pnpm 10.33.0.
   Commit the lockfile change.
2. Check `ignoredBuilds` in `node_modules/.modules.yaml`. A new skipped script
   falls under the build-script policy above.
3. Run `make lint`, `make test-web`, `make test-mobile` and `make build-web`.
   `web/src/atprotoSyntaxConformance.test.ts` and
   `web/src/atprotoLexiconConformance.test.ts` read the same contract corpora
   as the Go tests.
4. Bump the `@atproto` packages separately from TypeScript. Record the new
   version in `docs/development/atproto-kernel.md`.
5. Run `make verify`.

### Tests and evidence

- Keep focused behavior tests. Do not import the old Subcults suite; rewrite a
  still-relevant invariant as a small OS test
  ([extraction inventory](development/extraction-inventory.md)).
- `make verify`, `make test-db` on a disposable database and real
  browser/device evidence are separate levels. Record each one separately
  ([verification matrix](development/verification.md)).
- Keep the Open Pilot issue and PR templates.
