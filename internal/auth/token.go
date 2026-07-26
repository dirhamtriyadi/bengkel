package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID      uuid.UUID  `json:"uid"`
	BranchID    *uuid.UUID `json:"bid,omitempty"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
	Type        string     `json:"type"`
	jwt.RegisteredClaims
}

type Manager struct {
	accessSecret, refreshSecret []byte
	accessTTL, refreshTTL       time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{[]byte(accessSecret), []byte(refreshSecret), accessTTL, refreshTTL}
}

func (m *Manager) Issue(userID uuid.UUID, branchID *uuid.UUID, roles, permissions []string) (string, string, time.Time, error) {
	now := time.Now()
	accessExpiry := now.Add(m.accessTTL)
	access, err := m.sign(Claims{UserID: userID, BranchID: branchID, Roles: roles, Permissions: permissions, Type: "access", RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), Subject: userID.String(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExpiry)}}, m.accessSecret)
	if err != nil {
		return "", "", time.Time{}, err
	}
	refresh, err := m.sign(Claims{UserID: userID, BranchID: branchID, Roles: roles, Permissions: permissions, Type: "refresh", RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), Subject: userID.String(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTTL))}}, m.refreshSecret)
	return access, refresh, accessExpiry, err
}

func (m *Manager) ParseAccess(raw string) (*Claims, error) {
	return m.parse(raw, m.accessSecret, "access")
}
func (m *Manager) ParseRefresh(raw string) (*Claims, error) {
	return m.parse(raw, m.refreshSecret, "refresh")
}
func (m *Manager) RefreshExpiry() time.Duration { return m.refreshTTL }
func Hash(raw string) string                    { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
func (m *Manager) sign(claims Claims, secret []byte) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}
func (m *Manager) parse(raw string, secret []byte, kind string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil || !token.Valid || claims.Type != kind {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
