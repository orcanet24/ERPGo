# Manual de despliegue real del ERPGo con TLS

Cada comando de este manual **se ejecutó y se verificó** el 2026-10-11 en
Windows. Donde aparece un resultado, es el que produjo esta máquina, no uno
esperado. Lo que no se probó aquí (producción con dominio público) está
marcado como tal.

## Arquitectura

```
Internet → :8443 Caddy (TLS) → 127.0.0.1:8080 ERPGo (HTTP plano)
```

El ERP **nunca** se abre directo: sirve por HTTP porque detrás de Caddy la
capa de TLS la pone el proxy. La única puerta al exterior es el 8443/443.

## Requisitos

| Qué | De dónde | Verificado |
|---|---|---|
| Caddy 2.11.7 | `curl -sL -o caddy.zip https://caddyserver.com/api/download?os=windows&arch=amd64` | ✅ |
| OpenSSL 3.5.7 | Git for Windows lo trae (`/mingw64/bin/openssl`) | ✅ |
| curl 8.21.0 | Git for Windows (`/usr/bin/curl`) | ✅ |
| MySQL 8 corriendo | existente en esta instalación (`erpgo`, `erpgo_default`) | ✅ |
| Binario ERPGo | `go build -o erpgow.exe ./cmd/server` | ✅ |

**Ojo Windows:** el `.zip` de la release de GitHub dio descarga de 9 bytes
(release rota). El endpoint `caddyserver.com/api/download` funcionó bien
(52 MB, `caddy version` → v2.11.7).

## Paso 1 — Certificado

**Pruebas / LAN** (cert propio, 10 años):

```bash
mkdir -p deploy/certificados
cd deploy/certificados
openssl req -x509 -newkey rsa:2048 -sha256 -nodes \
  -keyout erpgow.key -out erpgow.crt -days 3650 \
  -subj "//CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,DNS:erpgo.pruebas.local"
```

⚠️ Detalle que rompe en Windows: `–subj "/CN=..."` (una barra) falla porque
Git Bash expande la ruta como `C:/Program Files/Git/CN=...`. Va `//CN=...`
(doble barra) o `MSYS_NO_PATHCONV=1` delante.

Verificado:
```
subject=CN=localhost
X509v3 Subject Alternative Name:
    DNS:localhost, IP Address:127.0.0.1, DNS:erpgo.pruebas.local
```

**Producción (dominio real):** no se genera certificado a mano. Caddy lo
emite y renueva de Let's Encrypt automáticamente si se configura
`tls erpgo@midominio.com` y el 443 es alcanzable desde internet. **Este
camino no se ejecutó en esta prueba** (no hay dominio público aquí); se
sustituye un solo bloque del Caddyfile, el paso 2 lo marca.

## Paso 2 — Caddyfile

El que está en [deploy/Caddyfile](Caddyfile) ya está validado con:

```bash
caddy validate --config deploy/Caddyfile
# → Valid configuration
```

Contenido (probado):

```caddyfile
{
	admin off
	auto_https off
}

https://localhost:8443, https://erpgo.pruebas.local:8443 {
	tls deploy/certificados/erpgow.crt deploy/certificados/erpgow.key

	reverse_proxy {
		to 127.0.0.1:8080
		transport http {
			response_header_timeout 120s
		}
		header_up X-Real-IP {remote_host}
		header_up X-Forwarded-Proto https
	}
}
```

Para **producción con dominio real** cambian tres líneas:

```caddyfile
https://erpgo.midominio.com:443 {
	tls erpgo@midominio.com     # ← emisión automática Let's Encrypt
	# lo demás igual
}
```

Una dirección de sitio, no dos. `auto_https off` se cambia a on (o se
quita el bloque global) en producción.

## Paso 3 — Arranque del ERP

El ERP escucha HTTP plano **solo en loopback** (`.env`):

```ini
SERVER_HOST=127.0.0.1
SERVER_PORT=8080
```

⚠️ Jamás `0.0.0.0` con este diseño: la instancia que corre en `0.0.0.0:8080`
queda expuesta por TCP plano, sin TLS y sin proxy, y Caddy encima no la
protege. Se corrigió en esta máquina durante el despliegue (havía erpgow.exe
sujetando `:8080` en 0.0.0.0; se mató).

Arranque en primer plano (consola visible):

```bash
./erpgow.exe
# → ERPGo server starting on http://127.0.0.1:8080
```

Arranque en background con log:

```bash
(./erpgow.exe > erpgow.log 2>&1 &)
# ERP en PID $! tras arrancar
```

