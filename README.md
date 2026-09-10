# NGX Storage Manager API v2 SDK — Go

Unified Go client for the NGX Storage Manager API v2. One client owns **auth,
TLS, bounded 725-busy retry, controller failover, and work-mode selection**;
every driver (CSI FC, CSI iSCSI, CSI NFS) imports this SDK instead of writing
its own NGX HTTP client.

It mirrors the Python SDK (`ngxstorage`) with the same architecture and
canonical backend fields.

## Features

- API-key auth (Bearer) — never logged
- Controller failover with work-mode resolution (`single-master`, `master-ready`, `cluster`)
- Bounded 725-busy retry (6 attempts, 10–30s exponential backoff)
- Transport failover replays safe methods (GET/DELETE) only — **POST never replayed**
- Optional TLS verification (skipped by default — NGX Storage Arrays are self-signed)
- Pluggable `Logger` and `RoundTripper` middleware chain
- Per-resource services covering every API v2 group

## Install

```bash
go get github.com/ngxstorage/ngxstorage-sdk-go
```

Requires Go 1.23 or newer. Inside the NGX workspace the driver `go.mod` uses a
local replace until a tagged release is consumed:

```
replace github.com/ngxstorage/ngxstorage-sdk-go => ../../../../API/v2/SDK/go
```

## Quick start

```go
package main

import (
    "context"

    "github.com/ngxstorage/ngxstorage-sdk-go/pkg/ngxstorage"
)

func main() {
    ctx := context.Background()

    client, err := ngxstorage.NewClient(ngxstorage.Config{
        Controllers:        []string{"192.168.1.201", "192.168.1.202"},
        APIKey:             "your-api-key",
        PoolName:           "pool1",
        InsecureSkipVerify: true, // self-signed NGX Storage Arrays
    })
    if err != nil {
        panic(err)
    }

    // Resolve the serving controller + pool once before serving traffic.
    if err := client.RefreshController(ctx); err != nil {
        panic(err)
    }

    // LUN (block)
    lun, err := client.LUNs().Create(ctx, "vol1", 100)

    // NFS share, created with its export disabled
    share, err := client.Shares().Create(ctx, ngxstorage.ShareCreateRequest{
        Name:           "share1",
        SoftQuotaBytes: 100 * 1024 * 1024 * 1024,
    })

    // Snapshot + clone
    snap, err := client.Snapshots().Create(ctx, lun.ID, "snap1")
    cloneID, err := client.Snapshots().Clone(ctx, snap.ID, "clone1")

    // Pool capacity / cluster status
    avail, _, err := client.Pools().GetConfiguredCapacity(ctx)
    cluster, err := client.Status().Cluster(ctx)
}
```

## Configuration

| Field | Description |
|-------|-------------|
| `Controllers` | 1–2 controller IPs/hostnames (required) |
| `APIKey` | Bearer token (required, never logged) |
| `PoolName` | Canonical pool name; empty skips pool ownership checks |
| `InsecureSkipVerify` | Skip TLS verification for self-signed NGX Storage Arrays |
| `RootCAs` | Optional customer CA bundle (set with `InsecureSkipVerify=false`) |
| `RoundTrippers` | Middleware chain wrapping the SDK transport, outermost-first |
| `Logger` | Pluggable logger (`Debugf/Infof/Warnf/Errorf`); default Nop |
| `HTTPClient` | Test/advanced transport override |
| `Timeout` | Per-request timeout (default 60s) |

## Services

