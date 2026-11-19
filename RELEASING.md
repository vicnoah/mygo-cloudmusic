# Releasing

This fork publishes releases at `vicnoah/mygo-cloudmusic`. Windows builds are
unsigned; if the fork has no macOS certificate secrets, macOS builds are also
unsigned. Update manifests are signed when `MYGO_UPDATER_PRIVATE_KEY` is
available.

1. Commit pending work, bring the fork's `main` branch up to date, and run `go test ./internal/...`:

   ```bash
   git switch main
   git pull --rebase origin main
   ```

2. Bump `version` in `mygo.json`, and add a matching section at the top of `CHANGELOG.md` saying, in Chinese, what changed for people using the app since the last release. Commit both in a commit titled just the version, e.g. `0.2.4`. MyGo's updater shows that section in the app's update window, and the release page takes it as its notes; `mygo build` fails without one.
3. Push `main` first, and tag once that push has gone through. Pushing both at once can start a release build even if the branch push is rejected:

   ```bash
   git push origin main
   git tag v0.2.4
   git push origin v0.2.4
   ```

   If `main` is rejected, pull again, add what came in to the changelog, amend the version commit, test, and push `main` again before tagging. If a tag went up on the wrong commit anyway, cancel its run, then delete the tag and the draft it made before tagging again:

   ```bash
   git push origin :refs/tags/v0.2.4
   gh release delete v0.2.4 --repo vicnoah/mygo-cloudmusic --yes
   ```

4. `.github/workflows/release.yml` builds macOS, Windows and Linux and uploads them to a draft release. Wait for every job to pass:

   ```bash
   gh run watch "$(gh run list --repo vicnoah/mygo-cloudmusic --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')" --repo vicnoah/mygo-cloudmusic --exit-status
   ```

   If a job fails, don't publish; fix it first.

5. Publish the draft and mark it latest:

   ```bash
   gh release edit v0.2.4 --repo vicnoah/mygo-cloudmusic --draft=false --latest
   ```

6. Check the update feed shows the new version:

   ```bash
   curl -sL https://github.com/vicnoah/mygo-cloudmusic/releases/latest/download/update-darwin-arm64.json
   ```

7. Deploy the website. It reads the latest release when it's prerendered, so it has to be rebuilt after publishing:

   ```bash
   cd website && bun run deploy
   ```

   Then check https://mygo-cloudmusic.egoist.workers.dev/download/ shows the new version.

## The speech recognition libraries

Podcast captions run sherpa-onnx, whose libraries the app downloads with the model from the `sherpa-onnx-<version>` release, not from the app's own. They only change with sherpa-onnx's version, in `scripts/sherpa`. Then pack them again, signing the macOS ones with the app's Developer ID (the app's hardened runtime loads no others), commit the `internal/pluginapi/speech/natives.go` it writes, and upload them as a prerelease, which never becomes the latest that updates look for:

```bash
go run ./scripts/sherpa -sign "Developer ID Application: xinyu zeng (GJE9R5VE87)"
gh release create sherpa-onnx-2.0.1 --prerelease --latest=false build/sherpa/*
```
