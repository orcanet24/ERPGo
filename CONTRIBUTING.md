# Contribuir a ERPGo

ERPGo es software libre bajo **GPLv3**: todo cambio que se integre queda bajo esa
misma licencia y el código sigue siendo público, también en los derivados.

No hay CLA (cesión de derechos): cada commit debe ir **firmado** con
`git commit -s` (Developer Certificate of Origin), que declara que tienes
derecho a aportar ese código.

## Antes de enviar un cambio

Desde la raíz del proyecto, todo debe quedar en verde:

```bash
go build ./...
go vet ./...
go test ./...
go run ./internal/webcheck -dir templates   # si tocaste una plantilla
```

El CI del repositorio corre esa misma batería: un cambio en rojo no se integra.

Las pruebas que necesitan MySQL real se activan con `TEST_DB_NAME=erpgo_test_<algo>`,
`TEST_DB_USER` y `TEST_DB_HOST`; sin esas variables se saltan y el resto corre sobre
SQLite.

## Forma de contribuir

- **Un cambio, una explicación**: mensaje de commit en español que diga QUÉ cambió
  y POR QUÉ (no un «fix» genérico).
- **Causa raíz, no parche**: si algo falla, se corrige el defecto real y se deja una
  prueba que lo delate si vuelve.
- **Datos y migraciones**: cualquier cambio de esquema va por `migrations/`, es
  idempotente y no borra datos históricos (ver la política del README).
- **Nunca al repositorio**: claves, `.env`, bases locales, respaldos ni capturas
  con datos de clientes.
- Los comentarios y textos de interfaz van en español, como el resto del proyecto.

## Reportar un fallo

Abre un issue en <https://github.com/orcanet24/ERPGo> con:

1. Qué esperabas que pasara y qué pasó.
2. Pasos para reproducirlo.
3. Versión (binario o commit) y sistema operativo.
4. Si hay datos de clientes de por medio, **no los pegues**: reemplázalos por
   ejemplos ficticios.

Para vulnerabilidades de seguridad, no abras un issue público: escribe a los
mantenedores de Orcanet1724 C.A. por el canal de contacto del proyecto.
