package web

// Pruebas del árbol de cuentas (COA).
//
// El árbol de /account/coa se dibuja con jsTree. El 2 de noviembre de 2026 la
// página se quedó en blanco y la consola mostraba, una y otra vez:
//
//	Uncaught TypeError: Cannot read properties of undefined (reading 'core')
//	    at self.onmessage (blob:...)
//
// El error salía DENTRO del Web Worker que jsTree abre para parsear los datos.
// jsTree elige entre dos parseadores mirando solo el primer nodo del array: si
// tiene `parent`, trata todo el envío como lista plana y exige que cada padre
// exista entre los nodos. Las cuentas raíz tienen PHeadCode 0 o NULL, no existe
// ningún nodo con ese id, y jsTree intenta registrar el error con
// `this._data.core.last_error` —pero en el worker `this` es el propio worker, que
// no tiene `_data`, y muere con ese TypeError antes de devolver nada. El árbol
// se queda sin dibujar y sin explicación.
//
// Estas pruebas fijan la forma del JSON que el handler entrega: un árbol anidado
// por `children`, sin `parent`, anidado por el PHeadCode real. Cualquier cambio
// que vuelva a incluir `parent` o que anide por el recorte del HeadCode en vez
// de por PHeadCode debe romperlas.

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func nuevaBaseCOA(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("no se pudo abrir la base en memoria: %v", err)
	}
	// Una sola conexión: con varias, cada una tendría su propia base :memory: y
	// la prueba pasaría o fallaría según el azar del pool.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE acc_coa (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		HeadCode INTEGER NOT NULL,
		HeadName TEXT NOT NULL DEFAULT '',
		PHeadName TEXT NOT NULL DEFAULT '',
		PHeadCode TEXT,
		HeadLevel INTEGER NOT NULL DEFAULT 1,
		HeadType TEXT NOT NULL DEFAULT '',
		IsActive INTEGER NOT NULL DEFAULT 1,
		IsTransaction INTEGER NOT NULL DEFAULT 0,
		IsGL INTEGER NOT NULL DEFAULT 0,
		isCashNature INTEGER NOT NULL DEFAULT 0,
		isBankNature INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("no se pudo crear acc_coa: %v", err)
	}
	return db
}

