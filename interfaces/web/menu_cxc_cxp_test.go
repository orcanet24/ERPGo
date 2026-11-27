package web

// Pruebas de que Cuentas por Cobrar y Cuentas por Pagar son SECCIONES del menú
// y no el último renglón suelto de otro módulo.
//
// POR QUÉ SE MIRAN LAS MIGRACIONES Y NO UNA LISTA. El menú lateral vive en la
// tabla `menu`, y quien la llena son los archivos de migrations/*.sql al
// arrancar. Una fila que queda colgada del grupo equivocado —o que pide un
// permiso que nadie tiene sembrado— no da ningún error: la sección simplemente
// aparece escondida dentro de otro grupo, o no aparece, y desde el código todo
// parece correcto.
//
// LA REGLA CAMBIÓ CON LOS DEPARTAMENTOS. Antes estas dos pantallas tenían que
// estar en el PRIMER nivel. Ahora su sitio es el departamento de su función
// («Clientes y cobros» y «Compras y proveedores»): colgando de él se llega en dos
// clics y se leen junto a lo que les da sentido. Lo que la prueba sigue exigiendo
// es lo que importa: que se vean, que pidan un permiso real, que ese permiso las
// esconda a quien no lo tiene, y que no se conviertan en un grupo vacío.
//
// Se aplican las sentencias REALES de las migraciones (ver menu_estructura_test.go
// para el porqué de la simulación en vez de leer SQLite, donde la siembra grande
// del menú no llega a la tabla).

import "testing"

func TestCxCYCxPSonSeccionesDelMenu(t *testing.T) {
	filas := menuSimulado(t)
	sembrados := subModuleSembrados(t)

	porID := map[int]*filaMenu{}
	porURL := map[string]*filaMenu{}
	for id, f := range filas {
		porID[id] = f
		if f.url != "" && f.status == 1 {
			porURL[f.url] = f
		}
	}

	casos := []struct {
		url, label, perm, departamento string
	}{
		{"/cartera/list", "Cuentas por Cobrar", "customer_ledger", "Clientes y cobros"},
		{"/pagar/list", "Cuentas por Pagar", "supplier_payment", "Compras y proveedores"},
	}

	for _, c := range casos {
		fila, existe := porURL[c.url]
		if !existe {
			t.Errorf("ninguna migración deja visible la entrada de menú %s: el módulo existe pero no hay forma de llegar a él desde la barra lateral", c.url)
			continue
		}
		if fila.label != c.label {
			t.Errorf("la entrada %s se llama %q y se esperaba %q", c.url, fila.label, c.label)
		}
		if fila.perm != c.perm {
			t.Errorf("la entrada %s pide el permiso %q y se esperaba %q: es el que el módulo usa en Roles y Permisos",
				c.url, fila.perm, c.perm)
		}

		// Su sitio es el departamento de su función.
		if fila.parentID == nil {
			t.Errorf("%s quedó suelta en el primer nivel: su sitio es el departamento %q", c.url, c.departamento)
		} else if padre, ok := porID[*fila.parentID]; !ok {
			t.Errorf("%s cuelga del menú %d, que no existe: no se pintaría", c.url, *fila.parentID)
		} else {
			if padre.status != 1 {
				t.Errorf("%s cuelga de %q, que está apagado: el enlace no se pinta", c.url, padre.label)
			}
			if padre.label != c.departamento {
				t.Errorf("%s cuelga de %q y su departamento es %q", c.url, padre.label, c.departamento)
			}
		}

		// Un nodo con hijos se pinta como grupo y deja de ser un enlace.
		for _, otra := range filas {
			if otra.parentID != nil && *otra.parentID == fila.id {
				t.Errorf("%s quedó como grupo porque %s cuelga de ella: su URL dejaría de ser un enlace",
					fila.label, otra.label)
			}
		}

		// El permiso tiene que existir para que alguien pueda tenerlo.
		if !sembrados[c.perm] {
			t.Errorf("el permiso %q no lo siembra ninguna migración en sub_module: el filtro del menú lo pediría y nadie podría tenerlo, ni el administrador de la empresa", c.perm)
		}

		// Con el permiso se ve; sin él, no. Es la promesa del menú: no ofrecer
		// una pantalla que el middleware responde 403.
		nodo := &menuNode{id: fila.id, label: fila.label, url: fila.url, perm: fila.perm}
		item, visible := buildMenuItem(nodo, map[string]bool{c.perm: true}, c.url)
		if !visible {
			t.Errorf("con el permiso %q concedido el filtro sigue escondiendo %q", c.perm, c.label)
		} else {
			if item.URL != c.url {
				t.Errorf("%q se pinta con la URL %q y se esperaba %q", c.label, item.URL, c.url)
			}
			if len(item.Children) != 0 {
				t.Errorf("%q se pinta como grupo con %d hijos", c.label, len(item.Children))
			}
			if !item.Active {
				t.Errorf("%q no se marca como activa estando en %s", c.label, c.url)
			}
		}
		if _, visible := buildMenuItem(nodo, map[string]bool{}, c.url); visible {
			t.Errorf("sin el permiso %q el menú muestra %q: prometería una pantalla que el middleware responde 403", c.perm, c.label)
		}
	}
}
