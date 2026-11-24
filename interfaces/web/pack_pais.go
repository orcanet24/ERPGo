package web

// pack_pais.go — la ficha de cada país, leída del árbol packs/.
//
// POR QUÉ EXISTE. El registro de países (pais.go) dice qué se ofrece hoy; la
// ficha dice POR QUÉ y EN QUÉ ESTADO. Separarlas es lo que permite que un
// colaborador del país aporte sus datos sin tocar el core: el árbol packs/ es
// suyo, el registro sigue siendo la puerta que decide qué se ofrece.
//
// EL VALIDADOR ES LA LEY DEL PROYECTO HECHA CÓDIGO: un pack en «borrador»
// tiene que decir qué le falta; uno «validado» tiene que decir quién lo firmó;
// uno «certificado» tiene que decir en qué ambiente se probó. Sin estas tres
// frases el estado sería una etiqueta de marketing, y el que pierde es el
// contador que firma delante de un cliente.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"erpgo"
)

// packPais es la ficha declarativa de un país (packs/<cc>/pack.json).
type packPais struct {
	Codigo             string     `json:"code"`
	Nombre             string     `json:"name"`
	Moneda             string     `json:"currency"`
	Estado             estadoPais `json:"estado"`
	LegacyEmbebido     bool       `json:"legacy_embebido"`
	ValidadoPor        string     `json:"validado_por"`
	CertificadoPor     string     `json:"certificado_por"`
	AmbientePruebas    string     `json:"ambiente_pruebas"`
	Pendientes         []string   `json:"pendientes"`
	Impuestos          []string   `json:"impuestos"`
	PlanDeCuentas      string     `json:"plan_de_cuentas"`
	FacturaElectronica string     `json:"factura_electronica"`
	Notas              string     `json:"notas"`
}

// cargarPacksPais lee todas las fichas de packs/<cc>/pack.json. Las carpetas que
// empiezan con "_" son plantillas/documentación y no son packs.
func cargarPacksPais() (map[string]packPais, error) {
	out := map[string]packPais{}
	raiz := erpgo.Packs()
	entradas, err := fs.ReadDir(raiz, "packs")
	if err != nil {
		return nil, fmt.Errorf("leyendo packs: %w", err)
	}
	for _, e := range entradas {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		nombre := e.Name()
		data, err := fs.ReadFile(raiz, "packs/"+nombre+"/pack.json")
		if err != nil {
			return nil, fmt.Errorf("packs/%s/pack.json: %w", nombre, err)
		}
		var p packPais
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("packs/%s/pack.json no es JSON válido: %w", nombre, err)
		}
		if p.Codigo != strings.ToUpper(nombre) {
			return nil, fmt.Errorf("packs/%s/pack.json declara code=%q: la carpeta y el código deben coincidir", nombre, p.Codigo)
		}
		out[p.Codigo] = p
	}
	return out, nil
}

// validarPackPais comprueba que el estado declarado esté sustentado. Devuelve el
// motivo exacto: el colaborador que abre el error tiene que saber qué arreglar.
func validarPackPais(p packPais) error {
	if normalizarPaisCodigo(p.Codigo) != p.Codigo || len(p.Codigo) != 2 {
		return fmt.Errorf("%s: el código debe ser ISO de dos letras en mayúsculas", p.Codigo)
	}
	if strings.TrimSpace(p.Nombre) == "" {
		return fmt.Errorf("%s: falta el nombre del país", p.Codigo)
	}
	if len(p.Moneda) != 3 || p.Moneda != strings.ToUpper(p.Moneda) {
		return fmt.Errorf("%s: la moneda debe ser ISO de tres letras (%q)", p.Codigo, p.Moneda)
	}
	switch p.Estado {
	case paisBorrador:
		if len(p.Pendientes) == 0 {
			return fmt.Errorf("%s: un pack en borrador tiene que declarar qué le falta (pendientes vacío)", p.Codigo)
		}
	case paisValidado:
		if strings.TrimSpace(p.ValidadoPor) == "" {
			return fmt.Errorf("%s: un pack validado tiene que declarar quién lo revisó (validado_por)", p.Codigo)
		}
	case paisCertificado:
		if strings.TrimSpace(p.CertificadoPor) == "" || strings.TrimSpace(p.AmbientePruebas) == "" {
			return fmt.Errorf("%s: un pack certificado necesita certificado_por y ambiente_pruebas", p.Codigo)
		}
	default:
		return fmt.Errorf("%s: estado inválido %q (borrador|validado|certificado)", p.Codigo, p.Estado)
	}
	return nil
}

// packsPaisOrdenados devuelve las fichas ordenadas por código (orden estable
// para pruebas, reportes y cualquier lista que se muestre).
func packsPaisOrdenados() ([]packPais, error) {
	todos, err := cargarPacksPais()
	if err != nil {
		return nil, err
	}
	salida := make([]packPais, 0, len(todos))
	for _, p := range todos {
		salida = append(salida, p)
	}
	sort.Slice(salida, func(i, j int) bool { return salida[i].Codigo < salida[j].Codigo })
	return salida, nil
}
