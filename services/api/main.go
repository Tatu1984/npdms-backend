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
	"github.com/npdms/api/internal/ai"
	"github.com/npdms/api/internal/authz"
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
	armouryRepo := repository.NewArmouryRepository(db)
	lookoutRepo := repository.NewLookoutRepository(db)
	accessLogRepo := repository.NewAccessLogRepository(db)
	permissionRepo := repository.NewPermissionRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	activityRepo := repository.NewActivityRepository(db)
	workloadRepo := repository.NewWorkloadRepository(db)
	riskRepo := repository.NewRiskRepository(db)
	vehicleRepo := repository.NewVehicleRepository(db)
	courtRepo := repository.NewCourtRepository(db)
	alertRepo := repository.NewAlertRepository(db)
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
	graphService := services.NewGraphService(graphRepo, auditRepo)
	citizenPortalService := services.NewCitizenPortalService(citizenPortalRepo, firRepo, auditRepo)
	trafficChallanService := services.NewTrafficChallanService(trafficChallanRepo, auditRepo)
	reportsService := services.NewReportsService(reportsRepo, auditRepo)
	aiReviewService := services.NewAIReviewService(db, auditRepo)
	referralService := services.NewReferralService(db, auditRepo)
	// Officer accounts. Until this module every account in the platform was
	// inserted by hand; there was no way to appoint, transfer or retire one.
	userAdminService := services.NewUserAdminService(db, auditRepo)
	roleService := services.NewRoleService(roleRepo, permissionRepo, userRepo, auditRepo)

	// The AI gateway (layer A0): the one route from this platform to any
	// model. Clients are attached per registered model, from the service
	// address each registry entry names. A model with no address here is not
	// connected, and the screens say so rather than showing an empty result.
	aiGateway := ai.NewGateway(aiReviewService)
	for _, entry := range aiModelEndpoints(context.Background(), aiReviewService) {
		aiGateway.Register(entry.model, ai.NewHTTPModel(entry.env))
	}

	investigationService := services.NewInvestigationService(investigationRepo, auditRepo, db)
	ipIntelService := services.NewIPIntelService(rdb, auditRepo)

	// Evidence storage. Filesystem by default so a single edge server needs no
	// extra service; set STORAGE_BACKEND=minio for S3-compatible storage, or
	// STORAGE_BACKEND=database where disk does not persist (Vercel) — that
	// backend refuses objects over its cap (8 MB), so recordings need MinIO/S3.
	storageCfg := storage.FromEnv()
	evidenceStore, err := storage.Open(storageCfg, db)
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
	// The sign-in entry records where the sign-in came from, not merely its
	// address. Resolution is bounded and cached, and absent rather than guessed
	// where no provider is configured.
	authHandler := handlers.NewAuthHandler(authService, auditRepo, accessLogRepo).
		WithGeo(ipIntelService)
	armouryHandler := handlers.NewArmouryHandler(services.NewArmouryService(armouryRepo, auditRepo))
	lookoutService := services.NewLookoutService(lookoutRepo, auditRepo)
	lookoutHandler := handlers.NewLookoutHandler(lookoutService)
	missingPersonService := services.NewMissingPersonService(
		repository.NewMissingPersonRepository(db), lookoutService, auditRepo, evidenceStore)
	missingPersonHandler := handlers.NewMissingPersonHandler(missingPersonService)
	// Phase 03: stream credentials are encrypted with their own key where one
	// is provided; otherwise the JWT secret is the key material.
	cctvCredentialKey := os.Getenv("CCTV_CREDENTIAL_KEY")
	if cctvCredentialKey == "" {
		cctvCredentialKey = cfg.JWTSecret
	}
	videoHandler := handlers.NewVideoHandler(services.NewVideoService(repository.NewVideoRepository(db), auditRepo, cctvCredentialKey))
	knowledgeHandler := handlers.NewKnowledgeHandler(services.NewKnowledgeService(repository.NewKnowledgeRepository(db), evidenceStore, auditRepo))
	accessLogHandler := handlers.NewAccessLogHandler(accessLogRepo)
	auditLogHandler := handlers.NewAuditLogHandler(repository.NewAuditQueryRepository(db))
	searchHandler := handlers.NewRecordSearchHandler(repository.NewSearchRepository(db), auditRepo)
	legalHandler := handlers.NewLegalHandler(services.NewLegalService(repository.NewLegalRepository(db), repository.NewGazetteerRepository(db), auditRepo))
	workloadHandler := handlers.NewWorkloadHandler(services.NewWorkloadService(workloadRepo, auditRepo))
	// AI layer A4 — vehicle detection and number-plate reading. The detection
	// service runs on the edge server (services/ml/vehicle_detection). With no
	// address configured the module reports "service not connected"; the
	// watchlist, stored-read search and hit review still work.
	anprRepo := repository.NewANPRRepository(db)
	anprHandler := handlers.NewANPRHandler(services.NewANPRService(
		anprRepo, services.NewANPRClient(os.Getenv("ML_VEHICLE_DETECTION_URL")), evidenceStore, auditRepo))

	// Phase 06 — traffic incidents and accident reconstruction
	trafficIncidentHandler := handlers.NewTrafficIncidentHandler(
		services.NewTrafficIncidentService(repository.NewTrafficIncidentRepository(db), auditRepo).WithANPR(anprRepo))

	// Phase 07 — Dispatch. The escalation rules are applied on a timer as well
	// as on every read, so an unacknowledged unit escalates unattended.
	dispatchService := services.NewDispatchService(repository.NewDispatchRepository(db), auditRepo)
	dispatchHandler := handlers.NewDispatchHandler(dispatchService)
	dispatchCtx, stopDispatch := context.WithCancel(context.Background())
	defer stopDispatch()
	go dispatchService.RunEscalations(dispatchCtx, 30*time.Second)
	// Face recognition for missing persons. The model runs in a separate
	// on-premises service (FR_SERVICE_URL); where it is not configured every
	// face recognition screen says "not connected" and nothing runs.
	faceRecognitionService := services.NewFaceRecognitionService(repository.NewFaceRecognitionRepository(db),
		repository.NewMissingPersonRepository(db), evidenceStore, auditRepo, services.NewFRClientFromEnv())
	faceRecognitionHandler := handlers.NewFaceRecognitionHandler(faceRecognitionService)
	missingPersonService.OnPhotoAdded(faceRecognitionService.OnPhotoAdded)
	go faceRecognitionService.RunAutoEnrolment(dispatchCtx, 2*time.Minute)
	riskHandler := handlers.NewRiskHandler(services.NewRiskService(riskRepo, auditRepo))
	malkhanaHandler := handlers.NewMalkhanaHandler(services.NewMalkhanaService(repository.NewMalkhanaRepository(db), alertRepo, auditRepo))
	firHandler := handlers.NewFIRHandler(firService, db)
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
	cyberFraudHandler := handlers.NewCyberFraudHandler(services.NewCyberFraudService(repository.NewCyberFraudRepository(db), auditRepo))
	graphHandler := handlers.NewGraphHandler(graphService)
	citizenPortalHandler := handlers.NewCitizenPortalHandler(citizenPortalService)
	// Phase 09 — citizen complaint register
	complaintHandler := handlers.NewComplaintHandler(services.NewComplaintService(repository.NewComplaintRepository(db), auditRepo))
	trafficChallanHandler := handlers.NewTrafficChallanHandler(trafficChallanService)
	reportsHandler := handlers.NewReportsHandler(reportsService)
	aiReviewHandler := handlers.NewAIReviewHandler(aiReviewService)
	referralHandler := handlers.NewReferralHandler(referralService)
	userAdminHandler := handlers.NewUserAdminHandler(userAdminService)
	roleHandler := handlers.NewRoleHandler(roleService)
	activityHandler := handlers.NewActivityHandler(activityRepo, userRepo)
	aiGatewayHandler := handlers.NewAIGatewayHandler(aiGateway)
	investigationHandler := handlers.NewInvestigationHandler(investigationService)
	ipIntelHandler := handlers.NewIPIntelHandler(ipIntelService)
	custodyHandler := handlers.NewCustodyHandler(custodyService)
	caseFileHandler := handlers.NewCaseFileHandler(services.NewCaseFileService(
		repository.NewCaseFileRepository(db), custodyService, evidenceStore, auditRepo))
	// Phase 13: body-worn camera footage goes through the same evidence store and register.
	bodycamHandler := handlers.NewBodycamHandler(services.NewBodycamService(
		repository.NewBodycamRepository(db), custodyService, evidenceStore, auditRepo))
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
	// What the request knows about itself, for whoever writes its audit entry.
	// Registered here so signing in — which happens before authentication, by
	// definition — is covered, and again inside the protected group once the
	// caller's rank and posting are known.
	router.Use(middleware.AuditContext(nil))

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
		protected.Use(middleware.AuthMiddleware(cfg.JWTSecret, rdb))
		protected.Use(middleware.SessionValidationMiddleware(rdbV8, zeroTrustConfig))
		protected.Use(middleware.DeviceVerificationMiddleware(rdbV8, zeroTrustConfig))
		protected.Use(middleware.ContinuousAuthMiddleware(rdbV8))
		// What this officer may do, checked against the route catalogue. The
		// per-route RequireRole guards below stay as they are: they say the
		// same thing about rank that the seeded roles say about permissions,
		// and two checks that agree are cheaper than the risk of removing 248
		// of them by hand.
		protected.Use(middleware.RequirePermission(permissionRepo))
		// Again, now that the caller is known: the entry gains their rank and
		// posting, so it can be read years later without joining to a users
		// table whose rows have since been amended.
		protected.Use(middleware.AuditContext(nil))
		// Now that the officer is known, limit them rather than their station's
		// address. Twelve screens a minute, each costing about five requests.
		protected.Use(middleware.PerOfficerRateLimiter(rdbV8))
		{
			// User routes
			protected.GET("/me", authHandler.GetCurrentUser)
			protected.PUT("/me", authHandler.UpdateProfile)
			protected.PUT("/me/password", authHandler.ChangePassword)
			// What this officer may do, by name, so the web application can
			// hide a control rather than offer one the server will refuse.
			protected.GET("/me/permissions", roleHandler.MyPermissions)
			// An officer's own trail. Being able to see what is kept about you
			// is part of being told it is kept at all.
			protected.GET("/me/activity", activityHandler.Mine)

			// Where officers went in the platform, and for how long. Reading
			// is what misuse of a police system usually looks like, and
			// reading changes nothing, so the audit trail alone cannot show it.
			activity := protected.Group("/activity")
			{
				activity.POST("", activityHandler.Record)
				activity.POST("/roll-up", activityHandler.RollUp)
			}
			protected.GET("/officers/:id/activity", activityHandler.ForOfficer)

			// FIR routes
			firs := protected.Group("/firs")
			{
				firs.GET("", firHandler.List)
				firs.GET("/:id", firHandler.Get)
				firs.POST("", firHandler.Create)
				firs.PUT("/:id", firHandler.Update)
				firs.PATCH("/:id/status", firHandler.UpdateStatus)
				firs.GET("/:id/timeline", firHandler.GetTimeline)
			}

			// Case routes
			cases := protected.Group("/cases")
			{
				cases.GET("", caseHandler.List)
				cases.GET("/:id", caseHandler.Get)
				cases.POST("", caseHandler.Create)
				cases.PUT("/:id", caseHandler.Update)
				cases.GET("/:id/accused", caseHandler.GetAccused)
				cases.POST("/:id/accused", caseHandler.AddAccused)
				cases.GET("/:id/witnesses", caseHandler.GetWitnesses)
				cases.POST("/:id/witnesses", caseHandler.AddWitness)
			}

			// Evidence routes
			evidence := protected.Group("/evidence")
			{
				evidence.GET("", evidenceHandler.List)
				evidence.GET("/:id", evidenceHandler.Get)
				evidence.POST("", evidenceHandler.Create)
				evidence.PUT("/:id", evidenceHandler.Update)
				evidence.GET("/:id/custody", evidenceHandler.GetChainOfCustody)
				// No legacy transfer route: every custody movement goes through the
				// signed POST /custody/:id/transfer.
			}

			// Warrant routes
			warrants := protected.Group("/warrants")
			{
				warrants.GET("", warrantHandler.List)
				warrants.GET("/:id", warrantHandler.Get)
				warrants.POST("", warrantHandler.Create)
				warrants.PUT("/:id", warrantHandler.Update)
				warrants.PATCH("/:id/status", warrantHandler.UpdateStatus)
				warrants.GET("/stats", warrantHandler.GetStats)
			}

			// Bail routes
			bail := protected.Group("/bail")
			{
				bail.GET("", bailHandler.List)
				bail.GET("/:id", bailHandler.Get)
				bail.POST("", bailHandler.Create)
				bail.PUT("/:id", bailHandler.Update)
				bail.PATCH("/:id/status", bailHandler.UpdateStatus)
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
				personnel.POST("", personnelHandler.Create)
				personnel.PUT("/:id", personnelHandler.Update)
				personnel.POST("/:id/assign-duty", personnelHandler.AssignDuty)
				personnel.DELETE("/:id", personnelHandler.Delete)
			}

			// Vehicle routes
			vehicles := protected.Group("/vehicles")
			{
				vehicles.GET("", vehicleHandler.List)
				vehicles.GET("/:id", vehicleHandler.Get)
				vehicles.POST("", vehicleHandler.Create)
				vehicles.PUT("/:id", vehicleHandler.Update)
				vehicles.POST("/:id/allocate", vehicleHandler.AllocateVehicle)
				vehicles.POST("/:id/return", vehicleHandler.ReturnVehicle)
				vehicles.DELETE("/:id", vehicleHandler.Delete)
			}

			// Court routes
			court := protected.Group("/court")
			{
				// Hearing routes
				court.GET("/hearings", courtHandler.ListHearings)
				court.GET("/hearings/:id", courtHandler.GetHearing)
				court.POST("/hearings", courtHandler.CreateHearing)
				court.PUT("/hearings/:id", courtHandler.UpdateHearing)

				// Order routes
				court.GET("/orders", courtHandler.ListOrders)
				court.GET("/orders/:id", courtHandler.GetOrder)
				court.POST("/orders", courtHandler.CreateOrder)
				// A recorded order is corrected, never deleted: it is a record
				// of what a court directed. The database refuses a DELETE.
				court.PATCH("/orders/:id", courtHandler.UpdateOrder)
				court.POST("/orders/:id/compliance", courtHandler.RecordCompliance)

				// Stats
				court.GET("/stats", courtHandler.GetStats)
			}

			// Alert routes
			alerts := protected.Group("/alerts")
			{
				alerts.GET("", alertHandler.List)
				alerts.GET("/:id", alertHandler.Get)
				alerts.POST("", alertHandler.Create)
				alerts.PUT("/:id", alertHandler.Update)
				alerts.POST("/:id/acknowledge", alertHandler.Acknowledge)
				alerts.DELETE("/:id", alertHandler.Delete)
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
			}

			// Stats & Dashboard (with database context)
			protected.GET("/dashboard/stats", func(c *gin.Context) {
				c.Set("db", db)
				handlers.GetDashboardStats(c)
			})

			// Global record search and the station reference list (any officer).
			protected.GET("/search", searchHandler.Search)
			protected.GET("/stations", searchHandler.Stations)

			// Audit logs (DSP+ only)
			audit := protected.Group("/audit")
			{
				audit.GET("/logs", auditLogHandler.List)
				audit.GET("/stats", auditLogHandler.Stats)
				audit.GET("/verify", auditLogHandler.Verify)
			}

			// Phase 05 — Cybercrime & Financial Fraud (functional without AI)
			cyber := protected.Group("/cyber-crime")
			{
				cyber.GET("", cyberFraudHandler.List)
				cyber.GET("/dashboard", cyberFraudHandler.Dashboard)
				cyber.GET("/clusters", cyberFraudHandler.Clusters)
				cyber.GET("/entities", cyberFraudHandler.SearchEntities)
				cyber.GET("/:id", cyberFraudHandler.Get)
				cyber.POST("", cyberFraudHandler.Register)
				cyber.PUT("/:id", cyberFraudHandler.Update)
				cyber.PATCH("/:id/status", cyberFraudHandler.SetStatus)
				cyber.GET("/:id/network", cyberFraudHandler.Network)

				cyber.GET("/:id/entities", cyberFraudHandler.Entities)
				cyber.POST("/:id/entities", cyberFraudHandler.RecordEntity)
				cyber.DELETE("/:id/entities/:linkId", cyberFraudHandler.RemoveEntity)

				cyber.GET("/:id/transactions", cyberFraudHandler.Transactions)
				cyber.POST("/:id/transactions", cyberFraudHandler.RecordTransaction)

				cyber.GET("/:id/freeze-requests", cyberFraudHandler.FreezeRequests)
				cyber.POST("/:id/freeze-requests", cyberFraudHandler.DraftFreeze)
				cyber.POST("/:id/freeze-requests/:freezeId/transition", cyberFraudHandler.TransitionFreeze)

				cyber.GET("/:id/recoveries", cyberFraudHandler.Recoveries)
				cyber.POST("/:id/recoveries", cyberFraudHandler.RecordRecovery)
			}

			// Graph Intelligence routes
			graph := protected.Group("/graph")
			{
				// Nodes
				graph.GET("/nodes", graphHandler.SearchNodes)
				graph.GET("/nodes/:id", graphHandler.GetNode)
				graph.POST("/nodes", graphHandler.CreateNode)
				graph.PUT("/nodes/:id", graphHandler.UpdateNode)
				graph.GET("/nodes/:id/connections", graphHandler.GetConnections)

				// Edges
				graph.POST("/edges", graphHandler.CreateEdge)

				// Networks
				graph.GET("/networks", graphHandler.ListNetworks)
				graph.GET("/networks/:id", graphHandler.GetNetwork)
				graph.POST("/networks", graphHandler.CreateNetwork)
				graph.POST("/networks/:id/members", graphHandler.AddNetworkMember)
				graph.GET("/networks/:id/members", graphHandler.GetNetworkMembers)

				// Analytics
				graph.GET("/stats", graphHandler.GetStatistics)
				graph.GET("/visualization", graphHandler.GetVisualizationData)
			}

			// Phase 09 — Citizen Complaint & Grievance register.
			// Any officer may take a complaint at the counter and read the
			// register; acting on a complaint needs ASI; approving what the
			// citizen is told needs SI and a second officer (see the service).
			complaints := protected.Group("/complaints")
			{
				complaints.GET("", complaintHandler.List)
				complaints.GET("/stats", complaintHandler.Stats)
				complaints.GET("/routing-targets", complaintHandler.RoutingTargets)
				complaints.POST("", complaintHandler.Record)
				complaints.GET("/:id", complaintHandler.Get)
				complaints.GET("/:id/duplicate-candidates", complaintHandler.DuplicateCandidates)
				complaints.POST("/:id/categorise", complaintHandler.Categorise)
				complaints.POST("/:id/route", complaintHandler.Route)
				complaints.POST("/:id/status", complaintHandler.SetStatus)
				complaints.POST("/:id/notes", complaintHandler.AddNote)
				complaints.POST("/:id/link-duplicate", complaintHandler.LinkDuplicate)
				complaints.POST("/:id/link-fir", complaintHandler.LinkFIR)
				complaints.POST("/:id/responses", complaintHandler.DraftResponse)
				complaints.POST("/:id/responses/:responseId/review", complaintHandler.ReviewResponse)
			}

			// Traffic Challan routes
			traffic := protected.Group("/traffic")
			{
				traffic.GET("/violation-types", trafficChallanHandler.GetViolationTypes)
				traffic.GET("/challans", trafficChallanHandler.List)
				traffic.GET("/challans/:id", trafficChallanHandler.Get)
				traffic.GET("/challans/number/:number", trafficChallanHandler.GetByNumber)
				traffic.POST("/challans", trafficChallanHandler.Create)
				traffic.PATCH("/challans/:id/status", trafficChallanHandler.UpdateStatus)
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
					files.DELETE("/*key", uploadHandler.DeleteFile)
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

			// Phase 07 — Dispatch & Resource Optimisation
			dispatch := protected.Group("/dispatch")
			{
				dispatch.GET("/policy", dispatchHandler.Policy)
				dispatch.GET("/stats", dispatchHandler.Stats)
				dispatch.GET("/units", dispatchHandler.Units)
				dispatch.GET("/incidents", dispatchHandler.List)
				dispatch.GET("/incidents/:id", dispatchHandler.Get)
				dispatch.GET("/incidents/:id/events", dispatchHandler.Events)
				// Any officer may log an incident (control room, walk-in, officer in the field).
				dispatch.POST("/incidents", dispatchHandler.Intake)
				// The operator's decisions need rank.
				dispatch.POST("/incidents/:id/classify", dispatchHandler.Classify)
				dispatch.POST("/incidents/:id/assign", dispatchHandler.Assign)
				dispatch.POST("/incidents/:id/escalate", dispatchHandler.Escalate)
				dispatch.POST("/incidents/:id/close", dispatchHandler.Close)
				dispatch.POST("/assignments/:assignmentId/cancel", dispatchHandler.CancelAssignment)
				// The unit's own steps: the assigned officer, or ASI and above on their behalf.
				dispatch.POST("/assignments/:assignmentId/acknowledge", dispatchHandler.Acknowledge())
				dispatch.POST("/assignments/:assignmentId/on-scene", dispatchHandler.OnScene())
				dispatch.POST("/assignments/:assignmentId/clear", dispatchHandler.Clear())
				dispatch.GET("/analytics", dispatchHandler.Analytics)
			}

			// Statute library and incident location gazetteer
			//
			// Any officer searches Acts, sections, the IPC-BNS correspondence and
			// map places. Published law text and imported OpenStreetMap places are
			// read-only (409); SP and above add, correct and retire custom entries,
			// each with a reason kept in the audit log.
			legal := protected.Group("/legal")
			{
				legal.GET("/acts", legalHandler.ListActs)
				legal.GET("/acts/:id", legalHandler.GetAct)
				legal.GET("/acts/:id/sections", legalHandler.ListSections)
				legal.GET("/sections", legalHandler.SearchSections)
				legal.GET("/sections/:id", legalHandler.GetSection)
				legal.GET("/correspondence", legalHandler.Correspondence)
				legal.POST("/acts", legalHandler.CreateAct)
				legal.PUT("/acts/:id", legalHandler.UpdateAct)
				legal.POST("/acts/:id/retire", legalHandler.RetireAct)
				legal.POST("/acts/:id/sections", legalHandler.AddSection)
				legal.PUT("/sections/:id", legalHandler.UpdateSection)
				legal.POST("/sections/:id/retire", legalHandler.RetireSection)
			}
			gazetteer := protected.Group("/gazetteer")
			{
				gazetteer.GET("/search", legalHandler.SearchPlaces)
				gazetteer.GET("/nearest", legalHandler.NearestPlaces)
				gazetteer.GET("/places", legalHandler.ListOfficerPlaces)
				gazetteer.POST("/places", legalHandler.AddPlace)
				gazetteer.POST("/places/:id/retire", legalHandler.RetirePlace)
			}

			// Armoury — weapon register and issue/return ledger
			armoury := protected.Group("/armoury")
			{
				armoury.GET("/weapons", armouryHandler.List)
				armoury.GET("/weapons/stats", armouryHandler.Stats)
				armoury.GET("/weapons/:id", armouryHandler.Get)
				armoury.GET("/weapons/:id/issuances", armouryHandler.Issuances)
				armoury.GET("/issuances", armouryHandler.Issuances)
				armoury.POST("/weapons", armouryHandler.Register)
				armoury.PATCH("/weapons/:id/state", armouryHandler.SetState)
				armoury.POST("/weapons/:id/issue", armouryHandler.Issue)
				armoury.POST("/weapons/:id/return", armouryHandler.Return)
			}

			// Phase 03 — CCTV & Video Intelligence (functional layer, no AI)
			//
			// Role floors:
			//   view camera register, stats, health history ... any officer
			//   raise an event from footage .................. any officer
			//   run a reachability check ..................... ASI
			//   search or open events (purpose required) ..... ASI
			//   confirm / dismiss, link to FIR or case ........ SI (never the raiser)
			//   register or edit cameras, stream credentials .. SHO
			//   change event retention or masking ............ SHO
			//   decommission cameras, purge expired events,
			//   read the purpose log ......................... DSP
			video := protected.Group("/video")
			{
				video.GET("/cameras", videoHandler.ListCameras)
				video.GET("/cameras/stats", videoHandler.CameraStats)
				video.GET("/cameras/:id", videoHandler.GetCamera)
				video.GET("/cameras/:id/health-checks", videoHandler.HealthChecks)
				video.POST("/cameras", videoHandler.RegisterCamera)
				video.PUT("/cameras/:id", videoHandler.UpdateCamera)
				video.POST("/cameras/:id/decommission", videoHandler.Decommission)
				video.POST("/cameras/:id/health-check", videoHandler.CheckHealth)

				video.POST("/events", videoHandler.RaiseEvent)
				video.GET("/events/stats", videoHandler.EventStats)
				video.POST("/events/search", videoHandler.SearchEvents)
				video.POST("/events/purge-expired", videoHandler.PurgeExpired)
				video.POST("/events/:id/access", videoHandler.AccessEvent)
				video.POST("/events/:id/triage", videoHandler.Triage)
				video.POST("/events/:id/link", videoHandler.Link)
				video.POST("/events/:id/retention", videoHandler.SetRetention)

				video.GET("/access-log", videoHandler.AccessLog)
			}

			// AI layer A4 — Vehicle detection and ANPR (AI-assisted)
			//
			// Role floors:
			//   module status .................................. any officer
			//   submit footage or a still, ingest a camera
			//   snapshot, open an analysis, search plate reads
			//   (purpose required and logged), view the
			//   watchlist, hit queue and reads map .............. ASI
			//   add or remove watchlist entries,
			//   confirm / dismiss hits (never the submitter) .... SI
			//   switch the module on or off, read the purpose log DSP
			anpr := protected.Group("/anpr")
			{
				anpr.GET("/status", anprHandler.Status)
				anpr.PUT("/switch", anprHandler.SetSwitch)
				anpr.POST("/analyses", anprHandler.SubmitAnalysis)
				anpr.GET("/analyses", anprHandler.ListAnalyses)
				anpr.POST("/analyses/:id/access", anprHandler.OpenAnalysis)
				anpr.GET("/analyses/:id/frames/:frameId/image", anprHandler.FrameImage)
				anpr.POST("/snapshots", anprHandler.IngestSnapshot)
				anpr.POST("/reads/search", anprHandler.SearchReads)
				anpr.GET("/watchlist", anprHandler.Watchlist)
				anpr.POST("/watchlist", anprHandler.AddWatchlistEntry)
				anpr.POST("/watchlist/:id/remove", anprHandler.RemoveWatchlistEntry)
				anpr.GET("/hits", anprHandler.Hits)
				anpr.POST("/hits/:id/review", anprHandler.ReviewHit)
				anpr.GET("/map", anprHandler.Map)
				anpr.GET("/access-log", anprHandler.AccessLog)
			}

			// Phase 04 — Missing & Vulnerable Persons
			// Any officer may view (a child's identifying details are restricted in the
			// service), record a sighting or log family contact; deciding sightings and
			// working the checklist need ASI; changing or closing a report needs SI.
			missingPersons := protected.Group("/missing-persons")
			{
				missingPersons.GET("", missingPersonHandler.List)
				missingPersons.GET("/stats", missingPersonHandler.Stats)
				// City-wide board: every open report from every station, in its
				// broadcast form, polled by every open screen.
				missingPersons.GET("/board", missingPersonHandler.Board)
				missingPersons.POST("", missingPersonHandler.Register)
				missingPersons.GET("/:id", missingPersonHandler.Get)
				missingPersons.PATCH("/:id", missingPersonHandler.Update)
				missingPersons.POST("/:id/start-search", missingPersonHandler.StartSearch)
				missingPersons.GET("/:id/checklist", missingPersonHandler.Checklist)
				missingPersons.POST("/:id/checklist/:itemCode/complete", missingPersonHandler.CompleteChecklistItem)
				missingPersons.GET("/:id/sightings", missingPersonHandler.Sightings)
				missingPersons.POST("/:id/sightings", missingPersonHandler.RecordSighting)
				missingPersons.POST("/:id/sightings/:sightingId/verify", missingPersonHandler.VerifySighting)
				missingPersons.POST("/:id/sightings/:sightingId/reject", missingPersonHandler.RejectSighting)
				missingPersons.GET("/:id/movement", missingPersonHandler.Movement)
				missingPersons.GET("/:id/family-contacts", missingPersonHandler.FamilyContacts)
				missingPersons.POST("/:id/family-contacts", missingPersonHandler.RecordFamilyContact)
				missingPersons.POST("/:id/close", missingPersonHandler.Close)
				missingPersons.POST("/:id/lookout", missingPersonHandler.IssueLookout)
				// Photographs: ASI and above add them and choose the primary; SI and
				// above retire one (never deleted). Bytes are served privately.
				missingPersons.GET("/:id/photos", missingPersonHandler.Photos)
				missingPersons.POST("/:id/photos", missingPersonHandler.UploadPhoto)
				missingPersons.GET("/:id/photos/:photoId/image", missingPersonHandler.PhotoImage)
				missingPersons.GET("/:id/photos/:photoId/thumbnail", missingPersonHandler.PhotoThumbnail)
				missingPersons.POST("/:id/photos/:photoId/primary", missingPersonHandler.SetPrimaryPhoto)
				missingPersons.POST("/:id/photos/:photoId/retire", missingPersonHandler.RetirePhoto)
				// Each station's check of an open report (ASI and above record).
				missingPersons.GET("/:id/station-checks", missingPersonHandler.StationChecks)
				missingPersons.POST("/:id/station-checks", missingPersonHandler.RecordStationCheck)
				missingPersons.GET("/:id/map", missingPersonHandler.SearchMap)
			}

			// Face recognition for missing persons: authorisation, enrolment,
			// footage search, candidate review. Routes and role floors are in
			// FaceRecognitionHandler.RegisterRoutes.
			faceRecognitionHandler.RegisterRoutes(protected)

			// Phase 11 — Police Knowledge Assistant (functional, no AI)
			// Reading is open to every officer, bounded by classification in SQL.
			// Filing and superseding: SI and above. Checklists: SI and above create,
			// ASI and above follow and tick. Classification and withdrawal: SP and above.
			knowledge := protected.Group("/knowledge")
			{
				knowledge.GET("/capabilities", knowledgeHandler.Capabilities)
				knowledge.GET("/documents", knowledgeHandler.Search)
				knowledge.GET("/documents/stats", knowledgeHandler.Stats)
				knowledge.GET("/documents/:id", knowledgeHandler.Get)
				knowledge.GET("/documents/:id/file", knowledgeHandler.Download)
				knowledge.POST("/documents", knowledgeHandler.Upload)
				knowledge.POST("/documents/:id/supersede", knowledgeHandler.Supersede)
				knowledge.POST("/documents/:id/withdraw", knowledgeHandler.Withdraw)
				knowledge.PATCH("/documents/:id/classification", knowledgeHandler.SetClassification)
				knowledge.GET("/checklists", knowledgeHandler.Checklists)
				knowledge.GET("/checklists/:id", knowledgeHandler.Checklist)
				knowledge.POST("/checklists", knowledgeHandler.CreateChecklist)
				knowledge.GET("/checklists/:id/runs", knowledgeHandler.Runs)
				knowledge.POST("/checklists/:id/runs", knowledgeHandler.StartRun)
				knowledge.GET("/runs/:id", knowledgeHandler.Run)
				knowledge.POST("/runs/:id/ticks", knowledgeHandler.Tick)
			}

			// Lookout notices and sightings
			lookouts := protected.Group("/lookouts")
			{
				lookouts.GET("", lookoutHandler.List)
				lookouts.GET("/stats", lookoutHandler.Stats)
				lookouts.GET("/:id", lookoutHandler.Get)
				lookouts.GET("/:id/sightings", lookoutHandler.Sightings)
				lookouts.POST("", lookoutHandler.Issue)
				lookouts.POST("/:id/resolve", lookoutHandler.Resolve)
				// Any officer may report a sighting; verifying one needs rank.
				lookouts.POST("/:id/sightings", lookoutHandler.ReportSighting)
				lookouts.POST("/:id/sightings/:sightingId/verify", lookoutHandler.VerifySighting)
			}

			// Phase 08 — Station Workload. SHO and above; an SHO is held to their
			// own station by the service, and station comparison is DSP and above.
			workload := protected.Group("/workload")
			{
				workload.GET("/scopes", workloadHandler.Scopes)
				workload.GET("/summary", workloadHandler.Summary)
				workload.GET("/backlog", workloadHandler.Backlog)
				workload.GET("/sla", workloadHandler.SLA)
				workload.GET("/trends", workloadHandler.Trends)
				workload.GET("/officers", workloadHandler.Officers)
				workload.GET("/stations", workloadHandler.Stations)
			}

			// Phase 06 — Traffic Incident & Accident Reconstruction
			// Reading is open to any officer; recording needs ASI and above;
			// approving or returning a report needs SI and above and an officer
			// other than the drafter (enforced in the service and the table).
			trafficIncidents := protected.Group("/traffic-incidents")
			{
				trafficIncidents.GET("", trafficIncidentHandler.List)
				trafficIncidents.GET("/stats", trafficIncidentHandler.Stats)
				trafficIncidents.GET("/:id", trafficIncidentHandler.Get)
				trafficIncidents.GET("/:id/workspace", trafficIncidentHandler.Workspace)
				trafficIncidents.POST("", trafficIncidentHandler.Register)
				trafficIncidents.PUT("/:id", trafficIncidentHandler.Update)

				trafficIncidents.GET("/:id/vehicles", trafficIncidentHandler.Vehicles)
				trafficIncidents.POST("/:id/vehicles", trafficIncidentHandler.AddVehicle)
				trafficIncidents.DELETE("/:id/vehicles/:recordId", trafficIncidentHandler.RemoveChild("vehicles"))
				trafficIncidents.GET("/:id/persons", trafficIncidentHandler.Persons)
				trafficIncidents.POST("/:id/persons", trafficIncidentHandler.AddPerson)
				trafficIncidents.DELETE("/:id/persons/:recordId", trafficIncidentHandler.RemoveChild("persons"))
				trafficIncidents.GET("/:id/cameras", trafficIncidentHandler.Cameras)
				trafficIncidents.POST("/:id/cameras", trafficIncidentHandler.AddCamera)
				trafficIncidents.DELETE("/:id/cameras/:recordId", trafficIncidentHandler.RemoveChild("cameras"))
				trafficIncidents.GET("/:id/plate-reads", trafficIncidentHandler.PlateReads)
				trafficIncidents.POST("/:id/plate-reads", trafficIncidentHandler.AddPlateRead)
				trafficIncidents.POST("/:id/plate-reads/from-anpr", trafficIncidentHandler.AttachANPRRead)
				trafficIncidents.DELETE("/:id/plate-reads/:recordId", trafficIncidentHandler.RemoveChild("plate-reads"))
				trafficIncidents.GET("/:id/signal-phases", trafficIncidentHandler.SignalPhases)
				trafficIncidents.POST("/:id/signal-phases", trafficIncidentHandler.AddSignalPhase)
				trafficIncidents.DELETE("/:id/signal-phases/:recordId", trafficIncidentHandler.RemoveChild("signal-phases"))
				trafficIncidents.GET("/:id/facts", trafficIncidentHandler.Facts)
				trafficIncidents.POST("/:id/facts", trafficIncidentHandler.AddFact)
				trafficIncidents.DELETE("/:id/facts/:recordId", trafficIncidentHandler.RemoveChild("facts"))
				trafficIncidents.GET("/:id/timeline", trafficIncidentHandler.Timeline)
				trafficIncidents.GET("/:id/prior-challans", trafficIncidentHandler.PriorChallans)

				trafficIncidents.GET("/:id/report-draft", trafficIncidentHandler.Draft)
				trafficIncidents.GET("/:id/reports", trafficIncidentHandler.Reports)
				trafficIncidents.GET("/:id/reports/:reportId", trafficIncidentHandler.Report)
				trafficIncidents.POST("/:id/reports", trafficIncidentHandler.CreateReport)
				trafficIncidents.PUT("/:id/reports/:reportId", trafficIncidentHandler.UpdateReport)
				trafficIncidents.POST("/:id/reports/:reportId/submit", trafficIncidentHandler.SubmitReport)
				trafficIncidents.POST("/:id/reports/:reportId/approve", trafficIncidentHandler.ApproveReport)
				trafficIncidents.POST("/:id/reports/:reportId/return", trafficIncidentHandler.ReturnReport)
			}

			// Phase 14 — Malkhana / Seized Property (functional layer, no blockchain)
			//
			// Role floors (officers below DSP work only with their own station):
			//   view register, item, label, history, dashboard .... any officer
			//   register property, verify seals, move out / return  ASI
			//   add storage locations, move within the malkhana .... SI
			//   reseal after a broken seal (records the reason) ..... SHO
			//   disposal against a court order ...................... SHO
			//   (narcotics destruction also needs a DSP-rank witness)
			malkhana := protected.Group("/malkhana")
			{
				malkhana.GET("/dashboard", malkhanaHandler.Dashboard)
				malkhana.GET("/stations", malkhanaHandler.Stations)
				malkhana.GET("/locations", malkhanaHandler.Locations)
				malkhana.POST("/locations", malkhanaHandler.CreateLocation)
				malkhana.GET("/items", malkhanaHandler.List)
				malkhana.POST("/items", malkhanaHandler.Register)
				malkhana.GET("/items/by-number/:number", malkhanaHandler.GetByNumber)
				malkhana.GET("/items/:id", malkhanaHandler.Get)
				malkhana.GET("/items/:id/label", malkhanaHandler.Label)
				malkhana.GET("/items/:id/seal-checks", malkhanaHandler.SealChecks)
				malkhana.GET("/items/:id/movements", malkhanaHandler.Movements)
				malkhana.GET("/items/:id/events", malkhanaHandler.Events)
				malkhana.POST("/items/:id/seal-checks", malkhanaHandler.VerifySeal)
				malkhana.POST("/items/:id/reseal", malkhanaHandler.Reseal)
				malkhana.POST("/items/:id/relocate", malkhanaHandler.Relocate)
				malkhana.POST("/items/:id/movements", malkhanaHandler.MoveOut)
				malkhana.POST("/items/:id/movements/:movementId/return", malkhanaHandler.Return)
				malkhana.GET("/items/:id/movements/:movementId/forwarding-letter", malkhanaHandler.ForwardingLetter)
				malkhana.POST("/items/:id/dispose", malkhanaHandler.Dispose)
			}

			// Phase 10 — Public Safety Risk & Hotspots. Scores places, never
			// people. SHO and above; below DSP an officer sees only their own
			// station (enforced in the service). Weights change at SP and above.
			risk := protected.Group("/risk")
			{
				risk.GET("/factors", riskHandler.Factors)
				risk.GET("/weights/history", riskHandler.WeightHistory)
				risk.POST("/weights", riskHandler.UpdateWeights)
				risk.GET("/areas", riskHandler.Areas)
				risk.GET("/recommendations", riskHandler.Recommendations)
				risk.POST("/simulate", riskHandler.Simulate)
				risk.GET("/beats", riskHandler.Beats)
				risk.POST("/beats", riskHandler.CreateBeat)
				risk.DELETE("/beats/:id", riskHandler.DeleteBeat)
				risk.GET("/firs", riskHandler.PlaceableFIRs)
				risk.POST("/placements", riskHandler.PlaceFIR)
				risk.DELETE("/placements/:firId", riskHandler.UnplaceFIR)
			}

			// Access log — sign-in activity from the audit trail
			accessLog := protected.Group("/access-log")
			{
				accessLog.GET("", accessLogHandler.List)
				accessLog.GET("/stats", accessLogHandler.Stats)
			}

			// Phase 12 — Case File & Court Readiness. Reading needs ASI; building the
			// file needs SI; approval needs Inspector, and never by the submitter.
			caseFiles := protected.Group("/case-files")
			{
				caseFiles.GET("", caseFileHandler.List)
				caseFiles.POST("", caseFileHandler.Create)
				caseFiles.GET("/by-workspace/:workspaceId", caseFileHandler.GetByWorkspace)
				caseFiles.GET("/:id", caseFileHandler.Get)
				caseFiles.GET("/:id/entries", caseFileHandler.Entries)
				caseFiles.GET("/:id/sources", caseFileHandler.Sources)
				caseFiles.POST("/:id/entries", caseFileHandler.AddEntry)
				caseFiles.POST("/:id/entries/upload", caseFileHandler.UploadEntry)
				caseFiles.POST("/:id/entries/:entryId/remove", caseFileHandler.RemoveEntry)
				caseFiles.GET("/:id/entries/:entryId/file", caseFileHandler.DownloadEntry)
				caseFiles.GET("/:id/evidence-matrix", caseFileHandler.EvidenceMatrix)
				caseFiles.POST("/:id/charges", caseFileHandler.AddCharge)
				caseFiles.DELETE("/:id/charges/:chargeId", caseFileHandler.RemoveCharge)
				caseFiles.POST("/:id/charges/:chargeId/evidence", caseFileHandler.LinkSupport)
				caseFiles.DELETE("/:id/charges/:chargeId/evidence/:evidenceId", caseFileHandler.UnlinkSupport)
				caseFiles.GET("/:id/witness-matrix", caseFileHandler.WitnessMatrix)
				caseFiles.POST("/:id/witness-facts", caseFileHandler.AddWitnessFact)
				caseFiles.DELETE("/:id/witness-facts/:factId", caseFileHandler.RemoveWitnessFact)
				caseFiles.GET("/:id/completeness", caseFileHandler.Completeness)
				caseFiles.GET("/:id/versions", caseFileHandler.Versions)
				caseFiles.GET("/:id/versions/:version", caseFileHandler.Version)
				caseFiles.GET("/:id/packs", caseFileHandler.Packs)
				caseFiles.POST("/:id/packs", caseFileHandler.Submit)
				caseFiles.GET("/:id/packs/:packId", caseFileHandler.Pack)
				caseFiles.POST("/:id/packs/:packId/approve", caseFileHandler.Approve)
				caseFiles.POST("/:id/packs/:packId/return", caseFileHandler.Return)
			}

			// Phase 13 — Body-Worn Camera Evidence (functional layer, no AI)
			//
			// Role floors:
			//   view camera register, readings, assignment ledger ... any officer
			//   dock a recording ............. the wearing officer, or ASI (checked in the handler)
			//   record a reading, issue and return a camera ........ ASI
			//   list recordings, open or download with a purpose,
			//   verify, custody chain ............................... ASI
			//   link a recording to an FIR or case (becomes evidence) SI
			//   register a camera, change its status ............... SHO
			//   access log, purge expired non-evidential footage ... DSP
			bodycam := protected.Group("/bodycam")
			{
				bodycam.GET("/devices", bodycamHandler.ListDevices)
				bodycam.GET("/devices/stats", bodycamHandler.Stats)
				bodycam.GET("/devices/:id", bodycamHandler.GetDevice)
				bodycam.POST("/devices", bodycamHandler.RegisterDevice)
				bodycam.PATCH("/devices/:id/status", bodycamHandler.SetStatus)
				bodycam.GET("/devices/:id/readings", bodycamHandler.Readings)
				bodycam.POST("/devices/:id/readings", bodycamHandler.RecordReading)
				bodycam.GET("/devices/:id/assignments", bodycamHandler.Assignments)
				bodycam.POST("/devices/:id/assignments", bodycamHandler.Issue)
				bodycam.POST("/devices/:id/assignments/:assignmentId/return", bodycamHandler.Return)
				bodycam.POST("/devices/:id/assignments/:assignmentId/recordings", bodycamHandler.Dock)

				bodycam.GET("/recordings", bodycamHandler.Recordings)
				bodycam.POST("/recordings/purge-expired", bodycamHandler.PurgeExpired)
				bodycam.POST("/recordings/:id/access", bodycamHandler.Access)
				bodycam.GET("/recordings/:id/file", bodycamHandler.Download)
				bodycam.POST("/recordings/:id/verify", bodycamHandler.Verify)
				bodycam.GET("/recordings/:id/custody", bodycamHandler.Chain)
				bodycam.POST("/recordings/:id/link", bodycamHandler.Link)
				bodycam.GET("/recordings/:id/access-log", bodycamHandler.AccessLog)
				bodycam.POST("/recordings/:id/purge", bodycamHandler.Purge)
			}

			// Phase 02 — Evidence & Chain of Custody
			custody := protected.Group("/custody")
			{
				custody.GET("", custodyHandler.List)
				custody.POST("", custodyHandler.Register)
				custody.GET("/stats", custodyHandler.Stats)
				custody.GET("/:id", custodyHandler.Get)

				custody.POST("/:id/file", custodyHandler.AttachFile)
				custody.GET("/:id/file", custodyHandler.Download)

				custody.POST("/:id/verify", custodyHandler.Verify)
				custody.GET("/:id/verifications", custodyHandler.IntegrityHistory)

				custody.GET("/:id/chain", custodyHandler.CustodyChain)
				custody.POST("/:id/transfer",

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

			// Records crossing between the four departments. A referral is
			// proposed by the force that holds the record and accepted by the
			// one it is sent to; until then nothing has crossed.
			referrals := protected.Group("/referrals")
			{
				referrals.GET("", referralHandler.List)
				referrals.GET("/:id", referralHandler.Get)
				referrals.POST("", referralHandler.Propose)
				referrals.POST("/:id/decision", referralHandler.Decide)
				referrals.POST("/:id/withdraw", referralHandler.Withdraw)
			}

			// Officer accounts: who may sign in, where they are posted, and
			// when they stop. Note what is not here — there is no DELETE. An
			// account is deactivated, and the FIRs, cases and custody entries
			// the officer made keep naming them.
			//
			// Reading the roster is a supervisor's work, so it sits at DSP:
			// a sub-divisional officer plans postings and needs to see who is
			// where, and the list carries no credential.
			//
			// Every write sits at SP. Opening an account creates authority
			// inside the platform, and a transfer moves an officer between
			// departments; the Superintendent is the first rank that commands
			// a district or a force unit and answers for both. One floor for
			// every write, rather than a scale, so that there is no rank that
			// can amend an account but not account for the amendment.
			officers := protected.Group("/officers")
			{
				officers.GET("", userAdminHandler.List)
				officers.GET("/options", userAdminHandler.Options)
				officers.GET("/:id", userAdminHandler.Get)
				officers.POST("", userAdminHandler.Create)
				officers.PATCH("/:id", userAdminHandler.Amend)
				officers.POST("/:id/deactivate", userAdminHandler.Deactivate)
				officers.POST("/:id/reactivate", userAdminHandler.Reactivate)
				officers.POST("/:id/password-reset", userAdminHandler.ResetPassword)
				officers.GET("/:id/roles", roleHandler.OfficerRoles)
				officers.POST("/:id/roles", roleHandler.Assign)
				officers.DELETE("/:id/roles/:roleId", roleHandler.Unassign)
			}

			// Roles and the permission catalogue. What an officer may do is
			// administered here; how senior they are stays on /officers.
			roles := protected.Group("/roles")
			{
				roles.GET("", roleHandler.List)
				roles.POST("", roleHandler.Create)
				roles.GET("/:id", roleHandler.Get)
				roles.PATCH("/:id", roleHandler.Update)
				roles.DELETE("/:id", roleHandler.Delete)
				roles.PUT("/:id/permissions", roleHandler.SetPermissions)
			}
			protected.GET("/permissions", roleHandler.Catalogue)

			aiReview := protected.Group("/ai-review")
			{
				// Review queue
				aiReview.GET("/queue", aiReviewHandler.GetReviewQueue)
				aiReview.GET("/my-assignments", aiReviewHandler.GetMyAssignments)
				aiReview.GET("/stats", aiReviewHandler.GetStats)
				// Acceptance and override rates per station, language or type.
				aiReview.GET("/acceptance", aiReviewHandler.GetAcceptance)

				// Decision management
				aiReview.GET("/decisions/:id", aiReviewHandler.GetDecision)
				aiReview.POST("/decisions/:id/review", aiReviewHandler.ReviewDecision)
				aiReview.POST("/decisions/:id/assign", aiReviewHandler.AssignDecision)
				aiReview.POST("/decisions/:id/feedback", aiReviewHandler.SubmitFeedback)
				aiReview.GET("/decisions/:id/history", aiReviewHandler.GetDecisionHistory)

				// Bulk operations
				aiReview.POST("/bulk-review", aiReviewHandler.BulkReview)

				// The model registry. Reading it is open to DSP and above;
				// registering, measuring and switching a model on is SP and above.
				aiReview.GET("/models", aiReviewHandler.GetModelConfigs)
				aiReview.POST("/models", aiReviewHandler.RegisterModel)
				aiReview.GET("/models/:modelName", aiReviewHandler.GetModelConfig)
				aiReview.PUT("/models/:modelName", aiReviewHandler.UpdateModelConfig)

				// Evaluations: a model is switched on only after one passes.
				aiReview.GET("/evaluations", aiReviewHandler.ListEvaluations)
				aiReview.POST("/evaluations", aiReviewHandler.RecordEvaluation)

				aiReview.POST("/models/:modelName/retire", aiReviewHandler.RetireModel)

				// The per-module off switches, one row per AI module. Face
				// recognition and vehicle detection keep their own screens and
				// their own rules; this covers every other module, without
				// which a measured model could never produce anything.
				aiReview.GET("/modules", aiReviewHandler.ListModuleSwitches)
				aiReview.PUT("/modules/:module", aiReviewHandler.SetModuleSwitch)

				// The gateway's own state: every registered model and whether
				// its service can be reached from this deployment.
				aiReview.GET("/gateway", aiGatewayHandler.Status)

				// Maintenance
				aiReview.POST("/expire", aiReviewHandler.ExpireDecisions)
			}

			// Biometric & Attendance routes
			biometric := protected.Group("/biometric")
			{
				// Device management
				biometric.GET("/devices", biometricHandler.GetDevices)
				biometric.GET("/devices/:id", biometricHandler.GetDevice)
				biometric.POST("/devices", biometricHandler.RegisterDevice)
				biometric.PATCH("/devices/:id/status", biometricHandler.UpdateDeviceStatus)
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
				district.POST("", districtHandler.CreateDistrict)
				district.PUT("/:id", districtHandler.UpdateDistrict)
				district.GET("/:id/dashboard", districtHandler.GetDistrictDashboard)
				district.GET("/:id/rankings", districtHandler.GetStationRankings)
				district.GET("/:id/hotspots", districtHandler.GetCrimeHotspots)
				district.GET("/:id/tasks", districtHandler.GetPendingTasks)

				// Stations
				district.GET("/stations", districtHandler.ListStations)
				district.GET("/stations/:id", districtHandler.GetStation)
				district.POST("/stations", districtHandler.CreateStation)
				district.PUT("/stations/:id", districtHandler.UpdateStation)

				// Cross-Station Coordination
				district.GET("/coordination/requests", districtHandler.ListCrossStationRequests)
				district.GET("/coordination/requests/:id", districtHandler.GetCrossStationRequest)
				district.POST("/coordination/requests", districtHandler.CreateCrossStationRequest)
				district.POST("/coordination/requests/:id/approve", districtHandler.ApproveCrossStationRequest)
				district.POST("/coordination/requests/:id/reject", districtHandler.RejectCrossStationRequest)
				district.POST("/coordination/requests/:id/complete", districtHandler.CompleteCrossStationRequest)

				// Meetings
				district.GET("/meetings", districtHandler.ListMeetings)
				district.GET("/meetings/:id", districtHandler.GetMeeting)
				district.POST("/meetings", districtHandler.CreateMeeting)
				district.PUT("/meetings/:id", districtHandler.UpdateMeeting)

				// Resource Allocation
				district.GET("/resources", districtHandler.ListResourceAllocations)
				district.GET("/resources/:id", districtHandler.GetResourceAllocation)
				district.POST("/resources", districtHandler.CreateResourceAllocation)
				district.POST("/resources/:id/approve", districtHandler.ApproveResourceAllocation)
				district.POST("/resources/:id/allocate", districtHandler.AllocateResource)
				district.POST("/resources/:id/return", districtHandler.ReturnResource)
			}

			// State Management routes
			state := protected.Group("/state")
			{
				// States
				state.GET("", stateHandler.ListStates)
				state.GET("/:id", stateHandler.GetState)
				state.GET("/code/:code", stateHandler.GetStateByCode)
				state.POST("", stateHandler.CreateState)
				state.PUT("/:id", stateHandler.UpdateState)
				state.GET("/:id/dashboard", stateHandler.GetStateDashboard)
				state.GET("/:id/metrics", stateHandler.GetPerformanceMetrics)
				state.GET("/:id/hotspots", stateHandler.GetStateCrimeHotspots)
				state.GET("/:id/resources", stateHandler.GetResourceOverview)

				// Zones
				state.GET("/zones", stateHandler.ListZones)
				state.GET("/zones/:id", stateHandler.GetZone)
				state.POST("/zones", stateHandler.CreateZone)
				state.PUT("/zones/:id", stateHandler.UpdateZone)

				// Ranges
				state.GET("/ranges", stateHandler.ListRanges)
				state.GET("/ranges/:id", stateHandler.GetRange)
				state.POST("/ranges", stateHandler.CreateRange)
				state.PUT("/ranges/:id", stateHandler.UpdateRange)

				// Inter-District Coordination
				state.GET("/coordination/requests", stateHandler.ListInterDistrictRequests)
				state.GET("/coordination/requests/:id", stateHandler.GetInterDistrictRequest)
				state.POST("/coordination/requests", stateHandler.CreateInterDistrictRequest)
				state.POST("/coordination/requests/:id/approve", stateHandler.ApproveInterDistrictRequest)
				state.POST("/coordination/requests/:id/reject", stateHandler.RejectInterDistrictRequest)

				// State Alerts
				state.GET("/alerts", stateHandler.ListStateAlerts)
				state.GET("/alerts/:id", stateHandler.GetStateAlert)
				state.POST("/alerts", stateHandler.CreateStateAlert)
				state.PUT("/alerts/:id", stateHandler.UpdateStateAlert)
				state.POST("/alerts/:id/acknowledge", stateHandler.AcknowledgeStateAlert)
			}

			// National Command Center routes (DGP+ only)
			national := protected.Group("/national")
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
			// Phase 09. Tracking numbers contain "/", so tracking is a POST
			// carrying the number with its second factor (phone, or the access
			// code an anonymous complainant was given) — never the number alone.
			citizen.POST("/complaints",
				handlers.LimitBody(handlers.PublicBodyLimit),
				middleware.RateLimiter(middleware.RateLimiterConfig{Limit: 10, Window: time.Hour, RedisClient: rdbV8, KeyPrefix: "public_complaint_submit"}),
				complaintHandler.SubmitPublic)
			citizen.POST("/complaints/track",
				handlers.LimitBody(4<<10),
				middleware.RateLimiter(middleware.RateLimiterConfig{Limit: 20, Window: 15 * time.Minute, RedisClient: rdbV8, KeyPrefix: "public_complaint_track"}),
				complaintHandler.TrackPublic)
			citizen.POST("/fir-status",
				handlers.LimitBody(4<<10),
				middleware.RateLimiter(middleware.RateLimiterConfig{Limit: 20, Window: 15 * time.Minute, RedisClient: rdbV8, KeyPrefix: "public_fir_status"}),
				citizenPortalHandler.TrackFIR)
			citizen.POST("/grievances", citizenPortalHandler.SubmitGrievance)
			citizen.POST("/missing-persons", citizenPortalHandler.SubmitMissingPersonReport)
			// Catch-all: report numbers are MIS/YYYY/NNNNN, and a :param cannot hold slashes.
			citizen.GET("/missing-persons/*reportNumber", citizenPortalHandler.TrackMissingPersonReport)
			citizen.POST("/fir-copy-request", citizenPortalHandler.RequestFIRCopy)
			citizen.GET("/stats", citizenPortalHandler.GetPortalStats)
		}
	}

	// Every route must name the permission it needs.
	//
	// Checked here, with the router fully built, because the alternative is
	// discovering it in a station: a route missing from the catalogue is
	// refused by the middleware, so it fails shut rather than open, but a door
	// nobody knew was locked is still a defect — and one found by an officer
	// rather than by whoever added the route. Refusing to start puts that
	// discovery in front of the person who caused it.
	registered := make([]authz.Route, 0, len(router.Routes()))
	for _, r := range router.Routes() {
		registered = append(registered, authz.Route{Method: r.Method, Path: r.Path})
	}
	if err := authz.VerifyRoutes(registered); err != nil {
		log.Fatalf("route catalogue is incomplete:\n%v", err)
	}
	log.Printf("Authorisation: %d routes, each naming a permission", len(registered))

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

// aiModelEndpoints reads which environment variable holds each registered
// model's service address. The registry is the source of truth, so adding a
// model is a registry entry and an address, not a change to this file.
//
// A registry that cannot be read (a database built before migration 000076,
// for instance) is not fatal: the API starts with no model clients, and every
// AI screen reports that nothing is connected.
func aiModelEndpoints(ctx context.Context, review *services.AIReviewService) []struct{ model, env string } {
	out := []struct{ model, env string }{}

	entries, err := review.GetAllModelConfigs(ctx)
	if err != nil {
		log.Printf("AI gateway: the model registry could not be read (%v); no model clients attached", err)
		return out
	}

	for _, entry := range entries {
		if entry.EndpointEnv == "" || entry.RetiredAt != nil {
			continue
		}
		out = append(out, struct{ model, env string }{entry.ModelName, entry.EndpointEnv})
	}

	if len(out) == 0 {
		log.Printf("AI gateway: %d models registered, none with a service address on this deployment", len(entries))
	}
	return out
}
