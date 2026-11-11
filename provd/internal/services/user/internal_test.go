package user

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/ubuntu-desktop-provision/provd/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		password string
		wantErr  bool
	}{
		"Valid password": {
			password: "securepassword123",
			wantErr:  false,
		},
		"Empty password": {
			password: "",
			wantErr:  true,
		},
	}

	for name, tc := range tests {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			salt := "$6$pepper$"
			got, err := hashPassword(tc.password, &salt)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err, "HashPassword should not return an error, but did")

			want := testutils.LoadWithUpdateFromGolden(t, got)
			require.Equal(t, want, got, "HashPassword returned an unexpected response")
		})
	}
}

type fakeHandoverSettings struct {
	bools   map[string]bool
	doubles map[string]float64
}

func (f fakeHandoverSettings) GetBoolean(key string) bool { return f.bools[key] }
func (f fakeHandoverSettings) GetDouble(key string) float64 {
	return f.doubles[key]
}

func TestHandoverINI(t *testing.T) {
	t.Parallel()

	settings := map[string]handoverSettings{
		"org.gnome.desktop.a11y.applications": fakeHandoverSettings{
			bools: map[string]bool{"screen-reader-enabled": true},
		},
		"org.gnome.desktop.interface": fakeHandoverSettings{
			bools:   map[string]bool{"enable-animations": false},
			doubles: map[string]float64{"text-scaling-factor": 1.25},
		},
	}
	keys := []a11yHandoverKey{
		{"org.gnome.desktop.a11y.applications", "screen-reader-enabled", "bool"},
		{"org.gnome.desktop.interface", "enable-animations", "bool"},
		{"org.gnome.desktop.interface", "text-scaling-factor", "double"},
	}

	got, err := handoverINI(keys, settings)
	require.NoError(t, err)
	require.Equal(t, `[org.gnome.desktop.a11y.applications]
screen-reader-enabled=true
[org.gnome.desktop.interface]
enable-animations=false
text-scaling-factor=1.25
`, got)

	// An unknown schema fails instead of rendering an empty section.
	_, err = handoverINI([]a11yHandoverKey{{"org.unknown", "key", "bool"}}, settings)
	require.Error(t, err)
}

func TestWriteHandoverFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	oldINI, oldUser := handoverINIPath, handoverUserPath
	handoverINIPath = filepath.Join(tmp, "dconf", "ucd-handover.ini")
	handoverUserPath = filepath.Join(tmp, "dconf", "ucd-handover-user")
	defer func() { handoverINIPath, handoverUserPath = oldINI, oldUser }()

	settings := map[string]handoverSettings{
		"org.gnome.desktop.a11y.applications": fakeHandoverSettings{
			bools: map[string]bool{"screen-reader-enabled": true},
		},
	}
	keys := []a11yHandoverKey{
		{"org.gnome.desktop.a11y.applications", "screen-reader-enabled", "bool"},
	}

	require.NoError(t, writeHandoverFiles("t26", keys, settings))

	ini, err := os.ReadFile(handoverINIPath)
	require.NoError(t, err)
	require.Equal(t, "[org.gnome.desktop.a11y.applications]\nscreen-reader-enabled=true\n", string(ini))

	user, err := os.ReadFile(handoverUserPath)
	require.NoError(t, err)
	require.Equal(t, "t26\n", string(user))
}
