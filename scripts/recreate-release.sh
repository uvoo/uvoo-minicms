# If v0.5.1 was already published, the clean way is only to recreate it if you are sure nobody has consumed it yet. Otherwise publish v0.5.2.

#   To recreate v0.5.1:

git status --short
git add go.mod Dockerfile web/package-lock.json
git commit -m "Bump dependencies for CVE fixes"

VERSION=v0.5.1 make unrelease
VERSION=v0.5.1 make release

# make unrelease removes the GitHub release and tag, then make release rebuilds assets, recreates the tag at the current commit, pushes it, and creates the GitHub release again.

# Afterwards verify:

gh release view v0.5.1
git ls-remote --tags origin refs/tags/v0.5.1
