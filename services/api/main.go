package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	redisv8 "github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/npdms/api/internal/config"
	"github.com/npdms/api/internal/database"
	"github.com/npdms/api/internal/handlers"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/repository"
	"github.com/npdms/api/internal/services"
	"github.com/npdms/api/internal/storage"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// sqlx handle for repositories that use sqlx instead of pgx
	sqlxDB, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to open sqlx connection: %v", err)
	}
	defer sqlxDB.Close()

	// Initialize Redis
	rdb := database.ConnectRedis(cfg.RedisURL)
	defer rdb.Close()

	// go-redis v8 client for middleware that still depends on the v8 API
	rdbV8Opt, err := redisv8.ParseURL(cfg.RedisURL)
	if err != nil {
		rdbV8Opt = &redisv8.Options{Addr: "localhost:6379"}
	}
	rdbV8 := redisv8.NewClient(rdbV8Opt)
	defer rdbV8.Close()

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	firRepo := repository.NewFIRRepository(db)
	caseRepo := repository.NewCaseRepository(db)
	evidenceRepo := repository.NewEvidenceRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	warrantRepo := repository.NewWarrantRepository(db)
	bailRepo := repository.NewBailRepository(db)
	forensicRepo := repository.NewForensicRepository(db)
	personnelRepo := repository.NewPersonnelRepository(db)
	vehicleRepo := repository.NewVehicleRepository(db)
	courtRepo := repository.NewCourtRepository(db)
	alertRepo := repository.NewAlertRepository(db)
	cyberCrimeRepo := repository.NewCyberCrimeRepository(sqlxDB)
	graphRepo := repository.NewGraphRepository(sqlxDB)
	citizenPortalRepo := repository.NewCitizenPortalRepository(sqlxDB)
	trafficChallanRepo := repository.NewTrafficChallanRepository(db)
	reportsRepo := repository.NewReportsRepository(db)
	investigationRepo := repository.NewInvestigationRepository(db)
	custodyRepo := repository.NewCustodyRepository(db)
		districtRepo := repository.NewDistrictRepository(db)
	stateRepo := repository.NewStateRepository(db)
	nationalRepo := repository.NewNationalRepository(db)
	uploadRepo := repository.NewUploadRepository(db)
	biometricRepo := repository.NewBiometricRepository(db)

	// Initialize services
	authService := services.NewAuthService(userRepo, rdb, cfg.JWTSecret)
	firService := services.NewFIRService(firRepo, auditRepo)
	caseService := services.NewCaseService(caseRepo, auditRepo)
	evidenceService := services.NewEvidenceService(evidenceRepo, auditRepo)
	warrantService := services.NewWarrantService(warrantRepo, auditRepo)
	bailService := services.NewBailService(bailRepo, auditRepo)
	forensicService := services.NewForensicService(forensicRepo, auditRepo)
	personnelService := services.NewPersonnelService(personnelRepo, auditRepo)
	vehicleService := services.NewVehicleService(vehicleRepo, auditRepo)
	courtService := services.NewCourtService(courtRepo, auditRepo)
	alertService := services.NewAlertService(alertRepo, auditRepo)
	mlService := services.NewMLService(firRepo, auditRepo)
	cyberCrimeService := services.NewCyberCrimeService(cyberCrimeRepo, auditRepo)
	graphService := services.NewGraphService(graphRepo, auditRepo)
	citizenPortalService := services.NewCitizenPortalService(citizenPortalRepo, firRepo, auditRepo)
	trafficChallanService := services.NewTrafficChallanService(trafficChallanRepo, auditRepo)
	reportsService := services.NewReportsService(reportsRepo, auditRepo)
	aiReviewService := services.NewAIReviewService(db, auditRepo)
	investigationService := services.NewInvestigationService(investigationRepo, auditRepo, db)
	ipIntelService := services.NewIPIntelService(rdb, auditRepo)

	// Evidence storage. Filesystem by default so a single edge server needs no
	// extra service; set STORAGE_BACKEND=minio for S3-compatible storage.
	storageCfg := storage.FromEnv()
	evidenceStore, err := storage.Open(storageCfg)
	if err != nil {
		log.Fatalf("Failed to open evidence storage (%s): %v", storageCfg.Backend, err)
	}
	log.Printf("Evidence storage: %s", evidenceStore.Backend())

	// Custody attestations get their own key where one is provided, so rotating
	// the JWT secret does not invalidate historical signatures.
	custodySigningKey := os.Getenv("CUSTODY_SIGNING_KEY")
	if custodySigningKey == "" {
		custodySigningKey = cfg.JWTSecret
	}
	custodyService := services.NewCustodyService(custodyRepo, evidenceStore, auditRepo, custodySigningKey)
	districtService := services.NewDistrictService(districtRepo, auditRepo)
	stateService := services.NewStateService(stateRepo, auditRepo)
	nationalService := services.NewNationalService(nationalRepo, auditRepo)

	// Biometric service with ML integration
	mlServiceURL := os.Getenv("ML_BIOMETRIC_URL")
	if mlServiceURL == "" {
		mlServiceURL = "http://localhost:8006"
	}
	aadhaarURL := os.Getenv("UIDAI_URL")
	if aadhaarURL == "" {
		aadhaarURL = "https://api.uidai.gov.in"
	}
	biometricService := services.NewBiometricService(db, biometricRepo, auditRepo, mlServiceURL, aadhaarURL)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	firHandler := handlers.NewFIRHandler(firService)
	caseHandler := handlers.NewCaseHandler(caseService)
	evidenceHandler := handlers.NewEvidenceHandler(evidenceService)
	warrantHandler := handlers.NewWarrantHandler(warrantService)
	bailHandler := handlers.NewBailHandler(bailService)
	forensicHandler := handlers.NewForensicHandler(forensicService)
	personnelHandler := handlers.NewPersonnelHandler(personnelService)
	vehicleHandler := handlers.NewVehicleHandler(vehicleService)
	courtHandler := handlers.NewCourtHandler(courtService)
	alertHandler := handlers.NewAlertHandler(alertService)
	mlHandler := handlers.NewMLHandler(mlService)
	healthHandler := handlers.NewHealthHandler(db, rdb)
	cyberCrimeHandler := handlers.NewCyberCrimeHandler(cyberCrimeService)
	graphHandler := handlers.NewGraphHandler(graphService)
	citizenPortalHandler := handlers.NewCitizenPortalHandler(citizenPortalService)
	trafficChallanHandler := handlers.NewTrafficChallanHandler(trafficChallanService)
	reportsHandler := handlers.NewReportsHandler(reportsService)
	aiReviewHandler := handlers.NewAIReviewHandler(aiReviewService)
	investigationHandler := handlers.NewInvestigationHandler(investigationService)
	ipIntelHandler := handlers.NewIPIntelHandler(ipIntelService)
	custodyHandler := handlers.NewCustodyHandler(custodyService)
	districtHandler := handlers.NewDistrictHandler(districtService)
	stateHandler := handlers.NewStateHandler(stateService)
	nationalHandler := handlers.NewNationalHandler(nationalService)
	biometricHandler := handlers.NewBiometricHandler(biometricService)

	// Initialize upload handler (MinIO)
	uploadHandler, err := handlers.NewUploadHandler(
		cfg.MinioEndpoint,
		cfg.MinioAccessKey,
		cfg.MinioSecretKey,
		cfg.MinioBucket,
		cfg.MinioPublicURL,
		cfg.MinioUseSSL,
		uploadRepo,
	)
	if err != nil {
		log.Printf("Warning: Failed to initialize MinIO upload handler: %v", err)
		// Continue without MinIO - uploads will fail gracefully
	}

	// Initialize ML handlers
	ocrServiceURL := os.Getenv("ML_OCR_URL")
	if ocrServiceURL == "" {
		ocrServiceURL = "http://localhost:8004"
	}
	ocrHandler := handlers.NewOCRHandler(ocrServiceURL, auditRepo)

	transcriptionServiceURL := os.Getenv("ML_TRANSCRIPTION_URL")
	if transcriptionServiceURL == "" {
		transcriptionServiceURL = "http://localhost:8005"
	}
	transcriptionHandler := handlers.NewTranscriptionHandler(transcriptionServiceURL, auditRepo)

	// Setup Gin router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Zero Trust Configuration
	zeroTrustConfig := middleware.DefaultZeroTrustConfig()

	// Global middleware
	router.Use(middleware.CORS())
	router.Use(middleware.SecurityHeadersMiddleware())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.GlobalRateLimiter(rdbV8))
	router.Use(middleware.ZeroTrustMiddleware(rdbV8, zeroTrustConfig))
	router.Use(middleware.AuditSecurityEventMiddleware(rdbV8))

	// The API contract, served by the build it describes.
	openAPIHandler := handlers.NewOpenAPIHandler()
	router.GET("/openapi.yaml", openAPIHandler.Serve)

	// Health check
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Auth routes (public)
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/logout", authHandler.Logout)
		}

		// Protected routes
		protected := v1.Group("")
		protected.Use(middleware.AuthMiddleware(cfg.JWTSecret))
		protected.Use(middleware.SessionValidationMiddleware(rdbV8, zeroTrustConfig))
		protected.Use(middleware.DeviceVerificationMiddleware(rdbV8, zeroTrustConfig))
		protected.Use(middleware.ContinuousAuthMiddleware(rdbV8))
		{
			// User routes
			protected.GET("/me", authHandler.GetCurrentUser)
			protected.PUT("/me", authHandler.UpdateProfile)
			protected.PUT("/me/password", authHandler.ChangePassword)

			// FIR routes
			firs := protected.Group("/firs")
			{
				firs.GET("", firHandler.List)
				firs.GET("/:id", firHandler.Get)
				firs.POST("", middleware.RequireRole("SI", "INSPECTOR", "SHO"), firHandler.Create)
				firs.PUT("/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), firHandler.Update)
				firs.PATCH("/:id/status", middleware.RequireRole("SI", "INSPECTOR", "SHO"), firHandler.UpdateStatus)
				firs.GET("/:id/timeline", firHandler.GetTimeline)
			}

			// Case routes
			cases := protected.Group("/cases")
			{
				cases.GET("", caseHandler.List)
				cases.GET("/:id", caseHandler.Get)
				cases.POST("", middleware.RequireRole("SI", "INSPECTOR", "SHO"), caseHandler.Create)
				cases.PUT("/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), caseHandler.Update)
				cases.GET("/:id/accused", caseHandler.GetAccused)
				cases.POST("/:id/accused", middleware.RequireRole("SI", "INSPECTOR", "SHO"), caseHandler.AddAccused)
				cases.GET("/:id/witnesses", caseHandler.GetWitnesses)
				cases.POST("/:id/witnesses", middleware.RequireRole("SI", "INSPECTOR", "SHO"), caseHandler.AddWitness)
			}

			// Evidence routes
			evidence := protected.Group("/evidence")
			{
				evidence.GET("", evidenceHandler.List)
				evidence.GET("/:id", evidenceHandler.Get)
				evidence.POST("", evidenceHandler.Create)
				evidence.PUT("/:id", evidenceHandler.Update)
				evidence.GET("/:id/custody", evidenceHandler.GetChainOfCustody)
				evidence.POST("/:id/transfer", evidenceHandler.Transfer)
			}

			// Warrant routes
			warrants := protected.Group("/warrants")
			{
				warrants.GET("", warrantHandler.List)
				warrants.GET("/:id", warrantHandler.Get)
				warrants.POST("", middleware.RequireRole("SI", "INSPECTOR", "SHO"), warrantHandler.Create)
				warrants.PUT("/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), warrantHandler.Update)
				warrants.PATCH("/:id/status", middleware.RequireRole("SI", "INSPECTOR", "SHO"), warrantHandler.UpdateStatus)
				warrants.GET("/stats", warrantHandler.GetStats)
			}

			// Bail routes
			bail := protected.Group("/bail")
			{
				bail.GET("", bailHandler.List)
				bail.GET("/:id", bailHandler.Get)
				bail.POST("", middleware.RequireRole("SI", "INSPECTOR", "SHO"), bailHandler.Create)
				bail.PUT("/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), bailHandler.Update)
				bail.PATCH("/:id/status", middleware.RequireRole("SI", "INSPECTOR", "SHO"), bailHandler.UpdateStatus)
				bail.GET("/stats", bailHandler.GetStats)
			}

			// Forensic routes
			forensics := protected.Group("/forensics")
			{
				forensics.GET("", forensicHandler.List)
				forensics.GET("/:id", forensicHandler.Get)
				forensics.POST("", forensicHandler.Create)
				forensics.PUT("/:id", forensicHandler.Update)
				forensics.POST("/:id/complete", forensicHandler.CompleteRequest)
				forensics.GET("/stats", forensicHandler.GetStats)
			}

			// Personnel routes
			personnel := protected.Group("/personnel")
			{
				personnel.GET("", personnelHandler.List)
				personnel.GET("/:id", personnelHandler.Get)
				personnel.POST("", middleware.RequireRole("SHO", "DSP", "SP"), personnelHandler.Create)
				personnel.PUT("/:id", middleware.RequireRole("SHO", "DSP", "SP"), personnelHandler.Update)
				personnel.POST("/:id/assign-duty", middleware.RequireRole("SHO", "DSP", "SP"), personnelHandler.AssignDuty)
				personnel.DELETE("/:id", middleware.RequireRole("DSP", "SP"), personnelHandler.Delete)
			}

			// Vehicle routes
			vehicles := protected.Group("/vehicles")
			{
				vehicles.GET("", vehicleHandler.List)
				vehicles.GET("/:id", vehicleHandler.Get)
				vehicles.POST("", middleware.RequireRole("SHO", "DSP", "SP"), vehicleHandler.Create)
				vehicles.PUT("/:id", middleware.RequireRole("SHO", "DSP", "SP"), vehicleHandler.Update)
				vehicles.POST("/:id/allocate", middleware.RequireRole("SHO", "DSP", "SP"), vehicleHandler.AllocateVehicle)
				vehicles.POST("/:id/return", vehicleHandler.ReturnVehicle)
				vehicles.DELETE("/:id", middleware.RequireRole("DSP", "SP"), vehicleHandler.Delete)
			}

			// Court routes
			court := protected.Group("/court")
			{
				// Hearing routes
				court.GET("/hearings", courtHandler.ListHearings)
				court.GET("/hearings/:id", courtHandler.GetHearing)
				court.POST("/hearings", middleware.RequireRole("SI", "INSPECTOR", "SHO"), courtHandler.CreateHearing)
				court.PUT("/hearings/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), courtHandler.UpdateHearing)

				// Order routes
				court.GET("/orders", courtHandler.ListOrders)
				court.GET("/orders/:id", courtHandler.GetOrder)
				court.POST("/orders", middleware.RequireRole("SI", "INSPECTOR", "SHO"), courtHandler.CreateOrder)

				// Stats
				court.GET("/stats", courtHandler.GetStats)
			}

			// Alert routes
			alerts := protected.Group("/alerts")
			{
				alerts.GET("", alertHandler.List)
				alerts.GET("/:id", alertHandler.Get)
				alerts.POST("", middleware.RequireRole("SHO", "DSP", "SP"), alertHandler.Create)
				alerts.PUT("/:id", middleware.RequireRole("SHO", "DSP", "SP"), alertHandler.Update)
				alerts.POST("/:id/acknowledge", alertHandler.Acknowledge)
				alerts.DELETE("/:id", middleware.RequireRole("SHO", "DSP", "SP"), alertHandler.Delete)
				alerts.GET("/active", alertHandler.GetActiveAlerts)
				alerts.GET("/unacknowledged", alertHandler.GetUnacknowledgedAlerts)
			}

			// ML routes (AI/ML features)
			ml := protected.Group("/ml")
			{
				ml.GET("/health", mlHandler.HealthCheck)
				ml.POST("/classify", mlHandler.ClassifyText)
				ml.POST("/search", mlHandler.SearchSimilar)
				ml.POST("/ocr", mlHandler.ExtractText)
				ml.GET("/predictions", mlHandler.GetPredictions)
				ml.GET("/hotspots", mlHandler.GetHotspots)
			}

			// Stats & Dashboard (with database context)
			protected.GET("/dashboard/stats", func(c *gin.Context) {
				c.Set("db", db)
				handlers.GetDashboardStats(c)
			})

			// Audit logs (DSP+ only)
			audit := protected.Group("/audit")
			audit.Use(middleware.RequireRole("DSP", "SP", "DIG", "IG", "DGP"))
			{
				audit.GET("/logs", func(c *gin.Context) {
					c.Set("db", db)
					handlers.GetAuditLogs(c)
				})
			}

			// Cyber Crime routes
			cyberCrime := protected.Group("/cyber-crime")
			{
				cyberCrime.GET("", cyberCrimeHandler.List)
				cyberCrime.GET("/:id", cyberCrimeHandler.Get)
				cyberCrime.POST("", middleware.RequireRole("SI", "INSPECTOR", "SHO"), cyberCrimeHandler.Create)
				cyberCrime.PUT("/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), cyberCrimeHandler.Update)
				cyberCrime.PATCH("/:id/status", middleware.RequireRole("SI", "INSPECTOR", "SHO"), cyberCrimeHandler.UpdateStatus)
				cyberCrime.GET("/stats", cyberCrimeHandler.GetStats)
				cyberCrime.POST("/:id/evidence", cyberCrimeHandler.AddDigitalEvidence)
				cyberCrime.GET("/:id/evidence", cyberCrimeHandler.GetDigitalEvidence)
				cyberCrime.POST("/:id/financial-trail", cyberCrimeHandler.AddFinancialTrail)
				cyberCrime.GET("/:id/financial-trails", cyberCrimeHandler.GetFinancialTrails)
				cyberCrime.POST("/:id/osint", cyberCrimeHandler.AddOSINTReport)
				cyberCrime.GET("/:id/osint", cyberCrimeHandler.GetOSINTReports)
			}

			// Graph Intelligence routes
			graph := protected.Group("/graph")
			{
				// Nodes
				graph.GET("/nodes", graphHandler.SearchNodes)
				graph.GET("/nodes/:id", graphHandler.GetNode)
				graph.POST("/nodes", middleware.RequireRole("SI", "INSPECTOR", "SHO"), graphHandler.CreateNode)
				graph.PUT("/nodes/:id", middleware.RequireRole("SI", "INSPECTOR", "SHO"), graphHandler.UpdateNode)
				graph.GET("/nodes/:id/connections", graphHandler.GetConnections)

				// Edges
				graph.POST("/edges", middleware.RequireRole("SI", "INSPECTOR", "SHO"), graphHandler.CreateEdge)

				// Networks
				graph.GET("/networks", graphHandler.ListNetworks)
				graph.GET("/networks/:id", graphHandler.GetNetwork)
				graph.POST("/networks", middleware.RequireRole("SHO", "DSP", "SP"), graphHandler.CreateNetwork)
				graph.POST("/networks/:id/members", middleware.RequireRole("SI", "INSPECTOR", "SHO"), graphHandler.AddNetworkMember)
				graph.GET("/networks/:id/members", graphHandler.GetNetworkMembers)

				// Analytics
				graph.GET("/stats", graphHandler.GetStatistics)
				graph.GET("/visualization", graphHandler.GetVisualizationData)
			}

			// Citizen Portal (protected routes for officers)
			citizenAdmin := protected.Group("/citizen-portal")
			{
				citizenAdmin.GET("/complaints", citizenPortalHandler.ListComplaints)
				citizenAdmin.GET("/complaints/:id", citizenPortalHandler.GetComplaint)
				citizenAdmin.PUT("/complaints/:id", citizenPortalHandler.UpdateComplaint)
				citizenAdmin.POST("/complaints/:id/acknowledge", citizenPortalHandler.AcknowledgeComplaint)
				citizenAdmin.POST("/complaints/:id/assign", middleware.RequireRole("SHO", "DSP", "SP"), citizenPortalHandler.AssignComplaint)
				citizenAdmin.POST("/complaints/:id/resolve", citizenPortalHandler.ResolveComplaint)
				citizenAdmin.POST("/complaints/:id/reject", citizenPortalHandler.RejectComplaint)
				citizenAdmin.POST("/complaints/:id/convert-to-fir", middleware.RequireRole("SI", "INSPECTOR", "SHO"), citizenPortalHandler.ConvertToFIR)
				citizenAdmin.POST("/complaints/:id/updates", citizenPortalHandler.AddComplaintUpdate)
				citizenAdmin.GET("/complaints/:id/updates", citizenPortalHandler.GetComplaintUpdates)
			}

			// Traffic Challan routes
			traffic := protected.Group("/traffic")
			{
				traffic.GET("/violation-types", trafficChallanHandler.GetViolationTypes)
				traffic.GET("/challans", trafficChallanHandler.List)
				traffic.GET("/challans/:id", trafficChallanHandler.Get)
				traffic.GET("/challans/number/:number", trafficChallanHandler.GetByNumber)
				traffic.POST("/challans", middleware.RequireRole("CONSTABLE", "HC", "SI", "INSPECTOR", "SHO"), trafficChallanHandler.Create)
				traffic.PATCH("/challans/:id/status", middleware.RequireRole("SI", "INSPECTOR", "SHO"), trafficChallanHandler.UpdateStatus)
				traffic.POST("/challans/:id/dispute", trafficChallanHandler.FileDispute)
				traffic.GET("/vehicle/:vehicleNumber", trafficChallanHandler.GetChallansByVehicle)
				traffic.GET("/defaulters", trafficChallanHandler.GetDefaulters)
				traffic.GET("/hotspots", trafficChallanHandler.GetHotspots)
				traffic.GET("/stats", trafficChallanHandler.GetStats)
				traffic.POST("/payments/initiate", trafficChallanHandler.InitiatePayment)
				traffic.POST("/payments/callback", trafficChallanHandler.PaymentCallback)
			}

			// Reports & Exports routes
			reports := protected.Group("/reports")
			{
				reports.GET("/types", reportsHandler.GetReportTypes)
				reports.GET("/daily-summary", reportsHandler.GetDailySummary)
				reports.GET("/fir-status", reportsHandler.GetFIRStatus)
				reports.GET("/pending-investigation", reportsHandler.GetPendingInvestigation)
				reports.GET("/crime-statistics", reportsHandler.GetCrimeStatistics)
				reports.GET("/officer-workload", reportsHandler.GetOfficerWorkload)
				reports.POST("/generate", reportsHandler.GenerateReport)
				reports.GET("/download/:type", reportsHandler.DownloadReport)
			}

			// OCR routes
			ocr := protected.Group("/ocr")
			{
				ocr.GET("/health", ocrHandler.HealthCheck)
				ocr.GET("/languages", ocrHandler.GetLanguages)
				ocr.GET("/document-types", ocrHandler.GetDocumentTypes)
				ocr.POST("/extract", ocrHandler.ExtractText)
				ocr.POST("/extract-batch", ocrHandler.ExtractBatch)
				ocr.POST("/classify", ocrHandler.ClassifyDocument)
				ocr.POST("/extract-entities", ocrHandler.ExtractEntities)
				ocr.POST("/enhance", ocrHandler.EnhanceImage)
			}

			// Voice Transcription routes
			transcription := protected.Group("/transcription")
			{
				transcription.GET("/health", transcriptionHandler.HealthCheck)
				transcription.GET("/languages", transcriptionHandler.GetLanguages)
				transcription.POST("/transcribe", transcriptionHandler.Transcribe)
				transcription.POST("/transcribe-fir", transcriptionHandler.TranscribeForFIR)
				transcription.POST("/extract-entities", transcriptionHandler.ExtractEntities)
				transcription.POST("/suggest-ipc", transcriptionHandler.SuggestIPCSections)
			}

			// File Upload routes (MinIO)
			if uploadHandler != nil {
				upload := protected.Group("/upload")
				{
					upload.POST("", uploadHandler.Upload)
					upload.POST("/presigned", uploadHandler.GetPresignedURL)
				}

				files := protected.Group("/files")
				{
					files.GET("/*key", uploadHandler.GetFile)
					files.DELETE("/*key", middleware.RequireRole("SI", "INSPECTOR", "SHO", "DSP", "SP"), uploadHandler.DeleteFile)
				}
			}

			// AI Review (Human-in-Loop) routes
			// Server-side intelligence lookups. These run here rather than in the
			// browser so egress is controlled, results are audited, and an
			// air-gapped deployment can disable them centrally.
			intel := protected.Group("/intel")
			{
				intel.GET("/ip/:ip", ipIntelHandler.Lookup)
			}

			// Phase 02 — Evidence & Chain of Custody
			custody := protected.Group("/custody")
			{
				custody.GET("", custodyHandler.List)
				custody.GET("/stats", custodyHandler.Stats)
				custody.GET("/:id", custodyHandler.Get)

				custody.POST("/:id/file", custodyHandler.AttachFile)
				custody.GET("/:id/file", custodyHandler.Download)

				custody.POST("/:id/verify", custodyHandler.Verify)
				custody.GET("/:id/verifications", custodyHandler.IntegrityHistory)

				custody.GET("/:id/chain", custodyHandler.CustodyChain)
				custody.POST("/:id/transfer",
					middleware.RequireRole("ASI", "SI", "INSPECTOR", "SHO", "DSP", "SP", "DIG", "IG", "DGP"),
					custodyHandler.Transfer)

				custody.GET("/:id/access-log", custodyHandler.AccessLog)
				custody.GET("/:id/court-verification", custodyHandler.CourtVerification)
			}

			// Phase 01 — Investigation Copilot
			investigation := protected.Group("/investigation")
			{
				investigation.GET("", investigationHandler.List)
				investigation.POST("", investigationHandler.Create)
				// Registered before /:id so "officers" is not read as a workspace id.
				investigation.GET("/officers", investigationHandler.ListOfficers)
				investigation.GET("/:id", investigationHandler.Get)
				investigation.PUT("/:id", investigationHandler.Update)
				investigation.GET("/:id/brief", investigationHandler.Brief)

				investigation.GET("/:id/persons", investigationHandler.ListPersons)
				investigation.POST("/:id/persons", investigationHandler.CreatePerson)
				investigation.PATCH("/:id/persons/:personId", investigationHandler.UpdatePerson)
				investigation.DELETE("/:id/persons/:personId", investigationHandler.DeletePerson)

				investigation.GET("/:id/timeline", investigationHandler.ListTimeline)
				investigation.POST("/:id/timeline", investigationHandler.CreateWorkspaceTimelineEntry)
				investigation.POST("/:id/timeline/:entryId/review", investigationHandler.ReviewWorkspaceTimelineEntry)
				investigation.DELETE("/:id/timeline/:entryId", investigationHandler.DeleteWorkspaceTimelineEntry)

				investigation.GET("/:id/contradictions", investigationHandler.ListContradictions)
				investigation.POST("/:id/contradictions", investigationHandler.CreateContradiction)
				investigation.POST("/:id/contradictions/:contradictionId/review", investigationHandler.ReviewContradiction)

				investigation.GET("/:id/gaps", investigationHandler.ListGaps)
				investigation.POST("/:id/gaps", investigationHandler.CreateGap)
				investigation.POST("/:id/gaps/recompute", investigationHandler.RecomputeGaps)
				investigation.PATCH("/:id/gaps/:gapId", investigationHandler.UpdateGapStatus)

				investigation.GET("/:id/tasks", investigationHandler.ListTasks)
				investigation.POST("/:id/tasks", investigationHandler.CreateTask)
				investigation.PATCH("/:id/tasks/:taskId", investigationHandler.UpdateTask)
				investigation.DELETE("/:id/tasks/:taskId", investigationHandler.DeleteTask)

				investigation.GET("/:id/links", investigationHandler.LinkGraph)
				investigation.GET("/:id/evidence", investigationHandler.ListEvidence)
				investigation.POST("/:id/evidence", investigationHandler.LinkEvidence)
				investigation.DELETE("/:id/evidence/:evidenceId", investigationHandler.UnlinkEvidence)
			}

			aiReview := protected.Group("/ai-review")
			{
				// Review queue
				aiReview.GET("/queue", aiReviewHandler.GetReviewQueue)
				aiReview.GET("/my-assignments", aiReviewHandler.GetMyAssignments)
				aiReview.GET("/stats", aiReviewHandler.GetStats)
				aiReview.GET("/metrics", aiReviewHandler.GetPerformanceMetrics)

				// Decision management
				aiReview.GET("/decisions/:id", aiReviewHandler.GetDecision)
				aiReview.POST("/decisions/:id/review", aiReviewHandler.ReviewDecision)
				aiReview.POST("/decisions/:id/assign", middleware.RequireRole("SHO", "DSP", "SP"), aiReviewHandler.AssignDecision)
				aiReview.POST("/decisions/:id/feedback", aiReviewHandler.SubmitFeedback)
				aiReview.GET("/decisions/:id/history", aiReviewHandler.GetDecisionHistory)

				// Bulk operations
				aiReview.POST("/bulk-review", middleware.RequireRole("SHO", "DSP", "SP"), aiReviewHandler.BulkReview)

				// Model configuration (admin only)
				aiReview.GET("/models", middleware.RequireRole("DSP", "SP", "DIG", "IG", "DGP"), aiReviewHandler.GetModelConfigs)
				aiReview.GET("/models/:modelName", middleware.RequireRole("DSP", "SP", "DIG", "IG", "DGP"), aiReviewHandler.GetModelConfig)
				aiReview.PUT("/models/:modelName", middleware.RequireRole("SP", "DIG", "IG", "DGP"), aiReviewHandler.UpdateModelConfig)

				// Maintenance
				aiReview.POST("/expire", middleware.RequireRole("SP", "DIG", "IG", "DGP"), aiReviewHandler.ExpireDecisions)
			}

			// Biometric & Attendance routes
			biometric := protected.Group("/biometric")
			{
				// Device management
				biometric.GET("/devices", biometricHandler.GetDevices)
				biometric.GET("/devices/:id", biometricHandler.GetDevice)
				biometric.POST("/devices", middleware.RequireRole("SHO", "DSP", "SP"), biometricHandler.RegisterDevice)
				biometric.PATCH("/devices/:id/status", middleware.RequireRole("SHO", "DSP", "SP"), biometricHandler.UpdateDeviceStatus)
				biometric.POST("/devices/:id/heartbeat", biometricHandler.DeviceHeartbeat)

				// Template management
				biometric.POST("/templates", biometricHandler.CreateTemplate)

				// Verification
				biometric.POST("/verify", biometricHandler.Verify)
				biometric.POST("/verify/aadhaar", biometricHandler.VerifyAadhaar)
				biometric.GET("/verifications/:subjectId/history", biometricHandler.GetVerificationHistory)

				// Suspect identification
				biometric.POST("/identify", biometricHandler.IdentifySuspect)
				biometric.POST("/identifications/:id/confirm", biometricHandler.ConfirmIdentification)

				// Secure access
				biometric.POST("/evidence/:evidenceId/verify", biometricHandler.VerifyEvidenceAccess)
				biometric.POST("/weapons/:weaponId/verify", biometricHandler.VerifyWeaponAccess)

				// Statistics
				biometric.GET("/stats", biometricHandler.GetStats)
			}

			// District Management routes
			district := protected.Group("/district")
			{
				// Districts
				district.GET("", districtHandler.ListDistricts)
				district.GET("/:id", districtHandler.GetDistrict)
				district.POST("", middleware.RequireRole("SP", "DIG", "IG", "DGP"), districtHandler.CreateDistrict)
				district.PUT("/:id", middleware.RequireRole("SP", "DIG", "IG", "DGP"), districtHandler.UpdateDistrict)
				district.GET("/:id/dashboard", districtHandler.GetDistrictDashboard)
				district.GET("/:id/rankings", districtHandler.GetStationRankings)
				district.GET("/:id/hotspots", districtHandler.GetCrimeHotspots)
				district.GET("/:id/tasks", districtHandler.GetPendingTasks)

				// Stations
				district.GET("/stations", districtHandler.ListStations)
				district.GET("/stations/:id", districtHandler.GetStation)
				district.POST("/stations", middleware.RequireRole("SP", "DIG", "IG", "DGP"), districtHandler.CreateStation)
				district.PUT("/stations/:id", middleware.RequireRole("SP", "DIG", "IG", "DGP"), districtHandler.UpdateStation)

				// Cross-Station Coordination
				district.GET("/coordination/requests", districtHandler.ListCrossStationRequests)
				district.GET("/coordination/requests/:id", districtHandler.GetCrossStationRequest)
				district.POST("/coordination/requests", middleware.RequireRole("SHO", "DSP", "SP"), districtHandler.CreateCrossStationRequest)
				district.POST("/coordination/requests/:id/approve", middleware.RequireRole("DSP", "SP"), districtHandler.ApproveCrossStationRequest)
				district.POST("/coordination/requests/:id/reject", middleware.RequireRole("DSP", "SP"), districtHandler.RejectCrossStationRequest)
				district.POST("/coordination/requests/:id/complete", middleware.RequireRole("SHO", "DSP", "SP"), districtHandler.CompleteCrossStationRequest)

				// Meetings
				district.GET("/meetings", districtHandler.ListMeetings)
				district.GET("/meetings/:id", districtHandler.GetMeeting)
				district.POST("/meetings", middleware.RequireRole("DSP", "SP"), districtHandler.CreateMeeting)
				district.PUT("/meetings/:id", middleware.RequireRole("DSP", "SP"), districtHandler.UpdateMeeting)

				// Resource Allocation
				district.GET("/resources", districtHandler.ListResourceAllocations)
				district.GET("/resources/:id", districtHandler.GetResourceAllocation)
				district.POST("/resources", middleware.RequireRole("SHO", "DSP", "SP"), districtHandler.CreateResourceAllocation)
				district.POST("/resources/:id/approve", middleware.RequireRole("DSP", "SP"), districtHandler.ApproveResourceAllocation)
				district.POST("/resources/:id/allocate", middleware.RequireRole("DSP", "SP"), districtHandler.AllocateResource)
				district.POST("/resources/:id/return", districtHandler.ReturnResource)
			}

			// State Management routes
			state := protected.Group("/state")
			{
				// States
				state.GET("", stateHandler.ListStates)
				state.GET("/:id", stateHandler.GetState)
				state.GET("/code/:code", stateHandler.GetStateByCode)
				state.POST("", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.CreateState)
				state.PUT("/:id", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.UpdateState)
				state.GET("/:id/dashboard", stateHandler.GetStateDashboard)
				state.GET("/:id/metrics", stateHandler.GetPerformanceMetrics)
				state.GET("/:id/hotspots", stateHandler.GetStateCrimeHotspots)
				state.GET("/:id/resources", stateHandler.GetResourceOverview)

				// Zones
				state.GET("/zones", stateHandler.ListZones)
				state.GET("/zones/:id", stateHandler.GetZone)
				state.POST("/zones", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.CreateZone)
				state.PUT("/zones/:id", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.UpdateZone)

				// Ranges
				state.GET("/ranges", stateHandler.ListRanges)
				state.GET("/ranges/:id", stateHandler.GetRange)
				state.POST("/ranges", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.CreateRange)
				state.PUT("/ranges/:id", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.UpdateRange)

				// Inter-District Coordination
				state.GET("/coordination/requests", stateHandler.ListInterDistrictRequests)
				state.GET("/coordination/requests/:id", stateHandler.GetInterDistrictRequest)
				state.POST("/coordination/requests", middleware.RequireRole("SP", "DIG", "IG"), stateHandler.CreateInterDistrictRequest)
				state.POST("/coordination/requests/:id/approve", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.ApproveInterDistrictRequest)
				state.POST("/coordination/requests/:id/reject", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.RejectInterDistrictRequest)

				// State Alerts
				state.GET("/alerts", stateHandler.ListStateAlerts)
				state.GET("/alerts/:id", stateHandler.GetStateAlert)
				state.POST("/alerts", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.CreateStateAlert)
				state.PUT("/alerts/:id", middleware.RequireRole("DIG", "IG", "DGP"), stateHandler.UpdateStateAlert)
				state.POST("/alerts/:id/acknowledge", stateHandler.AcknowledgeStateAlert)
			}

			// National Command Center routes (DGP+ only)
			national := protected.Group("/national")
			national.Use(middleware.RequireRole("IG", "DGP"))
			{
				// Dashboard
				national.GET("/dashboard", nationalHandler.GetNationalDashboard)
				national.GET("/rankings", nationalHandler.GetStateRankings)
				national.GET("/metrics", nationalHandler.GetPerformanceMetrics)
				national.GET("/hotspots", nationalHandler.GetNationalCrimeHotspots)
				national.GET("/resources", nationalHandler.GetResourceOverview)
				national.POST("/compare", nationalHandler.CompareStates)

				// National Alerts
				national.GET("/alerts", nationalHandler.ListNationalAlerts)
				national.GET("/alerts/:id", nationalHandler.GetNationalAlert)
				national.POST("/alerts", nationalHandler.CreateNationalAlert)
				national.PUT("/alerts/:id", nationalHandler.UpdateNationalAlert)
				national.POST("/alerts/:id/acknowledge", nationalHandler.AcknowledgeNationalAlert)

				// Inter-State Coordination
				national.GET("/coordination/requests", nationalHandler.ListInterStateRequests)
				national.GET("/coordination/requests/:id", nationalHandler.GetInterStateRequest)
				national.POST("/coordination/requests", nationalHandler.CreateInterStateRequest)
				national.POST("/coordination/requests/:id/approve", nationalHandler.ApproveInterStateRequest)
				national.POST("/coordination/requests/:id/reject", nationalHandler.RejectInterStateRequest)

				// Crime Patterns
				national.GET("/patterns", nationalHandler.ListCrimePatterns)
				national.GET("/patterns/:id", nationalHandler.GetCrimePattern)
				national.POST("/patterns", nationalHandler.CreateCrimePattern)
				national.PUT("/patterns/:id", nationalHandler.UpdateCrimePattern)
			}
		}

		// Public Citizen Portal routes (no auth required)
		citizen := v1.Group("/public")
		{
			citizen.POST("/complaints", citizenPortalHandler.SubmitComplaint)
			citizen.GET("/complaints/:trackingNumber", citizenPortalHandler.TrackComplaint)
			citizen.GET("/fir-status", citizenPortalHandler.TrackFIR)
			citizen.POST("/grievances", citizenPortalHandler.SubmitGrievance)
			citizen.POST("/missing-persons", citizenPortalHandler.SubmitMissingPersonReport)
			citizen.GET("/missing-persons/:reportNumber", citizenPortalHandler.TrackMissingPersonReport)
			citizen.POST("/fir-copy-request", citizenPortalHandler.RequestFIRCopy)
			citizen.GET("/stats", citizenPortalHandler.GetPortalStats)
		}
	}

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
