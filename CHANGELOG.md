# Changelog

## [0.1.0](https://github.com/cacack/my-family/compare/v0.0.1...v0.1.0) (2025-12-21)


### Features

* add API docs (Swagger UI) and frontend component tests ([81fc770](https://github.com/cacack/my-family/commit/81fc770b31aa1c0206e2ed1aafe7e93e1c7ab229))
* add Codecov integration and README badges ([3ea0049](https://github.com/cacack/my-family/commit/3ea004958c0dbd3e245515ec2c53ea3f745ec294))
* add family detail page and CI pipeline ([b798ce8](https://github.com/cacack/my-family/commit/b798ce833291556c7caf933f5acc1423963be451))
* add PostgreSQL integration tests, performance benchmarks, and Docker fixes ([c55c6ed](https://github.com/cacack/my-family/commit/c55c6ed66ef2773790b859bd4f1b8385d38962a2))
* add SQLite and PostgreSQL persistence with Docker deployment ([bf26cd0](https://github.com/cacack/my-family/commit/bf26cd0eee91a005d45a4bdd187ceceb07b22024))
* **ci:** add Dependabot for automated dependency updates ([621d0d8](https://github.com/cacack/my-family/commit/621d0d80d638a89054d363938bf0e53abcb19f39))
* **ci:** add GoReleaser for automated release binaries ([fa2e2c9](https://github.com/cacack/my-family/commit/fa2e2c9e93f9cddb73bd7cf8a8038761e1020e27))
* **ci:** add release-please for automated releases ([9255b18](https://github.com/cacack/my-family/commit/9255b18fffb74e1fef85087b070963b81d1f58ba))
* Genealogy MVP - Full-stack implementation with GEDCOM support ([e095512](https://github.com/cacack/my-family/commit/e095512417dc72e88d9f71687e53db16a9cf7ac2))
* implement backend MVP (Phases 1-8) ([ac9233e](https://github.com/cacack/my-family/commit/ac9233e546481b5be9ef84aea192c35dcf51f525))
* implement frontend MVP with embedded SPA ([954a9b8](https://github.com/cacack/my-family/commit/954a9b85236f590a91cf51cf73e83b71a3690e8d))
* Initial commit ([8a0c3ea](https://github.com/cacack/my-family/commit/8a0c3ea9abc5b090774ed19193e53dc6d6112130))
* **spec:** add genealogy MVP specification and project setup ([0da6d47](https://github.com/cacack/my-family/commit/0da6d4706e1fc3ded6651f0dd1fc5736657b9b95))
* **spec:** add genealogy MVP specification and project setup ([24421dd](https://github.com/cacack/my-family/commit/24421ddf2278572092d3b6a9f12fb354cc4f06e6))


### Bug Fixes

* add nosec annotation for safe SQL string formatting ([3bf932f](https://github.com/cacack/my-family/commit/3bf932f245757f810a7e777856958e4ca6342d30))
* **ci:** add go mod download before govulncheck ([c01b268](https://github.com/cacack/my-family/commit/c01b268b1910d2780477611b4527fda37bc7b282))
* **ci:** add placeholder for embedded files in security job ([b2c61b1](https://github.com/cacack/my-family/commit/b2c61b18857dd4d6f005a2d4ccf433a6a601c1f6))
* **ci:** create placeholder for embedded web files ([4bc8ee3](https://github.com/cacack/my-family/commit/4bc8ee3b41603f6c35c25082e4e037f56c00c11c))
* **ci:** exclude G104 from gosec (unhandled errors) ([fbe5708](https://github.com/cacack/my-family/commit/fbe57085a76b940dd046479da79cec71be8aa0ba))
* **ci:** upgrade Go version to 1.24 in all jobs ([b2989a8](https://github.com/cacack/my-family/commit/b2989a89dc026d91b6c358a136f24667a44bcc94))
* **ci:** use Go 1.24 for security job to match go.mod ([c09b848](https://github.com/cacack/my-family/commit/c09b848743adf07b767e72fa7f6afa5ea8adf04e))
* **ci:** use release-please manifest mode ([555f6ac](https://github.com/cacack/my-family/commit/555f6acef75166cfad798c553f545f9acbcf82b5))
* **docs:** correct API docs URL and license text in README ([5058ede](https://github.com/cacack/my-family/commit/5058ede057c6377ab76ad5834e70c47f3597b4a8))
* preserve empty surnames in GEDCOM import/export round-trip ([a8f3597](https://github.com/cacack/my-family/commit/a8f35977732d87b4b59e0c90c7bac2598f019eb4))
* **release:** remove bump-patch-for-minor-pre-major ([0cc58f3](https://github.com/cacack/my-family/commit/0cc58f3f0d824e3f1a07735632b0fb2e6b56b091))
* **release:** set manifest to 0.0.1 to fix pre-major versioning ([625223b](https://github.com/cacack/my-family/commit/625223bea0eabd645ad7a29f3df274ebbe096926))
* resolve CI pipeline failures ([8f01dfd](https://github.com/cacack/my-family/commit/8f01dfd2d66b4b38e771bb58ae52f7201b33cad7))
* use goreleaser ldflags for dynamic version injection ([8531f28](https://github.com/cacack/my-family/commit/8531f28b8157c30434173388c107fc6cbfd3920e))
* **web:** use class-based ResizeObserver mock to fix flaky tests ([2a2d938](https://github.com/cacack/my-family/commit/2a2d938160fb7783aceb2e76d6b4cb43c7ec176b))
