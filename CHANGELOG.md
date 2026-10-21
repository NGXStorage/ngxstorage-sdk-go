# Changelog

## Unreleased

### Added

- `IsCanonicalVolumeID` validates the volume-ID format accepted by NGX API v2 filters.
- `PortalGroups().ListDetail` reads GET /api/v2/portal_group/list (listen_ips).
- `InitiatorService.GetFC`/`GetISCSI` accept an empty id and use the bare
  /api/v2/initiators/{fc,iscsi} routes.

### Contract fixes (verified against the deployed API v2)

The following operations sent or decoded fields the backend rejects or never
returns; each is pinned by a mocked wire-contract test:

- `Shares().Create`: an empty `BlockSize` is now omitted (the create route
  rejects `""` with 400/5008; backend default 128k applies), and
  `FlashCache`/`DramCache`/`FlashTierLimit` — which the create route does not
  declare — are applied with a follow-up modify using the canonical modify
  keys (`dram_cache`, `flash_cache`, `flashtier_blocksize_limit`).
- `Share` now decodes the real wire shape: identity, `owner`, `created`,
  `share_ips`, and the attributes nested under `capacity` and `storage`
  (the old top-level `blocksize`/`deduplication`/... fields were never
  populated by the backend). `Capacity` quotas decode to nullable
  `*FlexBytes`.
- `Snapshots().CreateSchedule(volumeID, cronTime, keepMax)` sends the backend
  contract (`cron_time` + `keep_max`, keep_max 1..512) and returns the
  human-readable cron text (`h_read`). `SnapshotSchedule` now mirrors the
  schedules detail document (`id`, `owner`, `h_read`, `c_time`, `keep_max`,
  `volume_id`, `volume_type`).
- `FCTargets().CreateInitiatorTag(initiatorNAA, tag)` sends `initiator_naa`
  (16 hex chars) + `tag`, the fields the backend requires; `FCInitiatorTag`
  decodes `{id, initiator_naa, tag, owner}`.
- `Pools().Overview` returns `[]map[string]interface{}` (the backend replies
  with a bare array whose entries lack the pool id/name).
- `Status().OverloadedVolumes` returns `*OverloadedVolumeReport`
  (`{overloaded_volumes, last_check_time}` — an object, not an array).
- `Users().Create` and `AuthGroups().Create` decode the numeric `id` ack
  (json.Number → decimal string).
- `ISCSITargets().Create` returns `error` — the backend acknowledges with
  only `{"success": true}`; resolve the created target id from `ListNames`
  by name (the previous shape decoded an always-empty target document).
- `ISCSITarget.Luns[].Number` now decodes the SCSI LUN number from either a
  JSON integer or a quoted string (the backend serializes it as a number);
  the field stays a `string` for callers. `ISCSITargetInitiator`
  also exposes the `alias` and `ip` fields the detail list returns.
- The CLI `create-share` sends `permissions: "111111111"` (the 9-bit binary
  rwx string); the previous `"rwx"` always failed validation (400/5008).

### Changed

- Breaking: `SnapshotSchedule`/`FCInitiatorTag` shapes, the
  `CreateSchedule(volumeID, cronTime, keepMax)` and
  `CreateInitiatorTag(initiatorNAA, tag)` signatures, the `Share` field
  layout, `ISCSITargets().Create` (now returns `error`),
  `Pools().Overview` and `Status().OverloadedVolumes` return types now
  match the deployed wire contract; callers of the old shapes must migrate
  (the old calls were rejected or silently empty against the backend).
- Retry is caller-owned: the SDK no longer performs generic same-controller
  transport retry (refused/EOF/reset). `Config.MaxRetries` bounds 725-busy
  attempts only. Safe GET/DELETE keep at most one cross-controller replay
  after a transport failure; POST/PUT/PATCH are never replayed.
- Server-provided error messages are redacted of the configured API key
  before they reach `APIError.Message` / `Error()` (a server or proxy can echo
  request material).
- Stricter `NewClient` controller validation (trimmed, non-empty, unique).

### Deprecated

- `APIError.Body` — never populated; always nil. Raw response bodies are not
  retained (echoed request material could leak credentials).
- `HostService` / `Client.Hosts()` — the deployed host endpoints are
  unsupported (5012 on the deployed 2.4.5 software). The released v1 wire
  behavior (`List`/`Get`) is retained unchanged for compile compatibility.
- `PortalGroups().Create` / `PortalGroups().Delete` — the deployed API has no
  portal-group create/delete routes; the calls return 5012. Retained for
  released v1 compile compatibility only.

### Compatibility debt (recorded, not repaired)

- `AuthGroup.ChapStatus` changed from `bool` to `FlexBool` after the v1.0.0
  release (both accept the JSON encodings seen in the field, but Go code that
  typed the field as `bool` needs adjustment). Deliberately left unrepaired
  in this series per the owner decision.
