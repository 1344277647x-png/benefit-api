/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	creationAssetAccessUse      = "creation_asset"
	creationAssetAccessIssuer   = "new-api"
	creationAssetAccessAudience = "new-api-creation-asset"
	// A ticket is deliberately shorter lived than a login session. It is only
	// used to let native media elements retain HTTP Range support.
	CreationAssetAccessTTL = 10 * time.Minute
)

var ErrCreationAssetAccessInvalid = errors.New("creation asset access ticket is invalid")

type CreationAssetAccess struct {
	UserID    int
	AssetID   string
	Download  bool
	ExpiresAt int64
}

type creationAssetAccessClaims struct {
	TokenUse string `json:"token_use"`
	AssetID  string `json:"asset_id"`
	Download bool   `json:"download,omitempty"`
	jwt.RegisteredClaims
}

func IssueCreationAssetAccessToken(userID int, assetID string, download bool) (string, int64, error) {
	assetID = strings.TrimSpace(assetID)
	if userID <= 0 || assetID == "" {
		return "", 0, ErrCreationAssetAccessInvalid
	}
	now := time.Now()
	expiresAt := now.Add(CreationAssetAccessTTL)
	claims := creationAssetAccessClaims{
		TokenUse: creationAssetAccessUse,
		AssetID:  assetID,
		Download: download,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    creationAssetAccessIssuer,
			Subject:   strconv.Itoa(userID),
			Audience:  jwt.ClaimStrings{creationAssetAccessAudience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(authSigningKey("creation-asset"))
	return signed, expiresAt.Unix(), err
}

func ParseCreationAssetAccessToken(raw string) (CreationAssetAccess, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return CreationAssetAccess{}, ErrCreationAssetAccessInvalid
	}
	claims := &creationAssetAccessClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: unexpected signing method", ErrCreationAssetAccessInvalid)
		}
		return authSigningKey("creation-asset"), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(creationAssetAccessIssuer), jwt.WithAudience(creationAssetAccessAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(5*time.Second))
	if err != nil || parsed == nil || !parsed.Valid {
		return CreationAssetAccess{}, ErrCreationAssetAccessInvalid
	}
	if claims.TokenUse != creationAssetAccessUse || claims.AssetID == "" || claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return CreationAssetAccess{}, ErrCreationAssetAccessInvalid
	}
	userID, err := strconv.Atoi(claims.Subject)
	if err != nil || userID <= 0 {
		return CreationAssetAccess{}, ErrCreationAssetAccessInvalid
	}
	return CreationAssetAccess{
		UserID:    userID,
		AssetID:   claims.AssetID,
		Download:  claims.Download,
		ExpiresAt: claims.ExpiresAt.Unix(),
	}, nil
}
