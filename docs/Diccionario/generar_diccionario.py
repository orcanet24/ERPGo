# -*- coding: utf-8 -*-
"""
Genera el Diccionario de Scripts interactivo de ERPGo.
Escanea el proyecto, extrae funciones con sus comentarios y produce:
  - datos.js   (datos del diccionario)
  - index.html (visor interactivo con arbol + panel de detalle)

Uso: python generar_diccionario.py   (ejecutar desde docs/Diccionario o cualquier lado)
"""
import os, re, json, sys, datetime, html

RAIZ = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
SALIDA = os.path.join(RAIZ, "docs", "Diccionario")

# ---------------------------------------------------------------- configuracion
OMITIR_DIRS = {
    ".git", ".github", ".commandcode", ".freebuff", ".testlogs", "node_modules",
    "bin", "obj", "vendor", "logs", "uploads", "tmp_final_tests", "tmp_tools",
    "tmp_review", "protoc", "protoc_bin", "backup", "data", "assets",
    "static\\plugins", "static\\js\\mousetrap-master", "static\\installer",
    "static\\dist", "static\\datatables", "static\\data",
    "legacy\\.freebuff", "examples", "instalador\\Output",
}
OMITIR_DIRS_NORM = {d.replace("\\", "/") for d in OMITIR_DIRS}

EXT_LENGUAJE = {
    ".go": "Go", ".py": "Python", ".js": "JavaScript", ".ts": "TypeScript",
    ".sql": "SQL", ".bat": "Batch", ".cmd": "Batch", ".sh": "Shell", ".ps1": "PowerShell",
}

# palabras clave del nombre de archivo -> descripcion corta en español
SUSTANTIVOS = {
    "invoice": "Facturas", "quotation": "Cotizaciones", "order": "Ordenes",
    "purchase": "Compras", "supplier": "Proveedores", "customer": "Clientes",
    "product": "Productos", "inventory": "Inventario", "stock": "Stock",
    "kardex": "Kardex (movimientos de inventario)", "pos": "Punto de Venta (POS)",
    "payroll": "Nómina", "hr": "Recursos Humanos", "crm": "CRM",
    "account": "Contabilidad / Cuentas", "accounting": "Contabilidad",
    "bank": "Bancos", "cash": "Caja", "payment": "Pagos", "pagar": "Cuentas por pagar",
    "cartera": "Cartera (cuentas por cobrar)", "report": "Reportes",
    "fiscal": "Fiscal (impuestos / facturación fiscal)", "tax": "Impuestos",
    "menu": "Menú", "user": "Usuarios", "session": "Sesiones",
    "permission": "Permisos", "security": "Seguridad", "csrf": "CSRF",
    "password": "Contraseñas", "identity": "Identidad", "auth": "Autenticación",
    "tenant": "Multi-empresa (tenants)", "tenants": "Multi-empresa (tenants)",
    "ai": "Inteligencia Artificial", "orchestrator": "Orquestador",
    "queue": "Cola de trabajos", "worker": "Trabajador en segundo plano",
    "upload": "Subida de archivos", "database": "Base de datos", "mysql": "MySQL",
    "schema": "Esquema SQL", "migration": "Migraciones", "setup": "Instalación / Setup",
    "wizard": "Asistente de instalación", "config": "Configuración",
    "dashboard": "Panel / Dashboard", "appearance": "Apariencia",
    "currency": "Moneda", "exchange": "Tipo de cambio", "cost": "Costos",
    "price": "Precios", "return": "Devoluciones", "adjustment": "Ajustes",
    "delivery": "Entregas", "credit": "Notas de crédito", "debit": "Notas de débito",
    "voucher": "Comprobantes", "coa": "Plan de cuentas (COA)",
    "period": "Período contable", "close": "Cierre", "legal": "Libros legales",
    "conasev": "CONASEV", "igtf": "IGTF", "audit": "Auditoría",
    "helper": "Funciones auxiliares", "helpers": "Funciones auxiliares",
    "service": "Lógica de servicio", "server": "Servidor", "handler": "Handlers HTTP",
    "handlers": "Handlers HTTP", "render": "Renderizado de plantillas",
    "template": "Plantillas", "cache": "Caché", "gzip": "Compresión gzip",
    "sync": "Sincronización", "guard": "Guardián / validación", "grpc": "gRPC",
    "semilla": "Semillas de datos de prueba", "seed": "Semillas de datos",
    "adapter": "Adaptador (impresora fiscal)", "diag": "Diagnóstico",
    "tool": "Utilidad", "tools": "Utilidades", "firmar": "Firma de binarios",
    "limpiar": "Limpieza", "estress": "Prueba de estrés", "loadtest": "Prueba de carga",
    "backup": "Respaldos", "mariadb": "MariaDB embebida", "port": "Puertos",
    "wal": "WAL (integridad SQLite)", "convert": "Conversión", "dump": "Volcado SQL",
    "plantilla": "Plantillas de módulos", "copy": "Copia de tablas",
    "admin": "Administración", "menu_manager": "Gestor de menú",
    "addon": "Add-ons / módulos extra", "notify": "Notificaciones",
    "capability": "Capacidades", "host": "Host de add-ons",
    "view_pref": "Preferencias de vista", "setting": "Ajustes",
    "first_run": "Primer arranque", "company": "Empresa",
    "cycle_count": "Conteo cíclico", "fefo": "FEFO (primero en expirar)",
    "reorder": "Punto de reorden", "service": "Servicios",
    "day_book": "Libro diario", "income": "Estado de resultados",
    "fixed": "Activos fijos", "today": "Reporte del día",
    "reconciliation": "Conciliación bancaria", "financial": "Ejercicio fiscal",
    "subaccount": "Subcuentas", "journal": "Asientos contables",
    "fieldkey": "Llaves de campos", "docgen": "Generador de documentación",
    "webcheck": "Verificador web", "overlay": "Overlay de documentación",
    "graph": "Grafo de dependencias", "scan": "Escáner de código",
    "manual": "Manual de usuario", "semillar": "Semillero", "genhash": "Generador de hashes",
    "logger": "Registro de logs (logger)", "crypto": "Criptografía",
    "manage": "Gestión de servicio Windows", "schema_check": "Verificación de esquema",
    "schema_apply": "Aplicación de esquema", "schema_integrity": "Integridad de esquema",
    "ledger": "Libro mayor de migraciones", "embedded": "MySQL embebido",
    "sqlite": "SQLite", "testdb": "Base de datos de pruebas",
    "pos_api": "API del POS", "pos_admin": "Administración POS",
    "pos_auth": "Autenticación POS", "pos_tenant": "Tenants del POS",
    "pos_ratelimit": "Límite de tasa POS", "pos_fiscal": "Bloques fiscales POS",
    "json": "Utilidades JSON", "dialect": "Dialecto SQL", "md5": "Compatibilidad MD5",
    "worker": "Trabajadores", "multi_currency": "Multi-moneda",
    "legal_books": "Libros legales", "module": "Módulos", "database_settings": "Ajustes de BD",
    # --- cobertura de nombres compuestos / sueltos ---
    "creditnote": "Notas de crédito", "debitnote": "Notas de débito",
    "cyclecount": "Conteo cíclico de inventario", "first": "Primer arranque",
    "run": "Ejecución / arranque", "md5compat": "Compatibilidad con claves MD5",
    "permissions": "Permisos", "view": "Vistas", "pref": "Preferencias de usuario",
    "context": "Contexto y control de trabajos", "manager": "Gestor de recursos",
    "cleanup": "Limpieza / mantenimiento", "collation": "Colación de base de datos",
    "iniciar": "Inicio del ERP (instalación)", "walguard": "Guardián WAL de SQLite",
    "mysqldump": "Volcado/respaldo de MySQL", "loadtest": "Prueba de carga",
    "extraer": "Extracción de tablas/VBA", "pickerjs": "Selector de fechas (DatePicker)",
    "tablehead": "Cabeceras fijas de tablas", "tableheadfixer": "Cabeceras fijas de tablas", "custom": "Personalización de UI",
    "server": "Servidor HTTP principal", "main": "Punto de entrada",
    "plantillas": "Plantillas de módulos", "doc": "Documentación",
    "tools": "Utilidades de desarrollo", "cmd": "Herramientas de línea de comandos",
    "queue": "Cola de trabajos", "upload": "Subida de archivos",
    "persistence": "Persistencia de datos", "interfaces": "Capa web/interfaces",
    "internal": "Lógica interna", "infrastructure": "Infraestructura",
    "instalador": "Instalador de Windows",
    "compilar": "Compilación", "lanzamiento": "Lanzamiento", "protegido": "Protección de código",
    "respaldar": "Respaldo de base de datos", "instalar": "Instalación",
}

