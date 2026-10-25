//go:build !windows

package service

import "context"

// ServiceName es el nombre del servicio Windows; en otras plataformas solo se
// mantiene como constante para que el código fuente sea común.
const ServiceName = "ERPGoServer"

// IsWindowsService siempre es false fuera de Windows: no hay SCM.
func IsWindowsService() bool { return false }

// RunService no aplica fuera de Windows. Se ejecuta la app en modo consola y
// se bloquea, para que el comportamiento sea coherente si alguien lo llamara.
func RunService(app Application) error {
	if err := app.Start(); err != nil {
		return err
	}
	<-make(chan struct{})
	_ = context.Background()
	return nil
}