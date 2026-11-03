#!/bin/sh
# Tags HEAD as v<VERSION> for release.yml. Safe to re-run after a failed release:
#   - a published release with this version  -> refuse (bump VERSION on main)
#   - a draft left by an earlier failed run    -> carry on; GoReleaser replaces it
#   - the tag already on this commit           -> carry on without re-tagging
#   - the tag on a different commit            -> refuse
set -eu

v="v$(tr -d '[:space:]' <VERSION)"
state="$(gh release view "$v" --repo "$GITHUB_REPOSITORY" --json isDraft --jq .isDraft 2>/dev/null || echo none)"
case "$state" in
false)
	echo "::error::$v is already released. Bump VERSION on main first."
	exit 1
	;;
true) echo "Finishing the unpublished draft of $v left by an earlier run." ;;
esac

head="$(git rev-parse HEAD)"
if tagged="$(git rev-parse -q --verify "refs/tags/$v^{commit}")"; then
	if [ "$tagged" != "$head" ]; then
		echo "::error::Tag $v already points at another commit ($tagged). Delete that tag or bump VERSION."
		exit 1
	fi
	echo "Tag $v is already on this commit."
else
	git tag "$v"
fi
git push origin "refs/tags/$v"
