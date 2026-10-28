# hfp - all in one

[中文](README.md) · [English](README.en.md)

**High-Performance · Local-First · AI-Native Video Screening Room & Infinite Canvas Studio**

hfp - all in one is an AI video and short-film workspace derived from [BeefTV](https://github.com/glanderness/BeefTV). Its visual design follows the Ready Library art direction: a screening-room desktop, mixed typography, glass surfaces and orange interaction accents.

## Unofficial derivative notice

This repository, [ALBEDO-TABAI/hfp-tv](https://github.com/ALBEDO-TABAI/hfp-tv), is an independently modified derivative. It is not an official BeefTV release and is not endorsed by or commercially authorized by BeefTV.

The derivative changes the product branding and visual interface while retaining the upstream local workspace, canvas, media, provider and assistant architecture. Feature descriptions are in [FEATURES.md](docs/FEATURES.md), and the visual specification is in [DESIGN.md](docs/DESIGN.md). These descriptions do not replace runtime acceptance testing for a particular build.

## Workspace

- An infinite canvas for images, video, audio, text, storyboards, workflows and the 3D director workspace.
- Media preview and editing tools, image processing, timeline editing and local export.
- A creative assistant using the official Pi Durable runtime, with skills, tool permissions and explicit generation confirmation.
- An independent creation workspace, project and asset libraries, model settings, plugins and external Agent connections.
- Local project data, resources and task history; model generation uses the channels configured by the user.

See the [repository map](docs/repository-map.md) for actual page routes and ownership of each capability.

## Local development

Requirements: Go 1.25+, Bun 1.4.4+, Node.js 24.15+, a C compiler for SQLite, and the plugin packaging tools described in the [local development guide](docs/local-development.md).

Follow that guide to install frontend and assistant dependencies, build protocol packages, configure the assistant and start the backend before the frontend. The browser entry is `http://127.0.0.1:43123`; development data stays in the Git-ignored `.local/project-workbench-debug` directory.

The development identity proxy is restricted to eligible requests from the local page. It is not a public deployment setup. Local readiness, provider connectivity and successful artifact delivery are separate acceptance checks.

## Development documentation

- [Repository map](docs/repository-map.md): directories, entry points and data flow.
- [Development rules](CONTRIBUTING.md): scope, verification and contribution workflow.
- [Local setup and restart](docs/local-development.md): commands, identity and data preservation.
- [Troubleshooting](docs/troubleshooting.md): canvas saves, plugins, assistant startup and provider failures.
- [Documentation index](docs/index.md) and [agent instructions](AGENTS.md).

Detailed developer documents are maintained in Chinese. Historical upstream verification reports apply only to the versions and environments they describe.

## License and acknowledgements

The repository retains the upstream [LICENSE](LICENSE): Part A, **BeefTV Source Available License 1.0**, and Part B, the historical **MIT License**. See [NOTICE](NOTICE), [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [licenses/](licenses/) for attribution and third-party terms.

The Ready Library typography and visual materials retain their respective font and component licenses in [licenses/ui/](licenses/ui/).
