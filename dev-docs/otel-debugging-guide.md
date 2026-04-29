# OpenTelemetry Debugging Guide in Ops Agent

This guide explains how the Ops Agent interacts with OpenTelemetry and provides a protocol for AI agents to find code snippets efficiently.

## Core Architecture
- The Ops Agent primarily operates as a **configuration generator** for the OpenTelemetry collector.
- **Context Specificity:** This guide applies specifically when you are debugging **OpenTelemetry code** (receivers, processors, exporters) within the Ops Agent context.
- **Ops Agent Debugging:** If the goal is to debug the Ops Agent itself (e.g., config generation, health checks), ignore this guide's search restrictions and search the main repository directories (e.g., `confgenerator/`, `internal/`, `integration_test/`).

## Where to Look First (For OTel Issues)
- **The Submodule:** Ops Agent imports OpenTelemetry via a git submodule at `submodules/opentelemetry-operations-collector`.
- **Active Components List:** Check `spec/otelopscol.yaml` in the main repository. This lists all components used by the collector and indicates if they are local or upstream.
- **Custom Component Source:** Look in `submodules/opentelemetry-operations-collector/components/otelopscol/` for components used when building the custom collector.

## Locating Upstream Code
- **The Manifest:** Check `submodules/opentelemetry-operations-collector/otelopscol/manifest.yaml` to see where all components are obtained from.
- **Upstream Repositories:**
    - **Core Components:** `https://github.com/open-telemetry/opentelemetry-collector`
    - **Non-Core Components (Kitchen Sink):** `github.com/open-telemetry/opentelemetry-collector-contrib`
    - **Gotcha (Thin Wrappers):** Code in `opentelemetry-collector-contrib` is often a thin wrapper. You MUST read the file and follow the Go `import` statements to find the actual implementation (e.g., Google-specific code often lives in `github.com/GoogleCloudPlatform/opentelemetry-operations-go`).
    - **Repository Divergence:** It is acceptable and encouraged to follow imports and references into other repositories outside the OTel organization (e.g., `github.com/prometheus/otlptranslator`) to find the core logic or solution requested by the user.

## AI Search Protocol (Follow these steps)
1. **Identify the goal:** Determine if you are debugging the Ops Agent or the OpenTelemetry collector.
2. **If debugging the Collector:**
    - Read `spec/otelopscol.yaml` to see if the component is active and where it comes from.
    - Scope your search: Use `grep_search` and set the `Includes` parameter to `submodules/opentelemetry-operations-collector/*`. Do NOT search the entire repository.
    - Trace imports: If the code leads you to the Contrib repository, do not assume the logic is there. Check the imports to find the upstream source.
3. **If debugging the Ops Agent:** Use normal search strategies across `confgenerator/`, `internal/`, `integration_test/`, etc.
4. **Target the correct branch:** For all searches, only search in the `master` or `main` branch. Ignore release branches.