# ------------------------------------------------------------ desc. automáticas
# Verbos (inglés -> español) para funciones sin doc comment
VERBOS_ES = {
    "create": "Crea", "add": "Añade", "append": "Añade", "insert": "Inserta", "new": "Crea",
    "get": "Obtiene", "fetch": "Obtiene", "load": "Carga", "list": "Lista", "ls": "Lista",
    "find": "Busca", "search": "Busca", "query": "Consulta", "read": "Lee", "scan": "Recorre",
    "update": "Actualiza", "set": "Asigna", "save": "Guarda", "store": "Almacena", "edit": "Edita",
    "modify": "Modifica", "patch": "Actualiza", "upsert": "Inserta o actualiza",
    "delete": "Elimina", "remove": "Elimina", "drop": "Elimina", "clear": "Vacía", "purge": "Elimina",
    "handle": "Maneja", "process": "Procesa", "serve": "Atiende", "dispatch": "Despacha",
    "validate": "Valida", "check": "Comprueba", "verify": "Verifica", "ensure": "Asegura", "assert": "Comprueba",
    "is": "Indica si", "has": "Indica si", "can": "Indica si puede", "should": "Indica si debe",
    "register": "Registra", "unregister": "De-registra", "init": "Inicializa", "setup": "Configura",
    "start": "Inicia", "stop": "Detiene", "pause": "Pausa", "resume": "Reanuda", "run": "Ejecuta", "exec": "Ejecuta",
    "close": "Cierra", "open": "Abre", "connect": "Conecta", "disconnect": "Desconecta",
    "parse": "Analiza", "decode": "Decodifica", "encode": "Codifica", "marshal": "Serializa",
    "unmarshal": "Deserializa", "convert": "Convierte", "transform": "Transforma",
    "build": "Construye", "make": "Construye", "compute": "Calcula", "calculate": "Calcula",
    "gen": "Genera", "generate": "Genera", "render": "Renderiza", "draw": "Dibuja",
    "send": "Envía", "notify": "Notifica", "emit": "Emite", "push": "Envía",
    "login": "Inicia sesión de", "logout": "Cierra sesión de", "authenticate": "Autentica",
    "authorize": "Autoriza", "hash": "Calcula hash de", "encrypt": "Cifra", "decrypt": "Descifra",
    "sign": "Firma", "migrate": "Migra", "backup": "Respaldan", "restore": "Restaura",
    "import": "Importa", "export": "Exporta", "copy": "Copia", "move": "Mueve", "rename": "Renombra",
    "log": "Registra en log", "debug": "Depura", "dump": "Vuelca", "flush": "Vuelca",
    "sync": "Sincroniza", "merge": "Fusiona", "split": "Divide", "join": "Une",
    "count": "Cuenta", "sum": "Suma", "avg": "Calcula promedio de",
    "max": "Obtiene el máximo de", "min": "Obtiene el mínimo de",
    "filter": "Filtra", "sort": "Ordena", "group": "Agrupa", "paginate": "Pagina",
    "format": "Formatea", "trim": "Recorta", "normalize": "Normaliza", "clean": "Limpia",
    "compare": "Compara", "equals": "Compara con", "match": "Comprueba coincidencia de",
    "wait": "Espera", "sleep": "Duerme", "retry": "Reintenta",
    "watch": "Observa", "listen": "Escucha", "accept": "Acepta", "bind": "Enlaza", "apply": "Aplica",
    "enable": "Habilita", "disable": "Deshabilita", "toggle": "Alterna", "reset": "Restablece",
    "reload": "Recarga", "refresh": "Refresca", "cache": "Usa caché de", "invalidate": "Invalida",
    "resolve": "Resuelve", "lookup": "Busca", "exists": "Comprueba la existencia de",
    "empty": "Comprueba si está vacío", "required": "Comprueba si es obligatorio",
}

