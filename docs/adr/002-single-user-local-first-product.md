# ADR-002 — Single-User Local-First Product

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-002 — Single-User Local-First Product,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Revolvr is designed for one operator on a local machine, not for a multi-user service.

## Decision

Build Revolvr exclusively for one local operator. Do not create user, organization, team, or membership tables, or a remote authentication system. A small local API secret may protect the loopback API from accidental local access, but it is transport protection rather than a user-account system.

## Consequences

- Multi-user identity, tenancy, and remote authentication are outside the architecture.
- Loopback API protection may exist without expanding into account management.