| Service | Accessor | Operations |
|---------|----------|------------|
| LUN | `client.LUNs()` | `Create`, `CreateWithOptions`, `Get`, `List`, `Delete`, `Modify`, `Expand` |
| Share (NFS) | `client.Shares()` | `Create`, `Get`, `List`, `ListNames`, `Delete`, `Modify`, `Expand`, `SetExportEnabled`, `SetReadOnly` |
| Snapshot | `client.Snapshots()` | `Create`, `Get`, `List`, `ListDetail`, `Delete`, `DeleteAll`, `Clone`, `Restore`, `CreateSchedule`, `GetSchedule`, `DeleteSchedule` |
| FC target | `client.FCTargets()` | `List`, `ListDetail`, `ListNames`, `Get`, `AddLUN`, `RemoveLUN`, `ListInitiatorTags`, `GetInitiatorTag`, `CreateInitiatorTag`, `DeleteInitiatorTag` |
| iSCSI target | `client.ISCSITargets()` | `List`, `ListDetail`, `ListNames`, `Get`, `Create`, `Delete`, `AddLUN`, `RemoveLUN`, `ChangeName`, `ChangePortalGroup`, `ChangeAuthGroup` |
| Auth group | `client.AuthGroups()` | `Create`, `Get`, `List`, `ListDetail`, `Delete`, `AddCHAP`, `DeleteCHAP`, `AddIQN`, `DeleteIQN` |
| Portal group | `client.PortalGroups()` | `List`, `Get`, `Create`, `Delete` |
| Pool | `client.Pools()` | `List`, `ListDetail`, `Get`, `Overview`, `GetConfiguredCapacity` |
| Status | `client.Status()` | `Cluster`, `Services`, `Capacity`, `IOPS`, `Bandwidth`, `CPU`, `Cache`, `License`, `OverloadedVolumes`, `Enclosures` |
| Hardware | `client.Hardware()` | `Disks`, `Memory`, `MemoryTotal`, `CPU`, `Fans`, `PowerSupplies`, `Enclosures`, `List` |
| Host | `client.Hosts()` | `List`, `Get` |
| Initiator | `client.Initiators()` | `List`, `GetFC`, `GetISCSI` |
| Log | `client.Logs()` | `Alerts`, `Audit`, `SystemEvents` |
| Services (NFS) | `client.Services()` | `NFSStatus`, `NFSStart`, `NFSStop`, `NFSRestart`, `NFSSettings` |
| Info / Network / User | `client.Info()`, `client.Network()`, `client.Users()` | `System`, `Info`, `List`/`Get` |

## CLI

The module ships a small CLI:

```bash
go run ./cmd/ngxstorage status  --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage pools   --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage luns    --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage shares  --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage fctargets  --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage snapshots  --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage create-lun <name> <sizeGB>
go run ./cmd/ngxstorage delete-lun <id>
go run ./cmd/ngxstorage create-share <name> <sizeGiB>
go run ./cmd/ngxstorage set-share-readonly <id> <true|false>
go run ./cmd/ngxstorage delete-share <id>
go run ./cmd/ngxstorage create-snapshot <volume-id> <name>
go run ./cmd/ngxstorage delete-snapshot <id>
```

## Error handling

The SDK returns typed errors; classify without string matching:

```go
err := client.LUNs().Delete(ctx, id)
if ngxstorage.IsNotFound(err)        { /* idempotent success */ }
if ngxstorage.IsBusy(err)            { /* 725, already retried */ }
if ngxstorage.IsAlreadyExists(err)   { /* name conflict */ }
if ngxstorage.IsTransportError(err)  { /* network-level */ }
```

Sentinel errors: `ngxstorage.ErrPoolNotFound`, `ngxstorage.ErrClusterNotReady`.

`APIError` carries `StatusCode`, `Code`, `Message`, `Method`, `Endpoint`, and
the raw `Body`; it is never logged by the SDK itself.

## Security

- The API key and any CHAP/S3 secret are never logged (only `has_*` booleans).
- TLS verification is optional: skipped by default because NGX Storage Arrays
  use self-signed certificates and most customers have no private CA/DNS. Pass
  `RootCAs` and set `InsecureSkipVerify=false` to enforce a customer trust chain.
- POST mutations are never replayed after an ambiguous transport failure.

## Canonical backend fields

Reads use the live backend JSON contract (nested `capacity.soft_quota`,
`exports.nfs.enabled`/`read_only`, synchronous snapshot records without a
`ready` flag); writes use the canonical flat fields (`soft_quota`, `nfs_export`,
`nfs_read_only`). The SDK does not probe alternate names.

## Test

```bash
go test ./...
go test -race ./...
go vet ./...
```

## License

Apache-2.0
