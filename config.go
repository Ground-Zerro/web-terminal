package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	configName = "config.json"
	configPerm = 0o600
)

type text string

func (t *text) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return t.Set(v)
}

func (t *text) Set(v string) error {
	if v != "" {
		*t = text(v)
	}
	return nil
}

func (t *text) String() string {
	return string(*t)
}

type count int

func (c *count) UnmarshalJSON(data []byte) error {
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return c.assign(v)
}

func (c *count) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	return c.assign(float64(n))
}

func (c *count) assign(v float64) error {
	if v > 0 {
		*c = count(v)
	}
	return nil
}

func (c *count) String() string {
	return strconv.Itoa(int(*c))
}

type minutes time.Duration

func (m *minutes) UnmarshalJSON(data []byte) error {
	return unmarshalDuration(data, time.Minute, (*time.Duration)(m))
}

func (m minutes) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(m).Minutes())
}

func (m *minutes) Set(v string) error {
	return parseDuration(v, time.Minute, (*time.Duration)(m))
}

func (m *minutes) String() string {
	return trimFloat(time.Duration(*m).Minutes())
}

type seconds time.Duration

func (s *seconds) UnmarshalJSON(data []byte) error {
	return unmarshalDuration(data, time.Second, (*time.Duration)(s))
}

func (s seconds) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(s).Seconds())
}

func (s *seconds) Set(v string) error {
	return parseDuration(v, time.Second, (*time.Duration)(s))
}

func (s *seconds) String() string {
	return trimFloat(time.Duration(*s).Seconds())
}

func unmarshalDuration(data []byte, unit time.Duration, dst *time.Duration) error {
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	assignDuration(v, unit, dst)
	return nil
}

func parseDuration(raw string, unit time.Duration, dst *time.Duration) error {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return err
	}
	assignDuration(v, unit, dst)
	return nil
}

func assignDuration(v float64, unit time.Duration, dst *time.Duration) {
	if v > 0 {
		*dst = time.Duration(v * float64(unit))
	}
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

type Config struct {
	ListenAddr text `json:"listen_addr" usage:"address and port to listen on"`

	Login    text `json:"login" usage:"login name"`
	Password text `json:"password" usage:"password"`

	TerminalDir text `json:"terminal_dir" usage:"directory the shell starts in"`
	Fail2banLog text `json:"fail2ban_log" usage:"path of the failed-login log for fail2ban"`

	MaxAttempts count   `json:"max_attempts" usage:"failed logins before an address is banned"`
	BanDuration minutes `json:"ban_duration" usage:"ban duration in minutes"`
	SessionTTL  minutes `json:"session_ttl" usage:"idle minutes before a session expires"`

	ReadHeaderTimeout seconds `json:"read_header_timeout" usage:"seconds allowed for reading request headers"`
	IdleTimeout       seconds `json:"idle_timeout" usage:"keep-alive idle timeout in seconds"`
}

func defaultConfig() *Config {
	return &Config{
		ListenAddr:        "0.0.0.0:8089",
		Login:             "root",
		Password:          "admin",
		TerminalDir:       "/root",
		Fail2banLog:       "/var/log/webterminal-bruteforce.log",
		MaxAttempts:       6,
		BanDuration:       minutes(15 * time.Minute),
		SessionTTL:        minutes(12 * time.Hour),
		ReadHeaderTimeout: seconds(15 * time.Second),
		IdleTimeout:       seconds(120 * time.Second),
	}
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("Cannot locate the executable: %v", err)
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

func configPath() string {
	exe := executablePath()
	if exe == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), configName)
}

func loadConfig(path string, cfg *Config) {
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error reading %s: %v, using defaults", path, err)
		}
		return
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		log.Printf("Invalid %s: %v, using defaults", path, err)
		*cfg = *defaultConfig()
		return
	}

	log.Printf("Loaded %s", path)
}

func writeConfig(path string) error {
	if path == "" {
		return fmt.Errorf("cannot locate the executable")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists, refusing to overwrite it", path)
	}

	data, err := json.MarshalIndent(defaultConfig(), "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), configPerm)
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fields := reflect.ValueOf(cfg).Elem()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Type().Field(i)
		value := fields.Field(i).Addr().Interface().(flag.Value)
		fs.Var(value, flagName(field), field.Tag.Get("usage"))
	}
}

func flagName(field reflect.StructField) string {
	return strings.ReplaceAll(field.Tag.Get("json"), "_", "-")
}

func writeUsage(w io.Writer, path string) {
	defaults := reflect.ValueOf(defaultConfig()).Elem()

	fmt.Fprintf(w, "webterminal — a web terminal with a file browser, in a single binary.\n\n")
	fmt.Fprintf(w, "Usage:\n  %s [options]\n\n", filepath.Base(os.Args[0]))
	fmt.Fprintf(w, "Settings come from built-in defaults, then %s next to the binary,\n", configName)
	fmt.Fprintf(w, "then the options below. Anything left unset keeps its default.\n\n")

	if path != "" {
		fmt.Fprintf(w, "Config file: %s\n\n", path)
	}

	fmt.Fprintf(w, "Actions:\n")
	fmt.Fprintf(w, "  --genconfig    write a default %s next to the binary and exit\n", configName)
	fmt.Fprintf(w, "  --service      create the %s systemd unit for this binary,\n", serviceName)
	fmt.Fprintf(w, "                 or remove it if the unit already exists\n")
	fmt.Fprintf(w, "  --help, -h     show this help and exit\n\n")

	fmt.Fprintf(w, "Options (each mirrors the config key shown in brackets):\n")
	for i := 0; i < defaults.NumField(); i++ {
		field := defaults.Type().Field(i)
		value := defaults.Field(i).Addr().Interface().(flag.Value)
		fmt.Fprintf(w, "  --%-26s [%s]\n", flagName(field)+" value", field.Tag.Get("json"))
		fmt.Fprintf(w, "  %-28s %s (default %s)\n", "", field.Tag.Get("usage"), value)
	}
}
