package ngxstorage

import (
	"encoding/json"
	"fmt"
)

// This file declares the canonical backend types returned by the NGX Storage
// Manager API v2. Field names mirror the live backend JSON keys.

// LUN is a block volume on the NGX Storage backend.
type LUN struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Size            FlexBytes `json:"size"` // bytes; number or string
	Used            FlexBytes `json:"used"`
	ScsiID          string    `json:"scsi_id"`
	BlockSize       string    `json:"blocksize"`
	ThinProvision   FlexOnOff `json:"thin_provision"`
	Deduplication   FlexOnOff `json:"deduplication"`
	Compression     FlexOnOff `json:"compression"`
	CompressRatio   string    `json:"compress_ratio"`
	DramCache       FlexOnOff `json:"dram_cache"`
	FlashCache      FlexOnOff `json:"flash_cache"`
	IoType          string    `json:"io_type"`
	QosPriority     FlexBytes `json:"qos_priority"` // number or string
	Clone           string    `json:"clone"`
	SizeBySnapshots FlexBytes `json:"sizebysnapshots"`
	Owner           string    `json:"owner"`
	PoolName        string    `json:"pool_name"`
	Created         FlexBytes `json:"created"` // unix timestamp; number or string
	// Protocols maps protocol name to its target reference, e.g. {"fc": "tgtA"}.
	Protocols map[string]string `json:"protocols"`
}

// Share is a file (NFS) volume on the NGX Storage backend. Field names mirror
// the live backend JSON per the canonical NFS contract; the SDK does not probe
// alternate names.
type Share struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PoolName string `json:"pool_name"`
	// Capacity nests the canonical soft_quota byte value.
	Capacity ShareCapacity `json:"capacity"`
	// Exports nests the canonical NFS export state.
	Exports ShareExports `json:"exports"`
	// Storage attributes use the canonical NFS field names.
	BlockSize      string `json:"blocksize"`
	Dedup          string `json:"deduplication"`
	Compress       string `json:"compression"`
	FlashCache     string `json:"flash_cache"`
	DramCache      string `json:"dram_cache"`
	Permissions    string `json:"permissions"`
	FlashTierLimit string `json:"flashtier_limit"`
}

// UnmarshalJSON decodes a share with the exact strictness each document
// requires:
//
//   - identity and storage attributes are lenient: type-invalid values decode
//     to their zero values so one malformed record inside a list response does
//     not hide the remaining records;
//   - capacity.soft_quota is lenient: a malformed quota decodes to zero and
//     callers that require a quota (expand, idempotency, list validation)
//     reject it explicitly;
//   - exports.nfs.enabled and exports.nfs.read_only are strict: a present but
//     malformed export state is an error, because silently treating a corrupt
//     export as disabled would publish or unpublish the wrong share mode.
func (s *Share) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	s.ID, _ = m["id"].(string)
	s.Name, _ = m["name"].(string)
	s.PoolName, _ = m["pool_name"].(string)
	if capacity, ok := m["capacity"].(map[string]interface{}); ok {
		if raw, ok := capacity["soft_quota"]; ok {
			if bytes, err := flexBytesFromValue(raw); err == nil {
				s.Capacity.SoftQuota = bytes
			}
		}
	}
	if exports, ok := m["exports"].(map[string]interface{}); ok {
		if nfs, ok := exports["nfs"].(map[string]interface{}); ok {
			if raw, exists := nfs["enabled"]; exists {
				enabled, err := flexBoolFromValue(raw)
				if err != nil {
					return fmt.Errorf("ngxstorage: share %q export enabled: %w", s.ID, err)
				}
				s.Exports.NFS.Enabled = enabled
			}
			if raw, exists := nfs["read_only"]; exists {
				readOnly, err := flexBoolFromValue(raw)
				if err != nil {
					return fmt.Errorf("ngxstorage: share %q export read_only: %w", s.ID, err)
				}
				s.Exports.NFS.ReadOnly = readOnly
			}
		}
	}
	s.BlockSize, _ = m["blocksize"].(string)
	s.Dedup, _ = m["deduplication"].(string)
	s.Compress, _ = m["compression"].(string)
	s.FlashCache, _ = m["flash_cache"].(string)
	s.DramCache, _ = m["dram_cache"].(string)
	s.Permissions, _ = m["permissions"].(string)
	s.FlashTierLimit, _ = m["flashtier_limit"].(string)
	return nil
}

// ShareCapacity is the nested capacity document of a share.
type ShareCapacity struct {
	// SoftQuota is the share quota in bytes; the backend serializes it as a
	// number or a decimal string.
	SoftQuota FlexBytes `json:"soft_quota"`
}

