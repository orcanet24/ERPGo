package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Simula N cajeros vendiendo a la vez sobre SQLite+WAL, con ráfagas de
// lecturas concurrentes entre venta y venta.

var (
	cajeros         = envInt("CAJEROS", 4)
	ventasPorCajero = envInt("VENTAS", 10)
	lecturasCiclo   = envInt("LECTURAS", 20)
	base            = envStr("BASE", "http://localhost:8082")
)

func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		n := 0
		for _, c := range v {
			if c < '0' || c > '9' {
				return d
			}
			n = n*10 + int(c-'0')
		}
		return n
	}
	return d
}

func envStr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var (
	okWrites, failWrites   int64
	okReads, failReads     int64
	writeLat, readLat      []time.Duration
	latMu                  sync.Mutex
	dates                  = time.Now().Format("2006-01-02")
)

func record(m *[]time.Duration, mu *sync.Mutex, d time.Duration) {
	mu.Lock()
	*m = append(*m, d)
	mu.Unlock()
}

func percentile(vals []time.Duration, p float64) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	c := append([]time.Duration(nil), vals...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[int(float64(len(c)-1)*p)]
}

func avg(vals []time.Duration) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	var s time.Duration
	for _, v := range vals {
		s += v
	}
	return s / time.Duration(len(vals))
}

func newClient() (*http.Client, *http.Cookie) {
	jar, _ := cookiejar.New(nil)
	cl := &http.Client{Timeout: 120 * time.Second, Jar: jar}
	// la sesión se crea al visitar la página de login
	resp, err := cl.Get(base + "/login")
	if err != nil {
		fmt.Println("GET /login error:", err)
		os.Exit(1)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	// POST /login responde 302 (el cliente puede seguirlo automáticamente);
	// lo que importa es que el jar quede con la cookie de sesión.
	resp, err = cl.PostForm(base+"/login", url.Values{"username": {"admin"}, "password": {"Admin@2026"}})
	if err != nil {
		fmt.Println("login error:", err)
		os.Exit(1)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	u, _ := url.Parse(base)
	for _, c := range jar.Cookies(u) {
		if c.Name == "erpgow_session" {
			return cl, c
		}
	}
	fmt.Println("sin cookie de sesión (login falló)")
	os.Exit(1)
	return nil, nil
}

func sell(cl *http.Client, cookie *http.Cookie) (bool, time.Duration) {
	form := url.Values{
		"customer_id":       {"1"},
		"product_id":        {os.Getenv("PID")},
		"product_quantity":  {"1"},
		"product_rate":      {"10"},
		"total_price":       {"10"},
		"desc":              {"Carga"},
		"discountvalue":     {"0"},
		"discount":          {"0"},
		"vatvalue":          {"0"},
		"vatpercent":        {"0"},
		"pay_method":        {"cash"},
		"pamount_by_method": {"10"},
		"sale_type":         {"cash"},
		"grand_total_price": {"10"},
		"paid_amount":       {"10"},
		"due_amount":        {"0"},
		"total_vat_amnt":    {"0"},
		"total_discount":    {"0"},
		"invoice_discount":  {"0"},
		"shipping_cost":     {"0"},
		"invoice_date":      {dates},
	}
	req, _ := http.NewRequest("POST", base+"/invoice/pos/save", bytes.NewReader([]byte(form.Encode())))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	t0 := time.Now()
	resp, err := cl.Do(req)
	d := time.Since(t0)
	if err != nil {
		return false, d
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		Status int `json:"status"`
	}
	_ = json.Unmarshal(body, &out)
	return resp.StatusCode == 200 && out.Status == 1, d
}

func readPage(cl *http.Client, cookie *http.Cookie, path string) (bool, time.Duration) {
	req, _ := http.NewRequest("GET", base+path, nil)
	req.AddCookie(cookie)
	t0 := time.Now()
	resp, err := cl.Do(req)
	d := time.Since(t0)
	if err != nil {
		return false, d
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return resp.StatusCode == 200, d
}

func main() {
	paths := []string{"/invoice/pos", "/invoice/list", "/customer/list", "/product/list", "/reports"}

	var wg sync.WaitGroup
	start := time.Now()
	for c := 0; c < cajeros; c++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cl, cookie := newClient()
			for v := 0; v < ventasPorCajero; v++ {
				// ráfaga de lecturas (otros módulos abiertos, como en el mostrador)
				for r := 0; r < lecturasCiclo; r++ {
					p := paths[(idx+v+r)%len(paths)]
					ok, d := readPage(cl, cookie, p)
					record(&readLat, &latMu, d)
					if ok {
						atomic.AddInt64(&okReads, 1)
					} else {
						atomic.AddInt64(&failReads, 1)
					}
				}
				// la venta (escritura con asientos contables)
				ok, d := sell(cl, cookie)
				record(&writeLat, &latMu, d)
				if ok {
					atomic.AddInt64(&okWrites, 1)
				} else {
					atomic.AddInt64(&failWrites, 1)
				}
			}
		}(c)
	}
	wg.Wait()
	elapsed := time.Since(start)

	totalWrites := atomic.LoadInt64(&okWrites) + atomic.LoadInt64(&failWrites)
	totalReads := atomic.LoadInt64(&okReads) + atomic.LoadInt64(&failReads)
	fmt.Printf("=== RESULTADO (%s) ===\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("Cajeros simultáneos: %d | Lecturas: %d (fallos %d) | Ventas: %d (fallos %d)\n",
		cajeros, totalReads, atomic.LoadInt64(&failReads), totalWrites, atomic.LoadInt64(&failWrites))
	fmt.Printf("Duración total: %v\n", elapsed.Round(time.Millisecond))
	if totalWrites > 0 {
		fmt.Printf("Throughput ventas: %.1f/min  |  ventas/seg: %.2f\n",
			float64(totalWrites)/elapsed.Minutes(), float64(totalWrites)/elapsed.Seconds())
	}
	fmt.Printf("Escritura (venta): promedio %v | p95 %v | máx %v\n", avg(writeLat).Round(time.Millisecond), percentile(writeLat, 0.95).Round(time.Millisecond), maxOf(writeLat).Round(time.Millisecond))
	fmt.Printf("Lectura (páginas): promedio %v | p95 %v | máx %v\n", avg(readLat).Round(time.Millisecond), percentile(readLat, 0.95).Round(time.Millisecond), maxOf(readLat).Round(time.Millisecond))
}

func maxOf(vals []time.Duration) time.Duration {
	var m time.Duration
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}
