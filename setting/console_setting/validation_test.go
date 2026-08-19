/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package console_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAnnouncementsPopupFields(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantError bool
	}{
		{
			name:  "legacy announcement remains valid",
			value: `[{"id":1,"content":"Legacy","publishDate":"2026-08-19T00:00:00Z","type":"default"}]`,
		},
		{
			name:  "popup announcement accepts supported frequency",
			value: `[{"id":2,"content":"New","publishDate":"2026-08-19T00:00:00Z","type":"success","popupEnabled":true,"popupFrequency":"daily"}]`,
		},
		{
			name:      "popup enabled must be boolean",
			value:     `[{"id":3,"content":"Bad","publishDate":"2026-08-19T00:00:00Z","popupEnabled":"true"}]`,
			wantError: true,
		},
		{
			name:      "popup frequency must be supported",
			value:     `[{"id":4,"content":"Bad","publishDate":"2026-08-19T00:00:00Z","popupEnabled":true,"popupFrequency":"hourly"}]`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateConsoleSettings(test.value, "Announcements")
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