// ShareExports is the nested export document of a share.
type ShareExports struct {
	NFS ShareNFSExport `json:"nfs"`
}

// ShareNFSExport is the NFS export state of a share.
type ShareNFSExport struct {
	// Enabled follows the live backend representation: "1" enabled, ""
	// disabled; FlexBool also accepts JSON booleans from other releases.
	Enabled FlexBool `json:"enabled"`
	// ReadOnly reports whether the export is read-only.
	ReadOnly FlexBool `json:"read_only"`
}

// FCTarget is a Fibre Channel target on the backend.
type FCTarget struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Owner string        `json:"owner"`
	Ports []FCPort      `json:"ports"`
	Luns  []FCTargetLUN `json:"luns"`
}

// FCPort is a physical FC adapter port.
type FCPort struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Wwpns      []FCWWPN      `json:"wwpns"`
	Initiators []FCInitiator `json:"initiators"`
}

// FCWWPN is a World Wide Port Name.
type FCWWPN struct {
	WWPN     string `json:"wwpn"`      // e.g. "naa.20020090faeae1a5"
	Owner    string `json:"owner"`     // controller product ID
	TargetID string `json:"target_id"` // FC target ID this WWPN is assigned to
}

// FCInitiator is a logged-in initiator on an FC port.
type FCInitiator struct {
	NAA string `json:"naa"`
	Tag string `json:"tag"`
}

// FCTargetLUN is a LUN mapped to an FC target.
type FCTargetLUN struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Number   string `json:"number"` // SCSI LUN number, string on the wire
	PoolName string `json:"pool_name"`
	ScsiID   string `json:"scsi_id"`
}

// FCPortInfo is a hardware FC adapter port from /api/v2/target/fc/ports.
type FCPortInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Number        int      `json:"number"`
	Available     bool     `json:"available"`
	AdapterSerial string   `json:"adapter_serial"`
	Wwpns         []FCWWPN `json:"wwpns"`
}

// ISCSITarget is an iSCSI target on the backend.
type ISCSITarget struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Owner           string `json:"owner"`
	TargetIQN       string `json:"target_iqn"`
	AuthGroupID     string `json:"auth_group_id"`
	AuthGroupName   string `json:"auth_group_name"`
	PortalGroupID   string `json:"portal_group_id"`
	PortalGroupName string `json:"portal_group_name"`
	Initiators      []struct {
		ID  string `json:"id"`
		IQN string `json:"iqn"`
	} `json:"initiators"`
	Luns []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Number string `json:"number"`
	} `json:"luns"`
}

// Initiator is an iSCSI initiator (IQN) or FC initiator (WWPN) currently
// connected to the system. iSCSI entries carry the iqn, source ip, auth-group
// alias, and the target iqn they are logged into; FC entries carry the
// initiator WWPN and the target port.
type Initiator struct {
	ID        string `json:"id,omitempty"`
	IQN       string `json:"iqn,omitempty"`
	Alias     string `json:"alias,omitempty"`
	NAA       string `json:"naa,omitempty"`
	Initiator string `json:"initiator,omitempty"`
	Port      string `json:"port,omitempty"`
	PortNAA   string `json:"port_naa,omitempty"`
	Tag       string `json:"tag,omitempty"`
	IP        string `json:"ip,omitempty"`
	TargetIQN string `json:"target_iqn,omitempty"`
	Type      string `json:"type,omitempty"`
}

// AuthGroup is an iSCSI auth group (CHAP credential container).
type AuthGroup struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	ChapStatus bool        `json:"chap_status"`
	Initiators []Initiator `json:"initiator"`
}

// PortalGroup is an iSCSI portal group exposing target IPs.
type PortalGroupListenIP struct {
	ID            string `json:"id"`
	InterfaceName string `json:"interface_name"`
	IPAddress     string `json:"ip_address"`
	Owner         string `json:"owner"`
}

type PortalGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Owner is present on lightweight list entries.
	Owner string `json:"owner"`
	// ListenIPs is present only on the detail endpoint; the lightweight list
	// omits it.
	ListenIPs []PortalGroupListenIP `json:"listen_ips"`
}

// IPAddresses flattens the listen_ips records to their ip_address values.
func (p PortalGroup) IPAddresses() []string {
	ips := make([]string, 0, len(p.ListenIPs))
	for _, lip := range p.ListenIPs {
		if lip.IPAddress != "" {
			ips = append(ips, lip.IPAddress)
		}
	}
	return ips
}

