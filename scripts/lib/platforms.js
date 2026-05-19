const PLATFORMS = [
  "darwin-arm64",
  "darwin-x64",
  "linux-arm64",
  "linux-x64",
  "win32-x64",
];

function tripleToGo(triple) {
  const [nodeOs, nodeArch] = triple.split("-");
  const goos = nodeOs === "win32" ? "windows" : nodeOs;
  const goarch = nodeArch === "x64" ? "amd64" : nodeArch;
  const ext = nodeOs === "win32" ? ".exe" : "";
  return { goos, goarch, ext };
}

module.exports = { PLATFORMS, tripleToGo };
