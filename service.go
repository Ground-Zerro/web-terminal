package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

const (
	serviceName     = "webterminal.service"
	serviceUnit     = "/etc/systemd/system/" + serviceName
	serviceUnitPerm = 0o644
	systemdRuntime  = "/run/systemd/system"
)

const unitTemplate = `[Unit]
Description=Web Terminal
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart="%s"
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=webterminal

[Install]
WantedBy=multi-user.target
`

func toggleService() error {
	if err := requireSystemd(); err != nil {
		return err
	}

	if previous, exists := installedExecStart(); exists {
		return removeService(previous)
	}

	exe := executablePath()
	if exe == "" {
		return fmt.Errorf("cannot locate the executable")
	}
	return installService(exe)
}

func requireSystemd() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("managing %s requires root", serviceName)
	}
	if _, err := os.Stat(systemdRuntime); err != nil {
		return fmt.Errorf("systemd is not running on this system")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl was not found in PATH")
	}
	return nil
}

func installService(exe string) error {
	if err := os.WriteFile(serviceUnit, fmt.Appendf(nil, unitTemplate, exe), serviceUnitPerm); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", "--now", serviceName); err != nil {
		return err
	}

	log.Printf("Created %s", serviceUnit)
	log.Printf("ExecStart=%s", exe)
	log.Printf("Service enabled and started. Follow it with: journalctl -u webterminal -f")
	return nil
}

func removeService(previous string) error {
	if err := systemctl("disable", "--now", serviceName); err != nil {
		log.Printf("Could not stop the running service: %v", err)
	}
	if err := os.Remove(serviceUnit); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}

	log.Printf("Removed %s", serviceUnit)
	if previous != "" {
		log.Printf("It pointed at %s", previous)
	}
	log.Printf("Run with --service again to recreate it for the current binary")
	return nil
}

func installedExecStart() (string, bool) {
	data, err := os.ReadFile(serviceUnit)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, found := strings.CutPrefix(strings.TrimSpace(line), "ExecStart="); found {
			return strings.Trim(value, `"`), true
		}
	}
	return "", true
}

func systemctl(args ...string) error {
	output, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, bytes.TrimSpace(output))
	}
	return nil
}
