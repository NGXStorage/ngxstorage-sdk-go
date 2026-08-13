package ngxstorage

import (
	"encoding/json"
	"fmt"
)

// This file declares the canonical backend types returned by the NGX Storage
// Manager API v2. Field names mirror the live backend JSON keys (verified
// against the Cinder FC driver api.py and the NFS CSI driver, which use the
// live names rather than the stale APIV2.openapi.json).

// LUN is a block volume on the NGX Storage backend.
type LUN struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        string `json:"size"` // bytes, as a string from the backend
	ScsiID      string `json:"scsi_id"`
	BlockSize   string `json:"blocksize"`
	Provision   string `json:"provision_type"`
	Dedup       string `json:"dedup"`
	Compress    string `json:"compress"`
	DramCache   string `json:"dram_cache"`
	FlashCache  string `json:"flash_cache"`
	IoType      string `json:"io_type"`
	QosPriority string `json:"qos_priority"`
	Owner       string `json:"owner"`
	PoolName    string `json:"pool_name"`
	// Protocols maps protocol name to its target reference, e.g. {"fc": "tgtA"}.
	Protocols map[string]string `json:"protocols"`
}

// Share is a file (NFS) volume on the NGX Storage backend. Field names mirror the
// live backend JSON per the canonical NFS contract (NFS CSI driver
// CODE_RULES §11); the SDK does not probe alternate names.
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	TargetIQN string `json:"target_iqn"`
	Luns      []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Number string `json:"number"`
	} `json:"luns"`
}

// Initiator is an iSCSI initiator (IQN) or FC initiator (WWPN).
type Initiator struct {
	ID    string `json:"id"`
	IQN   string `json:"iqn,omitempty"`
	Alias string `json:"alias,omitempty"`
	NAA   string `json:"naa,omitempty"`
}

// AuthGroup is an iSCSI auth group (CHAP credential container).
type AuthGroup struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	ChapStatus bool        `json:"chap_status"`
	Initiators []Initiator `json:"initiator"`
}

// PortalGroup is an iSCSI portal group exposing target IPs.
type PortalGroup struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	IPs  []string `json:"ips"`
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

// Pool is a storage pool on the backend.
type Pool struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	IsFlash   bool   `json:"is_flash"`
	BlockSize string `json:"blocksize"`
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