# Sustantivos/abreviaturas frecuentes en nombres de funciones
NOMBRES_FUNC = {
    "invoice": "facturas", "customer": "clientes", "client": "clientes", "product": "productos",
    "order": "órdenes", "purchase": "compras", "payment": "pagos", "user": "usuarios",
    "role": "roles", "perm": "permisos", "permission": "permisos", "menu": "menú",
    "company": "empresa", "tenant": "empresa (tenant)", "stock": "stock", "kardex": "kardex",
    "report": "reportes", "tax": "impuestos", "currency": "moneda", "bank": "bancos",
    "cash": "caja", "price": "precios", "cost": "costos", "supplier": "proveedores",
    "quotation": "cotizaciones", "payroll": "nómina", "login": "inicio de sesión",
    "auth": "autenticación", "password": "contraseñas", "csrf": "CSRF", "jwt": "JWT",
    "handler": "peticiones HTTP", "route": "rutas", "rutas": "rutas", "endpoint": "endpoints",
    "api": "API", "db": "base de datos", "sql": "SQL", "query": "consultas", "row": "filas",
    "rows": "filas", "json": "JSON", "html": "HTML", "template": "plantillas", "cache": "caché",
    "file": "archivos", "dir": "directorios", "path": "rutas", "name": "nombres", "id": "ID",
    "data": "datos", "body": "cuerpo", "header": "cabeceras", "param": "parámetros",
    "status": "estados", "error": "errores", "msg": "mensajes", "message": "mensajes",
    "log": "logs", "time": "tiempos", "date": "fechas", "total": "totales", "count": "conteos",
    "list": "listas", "item": "elementos", "config": "configuración", "cfg": "configuración",
    "ctx": "contexto", "req": "peticiones", "res": "respuestas", "err": "errores",
    "sum": "sumas", "page": "páginas", "filter": "filtros", "sort": "ordenación",
    "limit": "límites", "start": "inicios", "end": "fines", "ok": "éxito", "flag": "indicadores",
    "mode": "modos", "format": "formatos", "value": "valores", "key": "claves",
    "input": "entradas", "output": "salidas", "args": "argumentos", "opts": "opciones",
    "option": "opciones", "conn": "conexiones", "pool": "pool de conexiones",
    "tx": "transacciones", "stmt": "sentencias", "pdf": "PDF", "xml": "XML", "csv": "CSV",
    "pos": "POS", "fiscal": "módulo fiscal", "backup": "respaldos", "queue": "colas",
    "job": "trabajos", "worker": "trabajadores", "test": "pruebas", "debug": "depuración",
    "default": "predeterminados", "utils": "utilidades", "helper": "funciones auxiliares",
    "http": "HTTP", "url": "URL", "uri": "URI", "hash": "hashes", "md5": "MD5",
    "bcrypt": "contraseñas (bcrypt)", "acl": "ACL", "port": "puertos", "host": "hosts",
    "schema": "esquemas", "table": "tablas", "column": "columnas", "index": "índices",
    "field": "campos", "record": "registros", "settings": "ajustes", "setting": "ajustes",
    "preference": "preferencias", "session": "sesiones", "token": "tokens", "credit": "créditos",
    "debit": "débitos", "note": "notas", "notes": "notas", "line": "líneas", "header": "cabeceras",
    "detail": "detalles", "details": "detalles", "summary": "resúmenes", "state": "estados",
    "size": "tamaños", "length": "longitudes", "index": "índices", "type": "tipos",
    "kind": "tipos", "code": "códigos", "id": "ID", "ids": "IDs",
    "by": "por", "for": "para", "from": "de", "with": "con", "without": "sin",
    "all": "todos los", "to": "a", "in": "en", "at": "en", "on": "en",
    "before": "antes de", "after": "después de", "if": "si", "or": "o", "and": "y",
    "no": "sin", "is": "de", "the": "", "a": "", "an": "", "el": "", "la": "",
    "edge": "aristas", "edges": "aristas", "target": "objetivos", "targets": "objetivos",
    "graph": "grafo", "overlay": "capa de sustitución", "file": "archivos", "files": "archivos",
    "path": "rutas", "paths": "rutas", "name": "nombres", "names": "nombres",
    "prefix": "prefijos", "suffix": "sufijos", "exists": "existencia de",
    "contains": "contenido de", "call": "llamadas", "calls": "llamadas",
    "scan": "escaneo de", "show": "muestra", "resolve": "resolución de",
    "invent": "invención de", "external": "externos", "relation": "relaciones",
    "relations": "relaciones", "deterministic": "determinismo de",
    "documented": "documentación de", "confidence": "confianza de",
    "does": "", "not": "no", "two": "dos", "deep": "anidados",
    "pkg": "paquete", "import": "importaciones", "exports": "exportaciones",
    "export": "exportaciones", "ref": "referencias", "refs": "referencias",
    "write": "escritura de", "reader": "lectores de", "writer": "escritores de",
    "bytes": "bytes", "buffer": "búferes", "reader": "lectura", "writer": "escritura",
    "ctx": "contexto", "done": "finalización de", "ok": "éxito de",
    "walk": "recorrido de", "visit": "visita de", "node": "nodos", "nodes": "nodos",
    "leaf": "hojas", "root": "raíz", "child": "hijos", "children": "hijos",
    "parent": "padres", "depth": "profundidad", "level": "niveles", "layer": "capas",
    "route": "rutas", "routes": "rutas", "router": "enrutadores",
    "form": "formularios", "forms": "formularios", "view": "vistas", "views": "vistas",
    "page": "páginas", "pages": "páginas", "template": "plantillas", "templates": "plantillas",
    "panel": "paneles", "menu": "menús", "tab": "pestañas", "tabs": "pestañas",
    "button": "botones", "buttons": "botones", "input": "entradas", "inputs": "entradas",
    "checkbox": "casillas", "select": "selecciones", "option": "opciones", "options": "opciones",
    "default": "valores por defecto", "defaults": "valores por defecto",
    "config": "configuración", "configuration": "configuración", "option": "opciones",
    "cancel": "cancelación de", "check": "verificación de", "verify": "verificación de",
    "save": "guardado de", "load": "carga de", "render": "renderizado de",
    "filter": "filtros", "filters": "filtros", "sort": "ordenación de", "order": "órdenes",
    "status": "estados", "error": "errores", "errors": "errores", "warning": "advertencias",
    "message": "mensajes", "messages": "mensajes", "text": "texto", "string": "cadenas",
    "number": "números", "value": "valores", "values": "valores", "data": "datos",
    "item": "elementos", "items": "elementos", "list": "listas", "lists": "listas",
    "map": "mapas", "set": "conjuntos", "array": "arreglos", "slice": "rebanadas",
    "count": "recuentos", "total": "totales", "sum": "sumas", "average": "promedios",
    "max": "máximo de", "min": "mínimo de", "first": "primer", "last": "último",
    "next": "siguiente", "prev": "anterior", "previous": "anterior",
    "begin": "inicio de", "start": "inicio de", "end": "fin de", "finish": "fin de",
    "open": "apertura de", "close": "cierre de", "init": "inicialización de",
    "setup": "configuración de", "install": "instalación de", "uninstall": "desinstalación de",
    "login": "inicio de sesión", "logout": "cierre de sesión", "auth": "autenticación",
    "authz": "autorización", "session": "sesiones", "user": "usuarios", "users": "usuarios",
    "admin": "administración", "role": "roles", "roles": "roles", "perm": "permisos",
    "permision": "permisos", "permisos": "permisos",
    "invoice": "facturas", "bill": "facturas", "customer": "clientes", "client": "clientes",
    "product": "productos", "category": "categorías", "stock": "existencias",
    "inventory": "inventario", "warehouse": "almacenes", "payment": "pagos",
    "purchase": "compras", "sale": "ventas", "order": "órdenes", "quote": "cotizaciones",
    "quotation": "cotizaciones", "expense": "gastos", "income": "ingresos",
    "bank": "bancos", "cash": "efectivo", "accounting": "contabilidad",
    "audit": "auditoría", "report": "informes", "reports": "informes",
    "export": "exportaciones", "import": "importaciones", "upload": "subidas de",
    "download": "descargas de", "file": "archivos", "folder": "carpetas",
    "image": "imágenes", "photo": "fotos", "avatar": "avatares",
    "email": "correos", "phone": "teléfonos", "address": "direcciones",
    "city": "ciudades", "country": "países", "state": "estados", "zip": "códigos postales",
    "tax": "impuestos", "discount": "descuentos", "total": "totales", "subtotal": "subtotales",
    "balance": "saldos", "debit": "débitos", "credit": "créditos",
    "ledger": "libros contables", "journal": "diarios", "entry": "asientos",
    "payroll": "nóminas", "employee": "empleados", "attendance": "asistencias",
    "leave": "permisos", "timesheet": "partes de horas", "timesheets": "partes de horas",
    "project": "proyectos", "task": "tareas", "tasks": "tareas", "milestone": "hitos",
    "pos": "TPV", "barcode": "códigos de barras", "barcode": "códigos de barras",
    "serial": "números de serie", "batch": "lotes", "lot": "lotes",
    "unit": "unidades", "uom": "unidades de medida", "currency": "monedas",
    "exchange": "tipos de cambio", "rate": "tasas", "price": "precios",
    "cost": "costos", "margin": "márgenes", "profit": "beneficios",
    "voucher": "vouchers", "receipt": "recibos", "note": "notas", "notes": "notas",
    "comment": "comentarios", "comments": "comentarios", "log": "registros", "logs": "registros",
    "history": "historial", "timeline": "línea de tiempo", "activity": "actividades",
    "notification": "notificaciones", "alert": "alertas", "reminder": "recordatorios",
    "dashboard": "panel principal", "widget": "widgets", "chart": "gráficos",
    "graph": "gráficos", "graph": "grafo", "table": "tablas", "grid": "rejillas",
    "column": "columnas", "row": "filas", "cell": "celdas", "header": "cabeceras",
    "footer": "pies de página", "sidebar": "barras laterales", "toolbar": "barras de herramientas",
}
_NOM_MAYUS = {k.upper(): v for k, v in NOMBRES_FUNC.items()}

