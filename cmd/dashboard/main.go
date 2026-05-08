package main

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	goruntime "runtime"
	"syscall"
	"time"

	"ai-flight-dashboard/internal/alert"
	"ai-flight-dashboard/internal/applock"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/desktop"
	"ai-flight-dashboard/internal/forwarder"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/scanner"
	"ai-flight-dashboard/internal/tui"
	"ai-flight-dashboard/internal/watcher"
	"ai-flight-dashboard/internal/web"

	root "ai-flight-dashboard"
	tea "github.com/charmbracelet/bubbletea"
	wailsrun "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed pricing_table.json
var embeddedPricing []byte

func fetchDynamicPricing(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
}

func startLANGoroutines(lanInst *lan.LAN, database *db.DB, token string, broadcastChan <-chan model.TokenUsage, usageChan chan<- model.TokenUsage) {
	fmt.Printf("📡 LAN discovery enabled. Multicast: %s\n", lan.MulticastAddr)
	go lanInst.StartBroadcaster(broadcastChan)
	go lanInst.StartListener(usageChan)
	go lanInst.StartPinger()
	go lanInst.StartAutoSync(database, token)
}

func startLANHTTPServer(ctx context.Context, port string, handler http.Handler) bool {
	addr := "0.0.0.0:" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("LAN API server unavailable on port %s: %v", port, err)
		return false
	}
	srv := &http.Server{Handler: handler}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		fmt.Printf("🌐 LAN API server: http://0.0.0.0:%s\n", port)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("LAN API server unavailable on port %s: %v", port, err)
		}
	}()
	return true
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse CLI flags
	webMode := flag.Bool("web", false, "Run in web dashboard mode")
	tuiMode := flag.Bool("tui", false, "Run in legacy Terminal UI mode")
	port := flag.String("port", "19100", "HTTP port for web mode")
	forwardTo := flag.String("forward-to", "", "Forward local usage to remote dashboard URL (e.g. http://server:19100/api/track)")
	token := flag.String("token", "", "Authorization token for web mode or forwarder")
	defaultDevice, _ := os.Hostname()
	if defaultDevice == "" {
		defaultDevice = "local"
	}
	deviceID := flag.String("device-id", defaultDevice, "Device identifier")
	billingModeStr := flag.String("billing-mode", "auto", "Billing mode: auto, subscription, api")
	plan := flag.String("plan", "", "Subscription plan: pro, max5, max20 (only for subscription mode)")
	budgetDaily := flag.Float64("budget-daily", 0, "Daily budget limit in USD (only for api mode, 0=disabled)")
	syncMode := flag.String("sync-mode", "poll", "Sync mode: poll (default), fsnotify, once")
	lanMode := flag.Bool("lan", true, "Enable UDP Multicast LAN discovery and broadcast")
	dataDir := flag.String("data-dir", "", "Data directory for database and config (default: current directory)")
	flag.BoolVar(webMode, "w", false, "Run in web dashboard mode (shorthand)")
	flag.StringVar(port, "p", "19100", "HTTP port for web mode (shorthand)")
	flag.Parse()
	args := flag.Args()
	repairHistory := len(args) > 0 && args[0] == "repair-history"
	if *dataDir != "" {
		config.SetDataDir(*dataDir)
	}

	// Default to GUI mode unless another mode is explicitly requested
	runGui := true
	if *tuiMode || *webMode || *forwardTo != "" || len(flag.Args()) > 0 {
		runGui = false
	}

	// Read token from environment variable if not provided via flag
	if *token == "" {
		*token = os.Getenv("DASHBOARD_TOKEN")
	}

	// Validate billing mode
	billingMode, err := model.ParseBillingMode(*billingModeStr)
	if err != nil {
		log.Fatalf("Invalid billing mode: %v", err)
	}
	if billingMode == model.BillingSubscription && *plan != "" {
		if _, ok := model.PlanLimits[*plan]; !ok {
			log.Fatalf("Unknown plan %q: must be pro, max5, or max20", *plan)
		}
	}

	// Subcommand dispatch: export | import | dedup | repair-history
	if len(args) > 0 {
		switch args[0] {
		case "export":
			runExport(*deviceID)
			return
		case "import":
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "Usage: dashboard import <file.csv>")
				os.Exit(1)
			}
			runImport(args[1])
			return
		case "dedup":
			runDedup()
			return
		case "repair-history":
			// Handled after calculator/database initialization.
		}
	}

	// Forwarder Mode
	if *forwardTo != "" {
		fmt.Printf("🚀 Starting forwarder mode. Device: %s, Target: %s\n", *deviceID, *forwardTo)

		w, err := watcher.New(*deviceID)
		if err != nil {
			log.Fatalf("Failed to initialize watcher: %v", err)
		}
		defer w.Close()

		home, _ := os.UserHomeDir()
		claudeProjects := filepath.Join(home, ".claude", "projects")
		if _, err := os.Stat(claudeProjects); err == nil {
			w.WatchDirRecursive(claudeProjects)
		}
		geminiTmp := filepath.Join(home, ".gemini", "tmp")
		if _, err := os.Stat(geminiTmp); err == nil {
			w.WatchDirRecursive(geminiTmp)
		}

		fw := forwarder.New(*forwardTo, *token, *deviceID)
		fw.Start(w.UsageChan) // Blocks forever
		return
	}

	// Initialize Calculator from pricing table
	pricingData := embeddedPricing
	const pricingURL = "https://raw.githubusercontent.com/icebear0828/ai-flight-dashboard/main/cmd/dashboard/pricing_table.json"
	if dynPricing, err := fetchDynamicPricing(pricingURL, 3*time.Second); err == nil {
		fmt.Println("☁️  Successfully fetched dynamic pricing table from GitHub.")
		pricingData = dynPricing
	} else {
		fmt.Printf("⚠️  Failed to fetch dynamic pricing table, using embedded version: %v\n", err)
	}

	calc, err := calculator.NewFromBytes(pricingData)
	if err != nil {
		log.Fatalf("Failed to initialize calculator: %v", err)
	}

	home, _ := os.UserHomeDir()
	customPricingPath := filepath.Join(home, ".ai-flight-dashboard", "custom_pricing.json")
	if err := calc.LoadCustomPrices(customPricingPath); err != nil {
		fmt.Printf("⚠️  Failed to load custom pricing: %v\n", err)
	}

	// Initialize Database
	appDataDir := config.GetDataDir()
	statsDir := filepath.Join(appDataDir, "stats")
	os.MkdirAll(statsDir, 0755)

	processLock := acquireProcessLock(appDataDir)
	defer processLock.Release()

	dbPath := filepath.Join(statsDir, "usage.db")
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	if n, err := database.BackfillProjectsFromFilePaths(watcher.ExtractProjectName); err != nil {
		log.Printf("Failed to backfill project names: %v", err)
	} else if n > 0 {
		fmt.Printf("🧭 Backfilled project names for %d existing records.\n", n)
	}
	const projectMetadataMigrationKey = "migration:project-metadata-v2"
	if done, err := database.GetOffset(projectMetadataMigrationKey); err == nil && done == 0 {
		var queued int64
		for _, pattern := range []string{
			"%/.claude/projects/%",
			"%\\.claude\\projects\\%",
			"%/.gemini/tmp/%",
			"%\\.gemini\\tmp\\%",
		} {
			n, err := database.ResetOffsetsLike(pattern)
			if err != nil {
				log.Printf("Failed to reset project metadata scan offsets for %q: %v", pattern, err)
				continue
			}
			queued += n
		}
		if queued > 0 {
			fmt.Printf("🧭 Queued %d log files for project metadata backfill.\n", queued)
		}
		if err := database.SetOffset(projectMetadataMigrationKey, 1); err != nil {
			log.Printf("Failed to mark project metadata migration complete: %v", err)
		}
	}
	// Collect scan directories
	var scanDirs []string

	claudeProjects := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(claudeProjects); err == nil {
		scanDirs = append(scanDirs, claudeProjects)
	}

	geminiTmp := filepath.Join(home, ".gemini", "tmp")
	if _, err := os.Stat(geminiTmp); err == nil {
		scanDirs = append(scanDirs, geminiTmp)
	}

	appConfig, _ := config.LoadConfig()
	for _, dir := range appConfig.ExtraWatchDirs {
		if _, err := os.Stat(dir); err == nil {
			scanDirs = append(scanDirs, dir)
		}
	}
	if repairHistory {
		runRepairHistory(database, calc, *deviceID, scanDirs)
		return
	}

	// Initialize Watcher immediately (instant, ~26µs)
	w, err := watcher.New(*deviceID)
	if err != nil {
		log.Fatalf("Failed to initialize watcher: %v", err)
	}
	defer w.Close()

	var lanInst *lan.LAN
	if *lanMode && appConfig.EnableLAN != nil && *appConfig.EnableLAN {
		portInt, _ := strconv.Atoi(*port)
		lanInst = lan.New(*deviceID, portInt)
	} else {
		fmt.Println("📡 LAN discovery is disabled in settings.")
	}

	// Fast path: register cached known directories (~1ms vs 134ms recursive)
	knownDirs, _ := database.ListKnownDirs()
	if *syncMode == "fsnotify" && len(knownDirs) > 0 {
		w.WatchKnownDirs(knownDirs)
		// Also watch roots recursively so new Gemini/Claude session subdirs are not missed.
		for _, dir := range scanDirs {
			w.WatchDirRecursive(dir)
		}
	}

	// Background: full scan + discover new directories
	go func() {
		s := scanner.New(database, calc, *deviceID)
		codexScanner := codexusage.New(database, calc, *deviceID)
		s.ScanAll(scanDirs, w.UsageChan) // incremental; auto-caches new dirs to known_dirs
		codexScanner.Scan(w.UsageChan)

		if *syncMode == "fsnotify" {
			if len(knownDirs) == 0 {
				// First run: no cache yet, do full recursive watch
				for _, dir := range scanDirs {
					w.WatchDirRecursive(dir)
				}
			} else {
				// Subsequent runs: pick up any newly discovered dirs
				newDirs, _ := database.ListKnownDirs()
				w.WatchKnownDirs(newDirs)
			}
			codexTicker := time.NewTicker(30 * time.Second)
			defer codexTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-codexTicker.C:
					if !w.IsPaused() {
						codexScanner.Scan(w.UsageChan)
					}
				}
			}
		} else if *syncMode == "poll" {
			fastTicker := time.NewTicker(2 * time.Second)
			slowTicker := time.NewTicker(60 * time.Second)
			defer fastTicker.Stop()
			defer slowTicker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-fastTicker.C:
					if !w.IsPaused() {
						s.ScanKnownFiles(w.UsageChan)
						codexScanner.Scan(w.UsageChan)
					}
				case <-slowTicker.C:
					if !w.IsPaused() {
						s.ScanAll(scanDirs, w.UsageChan)
					}
				}
			}
		}
		// "once" mode does nothing more
	}()

	// Background goroutine shared by web/gui modes: drain watcher events and persist to DB
	startDBDrain := func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case u, ok := <-w.UsageChan:
					if !ok {
						return
					}
					isLocal := (u.DeviceID == "" || u.DeviceID == *deviceID)

					// fsnotify events are inserted here; poll scanners already inserted
					// their records before sending usage notifications.
					if !isLocal || *syncMode == "fsnotify" {
						cost, _ := calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)

						effectiveDevice := u.DeviceID
						if effectiveDevice == "" {
							effectiveDevice = *deviceID
						}

						if err := database.InsertUsage(u, cost, effectiveDevice); err != nil {
							log.Printf("Failed to insert usage for device %s: %v", effectiveDevice, err)
							continue
						}
					}
					if isLocal && lanInst != nil {
						lanInst.AnnounceDirty()
					}
				}
			}
		}()
	}

	if runGui {
		startDBDrain()

		app := desktop.NewApp(database, calc)

		// Build system tray / application menu
		appMenu := menu.NewMenu()
		fileMenu := appMenu.AddSubmenu("File")
		fileMenu.AddText("Show Dashboard", keys.CmdOrCtrl("d"), func(cd *menu.CallbackData) {
			runtime.WindowShow(app.GetContext())
		})
		fileMenu.AddSeparator()
		fileMenu.AddText("Quit", keys.CmdOrCtrl("q"), func(cd *menu.CallbackData) {
			runtime.Quit(app.GetContext())
		})

		if goos := goruntime.GOOS; goos == "darwin" {
			appMenu.Append(menu.EditMenu())
		}

		fmt.Println("🖥️  Starting AI Flight Dashboard (GUI mode)...")

		// Wails expects index.html at the FS root; strip the "dist" prefix
		guiAssets, err := fs.Sub(web.StaticFiles, "dist")
		if err != nil {
			log.Fatalf("Failed to create asset FS: %v", err)
		}

		if lanInst != nil {
			if startLANHTTPServer(ctx, *port, web.NewLANHandler(database, *token)) {
				startLANGoroutines(lanInst, database, *token, w.BroadcastChan, w.UsageChan)
			} else {
				lanInst = nil
			}
		}

		// Reuse the existing HTTP handler inside Wails.
		apiHandler := web.NewHandler(database, calc, w, lanInst, *token, root.DistBinFS)

		err = wailsrun.Run(&options.App{
			Title:     "AI Flight Dashboard",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 600,
			SingleInstanceLock: &options.SingleInstanceLock{
				UniqueId: "ai-flight-dashboard",
				OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
					// Wails handles focusing the primary window automatically
				},
			},
			AssetServer: &assetserver.Options{
				Assets:  guiAssets,
				Handler: apiHandler,
			},
			OnStartup: app.Startup,
			Menu:      appMenu,
			Bind: []interface{}{
				app,
			},
			Mac: &mac.Options{
				TitleBar:             mac.TitleBarHiddenInset(),
				WebviewIsTransparent: true,
				WindowIsTranslucent:  false,
				About: &mac.AboutInfo{
					Title:   "AI Flight Dashboard",
					Message: "Real-time AI token cost monitoring",
				},
			},
		})
		if err != nil {
			log.Fatalf("GUI error: %v", err)
		}
		return
	}

	if *webMode {
		startDBDrain()

		// Web dashboard mode with graceful shutdown
		handler := web.NewHandler(database, calc, w, lanInst, *token, root.DistBinFS)
		srv := &http.Server{Addr: "0.0.0.0:" + *port, Handler: handler}
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			log.Fatalf("Web server error: %v", err)
		}
		if lanInst != nil {
			startLANGoroutines(lanInst, database, *token, w.BroadcastChan, w.UsageChan)
		}

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			fmt.Println("\n🛬 Shutting down gracefully...")
			srv.Shutdown(context.Background())
		}()

		fmt.Printf("🌐 Web dashboard: http://localhost:%s\n", *port)
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			log.Fatalf("Web server error: %v", err)
		}
		return
	}

	// TUI mode — starts instantly, data populates in background
	homeDirTui, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDirTui, ".ai-flight-dashboard", "stats", "debug.log")
	os.MkdirAll(filepath.Dir(logPath), 0755)
	f, err := tea.LogToFile(logPath, "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()

	// Budget alert: active in api mode with a daily limit
	var budgetAlert *alert.BudgetAlert
	if billingMode == model.BillingAPI && *budgetDaily > 0 {
		budgetAlert = alert.NewBudgetAlert(database, *budgetDaily)
		fmt.Printf("💰 Budget mode: $%.2f/day limit\n", *budgetDaily)
	}
	if lanInst != nil {
		if startLANHTTPServer(ctx, *port, web.NewLANHandler(database, *token)) {
			startLANGoroutines(lanInst, database, *token, w.BroadcastChan, w.UsageChan)
		} else {
			lanInst = nil
		}
	}

	skipDBWrite := (*syncMode != "fsnotify")
	p := tea.NewProgram(tui.NewModel(calc, w, database, budgetAlert, skipDBWrite))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func openDB() (*db.DB, *applock.Lock) {
	appDataDir := config.GetDataDir()
	lock := acquireProcessLock(appDataDir)
	dbPath := filepath.Join(appDataDir, "stats", "usage.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	database, err := db.New(dbPath)
	if err != nil {
		lock.Release()
		log.Fatalf("Failed to open database: %v", err)
	}
	return database, lock
}

func acquireProcessLock(dataDir string) *applock.Lock {
	lockPath := filepath.Join(dataDir, "dashboard.lock")
	lock, err := applock.TryAcquire(lockPath)
	if err == nil {
		return lock
	}
	if errors.Is(err, applock.ErrAlreadyLocked) {
		log.Fatalf("Another AI Flight Dashboard process is already using %s. Stop it before starting a second dashboard or running a repair/import/dedup command.", dataDir)
	}
	log.Fatalf("Failed to acquire process lock %s: %v", lockPath, err)
	return nil
}

func runExport(deviceID string) {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	filter := ""
	if deviceID != "local" && deviceID != "" {
		filter = deviceID
	}

	count, err := database.ExportCSV(os.Stdout, filter)
	if err != nil {
		log.Fatalf("Export failed: %v", err)
	}
	fmt.Fprintf(os.Stderr, "✅ Exported %d records", count)
	if filter != "" {
		fmt.Fprintf(os.Stderr, " (device=%s)", filter)
	}
	fmt.Fprintln(os.Stderr)
}

func runImport(filePath string) {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", filePath, err)
	}
	defer file.Close()

	imported, skipped, err := database.ImportCSV(file)
	if err != nil {
		log.Fatalf("Import failed: %v", err)
	}
	fmt.Printf("✅ Import complete: %d imported, %d skipped (duplicates)\n", imported, skipped)
}

func runDedup() {
	database, lock := openDB()
	defer lock.Release()
	defer database.Close()

	removed, err := database.DeduplicateExisting()
	if err != nil {
		log.Fatalf("Dedup failed: %v", err)
	}
	fmt.Printf("✅ Removed %d duplicate records\n", removed)
}

func runRepairHistory(database *db.DB, calc *calculator.Calculator, deviceID string, scanDirs []string) {
	geminiFiles, err := discoverGeminiHistoryFiles(scanDirs)
	if err != nil {
		log.Fatalf("Failed to discover Gemini history files: %v", err)
	}

	var resetFiles int64
	for _, filePath := range geminiFiles {
		n, err := database.ResetOffset(filePath)
		if err != nil {
			log.Fatalf("Failed to reset Gemini offset for %q: %v", filePath, err)
		}
		resetFiles += n
	}
	for _, pattern := range []string{
		"%/.claude/projects/%",
		"%\\.claude\\projects\\%",
	} {
		n, err := database.ResetOffsetsLike(pattern)
		if err != nil {
			log.Fatalf("Failed to reset Claude offsets for %q: %v", pattern, err)
		}
		resetFiles += n
	}
	if err := database.SetOffset(codexusage.OffsetKey, 0); err != nil {
		log.Fatalf("Failed to reset Codex offset: %v", err)
	}

	s := scanner.New(database, calc, deviceID)
	scanned, err := s.ScanAllStrict(scanDirs, nil)
	if err != nil {
		log.Fatalf("History repair scan failed: %v", err)
	}
	codexScanner := codexusage.New(database, calc, deviceID)
	codexScanned, err := codexScanner.Scan(nil)
	if err != nil {
		log.Fatalf("Codex repair scan failed: %v", err)
	}

	supersededGemini, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", geminiFiles, localRepairDeviceIDs(deviceID))
	if err != nil {
		log.Fatalf("Failed to supersede replayed Gemini rows: %v", err)
	}

	fmt.Printf("✅ History repair complete: superseded %d old Gemini rows, reset %d file offsets, replayed %d JSONL records and %d Codex events\n", supersededGemini, resetFiles, scanned, codexScanned)
}

func discoverGeminiHistoryFiles(scanDirs []string) ([]string, error) {
	return discoverHistoryFiles(scanDirs, func(path string) bool {
		if !isGeminiHistoryFile(path) {
			return false
		}
		ok, err := fileHasUsageSource(path, "Gemini CLI")
		return err == nil && ok
	})
}

func discoverHistoryFiles(scanDirs []string, match func(string) bool) ([]string, error) {
	seen := make(map[string]struct{})
	for _, dir := range scanDirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !match(path) {
				return nil
			}
			seen[path] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func isGeminiHistoryFile(path string) bool {
	if !strings.HasSuffix(path, ".jsonl") {
		return false
	}
	return strings.Contains(filepath.ToSlash(path), "/.gemini/tmp/")
}

func fileHasUsageSource(path string, source string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if u, ok := watcher.ParseLine(line); ok && u.Source == source {
				return true, nil
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func localRepairDeviceIDs(deviceID string) []string {
	ids := []string{deviceID, "local", ""}
	seen := make(map[string]struct{}, len(ids))
	unique := ids[:0]
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
