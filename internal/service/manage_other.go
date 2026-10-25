//go:build !windows

package service

import "fmt"

// InstallService no aplica fuera de Windows.
func InstallService(name, displayName, description string) error {
	return fmt.Errorf("servicios Windows no están disponibles en esta plataforma (use el modo consola)")
}

// UninstallService no aplica fuera de Windows.
func UninstallService(name string) error {
	return fmt.Errorf("servicios Windows no están disponibles en esta plataforma")
}