def tokenizar_nombre(nombre):
    """Divide un identificador en tokens (snake_case, kebab y camelCase)."""
    partes = []
    for p in re.split(r"[_\-.]", nombre or ""):
        partes += re.findall(r"[A-Z]+(?![a-z])|[A-Z][a-z0-9]+|[a-z0-9]+", p)
    return [p for p in partes if p]

def traducir_token(t):
    k = t.lower()
    if k in NOMBRES_FUNC:
        return NOMBRES_FUNC[k]
    if k.upper() in _NOM_MAYUS:
        return _NOM_MAYUS[k.upper()]
    if k in SUSTANTIVOS:
        s = SUSTANTIVOS[k]
        return s.split(" (")[0].lower()
    return t

def describir_auto(f, contexto):
    """Genera una descripción en español a partir del nombre, receptor y cuerpo."""
    nombre = f.get("nombre", "")
    toks = tokenizar_nombre(nombre)
    if not toks:
        return ""
    # casos especiales
    if nombre.lower() == "main":
        return "Punto de entrada del programa"
    if nombre.startswith("Test") and len(nombre) > 4:
        rest = tokenizar_nombre(nombre[4:])
        obj = " ".join(traducir_token(t) for t in rest).strip()
        return ("Pruebas de " + obj).strip() if obj else "Pruebas"
    if nombre.startswith("Benchmark") and len(nombre) > 9:
        rest = tokenizar_nombre(nombre[9:])
        obj = " ".join(traducir_token(t) for t in rest).strip()
        return ("Medición de rendimiento de " + obj).strip() if obj else "Medición de rendimiento"
    cuerpo = f.get("cuerpo", "") or ""
    cu = cuerpo.upper()
    receptor = f.get("receptor") or ""
    # objeto del receptor tipo *InvoiceService
    tipo = ""
    m = re.search(r"\*?([A-Za-z_]\w*)\s*\)", receptor)
    if m:
        tipo = " ".join(traducir_token(t) for t in tokenizar_nombre(m.group(1))).strip()
    verbo = VERBOS_ES.get(toks[0].lower())
    if verbo:
        resto = [traducir_token(t) for t in toks[1:]]
        obj = " ".join(r for r in resto if r)
        # si el resto solo son preposiciones, usar el tipo del receptor
        if not obj or obj.split()[0] in ("por", "de", "con", "para", "en", "los", "las", "el", "la"):
            obj = (tipo + " " + obj).strip() or (contexto.split("(")[0].strip() or "datos").lower()
    else:
        obj = " ".join(traducir_token(t) for t in toks)
    if not obj:
        obj = tipo or (contexto.split("(")[0].strip() or "datos").lower()
    if verbo:
        return (verbo + " " + obj).strip()[:300]
    # sin verbo explícito: patrones del cuerpo
    if "SELECT " in cu or ".QUERY(" in cu or "QUERYROW" in cu:
        return ("Consulta SQL de " + obj)[:300]
    if "INSERT INTO" in cu:
        return ("Inserta registros en " + obj)[:300]
    if "UPDATE " in cu and " SET " in cu:
        return ("Actualiza registros de " + obj)[:300]
    if "DELETE FROM" in cu:
        return ("Elimina registros de " + obj)[:300]
    if "R.URL" in cuerpo or "r.URL" in cuerpo or "R.METHOD" in cu or "r.Method" in cuerpo:
        return ("Maneja peticiones HTTP de " + obj)[:300]
    if "JSON." in cu or "JSON:" in cu:
        return ("Procesa JSON de " + obj)[:300]
    if "TEMPLATE." in cu or "TPL.EXECUTE" in cu:
        return ("Renderiza plantilla de " + obj)[:300]
    if "BCRYPT" in cu or "BCRYPT" in nombre.upper():
        return ("Gestiona hash de contraseñas de " + obj)[:300]
    if "JWT" in cuerpo:
        return ("Gestiona tokens JWT de " + obj)[:300]
    if ".EXEC(" in cu or "EXECUTE(" in cu:
        return ("Ejecuta sentencia SQL sobre " + obj)[:300]
    return ("Opera " + obj)[:300]

