# Changelog

## Unreleased

### Added

- Add optional IP-only identities through `NewWithConfiguration` and
  `Configuration.IPv6PrefixBits`, with IPv6 prefix aggregation, canonical IPv4
  identities, and IPv4-mapped IPv6 unmapping before Redis or LRU access.
- Reject malformed or zoned IP identities before storage access in IP mode.
- Add unit, backend integration, randomized property, and fuzz tests for IP
  identities; document configuration and operational tradeoffs.