func insertarCOA(t *testing.T, db *sql.DB, headCode int64, nombre, pHeadCode any) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO acc_coa (HeadCode, HeadName, PHeadCode) VALUES (?, ?, ?)`,
		headCode, nombre, pHeadCode); err != nil {
		t.Fatalf("no se pudo insertar la cuenta %d: %v", headCode, err)
	}
}

// arbolCOAData llama al handler y devuelve el árbol indexado por id, más la
// lista de ids de primer nivel. Falla la prueba si algún nodo lleva `parent`
// (que es lo que confunde a jsTree) o si un nodo aparece dos veces.
func arbolCOAData(t *testing.T, db *sql.DB) (map[string]map[string]interface{}, []string) {
	t.Helper()
	s := &Server{db: db}
	rec := httptest.NewRecorder()
	s.coaData(rec, httptest.NewRequest("GET", "/account/coa/data", nil))
	if rec.Code != 200 {
		t.Fatalf("el handler respondió %d: %s", rec.Code, rec.Body.String())
	}

	var roots []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &roots); err != nil {
		t.Fatalf("el JSON no es un array de nodos: %v (body: %s)", err, rec.Body.String())
	}
	byID := map[string]map[string]interface{}{}
	rootIDs := []string{}
	var walk func(nodes []map[string]interface{})
	walk = func(nodes []map[string]interface{}) {
		for _, n := range nodes {
			id, _ := n["id"].(string)
			if id == "" {
				t.Fatalf("nodo sin id: %+v", n)
			}
			if _, dup := byID[id]; dup {
				t.Fatalf("el nodo %s aparece dos veces en el árbol", id)
			}
			byID[id] = n
			if _, tieneParent := n["parent"]; tieneParent {
				t.Errorf("el nodo %s lleva `parent`: jsTree lo interpreta como lista plana y revienta en su worker", id)
			}
			if ch, ok := n["children"].([]interface{}); ok {
				kids := make([]map[string]interface{}, 0, len(ch))
				for _, c := range ch {
					kids = append(kids, c.(map[string]interface{}))
				}
				walk(kids)
			}
		}
	}
	for _, n := range roots {
		rootIDs = append(rootIDs, n["id"].(string))
	}
	walk(roots)
	return byID, rootIDs
}

func hijosCOA(t *testing.T, n map[string]interface{}) []string {
	t.Helper()
	raw, ok := n["children"].([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(raw))
	for _, c := range raw {
		ids = append(ids, c.(map[string]interface{})["id"].(string))
	}
	return ids
}

// TestCOADataAnidaPorPHeadCodeYSinParent reproduce el caso real que dejó la
// página en blanco: cuentas raíz con PHeadCode 0 y NULL, un nodo cuyo
// PHeadCode no coincide con el recorte del HeadCode (40301 cuelga de 402), un
// PHeadCode que no existe y un auto-padre.
func TestCOADataAnidaPorPHeadCodeYSinParent(t *testing.T) {
	db := nuevaBaseCOA(t)
	// Raíces: PHeadCode "0" y NULL.
	insertarCOA(t, db, 1, "Assets", "0")
	insertarCOA(t, db, 2, "Shareholder's Equity", nil)
	insertarCOA(t, db, 4, "Expenses", "0")
	insertarCOA(t, db, 2111000001, "5-Proveedor", nil)
	// Jerarquía normal.
	insertarCOA(t, db, 101, "Fixed Assets", "1")
	insertarCOA(t, db, 10101, "Property & Equipment", "101")
	// El PHeadCode real manda sobre el recorte del HeadCode: 40301 es
	// "Purchase Account" con PHeadCode 402, no 403 ni 4.
	insertarCOA(t, db, 402, "Over Head Cost", "4")
	insertarCOA(t, db, 40301, "Purchase Account", "402")
	// PHeadCode que no existe: se prueba el recorte histórico.
	insertarCOA(t, db, 201, "Equity", "2")
	insertarCOA(t, db, 20101, "Equity Capital", "999999")
	// Auto-padre: no debe colgarse de sí mismo.
	insertarCOA(t, db, 501, "Long Term Liabilities", "5")
	insertarCOA(t, db, 50101, "Long-Term Debt", "50101")

	byID, rootIDs := arbolCOAData(t, db)

	// Todas las cuentas aparecen, una sola vez: ningún huérfano se pierde.
	if len(byID) != 12 {
		t.Errorf("se esperaban 12 cuentas en el árbol, llegaron %d", len(byID))
	}
	// Solo son raíz las que no tienen padre válido, en orden de HeadCode.
	if want := []string{"1", "2", "4", "501", "2111000001"}; !reflect.DeepEqual(rootIDs, want) {
		t.Errorf("raíces = %v, se esperaban %v", rootIDs, want)
	}
	// El JSON va anidado por PHeadCode, no por el recorte del HeadCode.
	casos := []struct {
		padre string
		hijos []string
	}{
		{"1", []string{"101"}},
		{"101", []string{"10101"}},
		{"4", []string{"402"}},
		{"402", []string{"40301"}}, // aquí se delata el recorte: 40301 no cuelga de 4
		{"2", []string{"201"}},
		{"201", []string{"20101"}}, // PHeadCode inexistente: regresa al recorte
		{"501", []string{"50101"}}, // auto-padre evitado: cuelga de 501
	}
	for _, c := range casos {
		n, ok := byID[c.padre]
		if !ok {
			t.Errorf("la cuenta padre %s no está en el árbol", c.padre)
			continue
		}
		if got := hijosCOA(t, n); !reflect.DeepEqual(got, c.hijos) {
			t.Errorf("hijos de %s = %v, se esperaban %v", c.padre, got, c.hijos)
		}
	}
}
