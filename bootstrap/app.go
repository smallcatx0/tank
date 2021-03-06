package bootstrap

import (
	"context"
	"errors"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

type App struct {
	HttpServ  *http.Server
	GinEngibe *gin.Engine
	// 还可以挂载些其他服务 如定时任务之类的
}

func NewApp(debug bool) *App {
	rand.Seed(time.Now().UnixNano())
	app := new(App)
	if debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	app.GinEngibe = gin.Default()
	return app
}

func (app *App) Use(fc ...func()) {
	for _, f := range fc {
		f()
	}
}

func (app *App) RegisterRoutes(registeRoutes func(*gin.Engine)) {
	registeRoutes(app.GinEngibe)
}

func (app *App) Run(port string) {
	app.HttpServ = &http.Server{
		Addr:    ":" + port,
		Handler: app.GinEngibe,
	}
	go func() {
		log.Println("[http-serv]", "binding at", app.HttpServ.Addr)
		err := app.HttpServ.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Panic("[http-serv]", err)
		}
	}()

}

func (app *App) Stop() {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	app.HttpServ.Shutdown(ctx)
}

func (app *App) WaitExit(funs ...func()) {
	fs := append([]func(){app.Stop}, funs...)
	waitExit(fs...)
}

func waitExit(funs ...func()) {
	quit := make(chan os.Signal, 4)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	switch <-quit {
	case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
		log.Printf("Shutdown quickly, bye...")
	case syscall.SIGHUP:
		log.Printf("Shutdown gracefully, bye...")
		// 处理各种服务的优雅关闭
		for _, f := range funs {
			if f != nil {
				f()
			}
		}
	}
	os.Exit(0)
}

var (
	Param struct {
		C string
		H bool
	}
)

func InitFlag() {
	flag.StringVar(&Param.C, "config", "conf/app.yaml", "配置文件地址")
	flag.BoolVar(&Param.H, "help", false, "帮助")
}

func Flag() bool {
	flag.Parse()
	if Param.H {
		flag.PrintDefaults()
		return false
	}
	// 存到viper中
	return true
}