def aplicar_desc_autos(funcs, contexto):
    """Rellena funcs sin doc comment con descripción automática; limpia cuerpo temporal."""
    n = 0
    for f in funcs:
        f.pop("cuerpo", None)
        if not f.get("descripcion"):
            d = describir_auto(f, contexto)
            if d:
                f["descripcion"] = d
                f["auto"] = True
                n += 1
    return n

def capturar_cuerpo(lineas, i, n, max_lineas=40):
    """Recoge el cuerpo de una función por balanceo de llaves desde la línea i."""
    prof, visto = 0, False
    partes = []
    k = i
    while k < n and len(partes) < max_lineas:
        l = lineas[k]
        partes.append(l)
        prof += l.count("{") - l.count("}")
        if "{" in l:
            visto = True
        if visto and prof <= 0:
            break
        k += 1
    return "\n".join(partes)[:3000]

def proposito_por_nombre(rel):
    """Describe el archivo: primero por su nombre; si no, por sus directorios."""
    rel = rel.replace("\\", "/")
    base = os.path.splitext(rel)[0].lower()
    nombre = base.rsplit("/", 1)[-1]
    dirs = base.split("/")[:-1]
    tokens = re.split(r"[_\-.]", nombre)
    frases = [SUSTANTIVOS[p] for p in tokens if p in SUSTANTIVOS]
    # "main.go"/"index.js" sueltos describen peor que su directorio inmediato
    genericos = {"main", "index", "app", "utils", "helpers", "core"}
    if not frases or set(tokens) <= genericos:
        padre = dirs[-1] if dirs else ""
        if padre in SUSTANTIVOS:
            frases = [SUSTANTIVOS[padre]]
    if not frases:
        frases = [SUSTANTIVOS[d] for d in dirs if d in SUSTANTIVOS]
    if frases:
        return "Módulo de " + " / ".join(dict.fromkeys(frases))
    return ""

