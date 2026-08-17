# Third-party software

Mihomo Stack's source code is MIT licensed. Release container images also
redistribute the following independent programs. Those programs remain under
their own licenses; the MIT license does not replace or weaken their terms.

## Mihomo

- Project: <https://github.com/MetaCubeX/mihomo>
- Pinned release: `v1.19.30`
- License: GNU General Public License v3.0
- Corresponding source: <https://github.com/MetaCubeX/mihomo/tree/v1.19.30>

Release assets verified by `deploy/Dockerfile`:

| Platform | SHA-256 |
| --- | --- |
| linux/amd64 baseline v1 | `cbe553d0319a414bd3a372c5976a252155b2c4882b66bce88a4d6bba9571a553` |
| linux/arm64 | `58896873736d28628f66de3677c8654fa0f180662523148e136cff4f6e890069` |
| linux/arm/v7 | `7b4e8793f9f8fa4a21ea66528890ebde5a73d557673fb94d3047eebb60ad7591` |

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
