## [1.5.2](https://github.com/kennyparsons/go-transcribe/compare/v1.5.1...v1.5.2) (2025-08-28)


### Bug Fixes

* use caveat for vlc cask dependency ([282ee76](https://github.com/kennyparsons/go-transcribe/commit/282ee763cb645f0f7291f1ec46a26365004a405b))

## [1.5.1](https://github.com/kennyparsons/go-transcribe/compare/v1.5.0...v1.5.1) (2025-08-28)


### Bug Fixes

* correct goreleaser syntax for brew dependencies ([a341faf](https://github.com/kennyparsons/go-transcribe/commit/a341fafb61045800a1b95a39d41eacfcf924d9a9))

# [1.5.0](https://github.com/kennyparsons/go-transcribe/compare/v1.4.0...v1.5.0) (2025-08-28)


### Features

* add ffmpeg and vlc as brew dependencies ([73e7d8a](https://github.com/kennyparsons/go-transcribe/commit/73e7d8a998b478b0cf5e300581d97fd627975fc8))

# [1.4.0](https://github.com/kennyparsons/go-transcribe/compare/v1.3.0...v1.4.0) (2025-08-27)


### Features

* **setup:** implement interactive setup menus ([6d9297b](https://github.com/kennyparsons/go-transcribe/commit/6d9297b370885144467ca6fce2d6f765c6bc6de3))

# [1.3.0](https://github.com/kennyparsons/go-transcribe/compare/v1.2.1...v1.3.0) (2025-08-26)


### Features

* **models:** add quantized large-v3 model ([3228a60](https://github.com/kennyparsons/go-transcribe/commit/3228a605c5de0a537d7de0a1094583b062d4c26a))

## [1.2.1](https://github.com/kennyparsons/go-transcribe/compare/v1.2.0...v1.2.1) (2025-08-22)


### Bug Fixes

* **logs:** display progress messages while suppressing cpp output ([038cfd3](https://github.com/kennyparsons/go-transcribe/commit/038cfd3cd42abff0e394d0811fd79310b28a9f3c))

# [1.2.0](https://github.com/kennyparsons/go-transcribe/compare/v1.1.0...v1.2.0) (2025-08-22)


### Features

* **extraction:** replace ffmpeg with vlc for robust audio extraction ([fa27059](https://github.com/kennyparsons/go-transcribe/commit/fa270591dd8d6dc63e59b9559d451bd76fa4ae84))

# [1.1.0](https://github.com/kennyparsons/go-transcribe/compare/v1.0.6...v1.1.0) (2025-08-22)


### Features

* **cli:** improve usability and add output suppression ([aa87881](https://github.com/kennyparsons/go-transcribe/commit/aa878819ddb5466a2f531983dfcc1f9afa4fe271))

## [1.0.6](https://github.com/kennyparsons/go-transcribe/compare/v1.0.5...v1.0.6) (2025-08-22)


### Bug Fixes

* **build:** build only for arm64 architecture ([b642618](https://github.com/kennyparsons/go-transcribe/commit/b64261816a85810e6f30659e590ed122b8cc30d1))

## [1.0.5](https://github.com/kennyparsons/go-transcribe/compare/v1.0.4...v1.0.5) (2025-08-22)


### Bug Fixes

* **ci:** use absolute paths for CGO flags in goreleaser ([c1bddf9](https://github.com/kennyparsons/go-transcribe/commit/c1bddf9ecac262351a1ddfe77fbe3fd4535c54ff))

## [1.0.4](https://github.com/kennyparsons/go-transcribe/compare/v1.0.3...v1.0.4) (2025-08-22)


### Bug Fixes

* **build:** use local whisper.cpp submodule ([8ebe1c0](https://github.com/kennyparsons/go-transcribe/commit/8ebe1c09095e01334577a50f30ae930894e0d3d5))

## [1.0.3](https://github.com/kennyparsons/go-transcribe/compare/v1.0.2...v1.0.3) (2025-08-22)


### Bug Fixes

* **ci:** reorder setup steps to install go first ([d7aab75](https://github.com/kennyparsons/go-transcribe/commit/d7aab757c57b0e1f7cb571716719005406f0e1d6))
* **version:** implement dynamic version injection ([1194614](https://github.com/kennyparsons/go-transcribe/commit/1194614db7ae1329a4c5c82ee771718e8969f717))

## [1.0.2](https://github.com/kennyparsons/go-transcribe/compare/v1.0.1...v1.0.2) (2025-08-21)


### Bug Fixes

* **ci:** use macos runner for goreleaser build ([7e101b5](https://github.com/kennyparsons/go-transcribe/commit/7e101b5a7d60ff38f4403a6c2dbda13dd54a0a6d))

## [1.0.1](https://github.com/kennyparsons/go-transcribe/compare/v1.0.0...v1.0.1) (2025-08-21)


### Bug Fixes

* **release:** downgrade goreleaser config to v1 ([6b3df7f](https://github.com/kennyparsons/go-transcribe/commit/6b3df7f6c13fd96d4e6ec715baa606cd9b4158e1))

# 1.0.0 (2025-08-21)


### Bug Fixes

* **build:** Add missing Go source files to repository ([4bd7b94](https://github.com/kennyparsons/go-transcribe/commit/4bd7b94a255cf4b6997964c7e5d23b03154dbc04))
* **release:** configure semantic-release for Go project ([7d9b170](https://github.com/kennyparsons/go-transcribe/commit/7d9b170724e853caa09a003e7a39fa5dda5b2e8c))
* **release:** install necessary semantic-release plugins ([d3c18e6](https://github.com/kennyparsons/go-transcribe/commit/d3c18e6b9c79cb2a807524efbb065a1c2d9c6ead))
