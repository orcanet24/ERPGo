package web

// pos_ratelimit.go — límite de VOLUMEN para el tráfico periódico del POS.
//
// El loginLimiter (handlers.go) protege contra fuerza bruta (bloquea tras N
// fallos). Esto es distinto: un token bucket que limita cuántas peticiones
// válidas por minuto acepta por terminal, para que una caja en mal estado (o
// un script que robe un token) no inunde el servidor con syncs/checkout.
//
// Es un límite generoso —una caja real manda una venta cada pocos segundos—
// pensado para cortar abuso, no para apretar la operación normal.

import (
	"sync"
	"time"
)

// posBucket describe un token bucket simple por clave.
type posBucket struct {
	tokens   float64
	capacity float64
	rate     float64 // tokens por segundo
	last     time.Time
}

type posRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*posBucket
	// capacity es el tamaño del bucket (ráfaga permitida); rate los tokens
	// por segundo (sostenido). Se comparten para todas las claves.
	capacity float64
	rate     float64
}

// newPosRateLimiter crea un limitador: `perMinute` peticiones sostenidas por
// minuto y una ráfaga de hasta `burst` encima de eso.
func newPosRateLimiter(perMinute, burst int) *posRateLimiter {
	return &posRateLimiter{
		buckets:  make(map[string]*posBucket),
		capacity: float64(burst),
		rate:     float64(perMinute) / 60.0,
	}
}

// allow consume un token para la clave. Devuelve false si no hay cupo.
func (l *posRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &posBucket{tokens: l.capacity, capacity: l.capacity, rate: l.rate, last: now}
		l.buckets[key] = b
	}
	// Reponer tokens según el tiempo transcurrido.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// posVolumeLimiter limita el volumen de las rutas POS autenticadas (checkout,
// catálogo, turnos, sync). Generoso: 300 req/min sostenidas con ráfaga de 60.
// Se aplica dentro de posAPIMiddleware (pos_api.go), que ya resolvió la sesión.
var posVolumeLimiter = newPosRateLimiter(300, 60)
