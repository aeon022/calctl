package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolate: temp HOME (os.UserConfigDir → $HOME/Library/Application Support on
// macOS) and a fresh package-global settings store.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "") // Linux CI: fall back to $HOME/.config
	t.Setenv("CALCTL_DATA_DIR", "")
	settings.Reset()
	Active = Config{}
	DBPathOverride = ""
	t.Cleanup(func() { settings.Reset(); Active = Config{}; DBPathOverride = "" })
	return home
}

func cfgDir(t *testing.T) string {
	t.Helper()
	d, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(d, "calctl")
}

func writeCfg(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(cfgDir(t), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir(t), "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	isolate(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Active.WorkingHoursFrom != "09:00" || Active.WorkingHoursTo != "18:00" || Active.MinFreeSlot != 30 || Active.DefaultCalendar != "" {
		t.Errorf("defaults = %+v", Active)
	}
}

func TestLoadFileOverridesDefaultsKeepsRest(t *testing.T) {
	isolate(t)
	writeCfg(t, "working_hours_from: \"08:30\"\nmin_free_slot_min: 45\ndefault_calendar: Arbeit\n")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Active.WorkingHoursFrom != "08:30" || Active.MinFreeSlot != 45 || Active.DefaultCalendar != "Arbeit" {
		t.Errorf("file values lost: %+v", Active)
	}
	if Active.WorkingHoursTo != "18:00" {
		t.Errorf("unset key must keep its default, got %q", Active.WorkingHoursTo)
	}
}

func TestLoadEnvBeatsFileAndParsesNumbers(t *testing.T) {
	isolate(t)
	writeCfg(t, "min_free_slot_min: 45\n")
	t.Setenv("CALCTL_MIN_FREE_SLOT_MIN", "15")
	t.Setenv("CALCTL_WORKING_HOURS_TO", "17:00")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Active.MinFreeSlot != 15 || Active.WorkingHoursTo != "17:00" {
		t.Errorf("env must win and numbers must stay numbers: %+v", Active)
	}
}

func TestLoadBrokenYAMLIsAnError(t *testing.T) {
	isolate(t)
	writeCfg(t, "default_calendar: [oops\n")
	if err := Load(); err == nil {
		t.Error("a corrupt config must be reported")
	}
}

func TestSetDefaultCalendarPersistsAndUpdatesActive(t *testing.T) {
	isolate(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if err := SetDefaultCalendar("Privat"); err != nil {
		t.Fatal(err)
	}
	if Active.DefaultCalendar != "Privat" {
		t.Errorf("Active = %q", Active.DefaultCalendar)
	}

	settings.Reset() // simulate a fresh process
	Active = Config{}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Active.DefaultCalendar != "Privat" {
		t.Errorf("not persisted: %q", Active.DefaultCalendar)
	}
	if Active.WorkingHoursFrom != "09:00" {
		t.Errorf("writing the file must not drop other settings, got %+v", Active)
	}
}

func TestLicenseGateAndPolarOrg(t *testing.T) {
	isolate(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if IsPro() {
		t.Error("no license → not pro")
	}
	if PolarOrgID() == "" {
		t.Error("PolarOrgID must fall back to the suite default")
	}

	if err := SetLicense("KEY-1", "granted", bundleBenefitID); err != nil {
		t.Fatal(err)
	}
	if !IsPro() || Active.LicenseKey != "KEY-1" {
		t.Errorf("bundle benefit must grant pro: %+v", Active)
	}
	if err := SetLicense("KEY-1", "granted", "some-other-product"); err != nil {
		t.Fatal(err)
	}
	if IsPro() {
		t.Error("a benefit from another product must not unlock calctl")
	}
	if err := SetLicense("KEY-1", "invalid", bundleBenefitID); err != nil {
		t.Fatal(err)
	}
	if IsPro() {
		t.Error("a non-granted status must not unlock calctl")
	}

	settings.Set("polar_org_id", "custom-org")
	if PolarOrgID() != "custom-org" {
		t.Errorf("polar_org_id override ignored: %q", PolarOrgID())
	}
}

func TestLicenseSurvivesReload(t *testing.T) {
	isolate(t)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if err := SetLicense("KEY-9", "granted", calctlBenefitID); err != nil {
		t.Fatal(err)
	}
	settings.Reset()
	Active = Config{}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if Active.LicenseKey != "KEY-9" || Active.LicenseStatus != "granted" || Active.LicenseBenefitID != calctlBenefitID || !IsPro() {
		t.Errorf("license not persisted: %+v", Active)
	}
}

func TestDBPathResolution(t *testing.T) {
	home := isolate(t)
	def := filepath.Join(cfgDir(t), "calctl.db")
	if got := DBPath(); got != def || Shared() {
		t.Errorf("default = %q shared=%v, want %q private", got, Shared(), def)
	}

	DBPathOverride = "/tmp/x.db"
	if DBPath() != "/tmp/x.db" || Shared() {
		t.Error("override wins and is never shared")
	}
	DBPathOverride = ""

	shared := filepath.Join(home, "Dropbox", "cal")
	writeCfg(t, "data_dir: "+shared+"\n")
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if DBPath() != filepath.Join(shared, "calctl.db") || !Shared() {
		t.Errorf("data_dir: %q shared=%v", DBPath(), Shared())
	}
}

func TestDataDirEnvAppliesOnlyAfterLoad(t *testing.T) {
	home := isolate(t)
	env := filepath.Join(home, "iCloud", "cal")
	t.Setenv("CALCTL_DATA_DIR", env)
	if DBPath() != filepath.Join(cfgDir(t), "calctl.db") {
		t.Error("before Load the shell environment must not redirect the DB")
	}
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if DBPath() != filepath.Join(env, "calctl.db") || !Shared() {
		t.Errorf("after Load: %q shared=%v", DBPath(), Shared())
	}
}

func TestLastSyncedPath(t *testing.T) {
	isolate(t)
	if got := LastSyncedPath(); got != filepath.Join(cfgDir(t), "last_synced") {
		t.Errorf("LastSyncedPath = %q", got)
	}
}