## Paso 4 — Arrancar Caddy

```bash
caddy start --config deploy/Caddyfile
```

Chequeo del listener:

```bash
netstat -ano | grep :8443
# → TCP 0.0.0.0:8443   LISTENING   <PID de caddy>
```

## Paso 5 — Verificación con curl (lo que demuestra que sirve)

Todas las siguientes pasaron por esta máquina:

```bash
# 1. HTTPS con confianza en TU certificado (no -k; se verifica de verdad)
curl -s -o /dev/null -w "HTTP %{http_code}\n" \
  --cacert deploy/certificados/erpgow.crt https://localhost:8443/login
# → HTTP 200

# 2. Certificado del dominio de prueba sin tocar el hosts
curl -s -o /dev/null -w "HTTP %{http_code}\n" \
  --cacert deploy/certificados/erpgow.crt \
  --resolve erpgo.pruebas.local:8443:127.0.0.1 \
  https://erpgo.pruebas.local:8443/login
# → HTTP 200

# 3. Activos estáticos pesados (57 KB) también pasan por TLS
curl -s -o /dev/null -w "HTTP %{http_code} | %{size_download} bytes\n" \
  --cacert deploy/certificados/erpgow.crt \
  https://localhost:8443/static/css/erpgow-tema.css
# → HTTP 200 | 57124 bytes

# 4. El flujo POST llega al ERP (401 de credenciales malas es correcto:
#    demuestra que la petición TLS→proxy→ERP→MySQL cicla entera)
curl -s -o /dev/null -w "HTTP %{http_code}\n" \
  --cacert deploy/certificados/erpgow.crt \
  -X POST https://localhost:8443/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"incorrecta_de_proposito"}'
# → HTTP 401

# 5. Ver el cert que el TLS está sirviendo
echo | openssl s_client -connect localhost:8443 -servername localhost 2>/dev/null \
  | openssl x509 -noout -subject -dates
# → subject=CN=localhost / notAfter Oct 8 2036 (10 años)

# 6. NEGATIVA (prueba que la verificación TLS funciona de verdad):
curl -s https://localhost:8443/login >/dev/null 2>&1
echo $?
# → 60 (certificado no confiable) — SIN --cacert, curl rechaza. Es lo
#    esperado en pruebas con cert propio, no un fallo.

# 7. El backend plano NO está expuesto al exterior:
curl -s --max-time 3 http://<ip-LAN>:8080/login -o /dev/null -w "%{http_code}"
# → 000 (conexión rechazada). Solo 127.0.0.1 lo alcanza.
```

## Paso 6 — Cierre de sesión del ERP vía HTTPS (opcional, trayecto completo)

En un navegador apuntando a `https://localhost:8443` se abre el login del
ERP. Se puede instalar el `.crt` en el almacén "Entidades de certificación
raíz de confianza" para que el candado verde sin `-k`/excepción:

```bash
# Importar el cert en el almacén del usuario (PowerShell):
Import-Certificate -FilePath .\deploy\certificados\erpgow.crt -CertStoreLocation Cert:\CurrentUser\Root
```

Este paso es local, no se ejecuta en esta sesión (no toca el almacén de
Windows sin tu permiso explícito).

## Lo que NO se probó aquí (honesto por diseño)

| Punto | Motivo |
|---|---|
| Cert Let's Encrypt / dominio público | no hay DNS público y 443 no es alcanzable desde internet en esta red; el cambio de bloque está documentado y validado sintácticamente |
| Importación del .crt al almacén de Windows | toca configuración del sistema, se hace solo con tu permiso |
| gRPC del POS (:9095) tras proxy TLS | el terminal POS usa su propio canal seguro (`config/netsec.go`); se probó aparte, no en este manual |

## Parar / reiniciar

```bash
taskkill //F //IM caddy.exe   # para Caddy
# ⚠️ VERIFICADO EN VIVO: `caddy stop` NO funciona con este Caddyfile porque
# lleva `admin off` (no hay endpoints de administración a los que pedirle
# la parada). El error real fue:
#   Post "http://localhost:2019/stop": connection refused
# Si quieres `caddy stop`, quita `admin off` del Caddyfile; la parada por
# taskkill es igual de válida y no deja listener en 2019.

taskkill //F //IM erpgow.exe  # mata el ERP (no conserva estado: MySQL manda)
```

Verificado tras la parada: `netstat -ano | grep :8443` sin LISTENING (puerto
libre) y el ERP sigue aislado en 127.0.0.1:8080.
