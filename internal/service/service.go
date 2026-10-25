// Package service adapta el servidor ERPGo para que corra igual como proceso
// de consola (doble clic, iniciar.bat, desarrollo) o, en Windows, como un
// servicio controlado por el Service Control Manager (SCM).
//
// El motivo de existir de este paquete: registrar un .exe con `sc.exe create`
// NO basta para que sea servicio. El binario debe, además, hacer el handshake
// StartServiceCtrlDispatcher con el SCM en unos pocos segundos. Si no lo hace,
// Windows rinde el tiempo y el arranque falla con el Error 1053. mysqld.exe lo
// trae de serie; un binario Go normal, no. Este paquete añade ese protocolo.
package service

import "context"

// Application es cualquier cosa que pueda arrancarse y detenerse ordenadamente.
type Application interface {
	// Start arranca el trabajo y se bloquea hasta que termine o falle.
	// Debe devolver nil si la parada fue ordenada.
	Start() error

	// Stop detiene la aplicación de forma ordenada dentro del tiempo dado
	// por ctx (drenar peticiones HTTP, cerrar conexiones, etc.).
	Stop(ctx context.Context) error
}