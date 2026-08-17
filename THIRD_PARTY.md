# Third-party software

Mihomo Stack's source code is MIT licensed. Release container images also
redistribute the following independent programs. Those programs remain under
their own licenses; the MIT license does not replace or weaken their terms.

## Mihomo

- Project: <https://github.com/MetaCubeX/mihomo>
- Pinned release: `v1.19.30`
- Pinned source commit: `ac017cdd246ce8bd547653d927e7bf77d7ee73d5`
- License: GNU General Public License v3.0
- Corresponding source: <https://github.com/MetaCubeX/mihomo/tree/v1.19.30>

`deploy/Dockerfile` verifies that the release tag resolves to the pinned commit,
then builds Mihomo from source for each target platform with Go 1.26 and the
upstream `with_gvisor` build tag. The container build updates
`golang.org/x/crypto` to `v0.53.0`, `golang.org/x/net` to `v0.56.0`, and
`golang.org/x/text` to `v0.39.0` before compiling to incorporate upstream
security fixes. The resulting binary reports version `v1.19.30-stack.2`.

## MetaCubeXD

- Project: <https://github.com/MetaCubeX/metacubexd>
- Pinned release: `v1.273.0`
- License: GNU General Public License v3.0
- Corresponding source: <https://github.com/MetaCubeX/metacubexd/tree/v1.273.0>
- `compressed-dist.tgz` SHA-256:
  `076e05d2e3dc6641a0ec281aa4b97a18193fbcc379d139762c32d90adb22793c`

## Go and web dependencies

The source distributions and generated SBOM enumerate Go modules and npm
packages with their versions. Their copyright and license terms remain with
their respective authors. Run `go list -m all` and `npm --prefix web ls` for
the dependency inventory, or download the SPDX SBOM attached to a release.

## Optional subconverter image

- Project: <https://github.com/tindy2013/subconverter>
- Pinned release: `v0.9.0`
- License: GNU General Public License v3.0
- Corresponding source: <https://github.com/tindy2013/subconverter/tree/v0.9.0>

The binary is not present in the Guardian or Mihomo images. A separate optional
image is built for the Compose `converter` profile:

| Platform | SHA-256 |
| --- | --- |
| linux/amd64 | `884a6d1168267eba076fcdd5171215bacf98c17948ab526e4cbbdcad5f7a0217` |
| linux/arm64 | `0914688a0af211360271a4eef8a731f09852b47edf094d3758070b660544659e` |
| linux/arm/v7 | `fd1e6f41616be6948fd988b46c3de81ac7c70bf7470d9b029f9e163c86cdb50f` |
