// Wizard-to-user accessibility handover.
//
// The wizard's accessibility preferences land in the WIZARD session's
// dconf database, which lives in the ephemeral gdm home that is wiped
// when provisioning completes - the created user starts with an empty
// database and none of the chosen settings (T25 gate-4 finding). All
// the other wizard settings (locale, keyboard layout, timezone) hand
// over because they are applied to SYSTEM-WIDE services (locale1,
// timedate1) that every user inherits; per-user dconf state has no
// such channel, so this file implements one.
//
// The mechanism: at user creation provd records the accessibility
// preferences and the target username in the handover channel
// /etc/dconf/channel/handover, and the static session handover unit
// baked in the desktop base (usr/lib/systemd/user/ucd-a11y-handover
// .service) applies them through the real session gsettings/dconf path
// at the target user's first login - so gnome-settings-daemon sees the
// change notification and actuates the reader, exactly like a GUI
// toggle. A per-user marker in $HOME/.cache keeps it one-shot, and the
// target username check keeps later accounts unaffected.
//
// The channel directory is created and re-enforced at boot by the
// desktop base's usr/lib/tmpfiles.d/ucd-handover.conf as
// root:gnome-initial-setup 1731: group-writable only for the
// provisioning group (the wizard session's dynamic uid's only group),
// and for every other user traverse-only (others=x) so the applying
// side - the target user's session manager, which is not in the
// provisioning group - can read the two recorded files by exact path.
// The applying side re-verifies the directory and file modes and
// allowlists the schema/key pairs before applying anything; the
// recorded files stay world-readable (0644).
package user

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/linuxdeepin/go-gir/gio-2.0"
)

const (
	// handoverMarkerPath is the per-user one-shot marker (relative to
	// the target user's HOME, written by the session handover unit).
	handoverMarkerPath = ".cache/ucd-a11y-handover.done"
)

// Paths are vars so tests can redirect them to a temporary tree.
var (
	// handoverINIPath holds the wizard's accessibility preferences as
	// a glib keyfile ([schema] sections with key=value lines), inside
	// the group-restricted handover channel (see the package comment
	// and the desktop base's usr/lib/tmpfiles.d/ucd-handover.conf).
	handoverINIPath = "/etc/dconf/channel/handover/ucd-handover.ini"
	// handoverUserPath holds the username the preferences apply to.
	handoverUserPath = "/etc/dconf/channel/handover/ucd-handover-user"
)

// a11yHandoverKey is one GSettings key carried across the handover.
type a11yHandoverKey struct {
	schema string
	key    string
	kind   string // "bool" or "double"
}

// a11yHandoverKeys is the complete set of keys the wizard's
// accessibility page can change (internal/services/accessibility).
var a11yHandoverKeys = []a11yHandoverKey{
	{"org.gnome.desktop.a11y.applications", "screen-reader-enabled", "bool"},
	{"org.gnome.desktop.a11y.applications", "screen-keyboard-enabled", "bool"},
	{"org.gnome.desktop.a11y.applications", "screen-magnifier-enabled", "bool"},
	{"org.gnome.desktop.a11y.interface", "high-contrast", "bool"},
	{"org.gnome.desktop.interface", "enable-animations", "bool"},
	{"org.gnome.desktop.interface", "text-scaling-factor", "double"},
	{"org.gnome.desktop.wm.preferences", "visual-bell", "bool"},
	{"org.gnome.desktop.a11y.keyboard", "stickykeys-enable", "bool"},
	{"org.gnome.desktop.a11y.keyboard", "slowkeys-enable", "bool"},
	{"org.gnome.desktop.a11y.keyboard", "mousekeys-enable", "bool"},
}

// handoverSettings is the GSettings subset the handover needs.
type handoverSettings interface {
	GetBoolean(key string) bool
	GetDouble(key string) float64
}

// writeA11yHandover records the current accessibility preferences for
// the created user. It is best-effort: a failure is logged but must
// not fail the user creation that already succeeded.
func writeA11yHandover(username string) {
	settings := make(map[string]handoverSettings)
	for _, k := range a11yHandoverKeys {
		if _, ok := settings[k.schema]; !ok {
			settings[k.schema] = gio.NewSettings(k.schema)
		}
	}
	if err := writeHandoverFiles(username, a11yHandoverKeys, settings); err != nil {
		slog.Warn(fmt.Sprintf("accessibility handover for %q not recorded: %v", username, err))
	}
}

// writeHandoverFiles writes the preferences keyfile and the target
// username into the handover channel. The channel directory itself is
// the desktop base's responsibility (tmpfiles, root:gnome-initial-setup
// 1731); the MkdirAll only covers the redirected test tree - at runtime
// a missing channel is a write failure, logged best-effort, never
// silently created with a weaker mode.
func writeHandoverFiles(username string, keys []a11yHandoverKey, settings map[string]handoverSettings) error {
	ini, err := handoverINI(keys, settings)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(handoverINIPath), 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(handoverINIPath), err)
	}
	if err := os.WriteFile(handoverINIPath, []byte(ini), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", handoverINIPath, err)
	}
	if err := os.WriteFile(handoverUserPath, []byte(username+"\n"), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", handoverUserPath, err)
	}
	return nil
}

// handoverINI renders the preferences as a glib keyfile grouped by
// schema, in the declaration order of the keys.
func handoverINI(keys []a11yHandoverKey, settings map[string]handoverSettings) (string, error) {
	var b strings.Builder
	var schema string
	for _, k := range keys {
		if k.schema != schema {
			schema = k.schema
			fmt.Fprintf(&b, "[%s]\n", schema)
		}
		get, ok := settings[k.schema]
		if !ok {
			return "", fmt.Errorf("no settings object for %s", k.schema)
		}
		switch k.kind {
		case "bool":
			fmt.Fprintf(&b, "%s=%t\n", k.key, get.GetBoolean(k.key))
		case "double":
			fmt.Fprintf(&b, "%s=%g\n", k.key, get.GetDouble(k.key))
		default:
			return "", fmt.Errorf("unsupported handover key kind %q", k.kind)
		}
	}
	return b.String(), nil
}
