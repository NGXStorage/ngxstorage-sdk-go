# NGX Storage Manager API v2 SDK — Go

Unified Go client for the NGX Storage Manager API v2.
One client owns **auth, TLS, bounded 725-busy retry, controller failover, and
work-mode selection**; every driver (CSI FC, CSI iSCSI, CSI NFS) imports this
SDK instead of writing its own NGX HTTP client.

Mirrors the Python SDK (`ngxstorage-sdk-python`) with the same architecture
and canonical backend fields.

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

In the NGX monorepo the driver `go.mod` uses a local replace:

```
replace github.com/ngxstorage/ngxstorage-sdk-go => ../../../../API/v2/SDK/Go
```

## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/ngxstorage/ngxstorage-sdk-go/pkg/ngxstorage"
)

func main() {
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
    if err := client.RefreshController(context.Background()); err != nil {
        panic(err)
    }

    // LUN (block)
    lun, err := client.LUNs().Create(context.Background(), "vol1", 100, "1")
    // ...

    // NFS share (created with export disabled)
    share, err := client.Shares().Create(context.Background(), ngxstorage.ShareCreateRequest{
        Name:           "share1",
        SoftQuotaBytes: 100 * 1024 * 1024 * 1024,
    })

    // Snapshot + clone
    snap, err := client.Snapshots().Create(context.Background(), lun.ID, "snap1")
    cloneID, err := client.Snapshots().Clone(context.Background(), snap.ID, "clone1")

    // Pool capacity / cluster status
    avail, reserved, err := client.Pools().GetConfiguredCapacity(context.Background())
    cluster, err := client.Status().Cluster(context.Background())
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
| LUN | `client.LUNs()` | `Create`, `Get`, `List`, `Delete`, `Modify`, `Expand` |
| Share (NFS) | `client.Shares()` | `Create`, `Get`, `List`, `ListNames`, `Delete`, `Modify`, `Expand`, `SetExportEnabled`, `SetReadOnly` |
| Snapshot | `client.Snapshots()` | `Create`, `Get`, `List`, `ListDetail`, `Delete`, `Clone`, `Restore` |
| FC target | `client.FCTargets()` | `List`, `Get`, `AddLUN`, `RemoveLUN` |
| iSCSI target | `client.ISCSITargets()` | `List`, `Get`, `Create`, `Delete`, `AddLUN`, `RemoveLUN` |
| Auth group | `client.AuthGroups()` | `Create`, `Get`, `List`, `Delete`, `AddCHAP`, `DeleteCHAP` |
| Portal group | `client.PortalGroups()` | `List`, `Get`, `Create`, `Delete` |
| Pool | `client.Pools()` | `List`, `Get`, `Overview`, `GetConfiguredCapacity` |
| Status | `client.Status()` | `Cluster`, `Services`, `Capacity`, `IOPS`, `Bandwidth` |
| Hardware | `client.Hardware()` | `Disks`, `Memory`, `CPU`, `Fans`, `PowerSupplies`, `Enclosures` |
| Host | `client.Hosts()` | `List`, `Get` |
| Info | `client.Info()` | `System` |
| Network | `client.Network()` | `Info` |
| User | `client.Users()` | `List`, `Get` |
| Log | `client.Logs()` | `Alerts`, `Audit`, `SystemEvents` |
| Initiator | `client.Initiators()` | `List`, `GetFC`, `GetISCSI` |
| Services (NFS) | `client.Services()` | `NFSStatus`, `NFSStart`, `NFSStop`, `NFSRestart`, `NFSSettings` |

## CLI

The SDK ships a small CLI:

```bash
go run ./cmd/ngxstorage status --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage pools   --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage luns    --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage shares  --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage fctargets --controllers 192.168.1.201 --api-key <key>
go run ./cmd/ngxstorage snapshots --controllers 192.168.1.201 --api-key <key>
```

## Error handling

The SDK returns typed errors; classify without string matching:

```go
err := client.LUNs().Delete(ctx, id)
if ngxstorage.IsNotFound(err) { /* idempotent success */ }
if ngxstorage.IsBusy(err) { /* 725, already retried */ }
if ngxstorage.IsTransportError(err) { /* network-level */ }
```

Sentinel errors: `ngxstorage.ErrPoolNotFound`, `ngxstorage.ErrClusterNotReady`.

`APIError` carries `StatusCode`, `Code`, `Message`, `Method`, `Endpoint`, and
the raw `Body`; it is never logged by the SDK itself.

## Security

- The API key and any CHAP/S3 secret are never logged (only `has_*` booleans).
- TLS verification is optional: skipped by default because NGX Storage Arrays use
  self-signed certificates and most customers have no private CA/DNS. Pass
  `RootCAs` and set `InsecureSkipVerify=false` to enforce a customer trust chain.
- POST mutations are never replayed after an ambiguous transport failure.

## Canonical backend fields

Reads use the live backend JSON contract (nested `capacity.soft_quota`,
`exports.nfs.enabled`/`read_only`); writes use the canonical flat fields
(`soft_quota`, `nfs_export`, `nfs_read_only`). The SDK does not probe
alternate names.

## Test

```bash
go test ./...
```

## License

Apache-2.0
