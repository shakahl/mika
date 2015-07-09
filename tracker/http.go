package tracker

import (
	"crypto/tls"
	"git.totdev.in/totv/mika/conf"
	"github.com/Sirupsen/logrus"
	log "github.com/Sirupsen/logrus"
	"github.com/gin-gonic/gin"
	"net/http"
)

type (
	APIErrorResponse struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
)

// APIErrorHandler is used as the default error handler for API requests
// in the echo router. The function requires the use of the forked echo router
// available at git@git.totdev.in:totv/echo.git because we are passing more information
// than the standard HTTPError used.
//func APIErrorHandler(http_error *echo.HTTPError, c *echo.Context) {
//	http_error.Log()
//
//	if http_error.Code == 0 {
//		http_error.Code = http.StatusInternalServerError
//	}
//	err_resp := NewApiError(http_error)
//	c.JSON(http_error.Code, err_resp)
//}

// TrackerErrorHandler is used as the default error handler for tracker requests
// the error is returned to the client as a bencoded error string as defined in the
// bittorrent specs.
//func TrackerErrorHandler(http_error *echo.HTTPError, c *echo.Context) {
//	http_error.Log()
//
//	if http_error.Code == 0 {
//		http_error.Code = MSG_GENERIC_ERROR
//	}
//
//	c.String(http_error.Code, responseError(http_error.Message))
//}

func handleApiErrors(c *gin.Context) {
	// Execute the next handler, recording any errors to be process below
	c.Next()

	error_returned := c.Errors.Last()
	if error_returned != nil {
		meta := error_returned.JSON().(gin.H)

		status := 500
		custom_status, found := meta["status"]
		if found {
			status = custom_status.(int)
		}

		// TODO handle private/public errors separately, like sentry output for priv errors
		if error_returned != nil && error_returned.Meta != nil {
			c.JSON(status, meta)
		}
	}
}

// Run starts all of the background goroutines related to managing the tracker
// and starts the tracker and API HTTP interfaces
func (t *Tracker) Run() {
	go t.dbStatIndexer()
	go t.syncWriter()
	go t.peerStalker()
	go t.listenTracker()
	t.listenAPI()
}

// listenTracker created a new http router, configured the routes and handlers, and
// starts the trackers HTTP server listening over HTTP. This function will not
// start the API endpoints. See listenAPI for those.
func (t *Tracker) listenTracker() {
	log.WithFields(log.Fields{
		"listen_host": conf.Config.ListenHost,
		"tls":         false,
	}).Info("Loading Tracker route handlers")

	router := NewRouter()

	router.GET("/:passkey/announce", t.HandleAnnounce)
	router.GET("/:passkey/scrape", t.HandleScrape)

	router.Run(conf.Config.ListenHost)
}

func errMeta(status int, message string, fields logrus.Fields, level logrus.Level) gin.H {
	return gin.H{
		"status":  status,
		"message": message,
		"fields":  fields,
		"level":   level,
	}
}

// NewRouter creates and returns a newly configured router instance using
// the default middleware handlers.
func NewRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	return router
}

// listenAPI creates a new api request router and start the http server listening over TLS
func (t *Tracker) listenAPI() {
	log.WithFields(log.Fields{
		"listen_host": conf.Config.ListenHostAPI,
		"tls":         true,
	}).Info("Loading API route handlers")

	router := NewRouter()
	router.Use(handleApiErrors)

	var api *gin.RouterGroup

	// Optionally enabled BasicAuth over the TLS only API
	if conf.Config.APIUsername == "" || conf.Config.APIPassword == "" {
		log.Warn("No credentials set for API. All users granted access.")
		api = router.Group("/api")
	} else {
		api = router.Group("/api", gin.BasicAuth(gin.Accounts{
			conf.Config.APIUsername: conf.Config.APIPassword,
		}))
	}

	api.GET("/version", t.HandleVersion)
	api.GET("/uptime", t.HandleUptime)
	api.GET("/torrent/:info_hash", t.HandleTorrentGet)
	api.POST("/torrent", t.HandleTorrentAdd)
	api.GET("/torrent/:info_hash/peers", t.HandleGetTorrentPeers)
	api.DELETE("/torrent/:info_hash", t.HandleTorrentDel)

	api.POST("/user", t.HandleUserCreate)
	api.GET("/user/:user_id", t.HandleUserGet)
	api.POST("/user/:user_id", t.HandleUserUpdate)
	api.DELETE("/user/:user_id", t.HandleUserDel)
	api.GET("/user/:user_id/torrents", t.HandleUserTorrents)

	api.POST("/whitelist", t.HandleWhitelistAdd)
	api.DELETE("/whitelist/:prefix", t.HandleWhitelistDel)

	tls_config := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		},
	}
	if conf.Config.SSLCert == "" || conf.Config.SSLPrivateKey == "" {
		log.Fatalln("SSL config keys not set in config!")
	}
	srv := http.Server{TLSConfig: tls_config, Addr: conf.Config.ListenHostAPI, Handler: router}
	log.Fatal(srv.ListenAndServeTLS(conf.Config.SSLCert, conf.Config.SSLPrivateKey))
}
