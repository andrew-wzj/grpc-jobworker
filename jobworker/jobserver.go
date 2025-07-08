package jobworker

import (
	"context"
	"net/http"
	"os"

	"jobworker/db"
	"jobworker/proto"

	"github.com/gin-gonic/gin"
)

func SetupHTTPRoutes(r *gin.Engine, worker *JobWorker) {
	r.POST("/run", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
			Cmd  string `json:"cmd"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		sessionID, err := worker.Run(req.Name, req.Cmd)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"session_id": sessionID})
	})

	r.POST("/stop", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		err := worker.Stop(req.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "job stopped"})
	})

	r.GET("/list", func(c *gin.Context) {
		jobs := db.ListJobsWithStatus()
		c.JSON(http.StatusOK, jobs)
	})

	r.GET("/log/:id", func(c *gin.Context) {
		id := c.Param("id")
		logPath, err := db.GetLogPath(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "log path not found"})
			return
		}

		content, err := os.ReadFile(logPath)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "log not found"})
			return
		}

		c.Data(http.StatusOK, "text/plain", content)
	})
}

type JobServer struct {
	Worker *JobWorker
}

func (j *JobServer) Run(ctx context.Context, request *proto.RunRequest) (any, any) {
	panic("unimplemented")
}

func NewJobServer(worker *JobWorker) *JobServer {
	return &JobServer{Worker: worker}
}
