# RUTA INMUTABLE DE ADDONS

**Definitiva. Verificada con pruebas reales.**

---

## Flujo

```
Compilar → Firmar → Verificar → Instalar → Capabilities → Activar → Proxy → Desinstalar
```

## Evidencia real (pruebas que pasan)

### SDK (ERPGo-SDK)

| Prueba | Resultado |
|--------|-----------|
| `TestE2E_VerifyPackage_Firmado` | ✅ VerifyPackage OK |
| `TestE2E_ProcessHandshake` | ✅ gRPC Hello devuelve addon_name="hello" |
| `TestAddonRPCAuthenticatesAndServesHandshake` | ✅ Handshake OK |
| `TestTokenInterceptorRejectsMissingAndWrongTokens` | ✅ Token validado |
| `TestValidateManifestAcceptsValidContract` | ✅ Manifiesto válido aceptado |
| `TestValidateManifestRejectsDuplicateCapabilitiesAndUnsafeMenu` | ✅ Rechaza duplicados e inseguridad |

### Host (ERPGo)

| Prueba | Resultado |
|--------|-----------|
| `TestAddonE2E_Minimal` | ✅ Addon activo en BD, mock HTTP 200 |
| `TestAddonE2EDiscoverInstallProxyUninstall` | ✅ Discover→Install→Proxy→Uninstall completo |
| `TestAddonActivateExigeCapabilitiesDecididas` | ✅ Activación requiere capabilities |
| `TestAddonAdminPageListaCapabilities` | ✅ Admin lista capabilities |
| `TestAddonSaveCapabilitiesRechazaInventadas` | ✅ Rechaza capabilities no declaradas |
| `TestAddonE2EInstallSinFirmaRechaza` | ✅ Rechaza sin firma |
| `TestAddonE2EAddonInfo` | ✅ Info de addon OK |
| `TestAddonE2EAddonList` | ✅ Lista de addons OK |

---

## Paso a paso para crear un addon

### 1. Crear el addon

```go
// examples/hello/main.go
package main

import (
    "net/http"
    "github.com/orcanet/ergoaddon"
)

type HelloModule struct{}

func (m *HelloModule) Name() string { return "hello" }
func (m *HelloModule) Routes() []ergoaddon.Route {
    return []ergoaddon.Route{
        {Method: "GET", Path: "/", Handler: func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte("Hola addon"))
        }},
    }
}

func main() {
    ergoaddon.Serve(&HelloModule{})
}
```

### 2. Crear module.json

```json
{
    "schema": 1,
    "code": "hello",
    "name": "Hello World",
    "version": "1.0.0",
    "api": 1,
    "entry": "hello.exe",
    "binary_sha256": "<hash del binario>",
    "runtime": "grpc",
    "web": "own",
    "grpc": false,
    "permissions": [],
    "menus": [{"code": "hello_home", "label": "Hello", "url": "/mod/hello/", "sort_order": 100}]
}
```

### 3. Compilar

```bash
cd examples/hello && go build -o hello.exe .
```

### 4. Firmar

```bash
addon-sign sign --key tu_key.b64 --manifest module.json --sig module.sig
```

### 5. Instalar en ERPGo

Copiar carpeta `hello/` a `addons/hello/` del ERP (directorio = código del addon).

### 6. Activar desde ERP

Ir a **Configuración > Addons** → aprobar capabilities → activar.

### 7. Probar

```
GET /mod/hello/
```

---

## Lo que falta (pendiente, no bloqueante)

| Item | Estado | Impacto |
|------|--------|---------|
| Endpoint de upload desde UI | Pendiente | Usar copia manual por ahora |
| dispatchAddonEvent solo en invoice.issued | Pendiente | Solo retenciones IVA/ISLR usan eventos ahora |
| Migraciones de addons | Rechazada por diseño | Addons no tocan BD del ERP |

---

## Reglas críticas

1. **AddonName en gRPC Hello = código del addon** (no el nombre del manifiesto)
2. **Directorio = código del addon** (`addons/hello/`, no `addons/hello_world/`)
3. **Sin capabilities = auto-activo**; con capabilities = requiere aprobación admin
4. **Sin firma = rechazado** por VerifyPackage
5. **Addon solo comunica vía HTTP proxy** (no accede a BD del ERP)
6. **X-ERPGo-Token** = header que host inyecta al proxyar al addon
