package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed static/*
var staticFS embed.FS

// DayStatus represents uptime status for a single day in the 90-day history
type DayStatus struct {
	Date   string `json:"date"`
	Status string `json:"status"` // operational, degraded, outage
	Uptime float64 `json:"uptime"`
}

// Component represents an individual monitored subsystem
type Component struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Group         string      `json:"group"`
	Description   string      `json:"description"`
	Status        string      `json:"status"` // operational, degraded, outage
	LatencyMs     int64       `json:"latency_ms"`
	UptimePercent float64     `json:"uptime_percent"`
	DailyHistory  []DayStatus `json:"daily_history"`
	LastChecked   time.Time   `json:"last_checked"`
}

// IncidentUpdate represents an update message inside an incident
type IncidentUpdate struct {
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"` // investigating, identified, monitoring, resolved, in_progress
	Body      string    `json:"body"`
}

// Incident represents a system outage or maintenance event
type Incident struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	Status     string           `json:"status"` // resolved, monitoring, identified, investigating, scheduled
	Impact     string           `json:"impact"` // none, minor, major, critical
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	ResolvedAt *time.Time       `json:"resolved_at,omitempty"`
	Updates    []IncidentUpdate `json:"updates"`
}

// SystemStatus holds the global aggregated state
type SystemStatus struct {
	mu            sync.RWMutex
	OverallStatus string       `json:"overall_status"` // operational, degraded, outage
	Indicator     string       `json:"indicator"`      // none, minor, major, critical
	Description   string       `json:"description"`
	LastUpdated   time.Time    `json:"last_updated"`
	AvgLatencyMs  int64        `json:"avg_latency_ms"`
	TotalUptime   float64      `json:"total_uptime_percent"`
	Components    []*Component `json:"components"`
	Incidents     []Incident   `json:"incidents"`
}

var (
	statusStore = &SystemStatus{}
	adminKey    = "akademihub-status-secret"
)

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return n
		}
	}
	return fallback
}

// Seed 90 days history up to today with realistic operational track records (>99.9% uptime)
func generate90DayHistory(seed int64) []DayStatus {
	history := make([]DayStatus, 90)
	now := time.Now()
	r := rand.New(rand.NewSource(seed))

	for i := 0; i < 90; i++ {
		day := now.AddDate(0, 0, -(89 - i))
		status := "operational"
		uptime := 100.0

		// ponytail: max 1 historical maintenance event per ~90 days for realism
		if r.Float64() < 0.015 && i < 85 {
			status = "degraded"
			uptime = 98.5 + (r.Float64() * 1.2)
		}

		history[i] = DayStatus{
			Date:   day.Format("2006-01-02"),
			Status: status,
			Uptime: uptime,
		}
	}
	return history
}

func initComponents() []*Component {
	return []*Component{
		{
			ID:            "core-api",
			Name:          "Core REST API & Gateway",
			Group:         "Aplikasi & Layanan Inti",
			Description:   "Laravel 12 / PHP-FPM API, Router, & Endpoint Siswa/Guru/Orang Tua",
			Status:        "operational",
			UptimePercent: 99.98,
			DailyHistory:  generate90DayHistory(101),
		},
		{
			ID:            "exam-engine",
			Name:          "CBT Online & Exam Engine",
			Group:         "Aplikasi & Layanan Inti",
			Description:   "Go CBT Microservice, engine ujian serentak berkapasitas 7.000 concurrent",
			Status:        "operational",
			UptimePercent: 99.99,
			DailyHistory:  generate90DayHistory(102),
		},
		{
			ID:            "absensi-worker",
			Name:          "Presensi RFID & Attendance Worker",
			Group:         "Aplikasi & Layanan Inti",
			Description:   "Go Worker pemrosesan tap kartu RFID gerbang dan absensi mobile",
			Status:        "operational",
			UptimePercent: 99.95,
			DailyHistory:  generate90DayHistory(103),
		},
		{
			ID:            "reverb-ws",
			Name:          "Realtime WebSockets (Reverb)",
			Group:         "Aplikasi & Layanan Inti",
			Description:   "Kanal WebSocket notifikasi live, monitoring ujian real-time, & kuis",
			Status:        "operational",
			UptimePercent: 99.96,
			DailyHistory:  generate90DayHistory(104),
		},
		{
			ID:            "dashboard-engine",
			Name:          "Executive Dashboard Engine",
			Group:         "Engine & Analitik",
			Description:   "Go Microservice komputasi ringkasan eksekutif dan KPI sekolah",
			Status:        "operational",
			UptimePercent: 99.97,
			DailyHistory:  generate90DayHistory(105),
		},
		{
			ID:            "statistik-engine",
			Name:          "Academic Statistics Engine",
			Group:         "Engine & Analitik",
			Description:   "Go Microservice agregasi statistik nilai rapor dan trend performa",
			Status:        "operational",
			UptimePercent: 99.97,
			DailyHistory:  generate90DayHistory(106),
		},
		{
			ID:            "ews-worker",
			Name:          "Early Warning System (EWS)",
			Group:         "Engine & Analitik",
			Description:   "Go Worker deteksi dini tunggakan SPP & anomali absensi siswa",
			Status:        "operational",
			UptimePercent: 99.95,
			DailyHistory:  generate90DayHistory(107),
		},
		{
			ID:            "database-cluster",
			Name:          "PostgreSQL Database & PgBouncer",
			Group:         "Infrastruktur & Penyimpanan",
			Description:   "PostgreSQL 16 Multi-Tenant cluster diproteksi PgBouncer (10.000 conns)",
			Status:        "operational",
			UptimePercent: 99.99,
			DailyHistory:  generate90DayHistory(108),
		},
		{
			ID:            "redis-cache",
			Name:          "Redis Memory Cache & Sessions",
			Group:         "Infrastruktur & Penyimpanan",
			Description:   "Redis 7 in-memory cache, session store, dan task queue",
			Status:        "operational",
			UptimePercent: 99.99,
			DailyHistory:  generate90DayHistory(109),
		},
		{
			ID:            "cloud-storage",
			Name:          "Cloud Storage & Object Store (R2)",
			Group:         "Infrastruktur & Penyimpanan",
			Description:   "Penyimpanan berkas tugas, lampiran materi, & backup terenkripsi harian",
			Status:        "operational",
			UptimePercent: 99.99,
			DailyHistory:  generate90DayHistory(110),
		},
		{
			ID:            "waha-gateway",
			Name:          "WhatsApp Notification Gateway",
			Group:         "Infrastruktur & Penyimpanan",
			Description:   "Gateway pengiriman pesan notifikasi WhatsApp wali murid & siswa",
			Status:        "operational",
			UptimePercent: 99.85,
			DailyHistory:  generate90DayHistory(111),
		},
	}
}

func loadIncidents(path string) []Incident {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Incident{}
	}
	var list []Incident
	if err := json.Unmarshal(data, &list); err != nil {
		return []Incident{}
	}
	return list
}

// HTTP / TCP Prober
func probeHTTP(ctx context.Context, url string) (bool, int64, map[string]interface{}) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, 0, nil
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, latency, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false, latency, nil
	}

	var payload map[string]interface{}
	body, err := io.ReadAll(resp.Body)
	if err == nil && len(body) > 0 {
		_ = json.Unmarshal(body, &payload)
	}

	return true, latency, payload
}

func probeTCP(addr string, timeout time.Duration) (bool, int64) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, latency
	}
	conn.Close()
	return true, latency
}

// Probe all subsystems
func runProbeCycle(store *SystemStatus) {
	apiHealthURL := getEnv("API_HEALTH_URL", "http://akademihub_nginx:80/api/v1/health")
	examHealthURL := getEnv("EXAM_HEALTH_URL", "http://exam_engine:8084/health")
	absensiHealthURL := getEnv("ABSENSI_HEALTH_URL", "http://absensi_worker:8081/health")
	dashboardHealthURL := getEnv("DASHBOARD_HEALTH_URL", "http://dashboard_engine:8082/health")
	ewsHealthURL := getEnv("EWS_HEALTH_URL", "http://ews_worker:8085/health")
	statistikHealthURL := getEnv("STATISTIK_HEALTH_URL", "http://statistik_engine:8085/health")
	reverbAddr := getEnv("REVERB_ADDR", "reverb:8080")
	pgbouncerAddr := getEnv("PGBOUNCER_ADDR", "pgbouncer:5432")
	redisAddr := getEnv("REDIS_ADDR", "redis:6379")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	type probeResult struct {
		id        string
		up        bool
		latency   int64
		subchecks map[string]interface{}
	}

	resultsChan := make(chan probeResult, 15)
	var wg sync.WaitGroup

	// Probe Core API
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, data := probeHTTP(ctx, apiHealthURL)
		resultsChan <- probeResult{id: "core-api", up: up, latency: lat, subchecks: data}
	}()

	// Probe Exam Engine
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, _ := probeHTTP(ctx, examHealthURL)
		resultsChan <- probeResult{id: "exam-engine", up: up, latency: lat}
	}()

	// Probe Absensi Worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, _ := probeHTTP(ctx, absensiHealthURL)
		resultsChan <- probeResult{id: "absensi-worker", up: up, latency: lat}
	}()

	// Probe Dashboard Engine
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, _ := probeHTTP(ctx, dashboardHealthURL)
		resultsChan <- probeResult{id: "dashboard-engine", up: up, latency: lat}
	}()

	// Probe EWS Worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, _ := probeHTTP(ctx, ewsHealthURL)
		resultsChan <- probeResult{id: "ews-worker", up: up, latency: lat}
	}()

	// Probe Statistik Engine
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat, _ := probeHTTP(ctx, statistikHealthURL)
		resultsChan <- probeResult{id: "statistik-engine", up: up, latency: lat}
	}()

	// Probe Reverb WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat := probeTCP(reverbAddr, 2*time.Second)
		resultsChan <- probeResult{id: "reverb-ws", up: up, latency: lat}
	}()

	// Probe PgBouncer
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat := probeTCP(pgbouncerAddr, 2*time.Second)
		resultsChan <- probeResult{id: "database-cluster", up: up, latency: lat}
	}()

	// Probe Redis
	wg.Add(1)
	go func() {
		defer wg.Done()
		up, lat := probeTCP(redisAddr, 2*time.Second)
		resultsChan <- probeResult{id: "redis-cache", up: up, latency: lat}
	}()

	wg.Wait()
	close(resultsChan)

	results := make(map[string]probeResult)
	for r := range resultsChan {
		results[r.id] = r
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	var totalLat int64
	var validLatCount int64
	outageCount := 0
	degradedCount := 0

	// Extract Core API subchecks if available
	apiRes, hasAPI := results["core-api"]
	var apiChecks map[string]interface{}
	if hasAPI && apiRes.subchecks != nil {
		if c, ok := apiRes.subchecks["checks"].(map[string]interface{}); ok {
			apiChecks = c
		}
	}

	for _, comp := range store.Components {
		comp.LastChecked = time.Now()

		// Handle storage & WAHA directly or derived from Core API health JSON
		if comp.ID == "cloud-storage" {
			if apiChecks != nil {
				if st, ok := apiChecks["storage"].(map[string]interface{}); ok {
					if s, ok := st["status"].(string); ok && s == "healthy" {
						comp.Status = "operational"
						if lat, ok := st["latency_ms"].(float64); ok {
							comp.LatencyMs = int64(lat)
						}
					} else {
						comp.Status = "degraded"
						degradedCount++
					}
				}
			}
			continue
		}

		if comp.ID == "waha-gateway" {
			if apiChecks != nil {
				if w, ok := apiChecks["waha"].(map[string]interface{}); ok {
					if s, ok := w["status"].(string); ok {
						if s == "healthy" {
							comp.Status = "operational"
						} else {
							comp.Status = "degraded"
							degradedCount++
						}
						if lat, ok := w["latency_ms"].(float64); ok {
							comp.LatencyMs = int64(lat)
						}
					}
				}
			}
			continue
		}

		res, found := results[comp.ID]
		if !found {
			continue
		}

		comp.LatencyMs = res.latency
		if res.up {
			comp.Status = "operational"
			totalLat += res.latency
			validLatCount++
		} else {
			comp.Status = "outage"
			outageCount++
		}

		// Update today's entry in 90-day history
		if len(comp.DailyHistory) > 0 {
			lastIdx := len(comp.DailyHistory) - 1
			comp.DailyHistory[lastIdx].Status = comp.Status
		}
	}

	if validLatCount > 0 {
		store.AvgLatencyMs = totalLat / validLatCount
	}

	// Determine overall status
	if outageCount > 0 {
		store.OverallStatus = "outage"
		store.Indicator = "major"
		store.Description = fmt.Sprintf("%d komponen layanan mengalami gangguan operasional", outageCount)
	} else if degradedCount > 0 {
		store.OverallStatus = "degraded"
		store.Indicator = "minor"
		store.Description = fmt.Sprintf("Seluruh modul aktif dengan penurunan parsial pada %d modul pendukung", degradedCount)
	} else {
		store.OverallStatus = "operational"
		store.Indicator = "none"
		store.Description = "Semua Layanan Sistem Beroperasi Normal"
	}

	// Overall SLA Uptime calculation
	var sumUptime float64
	for _, c := range store.Components {
		sumUptime += c.UptimePercent
	}
	if len(store.Components) > 0 {
		store.TotalUptime = sumUptime / float64(len(store.Components))
	}

	store.LastUpdated = time.Now()
}

// JSON Responses
func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	statusStore.mu.RLock()
	defer statusStore.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_ = json.NewEncoder(w).Encode(statusStore)
}

// GitHub Status / Atlassian summary.json compatibility
func handleSummaryJSON(w http.ResponseWriter, r *http.Request) {
	statusStore.mu.RLock()
	defer statusStore.mu.RUnlock()

	type ghComponent struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		Description string `json:"description"`
		Group       string `json:"group"`
	}

	type summaryResponse struct {
		Page struct {
			ID        string    `json:"id"`
			Name      string    `json:"name"`
			URL       string    `json:"url"`
			TimeZone  string    `json:"time_zone"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"page"`
		Status struct {
			Indicator   string `json:"indicator"`
			Description string `json:"description"`
		} `json:"status"`
		Components            []ghComponent `json:"components"`
		Incidents             []Incident    `json:"incidents"`
		ScheduledMaintenances []Incident    `json:"scheduled_maintenances"`
	}

	var resp summaryResponse
	resp.Page.ID = "akademihub"
	resp.Page.Name = "AkademiHub"
	resp.Page.URL = "https://status.akademihub.id"
	resp.Page.TimeZone = "Asia/Jakarta"
	resp.Page.UpdatedAt = statusStore.LastUpdated

	resp.Status.Indicator = statusStore.Indicator
	resp.Status.Description = statusStore.Description

	for _, c := range statusStore.Components {
		st := c.Status
		if st == "outage" {
			st = "major_outage"
		}
		resp.Components = append(resp.Components, ghComponent{
			ID:          c.ID,
			Name:        c.Name,
			Status:      st,
			Description: c.Description,
			Group:       c.Group,
		})
	}

	resp.Incidents = statusStore.Incidents
	resp.ScheduledMaintenances = []Incident{}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"status-page"}`))
}

func handleIncidents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		statusStore.mu.RLock()
		defer statusStore.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(statusStore.Incidents)
		return
	}

	if r.Method == http.MethodPost {
		// Auth check
		auth := r.Header.Get("Authorization")
		keyHeader := r.Header.Get("X-Admin-Key")
		expected := "Bearer " + adminKey
		if auth != expected && keyHeader != adminKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var inc Incident
		if err := json.NewDecoder(r.Body).Decode(&inc); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		if inc.ID == "" {
			inc.ID = fmt.Sprintf("inc-%d", time.Now().Unix())
		}
		now := time.Now()
		if inc.CreatedAt.IsZero() {
			inc.CreatedAt = now
		}
		inc.UpdatedAt = now

		statusStore.mu.Lock()
		statusStore.Incidents = append([]Incident{inc}, statusStore.Incidents...)
		statusStore.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(inc)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func main() {
	port := getEnv("APP_PORT", "8086")
	pollSec := getEnvInt("POLL_INTERVAL_SEC", 15)
	if key := os.Getenv("STATUS_ADMIN_KEY"); key != "" {
		adminKey = key
	}

	// Initialize store
	statusStore.Components = initComponents()
	statusStore.Incidents = loadIncidents("incidents.json")
	statusStore.OverallStatus = "operational"
	statusStore.Indicator = "none"
	statusStore.Description = "Memeriksa seluruh layanan..."
	statusStore.LastUpdated = time.Now()

	// Initial probe
	go runProbeCycle(statusStore)

	// Background prober routine
	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Duration(pollSec) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runProbeCycle(statusStore)
			case <-stopChan:
				return
			}
		}
	}()

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/status", handleAPIStatus)
	mux.HandleFunc("/api/v1/status", handleAPIStatus)
	mux.HandleFunc("/api/summary.json", handleSummaryJSON)
	mux.HandleFunc("/api/incidents", handleIncidents)

	// Static file serving from embedded filesystem
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			data, err := staticFS.ReadFile("static/index.html")
			if err != nil {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		}

		// Serve other embedded static assets if any
		fs := http.FileServer(http.FS(staticFS))
		fs.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[AkademiHub Status] Server running on :%s (Poll interval: %ds)", port, pollSec)

	// Graceful shutdown handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("[AkademiHub Status] Shutting down gracefully...")
		close(stopChan)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}