def proposito_por_encabezado(lineas):
    """Primer bloque de comentarios del archivo (antes de la primera línea de código)."""
    bloque, visto_codigo = [], False
    for linea in lineas[:40]:
        s = linea.strip()
        if not s:
            if bloque:
                break
            continue
        if s.startswith(("//", "#", "--", "/*", "*", "@rem", "REM", "::")):
            bloque.append(s)
            if s.endswith("*/") and bloque[0].startswith("/*"):
                break
        elif s.startswith(('"', "'", "@echo", "SET ", "set ", "@set")):
            break  # código batch antes de comentarios útiles
        else:
            visto_codigo = True
            break
    if not bloque:
        return ""
    # descarta líneas decorativas de banners (==== ---- ***)
    bloque = [b for b in bloque if not re.fullmatch(r"[=\-*#~_/\s]*|REM\s*[=\-*#~_/\s]+|//\s*[=\-*#~_/\s]+|#\s*[=\-*#~_/\s]+|--\s*[=\-*#~_/\s]+", b)]
    if not bloque:
        return ""
    texto = combinar_comentario(bloque)
    texto = re.sub(r"\s+", " ", texto).strip()
    if len(texto) < 15:
        return ""
    return texto[:300]

def leer_lineas(ruta):
    try:
        with open(ruta, "r", encoding="utf-8", errors="replace") as f:
            return f.read().splitlines()
    except Exception:
        return []

def combinar_comentario(bloque):
    out = []
    for c in bloque:
        c = re.sub(r"^//+\s?", "", c)
        c = re.sub(r"^#\s?", "", c)
        c = re.sub(r"^/\*+|\*+/|\*", "", c).strip()
        if c:
            out.append(c)
    return " ".join(out).strip()

def es_minificado(ruta, lineas):
    if ruta.endswith(".min.js"):
        return True
    if any(".min.js" in p for p in ruta.split(os.sep)):
        return True
    if len(lineas) == 1 and len(lineas[0]) > 5000:
        return True
    return False

def debe_omitirela(rel):
    rel_n = rel.replace("\\", "/")
    partes = rel_n.split("/")
    for i in range(1, len(partes)):
        sub = "/".join(partes[:i])
        if sub in OMITIR_DIRS_NORM:
            return True
    if rel_n.startswith("docs/Diccionario"):
        return True
    base = os.path.basename(rel_n)
    if base.endswith(".pb.go") or base.endswith("_pb2.py"):
        return True
    if base.startswith("tmp_") or base.endswith(".log") or base.endswith(".stackdump"):
        return True
    if base in ("nul", "la", "ofusca"):
        return True
    return False

# ---------------------------------------------------------------- extractores GO
RE_GO_FUNC = re.compile(r"^func\s+(\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(")
RE_GO_TYPE = re.compile(r"^type\s+([A-Za-z_]\w*)\s+")
RE_GO_PKG = re.compile(r"^package\s+(\w+)")

def extraer_go(ruta, lineas):
    funcs, types, pkg = [], [], ""
    i = 0
    n = len(lineas)
    while i < n:
        linea = lineas[i]
        if not pkg:
            m = RE_GO_PKG.match(linea)
            if m:
                pkg = m.group(1)
        # comentarios previos
        j = i
        bloque = []
        while j > 0 and lineas[j-1].strip().startswith("//"):
            bloque.insert(0, lineas[j-1].strip())
            j -= 1
        m = RE_GO_FUNC.match(linea)
        if m:
            receptor = (m.group(1) or "").strip()
            nombre = m.group(2)
            desc = combinar_comentario(bloque)
            # firma hasta '{' o fin
            sig = linea.rstrip()
            k = i
            while "{" not in sig and k + 1 < n and k - i < 3:
                k += 1
                sig += " " + lineas[k].strip()
            sig = sig.split("{")[0].strip()
            funcs.append({
                "nombre": nombre,
                "receptor": receptor,
                "firma": re.sub(r"\s+", " ", sig),
                "descripcion": desc,
                "linea": i + 1,
                "exportada": nombre[:1].isupper(),
                "cuerpo": capturar_cuerpo(lineas, i, n),
            })
        else:
            m2 = RE_GO_TYPE.match(linea)
            if m2 and not bloque:
                j2 = i
                bloque2 = []
                while j2 > 0 and lineas[j2-1].strip().startswith("//"):
                    bloque2.insert(0, lineas[j2-1].strip())
                    j2 -= 1
                desc = combinar_comentario(bloque2)
                tipo = linea.split(None, 2)[2] if len(linea.split(None, 2)) > 2 else ""
                types.append({"nombre": m2.group(1), "tipo": tipo.strip().rstrip("{").strip(),
                              "descripcion": desc, "linea": i + 1})
        i += 1
    return pkg, funcs, types

