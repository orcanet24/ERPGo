package web

// guia_demo_tenant.go — la empresa DEMO de práctica, SIEMPRE disponible.
//
// Decisión de diseño (decisión del dueño): la demo no se activa por admin.
// Se auto-provisiona en cada instalación y queda ahí para cualquier usuario
// con sesión: refrescar conocimiento, validar una acción antes de hacerla en
// la empresa real, o entrenar al personal sin gastar horas de técnico.
//
// Comportamiento:
//   - Al arrancar el ERP, si no existe la empresa `demo`, se provisiona
//     (base `erpgo_demo` + esquema completo + registro en el catálogo con
//     is_demo=1). Un fallo aquí NO impide arrancar: solo deja GOti silencioso.
//   - Cada usuario autenticado puede ENTRAR a la demo aunque no sea miembro;
//     en el selector de empresas aparece como "Empresa DEMO (práctica)".
//   - El admin puede desactivarla con el comando que ya existe
//     (POST /guia/demo-marcar code=demo demo=0): deja de aparecer en el
//     selector y GOti deja de responder; reactivarla es demo=1.
//   - La demo es una base aparte (erpgo_demo): cero riesgo para datos reales.

import (
	"database/sql"
	"fmt"
	"log"

	"erpgow/infrastructure/persistence"
)

// codNombreDemo y etiquetaDemo: constantes públicas del identificador.
const (
	tenantDemoCode  = "demo"
	tenantDemoLabel = "Empresa DEMO (práctica)"
)

// ensureDemoTenant garantiza que la empresa de práctica exista. Idempotente
// en el arranque y barato cuando ya existe (una consulta al catálogo).
func (s *Server) ensureDemoTenant() {
	ctl, err := s.controlStore()
	if err != nil {
		// Sin catálogo no hay multi-empresa: la demo no tiene dónde vivir.
		return
	}
	s.ensureDemoColumn()

	var existe int
	if err := ctl.QueryRow(`SELECT COUNT(*) FROM tenants WHERE code = ?`, tenantDemoCode).Scan(&existe); err != nil {
		log.Printf("demo: no se pudo leer el catálogo: %v", err)
		return
	}
	if existe == 0 {
		if err := s.crearEmpresaDemo(ctl); err != nil {
			// Igual que el orquestador: preferible operar avisando que
			// negarse a levantar el ERP por falta de la demo.
			log.Printf("demo: no se pudo provisionar la empresa de práctica: %v", err)
			return
		}
		log.Printf("demo: empresa de práctica provisionada (%s) — visible para todos los usuarios", tenantDemoCode)
	} else {
		// Ya existe: asegurar el flag (una instalación vieja puede tenerla sin is_demo).
		_, _ = ctl.Exec(`UPDATE tenants SET is_demo = 1 WHERE code = ? AND status = 1`, tenantDemoCode)
	}
}

// crearEmpresaDemo provisiona la base de práctica con el mismo mecanismo de
// ProvisionTenant pero SIN crear usuarios: la demo no tiene administrador
// propio, el acceso es por la excepción is_demo de CanAccessTenant.
func (s *Server) crearEmpresaDemo(ctl *sql.DB) error {
	// 1) Crear la base MySQL vacía (idempotente).
	nombre := s.tenantDBName(tenantDemoCode)
	if err := persistence.EnsureDatabase(persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.DBUser,
		Clave:   s.config.DBPassword,
		Nombre:  nombre,
	}); err != nil {
		return fmt.Errorf("creando la base %s: %w", nombre, err)
	}

	// 2) Aplicar el esquema completo + siembras idempotentes (mismo camino que
	// cualquier empresa: migraciones embebidas). Si ya estaba media-instalada,
	// el registro de migraciones evita repetir trabajo.
	db, err := s.openTenantDB(tenantDemoCode)
	if err != nil {
		return fmt.Errorf("abriendo la base de la demo: %w", err)
	}
	if err := persistence.RunMigrationsForMySQL(db, persistence.ConfigMySQL{
		Host:    s.config.DBHost,
		Port:    s.config.DBPort,
		Usuario: s.config.DBUser,
		Clave:   s.config.DBPassword,
		Nombre:  nombre,
	}); err != nil {
		return fmt.Errorf("migraciones de la demo: %w", err)
	}

	// 3) Registrar en el catálogo con is_demo=1 y SIN usuarios.
	if _, err := ctl.Exec(`INSERT INTO tenants (code, legal_name, rif, status, is_demo, created_at)
		VALUES (?, ?, 'D-00000000-0', 1, 1, ?)
		ON DUPLICATE KEY UPDATE is_demo = 1, status = 1`,
		tenantDemoCode, tenantDemoLabel, now()); err != nil {
		return fmt.Errorf("registrando la demo en el catálogo: %w", err)
	}
	return nil
}
