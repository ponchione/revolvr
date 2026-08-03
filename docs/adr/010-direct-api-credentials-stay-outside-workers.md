# ADR-010 — Direct API Credentials Stay Outside Workers

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-010 — Direct API Credentials Stay Outside Workers,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Untrusted worker sandboxes must not hold long-lived credentials for remote model access.

## Decision

Keep the OpenAI API key in the trusted control plane. Do not provide long-lived OpenAI credentials to worker sandboxes. The trusted agent runtime performs model calls and brokers validated tools into sandboxes.

## Consequences

- Workers cannot call OpenAI directly with durable credentials.
- Remote model calls and tool mediation remain trusted control-plane responsibilities.
