#!/bin/sh
# Installs lanstream (https://github.com/Abinsanth/stream-video-lan) into ~/.local/bin.
#
#   curl -fsSL https://raw.githubusercontent.com/Abinsanth/stream-video-lan/release/install.sh | sh
#
# It adds the install folder to your PATH in your shell's startup file (once, marked with a
# comment; `lanstream --uninstall` removes it). Set LANSTREAM_NO_MODIFY_PATH=1 to skip that, or
# LANSTREAM_INSTALL_DIR to install somewhere else. Running it again updates lanstream.
set -eu

fail() {
	echo "$*" >&2
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "lanstream supports macOS and Linux only." ;;
esac
case "$(uname -m)" in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) fail "There is no lanstream build for $(uname -m)." ;;
esac
command -v curl >/dev/null 2>&1 || fail "curl is needed to install lanstream."

asset="lanstream_${os}_${arch}.tar.gz"
base="${LANSTREAM_DOWNLOAD_BASE:-https://github.com/Abinsanth/stream-video-lan/releases/latest/download}"
dir="${LANSTREAM_INSTALL_DIR:-$HOME/.local/bin}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT TERM # stop here on Ctrl-C (the EXIT trap still cleans up)

echo "Downloading $asset..."
curl -fsSL "$base/$asset" -o "$tmp/$asset" || fail "Download failed: $base/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "Download failed: $base/checksums.txt"

# Check the archive against its published SHA-256 before touching anything.
grep "  $asset\$" "$tmp/checksums.txt" >"$tmp/expected" || fail "Checksum mismatch: nothing was installed."
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$tmp" && sha256sum -c expected) >/dev/null 2>&1 || fail "Checksum mismatch: nothing was installed."
else
	(cd "$tmp" && shasum -a 256 -c expected) >/dev/null 2>&1 || fail "Checksum mismatch: nothing was installed."
fi

tar -xzf "$tmp/$asset" -C "$tmp" lanstream 2>/dev/null || fail "The download is damaged: nothing was installed."
mkdir -p "$dir"
# Copy next to the target, then rename: the swap is atomic and a running lanstream keeps working.
cp "$tmp/lanstream" "$dir/.lanstream.new"
chmod 755 "$dir/.lanstream.new"
mv -f "$dir/.lanstream.new" "$dir/lanstream"

version="$("$dir/lanstream" --version 2>/dev/null || true)"
echo "Installed ${version:-lanstream} to $dir/lanstream"

marker="# Added by lanstream installer"
pathline() { # $1 = shell kind
	shown="$dir"
	[ "$dir" = "$HOME/.local/bin" ] && shown='$HOME/.local/bin'
	if [ "$1" = fish ]; then
		echo "fish_add_path $dir"
	else
		echo "export PATH=\"$shown:\$PATH\""
	fi
}

case ":$PATH:" in
*":$dir:"*) ;; # already on PATH: nothing to do
*)
	kind=sh
	case "${SHELL:-}" in
	*/zsh) kind=zsh rc="$HOME/.zshrc" ;;
	*/bash)
		kind=bash rc="$HOME/.bashrc"
		[ "$os" = darwin ] && rc="$HOME/.bash_profile"
		;;
	*/fish) kind=fish rc="$HOME/.config/fish/conf.d/lanstream.fish" ;;
	*) rc="$HOME/.profile" ;;
	esac
	echo
	if [ -n "${LANSTREAM_NO_MODIFY_PATH:-}" ]; then
		echo "Add it to your PATH:"
		if [ "$kind" = fish ]; then
			echo "  $(pathline fish)"
		else
			echo "  echo '$(pathline sh)' >> $rc"
		fi
	else
		if ! grep -qs "^$marker\$" "$rc"; then
			mkdir -p "$(dirname "$rc")"
			# Start on a fresh line even if the file doesn't end with one.
			if [ -s "$rc" ] && [ -n "$(tail -c 1 "$rc")" ]; then echo >>"$rc"; fi
			printf '%s\n%s\n' "$marker" "$(pathline "$kind")" >>"$rc"
		fi
		echo "Added $dir to your PATH in $rc"
	fi
	echo "Open a new terminal, then run: lanstream"
	exit 0
	;;
esac
echo "Run: lanstream"
