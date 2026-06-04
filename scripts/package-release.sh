#!/bin/sh
set -eu

if [ $# -gt 1 ]; then
  printf 'usage: %s [version]\n' "$0" >&2
  exit 2
fi

version=${1:-}
if [ -z "$version" ]; then
  if command -v git >/dev/null 2>&1; then
    version=$(git describe --tags --exact-match 2>/dev/null || true)
    version=${version#v}
  fi
fi

if [ -z "$version" ]; then
  printf 'release version is required\n' >&2
  exit 2
fi

case "$version" in
  v*)
    printf 'version must not include leading v: %s\n' "$version" >&2
    exit 2
    ;;
esac

out_dir=${OUT_DIR:-dist}
platforms=${HITCH_RELEASE_PLATFORMS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"}
ldflags=${HITCH_LDFLAGS:-"-s -w -X main.version=$version"}

rm -rf "$out_dir"
mkdir -p "$out_dir/build" "$out_dir/packages"

for target in $platforms; do
  goos=${target%/*}
  goarch=${target#*/}
  build_dir="$out_dir/build/hitch_${version}_${goos}_${goarch}"
  mkdir -p "$build_dir"

  binary="$build_dir/hitch"
  if [ "$goos" = windows ]; then
    binary="$binary.exe"
  fi

  printf 'building %s/%s...\n' "$goos" "$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$ldflags" -o "$binary" ./cmd/hitch

done

python3 - "$version" "$out_dir" <<'PY'
from pathlib import Path
import hashlib
import sys
import tarfile
import zipfile

version = sys.argv[1]
out_dir = Path(sys.argv[2])
build_root = out_dir / "build"
package_root = out_dir / "packages"
package_root.mkdir(parents=True, exist_ok=True)

repo_files = [Path("README.md")]
if Path("install.sh").exists():
    repo_files.append(Path("install.sh"))

archives = []
for build_dir in sorted(p for p in build_root.iterdir() if p.is_dir()):
    name = build_dir.name
    is_windows = "_windows_" in name
    binary = build_dir / ("hitch.exe" if is_windows else "hitch")
    if not binary.exists():
        raise SystemExit(f"missing built binary: {binary}")

    if is_windows:
        archive = package_root / f"{name}.zip"
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            zf.write(binary, arcname=binary.name)
            for path in repo_files:
                zf.write(path, arcname=path.name)
    else:
        archive = package_root / f"{name}.tar.gz"
        with tarfile.open(archive, "w:gz") as tf:
            tf.add(binary, arcname="hitch")
            for path in repo_files:
                tf.add(path, arcname=path.name)

    archives.append(archive)

checksum_path = package_root / "checksums.txt"
with checksum_path.open("w", encoding="utf-8") as f:
    for archive in sorted(archives):
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        f.write(f"{digest}  {archive.name}\n")

print(f"packaged {len(archives)} archives for hitch {version}")
PY
