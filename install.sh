#!/bin/sh
set -eu

repo="HaaapyDay/llm-proxy"
bin_name="llm-proxy"
install_dir="${LLM_PROXY_INSTALL_DIR:-$HOME/.local/bin}"
version="${LLM_PROXY_VERSION:-latest}"

err() {
	printf 'llm-proxy install: %s\n' "$*" >&2
}

need_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		err "missing required command: $1"
		exit 1
	fi
}

download() {
	url="$1"
	out="$2"

	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$out"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$out" "$url"
	else
		err "missing required command: curl or wget"
		exit 1
	fi
}

detect_os() {
	case "$(uname -s)" in
		Linux) printf 'linux' ;;
		Darwin) printf 'darwin' ;;
		*)
			err "unsupported OS: $(uname -s)"
			exit 1
			;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64 | amd64) printf 'amd64' ;;
		arm64 | aarch64) printf 'arm64' ;;
		*)
			err "unsupported architecture: $(uname -m)"
			exit 1
			;;
	esac
}

latest_version() {
	tmp_json="$1"
	download "https://api.github.com/repos/$repo/releases/latest" "$tmp_json"
	sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1
}

checksum_for() {
	checksums_file="$1"
	archive_name="$2"
	awk -v file="$archive_name" '$2 == file { print $1 }' "$checksums_file" | head -n 1
}

sha256_file() {
	file="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{ print $1 }'
	else
		err "missing required command: sha256sum or shasum"
		exit 1
	fi
}

shell_config_file() {
	shell_name="$(basename "${SHELL:-}")"
	case "$shell_name" in
		zsh) printf '%s\n' "$HOME/.zshrc" ;;
		bash) printf '%s\n' "$HOME/.bashrc" ;;
		fish) printf '%s\n' "$HOME/.config/fish/config.fish" ;;
		*) printf '%s\n' "$HOME/.profile" ;;
	esac
}

path_entry() {
	case "$install_dir" in
		"$HOME"/*) printf '$HOME/%s' "${install_dir#"$HOME"/}" ;;
		*) printf '%s' "$install_dir" ;;
	esac
}

append_path_config() {
	config_file="$1"
	entry="$(path_entry)"

	case "$config_file" in
		*/config.fish)
			mkdir -p "$(dirname "$config_file")"
			if ! grep -F "set -gx PATH $entry \$PATH" "$config_file" >/dev/null 2>&1; then
				printf '\n# llm-proxy\nset -gx PATH %s $PATH\n' "$entry" >>"$config_file"
			fi
			;;
		*)
			if ! grep -F "export PATH=\"$entry:\$PATH\"" "$config_file" >/dev/null 2>&1; then
				printf '\n# llm-proxy\nexport PATH="%s:$PATH"\n' "$entry" >>"$config_file"
			fi
			;;
	esac
}

need_cmd uname
need_cmd basename
need_cmd grep
need_cmd head
need_cmd install
need_cmd mktemp
need_cmd mkdir
need_cmd tar
need_cmd sed
need_cmd awk

os="$(detect_os)"
arch="$(detect_arch)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

if [ "$version" = "latest" ]; then
	version="$(latest_version "$tmp_dir/latest.json")"
	if [ -z "$version" ]; then
		err "could not find the latest release version"
		exit 1
	fi
fi

case "$version" in
	v*) release_tag="$version" ;;
	*) release_tag="v$version" ;;
esac

asset_version="${version#v}"
archive_name="${bin_name}_${asset_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repo/releases/download/$release_tag"
archive_path="$tmp_dir/$archive_name"
checksums_path="$tmp_dir/checksums.txt"
extract_dir="$tmp_dir/extract"

printf 'Installing %s %s for %s/%s\n' "$bin_name" "$release_tag" "$os" "$arch"

download "$base_url/$archive_name" "$archive_path"
download "$base_url/checksums.txt" "$checksums_path"

expected_checksum="$(checksum_for "$checksums_path" "$archive_name")"
if [ -z "$expected_checksum" ]; then
	err "checksum for $archive_name not found in checksums.txt"
	exit 1
fi

actual_checksum="$(sha256_file "$archive_path")"
if [ "$actual_checksum" != "$expected_checksum" ]; then
	err "checksum mismatch for $archive_name"
	err "expected: $expected_checksum"
	err "actual:   $actual_checksum"
	exit 1
fi

mkdir -p "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir"

if [ ! -f "$extract_dir/$bin_name" ]; then
	err "$bin_name not found in release archive"
	exit 1
fi

mkdir -p "$install_dir"
install -m 0755 "$extract_dir/$bin_name" "$install_dir/$bin_name"

printf 'Installed %s to %s/%s\n' "$bin_name" "$install_dir" "$bin_name"

case ":$PATH:" in
	*":$install_dir:"*) ;;
	*)
		printf '%s is not currently on PATH.\n' "$install_dir"
		if [ -r /dev/tty ] && [ -w /dev/tty ]; then
			config_file="$(shell_config_file)"
			printf 'Add %s to PATH in %s? [y/N] ' "$install_dir" "$config_file" >/dev/tty
			read answer </dev/tty
			case "$answer" in
				y | Y | yes | YES)
					append_path_config "$config_file"
					printf 'Updated %s\n' "$config_file"
					printf 'Run this command or open a new terminal:\n'
					printf '  source %s\n' "$config_file"
					;;
				*)
					printf 'Skipped PATH update. Add this line to your shell config when ready:\n'
					printf '  export PATH="%s:$PATH"\n' "$(path_entry)"
					;;
			esac
		else
			printf 'Add this line to your shell config:\n'
			printf '  export PATH="%s:$PATH"\n' "$(path_entry)"
		fi
		;;
esac

"$install_dir/$bin_name" version
