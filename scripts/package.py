#!/usr/bin/env python3
"""Build native MyGO packages, preserve executable modes, and write checksums."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = {"windows/amd64": "windows-amd64", "darwin/arm64": "macos-arm64", "darwin/amd64": "macos-amd64"}


def run(*args, env=None):
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True)


def collect_licenses(go):
    dest = ROOT / "licenses"
    dest.mkdir(exist_ok=True)
    modules = json.JSONDecoder()
    raw = run(go, "list", "-m", "-json", "all")
    while raw.strip():
        info, end = modules.raw_decode(raw.lstrip())
        raw = raw.lstrip()[end:]
        directory = info.get("Dir")
        if not directory:
            continue
        for pattern in ("LICENSE*", "COPYING*", "NOTICE*"):
            for source in Path(directory).glob(pattern):
                if source.is_file():
                    name = info["Path"].replace("/", "_") + "-" + source.name
                    shutil.copyfile(source, dest / name)
    goroot = Path(run(go, "env", "GOROOT").strip())
    shutil.copyfile(goroot / "LICENSE", dest / "Go-LICENSE")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--go", default=shutil.which("go") or "go")
    parser.add_argument("--targets", default=",".join(TARGETS))
    args = parser.parse_args()
    selected = args.targets.split(",")
    if any(t not in TARGETS for t in selected):
        parser.error("targets must be windows/amd64, darwin/arm64, or darwin/amd64")
    go = str(Path(args.go).resolve())
    collect_licenses(go)
    env = dict(os.environ, CGO_ENABLED="0")
    env["GOFLAGS"] = (env.get("GOFLAGS", "") + " -buildvcs=false").strip()
    env["PATH"] = str(Path(go).parent) + os.pathsep + env.get("PATH", "")
    subprocess.run([go, "tool", "mygo", "build", "-platform", args.targets, "-skip-dmg", "-skip-notarize"], cwd=ROOT, env=env, check=True)
    version = json.loads((ROOT / "mygo.json").read_text())["version"]
    output = ROOT / "dist"
    checksums = []
    for target in selected:
        native = output / "native" / target.replace("/", "-")
        archive = output / f"RepoDock-{version}-{TARGETS[target]}.zip"
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for file in sorted(native.rglob("*")):
                if file.is_file() and file.suffix not in (".dmg", ".zip") and "Setup" not in file.name:
                    z.write(file, file.relative_to(native))
            z.write(ROOT / "README.md", "README.md")
        checksums.append(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}")
        print(f"{archive} ({archive.stat().st_size / 1024 / 1024:.1f} MiB)")
    checksum = output / "SHA256SUMS.txt"
    existing = {}
    if checksum.exists():
        for line in checksum.read_text().splitlines():
            if "  " in line:
                digest, name = line.split("  ", 1)
                existing[name] = digest
    for line in checksums:
        digest, name = line.split("  ", 1)
        existing[name] = digest
    checksum.write_text("".join(f"{digest}  {name}\n" for name, digest in sorted(existing.items())))


if __name__ == "__main__":
    main()
