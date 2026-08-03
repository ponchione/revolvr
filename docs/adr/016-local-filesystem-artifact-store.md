# ADR-016 — Local Filesystem Artifact Store

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-016 — Local Filesystem Artifact Store,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Large run artifacts need durable local storage without adding an object-storage service to v1.

## Decision

Store large artifacts in a content-addressed local filesystem store. Store artifact metadata and hashes in PostgreSQL. Do not add MinIO or S3 compatibility to v1. A future object-store adapter may be added without changing artifact identities.

## Consequences

- v1 artifact storage requires only the local filesystem and PostgreSQL metadata.
- Content-derived identities remain stable if a future object-store adapter is introduced.
