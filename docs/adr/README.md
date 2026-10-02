# Architecture Decision Records

Use ADRs for decisions that affect structure, dependencies, operations, security, or long-term maintenance.

Suggested format:

```markdown
# ADR NNNN: Title

## Status

Proposed | Accepted | Superseded

## Context

What forces or constraints led to this decision?

## Decision

What did we choose?

## Consequences

What trade-offs follow from this choice?
```

## Accepted decisions

- [ADR 0001](0001-bootstrap-full-stack.md): bootstrap full stack
- [ADR 0002](0002-stripe-payment-provider.md): Stripe payment provider
- [ADR 0003](0003-no-global-reputation.md): no global reputation
- [ADR 0004](0004-event-discovery-ahead-of-first-cut.md): isolate alpha discovery
- [ADR 0005](0005-subcult-os-platform-core.md): make Subcult OS the Subcult.tv platform core and extract from Subcults selectively
- [ADR 0006](0006-no-prototype-compatibility-contract.md): allow deliberate prototype-breaking redesign unless real data or external state requires migration
- [ADR 0007](0007-minimal-lexicon-admission.md): admit a minimal `tv.subcult.*` Lexicon chain for event, profile and place
- [ADR 0008](0008-excluded-social-ranking-reputation.md): keep social, ranking, alliance, streaming and reputation concepts out of Subcult OS
