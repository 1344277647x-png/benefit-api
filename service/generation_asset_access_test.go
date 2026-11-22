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
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCreationAssetAccessTokenBindsUserAssetAndDisposition(t *testing.T) {
	previousSecret := common.SessionSecret
	common.SessionSecret = "creation-asset-access-test-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })

	token, expiresAt, err := IssueCreationAssetAccessToken(42, "asset_test_123", true)
	require.NoError(t, err)
	require.Greater(t, expiresAt, common.GetTimestamp())

	access, err := ParseCreationAssetAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, CreationAssetAccess{UserID: 42, AssetID: "asset_test_123", Download: true, ExpiresAt: expiresAt}, access)
}

func TestCreationAssetAccessTokenRejectsTamperingAndInvalidInputs(t *testing.T) {
	previousSecret := common.SessionSecret
	common.SessionSecret = "creation-asset-access-test-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })

	_, _, err := IssueCreationAssetAccessToken(0, "asset_test_123", false)
	require.ErrorIs(t, err, ErrCreationAssetAccessInvalid)

	token, _, err := IssueCreationAssetAccessToken(42, "asset_test_123", false)
	require.NoError(t, err)
	tampered := token[:len(token)-2] + "x" + token[len(token)-1:]
	_, err = ParseCreationAssetAccessToken(tampered)
	require.ErrorIs(t, err, ErrCreationAssetAccessInvalid)

	_, err = ParseCreationAssetAccessToken("")
	require.ErrorIs(t, err, ErrCreationAssetAccessInvalid)
}