// Snapshot is a point-in-time copy of a LUN or share. Field names mirror the
// live backend JSON; the SDK does not probe alternate names.
type Snapshot struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	VolumeID   string `json:"volume_id"`
	VolumeName string `json:"volume_name"`
	PoolName   string `json:"pool_name"`
	Path       string `json:"path"`
	VolumeType string `json:"volume_type"`
	Size       string `json:"reserved"` // bytes, string on the wire
	Created    string `json:"created"`
	Ready      bool   `json:"ready"`
}

// UnmarshalJSON decodes a snapshot record. NGX snapshots are synchronous
// point-in-time copies: the backend (verified live on V6BGWYPA and SGZ1RVC2,
// create and get) returns no `ready` field at all, and a fetched record is
// usable immediately. Ready therefore defaults to true and is only cleared
// by an explicit wire `ready` value (boolean or 0/1 number). Without this,
// every snapshot would report ReadyToUse=false forever and no
// VolumeSnapshot could ever reach readyToUse.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	// The wire struct carries every field except Ready so a non-boolean
	// wire encoding can never fail the whole decode; Ready is resolved
	// explicitly below.
	var raw struct {
		ID         string          `json:"id"`
		Name       string          `json:"name"`
		VolumeID   string          `json:"volume_id"`
		VolumeName string          `json:"volume_name"`
		PoolName   string          `json:"pool_name"`
		Path       string          `json:"path"`
		VolumeType string          `json:"volume_type"`
		Size       string          `json:"reserved"`
		Created    string          `json:"created"`
		Ready      json.RawMessage `json:"ready"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.ID, s.Name = raw.ID, raw.Name
	s.VolumeID, s.VolumeName = raw.VolumeID, raw.VolumeName
	s.PoolName, s.Path = raw.PoolName, raw.Path
	s.VolumeType, s.Size = raw.VolumeType, raw.Size
	s.Created = raw.Created
	s.Ready = true
	if len(raw.Ready) != 0 && string(raw.Ready) != "null" {
		var b bool
		if err := json.Unmarshal(raw.Ready, &b); err == nil {
			s.Ready = b
		} else {
			var n int
			if err := json.Unmarshal(raw.Ready, &n); err != nil {
				return fmt.Errorf("decode snapshot ready: %w", err)
			}
			s.Ready = n != 0
		}
	}
	return nil
}

// Pool is a storage pool on the backend.
type Pool struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	IsFlash   bool   `json:"is_flash"`
	BlockSize string `json:"blocksize"`
}

// UnmarshalJSON accepts the canonical pool fields. The backend encodes
// is_flash as either a JSON boolean or a 0/1 number depending on endpoint.
func (p *Pool) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Owner     string          `json:"owner"`
		IsFlash   json.RawMessage `json:"is_flash"`
		BlockSize string          `json:"blocksize"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ID, p.Name = raw.ID, raw.Name
	p.Owner, p.BlockSize = raw.Owner, raw.BlockSize
	if len(raw.IsFlash) != 0 && string(raw.IsFlash) != "null" {
		var b bool
		if err := json.Unmarshal(raw.IsFlash, &b); err == nil {
			p.IsFlash = b
		} else {
			var n int
			if err := json.Unmarshal(raw.IsFlash, &n); err != nil {
				return fmt.Errorf("decode pool is_flash: %w", err)
			}
			p.IsFlash = n != 0
		}
	}
	return nil
}

// SnapshotSchedule is the snapshot schedule installed on a volume
// (GET/POST/DELETE /api/v2/snapshot/schedule/{volume_id}).
type SnapshotSchedule struct {
	ID         string `json:"id"`
	VolumeID   string `json:"volume_id"`
	VolumeName string `json:"volume_name"`
	Interval   string `json:"interval"`
	Retention  string `json:"retention"`
	Enabled    bool   `json:"enabled"`
	NextRun    string `json:"next_run"`
}

// FCInitiatorTag is an FC initiator tag (zoning alias) that names one or more
// initiator WWPNs (GET/POST /api/v2/target/fc/initiator/tag).
type FCInitiatorTag struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	InitiatorIDs []string `json:"initiator_ids"`
	InitiatorWWN []string `json:"initiator_wwn"`
}

// Peer is a cluster member in the cluster status response.
type Peer struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ClusterStatus is the response of GET /api/v2/status/cluster.
type ClusterStatus struct {
	Connected int    `json:"connected"`
	Status    string `json:"status"` // Master | Ready | Cluster
	Peers     []Peer `json:"peers"`
}

// mutationResponse is the canonical acknowledgement for LUN/share/snapshot
// mutations.
type mutationResponse struct {
	Success successFlag `json:"success"`
}
