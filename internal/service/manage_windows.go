//go:build windows

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

// InstallService registra el binario actual como servicio de Windows usando
// el SCM nativo (sin depender de sc.exe). El servicio queda configurado con
// inicio automático y el nombre indicado (por defecto ServiceName).
//
// El binPath incluye la ruta completa entre comillas, que es lo que sc.exe
// exige cuando el path tiene espacios.
func InstallService(name, displayName, description string) error {
	if name == "" {
		name = ServiceName
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("no se pudo resolver la ruta del ejecutable: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("no se pudo obtener la ruta absoluta: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar al Service Control Manager: %w", err)
	}
	defer m.Disconnect()

	// Si el servicio ya existe, eliminarlo primero (reinstalación limpia).
	if existing, err := m.OpenService(name); err == nil {
		existing.Close()
		if err := removeService(name); err != nil {
			return fmt.Errorf("eliminar servicio existente: %w", err)
		}
	}

	cfg := mgr.Config{
		DisplayName:  displayName,
		Description:  description,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		Dependencies: nil,
		SidType:      0xFFFFFFFF, // SERVICE_SID_TYPE_UNRESTRICTED
	}

	svcHandle, err := m.CreateService(name, exePath, cfg)
	if err != nil {
		return fmt.Errorf("crear servicio %q: %w", name, err)
	}
	defer svcHandle.Close()

	// Configurar recuperación ante fallos: reiniciar en 5s el primer y
	// segundo fallo, y reiniciar después de fallos subsecuentes.
	resetPeriod := uint32(86400) // 24 horas en segundos
	actions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
	}

	if err := svcHandle.SetRecoveryActions(actions, resetPeriod); err != nil {
		// No fatal: el servicio sigue funcionando sin recuperación.
		fmt.Printf("Aviso: no se pudo configurar recuperación automática: %v\n", err)
	}

	if description != "" {
		// SetDescription no está disponible en esta versión de golang.org/x/sys/windows.
		// Se puede configurar vía registry o sc.exe, pero no es crítico.
		_ = description
	}

	return nil
}

// UninstallService detiene (si está corriendo) y elimina el servicio indicado.
// Si el nombre está vacío, usa ServiceName.
func UninstallService(name string) error {
	if name == "" {
		name = ServiceName
	}
	return removeService(name)
}

// removeService detiene y elimina un servicio del SCM.
func removeService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("conectar al Service Control Manager: %w", err)
	}
	defer m.Disconnect()

	svcHandle, err := m.OpenService(name)
	if err != nil {
		if isErrServiceNotExist(err) {
			return nil // ya no existe
		}
		return fmt.Errorf("abrir servicio %q: %w", name, err)
	}
	defer svcHandle.Close()

	// Detener si está corriendo.
	status, err := svcHandle.Query()
	if err == nil && status.State != 0 && status.State != 4 { // 4 = SERVICE_STOPPED
		if _, err := svcHandle.Control(1); err != nil { // 1 = SERVICE_CONTROL_STOP
			// No fatal: intentar eliminar de todos modos.
			fmt.Printf("Aviso: no se pudo detener el servicio (se eliminará de todos modos): %v\n", err)
		}
		// Esperar a que se detenga (hasta 15s).
		deadline := time.Now().Add(15 * time.Second)
		for {
			status, err := svcHandle.Query()
			if err == nil && status.State == 4 {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	if err := svcHandle.Delete(); err != nil {
		return fmt.Errorf("eliminar servicio %q: %w", name, err)
	}

	return nil
}

// isErrServiceNotExist detecta el error 1060 (ERROR_SERVICE_DOES_NOT_EXIST).
func isErrServiceNotExist(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "1060") ||
		strings.Contains(msg, "does not exist")
}
