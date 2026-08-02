package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	DefaultCalendar   string       `mapstructure:"default_calendar"`
	WorkingHoursFrom  string       `mapstructure:"working_hours_from"`
	WorkingHoursTo    string       `mapstructure:"working_hours_to"`
	WorkingDays       []string     `mapstructure:"working_days"`
	MinFreeSlot       int          `mapstructure:"min_free_slot_min"`
	ExcludedCalendars []string     `mapstructure:"excluded_calendars"`
	Google            GoogleConfig `mapstructure:"google"`
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

func DBPath() string {
	if DBPathOverride != "" {
		return DBPathOverride
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "calctl.db")
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
