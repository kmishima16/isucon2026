package isureal

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type HandlerContext struct {
	DB        *sqlx.DB
	Config    *Config
	IPBans    *IPBanList
	ImageData *ImageDataCache
	Windows   *PostingWindowHub
	// 利用者の写しで検索する台だけ（search_snapshot.go）
	SearchUsers *searchSnapshot
}

func NewServer(config *Config) *echo.Echo {
	if !validRegion(config.Region) {
		panic("invalid server region: " + config.Region)
	}
	e := echo.New()
	e.IPExtractor = extractClientIP

	e.Use(ActionLogMiddleware(config.LogLevel, os.Stdout))
	e.Use(middleware.Recover())
	e.Use(DrainRequestBodyMiddleware())
	e.HTTPErrorHandler = APIErrorHandler

	db, err := sqlx.Open("pgx", config.DatabaseDSN())
	if err != nil {
		panic("failed to open database: " + err.Error())
	}
	// PostgreSQL の max_connections は 200。アプリが 4 台になったので、台ごとの本数を systemd の drop-in（server/<台>/）で決める
	maxConns, err := strconv.Atoi(envOrDefault("ISUREAL_DB_MAX_CONNS", "30"))
	if err != nil || maxConns <= 0 {
		panic("invalid ISUREAL_DB_MAX_CONNS")
	}
	db.SetMaxOpenConns(maxConns)
	// 既定では待機させる接続が 2 本だけで、負荷の山のたびに残りを閉じては張り直していた（PostgreSQL のプロセスの起動と認証が毎回 DB の CPU を使い、遠い台では往復も重なる）
	db.SetMaxIdleConns(maxConns)
	// 3 台が 30 本ずつ持ち続けると max_connections 100 の残りが数本になり、初期化の psql が too many clients で落ちうる。空いた接続は 1 分で返す
	db.SetConnMaxIdleTime(1 * time.Minute)
	// DB まで 1 往復 100ms の europe1・america1 では、ベンチの合間に閉じた接続を負荷の始めに張り直すと、TCP と認証で数往復かかる。
	// 本数は台ごとに決めて max_connections に収めてあるので（server/<台>/）、閉じずに持ち続け、起動時に全部開いておく
	keepConns := envOrDefault("ISUREAL_DB_KEEP_CONNS", "") == "1"
	if keepConns {
		db.SetConnMaxIdleTime(0)
	}
	err = db.Ping()
	if err != nil {
		panic("failed to ping database: " + err.Error())
	}
	if keepConns {
		go openAllConns(db, maxConns)
	}

	handlerContext := &HandlerContext{
		DB:        db,
		Config:    config,
		IPBans:    &IPBanList{},
		ImageData: newImageDataCache(),
		Windows:   newPostingWindowHub(),
	}
	// asia1 の署名の計算を europe1・america1 に頼む（image_signer.go）。ほかの台では nil
	imageSigner = newRemoteSigner(signerPeersForRegion(config.Region))
	go handlerContext.Windows.poll(db, config.Region)
	if searchUsersInMemory {
		handlerContext.SearchUsers = newSearchSnapshot()
		go handlerContext.SearchUsers.refreshLoop(db)
	}
	localWindowHub.Store(handlerContext.Windows)
	configureImageStore(config.Region, handlerContext.ImageData)
	configureSessionCache()
	e.Use(IPBanMiddleware(handlerContext))
	e.Use(LowPriorityLimitMiddleware(newLowPriorityLimiterFromEnv()))

	// deploy の疎通確認用。nginx の無い asia2 でもアプリを動かすようになり、nginx の index.html では確かめられないため。nginx は /internal を通さない
	e.GET("/internal/health", func(c *echo.Context) error { return c.String(200, "ok") })
	e.POST("/api/initialize", initializeHandler(handlerContext))
	e.POST("/api/auth/register", registerHandler(handlerContext))
	e.POST("/api/auth/login", loginHandler(handlerContext))
	e.POST("/api/auth/logout", logoutHandler(handlerContext), SessionMiddleware(handlerContext, true))
	e.GET("/api/users", searchUsersHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/users/:userId", getUserHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/me/export", exportPostsHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/me", getMeHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.PATCH("/api/me", updateMeHandler(handlerContext), SessionMiddleware(handlerContext, true))
	e.PUT("/api/me/profile-image", putProfileImageHandler(handlerContext), SessionMiddleware(handlerContext, true))
	e.DELETE("/api/me/profile-image", deleteProfileImageHandler(handlerContext), SessionMiddleware(handlerContext, true))
	e.POST("/api/me/account-deletion", deleteAccountHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/posts", createPostHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/posting-window/current", getCurrentPostingWindowHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/posts/me/current", getMyCurrentPostHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/posts/:postId", getPostHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.PATCH("/api/posts/:postId", updatePostHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/posts/:postId", deletePostHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/feeds/:feed", getFeedHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/memories", getMemoriesHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/memories/:date", getMemoriesByDateHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/posts/:postId/comments", createCommentHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/posts/:postId/comments", getCommentsHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/comments/:commentId", deleteCommentHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/posts/:postId/realmojis", replaceRealMojiHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/posts/:postId/realmojis", getRealMojisHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/realmojis/:realMojiId", deleteRealMojiHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/images/:imageId", getImageHandler(handlerContext))
	e.GET("/api/notifications", getNotificationsHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/friend-requests", createFriendRequestHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/friend-requests/:requestId/:action", transitionFriendRequestHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.GET("/api/relationships/:kind", getRelationshipsHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/friendships/:userId", deleteFriendshipHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.PUT("/api/follows/:userId", followHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/follows/:userId", unfollowHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.PUT("/api/blocks/:userId", blockHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.DELETE("/api/blocks/:userId", unblockHandler(handlerContext), SessionMiddleware(handlerContext, true), RegionMiddleware(handlerContext))
	e.POST("/api/admin/auth/login", adminLoginHandler(handlerContext))
	e.POST("/api/admin/auth/logout", adminLogoutHandler(handlerContext), AdminSessionMiddleware(handlerContext, true))
	e.PUT("/api/admin/users/:userId/certification", certifyUserHandler(handlerContext), AdminSessionMiddleware(handlerContext, true))
	e.DELETE("/api/admin/users/:userId/certification", deleteUserCertificationHandler(handlerContext), AdminSessionMiddleware(handlerContext, true))
	e.POST("/api/admin/posting-window", openPostingWindowHandler(handlerContext), AdminSessionMiddleware(handlerContext, true))

	return e
}

// openAllConns は、接続を n 本まで同時に開いて待機させる（張る往復を負荷の前に済ませる）
func openAllConns(db *sqlx.DB, n int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conns := make(chan *sql.Conn, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			conn, err := db.Conn(ctx)
			if err != nil {
				log.Printf("failed to open a database connection: %v", err)
				return
			}
			conns <- conn
		})
	}
	wg.Wait()
	close(conns)
	for conn := range conns {
		_ = conn.Close()
	}
}
