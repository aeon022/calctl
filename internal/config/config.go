package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	DefaultCalendar     string       `mapstructure:"default_calendar"`
	WorkingHoursFrom    string       `mapstructure:"working_hours_from"`
	WorkingHoursTo      string       `mapstructure:"working_hours_to"`
	WorkingDays         []string     `mapstructure:"working_days"`
	MinFreeSlot         int          `mapstructure:"min_free_slot_min"`
	ExcludedCalendars   []string     `mapstructure:"excluded_calendars"`
	Google              GoogleConfig `mapstructure:"google"`
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

func DBPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "calctl.db")
}

func TokenPath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "calctl", "google_token.json")
}