# ---------------------------------------------------------------- extractores JS
RE_JS_FUNC = re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)\s*\(")
RE_JS_ARROW = re.compile(r"^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:function\b|\([^)]*\)\s*=>|[A-Za-z_$][\w$]*\s*=>)")
RE_JS_PROTO = re.compile(r"^\s*([A-Za-z_$][\w$]*)\s*:\s*(?:async\s+)?function\b")
RE_JS_CLASS = re.compile(r"^\s*(?:export\s+)?(?:class)\s+([A-Za-z_$][\w$]*)")

def extraer_js(ruta, lineas):
    funcs, classes = [], []
    n = len(lineas)
    for i, linea in enumerate(lineas):
        bloque = []
        j = i
        while j > 0 and (lineas[j-1].strip().startswith("//") or lineas[j-1].strip().startswith("*") or lineas[j-1].strip().startswith("/*")):
            bloque.insert(0, lineas[j-1].strip())
            if lineas[j-1].strip().startswith("/*"):
                break
            j -= 1
        desc = combinar_comentario(bloque)
        m = RE_JS_FUNC.match(linea)
        if m:
            funcs.append({"nombre": m.group(1), "firma": linea.strip()[:200], "descripcion": desc,
                          "linea": i + 1, "tipo": "function", "cuerpo": capturar_cuerpo(lineas, i, n, 30)})
            continue
        m = RE_JS_ARROW.match(linea)
        if m:
            funcs.append({"nombre": m.group(1), "firma": linea.strip()[:200], "descripcion": desc,
                          "linea": i + 1, "tipo": "arrow", "cuerpo": capturar_cuerpo(lineas, i, n, 30)})
            continue
        m = RE_JS_PROTO.match(linea)
        if m and len(m.group(1)) > 1:
            funcs.append({"nombre": m.group(1), "firma": linea.strip()[:200], "descripcion": desc,
                          "linea": i + 1, "tipo": "método", "cuerpo": capturar_cuerpo(lineas, i, n, 30)})
            continue
        m = RE_JS_CLASS.match(linea)
        if m:
            classes.append({"nombre": m.group(1), "descripcion": desc, "linea": i + 1})
    return funcs, classes

# ---------------------------------------------------------------- extractores Python
RE_PY_DEF = re.compile(r"^(\s*)(?:async\s+)?def\s+([A-Za-z_]\w*)\s*\(")
RE_PY_CLASS = re.compile(r"^class\s+([A-Za-z_]\w*)")

def extraer_py(ruta, lineas):
    funcs, classes = [], []
    n = len(lineas)
    for i, linea in enumerate(lineas):
        m = RE_PY_DEF.match(linea)
        if m:
            indent, nombre = m.group(1), m.group(2)
            desc = ""
            # docstring
            k = i + 1
            while k < n and not lineas[k].strip():
                k += 1
            if k < n and lineas[k].strip().startswith(('"""', "'''")):
                q = lineas[k].strip()[:3]
                texto = lineas[k].strip()[3:]
                if not texto.endswith(q) or len(texto) < 3:
                    k += 1
                    while k < n and q not in lineas[k]:
                        texto += " " + lineas[k].strip()
                        k += 1
                    if k < n:
                        texto += " " + lineas[k].strip().split(q)[0]
                desc = texto.strip()
            if not desc:
                kk = i - 1
                bloque = []
                while kk >= 0 and lineas[kk].strip().startswith("#"):
                    bloque.insert(0, lineas[kk].strip())
                    kk -= 1
                desc = combinar_comentario(bloque)
            cuerpo = []
            base = len(indent)
            for kk in range(i + 1, min(i + 30, n)):
                l = lineas[kk]
                if l.strip() and (len(l) - len(l.lstrip())) <= base:
                    break
                cuerpo.append(l)
            funcs.append({"nombre": nombre, "firma": linea.strip()[:200], "descripcion": desc[:500],
                          "linea": i + 1, "nivel": "método" if indent else "función",
                          "cuerpo": "\n".join(cuerpo)[:3000]})
            continue
        m = RE_PY_CLASS.match(linea)
        if m:
            desc = ""
            k = i + 1
            while k < n and not lineas[k].strip():
                k += 1
            if k < n and lineas[k].strip().startswith(('"""', "'''")):
                q = lineas[k].strip()[:3]
                desc = lineas[k].strip()[3:]
                if desc.endswith(q):
                    desc = desc[:-3]
                else:
                    k += 1
                    partes = [desc]
                    while k < n and q not in lineas[k]:
                        partes.append(lineas[k].strip())
                        k += 1
                    if k < n:
                        partes.append(lineas[k].strip().split(q)[0])
                    desc = " ".join(p for p in partes if p)
            classes.append({"nombre": m.group(1), "descripcion": desc.strip()[:500], "linea": i + 1})
    return funcs, classes

# ---------------------------------------------------------------- extractores SQL
RE_SQL_OBJ = re.compile(
    r"CREATE\s+(OR\s+REPLACE\s+)?(TABLE|VIEW|PROCEDURE|FUNCTION|TRIGGER|EVENT|INDEX)\s+`?([\w.]+)`?",
    re.IGNORECASE)

def extraer_sql(ruta, lineas):
    objs, comments = [], []
    buf = " ".join(lineas)
    for m in RE_SQL_OBJ.finditer(buf):
        objs.append({"tipo": m.group(2).upper(), "nombre": m.group(3)})
    # comentarios que empiezan el archivo
    for linea in lineas[:30]:
        s = linea.strip()
        if s.startswith("--"):
            comments.append(s.lstrip("- ").strip())
        elif s.startswith("/*"):
            comments.append(s.strip("/* ").strip())
        elif s and not s.startswith(("SET", "CREATE", "USE", "ALTER", "INSERT", "--")):
            break
    return objs, " ".join(c for c in comments if len(c) > 10)[:400]

# ---------------------------------------------------------------- extractores BAT/SH
def es_linea_decorativa(s):
    return bool(re.fullmatch(r"(?:REM\s*|//\s*|::\s*|#\s*|--\s*)?[=\-*#~_/\s]+", s))

def extraer_script(ruta, lineas):
    """Primer bloque de comentarios del inicio del script (sin banners) + comandos."""
    props, cmds, visto_codigo = [], [], False
    for i, linea in enumerate(lineas[:60]):
        s = linea.strip()
        if not s:
            if props:
                break
            continue
        if s.startswith(("REM ", "REM\t", "rem ", "::", "# ", "-- ")):
            if not es_linea_decorativa(s):
                props.append(s.lstrip(":#").strip().removeprefix("REM ").strip())
            if len(props) >= 6:
                break
            continue
        if s.startswith(("@echo", "echo", "@", "SET ", "set ", "cd ", "chdir", "exit")):
            break
        if re.match(r'^"?[A-Za-z]', s):
            cmds.append(re.sub(r"\s+", " ", s)[:150])
            visto_codigo = True
            if len(cmds) >= 25:
                break
    return " ".join(props)[:400], cmds[:25]

# ---------------------------------------------------------------- escaneo general
def escanear():
    archivos = {}
    raiz_arbol = {"nombre": "ERPGo", "ruta": "", "tipo": "dir", "hijos": []}
    index_dirs = {"": raiz_arbol}

    def asegurar_dir(rel_dir):
        if rel_dir in index_dirs:
            return index_dirs[rel_dir]
        padre_rel, nombre = os.path.split(rel_dir)
        padre = asegurar_dir(padre_rel.replace("\\", "/"))
        nodo = {"nombre": nombre, "ruta": rel_dir.replace("\\", "/"), "tipo": "dir", "hijos": []}
        padre["hijos"].append(nodo)
        index_dirs[nodo["ruta"]] = nodo
        return nodo

    total = 0
    for dirpath, dirnames, filenames in os.walk(RAIZ):
        rel_dir = os.path.relpath(dirpath, RAIZ)
        if rel_dir == ".":
            rel_dir = ""
        # filtrar dirs
        dirnames[:] = [d for d in sorted(dirnames)
                       if not debe_omitirela(os.path.join(rel_dir, d) if rel_dir else d)
                       and d not in (".git", "node_modules")]
        for fn in sorted(filenames):
            ext = os.path.splitext(fn)[1].lower()
            if ext not in EXT_LENGUAJE:
                continue
            rel = os.path.join(rel_dir, fn) if rel_dir else fn
            rel = rel.replace("\\", "/")
            if debe_omitirela(rel):
                continue
            ruta = os.path.join(RAIZ, rel)
            lineas = leer_lineas(ruta)
            if es_minificado(ruta, lineas):
                continue
            lenguaje = EXT_LENGUAJE[ext]
            info = {"ruta": rel, "nombre": fn, "lenguaje": lenguaje,
                    "lineas": len(lineas), "proposito": "", "funcs": [], "types": [],
                    "classes": [], "objs_sql": [], "comando_extra": []}

            if lenguaje == "Go":
                pkg, funcs, types = extraer_go(ruta, lineas)
                info["paquete"] = pkg
                info["funcs"] = funcs
                info["types"] = types
            elif lenguaje == "JavaScript":
                funcs, classes = extraer_js(ruta, lineas)
                info["funcs"] = funcs
                info["classes"] = classes
            elif lenguaje == "Python":
                funcs, classes = extraer_py(ruta, lineas)
                info["funcs"] = funcs
                info["classes"] = classes
            elif lenguaje == "SQL":
                objs, desc = extraer_sql(ruta, lineas)
                info["objs_sql"] = objs
                info["proposito"] = desc
            else:
                props, cmds = extraer_script(ruta, lineas)
                info["proposito"] = props
                info["comando_extra"] = cmds

            if not info["proposito"]:
                info["proposito"] = proposito_por_encabezado(lineas)
            if not info["proposito"]:
                info["proposito"] = proposito_por_nombre(rel)
            if not info["proposito"] and fn.endswith(("_test.go", "_test.py", ".test.js")):
                base = os.path.splitext(fn)[0].replace("_test", "")
                desc = proposito_por_nombre(base)
                info["proposito"] = ("Pruebas de " + desc[7:]) if desc.startswith("Módulo de ") else "Pruebas automatizadas"
            if not info["proposito"]:
                info["proposito"] = "Archivo de " + lenguaje
            if info["funcs"] and lenguaje in ("Go", "JavaScript", "Python"):
                aplicar_desc_autos(info["funcs"], info["proposito"])
            # mejor descripción del propósito si no hay comentarios en funcs
            archivos[rel] = info
            padre = os.path.dirname(rel)
            nodo_dir = asegurar_dir(padre) if padre else raiz_arbol
            nodo_dir["hijos"].append({
                "nombre": fn, "ruta": rel, "tipo": "file",
                "lenguaje": lenguaje, "nfuncs": len(info["funcs"]),
            })
            total += 1

    def ordenar(n):
        if n.get("hijos"):
            n["hijos"].sort(key=lambda x: (0 if x["tipo"] == "dir" else 1, x["nombre"].lower()))
            for h in n["hijos"]:
                ordenar(h)
    ordenar(raiz_arbol)
    return archivos, raiz_arbol, total

# ---------------------------------------------------------------- main
def main():
    archivos, arbol, total = escanear()
    fecha = datetime.datetime.now().strftime("%Y-%m-%d %H:%M")
    n_funcs = sum(len(a["funcs"]) for a in archivos.values())
    datos = {
        "generado": fecha,
        "totalArchivos": total,
        "totalFunciones": n_funcs,
        "arbol": arbol,
        "archivos": archivos,
    }
    js = "// Generado por generar_diccionario.py — no editar a mano\nconst DATOS = " + \
         json.dumps(datos, ensure_ascii=False, separators=(",", ":")) + ";\n"
    with open(os.path.join(SALIDA, "datos.js"), "w", encoding="utf-8") as f:
        f.write(js)
    print("Archivos: %d | Funciones: %d | datos.js: %.1f MB" %
          (total, n_funcs, os.path.getsize(os.path.join(SALIDA, "datos.js")) / 1048576))
    return 0

if __name__ == "__main__":
    sys.exit(main())

