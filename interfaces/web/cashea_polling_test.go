package web

// cashea_polling_test.go — El arranque del ERP se colgaba y el puerto 8586 nunca
// se abría: StartCasheaStatusPoller tenía un `for range ticker.C` infinito y se
// llamaba EN SERIE desde main, que por tanto nunca llegaba a levantar el HTTP
// (el proceso quedaba vivo pero sin escuchar).
//
// Esta prueba fija el contrato que lo evita: arrancar el polling NO debe
// bloquear a quien lo llama.

import (
	"testing"
	"time"
)

func TestArrancarElPollingDeCasheaNoBloquea(t *testing.T) {
	s, _ := basePrueba(t)

	// Intervalo largo: la prueba mide el arranque, no quiere ciclos corriendo.
	viejo := casheaPollInterval
	casheaPollInterval = time.Hour
	t.Cleanup(func() { casheaPollInterval = viejo })

	hecho := make(chan struct{})
	go func() {
		s.StartCasheaStatusPoller()
		close(hecho)
	}()

	select {
	case <-hecho:
		// Contrato cumplido: la función devuelve y el caller puede seguir
		// (aquí, main sigue hasta abrir el puerto 8586).
	case <-time.After(3 * time.Second):
		t.Fatal("StartCasheaStatusPoller bloqueó: el arranque del HTTP quedaba colgado y el puerto 8586 no se abría")
	}
}
