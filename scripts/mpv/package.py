#!/usr/bin/env python3
"""Create an MPV component archive from one verified native dependency set."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import struct
import zipfile


FRAMEWORKS = "Supersonic.app/Contents/Frameworks/"
OLD_PREFIX = "@executable_path/../Frameworks/"
SYSTEM_MAC = ("/usr/lib/", "/System/Library/")
SYSTEM_ELF = {
    "libc.so.6", "libm.so.6", "libdl.so.2", "librt.so.1", "libpthread.so.0",
    "libresolv.so.2", "libutil.so.1", "libanl.so.1", "ld-linux-x86-64.so.2",
    "ld-linux-aarch64.so.1",
}


def run(*args):
    return subprocess.check_output(args, text=True)


def elf_dependencies(path):
    return re.findall(r"\(NEEDED\).*\[(.*?)\]", run("readelf", "-d", path))


def prepare_linux(root):
    files = {path.name for path in root.iterdir()}
    for system in SYSTEM_ELF & files:
        (root / system).unlink()
    for path in root.glob("*.so*"):
        if missing := set(elf_dependencies(path)) - files - SYSTEM_ELF:
            raise ValueError(f"{path.name} is missing {sorted(missing)}")
        run("patchelf", "--set-rpath", "$ORIGIN", path)
    return "libmpv.so.2", "0.35.1", "glibc 2.36"


def macho_commands(path):
    binary = path.read_bytes()
    if binary[:4] != b"\xcf\xfa\xed\xfe":
        raise ValueError(f"expected a thin 64-bit Mach-O in {path.name}")
    count = struct.unpack_from("<I", binary, 16)[0]
    offset = 32
    for _ in range(count):
        command, size = struct.unpack_from("<II", binary, offset)
        yield command, binary[offset:offset + size]
        offset += size


def prepare_mac(root, source, architecture):
    if architecture not in {"amd64", "arm64"}:
        raise ValueError(architecture)
    with zipfile.ZipFile(source) as upstream:
        for file in upstream.infolist():
            name = file.filename.removeprefix(FRAMEWORKS)
            if file.filename.startswith(FRAMEWORKS) and re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]*", name):
                (root / name).write_bytes(upstream.read(file))

    minimum = (0, 0, 0)
    cpu = {"amd64": 0x01000007, "arm64": 0x0100000C}[architecture]
    for path in sorted(root.iterdir()):
        with path.open("rb") as f:
            if struct.unpack("<II", f.read(8)) != (0xFEEDFACF, cpu):
                raise ValueError(f"unexpected architecture for {path.name}")
        args = []
        rpaths = set()
        for command, span in macho_commands(path):
            if command in (0xC, 0xD, 0x80000018, 0x8000001F, 0x20, 0x80000023):
                start = struct.unpack_from("<I", span, 8)[0]
                name = span[start:].split(b"\0", 1)[0].decode()
                if name.startswith(OLD_PREFIX):
                    target = name.removeprefix(OLD_PREFIX)
                    if not (root / target).is_file():
                        raise ValueError(f"{path.name} is missing {target}")
                    replacement = "@loader_path/" + target
                    args.extend(["-id", replacement] if command == 0xD else ["-change", name, replacement])
                elif not name.startswith(SYSTEM_MAC):
                    raise ValueError(f"{path.name} references {name}")
            elif command == 0x8000001C:
                start = struct.unpack_from("<I", span, 8)[0]
                rpaths.add(span[start:].split(b"\0", 1)[0].decode())
            elif command in (0x24, 0x32):
                version = struct.unpack_from("<I", span, 8 if command == 0x24 else 12)[0]
                minimum = max(minimum, (version >> 16, (version >> 8) & 255, version & 255))
        for rpath in sorted(rpaths):
            args.extend(["-delete_rpath", rpath])
        if args:
            run(os.environ.get("INSTALL_NAME_TOOL", "llvm-install-name-tool-18"), *args, path)
        run(os.environ.get("RCODESIGN", "rcodesign"), "sign", path)
        if not any(command == 0x1D for command, _ in macho_commands(path)):
            raise ValueError(f"missing signature in {path.name}")
    return "libmpv.2.dylib", "0.41.0", "macOS " + ".".join(map(str, minimum[:2]))


def prepare_windows(root, architecture):
    library = root / "mpv-2.dll"
    header = library.read_bytes()
    machine = struct.unpack_from("<H", header, struct.unpack_from("<I", header, 0x3C)[0] + 4)[0]
    if machine != {"amd64": 0x8664, "arm64": 0xAA64}[architecture]:
        raise ValueError("Windows DLL machine does not match its target")
    files = {path.name.lower() for path in root.iterdir()}
    system_dll = re.compile(
        r"(api-ms-.*|ext-ms-.*|advapi32|avicap32|avrt|bcrypt|bcryptprimitives|cfgmgr32|"
        r"comctl32|comdlg32|crypt32|d2d1|d3d11|d3d12|dcomp|dwmapi|dwrite|dxgi|gdi32|"
        r"imm32|iphlpapi|kernel32|mfplat|msvcrt|normaliz|ntdll|ole32|oleaut32|opengl32|"
        r"powrprof|psapi|secur32|setupapi|shcore|shell32|shlwapi|user32|userenv|uxtheme|"
        r"version|winmm|winhttp|wintrust|wldap32|ws2_32)\.dll"
    )
    for dll in root.glob("*.dll"):
        imported = re.findall(r"^  Name: (\S+\.dll)$", run(
            os.environ.get("LLVM_READOBJ", "llvm-readobj-18"), "--coff-imports", dll
        ), re.IGNORECASE | re.MULTILINE)
        if not imported:
            raise ValueError(f"failed to inspect imports of {dll.name}")
        missing = {name for name in imported if not system_dll.fullmatch(name.lower()) and name.lower() not in files}
        if missing:
            raise ValueError(f"{dll.name} is missing {sorted(missing)}")
    versions = re.findall(rb"mpv v?([0-9]+\.[0-9]+\.[0-9]+[^\x00\r\n ]*)\x00", header)
    if not versions:
        raise ValueError("could not read Windows MPV version")
    return library.name, versions[0].decode(), "Windows 10"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("platform")
    parser.add_argument("directory", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--source", type=Path)
    args = parser.parse_args()

    system, architecture = args.platform.split("-")
    if system == "linux":
        library, version, runtime = prepare_linux(args.directory)
    elif system == "darwin":
        library, version, runtime = prepare_mac(args.directory, args.source, architecture)
    else:
        library, version, runtime = prepare_windows(args.directory, architecture)

    paths = sorted(args.directory.iterdir())
    if len(paths) > 256 or sum(path.stat().st_size for path in paths) > 768 << 20:
        raise ValueError("component exceeds installer limits")
    manifest = {
        "format": 1,
        "version": args.version,
        "mpvVersion": version,
        "platform": args.platform,
        "minimumRuntime": runtime,
        "library": library,
        "files": [
            {"name": path.name, "size": path.stat().st_size,
             "sha256": hashlib.file_digest(path.open("rb"), "sha256").hexdigest()}
            for path in paths
        ],
    }
    args.output.mkdir(parents=True, exist_ok=True)
    archive = args.output / f"mpv-{args.version}-{args.platform}.zip"
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as bundle:
        bundle.writestr("component.json", json.dumps(manifest))
        for path in paths:
            bundle.write(path, path.name)
    if archive.stat().st_size > 256 << 20:
        raise ValueError("component archive exceeds installer limit")
    print(f"{archive.name}: MPV {version}, {runtime}, {len(paths)} files")


if __name__ == "__main__":
    main()
