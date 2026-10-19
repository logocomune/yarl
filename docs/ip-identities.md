# IP identity configuration

`NewWithConfiguration(backend, Configuration{IPv6PrefixBits: 64}, rules...)`
enables IP-only keys for a limiter instance. Configuration is copied at
construction; changing the original value does not reconfigure the limiter.
`New(backend, rules...)` and a zero configuration preserve generic string keys.

## Normalization contract

- `IPv6PrefixBits` accepts `0..128`; zero disables normalization.
- Enabled mode parses a bare, unzoned IP address using `net/netip`.
- IPv4-mapped IPv6 addresses are unmapped before classification.
- IPv4 uses a canonical individual address, equivalent to a `/32` identity.
- IPv6 uses a canonical, masked CIDR prefix with the configured length.
- Invalid identities return `ErrInvalidIPIdentity` before serial or batch
  backend access. The error does not include the supplied input.
- Rule namespaces, limits, TTLs, Redis pipelines, and LRU storage remain unchanged.

Select `/64` to prevent clients from gaining new counters by rotating addresses
within one IPv6 subnet. `/56` groups 256 `/64` networks and can affect more
independent users. `/128` canonicalizes individual addresses but does not protect
against address rotation. Limits apply to the shared network identity, not a
person or device. Review thresholds for legitimate clients sharing a prefix.

## Middleware and trusted client addresses

The existing HTTP and Gin middleware support an IP-mode limiter with `UseIP`
enabled and no `Headers`. Generic keys containing user IDs or appended headers
are unsupported in IP mode. Such inputs normally fail parsing, but a composed
string can accidentally be syntactically valid IPv6; never use composite keys
with an IP-mode limiter.

The HTTP middleware currently trusts the first `X-Forwarded-For` value. Restrict
access to trusted proxies that overwrite this header, or supply a validated IP
through direct `Check` calls. Configure Gin's trusted proxies appropriately.
This feature does not change proxy trust behavior. Middleware retain their
existing HTTP 500 response for errors returned by `Check`.

## Rollout and maintenance

Use the same prefix configuration on every instance sharing a Redis namespace.
IPv6 keys change from individual addresses to CIDR identities, so new counters
start at zero. Existing counters expire through their original TTL. A mixed
rollout separates traffic between old and new identities and temporarily weakens
enforcement. Rollback can similarly resume old counters or start fresh ones.

Aggregation reduces key cardinality for rotation inside a prefix. It does not
prevent rotation across prefixes, distributed attacks, or LRU eviction under
high cardinality. LRU capacity remains a per-rule limit on tracked identities.

## Verification

```bash
go test ./...
go test -race ./...
go test -run TestIPIdentityProperties -count=1 .
go test -run '^$' -fuzz '^FuzzIPIdentity$' -fuzztime=15s .
```

Random properties cover prefix containment, canonical masking, host rotation,
and normalization stability after extracting a normalized prefix's address.
The fuzz target checks arbitrary external strings and all supported nonzero
prefix lengths. Persistent seeds reside in `testdata/fuzz/FuzzIPIdentity/`.
Redis integration tests require a server at `localhost:6379`; they skip when it
is unavailable. LRU and serial/batch unit tests do not require Redis.
