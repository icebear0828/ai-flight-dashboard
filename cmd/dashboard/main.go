package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"syscall"
	"time"
	goruntime "runtime"

	"ai-flight-dashboard/internal/alert"
	"ai-flight-dashboard/internal/calculator"
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

	// Subcommand dispatch: export | import | dedup
	if args := flag.Args(); len(args) > 0 {
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

	// Initialize Calculator from embedded pricing table
	calc, err := calculator.NewFromBytes(embeddedPricing)
	if err != nil {
		log.Fatalf("Failed to initialize calculator: %v", err)
	}
	
	home, _ := os.UserHomeDir()
	customPricingPath := filepath.Join(home, ".ai-flight-dashboard", "custom_pricing.json")
	if err := calc.LoadCustomPrices(customPricingPath); err != nil {
		fmt.Printf("⚠️  Failed to load custom pricing: %v\n", err)
	}

	// Initialize Database
	if *dataDir != "" {
		config.SetDataDir(*dataDir)
	}
	appDataDir := config.GetDataDir()
	statsDir := filepath.Join(appDataDir, "stats")
	os.MkdirAll(statsDir, 0755)
	
	dbPath := filepath.Join(statsDir, "usage.db")
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

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

	// Initialize Watcher immediately (instant, ~26µs)
	w, err := watcher.New(*deviceID)
	if err != nil {
		log.Fatalf("Failed to initialize watcher: %v", err)
	}
	defer w.Close()

	var lanInst *lan.LAN
	if *lanMode && appConfig.EnableLAN != nil && *appConfig.EnableLAN {
		lanInst = lan.New(*deviceID)
		fmt.Printf("📡 LAN discovery enabled. Multicast: %s\n", lan.MulticastAddr)
		go lanInst.StartBroadcaster(w.BroadcastChan)
		go lanInst.StartListener(w.UsageChan)
		go lanInst.StartPinger()
	} else {
		fmt.Println("📡 LAN discovery is disabled in settings.")
	}

	// Fast path: register cached known directories (~1ms vs 134ms recursive)
	knownDirs, _ := database.ListKnownDirs()
	if *syncMode == "fsnotify" && len(knownDirs) > 0 {
		w.WatchKnownDirs(knownDirs)
		// Also watch project roots for new session dirs
		for _, dir := range scanDirs {
			w.WatchDir(dir)
		}
	}

	// Background: full scan + discover new directories
	go func() {
		s := scanner.New(database, calc, *deviceID)
		s.ScanAll(scanDirs, w.UsageChan) // incremental; auto-caches new dirs to known_dirs

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
					
					// If it's a remote packet from LAN, ALWAYS save it regardless of syncMode
					// If it's a local packet, ONLY save it if fsnotify (poll mode saves it directly in ScanAll)
					if !isLocal || *syncMode == "fsnotify" {
						cost, _ := calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
						
						effectiveDevice := u.DeviceID
						if effectiveDevice == "" {
							effectiveDevice = *deviceID
						}
						
						database.InsertUsage(u, cost, effectiveDevice)
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

		// Reuse the existing HTTP handler to serve /api/* routes inside Wails
		apiHandler := web.NewHandler(database, calc, w, lanInst, *token, root.DistBinFS)

		err = wailsrun.Run(&options.App{
			Title:     "AI Flight Dashboard",
			Width:     1280,
			Height:    800,
			MinWidth:  900,
			MinHeight: 600,
			SingleInstanceLock: &options.SingleInstanceLock{
				UniqueId:               "ai-flight-dashboard",
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

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			fmt.Println("\n🛬 Shutting down gracefully...")
			srv.Shutdown(context.Background())
		}()

		fmt.Printf("🌐 Web dashboard: http://localhost:%s\n", *port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
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

	skipDBWrite := (*syncMode != "fsnotify")
	p := tea.NewProgram(tui.NewModel(calc, w, database, budgetAlert, skipDBWrite))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func openDB() *db.DB {
	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".ai-flight-dashboard", "stats", "usage.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	return database
}

func runExport(deviceID string) {
	database := openDB()
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
	database := openDB()
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
	database := openDB()
	defer database.Close()

	removed, err := database.DeduplicateExisting()
	if err != nil {
		log.Fatalf("Dedup failed: %v", err)
	}
	fmt.Printf("✅ Removed %d duplicate records\n", removed)
}

