**INFORME DE ETAPA**  

1) **Que se hizo**  
Se migró el motor de datos de SQLite a MySQL 8, implementando un esquema con 415 sentencias en 16 archivos .sql (0 avisos, 121 tablas InnoDB). Se configuró una base por empresa (erpgo_<codigo>) y una base de control (erpgo). Se integró un servidor MySQL embebido que arranca en el puerto 3307 si no hay un servidor activo, con opción de desactivación mediante DB_AUTO_START=0.  

2) **Decisiones y por que**  
Se optó por MySQL en lugar de PostgreSQL porque el esquema existente era compatible, evitando una capa de traducción. La base por empresa se generó en ~9 segundos, atribuido a la creación de 121 tablas InnoDB, no a la red. El puerto 3307 se usó para evitar conflictos con instalaciones locales en 3306. El modo idempotentes garantiza consistencia en despliegues repetidos.  

3) **Lo que se rompio por el camino**  
- **IFNULL(fecha,'')** causó errores 1525 en 8 archivos de producción al comparar DATE con texto.  
- Tests escribían en bases reales: openTenantDB("default") creó 209 bases huérfanas acumuladas.  
- La base tenía 6 juegos de caracteres (utf8mb3, utf8mb4, latin1), provocando errores 1267 por colaciones incompatibles. Se unificó a utf8mb4_unicode_ci y se fijó la colación de conexión.  

4) **Estado actual de las pruebas**  
El banco de pruebas (tools/estress) ahora utiliza bases desechables por corrida, evitando interacciones con bases reales. La migración del esquema se validó con 0 avisos y ejecuciones idempotentes.  

5) **Lo que falta**  
- Limpiar las 209 bases huérfanas acumuladas por tests incorrectos.  
- Revisar si otros componentes del ERP dependen de tipos o funciones específicas de SQLite no soportados en MySQL (ej: manejo de fechas, funciones integradas).  
- Automatizar la validación de collations en nuevas bases de empresa para prevenir errores 1267.  

**Palabras clave**: Migración de base de datos, MySQL 8, esquema InnoDB, collations, IFNULL, bases huérfanas.