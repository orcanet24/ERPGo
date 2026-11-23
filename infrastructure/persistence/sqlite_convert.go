package persistence

import (
	"regexp"
	"strings"
)

// The migrations/*.sql files are MySQL dumps. SQLite mode translates them on
// the fly: PKs declared by later "ALTER TABLE ... ADD PRIMARY KEY" statements
// are folded into the CREATE TABLE, KEY/UNIQUE KEY clauses become CREATE
// INDEX, AUTO_INCREMENT becomes INTEGER PRIMARY KEY AUTOINCREMENT and
// MySQL-only clauses (ENGINE, COMMENT, ON UPDATE ...) are dropped.

var (
	reCreateTableHead = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + "`" + `?(\w+)` + "`" + `?`)
	reAlterTableHead  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+` + "`" + `?(\w+)` + "`" + `?`)
	reAddPKClause     = regexp.MustCompile(`(?i)ADD\s+PRIMARY\s+KEY\s*\(([^)]*)\)`)
	// reAlterModifyAutoInc: ALTER TABLE t MODIFY `ID` int NOT NULL
	// AUTO_INCREMENT — la forma en que el dump declara la clave autonumerada
	// de las tablas heredadas sin PRIMARY KEY.
	reAlterModifyAutoInc = regexp.MustCompile("(?i)^ALTER\\s+TABLE\\s+\\S+\\s+(?:MODIFY|CHANGE)\\s+`?(\\w+)`?[^;]*\\bAUTO_INCREMENT\\b")
	reInlinePK           = regexp.MustCompile(`(?i)^\s*PRIMARY\s+KEY\s*\(([^)]*)\)\s*,?\s*$`)
	reInlineKeyLine      = regexp.MustCompile(`(?i)^\s*(UNIQUE\s+)?(?:KEY|INDEX)\s+` + "`" + `?(\w+)` + "`" + `?\s*\(([^)]*)\)\s*,?\s*$`)
	reAddKeyClause       = regexp.MustCompile(`(?i)ADD\s+(UNIQUE\s+)?(?:KEY|INDEX)\s+` + "`" + `?(\w+)` + "`" + `?\s*\(([^)]*)\)`)
	reOnUpdate           = regexp.MustCompile(`(?i)\s+ON\s+UPDATE\s+CURRENT_TIMESTAMP`)
	reAutoIncrWord       = regexp.MustCompile(`(?i)\s+AUTO_INCREMENT\b`)
	reInlineCollation    = regexp.MustCompile(`(?i)\s+CHARACTER\s+SET\s+\w+(\s+COLLATE\s+\w+)?`)
	// reBareCollate: COLLATE sin CHARACTER SET delante (forma suelta de los
	// dumps de MySQL 8; sin quitarla el CREATE moría en "no such collation
	// sequence", que es como pos_sale_sync no nacía en SQLite).
	reBareCollate   = regexp.MustCompile(`(?i)\s+COLLATE\s+\w+`)
	reCommentClause = regexp.MustCompile(`(?si)\s+COMMENT\s+'(?:[^']|'')*'`)
	// reEnumType/reSetType: los dominios ENUM/SET de MySQL con sus valores
	// citados ('sale','purchase',…) — la alternativa con paréntesis anidados
	// no se da en los dumps que consume este paquete.
	reEnumType      = regexp.MustCompile(`(?i)\benum\s*\((?:'(?:[^']|'')*'|[^)])*\)`)
	reSetType       = regexp.MustCompile(`(?i)\bset\s*\((?:'(?:[^']|'')*'|[^)])*\)`)
	reMySQLOnlyStmt = regexp.MustCompile(`(?i)^\s*(SET\s|START\s+TRANSACTION\b|COMMIT\b|LOCK\s+TABLES\b|UNLOCK\s+TABLES\b|USE\s|FLUSH\b|DROP\s+PROCEDURE\b)`)
	reNowFunc       = regexp.MustCompile(`(?i)\bNOW\s*\(\s*\)`)
	reFromDual      = regexp.MustCompile(`(?i)\s+FROM\s+DUAL\b`)
	reIntBase       = regexp.MustCompile(`(?i)^(INT|INTEGER|BIGINT|MEDIUMINT|SMALLINT|TINYINT|SERIAL)$`)
)

