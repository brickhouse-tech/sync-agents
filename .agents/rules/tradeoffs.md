---
trigger: always_on
---

# Decision-Making: Explain Trade-offs

## Purpose
Every implementation decision has costs. Make them explicit so the human can make informed decisions. Don't hide trade-offs behind confident recommendations.

## When to Apply
- Proposing an implementation approach
- Suggesting a refactor or optimization
- Recommending a library/tool/technology
- Designing system architecture
- Choosing between multiple valid solutions

## What to Cover

### Performance
- **Execution time**: Will this be faster/slower? By how much?
- **Memory usage**: Does this trade memory for speed or vice versa?
- **Scalability**: How does this behave at 10x, 100x, 1000x current load?
- **Latency**: Does this add network calls, disk I/O, blocking operations?
- **Warm-up time**: Does this need initialization, caching, JIT compilation?

### Cost
- **Compute cost**: More CPU/memory = higher cloud bills
- **Storage cost**: Data duplication, retention, backups
- **Network cost**: Cross-region calls, data transfer fees
- **Licensing cost**: Proprietary tools, SaaS subscriptions
- **Maintenance cost**: How much ongoing effort does this require?
- **Opportunity cost**: What can't we do because we're doing this?

### Security
- **Attack surface**: Does this open new vulnerabilities?
- **Data exposure**: Where does sensitive data flow?
- **Access control**: Who can read/write/delete this?
- **Compliance**: Does this meet regulatory requirements (GDPR, SOC2, HIPAA)?
- **Audit trail**: Can we track what happened and when?
- **Secrets management**: How are credentials stored and rotated?

### Maintainability
- **Readability**: Will the next developer understand this?
- **Testability**: How easy is this to test?
- **Debuggability**: When this breaks, can we find the problem?
- **Extensibility**: How hard is it to add features later?
- **Dependencies**: Does this lock us into a specific vendor/version?
- **Documentation**: Is this self-explanatory or does it need docs?

### Correctness
- **Edge cases**: Have we handled all the boundary conditions?
- **Error handling**: What happens when things go wrong?
- **Concurrency**: Does this handle race conditions correctly?
- **State management**: Is state clear and predictable?
- **Backwards compatibility**: Does this break existing behavior?

## Format

### Pros
- List advantages clearly
- Quantify when possible ("2x faster", "saves $500/mo")
- Explain why each advantage matters

### Cons
- List disadvantages honestly
- Quantify costs ("adds 50ms latency", "requires 2 more DB queries")
- Explain the impact ("slower user experience", "higher cloud bill")

### Trade-offs Summary
- **We're trading X for Y** — be explicit
- **We gain A but lose B** — show the exchange
- **This works well when... but fails when...** — show boundary conditions

### Recommendation
- **Preferred approach**: State which option you recommend
- **Why**: Explain your reasoning
- **When to reconsider**: List conditions that would change your recommendation

## Anti-patterns to Avoid
- **Hidden costs** — don't downplay downsides to push your preference
- **Vague language** — "might be slower" → "adds ~30% latency based on benchmarks"
- **One-sided analysis** — always cover both pros and cons
- **False certainty** — acknowledge unknowns, mark assumptions
- **Deferring hard questions** — if you don't know the cost, say so and estimate
- **Over-recommendation** — present options, let the human decide

## Example

```markdown
## Approach A: Cache in Redis
Pros:
- Reduces DB load by ~80% under typical traffic
- Response time drops from 150ms to 20ms for cached queries
- Well-tested, production-ready solution

Cons:
- Adds Redis dependency (operational complexity)
- Data staleness for up to 5 minutes (cache TTL)
- Additional $50/mo infrastructure cost
- Need to implement cache invalidation logic

Trade-offs:
- We're trading consistency for performance (5-min staleness)
- We're trading simplicity for scalability (more moving parts)
- This works well for read-heavy workloads but adds complexity for writes

Recommendation:
Approach A if traffic > 100 req/sec and data can be 5 min stale.
Reconsider if write latency becomes unacceptable or Redis ops burden grows.
```
