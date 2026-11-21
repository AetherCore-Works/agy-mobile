package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

// HostProfile stores a remote SSH / Wake-on-LAN machine profile for multi-host switching.
type HostProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	SSHUser   string `json:"ssh_user"`
	SSHPort   string `json:"ssh_port"`
	WolMAC    string `json:"wol_mac"`
	OSHint    string `json:"os_hint"`
	IsActive  bool   `json:"is_active"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// HostProfileInput holds fields to create or update a host profile.
type HostProfileInput struct {
	Name      string  `json:"name"`
	Host      string  `json:"host"`
	SSHUser   string  `json:"ssh_user"`
	SSHPort   *string `json:"ssh_port"`
	WolMAC    *string `json:"wol_mac"`
	OSHint    *string `json:"os_hint"`
	SortOrder *int    `json:"sort_order"`
}

func randomHexID(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeOSHint(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "windows", "win":
		return "windows"
	case "darwin", "macos", "mac":
		return "darwin"
	case "auto":
		return "auto"
	default:
		return "linux"
	}
}

func normalizeSSHPort(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "22"
	}
	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return "22"
		}
	}
	return trimmed
}

// EnsureDefaultHostProfile seeds an initial profile from environment defaults when host_profiles is empty.
func (g *GatewayDB) EnsureDefaultHostProfile(name, host, sshUser, sshPort, wolMAC, osHint string) error {
	cleanHost := strings.TrimSpace(host)
	cleanUser := strings.TrimSpace(sshUser)
	if cleanHost == "" || cleanUser == "" {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	var count int
	if err := g.sql.QueryRow("SELECT COUNT(*) FROM host_profiles").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = cleanHost
	}
	now := utcNow()
	_, err := g.sql.Exec(
		`INSERT INTO host_profiles (id, name, host, ssh_user, ssh_port, wol_mac, os_hint, is_active, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1, 10, ?, ?)`,
		"host-default",
		cleanName,
		cleanHost,
		cleanUser,
		normalizeSSHPort(sshPort),
		strings.TrimSpace(wolMAC),
		normalizeOSHint(osHint),
		now,
		now,
	)
	return err
}

func scanHostProfile(scanner interface {
	Scan(dest ...any) error
}) (HostProfile, error) {
	var p HostProfile
	var activeInt int
	err := scanner.Scan(
		&p.ID, &p.Name, &p.Host, &p.SSHUser, &p.SSHPort, &p.WolMAC,
		&p.OSHint, &activeInt, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt,
	)
	p.IsActive = activeInt != 0
	return p, err
}

// ListHostProfiles returns all registered host profiles ordered by active/sort_order/name.
func (g *GatewayDB) ListHostProfiles() ([]HostProfile, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	rows, err := g.sql.Query(
		`SELECT id, name, host, ssh_user, ssh_port, wol_mac, os_hint, is_active, sort_order, created_at, updated_at
		 FROM host_profiles
		 ORDER BY sort_order ASC, created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []HostProfile{}
	for rows.Next() {
		p, err := scanHostProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetHostProfile fetches a single host profile by ID.
func (g *GatewayDB) GetHostProfile(id string) (*HostProfile, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, err := scanHostProfile(g.sql.QueryRow(
		`SELECT id, name, host, ssh_user, ssh_port, wol_mac, os_hint, is_active, sort_order, created_at, updated_at
		 FROM host_profiles WHERE id = ?`,
		id,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetActiveHostProfile returns the currently active host profile, falling back to the first profile if needed.
func (g *GatewayDB) GetActiveHostProfile() (*HostProfile, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, err := scanHostProfile(g.sql.QueryRow(
		`SELECT id, name, host, ssh_user, ssh_port, wol_mac, os_hint, is_active, sort_order, created_at, updated_at
		 FROM host_profiles
		 ORDER BY is_active DESC, sort_order ASC, created_at ASC
		 LIMIT 1`,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateHostProfile inserts a new host profile. If it is the first profile, marks it active.
func (g *GatewayDB) CreateHostProfile(in HostProfileInput) (*HostProfile, error) {
	name := strings.TrimSpace(in.Name)
	host := strings.TrimSpace(in.Host)
	user := strings.TrimSpace(in.SSHUser)
	if host == "" || user == "" {
		return nil, fmt.Errorf("host e ssh_user são obrigatórios")
	}
	if name == "" {
		name = host
	}
	port := "22"
	if in.SSHPort != nil {
		port = normalizeSSHPort(*in.SSHPort)
	}
	mac := ""
	if in.WolMAC != nil {
		mac = strings.TrimSpace(*in.WolMAC)
	}
	osHint := "linux"
	if in.OSHint != nil {
		osHint = normalizeOSHint(*in.OSHint)
	}
	sortOrder := 100
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	id := "host-" + randomHexID(6)
	now := utcNow()

	g.mu.Lock()
	var count int
	_ = g.sql.QueryRow("SELECT COUNT(*) FROM host_profiles").Scan(&count)
	isActive := 0
	if count == 0 {
		isActive = 1
	}
	_, err := g.sql.Exec(
		`INSERT INTO host_profiles (id, name, host, ssh_user, ssh_port, wol_mac, os_hint, is_active, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, host, user, port, mac, osHint, isActive, sortOrder, now, now,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return g.GetHostProfile(id)
}

// UpdateHostProfile updates an existing host profile in place.
func (g *GatewayDB) UpdateHostProfile(id string, in HostProfileInput) (*HostProfile, error) {
	existing, err := g.GetHostProfile(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = existing.Name
	}
	host := strings.TrimSpace(in.Host)
	if host == "" {
		host = existing.Host
	}
	user := strings.TrimSpace(in.SSHUser)
	if user == "" {
		user = existing.SSHUser
	}
	port := existing.SSHPort
	if in.SSHPort != nil {
		port = normalizeSSHPort(*in.SSHPort)
	}
	mac := existing.WolMAC
	if in.WolMAC != nil {
		mac = strings.TrimSpace(*in.WolMAC)
	}
	osHint := existing.OSHint
	if in.OSHint != nil {
		osHint = normalizeOSHint(*in.OSHint)
	}
	sortOrder := existing.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	g.mu.Lock()
	_, err = g.sql.Exec(
		`UPDATE host_profiles
		 SET name = ?, host = ?, ssh_user = ?, ssh_port = ?, wol_mac = ?, os_hint = ?, sort_order = ?, updated_at = ?
		 WHERE id = ?`,
		name, host, user, port, mac, osHint, sortOrder, utcNow(), id,
	)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return g.GetHostProfile(id)
}

// ActivateHostProfile marks the specified host profile as the single active host.
func (g *GatewayDB) ActivateHostProfile(id string) (*HostProfile, error) {
	g.mu.Lock()
	tx, err := g.sql.Begin()
	if err != nil {
		g.mu.Unlock()
		return nil, err
	}
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM host_profiles WHERE id = ?", id).Scan(&exists); err != nil || exists == 0 {
		_ = tx.Rollback()
		g.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	now := utcNow()
	if _, err := tx.Exec("UPDATE host_profiles SET is_active = 0, updated_at = ?", now); err != nil {
		_ = tx.Rollback()
		g.mu.Unlock()
		return nil, err
	}
	if _, err := tx.Exec("UPDATE host_profiles SET is_active = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
		_ = tx.Rollback()
		g.mu.Unlock()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		g.mu.Unlock()
		return nil, err
	}
	g.mu.Unlock()
	return g.GetHostProfile(id)
}

// DeleteHostProfile deletes a host profile while refusing to delete the sole remaining profile.
func (g *GatewayDB) DeleteHostProfile(id string) (bool, error) {
	g.mu.Lock()
	var total int
	if err := g.sql.QueryRow("SELECT COUNT(*) FROM host_profiles").Scan(&total); err != nil {
		g.mu.Unlock()
		return false, err
	}
	if total <= 1 {
		g.mu.Unlock()
		return false, fmt.Errorf("não é possível remover o único host cadastrado")
	}

	var wasActive int
	err := g.sql.QueryRow("SELECT is_active FROM host_profiles WHERE id = ?", id).Scan(&wasActive)
	if err == sql.ErrNoRows {
		g.mu.Unlock()
		return false, nil
	}
	if err != nil {
		g.mu.Unlock()
		return false, err
	}

	if _, err := g.sql.Exec("DELETE FROM host_profiles WHERE id = ?", id); err != nil {
		g.mu.Unlock()
		return false, err
	}
	if wasActive != 0 {
		_, _ = g.sql.Exec(
			`UPDATE host_profiles SET is_active = 1, updated_at = ?
			 WHERE id = (SELECT id FROM host_profiles ORDER BY sort_order ASC, created_at ASC LIMIT 1)`,
			utcNow(),
		)
	}
	g.mu.Unlock()
	return true, nil
}
