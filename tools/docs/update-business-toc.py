#!/usr/bin/python3
"""Refresh and save the proposal TOC through LibreOffice UNO."""

from __future__ import annotations

import os
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import uno


def connect(pipe_name: str, process: subprocess.Popen, stderr_path: Path):
    local = uno.getComponentContext()
    resolver = local.ServiceManager.createInstanceWithContext(
        "com.sun.star.bridge.UnoUrlResolver", local
    )
    url = f"uno:pipe,name={pipe_name};urp;StarOffice.ComponentContext"
    for _ in range(100):
        if process.poll() is not None:
            detail = stderr_path.read_text(encoding="utf-8", errors="replace").strip()
            raise RuntimeError(f"LibreOffice 启动失败，退出码 {process.returncode}：{detail[-1200:]}")
        try:
            return resolver.resolve(url)
        except Exception:
            time.sleep(0.1)
    raise RuntimeError("连接 LibreOffice 超时")


def start_office(work_dir: Path, pipe_name: str) -> tuple[subprocess.Popen, Path]:
    home = work_dir / "home"
    runtime = work_dir / "runtime"
    profile = work_dir / "profile"
    home.mkdir(parents=True, exist_ok=True)
    runtime.mkdir(parents=True, exist_ok=True, mode=0o700)
    runtime.chmod(0o700)
    environment = os.environ.copy()
    environment.update({"HOME": str(home), "XDG_RUNTIME_DIR": str(runtime)})
    stderr_path = work_dir / "libreoffice.stderr.log"
    with stderr_path.open("w", encoding="utf-8") as stderr:
        process = subprocess.Popen(
            [
                "libreoffice",
                f"-env:UserInstallation={uno.systemPathToFileUrl(str(profile))}",
                "--headless",
                "--nologo",
                "--nodefault",
                "--nofirststartwizard",
                "--norestore",
                f"--accept=pipe,name={pipe_name};urp;StarOffice.ServiceManager",
            ],
            env=environment,
            stderr=stderr,
        )
    return process, stderr_path


def stop_office(process: subprocess.Popen) -> None:
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)


def refresh_once(document_path: Path, work_dir: Path, attempt: int) -> None:
    pipe_name = f"sysarmor_business_toc_{os.getpid()}_{attempt}"
    process, stderr_path = start_office(work_dir, pipe_name)
    document = None
    try:
        context = connect(pipe_name, process, stderr_path)
        desktop = context.ServiceManager.createInstanceWithContext(
            "com.sun.star.frame.Desktop", context
        )
        hidden = uno.createUnoStruct("com.sun.star.beans.PropertyValue")
        hidden.Name = "Hidden"
        hidden.Value = True
        url = uno.systemPathToFileUrl(str(document_path.resolve()))
        document = desktop.loadComponentFromURL(url, "_blank", 0, (hidden,))
        if document is None:
            raise RuntimeError("LibreOffice 无法打开 DOCX")
        indexes = document.getDocumentIndexes()
        for index in range(indexes.getCount()):
            indexes.getByIndex(index).update()
        document.store()
    finally:
        try:
            if document is not None:
                document.close(True)
        finally:
            stop_office(process)


def refresh_toc(document_path: Path, work_dir: Path) -> None:
    errors = []
    for attempt in range(1, 3):
        try:
            refresh_once(document_path, work_dir / f"attempt-{attempt}", attempt)
            return
        except Exception as error:
            errors.append(f"{type(error).__name__}: {error}")
    raise RuntimeError("；".join(errors))


def main() -> int:
    if len(sys.argv) != 2:
        print("用法：update-business-toc.py <docx>", file=sys.stderr)
        return 2
    with tempfile.TemporaryDirectory(prefix="sysarmor-business-toc-") as work_dir:
        refresh_toc(Path(sys.argv[1]), Path(work_dir))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        print(f"[business-docx][ERROR] 无法刷新中文目录：{error}", file=sys.stderr)
        raise SystemExit(1)
