package ngxsdk

// This file declares the canonical backend types returned by the NGX Storage
// Manager API v2. Field names mirror the live backend JSON keys (verified
// against the Cinder FC driver api.py and the NFS CSI driver, which use the
// live names rather than the stale APIV2.openapi.json).

// LUN is a block volume on the NGX backend.
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

// Share is a file (NFS) volume on the NGX backend.
type Share struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Size          string `json:"size"`
	BlockSize     string `json:"blocksize"`
	Dedup         string `json:"dedup"`
	Compress      string `json:"compress"`
	Permissions   string `json:"permissions"`
	Owner         string `json:"owner"`
	PoolName      string `json:"pool_name"`
	ExportEnabled bool   `json:"export_enabled"`
	ReadOnly      bool   `json:"read_only"`
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

// Snapshot is a point-in-time copy of a LUN or share.
type Snapshot struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	VolumeID string `json:"volume_id"`
	Size     string `json:"reserved"`
	Created  string `json:"created"`
	Ready    bool   `json:"ready"`
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

// successFlag decodes the success acknowledgement in mutation responses. NGX
// encodes this as a boolean, a string, or a numeric "1"/"0"; all forms are
// accepted.
type successFlag bool

// mutationResponse is the canonical acknowledgement for LUN/share/snapshot
// mutations.
type mutationResponse struct {
	Success successFlag `json:"success"`
}
