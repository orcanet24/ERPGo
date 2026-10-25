//go:build windows

package service

import (
	"context"
	"log"
	"time"

	"golang.org/x/sys/windows/svc"
)

// ServiceName es el nombre con el que el servicio queda registrado en el SCM.
// Debe coincidir con el que usa el instalador (ERPgo-Installer.iss).
const ServiceName = "ERPGoServer"

// IsWindowsService informa si el proceso fue lanzado por el Service Control
// Manager. Si es true hay que atender el protocolo de servicio; si no, se
// arranca en modo consola como siempre.
func IsWindowsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		// Ante la duda (no se pudo determinar) se asume modo consola: así un
		// fallo de detección nunca impide arrancar el programa a mano.
		return false
	}
	return isSvc
}

// RunService bloquea ejecutando app bajo el SCM hasta que llegue una petición
// de stop o shutdown. Es lo que hace el StartServiceCtrlDispatcher que le
// faltaba al binario y que, sin esto, provoca el Error 1053.
func RunService(app Application) error {
	return svc.Run(ServiceName, &handler{app: app})
}

// handler implementa svc.Handler: el SCM le pide que arranque, y a partir de
// ahí reenvía las órdenes de control (stop/shutdown) a la aplicación.
type handler struct {
	app Application
}

func (h *handler) Execute(args []string, req <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const aceptaCtrl = svc.AcceptStop | svc.AcceptShutdown

	// Arranque: se avisa START_PENDING, se lanza la app en segundo plano y se
	// pasa a RUNNING. Los avisos de estado son lo que evita que el SCM crea
	// que el servicio se colgó durante el arranque.
	changes <- svc.Status{State: svc.StartPending}

	errCh := make(chan error, 1)
	go func() { errCh <- h.app.Start() }()

	changes <- svc.Status{State: svc.Running, Accepts: aceptaCtrl}

	for {
		select {
		case err := <-errCh:
			// El servidor terminó por su cuenta (fallo o parada ordenada).
			if err != nil {
				log.Printf("servicio: el servidor terminó con error: %v", err)
				return false, 1
			}
			return false, 0

		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				// Se drena ordenadamente; el SCM espera ~30 s por defecto, de
				// ahí el margen de 20 s para no excederlo.
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				if err := h.app.Stop(ctx); err != nil {
					log.Printf("servicio: apagado ordenado con error: %v", err)
				}
				cancel()
				return false, 0
			}
		}
	}
}