// cleanIdent strips backticks from a column/table identifier.
func cleanIdent(s string) string {
	return strings.Trim(strings.TrimSpace(s), "`")
}

// collectMySQLPrimaryKeys returns table -> PK column list (raw, comma
// separated as declared) for every "ALTER TABLE t ADD PRIMARY KEY (...)" in
// the given migration files.
//
// También registra la segunda forma con la que el dump heredado declara su
// clave: `ALTER TABLE t MODIFY `ID` int NOT NULL AUTO_INCREMENT` sin
// PRIMARY KEY (acc_transaction, acc_vaucher… —en MySQL el AUTO_INCREMENT
// arrastra su propio uniqueness implícito—). Tratar esa columna como clave
// de una columna hace que convertCreateTableForSQLite la repliegue a
// `INTEGER PRIMARY KEY AUTOINCREMENT`, que es SEMÁNTICAMENTE lo mismo en
// SQLite: inserción sin id, numeración garantizada. Sin esto, el INSERT del
// asiento del POS moría en "NOT NULL constraint failed: ID" y la vida de
// SQLite mentía sobre la vida de MySQL (D-10, auditoría 2026-11-22).
func collectMySQLPrimaryKeys(migs []migFile) map[string]string {
	pkOf := map[string]string{}
	for _, m := range migs {
		for _, stmt := range splitSQL(m.content) {
			s := stripSQLComments(stmt)
			if s == "" {
				continue
			}
			tm := reAlterTableHead.FindStringSubmatch(s)
			if tm == nil {
				continue
			}
			pkm := reAddPKClause.FindStringSubmatch(s)
			if pkm != nil {
				cols := strings.TrimSpace(pkm[1])
				if cols != "" {
					if _, ok := pkOf[tm[1]]; !ok {
						pkOf[tm[1]] = cols
					}
				}
				continue
			}
			if am := reAlterModifyAutoInc.FindStringSubmatch(s); am != nil {
				col := strings.TrimSpace(am[1])
				if col != "" {
					if _, ok := pkOf[tm[1]]; !ok {
						pkOf[tm[1]] = col
					}
				}
			}
		}
	}
	return pkOf
}

// unescapeMySQLLiterals rewrites MySQL backslash-escaped string literals into
// standard SQL so SQLite can parse them: \' -> ”, \\ -> \, \n -> newline,
// \r -> CR, \t -> tab. Outside of single-quoted strings nothing changes.
func unescapeMySQLLiterals(s string) string {
	var b strings.Builder
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			inStr = !inStr
			b.WriteByte(c)
			continue
		}
		if inStr && c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case '\'':
				b.WriteString("''")
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '0':
				// NUL: not representable in most tooling; drop it.
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// convertForSQLite translates one MySQL statement into zero or more
// SQLite-compatible statements. An empty result means "skip".
func convertForSQLite(s string, pkOf map[string]string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Los comentarios van FUERA, antes de clasificar: una sentencia que
	// empieza con su bloque de cabecera ("-- Migración: …") no casa con el
	// prefijo CREATE TABLE y caía al default sin traducir —así nació el
	// `near "AUTO_INCREMENT"` que dejó pos_sale_sync y otras tablas sin
	// crear en SQLite, la deuda D-10 que la auditoría 2026-11-22 puso en
	// evidencia al intentar probar el motor de ventas offline sobre SQLite—.
	s = strings.TrimSpace(stripSQLComments(s))
	if s == "" {
		return nil
	}
	if reMySQLOnlyStmt.MatchString(s) {
		return nil
	}
	s = unescapeMySQLLiterals(s)
	if strings.Contains(s, "NOW(") || strings.Contains(s, "NOW (") {
		s = reNowFunc.ReplaceAllString(s, "CURRENT_TIMESTAMP")
	}
	// "SELECT ... FROM DUAL" (MySQL-only dummy table) -> plain SELECT.
	s = reFromDual.ReplaceAllString(s, "")
	upper := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(upper, "CREATE TABLE"):
		if !strings.Contains(upper, "IF NOT EXISTS") {
			s = "CREATE TABLE IF NOT EXISTS " + strings.TrimPrefix(s, "CREATE TABLE")
		}
		return convertCreateTableForSQLite(s, pkOf)
	case strings.HasPrefix(upper, "ALTER TABLE"):
		return convertAlterTableForSQLite(s)
	case strings.HasPrefix(upper, "INSERT IGNORE INTO"):
		return []string{"INSERT OR IGNORE INTO " + s[len("INSERT IGNORE INTO"):] + " "}
	case strings.HasPrefix(upper, "INSERT INTO"):
		return []string{"INSERT OR IGNORE INTO " + s[len("INSERT INTO"):] + " "}
	default:
		return []string{s}
	}
}

