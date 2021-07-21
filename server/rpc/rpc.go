package rpc

import (
	"fmt"
	"net/http"

	"tflgame/server/app"
	"tflgame/server/lib/cher"
	"tflgame/server/lib/config"
	"tflgame/server/lib/crpc"
	"tflgame/server/lib/httperr"
	"tflgame/server/rpc/middleware"

	"github.com/go-chi/chi"
	"github.com/sirupsen/logrus"
	"github.com/xeipuuv/gojsonschema"
)

const (
	UnsafeNoAuth AuthType = iota
	InternalOnlyAuth
	JWTAuth
)

type AuthType int

type RPC struct {
	app          *app.App
	internalKeys []string
	Logger       *logrus.Logger
	limiter      *Limiter

	httpServer *http.Server
}

func New(app *app.App, l *logrus.Logger, internalKeys []string, rateLimit *Limiter) *RPC {
	r := &RPC{
		app:          app,
		Logger:       l,
		internalKeys: internalKeys,
		limiter:      rateLimit,
	}

	mux := chi.NewRouter()

	mux.NotFound(func(res http.ResponseWriter, req *http.Request) {
		httperr.HandleError(res, cher.New(cher.RouteNotFound, nil))
		return
	})

	mux.Post(r.route("/authenticate", r.Authenticate, AuthenticateSchema, UnsafeNoAuth))
	mux.Post(r.route("/create_user", r.CreateUser, CreateUserSchema, UnsafeNoAuth))
	mux.Post(r.route("/change_handle", r.ChangeHandle, ChangeHandleSchema, JWTAuth))
	mux.Post(r.route("/release_handle", r.ReleaseHandle, ReleaseHandleSchema, JWTAuth))
	mux.Post(r.route("/change_pin", r.ChangePin, ChangePinSchema, JWTAuth))
	mux.Post(r.route("/list_events", r.ListEvents, ListEventsSchema, JWTAuth))
	mux.Post(r.route("/list_game_history", r.ListGameHistory, ListGameHistorySchema, JWTAuth))

	// server endpoints
	mux.Post(r.route("/sync_tfl_data", r.SyncTFLData, nil, InternalOnlyAuth))

	// game endpoints
	mux.Post(r.route("/test_game_options", r.TestGameOptions, TestGameOptionsSchema, UnsafeNoAuth))
	mux.Post(r.route("/get_game_options", r.GetGameOptions, nil, UnsafeNoAuth))
	mux.Post(r.route("/create_game", r.CreateGame, CreateGameSchema, JWTAuth))
	mux.Post(r.route("/submit_answer", r.SubmitAnswer, SubmitAnswerSchema, JWTAuth))
	mux.Post(r.route("/get_current_game", r.GetCurrentGame, GetCurrentGameSchema, JWTAuth))
	mux.Post(r.route("/get_hint", r.GetHint, GetHintSchema, JWTAuth))
	mux.Post(r.route("/get_game_state", r.GetGameState, GetGameStateSchema, JWTAuth))
	mux.Post(r.route("/explain_score", r.ExplainScore, ExplainScoreSchema, JWTAuth))
	mux.Post(r.route("/get_leaderboard", r.GetLeaderboard, nil, UnsafeNoAuth))

	mux.Options("/*", func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		res.Header().Set("Access-Control-Allow-Headers", "*")
		res.Header().Set("Access-Control-Request-Headers", "*")
		res.Header().Set("Access-Control-Allow-Methods", "POST")
	})

	r.httpServer = &http.Server{Handler: mux}

	return r
}

func (r RPC) route(pattern string, fnR interface{}, schema gojsonschema.JSONLoader, authRequirement AuthType) (string, http.HandlerFunc) {
	fn := crpc.MustWrap(fnR)
	hasSchema := schema != nil

	if fn.AcceptsInput != hasSchema {
		if hasSchema {
			panic("schema validation configured, but handler doesn't accept input")
		} else {
			panic("no schema validation configured")
		}
	}

	var handler http.HandlerFunc = fn.Handler

	if schema != nil {
		compiledSchema, err := gojsonschema.NewSchemaLoader().Compile(schema)
		if err != nil {
			panic(fmt.Errorf("json schema error in %s: %w", pattern, err))
		}

		handler = middleware.ValidateSchema(handler, compiledSchema)
	}

	switch authRequirement {
	case UnsafeNoAuth:
	case InternalOnlyAuth:
		handler = middleware.AuthenticateInternalKey(handler, r.internalKeys)
	case JWTAuth:
		handler = middleware.AuthenticateJWT(handler, r.app.SigningKeys.GetPublicKey())
	}

	handler = middleware.AddCORSHeaders(handler)

	return pattern, handler
}

func (r *RPC) Run(cfg config.Server) (err error) {
	r.Logger.WithField("addr", cfg.Addr).Info("listening")
	if err = cfg.ListenAndServe(r.httpServer); err != nil {
		err = fmt.Errorf("server: %w", err)
	}

	return
}
