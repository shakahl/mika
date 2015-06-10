package tracker

import (
	"crypto/tls"
	"git.totdev.in/totv/mika/conf"
	"github.com/Sirupsen/logrus"
	log "github.com/Sirupsen/logrus"
	"github.com/gin-gonic/gin"
	"net/http"
	//	"reflect"
)

type (
	APIErrorResponse struct {
		Code    int    `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
)

// NewApiError creates a new error model based on the HTTPError stuct values passed in
//func NewApiError(err *echo.HTTPError) *APIErrorResponse {
//	if err.Error != nil {
//		return &APIErrorResponse{
//			Error:   err.Error.Error(),
//			Message: err.Message,
//		}
//	} else {
//		return &APIErrorResponse{
//			Error:   err.Message,
//			Message: err.Message,
//		}
//	}
//}

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
	// Execute the handlers, recording any errors to be process below
	c.Next()

	error_returned := c.Errors.ByType(gin.ErrorTypePublic).Last()

	//	if error_returned.Meta != nil {
	//		value := reflect.ValueOf(error_returned.Meta)
	//		switch value.Kind() {
	//		case reflect.Struct:
	//			return error_returned.Meta
	//		case reflect.Map:
	//			for _, key := range value.MapKeys() {
	//				json[key.String()] = value.MapIndex(key).Interface()
	//			}
	//		default:
	//			json["meta"] = msg.Meta
	//		}
	//	}

	// TODO handle private/public errors separately, like sentry output for priv errors
	if error_returned != nil && error_returned.Meta != nil {
		c.JSON(500, error_returned.Meta)
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

	router := gin.Default()

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

// listenAPI created a new api request router and start the http server listening over TLS
func (t *Tracker) listenAPI() {
	log.WithFields(log.Fields{
		"listen_host": conf.Config.ListenHostAPI,
		"tls":         true,
	}).Info("Loading API route handlers")

	router := gin.Default()
	//	var handler *gin.RouterGroup
	//	// Optionally enabled BasicAuth over the TLS only API
	//	if conf.Config.APIUsername == "" || conf.Config.APIPassword == "" {
	//		log.Warn("No credentials set for API. All users granted access.")
	//	} else {
	//		handler = router.Group("/api", gin.BasicAuth(gin.Accounts{
	//			conf.Config.APIUsername: conf.Config.APIPassword,
	//		}))
	//	}

	router.Use(handleApiErrors)

	api := router.Group("/api")
	api.GET("/version", t.HandleVersion)
	api.GET("/uptime", t.HandleUptime)
	api.GET("/torrent/:info_hash", t.HandleTorrentGet)
	api.POST("/torrent", t.HandleTorrentAdd)
	api.GET("/torrent/:info_hash/peers", t.HandleGetTorrentPeers)
	api.DELETE("/torrent/:info_hash", t.HandleTorrentDel)

	api.POST("/user", t.HandleUserCreate)
	api.GET("/user/:user_id", t.HandleUserGet)
	api.POST("/user/:user_id", t.HandleUserUpdate)
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
