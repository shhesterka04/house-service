package app

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"github.com/shhesterka04/house-service/internal/config"
	"github.com/shhesterka04/house-service/internal/handlers"
	"github.com/shhesterka04/house-service/internal/middleware"
	"github.com/shhesterka04/house-service/internal/repository"
	"github.com/shhesterka04/house-service/internal/service"
	"github.com/shhesterka04/house-service/pkg/db"
	"github.com/shhesterka04/house-service/pkg/logger"
)

func Run(ctx context.Context) error {
	logger.Infof(ctx, "starting app")

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Errorf(ctx, "config load error: %v", err)
		return errors.Wrap(err, "load config")
	}

	pgClient := db.NewClient(
		cfg.DBName,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
	)

	dbConn, err := pgClient.Connect(ctx)
	if err != nil {
		return errors.Wrap(err, "connect to database")
	}
	logger.Infof(ctx, "connected to database")

	if err = pgClient.Migrate("/migrations"); err != nil {
		return errors.Wrap(err, "migrate")
	}
	defer pgClient.Close()

	userRepo := repository.NewUserRepository(dbConn.Cluster)
	authService := service.NewAuthService(userRepo)
	authHandlers := handlers.NewAuthHandlers(authService)

	houseRepo := repository.NewHouseRepository(dbConn.Cluster)
	houseService := service.NewHouseService(houseRepo)
	houseHandlers := handlers.NewHouseHandler(houseService)

	flatRepo := repository.NewFlatRepository(dbConn.Cluster)
	flatService := handlers.NewFlatService(flatRepo, houseRepo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /dummyLogin", authHandlers.DummyLogin)
	mux.HandleFunc("POST /login", authHandlers.Login)
	mux.HandleFunc("POST /register", authHandlers.Register)

	protectedRoutes := http.NewServeMux()
	protectedRoutes.Handle("POST /house/create", middleware.AuthMiddleware("moderator")(http.HandlerFunc(houseHandlers.CreateHouse)))
	protectedRoutes.Handle("GET /house/{id}", middleware.AuthMiddleware("client")(http.HandlerFunc(flatService.GetFlatsByHouseID)))
	protectedRoutes.Handle("POST /house/{id}/subscribe", middleware.AuthMiddleware("client")(http.HandlerFunc(houseHandlers.SubscribeToHouse)))
	protectedRoutes.Handle("POST /flat/create", middleware.AuthMiddleware("client")(http.HandlerFunc(flatService.CreateFlat)))
	protectedRoutes.Handle("POST /flat/update", middleware.AuthMiddleware("client")(http.HandlerFunc(flatService.UpdateFlat)))

	mux.Handle("/", protectedRoutes)

	logger.Infof(ctx, "starting server on %s", cfg.HostAddr)
	if err := http.ListenAndServe(cfg.HostAddr, mux); err != nil {
		return errors.Wrap(err, "listen and serve")
	}

	return nil
}
