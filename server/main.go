package main

import (
	"fmt"
	"jobworker/db"
	"jobworker/jobworker"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化数据库
	db.Init()

	// 创建 JobWorker 实例
	worker := jobworker.NewJobWorker()

	// CLI 模式
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			if len(os.Args) < 4 {
				fmt.Println("Usage: jobrunner run <name> <cmd>")
				return
			}
			id, err := worker.Run(os.Args[3], os.Args[2])
			if err != nil {
				fmt.Printf("❌ %v\n", err)
			} else {
				fmt.Printf("🚀 Job started with ID: %s\n", id)
			}
		case "stop":
			if len(os.Args) < 3 {
				fmt.Println("Usage: jobrunner stop <jobID>")
				return
			}
			err := worker.Stop(os.Args[2])
			if err != nil {
				fmt.Printf("❌ %v\n", err)
			} else {
				fmt.Println("🛑 Job stopped.")
			}
		case "list":
			jobs, err := worker.List()
			if err != nil {
				fmt.Printf("❌ Failed to list jobs: %v\n", err)
				return
			}
			for _, job := range jobs {
				fmt.Printf("📝 [%s] %s - %s\n", job.ID, job.Name, job.Status)
			}
		case "log":
			if len(os.Args) < 3 {
				fmt.Println("Usage: jobrunner log <jobID>")
				return
			}
			err := worker.ShowLog(os.Args[2])
			if err != nil {
				fmt.Printf("❌ %v\n", err)
			}
		case "serve":
			startHTTPServer(worker)
		default:
			fmt.Println("Unknown command. Available: run, stop, list, log, serve")
		}
		return
	}

	// 默认运行 HTTP Server
	startHTTPServer(worker)
}

// 启动 HTTP API Server
func startHTTPServer(worker *jobworker.JobWorker) {
	r := gin.Default()
	r.Use(cors.Default()) // 支持跨域请求

	// 注册 API 路由
	jobworker.SetupHTTPRoutes(r, worker)

	// 健康检查
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// 设置默认首页（打开 / 自动跳到 index.html）
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/web/index.html")
	})

	// 提供静态文件（前端页面）
	r.Static("/web", "./web")

	// 页面跳转：例如 /viewlog/abc123 -> /web/log.html?id=abc123
	r.GET("/viewlog/:id", func(c *gin.Context) {
		id := c.Param("id")
		c.Redirect(302, "/web/log.html?id="+id)
	})

	// 提供日志内容 API（前端 JS 用于读取日志）
	r.GET("/api/log/:id", func(c *gin.Context) {
		id := c.Param("id")
		logPath, err := db.GetLogPath(id)
		if err != nil {
			c.String(404, "Log not found")
			return
		}
		content, err := os.ReadFile(logPath)
		if err != nil {
			c.String(500, "Failed to read log")
			return
		}
		c.String(200, string(content))
	})

	fmt.Println("🚦 HTTP API server running at http://localhost:8080")
	r.Run(":8080")
}