// idxStmt builds a SQLite CREATE INDEX. Index names are global in SQLite
// (per-database), unlike MySQL (per-table), so they get a table prefix.
func idxStmt(table, name string, unique bool, cols string) string {
	uniq := ""
	if unique {
		uniq = "UNIQUE "
	}
	return "CREATE " + uniq + "INDEX IF NOT EXISTS " +
		"idx_" + table + "_" + name + " ON " + table + " (" + cols + ")"
}

// convertCreateTableForSQLite rewrites a MySQL CREATE TABLE for SQLite:
// drops ENGINE tail, COMMENTs, collations, ON UPDATE; folds the PK in;
// emits index statements for inline KEY clauses.
func convertCreateTableForSQLite(s string, pkOf map[string]string) []string {
	tm := reCreateTableHead.FindStringSubmatch(s)
	if tm == nil {
		return []string{s}
	}
	table := tm[1]
	open := strings.IndexByte(s, '(')
	closeIdx := strings.LastIndexByte(s, ')')
	if open < 0 || closeIdx <= open {
		return []string{s}
	}
	head := s[:open+1] // includes "CREATE TABLE IF NOT EXISTS `t` ("
	body := s[open+1 : closeIdx]

	var newLines []string
	var indexStmts []string
	alreadyIndexed := map[string]bool{}
	inlinePK := ""
	autoIncrCol := ""

	for _, raw := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if m := reInlinePK.FindStringSubmatch(trimmed); m != nil {
			inlinePK = strings.TrimSpace(m[1])
			continue
		}
		if m := reInlineKeyLine.FindStringSubmatch(trimmed); m != nil {
			unique := m[1] != ""
			stmt := idxStmt(table, m[2], unique, m[3])
			if !alreadyIndexed[stmt] {
				alreadyIndexed[stmt] = true
				indexStmts = append(indexStmts, stmt)
			}
			continue
		}
		cleaned := reInlineCollation.ReplaceAllString(trimmed, "")
		// COLLATE suelto (los dumps de MySQL 8 escriben `varchar(64)
		// COLLATE utf8mb4_unicode_ci NOT NULL` sin CHARACTER SET delante):
		// reInlineCollation solo conocía la forma acompañada y el token
		// sobrevivía, con lo que SQLite abortaba el CREATE entero con
		// "no such collation sequence" —así se perdió pos_sale_sync y el
		// motor de ventas offline no tenía dónde reclamar (D-10, auditoría
		// 2026-11-22)—. En SQLite la comparación ya es la que usan las
		// pruebas; quitar el COLLATE no cambia ningún resultado.
		cleaned = reBareCollate.ReplaceAllString(cleaned, "")
		cleaned = reOnUpdate.ReplaceAllString(cleaned, "")
		cleaned = reCommentClause.ReplaceAllString(cleaned, "")
		// ENUM(...) no existe en SQLite. Se traduce a TEXT conservando las
		// demás palabras de la columna (NOT NULL, DEFAULT 'sale'): los
		// valores escritos por la aplicación son exactamente los mismos,
		// y el CONSTRAINT de dominio del enum no la aplicaba nadie ni en
		// MySQL con sql_mode relajado. Sin esta traducción,
		// stock_movement —la tabla del kardex— no se creaba en SQLite
		// y toda prueba del inventario era humo (D-10, auditoría).
		cleaned = reEnumType.ReplaceAllString(cleaned, "text")
		cleaned = reSetType.ReplaceAllString(cleaned, "text")
		if reAutoIncrWord.MatchString(cleaned) {
			fields := strings.Fields(cleaned)
			if len(fields) > 0 {
				autoIncrCol = cleanIdent(fields[0])
			}
			cleaned = reAutoIncrWord.ReplaceAllString(cleaned, "")
		}
		newLines = append(newLines, cleaned)
	}

	// Fold in the primary key: prefer the inline one, else the map built
	// from the ALTER TABLE statements of the whole migration set.
	pkCols := inlinePK
	if pkCols == "" {
		pkCols = pkOf[table]
	}
	if pkCols != "" && len(newLines) > 0 {
		cols := strings.Split(pkCols, ",")
		for i := range cols {
			cols[i] = cleanIdent(cols[i])
		}
		if len(cols) == 1 {
			c := cols[0]
			if isIntegerPrimaryKeyCandidate(newLines, c, autoIncrCol) {
				// rowid alias: INTEGER PRIMARY KEY AUTOINCREMENT
				for i, ln := range newLines {
					if cleanIdent(strings.Fields(ln)[0]) == c {
						comma := ""
						if i != len(newLines)-1 {
							comma = ","
						}
						newLines[i] = "`" + c + "` INTEGER PRIMARY KEY AUTOINCREMENT" + comma
						pkCols = ""
						break
					}
				}
			}
		}
	}
	if len(newLines) > 0 {
		// The last column definition may carry a trailing comma (in the
		// source it was followed by a PRIMARY KEY / KEY constraint line).
		// SQLite ends the column list there, so normalize it.
		newLines[len(newLines)-1] = strings.TrimRight(strings.TrimSuffix(newLines[len(newLines)-1], ","), " \t")
		if pkCols != "" {
			// Table-constraint PK (varchar keys, multi-column keys, or int
			// columns not rewritten as rowid aliases). The inline PRIMARY KEY
			// line was dropped from newLines above, so the last column line
			// must get its separator back before the PK is appended.
			newLines[len(newLines)-1] += ","
			newLines = append(newLines, "PRIMARY KEY ("+pkCols+")")
		}
	}

	out := []string{head + "\n" + strings.Join(newLines, "\n") + "\n)"}
	return append(out, indexStmts...)
}

