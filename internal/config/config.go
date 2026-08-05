package config

import (
	"fmt"
	"os"
	"path/filepath"

	coreconfig "github.com/aeon022/missionctl-core/config"
	"github.com/aeon022/missionctl-core/licensing"
	"github.com/spf13/viper"
)

type Config struct {
	DefaultCalendar   string       `mapstructure:"default_calendar"`
	WorkingHoursFrom  string       `mapstructure:"working_hours_from"`
	WorkingHoursTo    string       `mapstructure:"working_hours_to"`
	WorkingDays       []string     `mapstructure:"working_days"`
	MinFreeSlot       int          `mapstructure:"min_free_slot_min"`
	ExcludedCalendars []string     `mapstructure:"excluded_calendars"`
	DataDir           string       `mapstructure:"data_dir"`
	Google            GoogleConfig `mapstructure:"google"`
	LicenseKey        string       `mapstructure:"license_key"`
	LicenseStatus     string       `mapstructure:"license_status"`
	LicenseBenefitID  string       `mapstructure:"license_benefit_id"`
}

// bundleBenefitID and calctlBenefitID identify the missionctl Bundle's and
// calctl's own individual-product license-key benefits in Polar. Both
// start empty (the calctl-only product doesn't exist in Polar yet) — see
// licensing.Result.Grants: empty IDs fall back to "any active key under
// our org grants access", so this is a no-op until both are filled in
// once the individual product is created and its benefit ID is known.
const (
	bundleBenefitID = ""
	calctlBenefitID = ""
)

// IsPro reports whether a valid Pro/Bundle or calctl-only license is
// active on this machine — gates the `calctl summarize` command.
func IsPro() bool {
	result := licensing.Result{Status: Active.LicenseStatus, BenefitID: Active.LicenseBenefitID}
	return result.Grants(calctlBenefitID, bundleBenefitID)
}

func PolarOrgID() string {
	if v := viper.GetString("polar_org_id"); v != "" {
		return v
	}
	return licensing.DefaultOrgID
}

// SetLicense persists the license key/status/benefit to
// ~/Library/Application Support/calctl/config.yaml and updates Active
// immediately.
func SetLicense(key, status, benefitID string) error {
	viper.Set("license_key", key)
	viper.Set("license_status", status)
	viper.Set("license_benefit_id", benefitID)
	Active.LicenseKey = key
	Active.LicenseStatus = status
	Active.LicenseBenefitID = benefitID
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	cfgDir := filepath.Join(dir, "calctl")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return err
	}
	return viper.WriteConfigAs(filepath.Join(cfgDir, "config.yaml"))
}

type GoogleConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	TokenFile    string `mapstructure:"token_file"`
}

var Active Config

func Load() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}
	dir := filepath.Join(configDir, "calctl")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(dir)
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("CALCTL")
	viper.AutomaticEnv()

	viper.SetDefault("default_calendar", "")
	viper.SetDefault("working_hours_from", "09:00")
	viper.SetDefault("working_hours_to", "18:00")
	viper.SetDefault("working_days", []string{"Mon", "Tue", "Wed", "Thu", "Fri"})
	viper.SetDefault("min_free_slot_min", 30)
	viper.SetDefault("excluded_calendars", []string{
		"Geburtstage", "Geburtstag", "Birthdays", "Birthday",
		"Feiertage in Österreich", "Feiertage", "Holidays", "Holiday",
		"Siri Suggestions",
	})

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("read config: %w", err)
		}
		// no config file is fine — defaults apply
	}

	return viper.Unmarshal(&Active)
}

// DBPathOverride, when non-empty, overrides DBPath()'s return value. Used by tests
// to point at a temporary database instead of the real one on disk.
var DBPathOverride string

// DBPath returns the database file path. DBPathOverride (test-only) wins
// if set; otherwise data_dir (viper key, also settable via CALCTL_DATA_DIR
// since AutomaticEnv is on) points it at a user-chosen directory — e.g.
// inside iCloud Drive or Dropbox — resolved via coreconfig.ResolveDir; with
// neither set, the private default (~/Library/Application Support/calctl)
// is unchanged from before this existed.
func DBPath() string {
	if DBPathOverride != "" {
		return DBPathOverride
	}
	if dir := viper.GetString("data_dir"); dir != "" {
		resolved, _ := coreconfig.ResolveDir("calctl", dir)
		return filepath.Join(resolved, "calctl.db")
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "calctl.db")
}

// Shared reports whether DBPath currently resolves to a user-configured
// directory (data_dir) rather than the tool's private default.
func Shared() bool {
	return DBPathOverride == "" && viper.GetString("data_dir") != ""
}

func TokenPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "google_token.json")
}

// LastSyncedPath is the marker file (see missionctl-core/lastsync) tracking
// when a sync last completed, for the TUI's "synced Xh ago" header display.
func LastSyncedPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "last_synced")
}

// SetDefaultCalendar persists default_calendar to the config file and
// updates Active in memory, so it takes effect immediately without a
// restart. Used by the TUI's "c" calendar picker.
func SetDefaultCalendar(name string) error {
	viper.Set("default_calendar", name)
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}
	path := filepath.Join(configDir, "calctl", "config.yaml")
	if err := viper.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	Active.DefaultCalendar = name
	return nil
}