// isIntegerPrimaryKeyCandidate reports whether column c should be the rowid
// alias: it is the AUTO_INCREMENT column, or the PK points at a column
// declared with an integer-ish type.
func isIntegerPrimaryKeyCandidate(lines []string, c, autoIncrCol string) bool {
	if c == autoIncrCol {
		return true
	}
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) < 2 || cleanIdent(fields[0]) != c {
			continue
		}
		base := strings.ToUpper(fields[1])
		if i := strings.IndexByte(base, '('); i >= 0 {
			base = base[:i] // strip display width: int(11) -> INT
		}
		return reIntBase.MatchString(base)
	}
	return false
}

// convertAlterTableForSQLite handles "ALTER TABLE" statements:
//   - MODIFY / CHANGE / CONVERT / AUTO_INCREMENT=n / DROP  -> skipped
//   - ADD PRIMARY KEY (folded into the CREATE TABLE)        -> skipped
//   - ADD [UNIQUE] KEY/INDEX                                -> CREATE INDEX
//   - ADD COLUMN (one or several)                           -> one ALTER each
//
// (SQLite executes a single action per ALTER TABLE.)
func convertAlterTableForSQLite(s string) []string {
	tm := reAlterTableHead.FindStringSubmatch(s)
	if tm == nil {
		return nil
	}
	table := "`" + tm[1] + "`"
	upper := strings.ToUpper(s)
	if strings.Contains(upper, "MODIFY") || strings.Contains(upper, "CHANGE ") ||
		strings.Contains(upper, "CONVERT TO") || strings.Contains(upper, "AUTO_INCREMENT =") ||
		strings.Contains(upper, "DROP ") || strings.Contains(upper, "RENAME") {
		// Untranslatable DDL changes: the table keeps its SQLite shape.
		return nil
	}

	// Split the action list on top-level commas (outside parens/strings)
	// so multi-action statements become one statement per action.
	var clauses []string
	if i := strings.Index(upper, "ALTER TABLE"); i >= 0 {
		rest := s[i+len("ALTER TABLE"):]
		rest = strings.TrimLeft(rest, " \t")
		// drop the table name (backtick-quoted or bare identifier)
		if strings.HasPrefix(rest, "`") {
			if j := strings.IndexByte(rest[1:], '`'); j >= 0 {
				rest = rest[j+2:]
			}
		} else if j := strings.IndexAny(rest, " \t\n"); j >= 0 {
			rest = rest[j:]
		}
		depth := 0
		inStr := false
		last := 0
		for j := 0; j < len(rest); j++ {
			switch c := rest[j]; c {
			case '\'':
				inStr = !inStr
			case '(':
				if !inStr {
					depth++
				}
			case ')':
				if !inStr && depth > 0 {
					depth--
				}
			case ',':
				if !inStr && depth == 0 {
					clauses = append(clauses, rest[last:j])
					last = j + 1
				}
			}
		}
		clauses = append(clauses, rest[last:])
	}

	var out []string
	seen := map[string]bool{}
	for _, cl := range clauses {
		c := strings.TrimSpace(reCommentClause.ReplaceAllString(strings.TrimSpace(cl), ""))
		if c == "" {
			continue
		}
		uc := strings.ToUpper(c)
		switch {
		case strings.HasPrefix(uc, "ADD COLUMN"):
			// SQLite no admite la cláusula IF NOT EXISTS (que MySQL 8
			// tampoco: la recorta SinSintaxisDeMariaDB, pero aquí las
			// sentencias llegan ya partidas por cláusula y vuelven a
			// aparecer). Sin quitarla, el ALTER moría en error de
			// sintaxis y la columna no se creaba NUNCA en las bases de
			// prueba —customer_information.rif, D-2 de la auditoría
			// 2026-11-22—. Quitarla es neutro: el "si falta" lo decide
			// quien ejecuta tolerando el error de columna duplicada. El
			// COMMENT de la definición ya lo quitó reCommentClause arriba.
			c = strings.TrimSpace(reIfNotExists.ReplaceAllString(c, ""))
			if !seen[c] {
				seen[c] = true
				out = append(out, "ALTER TABLE "+table+" "+c)
			}
		case strings.HasPrefix(uc, "ADD PRIMARY KEY"):
			// folded into the CREATE TABLE
		default:
			if m := reAddKeyClause.FindStringSubmatch(c); m != nil {
				uniq := m[1] != ""
				stmt := idxStmt(tm[1], m[2], uniq, m[3])
				if !seen[stmt] {
					seen[stmt] = true
					out = append(out, stmt)
				}
			}
			// other actions (CONSTRAINT FK, FULLTEXT...) are dropped
		}
	}
	return out
}
